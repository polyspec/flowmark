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
	lay := &filesLayout{table: table, fks: map[string]string{}}
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
// whose bytes are data. A row is returned unless a diagnostic makes the file
// unreadable; a noncanonical file returns its row with the diagnostics, and
// emitting the row gives the canonical file.
func ParseFile(cat *catalog.Catalog, table, rel string, data []byte, lk Linker) (*Row, []Diagnostic) {
	lay, err := layoutOf(cat, table)
	if err != nil {
		return nil, []Diagnostic{fileDiag(rel, RuleWalk, err.Error())}
	}
	key, diags := keyOfFile(lay, rel)
	if diags != nil {
		return nil, diags
	}
	p := &parser{lay: lay, rel: rel, lk: lk}
	text, ok := p.normalize(data)
	if !ok {
		return nil, p.diags
	}
	lines := splitLines(text)
	row := &Row{Table: table, Key: key, Scalars: map[string]*string{}, Sections: map[string]*string{}}
	if p.readTitle(lines, row) {
		i := p.readFields(lines, row)
		b := p.readBody(lines, i, row)
		p.readSections(lines, b, row)
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
	lay   *filesLayout
	rel   string
	lk    Linker
	diags []Diagnostic
	fatal bool
}

func (p *parser) fail(line, col int, rule, msg string) {
	p.fatal = true
	p.diags = append(p.diags, Diagnostic{File: p.rel, Line: line, Column: col, Rule: rule, Message: msg})
}

func (p *parser) warn(line, col int, rule, msg string) {
	p.diags = append(p.diags, Diagnostic{File: p.rel, Line: line, Column: col, Rule: rule, Message: msg})
}

// normalize checks the encoding and returns the text with LF line endings and
// a final line feed. Each departure from the canonical bytes is a diagnostic.
func (p *parser) normalize(data []byte) (string, bool) {
	data = bytes.Clone(data)
	if !utf8.Valid(data) {
		line, col := invalidAt(data)
		p.fail(line, col, RuleEncoding, "the file is not UTF-8")
		return "", false
	}
	if bytes.HasPrefix(data, bom) {
		p.warn(1, 1, RuleNoncanonical, "the file begins with a byte order mark; the canonical file has none")
		data = data[len(bom):]
	}
	if i := bytes.Index(data, []byte("\r\n")); i >= 0 {
		p.warn(lineOf(data, i), colOf(data, i), RuleNoncanonical, "CRLF line ending; the canonical line ending is LF")
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	}
	if n := len(data); n > 0 && data[n-1] != '\n' {
		p.warn(lineOf(data, n), colOf(data, n), RuleNoncanonical, "the last line has no line feed")
		data = append(data, '\n')
	}
	return string(data), true
}

// readTitle reads the H1 line, which is the title of the row.
func (p *parser) readTitle(lines []string, row *Row) bool {
	if len(lines) == 0 {
		p.fail(1, 1, RuleBlock, "the file has no H1 title")
		return false
	}
	level, text, ok := headingOf(lines[0])
	if !ok || level != 1 {
		p.fail(1, 1, RuleBlock, "the first line is not the H1 title `# <title>`")
		return false
	}
	if text == "" {
		p.fail(1, 3, RuleBlock, "the title is empty")
		return false
	}
	if f := singleLineFault(text); f != "" {
		p.fail(1, 3, RuleBlock, "the title has "+f)
		return false
	}
	row.Title = unescapeTitle(text)
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
		valueCol := len("- "+name+": ") + 1
		if v, ok := p.fieldValue(lineNo, lay.scalars[idx], raw, valueCol); ok {
			row.Scalars[name] = &v
		}
	}
	for _, c := range lay.scalars {
		if !c.Null && !seen[c.Name] {
			p.fail(i+1, 1, RuleBlock, "the non-null field "+c.Name+" is missing")
		}
	}
	switch {
	case i < len(lines) && lines[i] == "":
		i++
	case i > start && i < len(lines):
		p.warn(i+1, 1, RuleNoncanonical, "the field list is followed by a blank line")
	}
	return i
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
// lineNo whose value starts at column valueCol.
func (p *parser) fieldValue(lineNo int, c dbspec.Column, raw string, valueCol int) (string, bool) {
	if table, ok := p.lay.fks[c.Name]; ok {
		key, link, ok := splitLink(raw)
		if !ok {
			p.fail(lineNo, valueCol, RuleBlock, "a foreign key is a link [key](target)")
			return "", false
		}
		want, err := p.lk.Target(p.rel, table, key)
		if err != nil {
			p.fail(lineNo, valueCol, RuleBlock, "dangling reference: "+err.Error())
			return "", false
		}
		if link != want {
			p.warn(lineNo, valueCol, RuleNoncanonical, "the link target is "+want)
		}
		return key, true
	}
	if f := singleLineFault(raw); f != "" {
		p.fail(lineNo, valueCol, RuleBlock, "the value has "+f)
		return "", false
	}
	canon, ok := readable(c.Type, raw)
	if !ok {
		p.fail(lineNo, valueCol, RuleBlock, "the value is not a "+c.Type.String()+" value")
		return "", false
	}
	if canon != raw {
		p.warn(lineNo, valueCol, RuleNoncanonical, "the value is written "+canon)
	}
	return canon, true
}

// readBody reads the body, which runs from line i to the first heading of
// level 2 or less outside a fence, and returns the index of that heading.
func (p *parser) readBody(lines []string, i int, row *Row) int {
	b, _ := boundary(lines, i)
	region := lines[i:b]
	if b < len(lines) && len(region) > 0 && region[len(region)-1] == "" {
		region = region[:len(region)-1]
	}
	if len(region) > 0 {
		if isListItem(region[0]) {
			p.fail(i+1, 1, RuleBlock, "the body begins with a list item")
		}
		row.Body = joinLines(region)
	}
	return b
}

// readSections reads the sections of the row from line b, each a level-2
// heading that names a declared markdown column.
func (p *parser) readSections(lines []string, b int, row *Row) {
	seen := map[string]bool{}
	for b < len(lines) {
		headLine := b + 1
		level, text, _ := headingOf(lines[b])
		name := strings.TrimSpace(text)
		s := b + 1
		if s < len(lines) && lines[s] == "" {
			s++
		}
		next, _ := boundary(lines, s)
		region := lines[s:next]
		if next < len(lines) && len(region) > 0 && region[len(region)-1] == "" {
			region = region[:len(region)-1]
		}
		b = next
		_, declared := p.lay.section(name)
		switch {
		case level != 2:
			p.fail(headLine, 1, RuleSection, "a heading of level "+strconv.Itoa(level)+" is not a section of the row")
		case !declared:
			p.fail(headLine, 1, RuleSection, "## "+name+" is not a section of "+p.lay.table)
		case seen[name]:
			p.fail(headLine, 1, RuleSection, "the section "+name+" is written twice")
		default:
			seen[name] = true
			if text != name {
				p.warn(headLine, 1, RuleNoncanonical, "the section heading has extra spaces")
			}
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
// refuses returns no bytes and CONSTRAINT diagnostics located in the file it
// would write.
func RenderFile(cat *catalog.Catalog, table string, row *Row, lk Linker) ([]byte, []Diagnostic) {
	lay, err := layoutOf(cat, table)
	if err != nil {
		return nil, []Diagnostic{fileDiag(table, RuleWalk, err.Error())}
	}
	return renderRow(lay, row, lk)
}

// writer collects the file being written and the refusals of its values.
type writer struct {
	rel   string
	b     strings.Builder
	lines int
	diags []Diagnostic
}

func (w *writer) refuse(line, col int, msg string) {
	w.diags = append(w.diags, Diagnostic{File: w.rel, Line: line, Column: col, Rule: RuleConstraint, Message: msg})
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
		if text, ok := w.fieldText(lay, c, *v, line, lk); ok {
			fields = append(fields, text)
		}
	}
	for _, f := range fields {
		w.put(f + "\n")
	}

	type item struct {
		heading string
		value   string
		body    bool
	}
	var items []item
	if row.Body != "" {
		items = append(items, item{value: row.Body, body: true})
	}
	for _, s := range lay.sections {
		v := row.Sections[s.name]
		if v == nil {
			if !s.nullable {
				w.refuse(w.next(), 1, "the non-null section "+s.name+" has no value")
			}
			continue
		}
		items = append(items, item{heading: s.name, value: *v})
	}
	for k, it := range items {
		w.put("\n")
		followed := k < len(items)-1
		if it.body {
			w.value("the body", it.value, w.next(), followed, true)
			w.put(it.value)
			continue
		}
		w.put("## " + it.heading + "\n")
		if it.value != "" {
			w.put("\n")
			w.value("the section "+it.heading, it.value, w.next(), followed, false)
			w.put(it.value)
		}
	}
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
	case singleLineFault(title) != "":
		w.refuse(1, 3, "the title has "+singleLineFault(title))
	case unescapeTitle(escapeTitle(title)) != title:
		w.refuse(1, 3, "the title cannot be written: its H1 reads back as "+unescapeTitle(escapeTitle(title)))
	case !utf8.ValidString(title):
		w.refuse(1, 3, "the title is not UTF-8")
	}
	w.put("# " + escapeTitle(title) + "\n")
}

// fieldText returns the field item of a stored scalar value, "- <column>: <value>".
func (w *writer) fieldText(lay *filesLayout, c dbspec.Column, v string, line int, lk Linker) (string, bool) {
	valueCol := len("- "+c.Name+": ") + 1
	var text string
	if table, ok := lay.fks[c.Name]; ok {
		target, err := lk.Target(w.rel, table, v)
		if err != nil {
			w.refuse(line, valueCol, "the foreign key names no row: "+err.Error())
			return "", false
		}
		text = "[" + v + "](" + target + ")"
	} else {
		switch {
		case v == "now":
			w.refuse(line, valueCol, "now is refused as a value")
			return "", false
		case !utf8.ValidString(v):
			w.refuse(line, valueCol, "the value is not UTF-8")
			return "", false
		}
		canon, ok := readable(c.Type, v)
		if !ok {
			w.refuse(line, valueCol, "the value is not a "+c.Type.String()+" value")
			return "", false
		}
		if canon != v {
			w.refuse(line, valueCol, "the value is not canonical; it is written "+canon)
			return "", false
		}
		if f := singleLineFault(v); f != "" {
			w.refuse(line, valueCol, "the value has "+f)
			return "", false
		}
		text = v
	}
	if text == "" {
		return "- " + c.Name + ":", true
	}
	return "- " + c.Name + ": " + text, true
}

// value checks a body or section value that is written at line. A value ends
// with a line feed, holds no heading of level 2 or less outside a fence, and
// a body does not begin with a list item; an open fence is refused when more
// follows it, since the next heading would be read inside the fence.
func (w *writer) value(name, v string, line int, followed, body bool) {
	if v == "" {
		return
	}
	switch {
	case !utf8.ValidString(v):
		w.refuse(line, 1, name+" is not UTF-8")
		return
	case strings.Contains(v, "\r\n"):
		w.refuse(line, 1, name+" holds a CRLF line ending")
		return
	case !strings.HasSuffix(v, "\n"):
		w.refuse(line, 1, name+" must end with a line feed")
		return
	}
	lines := splitLines(v)
	if body && isListItem(lines[0]) {
		w.refuse(line, 1, "the body begins with a list item")
	}
	idx, open := boundary(lines, 0)
	if idx < len(lines) {
		w.refuse(line+idx, 1, name+" holds a heading of level 2 or less")
	}
	if open && followed {
		w.refuse(line, 1, name+" opens a fence that would hide the next heading")
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
