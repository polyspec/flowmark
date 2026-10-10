package store

import (
	"bytes"
	"fmt"
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
	return layoutOfTable(cat, tb, "")
}

// layoutOfTable returns the layout of the list or table tb, whose placement foreign
// key is placement, "" for a document table. Its child blocks are laid out in turn.
func layoutOfTable(cat *catalog.Catalog, tb *catalog.Table, placement string) (*docLayout, error) {
	table := tb.Name
	lay := &docLayout{
		table: table, list: tb.Storage.Shape == catalog.ShapeList, scope: tb.RowScope, placement: placement,
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
		case c.Name == tb.Body || c.Name == tb.Order || c.Name == placement:
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
	children, err := childLayouts(cat, table)
	if err != nil {
		return nil, err
	}
	lay.children = children
	return lay, nil
}

// childLayouts returns the layouts of the block tables whose placement parent is
// table, in table name order, which is the order of their sections in a row scope.
func childLayouts(cat *catalog.Catalog, table string) ([]*docLayout, error) {
	var out []*docLayout
	for _, t := range cat.Tables {
		if t.Storage.Kind != catalog.StorageBlock || t.Storage.Parent != table {
			continue
		}
		placement, err := placementColumn(t)
		if err != nil {
			return nil, err
		}
		child, err := layoutOfTable(cat, t, placement)
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
// child blocks that its detail section has read.
type docRec struct {
	row    *Row
	lay    *docLayout
	line   int
	detail bool
	seen   map[string]bool
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
	details map[string]bool // the heading text of each detail section read: two of one text collide
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
	p := newDocParser(&sink{rel: rel}, rel, lk)
	text, ok := p.normalize(data)
	if !ok {
		return nil, p.diags, nil
	}
	recs := p.run(lay, splitLines(text))
	if p.fatal {
		return nil, p.diags, nil
	}
	rows := sortedRows(lay, recs)
	out, rd := renderDocument(lay, rows, lk, false)
	if len(rd) > 0 {
		return nil, append(p.diags, rd...), nil
	}
	if !hasRule(p.diags, RuleNoncanonical) && !bytes.Equal(out, []byte(text)) {
		line, col := diffAt([]byte(text), out)
		p.warn(line, col, RuleNoncanonical, "the document is not in its canonical form")
	}
	return rows, p.diags, nil
}

// newDocParser returns a parser of the file rel, whose diagnostics go to s.
func newDocParser(s *sink, rel string, lk Linker) *docParser {
	return &docParser{sink: s, rel: rel, lk: lk, details: map[string]bool{}}
}

// run reads the H1, the list or table of the rows, and their detail sections, and
// returns the rows of the file's top scope.
func (p *docParser) run(lay *docLayout, lines []string) []*docRec {
	i := 0
	if len(lines) > 0 {
		if level, text, ok := headingOf(lines[0]); ok && level == 1 && trimWhite(text) == lay.table {
			if text != lay.table {
				p.warn(1, 3, RuleNoncanonical, "the H1 is written `# "+lay.table+"`")
			}
			i = 1
		}
	}
	if i == 0 {
		p.warn(1, 1, RuleNoncanonical, "the document has no H1 line `# "+lay.table+"`")
	}
	if i < len(lines) && lines[i] == "" {
		p.warn(i+1, 1, RuleNoncanonical, "a blank line between the H1 and the rows")
		i++
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
	return strings.HasPrefix(strings.TrimLeft(line, " "), "- ")
}

// readList reads the list rows from line i: two spaces of indentation per level,
// each row the child of the last row of the level above it. It returns the index
// of the first line after the rows.
func (p *docParser) readList(lines []string, i int) int {
	var chain []string // the keys of the open rows, by depth
	for ; i < len(lines) && isListRow(lines[i]); i++ {
		lineNo := i + 1
		line := lines[i]
		if trimmed := strings.TrimRight(line, " \t"); trimmed != line {
			p.warn(lineNo, colAt(line, len(trimmed)), RuleNoncanonical, "a space or a tab at the end of the row")
			line = trimmed
		}
		indent := indentOf(line)
		depth := indent / 2
		switch {
		case indent%2 != 0:
			p.fail(lineNo, 1, RuleBlock, "a row is indented by two spaces per level")
			continue
		case depth > len(chain):
			p.fail(lineNo, 1, RuleBlock, "a row is indented two spaces under the row of the level above it")
			continue
		case depth > 0 && p.lay.parent == "":
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
		p.recs = append(p.recs, rec)
	}
	return i
}

// listRow reads one list row of line, whose marker is at indent: the state glyph,
// the key, the title and the fields. It returns nil when the row is refused.
func (p *docParser) listRow(lineNo int, line string, indent int) *docRec {
	lay := p.lay
	off := p.separator(lineNo, line, indent+1)
	var state string
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
		state = st
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
	title, msg := titleOf(lay, line[titleStart:titleEnd])
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
	for k, tok := range toks {
		p.tokenSeparator(lineNo, line, titleStart, k, tok)
	}
	rec := &docRec{row: newRow(lay.table, key, title), lay: lay, line: lineNo}
	if state != "" {
		rec.row.Scalars[lay.state] = sp(state)
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

// tokenSeparator checks the white space before field k of a list row, which is
// the title's end for the first field: a tab that ends the title before a field is
// format.block, another tab or a run of spaces noncanonical.
func (p *docParser) tokenSeparator(lineNo int, line string, titleStart, k int, tok tailTok) {
	sep := line[tok.sepAt:tok.start]
	switch {
	case sep == " ":
	case k == 0 && strings.Contains(sep, "\t"):
		p.fail(lineNo, colAt(line, titleStart), RuleBlock, "a tab ends the title before a field")
	default:
		p.warn(lineNo, colAt(line, tok.sepAt), RuleNoncanonical, "the separator is one space")
	}
}

// titleOf reads the title of a list row: a token <column>\: is <column>:, and a
// token that begins with a declared column and a colon is refused, since the write
// escapes it. It returns the title and a fault, if any.
func titleOf(lay *docLayout, raw string) (string, string) {
	toks := strings.Split(raw, " ")
	for i, t := range toks {
		for _, n := range lay.names() {
			if strings.HasPrefix(t, n+`\:`) {
				toks[i] = n + ":" + t[len(n)+2:]
				break
			}
			if strings.HasPrefix(t, n+":") {
				return "", "the title token " + n + ": is not escaped as " + n + `\:`
			}
		}
	}
	return strings.Join(toks, " "), ""
}

// newRow returns a row of table with the given key and title and no value yet.
func newRow(table, key, title string) *Row {
	return &Row{
		Table: table, Key: key, Title: title,
		Scalars: map[string]*string{}, Sections: map[string]*string{},
		Raw: map[string]string{}, Dangling: map[string]DanglingLink{},
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

// splitTableCells splits a table row, which begins and ends with a pipe, into its
// cells. A pipe that directly follows a backslash is text, and that one backslash
// is removed.
func splitTableCells(line string) []tableCell {
	var cells []tableCell
	var cur strings.Builder
	start := 1
	for i := 1; i < len(line)-1; i++ {
		switch c := line[i]; {
		case c == '\\' && line[i+1] == '|' && i+1 < len(line)-1:
			cur.WriteByte('|')
			i++
		case c == '|':
			cells = append(cells, tableCell{raw: cur.String(), at: start})
			cur.Reset()
			start = i + 1
		default:
			cur.WriteByte(c)
		}
	}
	return append(cells, tableCell{raw: cur.String(), at: start})
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
	end := i
	for end < len(lines) && strings.HasPrefix(lines[end], "|") {
		end++
	}
	if end == i {
		return i
	}
	if end-i < 2 {
		p.fail(i+1, 1, RuleBlock, "a table has a header row and a delimiter row")
		return end
	}
	want := []string{lay.key.Name}
	for _, c := range lay.scalars {
		want = append(want, c.Name)
	}
	header := splitTableCells(lines[i])
	names := make([]string, len(header))
	for j, c := range header {
		names[j] = trimWhite(c.raw)
	}
	if !slices.Equal(names, want) {
		p.fail(i+1, 1, RuleBlock, "the header is not the columns "+strings.Join(want, ", ")+" of "+lay.table)
		return end
	}
	delim := splitTableCells(lines[i+1])
	if len(delim) != len(want) {
		p.fail(i+2, 1, RuleBlock, "the delimiter row has one cell per column of the header")
		return end
	}
	for _, c := range delim {
		switch t := trimWhite(c.raw); {
		case c.raw == " --- ":
		case t != "" && strings.Trim(t, ":") != "" && strings.Trim(strings.Trim(t, ":"), "-") == "":
			p.warn(i+2, colAt(lines[i+1], c.at), RuleNoncanonical, "the delimiter is written `---`")
		default:
			p.fail(i+2, colAt(lines[i+1], c.at), RuleBlock, "the delimiter row is not one")
			return end
		}
	}
	for k := i + 2; k < end; k++ {
		if rec := p.tableRow(k+1, lines[k]); rec != nil {
			p.recs = append(p.recs, rec)
		}
	}
	return end
}

// tableRow reads one row of a table: the key cell, then one cell per scalar column
// in column order. It returns nil when the row is refused.
func (p *docParser) tableRow(lineNo int, text string) *docRec {
	lay := p.lay
	if trimmed := strings.TrimRight(text, " \t"); trimmed != text {
		p.warn(lineNo, colAt(text, len(trimmed)), RuleNoncanonical, "a space or a tab at the end of the row")
		text = trimmed
	}
	if strings.HasSuffix(text, `\|`) {
		p.fail(lineNo, colAt(text, len(text)-1), RuleBlock, "the row ends with an escaped pipe, so it has no cell boundary at its end")
		return nil
	}
	cells := splitTableCells(text)
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
	for k := i; k < first; k++ {
		if lines[k] != "" {
			p.fail(k+1, 1, RuleBlock, "content after the list or table is not a detail section")
			break
		}
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
	parts := strings.Fields(trimWhite(h.text))
	if len(parts) != 3 || parts[0] != lay.table || !strings.EqualFold(parts[1], "row") || parts[2] == "" {
		p.fail(h.at+1, 4, RuleBlock, "a detail section is `"+marks+" "+lay.table+" row <key>`")
		return
	}
	key := parts[2]
	if h.last != h.at || h.text != lay.table+" row "+key {
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
	body, _ := p.valueLines(lines, h.last+1, bEnd, true)
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
		if s.text != name {
			p.warn(s.at+1, 5, RuleNoncanonical, "the block heading has extra spaces")
		}
		if rows := p.readChildBlock(lines, s, sEnd, child); len(rows) > 0 {
			setBlock(rec.row, child.table, rows)
		}
		return
	}
	if s.text != name {
		p.warn(s.at+1, 5, RuleNoncanonical, "the section heading has extra spaces")
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
	if i < end && lines[i] == "" {
		p.warn(i+1, 1, RuleNoncanonical, "a blank line between the block heading and its rows")
		i++
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
// belong to the rows of the file, where a link finds them.
func (p *docParser) checkScope(recs []*docRec) {
	lay := p.lay
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
		p.fail(recs[unreached[0]].line, 1, RuleParentCycle, "the parents of the rows form a cycle")
	}
	p.all = append(p.all, recs...)
}

// resolveLinks checks each foreign key of the file against the row it names, once
// every row of the file is known: a key that names no row keeps its written target,
// and a target other than the derived one is noncanonical.
func (p *docParser) resolveLinks() {
	for _, l := range p.links {
		want, ok := p.linkTarget(l.table, l.key)
		if !ok {
			if l.column != l.rec.lay.parent {
				l.rec.row.Dangling[l.column] = DanglingLink{Key: l.key, Target: l.target}
			}
			continue
		}
		if want != l.target {
			p.warn(l.line, l.col, RuleNoncanonical, "the link target is "+want)
		}
	}
}

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

// linkTarget derives the target of a link to the row key of table: for a row of
// this file, the file's name with the anchor of the row's detail section when it
// has one; otherwise the linker's target. It reports false when no row has the key.
func (p *docParser) linkTarget(table, key string) (string, bool) {
	for _, rec := range p.all {
		if rec.row.Table == table && rec.row.Key == key {
			return localLink(rec.lay, p.rel, rec.row), true
		}
	}
	target, err := p.lk.Target(p.rel, table, key)
	return target, err == nil
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
