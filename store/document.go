package store

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/orm/engine/dbspec"
)

// docLayout is the storage of one list or table of rows as its catalog declares it:
// a document table, or a block table inside the row scope of its placement parent.
// It holds the key, the title, body and order, the state and parent, the scalar
// columns, sections and foreign keys, the row scope level, and the child blocks.
type docLayout struct {
	table     string
	list      bool
	scope     int // the level of the row scope: the level of its sections
	key       dbspec.Column
	title     string            // the title column, "" when none
	body      string            // the body column, "" when none
	order     string            // the order column, "" when none
	orderType dbspec.Type       // the type of the order column
	state     string            // the state column of a list, "" when none
	parent    string            // "parent" when the table has the self foreign key parent, "" otherwise
	placement string            // the placement foreign key of a block table, "" for a document table
	scalars   []dbspec.Column   // the stored scalar columns: every column but the key, body, order, markdown and placement columns
	fields    []dbspec.Column   // a list's fields: the scalars but its title, state and parent
	sections  []section         // the markdown columns other than the body, in column order
	fks       map[string]string // the referenced table of each foreign key
	glyphs    map[string]string // state -> glyph of a list
	states    map[string]string // glyph -> state of a list
	children  []*docLayout      // the block tables stored in the row scope of this table, by table name
	// inFiles reports that the placement chain of the table reaches a files table, so
	// that rows of the table lie in several files and a file holds only some of them.
	inFiles bool
}

// docLayoutOf returns the layout of table, which must be a document table of cat.
func docLayoutOf(cat *catalog.Catalog, table string) (*docLayout, error) {
	tb, ok := cat.Table(table)
	if !ok {
		return nil, fmt.Errorf("table %s is not in the catalog", table)
	}
	if tb.Storage.Kind != catalog.StorageDocument {
		return nil, fmt.Errorf("table %s is not a document table", table)
	}
	return layoutOfTable(cat, tb, "", false)
}

// layoutOfTable returns the layout of the list or table tb, whose placement foreign
// key is placement, "" for a document table; inFiles reports that its placement
// chain reaches a files table. Its child blocks are laid out in turn.
func layoutOfTable(cat *catalog.Catalog, tb *catalog.Table, placement string, inFiles bool) (*docLayout, error) {
	table := tb.Name
	lay := &docLayout{
		table: table, list: tb.Storage.Shape == catalog.ShapeList, scope: tb.RowScope, placement: placement, inFiles: inFiles,
		title: tb.Title, body: tb.Body, order: tb.Order,
		fks: map[string]string{}, glyphs: map[string]string{}, states: map[string]string{},
	}
	markdown := map[string]bool{}
	for _, m := range tb.Markdown {
		markdown[m] = true
	}
	keyFound := false
	for _, c := range tb.Columns {
		switch {
		case c.Name == tb.Key:
			lay.key, keyFound = c, true
		case c.Name == tb.Order:
			lay.orderType = c.Type
		case c.Name == tb.Body || c.Name == placement:
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
		if len(fk.Columns) != 1 {
			continue
		}
		lay.fks[fk.Columns[0]] = fk.Table
		if fk.Table == table && fk.Columns[0] == "parent" {
			lay.parent = "parent"
		}
	}
	if sm := tb.StateMachine; sm != nil && lay.list {
		lay.state = sm.Column
		for state, glyph := range sm.Checkboxes {
			lay.glyphs[state] = glyph
			lay.states[glyph] = state
		}
	}
	for _, c := range lay.scalars {
		if lay.list && (c.Name == lay.title || c.Name == lay.state || c.Name == lay.parent) {
			continue
		}
		if lay.list {
			lay.fields = append(lay.fields, c)
		}
	}
	children, err := childLayouts(cat, table, inFiles)
	if err != nil {
		return nil, err
	}
	lay.children = children
	return lay, nil
}

// childLayouts returns the layouts of the block tables whose placement parent is
// table, in table name order, which is the order of their sections in a row scope;
// inFiles reports that the placement chain of table reaches a files table.
func childLayouts(cat *catalog.Catalog, table string, inFiles bool) ([]*docLayout, error) {
	var out []*docLayout
	for _, t := range cat.Tables {
		if t.Storage.Kind != catalog.StorageBlock || t.Storage.Parent != table {
			continue
		}
		placement, err := placementColumn(t)
		if err != nil {
			return nil, err
		}
		child, err := layoutOfTable(cat, t, placement, inFiles)
		if err != nil {
			return nil, err
		}
		out = append(out, child)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].table < out[j].table })
	return out, nil
}

// placementColumn returns the column of the placement foreign key of block table t.
func placementColumn(t *catalog.Table) (string, error) {
	for _, fk := range t.ForeignKeys {
		if fk.Name == t.Storage.ParentForeignKey && len(fk.Columns) == 1 {
			return fk.Columns[0], nil
		}
	}
	return "", fmt.Errorf("table %s: the placement foreign key %s is not a one-column foreign key", t.Name, t.Storage.ParentForeignKey)
}

// childNamed returns the child block of the table whose name is name, or nil.
func (l *docLayout) childNamed(name string) *docLayout {
	for _, c := range l.children {
		if c.table == name {
			return c
		}
	}
	return nil
}

// isField reports whether name is a field of a list row.
func (l *docLayout) isField(name string) bool {
	for _, c := range l.fields {
		if c.Name == name {
			return true
		}
	}
	return false
}

// required returns the columns whose value a row must have: a list's fields and
// state, and a table's cells but its title, which the row holds as its title.
func (l *docLayout) required() []dbspec.Column {
	var out []dbspec.Column
	for _, c := range l.scalars {
		switch {
		case c.Name == l.title:
		case l.list && c.Name != l.state && !l.isField(c.Name):
		default:
			out = append(out, c)
		}
	}
	return out
}

// writable reports whether a row may hold a value of name in Row.Scalars: a stored
// scalar column other than the title, or the order column, which a write takes.
func (l *docLayout) writable(name string) bool {
	if name == l.order && name != "" {
		return true
	}
	for _, c := range l.scalars {
		if c.Name == name && name != l.title {
			return true
		}
	}
	return false
}

// sectionOf returns the markdown section named name.
func (l *docLayout) sectionOf(name string) (section, bool) {
	for _, s := range l.sections {
		if s.name == name {
			return s, true
		}
	}
	return section{}, false
}

// docRec is a row of a list or table: its row, its layout, the line of the row, and
// whether the row has a detail section. seen holds the names of the sections and
// child blocks that its detail section has read; block numbers the scope, the
// document's list or table or one child block, that holds the row.
type docRec struct {
	row    *Row
	lay    *docLayout
	line   int
	detail bool
	seen   map[string]bool
	block  int
}

// docLink is a foreign key value whose target is resolved once every detail
// section of the file is known, since a row in the file has an anchor exactly
// when it has a detail section.
type docLink struct {
	rec    *docRec
	column string
	table  string
	key    string
	target string
	line   int
	col    int
}

// docParser reads one document file, or the block sections of a files row. The
// rows of the scope being read are recs, under the layout lay; all holds every row
// of the file at any depth once its scope is read. Links are resolved at the end,
// once every row of the file is known.
type docParser struct {
	*sink
	rel     string // the file's name, from which its links are derived
	lay     *docLayout
	lk      Linker
	recs    []*docRec
	all     []*docRec
	links   []docLink
	blocks  int             // the number of scopes checked, which numbers the block of each row
	details map[string]bool // the heading text of each detail section read: two of one text collide
	// ownTable and ownKey name the files row this file holds; empty for a document file.
	ownTable, ownKey string
}

// ParseDocument reads the document file rel of a document table, <table>.flowmark.md,
// whose bytes are data. It returns the rows in storage order, each with its child
// blocks, unless a diagnostic makes the file unreadable. A noncanonical file returns
// its rows with the diagnostics, and writing the rows gives the canonical file.
func ParseDocument(cat *catalog.Catalog, table, rel string, data []byte, lk Linker) ([]*Row, []Diagnostic, error) {
	lay, err := docLayoutOf(cat, table)
	if err != nil {
		return nil, nil, err
	}
	p := newDocParser(&sink{rel: rel}, rel, lk, "", "")
	text, ok := p.normalize(data)
	if !ok {
		return nil, p.diags, nil
	}
	lines := splitLines(text)
	recs := p.run(lay, lines)
	for _, c := range anchorClashes(lines, tableLevels(lay.table, lay.scope-1, lay.children)) {
		p.fail(c.line, clashColumn(lines, c), RuleBlock, c.message())
	}
	if p.fatal {
		return nil, p.diags, nil
	}
	rows := sortedRows(lay, recs)
	out, _, rf := renderDocument(lay, rows, lk, false)
	if len(rf) > 0 {
		return nil, append(p.diags, readDiagnostics(rel, rf, rows)...), nil
	}
	if !hasRule(p.diags, RuleNoncanonical) && !bytes.Equal(out, []byte(text)) {
		line, col := diffAt([]byte(text), out)
		p.warn(line, col, RuleNoncanonical, "the document is not in its canonical form")
	}
	return rows, p.diags, nil
}

// newDocParser returns a parser of the file rel, whose diagnostics go to s. The
// files row ownTable/ownKey is the one the file holds, empty for a document file.
func newDocParser(s *sink, rel string, lk Linker, ownTable, ownKey string) *docParser {
	return &docParser{sink: s, rel: rel, lk: lk, details: map[string]bool{}, ownTable: ownTable, ownKey: ownKey}
}

// run reads the H1, the list or table of the rows, and their detail sections, and
// returns the rows of the file's top scope.
func (p *docParser) run(lay *docLayout, lines []string) []*docRec {
	i := 0
	if len(lines) > 0 {
		// The H1 names the table once lowercased as the viewers lowercase it.
		if level, text, tab, ok := headingOf(lines[0]); ok && level == 1 && lowerAsViewers(trimWhite(text)) == lay.table {
			if text != lay.table || tab {
				p.warn(1, 3, RuleNoncanonical, "the H1 is written `# "+lay.table+"`")
			}
			i = 1
		}
	}
	if i == 0 {
		p.warn(1, 1, RuleNoncanonical, "the document has no H1 line `# "+lay.table+"`")
	}
	if i < len(lines) && isBlank(lines[i]) {
		p.warn(i+1, 1, RuleNoncanonical, "blank lines between the H1 and the rows")
		for i < len(lines) && isBlank(lines[i]) {
			i++
		}
	}
	p.lay, p.recs = lay, nil
	i = p.readRows(lines, i)
	recs := p.recs
	p.readDetails(lines, i, len(lines))
	p.checkScope(recs)
	p.resolveLinks()
	return recs
}

// readRows reads the list or the table of the rows of the layout p.lay from line i,
// and returns the index of the first line after them.
func (p *docParser) readRows(lines []string, i int) int {
	if p.lay.list {
		return p.readList(lines, i)
	}
	return p.readTable(lines, i)
}

// colAt returns the 1-based column, in code points, of byte offset off of line.
func colAt(line string, off int) int {
	if off > len(line) {
		off = len(line)
	}
	return utf8.RuneCountInString(line[:off]) + 1
}

// separator reads the white space run at off of line, and returns the index after
// it. A run other than one space is noncanonical.
func (p *docParser) separator(lineNo int, line string, off int) int {
	end := off
	for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
		end++
	}
	if line[off:end] != " " {
		p.warn(lineNo, colAt(line, off), RuleNoncanonical, "the separator is one space")
	}
	return end
}

// isListRow reports whether line opens a list item, with any indentation.
func isListRow(line string) bool {
	t := strings.TrimLeft(line, " ")
	return strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "-\t")
}

// readList reads the list rows from line i, nested as CommonMark nests them: a row
// is canonical two spaces under its parent; a row that the two-space grammar and
// CommonMark both place, but differently, is format.block; a row that only
// CommonMark places, at most three columns past a wider parent's content indent,
// is that row's child and noncanonical. It returns the index of the first line
// after the rows.
func (p *docParser) readList(lines []string, i int) int {
	var chain []string // the keys of the open rows, by depth
	var at []int       // the column of each open row's marker
	var content []int  // the column where each open row's content begins, as CommonMark reads it
	for ; i < len(lines) && isListRow(lines[i]); i++ {
		lineNo := i + 1
		line := lines[i]
		if trimmed := strings.TrimRight(line, " \t"); trimmed != line {
			p.warn(lineNo, colAt(line, len(trimmed)), RuleNoncanonical, "a space or a tab at the end of the row")
			line = trimmed
		}
		indent := indentOf(line)
		// CommonMark nests the row under every open row whose content begins at or
		// before its marker.
		d := 0
		for d < len(chain) && content[d] <= indent {
			d++
		}
		// The grammar nests it two spaces under an open row, or beside an open row at
		// its own indent.
		g, gok := 0, indent == 0
		for k := len(at) - 1; k >= 0; k-- {
			if at[k] == indent {
				g, gok = k, true
				break
			}
			if at[k]+2 == indent {
				g, gok = k+1, true
				break
			}
			if at[k] < indent {
				break
			}
		}
		depth := g
		switch {
		case gok && g != d:
			p.fail(lineNo, 1, RuleBlock, "CommonMark nests this row elsewhere than its indentation writes")
			continue
		case !gok && d > 0 && content[d-1] > at[d-1]+2 && indent-content[d-1] <= 3:
			p.warn(lineNo, 1, RuleNoncanonical, "a child row is indented two spaces deeper than its parent")
			depth = d
		case !gok:
			p.fail(lineNo, 1, RuleBlock, "a row is indented two spaces under the row of the level above it")
			continue
		}
		if depth > 0 && p.lay.parent == "" {
			p.fail(lineNo, 1, RuleBlock, "a row is nested, but the table has no parent column")
			continue
		}
		rec := p.listRow(lineNo, line, indent)
		if rec == nil {
			continue
		}
		if depth > 0 {
			rec.row.Scalars[p.lay.parent] = sp(chain[depth-1])
		}
		chain = append(chain[:depth], rec.row.Key)
		at = append(at[:depth], indent)
		content = append(content[:depth], indent+1+markerGap(line, indent))
		p.recs = append(p.recs, rec)
	}
	return i
}

// markerGap is the width in columns of the spaces and tabs after the list marker at
// indent, a tab advancing to the next multiple of four (CommonMark 0.31.2 §2.2).
func markerGap(line string, indent int) int {
	col := indent + 1
	for k := indent + 1; k < len(line) && (line[k] == ' ' || line[k] == '\t'); k++ {
		if line[k] == '\t' {
			col += 4 - col%4
		} else {
			col++
		}
	}
	return col - (indent + 1)
}

// listRow reads one list row of line, whose marker is at indent: the state glyph,
// the key, the title and the fields. It returns nil when the row is refused.
func (p *docParser) listRow(lineNo int, line string, indent int) *docRec {
	lay := p.lay
	// Five or more columns after the marker open an indented code block in the item,
	// as CommonMark reads it, so a viewer shows no task item; a narrower run, a tab
	// in it included, is a separator.
	if markerGap(line, indent) >= 5 {
		p.fail(lineNo, colAt(line, indent+1), RuleBlock, "five or more columns after the list marker open an indented code block in the item")
		return nil
	}
	off := p.separator(lineNo, line, indent+1)
	var state string
	glyphAt := 0 // the byte index of the state glyph
	if lay.state != "" {
		if off >= len(line) || line[off] != '[' {
			p.fail(lineNo, colAt(line, off), RuleBlock, "a row starts with its state glyph `[g]`")
			return nil
		}
		r, size := utf8.DecodeRuneInString(line[off+1:])
		end := off + 1 + size
		if size == 0 || end >= len(line) || line[end] != ']' {
			p.fail(lineNo, colAt(line, off+1), RuleBlock, "the state glyph is one character between brackets")
			return nil
		}
		st, ok := lay.states[string(r)]
		if !ok {
			p.fail(lineNo, colAt(line, off+1), RuleBlock, "the glyph "+string(r)+" names no state")
			return nil
		}
		state, glyphAt = st, off+1
		off = end + 1
		if off >= len(line) || (line[off] != ' ' && line[off] != '\t') {
			p.fail(lineNo, colAt(line, off), RuleBlock, "a space follows the state glyph")
			return nil
		}
		off = p.separator(lineNo, line, off)
	}
	keyEnd := off
	for keyEnd < len(line) && line[keyEnd] != ' ' && line[keyEnd] != '\t' {
		keyEnd++
	}
	key := line[off:keyEnd]
	if msg := docKeyFault(lay, key); msg != "" {
		p.fail(lineNo, colAt(line, off), RuleBlock, msg)
		return nil
	}
	if keyEnd >= len(line) {
		p.fail(lineNo, colAt(line, keyEnd), RuleBlock, "the title is missing")
		return nil
	}
	titleStart := p.separator(lineNo, line, keyEnd)
	toks, titleEnd, ok := p.listTail(lineNo, line, titleStart)
	if !ok {
		return nil
	}
	title, noncanon, msg := titleOf(lay, line[titleStart:titleEnd])
	switch {
	case msg != "":
		p.fail(lineNo, colAt(line, titleStart), RuleBlock, msg)
		return nil
	case title == "":
		p.fail(lineNo, colAt(line, titleStart), RuleBlock, "the title is missing")
		return nil
	case singleLineFault(title) != "":
		p.fail(lineNo, colAt(line, titleStart), RuleBlock, "the title has "+singleLineFault(title))
		return nil
	}
	for _, off := range noncanon {
		p.warn(lineNo, colAt(line, titleStart+off), RuleNoncanonical, "a title token that begins with a column is written with `\\:`")
	}
	for _, tok := range toks {
		p.tokenSeparator(lineNo, line, tok)
	}
	rec := &docRec{row: newRow(lay.table, key, title), lay: lay, line: lineNo}
	rec.row.setAt("", lineNo, 1)
	rec.row.setAt(lay.key.Name, lineNo, colAt(line, off))
	if lay.title != "" {
		rec.row.setAt(lay.title, lineNo, colAt(line, titleStart))
	}
	if state != "" {
		rec.row.Scalars[lay.state] = sp(state)
		rec.row.setAt(lay.state, lineNo, colAt(line, glyphAt))
	}
	last := -1
	for _, tok := range toks {
		idx := slices.IndexFunc(lay.fields, func(c dbspec.Column) bool { return c.Name == tok.name })
		switch {
		case idx < 0:
			p.fail(lineNo, colAt(line, tok.start), RuleBlock, tok.name+" is not a field of the list "+lay.table)
			return nil
		case idx <= last:
			p.fail(lineNo, colAt(line, tok.start), RuleBlock, "field "+tok.name+" is out of column order or given twice")
			return nil
		}
		last = idx
		rec.row.setAt(tok.name, lineNo, colAt(line, tok.start))
		p.scalarValue(lineNo, colAt(line, tok.start), rec, lay.fields[idx], tok.value)
	}
	return rec
}

// tailTok is a field token at the end of a list row: its column, the index of
// its name and of the white space before it, and its decoded value.
type tailTok struct {
	name   string
	value  string
	quoted bool
	start  int // the index in the line of the name
	sepAt  int // the index in the line where the white space before the token starts
}

// listTail reads the fields at the end of a list row's text, from the title start
// from to the end of line, parsing from the right. It returns the fields in line
// order and the index where the title ends; ok is false when the row is refused.
func (p *docParser) listTail(lineNo int, line string, from int) ([]tailTok, int, bool) {
	end := len(line)
	var toks []tailTok
	for {
		tok, found, at, msg := p.lastField(line, from, end)
		if msg != "" {
			p.fail(lineNo, colAt(line, at), RuleBlock, msg)
			return nil, 0, false
		}
		if !found {
			break
		}
		toks = append(toks, tok)
		end = tok.sepAt
	}
	slices.Reverse(toks)
	return toks, end, true
}

// lastField reads the field token that ends line[from:end], if there is one. A
// quoted value is found from its opening quote, which must be preceded by a
// declared column name at the start of a token; a bare value is the last token.
// It returns the token, whether there is one, and a fault with its index.
func (p *docParser) lastField(line string, from, end int) (tailTok, bool, int, string) {
	if end <= from {
		return tailTok{}, false, 0, ""
	}
	if line[end-1] == '"' {
		for q := end - 2; q > from; q-- {
			if line[q] != '"' || line[q-1] != ':' {
				continue
			}
			name, start, ok := p.nameBefore(line, from, q-1)
			if !ok {
				continue
			}
			value, n, ok := scanQuoted(line[q:end])
			if !ok || q+n != end {
				return tailTok{}, false, start, "the quoted value of " + name + " is malformed"
			}
			return tailTok{name: name, value: value, quoted: true, start: start, sepAt: sepBefore(line, from, start)}, true, 0, ""
		}
		return tailTok{}, false, 0, ""
	}
	tokStart := from
	if k := strings.LastIndexAny(line[from:end], " \t"); k >= 0 {
		tokStart = from + k + 1
	}
	token := line[tokStart:end]
	for _, n := range p.lay.names() {
		if !strings.HasPrefix(token, n+":") {
			continue
		}
		value := token[len(n)+1:]
		switch {
		case value == "":
			return tailTok{}, false, tokStart, "field " + n + " has no value"
		case strings.ContainsAny(value, `"\`) || hasInvisible(value):
			return tailTok{}, false, tokStart, "the bare value of " + n + " holds a quote, a backslash or an invisible character"
		}
		return tailTok{name: n, value: value, start: tokStart, sepAt: sepBefore(line, from, tokStart)}, true, 0, ""
	}
	return tailTok{}, false, 0, ""
}

// nameBefore returns the declared column name that ends line[:colon] at the start
// of a token at or after from, and the index of that name.
func (p *docParser) nameBefore(line string, from, colon int) (string, int, bool) {
	for _, n := range p.lay.names() {
		start := colon - len(n)
		if start < from || line[start:colon] != n {
			continue
		}
		if start > from && line[start-1] != ' ' && line[start-1] != '\t' {
			continue
		}
		return n, start, true
	}
	return "", 0, false
}

// names returns the names of every column of the table, in column order.
func (l *docLayout) names() []string {
	var out []string
	out = append(out, l.key.Name)
	for _, c := range l.scalars {
		out = append(out, c.Name)
	}
	for _, s := range l.sections {
		out = append(out, s.name)
	}
	if l.body != "" {
		out = append(out, l.body)
	}
	if l.order != "" {
		out = append(out, l.order)
	}
	if l.placement != "" {
		out = append(out, l.placement)
	}
	if l.title != "" && !slices.Contains(out, l.title) {
		out = append(out, l.title)
	}
	return out
}

// sepBefore returns the index where the white space before the token at start
// begins, not before from.
func sepBefore(line string, from, start int) int {
	j := start
	for j > from && (line[j-1] == ' ' || line[j-1] == '\t') {
		j--
	}
	return j
}

// scanQuoted reads the quoted value at the start of s, "…" with \" and \\ as its
// escapes. It returns the decoded value and the length of the quoted text.
func scanQuoted(s string) (string, int, bool) {
	if s == "" || s[0] != '"' {
		return "", 0, false
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			if i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
				b.WriteByte(s[i+1])
				i++
				continue
			}
			return "", 0, false
		case '"':
			return b.String(), i + 1, true
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, false
}

// hasInvisible reports whether v holds an invisible character.
func hasInvisible(v string) bool {
	for _, r := range v {
		if invisible(r) {
			return true
		}
	}
	return false
}

// tokenSeparator checks the white space before a field of a list row: the separator
// is one space, and any other run of spaces and tabs, a tab that ends the title
// included, is noncanonical.
func (p *docParser) tokenSeparator(lineNo int, line string, tok tailTok) {
	if line[tok.sepAt:tok.start] != " " {
		p.warn(lineNo, colAt(line, tok.sepAt), RuleNoncanonical, "the separator is one space")
	}
}

// titleTokens returns the spans [start, end) of the tokens of a title: the runs of
// characters between spaces and tabs.
func titleTokens(title string) [][2]int {
	var spans [][2]int
	for i := 0; i < len(title); {
		if title[i] == ' ' || title[i] == '\t' {
			i++
			continue
		}
		j := i
		for j < len(title) && title[j] != ' ' && title[j] != '\t' {
			j++
		}
		spans = append(spans, [2]int{i, j})
		i = j
	}
	return spans
}

// titleOf reads the title of a list row from raw, the text between its key and its
// first field. A token <column>\: reads as <column>:; the white space between the
// tokens is kept byte for byte. An unescaped token that begins with a declared
// column and a colon is noncanonical where another token follows it, and a fault
// where it ends the title, since a field stands there. It returns the title, the
// offsets in raw of the noncanonical tokens and the fault, if any.
func titleOf(lay *docLayout, raw string) (string, []int, string) {
	var b strings.Builder
	var noncanon []int
	spans := titleTokens(raw)
	last := 0
	for k, sp := range spans {
		b.WriteString(raw[last:sp[0]])
		last = sp[1]
		t := raw[sp[0]:sp[1]]
		out := t
		for _, n := range lay.names() {
			if strings.HasPrefix(t, n+`\:`) {
				out = n + ":" + t[len(n)+2:]
				break
			}
			if strings.HasPrefix(t, n+":") {
				if k == len(spans)-1 {
					return "", nil, "the title ends with the token " + n + ": where a field stands"
				}
				noncanon = append(noncanon, sp[0])
				break
			}
		}
		b.WriteString(out)
	}
	b.WriteString(raw[last:])
	return b.String(), noncanon, ""
}

// newRow returns a row of table with the given key and title and no value yet.
func newRow(table, key, title string) *Row {
	return &Row{
		Table: table, Key: key, Title: title,
		Scalars: map[string]*string{}, Sections: map[string]*string{},
		Raw: map[string]string{}, Dangling: map[string]DanglingLink{}, Shared: map[string]DanglingLink{},
	}
}

// docKeyFault names the key rule that key breaks, or "" when it is canonical.
func docKeyFault(lay *docLayout, key string) string {
	if msg := keyFault(key); msg != "" {
		return msg
	}
	if canon, ok := readable(lay.key.Type, key); !ok || canon != key {
		return "the key is not the canonical text of a " + lay.key.Type.String() + " key"
	}
	return ""
}

// scalarValue reads the value v of column c of rec, found at line and col: a
// foreign key is a link, to be resolved with the other links of the file; any
// other value is checked against its single-line rules and its column type.
func (p *docParser) scalarValue(line, col int, rec *docRec, c dbspec.Column, v string) {
	if table, isFK := p.lay.fks[c.Name]; isFK {
		key, target, ok := splitLink(v)
		if !ok {
			p.fail(line, col, RuleBlock, "a foreign key is a link [key](target)")
			return
		}
		p.links = append(p.links, docLink{rec: rec, column: c.Name, table: table, key: key, target: target, line: line, col: col})
		rec.row.Scalars[c.Name] = sp(key)
		if _, ok := readable(c.Type, key); !ok {
			rec.row.Raw[c.Name] = v
		}
		return
	}
	if msg := singleLineFault(v); msg != "" {
		p.fail(line, col, RuleBlock, "the value has "+msg)
		return
	}
	canon, ok := readable(c.Type, v)
	if !ok {
		rec.row.Raw[c.Name] = v
		rec.row.Scalars[c.Name] = sp(v)
		return
	}
	if canon != v {
		p.warn(line, col, RuleNoncanonical, "the value is written "+canon)
	}
	rec.row.Scalars[c.Name] = sp(canon)
}

// tableCell is a cell of a table row: its raw text between the pipes and the index
// in the line where that text begins.
type tableCell struct {
	raw string
	at  int
}

// splitTableCells splits a table row into its cells as GFM reads it: the pipe at
// either end is optional, and a pipe that directly follows a backslash is text,
// that one backslash being removed. The row has no indent and no white space at
// its end. outer reports whether the row has a pipe at both ends.
func splitTableCells(line string) (cells []tableCell, outer bool) {
	var cur strings.Builder
	i := 0
	lead := strings.HasPrefix(line, "|")
	if lead {
		i = 1
	}
	start, trail := i, false
	for ; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\\' && i+1 < len(line) && line[i+1] == '|':
			cur.WriteByte('|')
			i++
		case c == '|':
			cells = append(cells, tableCell{raw: cur.String(), at: start})
			cur.Reset()
			start, trail = i+1, i == len(line)-1
		default:
			cur.WriteByte(c)
		}
	}
	if !trail {
		cells = append(cells, tableCell{raw: cur.String(), at: start})
	}
	return cells, lead && trail
}

// cellValue reads the text of a table cell of the line text: one space separates
// the cell from each pipe, an empty cell is NULL, and a cell that begins with a
// quote is a quoted value. It returns the value, whether it is NULL, and whether
// the cell is well formed; a space or a tab beyond the separators is noncanonical.
func (p *docParser) cellValue(lineNo int, text string, c tableCell) (string, bool, bool) {
	inner := trimWhite(c.raw)
	if c.raw != " "+inner+" " {
		p.warn(lineNo, colAt(text, c.at), RuleNoncanonical, "a cell is written `| value |`")
	}
	if inner == "" {
		return "", true, true
	}
	if inner[0] == '"' {
		v, n, ok := scanQuoted(inner)
		if !ok || n != len(inner) {
			p.fail(lineNo, colAt(text, c.at+1), RuleBlock, "the quoted cell is malformed")
			return "", false, false
		}
		return v, false, true
	}
	return inner, false, true
}

// readTable reads the table of rows from line i: its header, its delimiter row and
// its rows. It returns the index of the first line after the table.
func (p *docParser) readTable(lines []string, i int) int {
	lay := p.lay
	// tableEnd returns the index of the first line from k that does not begin with a pipe.
	tableEnd := func(k int) int {
		for k < len(lines) && strings.HasPrefix(lines[k], "|") {
			k++
		}
		return k
	}
	if i >= len(lines) {
		return i
	}
	// The header is a paragraph line, as GFM reads it: at most three spaces of indent
	// and no tab before it, the pipe at either end optional, and any of space, tab,
	// vertical tab and form feed at its end.
	hl := lines[i]
	ind := indentOf(hl)
	if ind > 3 || (ind < len(hl) && hl[ind] == '\t') {
		return i
	}
	hbody := strings.TrimRight(hl[ind:], " \t\v\f")
	header, _ := splitTableCells(hbody)
	want := []string{lay.key.Name}
	for _, c := range lay.scalars {
		want = append(want, c.Name)
	}
	names := make([]string, len(header))
	for j, c := range header {
		names[j] = trimWhite(c.raw)
	}
	if !slices.Equal(names, want) {
		if !strings.HasPrefix(hbody, "|") {
			return i
		}
		p.fail(i+1, 1, RuleBlock, "the header is not the columns "+strings.Join(want, ", ")+" of "+lay.table)
		return tableEnd(i + 1)
	}
	if i+1 >= len(lines) {
		p.fail(i+1, 1, RuleBlock, "a table has a header row and a delimiter row")
		return i + 1
	}
	if canon := "| " + strings.Join(want, " | ") + " |"; hl != canon {
		p.warn(i+1, 1, RuleNoncanonical, "the header row is written `"+canon+"`")
	}
	end := tableEnd(i + 2)
	cells, ok := delimiterCells(lines[i+1])
	if !ok || len(cells) != len(want) {
		p.fail(i+2, 1, RuleBlock, "the delimiter row is not one with one cell per column of the header")
		return end
	}
	if lines[i+1] != delimiterRow(len(want)) {
		p.warn(i+2, 1, RuleNoncanonical, "the delimiter row is written `"+delimiterRow(len(want))+"`")
	}
	for k := i + 2; k < end; k++ {
		if rec := p.tableRow(k+1, lines[k]); rec != nil {
			p.recs = append(p.recs, rec)
		}
	}
	return end
}

// delimiterCell is one cell of a delimiter row: hyphens with at most one colon at
// either end, and around them the white space of tableSpace.
var delimiterCell = regexp.MustCompile("^[ \t\v\f]*:?-+:?[ \t\v\f]*$")

// delimiterRow is the canonical delimiter row of a table of n columns.
func delimiterRow(n int) string {
	return "| " + strings.TrimSuffix(strings.Repeat("--- | ", n), " | ") + " |"
}

// delimiterCells reads a line as the delimiter row that GFM reads under a header:
// at most three spaces of indent and no tab before it, then cells of delimiterCell
// separated by pipes, the pipes at either end optional. It returns the cells and
// whether the line is such a row. A line without a leading pipe that CommonMark
// reads first as a setext underline or as a list item is not one, whatever its
// cells; a tab in the indent reaches the fourth column, so CommonMark reads the
// line as indented, a continuation of the header's paragraph.
func delimiterCells(line string) ([]string, bool) {
	body := strings.TrimLeft(line, " ")
	if len(line)-len(body) > 3 || strings.HasPrefix(body, "\t") {
		return nil, false
	}
	if !strings.HasPrefix(body, "|") {
		rest := strings.TrimRight(body, " \t")
		if rest != "" && strings.Trim(rest, "-") == "" {
			return nil, false
		}
		if len(body) >= 2 && body[0] == '-' && (body[1] == ' ' || body[1] == '\t') {
			return nil, false
		}
	}
	s := strings.TrimRight(body, " \t\v\f")
	s = strings.TrimSuffix(strings.TrimPrefix(s, "|"), "|")
	cells := strings.Split(s, "|")
	for _, c := range cells {
		if !delimiterCell.MatchString(c) {
			return nil, false
		}
	}
	return cells, true
}

// tableRow reads one row of a table: the key cell, then one cell per scalar column
// in column order. It returns nil when the row is refused.
func (p *docParser) tableRow(lineNo int, text string) *docRec {
	lay := p.lay
	if trimmed := strings.TrimRight(text, " \t"); trimmed != text {
		p.warn(lineNo, colAt(text, len(trimmed)), RuleNoncanonical, "a space or a tab at the end of the row")
		text = trimmed
	}
	// A row without its last pipe is read as GFM reads it: its last cell runs to the
	// end of the line, a final `\|` being a pipe of that cell.
	cells, outer := splitTableCells(text)
	if !outer {
		p.warn(lineNo, 1, RuleNoncanonical, "a row is written with a pipe at either end")
	}
	if len(cells) != len(lay.scalars)+1 {
		p.fail(lineNo, 1, RuleBlock, "a row has one cell per column of the header")
		return nil
	}
	key, null, ok := p.cellValue(lineNo, text, cells[0])
	if !ok {
		return nil
	}
	if null {
		p.fail(lineNo, colAt(text, cells[0].at+1), RuleBlock, "the key is missing")
		return nil
	}
	if msg := docKeyFault(lay, key); msg != "" {
		p.fail(lineNo, colAt(text, cells[0].at+1), RuleBlock, msg)
		return nil
	}
	rec := &docRec{row: newRow(lay.table, key, ""), lay: lay, line: lineNo}
	rec.row.setAt("", lineNo, 1)
	rec.row.setAt(lay.key.Name, lineNo, colAt(text, cells[0].at+1))
	for j, c := range lay.scalars {
		cell := cells[j+1]
		v, null, ok := p.cellValue(lineNo, text, cell)
		if !ok {
			return nil
		}
		if null {
			continue
		}
		col := colAt(text, cell.at+1)
		rec.row.setAt(c.Name, lineNo, col)
		if c.Name == lay.title {
			if msg := singleLineFault(v); msg != "" {
				p.fail(lineNo, col, RuleBlock, "the title has "+msg)
				return nil
			}
			rec.row.Title = v
			continue
		}
		p.scalarValue(lineNo, col, rec, c, v)
	}
	return rec
}

// readDetails reads the content after the list or table of the rows p.recs, from
// line i to end: blank lines, then the detail sections of the rows, each a heading
// of the detail level of the layout (the scope level less one) and the lines up to
// the next heading of that level or less. Any other content is format.block.
func (p *docParser) readDetails(lines []string, i, end int) {
	h := p.lay.scope - 1
	heads := scanHeadings(lines[:end], i).heads
	first := end
	if len(heads) > 0 {
		first = heads[0].at
	}
	blank := true
	for k := i; k < first; k++ {
		if !isBlank(lines[k]) {
			p.fail(k+1, 1, RuleBlock, "content after the list or table is not a detail section")
			blank = false
			break
		}
	}
	// One blank line separates the rows from the element after them, and none stands
	// at the end of the file; a blank line is a line of only spaces and tabs.
	if want := min(1, len(lines)-first); blank && (first-i != want || (want == 1 && lines[i] != "")) {
		p.warn(i+1, 1, RuleNoncanonical, "one blank line separates the rows from the element after them")
	}
	for k := 0; k < len(heads); {
		hd := heads[k]
		next := k + 1
		for next < len(heads) && heads[next].level > h {
			next++
		}
		stop := end
		if next < len(heads) {
			stop = heads[next].at
		}
		if hd.level == h {
			p.readDetail(lines, hd, heads[k+1:next], stop)
		} else {
			p.fail(hd.at+1, 1, RuleBlock, "content after the list or table is not a detail section")
		}
		k = next
	}
}

// readDetail reads the detail section whose heading is h, with the headings sub
// below it, up to end. The section names one row of p.recs; its body follows the
// heading up to the first heading of the section level, and then its sections and
// child blocks.
func (p *docParser) readDetail(lines []string, h heading, sub []heading, end int) {
	lay := p.lay
	marks := strings.Repeat("#", h.level)
	parts := detailFields(h.text)
	if len(parts) != 3 || lowerAsViewers(parts[0]) != lowerAsViewers(lay.table) || strings.ToLower(parts[1]) != "row" || parts[2] == "" {
		p.fail(h.at+1, 4, RuleBlock, "a detail section is `"+marks+" "+lay.table+" row <key>`")
		return
	}
	key := parts[2]
	if h.last != h.at || h.tab || h.text != lay.table+" row "+key {
		p.warn(h.at+1, 4, RuleNoncanonical, "the detail heading is written `"+marks+" "+lay.table+" row "+key+"`")
	}
	// Two detail sections of one file under one heading text collide, whatever block
	// holds them, since their anchors are equal; a case difference is key.duplicate.
	text := lay.table + " row " + key
	if p.details[text] {
		p.fail(h.at+1, 4, RuleBlock, "the detail section "+text+" collides with another detail section of the file")
		return
	}
	p.details[text] = true
	var match []*docRec
	for _, rec := range p.recs {
		if rec.row.Key == key {
			match = append(match, rec)
		}
	}
	switch {
	case len(match) == 0:
		p.fail(h.at+1, 4, RuleBlock, "the detail section names no row "+key)
		return
	case len(match) > 1:
		p.fail(h.at+1, 4, RuleBlock, "the key "+key+" is shared by two rows, so its detail section cannot be assigned")
		return
	case match[0].detail:
		p.fail(h.at+1, 4, RuleBlock, "the row "+key+" has a second detail section")
		return
	}
	rec := match[0]
	rec.detail = true
	rec.seen = map[string]bool{}
	var bounds []heading
	for _, s := range sub {
		if s.level <= lay.scope {
			bounds = append(bounds, s)
		}
	}
	bEnd := end
	if len(bounds) > 0 {
		bEnd = bounds[0].at
	}
	body, bodyAt := p.valueLines(lines, h.last+1, bEnd, true)
	if len(body) > 0 && lay.body != "" {
		rec.row.setAt(lay.body, bodyAt+1, 1)
	}
	rec.row.Body = joinLines(body)
	for k, s := range bounds {
		sEnd := end
		if k+1 < len(bounds) {
			sEnd = bounds[k+1].at
		}
		p.readSection(lines, s, sEnd, rec)
	}
	if !hasContent(rec.row) && blankLines(lines[h.last+1:end]) {
		p.warn(h.at+1, 1, RuleNoncanonical, "a detail section with no body, section or block is not written: the row has no detail section")
	}
}

// readSection reads the heading s of a detail section of rec, up to sEnd: a markdown
// section of the row, or a child block of the row, which is read in its own scope.
func (p *docParser) readSection(lines []string, s heading, sEnd int, rec *docRec) {
	lay := p.lay
	name := trimWhite(s.text)
	marks := strings.Repeat("#", s.level)
	if child := lay.childNamed(name); child != nil {
		if rec.seen[name] {
			p.fail(s.at+1, 5, RuleSection, "the block "+name+" is written twice")
			return
		}
		rec.seen[name] = true
		if s.text != name || s.tab {
			p.warn(s.at+1, 5, RuleNoncanonical, "the block heading is not written with one space after its hashes")
		}
		if rows := p.readChildBlock(lines, s, sEnd, child); len(rows) > 0 {
			setBlock(rec.row, child.table, rows)
		}
		return
	}
	if s.text != name || s.tab {
		p.warn(s.at+1, 5, RuleNoncanonical, "the section heading is not written with one space after its hashes")
	}
	if _, ok := lay.sectionOf(name); !ok {
		p.fail(s.at+1, 5, RuleSection, marks+" "+name+" is not a section of "+lay.table)
		return
	}
	if rec.seen[name] {
		p.fail(s.at+1, 5, RuleSection, "the section "+name+" is written twice")
		return
	}
	rec.seen[name] = true
	value, _ := p.valueLines(lines, s.last+1, sEnd, true)
	v := joinLines(value)
	rec.row.setAt(name, s.at+1, s.level+2) // the heading text follows the hashes and a space
	rec.row.Sections[name] = &v
}

// readChildBlock reads the child block whose heading is h, up to end, in the scope
// of the child's layout: its list or table, then the detail sections of its rows. It
// returns the rows in storage order; a block with no rows is noncanonical.
func (p *docParser) readChildBlock(lines []string, h heading, end int, child *docLayout) []*Row {
	outerLay, outerRecs := p.lay, p.recs
	p.lay, p.recs = child, nil
	defer func() { p.lay, p.recs = outerLay, outerRecs }()
	i := h.last + 1
	if i < end && isBlank(lines[i]) {
		p.warn(i+1, 1, RuleNoncanonical, "a blank line between the block heading and its rows")
		for i < end && isBlank(lines[i]) {
			i++
		}
	}
	start := i
	i = p.readRows(lines[:end], i)
	if i == start {
		p.warn(h.at+1, 1, RuleNoncanonical, "an empty block heading: a block with no rows is not written")
	}
	p.readDetails(lines, i, end)
	recs := p.recs
	p.checkScope(recs)
	return sortedRows(child, recs)
}

// blankLines reports whether every line is blank.
func blankLines(lines []string) bool {
	for _, line := range lines {
		if !isBlank(line) {
			return false
		}
	}
	return true
}

// setBlock sets the child rows of the block table of row r.
func setBlock(r *Row, table string, rows []*Row) {
	if r.Blocks == nil {
		r.Blocks = map[string][]*Row{}
	}
	r.Blocks[table] = rows
}

// checkScope checks the rows of one scope once their detail sections are read: the
// non-null sections and values are present, the order is stated by the line order,
// the parents name rows of the scope, and the parents form no cycle. The rows then
// belong to the rows of the file, where a link finds them, each stamped with the
// number of its block.
func (p *docParser) checkScope(recs []*docRec) {
	lay := p.lay
	p.blocks++
	for _, rec := range recs {
		rec.block = p.blocks
	}
	for _, rec := range recs {
		for _, s := range lay.sections {
			if !s.nullable && rec.row.Sections[s.name] == nil {
				p.fail(rec.line, 1, RuleBlock, "the non-null section "+s.name+" is missing")
			}
		}
		for _, c := range lay.required() {
			if rec.row.Scalars[c.Name] == nil && !c.Null {
				p.fail(rec.line, 1, RuleBlock, "the non-null field "+c.Name+" is missing")
			}
		}
		if lay.title != "" && !lay.list && rec.row.Title == "" {
			p.fail(rec.line, 1, RuleBlock, "the title is missing")
		}
	}
	if lay.order != "" {
		pos := map[string]int{}
		for _, rec := range recs {
			g := parentKey(lay, rec.row)
			pos[g]++
			rec.row.Scalars[lay.order] = sp(strconv.Itoa(pos[g]))
		}
	}
	if lay.parent != "" {
		for _, rec := range recs {
			v := rec.row.Scalars[lay.parent]
			if v == nil {
				continue
			}
			switch n := countRows(recs, *v); {
			case n == 0:
				p.fail(rec.line, 1, RuleBlock, "the parent "+*v+" names no row of its block")
			case n > 1:
				p.fail(rec.line, 1, RuleBlock, "the parent "+*v+" names a key that two rows of the block share")
			}
		}
	}
	if _, _, unreached := storageOrder(lay, recRows(recs)); len(unreached) > 0 {
		for _, i := range unreached {
			if rowOnCycle(lay, recRows(recs), i) {
				p.fail(recs[i].line, 1, RuleParentCycle, "the parents of the rows form a cycle")
				break
			}
		}
	}
	p.all = append(p.all, recs...)
}

// rowOnCycle reports whether the parents of rows[i] lead back to rows[i]. A row
// under a parent that no row holds, or that two rows hold, is not on a cycle: that
// parent is reported by its own rule.
func rowOnCycle(lay *docLayout, rows []*Row, i int) bool {
	cur := i
	for range rows {
		next := uniqueRow(rows, parentKey(lay, rows[cur]))
		if next < 0 {
			return false
		}
		if next == i {
			return true
		}
		cur = next
	}
	return false
}

// uniqueRow returns the index of the row of rows whose key is key, -1 when no row or
// two rows have it.
func uniqueRow(rows []*Row, key string) int {
	found := -1
	for j, r := range rows {
		if r.Key == key {
			if found >= 0 {
				return -1
			}
			found = j
		}
	}
	return found
}

// resolveLinks checks each foreign key of the file against the row it names, once
// every row of the file is known: a key that names no row, or that two or more rows
// share byte for byte, has no derived target and keeps its written target; a target
// other than the derived one is noncanonical. A parent that names no row of its
// block, or one that two rows of its block share, is reported by checkScope.
func (p *docParser) resolveLinks() {
	for _, l := range p.links {
		want, found := p.linkTarget(l)
		parent := l.column == l.rec.lay.parent
		switch {
		case found == linkNone && !parent:
			l.rec.row.Dangling[l.column] = DanglingLink{Key: l.key, Target: l.target}
		case found == linkShared && !parent:
			l.rec.row.Shared[l.column] = DanglingLink{Key: l.key, Target: l.target}
		case found == linkOne && want != l.target:
			p.warn(l.line, l.col, RuleNoncanonical, "the link target is "+want)
		}
	}
}

// The outcomes of the derivation of a link's target: the key names one row, which
// gives the target; it names no row; or two or more rows share it byte for byte.
const (
	linkOne = iota
	linkNone
	linkShared
)

// recRows returns the rows of recs, in their order.
func recRows(recs []*docRec) []*Row {
	rows := make([]*Row, len(recs))
	for i, rec := range recs {
		rows[i] = rec.row
	}
	return rows
}

// sortedRows returns the rows of recs in the storage order of lay.
func sortedRows(lay *docLayout, recs []*docRec) []*Row {
	rows := recRows(recs)
	order, _, _ := storageOrder(lay, rows)
	out := make([]*Row, 0, len(order))
	for _, i := range order {
		out = append(out, rows[i])
	}
	return out
}

// linkTarget derives the target of the link l from the one row its key names. A
// parent names a row of its own block, so its target is derived from that row. Any
// other key names the row of this file that holds it — the file's name with the
// anchor of the row's detail section when it has one — or else the row the linker
// finds. A key that two or more rows of the table share byte for byte, in this file
// or, when the table's placement chain reaches a files table, in the corpus, names
// no one row and has no derived target.
func (p *docParser) linkTarget(l docLink) (string, int) {
	if l.table == p.ownTable && l.key == p.ownKey {
		return path.Base(p.rel), linkOne
	}
	parent := l.column == l.rec.lay.parent
	var match []*docRec
	for _, rec := range p.all {
		if rec.row.Table == l.table && rec.row.Key == l.key && (!parent || rec.block == l.rec.block) {
			match = append(match, rec)
		}
	}
	switch {
	case len(match) > 1:
		return "", linkShared
	case len(match) == 1:
		if match[0].lay.inFiles && !parent {
			if _, err := p.lk.Target(p.rel, l.table, l.key); errors.Is(err, ErrKeyShared) {
				return "", linkShared
			}
		}
		return localLink(match[0].lay, p.rel, match[0].row), linkOne
	case parent:
		return "", linkNone
	}
	target, err := p.lk.Target(p.rel, l.table, l.key)
	switch {
	case errors.Is(err, ErrKeyShared):
		return "", linkShared
	case err != nil:
		return "", linkNone
	}
	return target, linkOne
}

// docFileName is the file name of the document table, <table>.flowmark.md.
func docFileName(lay *docLayout) string { return lay.table + fileSuffix }

// localLink returns the link from the document file to row r of its own table: its
// anchor when the row has a detail section, or the file's name when it has none.
func localLink(lay *docLayout, rel string, r *Row) string {
	anchor := ""
	if hasDetail(lay, r) {
		anchor = DetailAnchor(lay.table, r.Key)
	}
	link, _ := RelLink(rel, rel, anchor)
	return link
}

// HasDetail reports whether row r of table has a detail section, the section that
// its link anchor names (DetailAnchor). A files row has none: its scope is its file.
// Its layout is the catalog's, so the answer is the one the parser gives the file.
func HasDetail(cat *catalog.Catalog, table string, r *Row) (bool, error) {
	tb, ok := cat.Table(table)
	if !ok {
		return false, fmt.Errorf("table %s is not in the catalog", table)
	}
	var lay *docLayout
	switch tb.Storage.Kind {
	case catalog.StorageFiles:
		return false, nil
	case catalog.StorageDocument:
		l, err := docLayoutOf(cat, table)
		if err != nil {
			return false, err
		}
		lay = l
	default:
		placement, err := placementColumn(tb)
		if err != nil {
			return false, err
		}
		root := tb
		for root.Storage.Kind == catalog.StorageBlock {
			p, ok := cat.Table(root.Storage.Parent)
			if !ok {
				return false, fmt.Errorf("table %s has no placement parent %s in the catalog", root.Name, root.Storage.Parent)
			}
			root = p
		}
		l, err := layoutOfTable(cat, tb, placement, root.Storage.Kind == catalog.StorageFiles)
		if err != nil {
			return false, err
		}
		lay = l
	}
	return hasDetail(lay, r), nil
}

// hasDetail reports whether row r has a detail section: content that it holds, or
// a non-null section, which is always present.
func hasDetail(lay *docLayout, r *Row) bool {
	if hasContent(r) {
		return true
	}
	for _, s := range lay.sections {
		if !s.nullable {
			return true
		}
	}
	return false
}

// hasContent reports whether row r holds a body, a present section or a child row:
// the content that a detail section writes.
func hasContent(r *Row) bool {
	if r.Body != "" {
		return true
	}
	for _, v := range r.Sections {
		if v != nil {
			return true
		}
	}
	for _, rows := range r.Blocks {
		if len(rows) > 0 {
			return true
		}
	}
	return false
}

// parentKey returns the key of the parent of row r, "" for a root row.
func parentKey(lay *docLayout, r *Row) string {
	if lay.parent != "" {
		if v := r.Scalars[lay.parent]; v != nil {
			return *v
		}
	}
	return ""
}

// countRows returns the number of recs whose key is key.
func countRows(recs []*docRec, key string) int {
	n := 0
	for _, rec := range recs {
		if rec.row.Key == key {
			n++
		}
	}
	return n
}

// storageOrder returns the indexes of rows in storage order, the pre-order of the
// tree of parents with each row's children in sibling order, and the depth of each
// row. A row no root reaches lies on a cycle or under a parent that no row has; it
// is returned in unreached.
func storageOrder(lay *docLayout, rows []*Row) (order, depth, unreached []int) {
	children := map[string][]int{}
	for i, r := range rows {
		g := parentKey(lay, r)
		children[g] = append(children[g], i)
	}
	for _, c := range children {
		sort.SliceStable(c, func(a, b int) bool { return siblingLess(lay, rows[c[a]], rows[c[b]]) })
	}
	depth = make([]int, len(rows))
	visited := make([]bool, len(rows))
	var walk func(parent string, d int)
	walk = func(parent string, d int) {
		for _, i := range children[parent] {
			if visited[i] {
				continue
			}
			visited[i] = true
			depth[i] = d
			order = append(order, i)
			walk(rows[i].Key, d+1)
		}
	}
	walk("", 0)
	for i := range rows {
		if !visited[i] {
			unreached = append(unreached, i)
		}
	}
	return order, depth, unreached
}

// siblingLess orders two siblings: by their order values when the table has an
// order column, else by their keys.
func siblingLess(lay *docLayout, a, b *Row) bool {
	if lay.order != "" {
		return orderValue(a, lay.order) < orderValue(b, lay.order)
	}
	return keyLess(lay.key.Type, a.Key, b.Key)
}

// orderValue returns the value of the order column of row r as a number. A missing
// or unreadable value is 0: a write refuses it, and a read always has the value.
func orderValue(r *Row, column string) int {
	v := r.Scalars[column]
	if v == nil {
		return 0
	}
	n, _ := strconv.Atoi(*v)
	return n
}

// keyLess orders two canonical keys: numerically for an integer key, else by their
// bytes, which are their code points in UTF-8.
func keyLess(t dbspec.Type, a, b string) bool {
	switch t.Kind {
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64:
		x, ex := strconv.ParseInt(a, 10, 64)
		y, ey := strconv.ParseInt(b, 10, 64)
		if ex == nil && ey == nil {
			return x < y
		}
	}
	return a < b
}
