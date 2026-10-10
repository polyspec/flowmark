package store

import (
	"bytes"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/orm/engine/dbspec"
)

// fileSuffix is the suffix of a corpus document.
const fileSuffix = ".flowmark.md"

// bom is the UTF-8 byte order mark, which the canonical file does not have.
var bom = []byte("\xef\xbb\xbf")

// filesLayout is the storage of one files table: its key, its stored scalar
// columns in column order, its markdown sections in column order and its
// single-column foreign keys.
type filesLayout struct {
	table    string
	key      dbspec.Column
	scalars  []dbspec.Column
	sections []section
	fks      map[string]string
	scope    int          // the row scope level of the file: its sections are at this level
	children []*docLayout // the block tables stored in the file, by table name
}

// childNamed returns the child block of the table whose name is name, or nil.
func (l *filesLayout) childNamed(name string) *docLayout {
	for _, c := range l.children {
		if c.table == name {
			return c
		}
	}
	return nil
}

// section is a markdown column other than the body; a nullable one may be absent.
type section struct {
	name     string
	nullable bool
}

// layoutOf returns the layout of table, which must be a files table of cat.
func layoutOf(cat *catalog.Catalog, table string) (*filesLayout, error) {
	tb, ok := cat.Table(table)
	if !ok {
		return nil, fmt.Errorf("table %s is not in the catalog", table)
	}
	if tb.Storage.Kind != catalog.StorageFiles {
		return nil, fmt.Errorf("table %s is not a files table", table)
	}
	lay := &filesLayout{table: table, fks: map[string]string{}, scope: tb.RowScope}
	markdown := map[string]bool{}
	for _, m := range tb.Markdown {
		markdown[m] = true
	}
	keyFound := false
	for _, c := range tb.Columns {
		switch {
		case c.Name == tb.Key:
			lay.key, keyFound = c, true
		case c.Name == tb.Title || c.Name == tb.Body:
		case markdown[c.Name]:
			lay.sections = append(lay.sections, section{name: c.Name, nullable: c.Null})
		default:
			lay.scalars = append(lay.scalars, c)
		}
	}
	if !keyFound {
		return nil, fmt.Errorf("table %s has no key column", table)
	}
	for _, fk := range tb.ForeignKeys {
		if len(fk.Columns) == 1 {
			lay.fks[fk.Columns[0]] = fk.Table
		}
	}
	children, err := childLayouts(cat, table)
	if err != nil {
		return nil, err
	}
	lay.children = children
	return lay, nil
}

func (l *filesLayout) scalarIndex(name string) int {
	for i, c := range l.scalars {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func (l *filesLayout) section(name string) (section, bool) {
	for _, s := range l.sections {
		if s.name == name {
			return s, true
		}
	}
	return section{}, false
}

// fileDiag locates a file-level diagnostic at the first line of a file.
func fileDiag(file, rule, msg string) Diagnostic {
	return Diagnostic{File: file, Line: 1, Column: 1, Rule: rule, Message: msg}
}

// keyOfFile reads the key of the file rel, which must be <table>/<key>.flowmark.md.
func keyOfFile(lay *filesLayout, rel string) (string, []Diagnostic) {
	fail := func(rule, msg string) (string, []Diagnostic) {
		return "", []Diagnostic{fileDiag(rel, rule, msg)}
	}
	if path.Dir(rel) != lay.table {
		return fail(RuleWalk, "the file is outside the folder "+lay.table+"/")
	}
	base := path.Base(rel)
	if !strings.HasSuffix(base, fileSuffix) {
		return fail(RuleWalk, "the file is not a "+fileSuffix+" document")
	}
	key := strings.TrimSuffix(base, fileSuffix)
	if msg := keyFault(key); msg != "" {
		return fail(RuleBlock, msg)
	}
	if canon, ok := readable(lay.key.Type, key); !ok || canon != key {
		return fail(RuleBlock, "the file name is not the canonical text of a "+lay.key.Type.String()+" key")
	}
	return key, nil
}

// ParseFile reads the file rel of a files table, <table>/<key>.flowmark.md,
// whose bytes are data. A row is returned unless a format diagnostic makes the
// file unreadable; a noncanonical file returns its row with the diagnostics, and
// emitting the row gives the canonical file. A scalar that is not of its column
// type is kept as its raw text in Row.Raw: the validator reports its type.
func ParseFile(cat *catalog.Catalog, table, rel string, data []byte, lk Linker) (*Row, []Diagnostic) {
	lay, err := layoutOf(cat, table)
	if err != nil {
		return nil, []Diagnostic{fileDiag(rel, RuleWalk, err.Error())}
	}
	key, diags := keyOfFile(lay, rel)
	if diags != nil {
		return nil, diags
	}
	p := &parser{sink: sink{rel: rel}, lay: lay, lk: lk}
	text, ok := p.normalize(data)
	if !ok {
		return nil, p.diags
	}
	lines := splitLines(text)
	row := &Row{
		Table: table, Key: key,
		Scalars:  map[string]*string{},
		Sections: map[string]*string{},
		Raw:      map[string]string{},
		Dangling: map[string]DanglingLink{},
	}
	if p.readTitle(lines, row) {
		i := p.readFields(lines, row)
		b := p.readBody(lines, i, row)
		dp := newDocParser(&p.sink, rel, lk)
		p.readSections(lines, b, row, dp)
		dp.resolveLinks()
	}
	for _, c := range anchorClashes(lines, tableLevels(lay.table, 0, lay.children)) {
		p.fail(c.line, 1, RuleBlock, c.message())
	}
	if p.fatal {
		return nil, p.diags
	}
	out, rd := renderRow(lay, row, lk)
	if len(rd) > 0 {
		return nil, append(p.diags, rd...)
	}
	if !hasRule(p.diags, RuleNoncanonical) && !bytes.Equal(out, []byte(text)) {
		line, col := diffAt([]byte(text), out)
		p.warn(line, col, RuleNoncanonical, "the file is not in its canonical form")
	}
	return row, p.diags
}

// parser reads one file. A fatal diagnostic makes the file unreadable; a
// warning marks a noncanonical file whose row is still returned.
type parser struct {
	sink
	lay    *filesLayout
	lk     Linker
	fields int // the number of field-list items read
}

// readTitle reads the H1 line, which is the title of the row. The title is read
// by unescapeTitle; the line is noncanonical when the write rule would spell it
// otherwise, which covers white space around the title and a closing sequence.
func (p *parser) readTitle(lines []string, row *Row) bool {
	if len(lines) == 0 {
		p.fail(1, 1, RuleBlock, "the file has no H1 title")
		return false
	}
	level, text, _, ok := headingOf(lines[0])
	if !ok || level != 1 {
		p.fail(1, 1, RuleBlock, "the first line is not the H1 title `# <title>`")
		return false
	}
	title := unescapeTitle(text)
	switch {
	case title == "":
		p.fail(1, 3, RuleBlock, "the title is empty")
		return false
	case singleLineFault(title) != "":
		p.fail(1, 3, RuleBlock, "the title has "+singleLineFault(title))
		return false
	case unescapeTitle(escapeTitle(title)) != title:
		p.fail(1, 3, RuleBlock, "the title cannot be written: it would read back as "+unescapeTitle(escapeTitle(title)))
		return false
	}
	row.Title = title
	if want := "# " + escapeTitle(title); lines[0] != want {
		p.warn(1, 3, RuleNoncanonical, "the H1 title is written as "+want)
	}
	return true
}

// readFields reads the field list directly under the H1 and its blank line,
// and returns the index of the first line after the header.
func (p *parser) readFields(lines []string, row *Row) int {
	lay := p.lay
	i := 1
	start := i
	last := -1
	seen := map[string]bool{}
	for ; i < len(lines) && strings.HasPrefix(lines[i], "- "); i++ {
		lineNo := i + 1
		name, raw, ok := splitField(lines[i])
		if !ok {
			p.fail(lineNo, 1, RuleBlock, "a field is `- <column>: <value>`")
			continue
		}
		idx := lay.scalarIndex(name)
		switch {
		case idx < 0:
			p.fail(lineNo, 3, RuleBlock, fmt.Sprintf("%s is not a stored scalar column of %s", name, lay.table))
			continue
		case idx <= last:
			p.fail(lineNo, 3, RuleBlock, fmt.Sprintf("field %s is out of column order", name))
			continue
		}
		last = idx
		seen[name] = true
		p.fields++
		valueCol := len("- "+name+": ") + 1
		p.fieldValue(lineNo, lay.scalars[idx], raw, valueCol, row)
	}
	for _, c := range lay.scalars {
		if !c.Null && !seen[c.Name] {
			p.fail(i+1, 1, RuleBlock, "the non-null field "+c.Name+" is missing")
		}
	}
	switch {
	case i < len(lines) && lines[i] == "":
		i++
	case i > start && i < len(lines) && isATXHeading(lines[i]):
		p.warn(i+1, 1, RuleNoncanonical, "the field list is followed by a section without a blank line")
	case i > start && i < len(lines):
		p.fail(i+1, 1, RuleBlock, "a body line directly after the field list is read as part of its last item; the blank line is missing")
	}
	return i
}

// isATXHeading reports whether line is an ATX heading of any level.
func isATXHeading(line string) bool {
	_, _, _, ok := headingOf(line)
	return ok
}

// splitField splits a field item "- <name>: <value>" into its name and its raw
// value; the value is empty when nothing follows the colon.
func splitField(line string) (name, raw string, ok bool) {
	rest := line[len("- "):]
	c := strings.IndexByte(rest, ':')
	if c < 0 {
		return "", "", false
	}
	name, after := rest[:c], rest[c+1:]
	switch {
	case after == "":
		return name, "", true
	case after[0] == ' ':
		return name, after[1:], true
	}
	return "", "", false
}

// fieldValue reads the value of a stored scalar column in its field, at line
// lineNo whose value starts at column valueCol. White space that rendering drops
// around the value is noncanonical; a foreign key whose row is absent keeps its
// written target; a value not of its column type is kept raw.
func (p *parser) fieldValue(lineNo int, c dbspec.Column, raw string, valueCol int, row *Row) {
	v := trimWhite(raw)
	if v != raw {
		p.warn(lineNo, valueCol, RuleNoncanonical, "the white space around the value is dropped by rendering")
	}
	if table, isFK := p.lay.fks[c.Name]; isFK {
		key, target, ok := splitLink(v)
		if !ok {
			p.fail(lineNo, valueCol, RuleBlock, "a foreign key is a link [key](target)")
			return
		}
		want, err := p.lk.Target(p.rel, table, key)
		if err != nil {
			row.Dangling[c.Name] = DanglingLink{Key: key, Target: target}
		} else if target != want {
			p.warn(lineNo, valueCol, RuleNoncanonical, "the link target is "+want)
		}
		row.Scalars[c.Name] = &key
		return
	}
	if f := singleLineFault(v); f != "" {
		p.fail(lineNo, valueCol, RuleBlock, "the value has "+f)
		return
	}
	canon, ok := readable(c.Type, v)
	if !ok {
		row.Raw[c.Name] = v
		row.Scalars[c.Name] = &v
		return
	}
	if canon != v {
		p.warn(lineNo, valueCol, RuleNoncanonical, "the value is written "+canon)
	}
	row.Scalars[c.Name] = &canon
}

// readBody reads the body, which runs from line i to the next heading of level 2
// or less outside a fence or an HTML block, and returns the index of that
// heading. A body begins with neither a list item nor, after a field list, an
// indented line.
func (p *parser) readBody(lines []string, i int, row *Row) int {
	b := boundary(lines, i)
	region, start := p.valueLines(lines, i, b, false)
	if len(region) > 0 {
		switch {
		case isListItem(region[0]):
			p.fail(start+1, 1, RuleBlock, "the body begins with a list item")
		case p.fields > 0 && indented(region[0]):
			p.fail(start+1, 1, RuleBlock, "the body begins with an indented line after the field list")
		}
		row.Body = joinLines(region)
	}
	return b
}

// readSections reads the sections of the row from line b, each a level-2 heading
// that names a declared markdown column or a child block. A child block is read
// by dp, in its own scope, and its rows become the row's child rows.
func (p *parser) readSections(lines []string, b int, row *Row, dp *docParser) {
	var bounds []heading
	for _, h := range scanHeadings(lines, b).heads {
		if h.level <= 2 {
			bounds = append(bounds, h)
		}
	}
	seen := map[string]bool{}
	for k, h := range bounds {
		end := len(lines)
		if k+1 < len(bounds) {
			end = bounds[k+1].at
		}
		headLine := h.at + 1
		name := trimWhite(h.text)
		if child := p.lay.childNamed(name); child != nil && h.level == 2 {
			if seen[name] {
				p.fail(headLine, 1, RuleSection, "the block "+name+" is written twice")
				continue
			}
			seen[name] = true
			if h.text != name || h.tab {
				p.warn(headLine, 1, RuleNoncanonical, "the block heading is not written with one space after its hashes")
			}
			if rows := dp.readChildBlock(lines, h, end, child); len(rows) > 0 {
				setBlock(row, child.table, rows)
			}
			continue
		}
		_, declared := p.lay.section(name)
		switch {
		case h.level != 2:
			p.fail(headLine, 1, RuleSection, "a heading of level "+strconv.Itoa(h.level)+" is not a section of the row")
		case !declared:
			p.fail(headLine, 1, RuleSection, "## "+name+" is not a section of "+p.lay.table)
		case seen[name]:
			p.fail(headLine, 1, RuleSection, "the section "+name+" is written twice")
		default:
			seen[name] = true
			if h.text != name || h.tab {
				p.warn(headLine, 1, RuleNoncanonical, "the section heading is not written with one space after its hashes")
			}
			region, _ := p.valueLines(lines, h.last+1, end, true)
			v := joinLines(region)
			row.Sections[name] = &v
		}
	}
	for _, s := range p.lay.sections {
		if !s.nullable && !seen[s.name] {
			p.fail(len(lines)+1, 1, RuleBlock, "the non-null section "+s.name+" is missing")
		}
	}
}

// RenderFile writes the file of row, a row of a files table. A row the writer
// refuses returns no bytes and diagnostics located in the file it would write.
func RenderFile(cat *catalog.Catalog, table string, row *Row, lk Linker) ([]byte, []Diagnostic) {
	lay, err := layoutOf(cat, table)
	if err != nil {
		return nil, []Diagnostic{fileDiag(table, RuleWalk, err.Error())}
	}
	out, diags := renderRow(lay, row, lk)
	if len(diags) > 0 {
		return nil, diags
	}
	rel := row.Table + "/" + row.Key + fileSuffix
	if rd := anchorRefusals(rel, out, tableLevels(lay.table, 0, lay.children)); len(rd) > 0 {
		return nil, rd
	}
	return out, nil
}

// writer collects the file being written and the refusals of its values. An element
// is a heading or a value; the framing of a value depends on the element that
// follows it, so each value is checked once the file is complete.
type writer struct {
	rel    string
	b      strings.Builder
	lines  int
	elems  int
	values []pendingValue
	diags  []Diagnostic
}

// pendingValue is a body or a section that the writer has written, to be checked
// once the file is complete. k is the number of elements before it.
type pendingValue struct {
	name        string
	v           string
	line        int // the line the value starts at
	level       int // the row scope level: a heading of this level or less ends the value
	body        bool
	afterFields bool
	k           int
}

// elem counts a heading as an element of the file.
func (w *writer) elem() { w.elems++ }

// finish checks the values written, each followed by an element when one was written after it.
func (w *writer) finish() {
	for _, p := range w.values {
		w.checkValue(p, w.elems > p.k+1)
	}
}

func (w *writer) refuse(line, col int, msg string) {
	w.refuseRule(RuleConstraint, line, col, msg)
}

func (w *writer) refuseRule(rule string, line, col int, msg string) {
	w.diags = append(w.diags, Diagnostic{File: w.rel, Line: line, Column: col, Rule: rule, Message: msg})
}

func (w *writer) put(s string) {
	w.b.WriteString(s)
	w.lines += strings.Count(s, "\n")
}

// next is the line the writer writes next.
func (w *writer) next() int { return w.lines + 1 }

// renderRow writes the file of row in its canonical form.
func renderRow(lay *filesLayout, row *Row, lk Linker) ([]byte, []Diagnostic) {
	w := &writer{rel: row.Table + "/" + row.Key + fileSuffix}
	if row.Table != lay.table {
		w.refuse(1, 1, "the row is of table "+row.Table+", not "+lay.table)
		return nil, w.diags
	}
	if msg := keyFault(row.Key); msg != "" {
		w.refuse(1, 1, "the key: "+msg)
		return nil, w.diags
	}
	if canon, ok := readable(lay.key.Type, row.Key); !ok || canon != row.Key {
		w.refuse(1, 1, "the key is not the canonical text of a "+lay.key.Type.String()+" key")
		return nil, w.diags
	}
	for _, name := range sortedKeys(row.Scalars) {
		if lay.scalarIndex(name) < 0 {
			w.refuse(1, 1, name+" is not a stored scalar column of "+lay.table)
		}
	}
	for _, name := range sortedKeys(row.Sections) {
		if _, ok := lay.section(name); !ok {
			w.refuse(1, 1, name+" is not a markdown section of "+lay.table)
		}
	}

	w.writeTitle(row.Title)
	var fields []string
	written := 0
	for _, c := range lay.scalars {
		line := 2 + written
		v := row.Scalars[c.Name]
		if v == nil {
			if !c.Null {
				w.refuse(line, 1, "the non-null column "+c.Name+" has no value")
			}
			continue
		}
		written++
		if text, ok := w.fieldText(lay, row, c, *v, line, lk); ok {
			fields = append(fields, text)
		}
	}
	for _, f := range fields {
		w.put(f + "\n")
	}
	for _, s := range lay.sections {
		if row.Sections[s.name] == nil && !s.nullable {
			w.refuse(w.next(), 1, "the non-null section "+s.name+" has no value")
		}
	}
	for _, name := range sortedBlocks(row) {
		if lay.childNamed(name) == nil {
			w.refuse(1, 1, name+" is not a block of "+lay.table)
		}
	}
	dw := newDocWriter(w, lk, true)
	for _, c := range lay.children {
		dw.index(c, row.Blocks[c.table])
	}
	dw.contents(lay.scope, lay.sections, lay.children, row, true, len(fields) > 0)
	w.finish()
	if len(w.diags) > 0 {
		return nil, w.diags
	}
	return []byte(w.b.String()), nil
}

// writeTitle writes the H1 line of the title.
func (w *writer) writeTitle(title string) {
	switch {
	case title == "":
		w.refuse(1, 3, "the title is empty")
	case !utf8.ValidString(title):
		w.refuse(1, 3, "the title is not UTF-8")
	case singleLineFault(title) != "":
		w.refuse(1, 3, "the title has "+singleLineFault(title))
	case unescapeTitle(escapeTitle(title)) != title:
		w.refuse(1, 3, "the title cannot be written: its H1 would read back as "+unescapeTitle(escapeTitle(title)))
	}
	w.put("# " + escapeTitle(title) + "\n")
}

// fieldText returns the field item of a stored scalar value, "- <column>: <value>".
// A value that is not of its column type is written as its raw text only when it
// is the text the row was read with; any other value is written in its canonical
// form. A foreign key with no row keeps the target it was read with.
func (w *writer) fieldText(lay *filesLayout, row *Row, c dbspec.Column, v string, line int, lk Linker) (string, bool) {
	valueCol := len("- "+c.Name+": ") + 1
	switch {
	case !utf8.ValidString(v):
		w.refuse(line, valueCol, "the value is not UTF-8")
		return "", false
	case singleLineFault(v) != "":
		w.refuse(line, valueCol, "the value has "+singleLineFault(v))
		return "", false
	}
	var text string
	if table, ok := lay.fks[c.Name]; ok {
		if strings.ContainsAny(v, "[]") {
			w.refuse(line, valueCol, "the foreign key text holds a bracket")
			return "", false
		}
		target, err := lk.Target(w.rel, table, v)
		if err != nil {
			d, kept := row.Dangling[c.Name]
			if !kept || d.Key != v {
				w.refuseRule(RuleForeignKey, line, valueCol, "the foreign key names no row: "+err.Error())
				return "", false
			}
			target = d.Target
		}
		text = "[" + v + "](" + target + ")"
	} else if raw, kept := row.Raw[c.Name]; kept && raw == v {
		text = v
	} else {
		canon, ok := readable(c.Type, v)
		if !ok {
			w.refuse(line, valueCol, "the value is not a "+c.Type.String()+" value")
			return "", false
		}
		text = canon
	}
	if text == "" {
		return "- " + c.Name + ":", true
	}
	return "- " + c.Name + ": " + text, true
}

// value checks a body or section value that is written at line. A value ends
// with a line feed and begins and ends with a line that is not blank; it holds
// no CR, no U+0000 and no heading of the level or less outside a fence, setext
// headings included. A files body begins with neither a list item nor, after a
// field list, an indented line. A value that leaves a fence or an HTML block open
// at the top level is refused when the element that follows it would be taken
// into that block.
func (w *writer) checkValue(p pendingValue, followed bool) {
	name, v, line, body, afterFields, level := p.name, p.v, p.line, p.body, p.afterFields, p.level
	switch {
	case !utf8.ValidString(v):
		w.refuse(line, 1, name+" is not UTF-8")
		return
	case strings.Contains(v, "\r"):
		w.refuse(line, 1, name+" holds a CR")
		return
	case strings.ContainsRune(v, 0):
		w.refuse(line, 1, name+" holds a U+0000")
		return
	case !strings.HasSuffix(v, "\n"):
		w.refuse(line, 1, name+" must end with a line feed")
		return
	}
	lines := splitLines(v)
	if isBlank(lines[0]) {
		w.refuse(line, 1, name+" begins with a blank line")
	}
	if isBlank(lines[len(lines)-1]) {
		w.refuse(line+len(lines)-1, 1, name+" ends with a blank line")
	}
	if body && isListItem(lines[0]) {
		w.refuse(line, 1, "the body begins with a list item")
	}
	if body && afterFields && indented(lines[0]) {
		w.refuse(line, 1, "the body begins with an indented line after the field list")
	}
	// The element that follows is set off by one blank line, which closes an HTML
	// block of type 6 or 7 and nothing else; the value is read with that line.
	framed := lines
	if followed {
		framed = append(append([]string(nil), lines...), "")
	}
	sc := scanHeadings(framed, 0)
	for _, h := range sc.heads {
		if h.level <= level {
			if h.last != h.at {
				w.refuse(line+h.last, 1, name+" holds a setext heading")
			} else {
				w.refuse(line+h.at, 1, name+" holds a heading of level "+strconv.Itoa(level)+" or less")
			}
			break
		}
	}
	if sc.open && followed {
		w.refuse(line, 1, name+" leaves a fence or an HTML block open before the next element")
	}
}

// lineOf returns the 1-based line of byte index i of data.
func lineOf(data []byte, i int) int { return 1 + bytes.Count(data[:i], []byte("\n")) }

// colOf returns the 1-based column in code points of byte index i of data.
func colOf(data []byte, i int) int {
	start := bytes.LastIndexByte(data[:i], '\n') + 1
	return utf8.RuneCount(data[start:i]) + 1
}

// invalidAt returns the position of the first byte that is not UTF-8.
func invalidAt(data []byte) (int, int) {
	line, col := 1, 1
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			return line, col
		}
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
		i += size
	}
	return line, col
}

// diffAt returns the position in a of the first byte where a and b differ.
func diffAt(a, b []byte) (int, int) {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return lineOf(a, min(i, len(a))), colOf(a, min(i, len(a)))
}

func hasRule(diags []Diagnostic, rule string) bool {
	for _, d := range diags {
		if d.Rule == rule {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]*string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
