package store

import (
	"strings"
	"unicode/utf8"
)

// The block structure that the sections of a row depend on, as CommonMark 0.31.2
// reads it at the top level: ATX and setext headings, fenced code blocks,
// paragraphs, the HTML blocks of types 1 to 7, and containers. A line of four or
// more spaces of indent is code, never a heading or a fence.

// heading is an ATX or a setext heading: the first line and the last line of it
// (the underline of a setext heading), its level, and its text.
type heading struct {
	at, last, level int
	text            string
	tab             bool // the separator after the hashes is a tab, a noncanonical spelling
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
}

// leafKind is the leaf block that is open in the innermost container.
type leafKind int

const (
	leafNone leafKind = iota
	leafPara
	leafFence
	leafHTML
)

// scanner reads the block structure of lines one at a time, as CommonMark's
// container matching reads it: the open containers, outermost first, and the
// leaf block open in the innermost one. Only a heading outside every container
// is reported; a heading inside a container, a fence or an HTML block is content.
type scanner struct {
	stack []block
	leaf  leafKind
	fence fenceMark // the open fence, when leaf is leafFence
	html  int       // the kind of the open HTML block, when leaf is leafHTML
	para  int       // the first line of the open paragraph, when leaf is leafPara
}

// scan is the block structure of a run of lines from some index on.
type scan struct {
	heads []heading
	// open is whether a fence or an HTML block is open at the top level at the end:
	// such a block takes in everything after it, so a value that leaves one open
	// is followed by no element of its own.
	open bool
}

// scanHeadings reads the block structure of lines from index from on. A setext
// heading is the whole paragraph before its underline; a fence or an HTML block
// hides the lines it holds, headings included.
func scanHeadings(lines []string, from int) scan {
	var sc scan
	var s scanner
	for i := from; i < len(lines); i++ {
		if h, ok := s.feed(lines, i); ok {
			sc.heads = append(sc.heads, h)
		}
	}
	sc.open = len(s.stack) == 0 && (s.leaf == leafFence || s.leaf == leafHTML)
	return sc
}

// feed reads line i of lines and returns the heading that it completes when the
// heading is outside every container.
func (s *scanner) feed(lines []string, i int) (heading, bool) {
	rest, col := expandLead(lines[i], 0), 0
	m := 0
	for ; m < len(s.stack); m++ {
		r, c, ok := s.stack[m].match(rest, col)
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
	if m < len(s.stack) {
		// Only a paragraph continues lazily into a container that did not match.
		if s.leaf == leafPara && lazyText(rest) {
			return heading{}, false
		}
		s.stack = s.stack[:m]
		s.leaf = leafNone
	}
	switch s.leaf {
	case leafFence:
		if closesFence(rest, s.fence) {
			s.leaf = leafNone
		}
		return heading{}, false
	case leafHTML:
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
			return heading{}, false
		}
		if s.leaf == leafPara {
			if level := setextLevel(rest); level > 0 {
				s.leaf = leafNone
				if top {
					return heading{at: s.para, last: i, level: level, text: setextText(lines[s.para:i])}, true
				}
				return heading{}, false
			}
		}
		if level, text, tab, ok := headingOf(rest); ok {
			s.leaf = leafNone
			if top {
				return heading{at: i, last: i, level: level, text: text, tab: tab}, true
			}
			return heading{}, false
		}
		if f, ok := fenceOpenAt(rest); ok {
			s.leaf, s.fence = leafFence, f
			return heading{}, false
		}
		if k := htmlStart(rest); k != 0 && (k != 7 || s.leaf != leafPara) {
			s.leaf = leafNone
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
		if it, ok := listStart(rest, col); ok && (s.leaf != leafPara || interruptsPara(it)) {
			s.leaf = leafNone
			s.stack = append(s.stack, block{indent: it.indent, empty: it.empty})
			rest, col = it.rest, it.col
			top = false
			continue
		}
		if s.leaf == leafNone {
			s.leaf, s.para = leafPara, i
		}
		return heading{}, false
	}
}

// match reports whether the container continues on a line whose text is s, at
// column col, and returns the text that the container passes on. A block quote
// continues on a line that opens with its marker; a list item continues on a line
// indented at least as far as its content, and on a blank line unless it is empty.
func (b block) match(s string, col int) (string, int, bool) {
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
func lazyText(rest string) bool {
	return !isBlank(rest) && !opensInterrupt(rest)
}

// opensInterrupt reports whether line opens a block that interrupts a paragraph:
// an ATX heading, a fence, an HTML block of types 1 to 6, a thematic break, a
// block quote, or a list item. An HTML block of type 7 does not.
func opensInterrupt(line string) bool {
	if indentOf(line) >= 4 {
		return false
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

// setextText is the text of a setext heading: its lines without their indent and
// their trailing white space, joined by line feeds.
func setextText(lines []string) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = strings.TrimRight(strings.TrimLeft(l, " "), " \t")
	}
	return strings.Join(parts, "\n")
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

// htmlStart reads the start of an HTML block of CommonMark 0.31.2 and returns its
// type, 1 to 7, or 0 when line starts none. A type 6 start is the name of a block
// element; a type 7 start is one complete tag and nothing but white space after it.
func htmlStart(line string) int {
	s := indentOf(line)
	if s > 3 || s >= len(line) || line[s] != '<' {
		return 0
	}
	rest := line[s:]
	lower := asciiLower(rest)
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
	if !blockTagNames[asciiLower(s[p:q])] {
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
// or -1 when s starts with none. An open tag is a name, then attributes that each
// begin with white space, then white space, an optional '/' and '>'.
func htmlTagEnd(s string) int {
	if strings.HasPrefix(s, "</") {
		p := tagNameEnd(s, 2)
		if p < 0 {
			return -1
		}
		if p = skipSpace(s, p); p < len(s) && s[p] == '>' {
			return p + 1
		}
		return -1
	}
	if !strings.HasPrefix(s, "<") {
		return -1
	}
	p := tagNameEnd(s, 1)
	if p < 0 {
		return -1
	}
	for {
		q := skipSpace(s, p)
		if q == p {
			break
		}
		n := attrNameEnd(s, q)
		if n < 0 {
			break
		}
		p = n
		if e := attrSpecEnd(s, p); e >= 0 {
			p = e
		} else if q := skipSpace(s, p); q < len(s) && s[q] == '=' {
			// A '=' that no value follows stays in the tag and ends it: no tag matches.
			return -1
		}
	}
	q := skipSpace(s, p)
	if q < len(s) && s[q] == '/' {
		q++
	}
	if q < len(s) && s[q] == '>' {
		return q + 1
	}
	return -1
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
	if p >= len(s) || !(isASCIILetter(s[p]) || s[p] == '_' || s[p] == ':') {
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

// asciiLower lowers the ASCII letters of s, keeping every other byte, so that its
// length is s's length.
func asciiLower(s string) string {
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
		lower := asciiLower(line)
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
