package store

import (
	"strings"
	"unicode/utf8"
)

// The block structure that the sections of a row depend on, as CommonMark 0.31.2
// reads it at the top level: ATX and setext headings, fenced code blocks,
// paragraphs, the HTML blocks of types 1 to 7, and containers. A line of four or
// more spaces of indent is code, never a heading or a fence.

// heading is an ATX or a setext heading: the first line of its text and its last line
// (the underline of a setext heading), its level, and its text. The link reference
// definitions at the start of a setext heading's paragraph are no part of it.
type heading struct {
	at, last, level int
	text            string
	tab             bool     // the separator after the hashes is a tab, a noncanonical spelling
	nested          bool     // the heading is inside a block quote, a list item or a footnote definition
	note            *noteDef // the innermost footnote definition that holds the heading, under blockSyntax.notes
}

// noteDef is a footnote definition that a reading with blockSyntax.notes opens: its label
// as written, and the footnote definition that holds it.
type noteDef struct {
	label  string
	parent *noteDef
}

// outermost returns the footnote definition that holds d and is held by none.
func (d *noteDef) outermost() *noteDef {
	for d.parent != nil {
		d = d.parent
	}
	return d
}

// blockSyntax is the block syntax that a viewer reads besides CommonMark 0.31.2, which
// changes the lines that are headings (spec 10, Detail headings and anchors). The zero
// value reads CommonMark alone.
type blockSyntax struct {
	// notes reads the footnote definitions of cmark-gfm and comrak 0.56.0, as GitHub and
	// GitLab render with footnotes: a line that begins, after at most three spaces, with
	// "[^", a label of no ']', space, tab or line ending, and "]:", then any spaces and
	// tabs, opens a container, also where it interrupts a paragraph; the container goes on
	// over a line indented four columns or more, whose first four it takes, and over a
	// line that is empty, and its paragraph lazily over any line that opens no other block.
	notes bool
	// math reads the math_block rule of @vscode/markdown-it-katex 1.1.2, as VS Code renders
	// with its markdown-math extension (mathOpens): a line that begins, after at most three
	// spaces, with "$$" opens a block that takes in every line up to the next that holds
	// "$$", or to the end of its container, whose next line it also takes in.
	math bool
	// tables reads the GFM tables (GFM 0.29 §4.10) that GitHub, GitLab and VS Code render: a delimiter row after a
	// line of a paragraph with as many cells makes that line the header row of a table, and the lines after the
	// delimiter row are its body rows until a blank line or a block that interrupts a paragraph. A line of the table is
	// no paragraph line, so a setext underline after a body row underlines nothing.
	tables bool
	// mdTables reads the tables as markdown-it 14.3.2's table rule reads them, which VS Code renders, under tables
	// (mdTable): the rule runs first at the start of every block, so a line that would open a heading, a fence, an HTML
	// block, a math block, a list item or a block quote is a header row when a delimiter row follows it, and it ends an
	// open paragraph at its header row; the delimiter row of a header row that holds a pipe needs no pipe itself, so
	// `| a |` over `---` is a table of one column where GitHub and GitLab read a setext heading.
	mdTables bool
	// cellLimit is the limit of the cells a table autocompletes in its body rows, past which it takes no more rows, as
	// its viewer counts them (the slug vectors, `table cell limit`): a row of fewer cells than the header row
	// autocompletes the rest.
	cellLimit cellLimit
}

// cellLimit is a viewer's limit of the cells a table autocompletes: max cells, checked before a row (cmark-gfm and
// comrak, which take a row while the cells autocompleted so far are at most max) or with it (markdown-it, which takes
// no row that would autocomplete past max). The zero value sets no limit.
type cellLimit struct {
	max     int
	withRow bool
}

// The cell limits of the viewers: cmark-gfm's 0x80000 as GitHub renders, comrak 0.56.0's 500000 as GitLab renders, and
// markdown-it 14's MAX_AUTOCOMPLETED_CELLS 0x10000 as VS Code renders, each observed by the slug vectors.
var (
	ghCells = cellLimit{max: 0x80000}
	glCells = cellLimit{max: 500000}
	vsCells = cellLimit{max: 0x10000, withRow: true}
)

// takes reports whether a table that has autocompleted filled cells takes a row that would autocomplete auto more.
func (c cellLimit) takes(filled, auto int) bool {
	if c.max == 0 {
		return true
	}
	if c.withRow {
		return filled+auto <= c.max
	}
	return filled <= c.max
}

// textCol returns the 1-based column, in code points, of the first character of the
// text of h, a heading outside every container, in the lines it was read from: after
// the indent, the hashes and the white space after them on the line of an ATX heading,
// and after the white space that begins the first line of a setext heading's text.
func (h heading) textCol(lines []string) int {
	l := lines[h.at]
	p := indentOf(l)
	if h.last == h.at {
		p += h.level
	}
	for p < len(l) && (l[p] == ' ' || l[p] == '\t') {
		p++
	}
	return 1 + utf8.RuneCountInString(l[:p])
}

// spelled reports whether h is spelled as the canonical heading of text: an ATX heading
// on one line, its hashes, one space and text. A tab after the hashes, other white
// space around or inside the text, and a setext underline are noncanonical spellings
// of the same heading.
func (h heading) spelled(text string) bool {
	return h.last == h.at && !h.tab && h.text == text
}

// headingOf reads an ATX heading: up to three spaces, one to six '#', then a
// space, a tab or the end of the line. The text is the rest of the line after that
// one separator. tab reports whether the separator is a tab: the canonical
// separator is one space, so a tab is a noncanonical spelling of the heading.
func headingOf(line string) (level int, text string, tab, ok bool) {
	s := indentOf(line)
	if s > 3 {
		return 0, "", false, false
	}
	h := s
	for h < len(line) && line[h] == '#' {
		h++
	}
	level = h - s
	if level < 1 || level > 6 {
		return 0, "", false, false
	}
	if h == len(line) {
		return level, "", false, true
	}
	switch line[h] {
	case ' ':
		return level, line[h+1:], false, true
	case '\t':
		return level, line[h+1:], true, true
	}
	return 0, "", false, false
}

// setextLevel reads a setext underline: up to three spaces, then only '=' (level 1)
// or only '-' (level 2), then any spaces or tabs. It returns 0 for any other line.
func setextLevel(line string) int {
	s := indentOf(line)
	if s > 3 {
		return 0
	}
	t := strings.TrimRight(line[s:], " \t")
	switch {
	case t == "":
		return 0
	case strings.Trim(t, "=") == "":
		return 1
	case strings.Trim(t, "-") == "":
		return 2
	}
	return 0
}

// isListItem reports whether line opens a list item: up to three spaces, a
// '-' and then a space or the end of the line.
func isListItem(line string) bool {
	s := indentOf(line)
	return s <= 3 && s < len(line) && line[s] == '-' && (s+1 == len(line) || line[s+1] == ' ')
}

// isBlank reports whether line holds only spaces and tabs, or nothing.
func isBlank(line string) bool { return trimWhite(line) == "" }

// indented reports whether line begins with two or more columns of white space,
// spaces or a tab.
func indented(line string) bool {
	return strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")
}

// fenceMark is an open fenced block: its character and the length of its opening run.
type fenceMark struct {
	ch byte
	n  int
}

// fenceOpenAt reads an opening fence: up to three spaces, at least three
// backticks or tildes, and for backticks no backtick in the info string.
func fenceOpenAt(line string) (fenceMark, bool) {
	s := indentOf(line)
	if s > 3 || s >= len(line) || (line[s] != '`' && line[s] != '~') {
		return fenceMark{}, false
	}
	ch := line[s]
	n := runOf(line[s:], ch)
	if n < 3 {
		return fenceMark{}, false
	}
	if ch == '`' && strings.Contains(line[s+n:], "`") {
		return fenceMark{}, false
	}
	return fenceMark{ch: ch, n: n}, true
}

// closesFence reports whether line closes the open fence: up to three spaces,
// a run of its character at least as long, and nothing but spaces after it.
func closesFence(line string, f fenceMark) bool {
	s := indentOf(line)
	if s > 3 || s >= len(line) || line[s] != f.ch {
		return false
	}
	n := runOf(line[s:], f.ch)
	return n >= f.n && strings.Trim(line[s+n:], " ") == ""
}

// indentOf counts the leading spaces of line.
func indentOf(line string) int {
	s := 0
	for s < len(line) && line[s] == ' ' {
		s++
	}
	return s
}

// runOf counts the leading run of ch in s.
func runOf(s string, ch byte) int {
	n := 0
	for n < len(s) && s[n] == ch {
		n++
	}
	return n
}

// block is an open container of the block scanner: a block quote, or a list item.
// A list item holds the column its content starts at, indent, relative to the text
// its container passes on, and empty while it has no content line yet: CommonMark
// ends an item that begins with a blank line when a second blank line follows.
type block struct {
	quote  bool
	indent int
	empty  bool
	note   *noteDef // a footnote definition, under blockSyntax.notes
}

// leafKind is the leaf block that is open in the innermost container.
type leafKind int

const (
	leafNone leafKind = iota
	leafPara
	leafFence
	leafHTML
	leafMath  // a math block, under blockSyntax.math
	leafTable // a table, under blockSyntax.tables
)

// scanner reads the block structure of lines one at a time, as CommonMark's
// container matching reads it: the open containers, outermost first, and the
// leaf block open in the innermost one. A heading is reported whether it is inside
// a container or not, and a top-level heading is the one a section boundary is;
// a fence holds no heading, and an HTML block holds no heading either.
type scanner struct {
	syn   blockSyntax
	stack []block
	leaf  leafKind
	fence fenceMark  // the open fence, when leaf is leafFence
	html  int        // the kind of the open HTML block, when leaf is leafHTML
	units []unit     // the paragraphs and HTML blocks read so far
	cur   int        // the index in units of the open paragraph or HTML block
	notes []*noteDef // the footnote definitions read so far
	// blank is whether the last line read was blank inside every container it continued,
	// which ends a block quote before a lazy line under blockSyntax.math.
	blank bool
	// delim is set when an mdTables table opened at the line read, whose next line is its delimiter row (mdTable).
	delim bool
	// cols is the number of columns of the open table, and filled the cells its body rows have autocompleted, when
	// leaf is leafTable.
	cols, filled int
	tables       []table // the tables read so far, the open one last
}

// unit is the text of a paragraph or of an HTML block: its lines without the markers
// of the containers that hold them, and the index of its first line. A paragraph that
// a setext underline closes is a heading, so it is marked and left out of the text.
type unit struct {
	at   int
	html bool
	// pipes is whether the viewer reads each `\|` of the paragraph as `|`: cmark-gfm and comrak unescape the pipes of
	// the paragraph that the header row of a table leaves (try_inserting_table_header_paragraph); markdown-it ends the
	// paragraph before the header row, and reads it as it stands.
	pipes   bool
	lines   []string
	heading bool
	note    *noteDef // the innermost footnote definition that holds it, under blockSyntax.notes
	// noTable is whether a delimiter row stood under the paragraph without a header row of as many cells: cmark-gfm
	// and comrak then open no table in that paragraph (the slug vectors, `table header after a failed header in its
	// paragraph`); markdown-it opens one all the same.
	noTable bool
}

// scan is the block structure of a run of lines from some index on.
type scan struct {
	heads []heading // the top-level headings
	all   []heading // every heading, those inside a container too
	units []unit    // the paragraphs and HTML blocks that are not headings
	// blocks is every paragraph and HTML block, the setext headings among them.
	blocks []unit
	// open is whether a fence or an HTML block is open at the top level at the end:
	// such a block takes in everything after it, so a value that leaves one open
	// is followed by no element of its own.
	open bool
	// notes is every footnote definition, in the order they open, under blockSyntax.notes.
	notes []*noteDef
	// tables is every table, in the order they open, under blockSyntax.tables.
	tables []table
}

// table is a GFM table of a scan: its rows, the header row first, and the innermost footnote definition that holds
// it, under blockSyntax.notes.
type table struct {
	rows []tableRow
	note *noteDef
}

// tableRow is a row of a table: the index of its line, the line's text after the markers of its containers, and the
// byte ranges in that text of the cells the viewer renders, each without the white space around it. A body row holds
// no cell past the columns of the header row: every viewer drops those cells (the slug vectors, `raw heading in an
// excess table cell`).
type tableRow struct {
	at    int
	rest  string
	cells [][2]int
}

// scanHeadings reads the block structure of lines from index from on. A setext
// heading is the paragraph before its underline after the link reference definitions
// at its start; a fence or an HTML block hides the lines it holds, headings included.
func scanHeadings(lines []string, from int) scan { return scanWith(lines, from, blockSyntax{}) }

// scanWith reads the block structure of lines from index from on, with the block
// syntax syn besides CommonMark's.
func scanWith(lines []string, from int, syn blockSyntax) scan {
	var sc scan
	s := scanner{syn: syn}
	for i := from; i < len(lines); i++ {
		h, ok := s.feed(lines, i)
		if ok {
			sc.all = append(sc.all, h)
			if !h.nested {
				sc.heads = append(sc.heads, h)
			}
		}
	}
	sc.blocks = s.units
	for _, u := range s.units {
		if !u.heading {
			sc.units = append(sc.units, u)
		}
	}
	sc.open = len(s.stack) == 0 && (s.leaf == leafFence || s.leaf == leafHTML)
	sc.notes = s.notes
	sc.tables = s.tables
	return sc
}

// innerNote returns the innermost footnote definition open, or nil.
func (s *scanner) innerNote() *noteDef {
	for k := len(s.stack) - 1; k >= 0; k-- {
		if s.stack[k].note != nil {
			return s.stack[k].note
		}
	}
	return nil
}

// addLine adds the text of a line to the open paragraph or HTML block.
func (s *scanner) addLine(text string) {
	s.units[s.cur].lines = append(s.units[s.cur].lines, text)
}

// paraText is the text of the lines of a paragraph: each without the white space around
// it, joined by line feeds, as a setext heading's text is.
func paraText(lines []string) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = strings.TrimRight(strings.TrimLeft(l, " \t"), " \t")
	}
	return strings.Join(parts, "\n")
}

// textStart returns the index of the first line of a paragraph that is text: the link
// reference definitions at its start are no text of it (CommonMark 0.31.2 §4.7), so the
// text begins at the first line they do not consume, or at len(lines) when they consume
// every line.
func textStart(lines []string) int {
	_, consumed := cmDefLines(lines)
	k := 0
	for k < len(lines) && consumed[k] {
		k++
	}
	return k
}

// feed reads line i of lines and returns the heading that it completes when the
// heading is outside every container.
func (s *scanner) feed(lines []string, i int) (heading, bool) {
	rest, col := expandLead(lines[i], 0), 0
	m := 0
	for ; m < len(s.stack); m++ {
		r, c, ok := s.stack[m].match(rest, col, lines[i])
		if !ok {
			break
		}
		rest, col = r, c
	}
	if !isBlank(lines[i]) {
		for k := 0; k < m; k++ {
			s.stack[k].empty = false
		}
	}
	blank := s.blank
	s.blank = m == len(s.stack) && isBlank(rest)
	if m < len(s.stack) {
		// Only a paragraph continues lazily into a container that did not match. Under mdTables a table whose header
		// row the line is ends the paragraph, unless a block quote is among the containers it does not continue:
		// markdown-it reads a lazy line of a block quote as no block start (sCount -1), and one of a list item
		// alone as a line the paragraph checks for its end.
		if s.leaf == leafPara && s.syn.lazyText(rest) && !s.lazyTable(lines, i, rest, m) {
			s.addLine(rest)
			return heading{}, false
		}
		if s.leaf == leafMath {
			s.mathEnd(m, rest, blank)
			return heading{}, false
		}
		s.stack = s.stack[:m]
		s.leaf = leafNone
	}
	switch s.leaf {
	case leafTable:
		// A body row, unless the line is blank, is indented four columns or more, or opens a block that interrupts a
		// paragraph, which ends the table: every viewer reads an indented line after a table as indented code (the slug
		// vectors, `table rows ended by an indented line`).
		if s.delim {
			s.delim = false
			return heading{}, false
		}
		auto := s.cols - min(tableRowCells(rest), s.cols)
		if s.syn.mdTables {
			// markdown-it counts a row of more cells than the header row as negative autocompletion.
			auto = s.cols - len(mdRowSpans(rest))
		}
		if isBlank(rest) || s.syn.mdTables && strings.TrimFunc(rest, jsTrimmed) == "" || indentOf(rest) >= 4 ||
			s.syn.opensInterrupt(rest) || s.syn.endsCmarkTable(rest) || !s.syn.cellLimit.takes(s.filled, auto) {
			s.leaf = leafNone
			if isBlank(rest) {
				return heading{}, false
			}
			return s.start(lines, i, rest, col)
		}
		s.filled += auto
		s.addRow(i, rest)
		return heading{}, false
	case leafMath:
		if strings.Contains(rest, "$$") {
			s.leaf = leafNone
		}
		return heading{}, false
	case leafFence:
		if closesFence(rest, s.fence) {
			s.leaf = leafNone
		}
		return heading{}, false
	case leafHTML:
		// The block takes in every line up to its end condition, that line too.
		s.addLine(rest)
		if htmlEnds(s.html, rest) {
			s.leaf = leafNone
		}
		return heading{}, false
	}
	if isBlank(rest) {
		s.leaf = leafNone
		return heading{}, false
	}
	return s.start(lines, i, rest, col)
}

// tableStarts reads the line rest after the open paragraph as the delimiter row of a table when it is one with as many
// cells as the last line of the paragraph, which is then the header row (GFM 0.29 §4.10): the header row leaves the
// paragraph, and the lines after the delimiter row are the body rows of the table. It reports whether it did.
func (s *scanner) tableStarts(rest string) bool {
	cells := tableDelimiterCells(rest, false)
	if cells == 0 {
		return false
	}
	u := &s.units[s.cur]
	if len(u.lines) == 0 || u.noTable {
		return false
	}
	header := u.lines[len(u.lines)-1]
	if tableRowCells(header) != cells {
		u.noTable = true
		return false
	}
	at := u.at + len(u.lines) - 1
	u.lines = u.lines[:len(u.lines)-1]
	u.pipes = true
	s.openTable(at, header, cells)
	if len(u.lines) == 0 {
		// The paragraph was the header row alone: the table stands where the paragraph did.
		s.units = append(s.units[:s.cur], s.units[s.cur+1:]...)
		s.cur = len(s.units) - 1
	}
	return true
}

// mdTable reports whether rest, the text of line i inside the containers open, is the header row of a table as
// markdown-it 14.3.2's table rule reads it (rules_block/table.mjs), and the number of its columns: the next line
// continues every open container, not lazily, and is a delimiter row (tableDelimiterCells) indented less than four
// columns inside them, and rest, trimmed as JavaScript trims, holds a pipe and splits into as many cells (mdRowSpans).
// The rule runs at the start of every block before any other, and on every line of a paragraph that is not lazy and
// not indented four columns or more, which is where scanner.start reads a line. It runs only under mdTables.
func (s *scanner) mdTable(lines []string, i int, rest string) (int, bool) {
	if !s.syn.mdTables || i+1 >= len(lines) || !strings.Contains(strings.TrimFunc(rest, jsTrimmed), "|") {
		return 0, false
	}
	next, col := expandLead(lines[i+1], 0), 0
	for _, b := range s.stack {
		r, c, ok := b.match(next, col, lines[i+1])
		if !ok {
			return 0, false
		}
		next, col = r, c
	}
	cells := tableDelimiterCells(next, true)
	return cells, cells > 0 && cells == len(mdRowSpans(rest))
}

// lazyTable reports whether the lazy line i, of text rest after the first m containers open, is the header row of a
// table that ends the open paragraph (mdTable): only when every container it does not continue is a list item.
func (s *scanner) lazyTable(lines []string, i int, rest string, m int) bool {
	for _, b := range s.stack[m:] {
		if b.quote {
			return false
		}
	}
	_, ok := s.mdTable(lines, i, rest)
	return ok
}

// openTable opens a table of cells columns whose header row is the line at, of text header.
func (s *scanner) openTable(at int, header string, cells int) {
	s.leaf = leafTable
	s.cols, s.filled = cells, 0
	s.tables = append(s.tables, table{note: s.innerNote()})
	s.addRow(at, header)
}

// addRow adds the line at, of text rest, to the open table as a row of its cells up to the table's columns. A cell is
// the text between the pipes that tableRowSpans finds, without the white space around it: cmark-gfm and comrak trim a
// cell of spaces and tabs (a line holds no line ending), markdown-it trims it as JavaScript's String.prototype.trim.
func (s *scanner) addRow(at int, rest string) {
	spans := tableRowSpans(rest)
	if s.syn.mdTables {
		spans = mdRowSpans(rest)
	}
	row := tableRow{at: at, rest: rest}
	for _, sp := range spans[:min(len(spans), s.cols)] {
		b, e := sp[0], sp[1]
		for b < e {
			r, n := utf8.DecodeRuneInString(rest[b:])
			if !s.cellSpace(r) {
				break
			}
			b += n
		}
		for e > b {
			r, n := utf8.DecodeLastRuneInString(rest[b:e])
			if !s.cellSpace(r) {
				break
			}
			e -= n
		}
		row.cells = append(row.cells, [2]int{b, e})
	}
	t := &s.tables[len(s.tables)-1]
	t.rows = append(t.rows, row)
}

// cellSpace reports whether the viewer trims r from either end of a cell (addRow).
func (s *scanner) cellSpace(r rune) bool {
	if s.syn.mdTables {
		return jsTrimmed(r)
	}
	return r == ' ' || r == '\t'
}

// mathEnd reads a line, rest, on which the containers from the m-th on do not continue
// while a math block is open, as markdown-it-katex 1.1.2's math_block reads it inside
// markdown-it's containers. A block quote takes the line in lazily when the quote's last
// line was not blank and the line opens no block that ends a quote: the line is then a
// line of the math block. Otherwise the container ends before the line, and the math
// block takes the line in all the same — the rule sets state.line one past the line at
// which it stops — so it ends there: a block quote closes with what it holds, while a
// list item, which a line indented less than its content ends, stays open for the lines
// after.
func (s *scanner) mathEnd(m int, rest string, blank bool) {
	if s.stack[m].quote && !blank && s.syn.lazyText(rest) {
		if strings.Contains(rest, "$$") {
			s.leaf = leafNone
		}
		return
	}
	s.leaf = leafNone
	if s.stack[m].quote {
		s.stack = s.stack[:m]
	} else {
		s.stack = s.stack[:m+1]
	}
}

// mathOpens reads the line rest, which is indented less than four columns, as
// math_block of @vscode/markdown-it-katex 1.1.2 does: it opens a math block when it
// begins with "$$", after its indent, and the text after that holds no second "$$" past
// one; the block is that line alone when the text after the "$$" holds one "$$", at its
// end. A line whose text after the first "$$" holds two or more opens none.
func mathOpens(rest string) (open, oneLine bool) {
	t := rest[indentOf(rest):]
	if !strings.HasPrefix(t, "$$") {
		return false, false
	}
	first := t[2:]
	switch n := strings.Count(first, "$$"); {
	case n > 1:
		return false, false
	case n == 1 && strings.Index(first, "$$") == len(first)-2:
		return true, true
	}
	return true, false
}

// noteStart reads the start of a footnote definition at the start of s, which is
// indented less than four columns (blockSyntax.notes): it returns the length of the
// marker, the spaces and tabs after it included, and the label.
func noteStart(s string) (int, string, bool) {
	p := indentOf(s)
	if !strings.HasPrefix(s[p:], "[^") {
		return 0, "", false
	}
	q := p + 2
	for q < len(s) && s[q] != ']' && s[q] != ' ' && s[q] != '\t' && s[q] != '\r' && s[q] != '\n' {
		q++
	}
	if q == p+2 || q+1 >= len(s) || s[q] != ']' || s[q+1] != ':' {
		return 0, "", false
	}
	label := s[p+2 : q]
	q += 2
	for q < len(s) && (s[q] == ' ' || s[q] == '\t') {
		q++
	}
	return q, label, true
}

// start reads the block starts of a line that no leaf block takes, from the text
// rest at column col: a container opens and the text it holds is read again, and
// a leaf block opens or continues.
func (s *scanner) start(lines []string, i int, rest string, col int) (heading, bool) {
	top := len(s.stack) == 0
	for {
		if isBlank(rest) {
			// A container that opens with nothing after its marker holds no paragraph.
			s.leaf = leafNone
			return heading{}, false
		}
		if indentOf(rest) >= 4 {
			// Indented code, or the continuation of an open paragraph.
			if s.leaf == leafPara {
				s.addLine(rest)
			}
			return heading{}, false
		}
		if cells, ok := s.mdTable(lines, i, rest); ok {
			// The table ends an open paragraph before its header row.
			s.openTable(i, rest, cells)
			s.delim = true
			return heading{}, false
		}
		if s.leaf == leafPara && s.syn.tables && !s.syn.mdTables && s.tableStarts(rest) {
			return heading{}, false
		}
		if s.leaf == leafPara {
			// The definitions at the start of the paragraph are read before the underline
			// (§4.7): the heading is the text after them, from its own first line. A
			// paragraph of definitions alone makes no heading, and the line is read as the
			// block starts below read it, as commonmark.js and markdown-it read it: `---` a
			// thematic break, anything else text of the paragraph. cmark-gfm reads `---`
			// as text too; no reading makes it a heading.
			if level := setextLevel(rest); level > 0 {
				u := &s.units[s.cur]
				if k := textStart(u.lines); k < len(u.lines) {
					u.heading = true
					s.leaf = leafNone
					return heading{at: u.at + k, last: i, level: level, text: paraText(u.lines[k:]), nested: !top, note: u.note}, true
				}
			}
		}
		if level, text, tab, ok := headingOf(rest); ok {
			s.leaf = leafNone
			return heading{at: i, last: i, level: level, text: text, tab: tab, nested: !top, note: s.innerNote()}, true
		}
		if f, ok := fenceOpenAt(rest); ok {
			s.leaf, s.fence = leafFence, f
			return heading{}, false
		}
		if s.syn.math {
			if open, oneLine := mathOpens(rest); open {
				s.leaf = leafMath
				if oneLine {
					s.leaf = leafNone
				}
				return heading{}, false
			}
		}
		if k := htmlStart(rest); k != 0 && (k != 7 || s.leaf != leafPara) {
			s.leaf = leafNone
			s.cur = len(s.units)
			s.units = append(s.units, unit{at: i, html: true, lines: []string{rest}, note: s.innerNote()})
			if !htmlEnds(k, rest) {
				s.leaf, s.html = leafHTML, k
			}
			return heading{}, false
		}
		if isThematicBreak(rest) {
			s.leaf = leafNone
			return heading{}, false
		}
		if isQuoteStart(rest) {
			r, c := quoteText(rest, col)
			s.leaf = leafNone
			s.stack = append(s.stack, block{quote: true})
			rest, col = r, c
			top = false
			continue
		}
		if s.syn.notes {
			if n, label, ok := noteStart(rest); ok {
				d := &noteDef{label: label, parent: s.innerNote()}
				s.notes = append(s.notes, d)
				s.leaf = leafNone
				s.stack = append(s.stack, block{note: d})
				rest, col = rest[n:], col+n
				top = false
				continue
			}
		}
		if it, ok := listStart(rest, col); ok && (s.leaf != leafPara || interruptsPara(it)) {
			s.leaf = leafNone
			s.stack = append(s.stack, block{indent: it.indent, empty: it.empty})
			rest, col = it.rest, it.col
			top = false
			continue
		}
		if s.leaf == leafNone {
			s.cur = len(s.units)
			s.units = append(s.units, unit{at: i, lines: []string{rest}, note: s.innerNote()})
			s.leaf = leafPara
		} else {
			s.addLine(rest)
		}
		return heading{}, false
	}
}

// match reports whether the container continues on the line full, whose text is s at
// column col after the containers outside this one, and returns the text that the
// container passes on. A block quote continues on a line that opens with its marker; a
// list item continues on a line indented at least as far as its content, and on a blank
// line unless it is empty; a footnote definition continues on a line indented four
// columns or more, passing on the text after them, and on a line that is empty as a
// whole, as cmark-gfm's and comrak's parse_footnote_definition_block_prefix read it.
func (b block) match(s string, col int, full string) (string, int, bool) {
	if b.note != nil {
		switch {
		case full == "":
			return s, col, true
		case indentOf(s) >= 4:
			return s[4:], col + 4, true
		}
		return s, col, false
	}
	if b.quote {
		if !isQuoteStart(s) {
			return s, col, false
		}
		r, c := quoteText(s, col)
		return r, c, true
	}
	if isBlank(s) {
		return s, col, !b.empty
	}
	if indentOf(s) < b.indent {
		return s, col, false
	}
	return s[b.indent:], col + b.indent, true
}

// lazyText reports whether a line that continues no open container is lazy
// continuation text of the open paragraph: it opens no block that interrupts a
// paragraph. A setext underline is such text, as CommonMark reads it. A list item
// is not held back here: CommonMark applies the rule on a list item's start only
// to a paragraph that is in the innermost matched container, which a lazy line
// is not, so a list item that opens on such a line closes the containers.
func (y blockSyntax) lazyText(rest string) bool {
	return !isBlank(rest) && !y.opensInterrupt(rest)
}

// opensInterrupt reports whether line opens a block that interrupts a paragraph:
// an ATX heading, a fence, an HTML block of types 1 to 6, a thematic break, a
// block quote, or a list item, and the footnote definition and the math block of the
// syntax y. An HTML block of type 7 does not.
func (y blockSyntax) opensInterrupt(line string) bool {
	if indentOf(line) >= 4 {
		return false
	}
	if y.notes {
		if _, _, ok := noteStart(line); ok {
			return true
		}
	}
	if y.math {
		if open, _ := mathOpens(line); open {
			return true
		}
	}
	if _, _, _, ok := headingOf(line); ok {
		return true
	}
	if _, ok := fenceOpenAt(line); ok {
		return true
	}
	if k := htmlStart(line); (k != 0 && k != 7) || isThematicBreak(line) || isQuoteStart(line) {
		return true
	}
	_, ok := listStart(line, 0)
	return ok
}

// isThematicBreak reports whether line is a thematic break: up to three spaces,
// then three or more of one of '*', '-' or '_', with only spaces and tabs between.
func isThematicBreak(line string) bool {
	s := indentOf(line)
	if s > 3 || s >= len(line) || (line[s] != '*' && line[s] != '-' && line[s] != '_') {
		return false
	}
	n := 0
	for i := s; i < len(line); i++ {
		switch line[i] {
		case line[s]:
			n++
		case ' ', '\t':
		default:
			return false
		}
	}
	return n >= 3
}

// isQuoteStart reports whether line opens a block quote: up to three spaces, then '>'.
func isQuoteStart(line string) bool {
	s := indentOf(line)
	return s <= 3 && s < len(line) && line[s] == '>'
}

// quoteText returns the text that a block quote marker at the start of s passes on,
// from column col: the marker and one optional space after it, whose tab, when
// there is one, is spread over columns.
func quoteText(s string, col int) (string, int) {
	c := col + indentOf(s) + 1
	r := expandLead(s[indentOf(s)+1:], c)
	if strings.HasPrefix(r, " ") {
		return r[1:], c + 1
	}
	return r, c
}

// listItem is a list item marker that opens a line: the column of its content
// relative to the text it is in, indent, and that content, rest, at column col.
type listItem struct {
	indent  int
	rest    string
	col     int
	ordered bool
	number  int
	empty   bool
}

// listStart reads a list item marker at the start of s, at column col: up to three
// spaces, then '-', '+' or '*', or one to nine digits and '.' or ')', then a space,
// a tab or the end of the line. The content starts after one to four spaces; five
// or more spaces make the content an indented code block that starts after one.
func listStart(s string, col int) (listItem, bool) {
	ind := indentOf(s)
	if ind > 3 || ind >= len(s) {
		return listItem{}, false
	}
	var it listItem
	end := ind + 1
	switch c := s[ind]; {
	case c == '-' || c == '+' || c == '*':
	case c >= '0' && c <= '9':
		n := runOfDigits(s[ind:])
		if n > 9 || ind+n >= len(s) || (s[ind+n] != '.' && s[ind+n] != ')') {
			return listItem{}, false
		}
		it.ordered = true
		it.number = atoiDigits(s[ind : ind+n])
		end = ind + n + 1
	default:
		return listItem{}, false
	}
	if end < len(s) && s[end] != ' ' && s[end] != '\t' {
		return listItem{}, false
	}
	after := expandLead(s[end:], col+end)
	n := indentOf(after)
	switch {
	case n == len(after):
		it.empty = true
		it.indent, it.rest, it.col = end+1, "", col+end+1
	case n >= 5:
		it.indent, it.rest, it.col = end+1, after[1:], col+end+1
	default:
		it.indent, it.rest, it.col = end+n, after[n:], col+end+n
	}
	return it, true
}

// interruptsPara reports whether a list item may interrupt a paragraph: it has
// content, and when it is ordered it starts at 1.
func interruptsPara(it listItem) bool {
	return !it.empty && (!it.ordered || it.number == 1)
}

// runOfDigits counts the leading decimal digits of s.
func runOfDigits(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}

// atoiDigits reads a run of decimal digits of at most nine digits.
func atoiDigits(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// expandLead returns s with the white space before its first other character
// written as spaces, a tab moving to the next column that is a multiple of four,
// for text that starts at column col.
func expandLead(s string, col int) string {
	end := 0
	for end < len(s) && (s[end] == ' ' || s[end] == '\t') {
		end++
	}
	if !strings.Contains(s[:end], "\t") {
		return s
	}
	var b strings.Builder
	c := col
	for i := 0; i < end; i++ {
		if s[i] == '\t' {
			w := 4 - c%4
			b.WriteString(strings.Repeat(" ", w))
			c += w
		} else {
			b.WriteByte(' ')
			c++
		}
	}
	b.WriteString(s[end:])
	return b.String()
}

// boundary returns the index of the first heading of level 2 or less at or after
// from, outside fenced blocks and HTML blocks, or len(lines) when there is none.
func boundary(lines []string, from int) int { return boundaryLevel(lines, from, 2) }

// boundaryLevel returns the index of the first top-level heading of level or less
// at or after from, or len(lines) when there is none.
func boundaryLevel(lines []string, from, level int) int {
	for _, h := range scanHeadings(lines, from).heads {
		if h.level <= level {
			return h.at
		}
	}
	return len(lines)
}

// endsCmarkTable reports whether line ends the rows of a table as cmark-gfm and comrak 0.56.0 read them, which GitHub
// and GitLab render (tables without mdTables), beside the blocks that interrupt a paragraph: an HTML block of type 7,
// which opens there and takes in the lines after it, and a line of no cell, a pipe alone with white space around it
// (the slug vectors, `table rows ended by an HTML block of type 7` and `table row of a lone pipe`). markdown-it takes
// both as rows.
func (y blockSyntax) endsCmarkTable(line string) bool {
	if !y.tables || y.mdTables || indentOf(line) >= 4 {
		return false
	}
	return htmlStart(line) == 7 || strings.Trim(line, " \t") == "|"
}

// htmlStart reads the start of an HTML block of CommonMark 0.31.2 and returns its
// type, 1 to 7, or 0 when line starts none. A type 6 start is the name of a block
// element; a type 7 start is one complete tag and nothing but white space after it.
func htmlStart(line string) int {
	s := indentOf(line)
	if s > 3 || s >= len(line) || line[s] != '<' {
		return 0
	}
	rest := line[s:]
	lower := FoldASCII(rest)
	for _, name := range []string{"script", "pre", "style", "textarea"} {
		if strings.HasPrefix(lower, "<"+name) {
			after := rest[len(name)+1:]
			if after == "" || after[0] == '>' || jsSpaceAt(after) > 0 {
				return 1
			}
		}
	}
	switch {
	case strings.HasPrefix(rest, "<!--"):
		return 2
	case strings.HasPrefix(rest, "<?"):
		return 3
	case strings.HasPrefix(rest, "<![CDATA["):
		return 5
	case len(rest) > 2 && rest[1] == '!' && isASCIILetter(rest[2]):
		return 4
	}
	if blockTag6(rest) {
		return 6
	}
	if htmlTag7(rest) {
		return 7
	}
	return 0
}

// blockTag6 reports whether s starts with a block element's name, opened or
// closed, that is followed by a space, a tab, the end of s, '>' or '/>'. The name
// is compared without regard to ASCII case, as commonmark.js reads it.
func blockTag6(s string) bool {
	p := 1
	if p < len(s) && s[p] == '/' {
		p++
	}
	q := p
	for q < len(s) && (isASCIILetter(s[q]) || ('0' <= s[q] && s[q] <= '9')) {
		q++
	}
	if !blockTagNames[FoldASCII(s[p:q])] {
		return false
	}
	return q == len(s) || s[q] == '>' || jsSpaceAt(s[q:]) > 0 ||
		(s[q] == '/' && q+1 < len(s) && s[q+1] == '>')
}

// blockTagNames are the element names of CommonMark 0.31.2's type 6 start condition.
var blockTagNames = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Fields(`address article aside base basefont blockquote body caption
		center col colgroup dd details dialog dir div dl dt fieldset figcaption figure footer form
		frame frameset h1 h2 h3 h4 h5 h6 head header hr html iframe legend li link main menu
		menuitem nav noframes ol optgroup option p param section search summary table tbody td
		tfoot th thead title tr track ul`) {
		m[n] = true
	}
	return m
}()

// htmlTag7 reports whether s is one complete open tag or closing tag followed by
// white space only: the type 7 start condition of CommonMark 0.31.2.
func htmlTag7(s string) bool {
	end := htmlTagEnd(s)
	return end >= 0 && skipSpace(s, end) == len(s)
}

// htmlTagEnd returns the index just after the open or closing tag that starts s,
// or -1 when s starts with none (see htmlTagAt).
func htmlTagEnd(s string) int {
	end, _ := htmlTagAt(s)
	return end
}

// tagNameEnd returns the index after the tag name at s[p:], a letter then letters,
// digits and dashes, or -1 when no letter starts it.
func tagNameEnd(s string, p int) int {
	if p >= len(s) || !isASCIILetter(s[p]) {
		return -1
	}
	for p++; p < len(s) && (isASCIILetter(s[p]) || ('0' <= s[p] && s[p] <= '9') || s[p] == '-'); p++ {
	}
	return p
}

// attrNameEnd returns the index after the attribute name at s[p:], a letter,
// underscore or colon, then letters, digits, colons, periods, underscores and
// dashes; -1 when none starts there.
func attrNameEnd(s string, p int) int {
	if p >= len(s) || (!isASCIILetter(s[p]) && s[p] != '_' && s[p] != ':') {
		return -1
	}
	for p++; p < len(s) && (isASCIILetter(s[p]) || ('0' <= s[p] && s[p] <= '9') || strings.IndexByte(":._-", s[p]) >= 0); p++ {
	}
	return p
}

// attrSpecEnd returns the index after an attribute value specification at s[p:],
// optional white space, '=', optional white space and a value, or -1 when there
// is none.
func attrSpecEnd(s string, p int) int {
	q := skipSpace(s, p)
	if q >= len(s) || s[q] != '=' {
		return -1
	}
	return attrValueEnd(s, skipSpace(s, q+1))
}

// attrValueEnd returns the index after the attribute value at s[p:]: an unquoted
// value of characters other than quotes, '=', '<', '>', a backtick and the
// control characters and space, or a value in single or double quotes. It
// returns -1 when none starts there.
func attrValueEnd(s string, p int) int {
	if p >= len(s) {
		return -1
	}
	if c := s[p]; c == '"' || c == '\'' {
		i := strings.IndexByte(s[p+1:], c)
		if i < 0 {
			return -1
		}
		return p + 1 + i + 1
	}
	q := p
	for q < len(s) && s[q] > 0x20 && strings.IndexByte("\"'=<>`", s[q]) < 0 {
		q++
	}
	if q == p {
		return -1
	}
	return q
}

// skipSpace returns the index of the first character at or after p that is not
// white space, as JavaScript's \s reads it: the ASCII white space and line
// terminators, and the Unicode space separators.
func skipSpace(s string, p int) int {
	for p < len(s) {
		n := jsSpaceAt(s[p:])
		if n == 0 {
			break
		}
		p += n
	}
	return p
}

// jsSpaceAt returns the byte length of the white space character at the start of
// s, as JavaScript's \s reads it, or 0 when s does not start with one.
func jsSpaceAt(s string) int {
	r, n := utf8.DecodeRuneInString(s)
	switch {
	case r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r' || r == ' ':
		return n
	case r == 0xa0 || r == 0x1680 || (0x2000 <= r && r <= 0x200a) || r == 0x2028 || r == 0x2029 ||
		r == 0x202f || r == 0x205f || r == 0x3000 || r == 0xfeff:
		return n
	}
	return 0
}

// isASCIILetter reports whether c is an ASCII letter.
func isASCIILetter(c byte) bool { return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') }

// FoldASCII lowers the ASCII letters of s, keeping every other byte, so that its
// length is s's length.
func FoldASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// htmlEnds reports whether line holds the end condition of an HTML block of type
// kind: for types 1 to 5 the closing marker of the block, for types 6 and 7 a
// blank line, which the block does not take in.
func htmlEnds(kind int, line string) bool {
	switch kind {
	case 1:
		lower := FoldASCII(line)
		for _, end := range []string{"</script>", "</pre>", "</style>", "</textarea>"} {
			if strings.Contains(lower, end) {
				return true
			}
		}
	case 2:
		return strings.Contains(line, "-->")
	case 3:
		return strings.Contains(line, "?>")
	case 4:
		return strings.Contains(line, ">")
	case 5:
		return strings.Contains(line, "]]>")
	case 6, 7:
		return isBlank(line)
	}
	return false
}

// splitLines splits text, which ends with a line feed or is empty, into its
// lines without the line feeds.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// joinLines writes lines as text, each one terminated by a line feed.
func joinLines(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}

// tableDelimiterCells returns the number of cells of line when it is the delimiter row of a GFM table (GFM 0.29
// §4.10), and 0 when it is not: up to three spaces of indent, then cells separated by pipes, each of hyphens with an
// optional colon at either end and spaces around, a pipe at either end optional. A row of one cell needs a pipe, since
// hyphens alone are a thematic break or a setext underline; markdown-it (pipeless) takes it, its header row holding a
// pipe, and reads the row by mdDelimiterCells.
func tableDelimiterCells(line string, pipeless bool) int {
	if indentOf(line) > 3 {
		return 0
	}
	if pipeless {
		return mdDelimiterCells(line)
	}
	// cmark-gfm and comrak read a vertical tab or a form feed around a delimiter row as white space; markdown-it
	// (pipeless) does not (the slug vectors, `table delimiter row with a vertical tab`).
	space := " \t\v\f"
	if pipeless {
		space = " \t"
	}
	t := strings.Trim(line, space)
	if t == "" || !pipeless && !strings.Contains(t, "|") {
		return 0
	}
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	cells := strings.Split(t, "|")
	for _, cell := range cells {
		c := strings.Trim(cell, space)
		c = strings.TrimPrefix(c, ":")
		c = strings.TrimSuffix(c, ":")
		if c == "" || strings.Trim(c, "-") != "" {
			return 0
		}
	}
	return len(cells)
}

// tableRowCells returns the number of cells of a table row: the cells between the pipes that a backslash does not
// escape, a pipe at either end optional. A pipe right after a backslash is escaped even when that backslash is itself
// escaped: GitHub, GitLab and VS Code split the row before they read escapes, so `a\\|b` is one cell (the slug
// vectors, `table header cells with escaped pipes`). A code span protects no pipe: they split the row there (GFM 0.29
// §4.10 example 200).
func tableRowCells(line string) int { return len(tableRowSpans(line)) }

// mdDelimiterCells returns the number of cells of line when markdown-it 14.3.2's table rule reads it as a delimiter
// row, and 0 when it does not: after the spaces and tabs that begin it, its first character is a pipe, a hyphen or a
// colon, its second one of those or a space or a tab, never a space or a tab after a hyphen, which would open a list
// item, and the rest only those; split at every pipe, each part trimmed is hyphens with an optional colon at either
// end, an empty part standing only first or last.
func mdDelimiterCells(line string) int {
	t := strings.TrimLeft(line, " \t")
	if len(t) < 2 || !strings.ContainsRune("|-:", rune(t[0])) || !strings.ContainsRune("|-: \t", rune(t[1])) ||
		t[0] == '-' && (t[1] == ' ' || t[1] == '\t') || strings.Trim(t, "|-: \t") != "" {
		return 0
	}
	parts := strings.Split(t, "|")
	n := 0
	for k, p := range parts {
		c := strings.Trim(p, " \t")
		if c == "" {
			if k == 0 || k == len(parts)-1 {
				continue
			}
			return 0
		}
		c = strings.TrimPrefix(c, ":")
		c = strings.TrimSuffix(c, ":")
		if c == "" || strings.Trim(c, "-") != "" {
			return 0
		}
		n++
	}
	return n
}

// pipeSpans returns the byte ranges of line[from:to] between the pipes that do not follow a backslash, the first from
// from and the last to to.
func pipeSpans(line string, from, to int) [][2]int {
	var out [][2]int
	b := from
	for i := from; i < to; i++ {
		if line[i] == '|' && (i == from || line[i-1] != '\\') {
			out = append(out, [2]int{b, i})
			b = i + 1
		}
	}
	return append(out, [2]int{b, to})
}

// mdRowSpans returns the byte ranges in line of the cells of a table row as markdown-it 14.3.2 splits it (escapedSplit):
// the line trimmed as JavaScript trims, split at every pipe that does not follow a backslash, an empty first or last
// part dropped. A row of a pipe alone has no cell.
func mdRowSpans(line string) [][2]int {
	from := len(line) - len(strings.TrimLeftFunc(line, jsTrimmed))
	to := max(len(strings.TrimRightFunc(line, jsTrimmed)), from)
	out := pipeSpans(line, from, to)
	if out[0][0] == out[0][1] {
		out = out[1:]
	}
	if n := len(out); n > 0 && out[n-1][0] == out[n-1][1] {
		out = out[:n-1]
	}
	return out
}

// tableRowSpans returns the byte ranges in line of the cells of a table row (tableRowCells), each with the white space
// around it.
func tableRowSpans(line string) [][2]int {
	from := len(line) - len(strings.TrimLeft(line, " \t"))
	to := max(len(strings.TrimRight(line, " \t")), from)
	out := pipeSpans(line, from, to)
	if to > from && line[from] == '|' {
		out = out[1:]
	}
	if to-from > 1 && line[to-1] == '|' && line[to-2] != '\\' {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		out = [][2]int{{from, to}}
	}
	return out
}
