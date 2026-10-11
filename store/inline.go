package store

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/polyspec/flowmark/internal/htmlentity"
)

// htmlAttr is an attribute of a raw HTML open tag: its name in lower case and its
// value, unquoted, with the character references decoded.
type htmlAttr struct {
	name, value string
}

// reading is the rendered text of a heading in one reading, split at its line breaks:
// S_GH and S_VS join its lines with nothing, S_GL with a space (spec 10, Detail headings
// and anchors). A line feed that a character reference writes is text, not a break.
type reading []string

// inlineResult is the rendered text of a heading or a paragraph: text, where an image
// renders no text, and alt, where an image renders its alt text; and the attributes of
// each raw HTML open tag of the source, in order.
type inlineResult struct {
	text, alt reading
	tags      [][]htmlAttr
	tagAt     []int // the offset in the source of the '<' of each tag
}

// inlineText returns the text that CommonMark 0.31.2 renders from the inline content
// s, the text of a heading's text nodes and code spans. A backslash escape renders the
// escaped character, and a backslash or two or more spaces before a line ending a hard
// line break; a line ending is a soft line break, without the spaces before it. A code
// span renders its content normalized (§6.1): each line ending a space, and one space
// stripped at each end when both ends are spaces and the content is not all spaces. A
// character reference renders its character, a numeric one its code point (§2.5). An
// autolink renders its text, a raw HTML construct nothing; an open tag is recorded. A
// link renders its text without its target and title; a bracket pair followed by no
// inline target is a link when refs defines its label, a full, a collapsed or a
// shortcut reference. An image renders no text in text, since the text of an img
// element is empty, and in alt the plain-text rendering of its description. Emphasis
// and strong delimiters render nothing where the delimiter algorithm of §6.2 and the
// appendix's process emphasis matches them, and are text elsewhere, as is every
// bracket and delimiter that forms nothing.
func inlineText(s string, refs linkRefs) inlineResult {
	r := inlineReader{s: s, refs: refs}
	r.run()
	text, alt := r.readings()
	return inlineResult{text: text, alt: alt, tags: r.tags, tagAt: r.tagAt}
}

// inlineReader holds the rendered pieces read so far, the stacks of emphasis
// delimiters and of open brackets, the tags read and their offsets, and the link
// reference definitions of the file.
type inlineReader struct {
	s      string
	refs   linkRefs
	nodes  []inlineNode
	delims []delimiter
	open   []bracket
	tags   [][]htmlAttr
	tagAt  []int
}

// inlineNode is a piece of the rendered text: a text, or a line break; image marks a
// piece of an image's description, which the text reading drops.
type inlineNode struct {
	text  string
	brk   bool
	image bool
}

// delimiter is a run of '*' or '_' that can open or close emphasis: the node that holds
// its remaining characters, its character, the length of its run as read, whether it
// can open and close, and whether it is still on the stack.
type delimiter struct {
	node              int
	char              byte
	length            int
	canOpen, canClose bool
	live              bool
}

// bracket is an open link or image bracket: the node of its "[" or "![", the index of
// its '[' in the source, whether it opens an image, whether it may still open a link,
// and the length of the delimiter stack when it was read, below which a link's
// emphasis is not processed.
type bracket struct {
	node   int
	src    int
	image  bool
	active bool
	bottom int
}

// text appends s to the rendered text.
func (r *inlineReader) text(s string) {
	r.nodes = append(r.nodes, inlineNode{text: s})
}

// run reads the whole of r.s, left to right, then processes the emphasis of the
// delimiters left.
func (r *inlineReader) run() {
	s := r.s
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && s[i+1] == '\n':
			r.nodes = append(r.nodes, inlineNode{brk: true})
			i += 2
		case c == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]):
			r.text(s[i+1 : i+2])
			i += 2
		case c == '\n':
			r.nodes = append(r.nodes, inlineNode{brk: true})
			i++
		case c == ' ':
			j := i + runOf(s[i:], ' ')
			if j == len(s) || s[j] != '\n' {
				r.text(s[i:j])
			}
			i = j
		case c == '`':
			i = r.codeSpan(i)
		case c == '<':
			i = r.angle(i)
		case c == '&':
			i = r.reference(i)
		case c == '*' || c == '_':
			i = r.delimiterRun(i)
		case c == '!' && i+1 < len(s) && s[i+1] == '[':
			r.open = append(r.open, bracket{node: len(r.nodes), src: i + 1, image: true, active: true, bottom: len(r.delims)})
			r.text("![")
			i += 2
		case c == '[':
			r.open = append(r.open, bracket{node: len(r.nodes), src: i, active: true, bottom: len(r.delims)})
			r.text("[")
			i++
		case c == ']':
			i = r.closeBracket(i)
		default:
			r.text(s[i : i+1])
			i++
		}
	}
	r.processEmphasis(0)
}

// readings returns the rendered text in its two readings: text without the
// descriptions of images, and alt with them.
func (r *inlineReader) readings() (reading, reading) {
	var text, alt []strings.Builder
	text, alt = make([]strings.Builder, 1), make([]strings.Builder, 1)
	for _, n := range r.nodes {
		if n.brk {
			alt = append(alt, strings.Builder{})
			if !n.image {
				text = append(text, strings.Builder{})
			}
			continue
		}
		alt[len(alt)-1].WriteString(n.text)
		if !n.image {
			text[len(text)-1].WriteString(n.text)
		}
	}
	return builtReading(text), builtReading(alt)
}

// builtReading returns the lines that the builders hold.
func builtReading(b []strings.Builder) reading {
	out := make(reading, len(b))
	for i := range b {
		out[i] = b[i].String()
	}
	return out
}

// codeSpan reads the code span that opens at i: a run of backticks, closed by the next
// run of the same length, whose content renders normalized as §6.1 states. An opening
// run with no closing run is text.
func (r *inlineReader) codeSpan(i int) int {
	s := r.s
	n := runOf(s[i:], '`')
	for k := i + n; k < len(s); {
		if s[k] != '`' {
			k++
			continue
		}
		m := runOf(s[k:], '`')
		if m == n {
			r.text(codeContent(s[i+n : k]))
			return k + m
		}
		k += m
	}
	r.text(s[i : i+n])
	return i + n
}

// codeContent returns the rendered content of a code span: each line ending a space,
// then one space stripped from each end when both ends are spaces and the content is
// not all spaces.
func codeContent(c string) string {
	c = strings.ReplaceAll(c, "\n", " ")
	if len(c) >= 2 && c[0] == ' ' && c[len(c)-1] == ' ' && strings.Trim(c, " ") != "" {
		return c[1 : len(c)-1]
	}
	return c
}

// angle reads what begins with '<' at i: an autolink, whose text renders, or a raw
// HTML construct, which renders no text and whose open tag is recorded. Any other '<'
// is text.
func (r *inlineReader) angle(i int) int {
	s := r.s[i:]
	if n, text := autolinkAt(s); n > 0 {
		r.text(text)
		return i + n
	}
	if n, attrs, open := htmlMarkupAt(s); n > 0 {
		if open {
			r.tags = append(r.tags, attrs)
			r.tagAt = append(r.tagAt, i)
		}
		return i + n
	}
	r.text("<")
	return i + 1
}

// reference reads the character reference at i, which renders its decoded text. An
// ampersand that begins no reference is text.
func (r *inlineReader) reference(i int) int {
	if text, n := htmlentity.Reference(r.s[i:]); n > 0 {
		r.text(text)
		return i + n
	}
	r.text("&")
	return i + 1
}

// delimiterRun reads the run of '*' or '_' at i as text, and pushes it on the delimiter
// stack when it can open or close emphasis by the flanking rules of §6.2: the start and
// the end of the text count as white space.
func (r *inlineReader) delimiterRun(i int) int {
	s := r.s
	c := s[i]
	n := runOf(s[i:], c)
	before, after := rune(-1), rune(-1)
	if i > 0 {
		before, _ = utf8.DecodeLastRuneInString(s[:i])
	}
	if i+n < len(s) {
		after, _ = utf8.DecodeRuneInString(s[i+n:])
	}
	left := !cmSpace(after) && (!cmPunct(after) || cmSpace(before) || cmPunct(before))
	right := !cmSpace(before) && (!cmPunct(before) || cmSpace(after) || cmPunct(after))
	open, closes := left, right
	if c == '_' {
		open = left && (!right || cmPunct(before))
		closes = right && (!left || cmPunct(after))
	}
	if open || closes {
		r.delims = append(r.delims, delimiter{node: len(r.nodes), char: c, length: n, canOpen: open, canClose: closes, live: true})
	}
	r.text(s[i : i+n])
	return i + n
}

// cmSpace reports whether r is Unicode white space as CommonMark 0.31.2 defines it, a
// character of category Zs, a tab, a line feed, a form feed or a carriage return; -1,
// the start or the end of the text, counts as white space.
func cmSpace(r rune) bool {
	switch r {
	case -1, '\t', '\n', '\f', '\r':
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// cmPunct reports whether r is Unicode punctuation as CommonMark 0.31.2 defines it, a
// character of a category P or S.
func cmPunct(r rune) bool {
	return r >= 0 && unicode.In(r, unicode.P, unicode.S)
}

// processEmphasis matches the openers and closers of the delimiter stack above bottom,
// as the delimiter algorithm of §6.2 and the appendix's process emphasis state: each
// closer, left to right, takes the nearest opener of its character that the rule of
// three allows, above the lowest one already known to have none; a
// match removes one delimiter character from each, two when both runs have two, and
// takes the delimiters between them off the stack. The delimiters above bottom then
// leave the stack, and their remaining characters are text.
func (r *inlineReader) processEmphasis(bottom int) {
	type key struct {
		char    byte
		mod     int
		canOpen bool
	}
	openersBottom := map[key]int{}
	for cur := bottom; cur < len(r.delims); {
		d := &r.delims[cur]
		if !d.live || !d.canClose {
			cur++
			continue
		}
		k := key{d.char, d.length % 3, d.canOpen}
		lower := bottom - 1
		if v, ok := openersBottom[k]; ok && v > lower {
			lower = v
		}
		found := -1
		for o := cur - 1; o > lower; o-- {
			od := &r.delims[o]
			if !od.live || od.char != d.char || !od.canOpen {
				continue
			}
			if (od.canClose || d.canOpen) && (od.length+d.length)%3 == 0 && (od.length%3 != 0 || d.length%3 != 0) {
				continue
			}
			found = o
			break
		}
		if found < 0 {
			openersBottom[k] = cur - 1
			if !d.canOpen {
				d.live = false
			}
			cur++
			continue
		}
		od := &r.delims[found]
		opener, closer := &r.nodes[od.node].text, &r.nodes[d.node].text
		use := 1
		if len(*opener) >= 2 && len(*closer) >= 2 {
			use = 2
		}
		*opener = (*opener)[:len(*opener)-use]
		*closer = (*closer)[use:]
		for m := found + 1; m < cur; m++ {
			r.delims[m].live = false
		}
		if *opener == "" {
			od.live = false
		}
		if *closer == "" {
			d.live = false
			cur++
		}
	}
	for m := bottom; m < len(r.delims); m++ {
		r.delims[m].live = false
	}
}

// closeBracket reads the ']' at i. The nearest open bracket forms a link or an image
// when an inline target or a reference that refs defines follows it: the emphasis of
// its text is processed, its brackets render nothing, a link deactivates the brackets
// of links before it, and the description of an image leaves the text reading.
// Otherwise the ']' is text, and an inactive bracket leaves the stack as text.
func (r *inlineReader) closeBracket(i int) int {
	n := len(r.open)
	if n == 0 {
		r.text("]")
		return i + 1
	}
	o := r.open[n-1]
	r.open = r.open[:n-1]
	if !o.active {
		r.text("]")
		return i + 1
	}
	end, ok := inlineTail(r.s, i+1)
	if !ok {
		end, ok = r.refTail(o, i)
	}
	if !ok {
		r.text("]")
		return i + 1
	}
	r.processEmphasis(o.bottom)
	r.nodes[o.node].text = ""
	if o.image {
		for k := o.node + 1; k < len(r.nodes); k++ {
			r.nodes[k].image = true
		}
	} else {
		for k := range r.open {
			if !r.open[k].image {
				r.open[k].active = false
			}
		}
	}
	return end
}

// refTail reads the reference that follows the ']' at i, which closes the bracket o:
// a full reference with a label, a collapsed one with an empty label, which reads the
// link text as its label, or a shortcut that is no label at all. A full or collapsed
// reference is read when its label is defined; a shortcut is read when the link text is.
// It returns the index after the reference, and false when none is defined.
func (r *inlineReader) refTail(o bracket, i int) (int, bool) {
	s := r.s
	text := s[o.src+1 : i]
	p := i + 1
	if p < len(s) && s[p] == '[' {
		if q := strings.IndexByte(s[p+1:], ']'); q >= 0 {
			label := s[p+1 : p+1+q]
			if label == "" {
				label = text
			}
			if r.refs.defined(label) {
				return p + q + 2, true
			}
		}
	}
	if r.refs.defined(text) {
		return i + 1, true
	}
	return 0, false
}

// autolinkAt reads the URI or the email autolink that begins s, which starts with
// '<', and returns its length and the text between its brackets; its length is 0
// when s begins with none.
func autolinkAt(s string) (int, string) {
	gt := strings.IndexByte(s, '>')
	if gt < 2 {
		return 0, ""
	}
	body := s[1:gt]
	if uriAutolink(body) || emailAutolink(body) {
		return gt + 1, body
	}
	return 0, ""
}

// uriAutolink reports whether body is an absolute URI: a scheme of two to thirty-two
// letters, digits, '+', '.' or '-' that begins with a letter, a colon, and no '<' or
// white space or control character after it.
func uriAutolink(body string) bool {
	colon := strings.IndexByte(body, ':')
	if colon < 2 || colon > 32 || !isASCIILetter(body[0]) {
		return false
	}
	for i := 1; i < colon; i++ {
		if c := body[i]; !isASCIIAlnum(c) && c != '+' && c != '.' && c != '-' {
			return false
		}
	}
	for i := 0; i < len(body); i++ {
		if body[i] <= 0x20 || body[i] == '<' {
			return false
		}
	}
	return true
}

// emailAutolink reports whether body is an email address: a local part of letters,
// digits and the characters of CommonMark's email pattern, an '@', and a domain of
// labels of up to sixty-three letters, digits and hyphens, none of them starting or
// ending with a hyphen.
func emailAutolink(body string) bool {
	at := strings.IndexByte(body, '@')
	if at < 1 {
		return false
	}
	for i := 0; i < at; i++ {
		if c := body[i]; !isASCIIAlnum(c) && strings.IndexByte(".!#$%&'*+/=?^_`{|}~-", c) < 0 {
			return false
		}
	}
	for _, label := range strings.Split(body[at+1:], ".") {
		if len(label) < 1 || len(label) > 63 || !isASCIIAlnum(label[0]) || !isASCIIAlnum(label[len(label)-1]) {
			return false
		}
		for i := 1; i < len(label)-1; i++ {
			if c := label[i]; !isASCIIAlnum(c) && c != '-' {
				return false
			}
		}
	}
	return true
}

// htmlMarkupAt reads the raw HTML construct that begins s, which starts with '<': a
// comment, a processing instruction, a CDATA section, a declaration, or an open or
// closing tag. It returns the length of the construct, the attributes of an open
// tag, and whether it is an open tag; the length is 0 when s begins with none.
func htmlMarkupAt(s string) (int, []htmlAttr, bool) {
	switch {
	case strings.HasPrefix(s, "<!--"):
		if strings.HasPrefix(s, "<!-->") {
			return 5, nil, false
		}
		if strings.HasPrefix(s, "<!--->") {
			return 6, nil, false
		}
		if e := strings.Index(s[4:], "-->"); e >= 0 {
			return 4 + e + 3, nil, false
		}
		return 0, nil, false
	case strings.HasPrefix(s, "<?"):
		if e := strings.Index(s[2:], "?>"); e >= 0 {
			return 2 + e + 2, nil, false
		}
		return 0, nil, false
	case strings.HasPrefix(s, "<![CDATA["):
		if e := strings.Index(s[9:], "]]>"); e >= 0 {
			return 9 + e + 3, nil, false
		}
		return 0, nil, false
	case strings.HasPrefix(s, "<!") && len(s) > 2 && isASCIILetter(s[2]):
		p := 2
		for p < len(s) && isASCIILetter(s[p]) {
			p++
		}
		if q := skipSpace(s, p); q > p {
			if e := strings.IndexByte(s[q:], '>'); e >= 0 {
				return q + e + 1, nil, false
			}
		}
		return 0, nil, false
	}
	end, attrs := htmlTagAt(s)
	if end <= 0 {
		return 0, nil, false
	}
	return end, attrs, s[1] != '/'
}

// htmlTagAt returns the index just after the open or closing tag that starts s, or
// -1 when s starts with none, and the attributes of an open tag. An open tag is a
// name, then attributes that each begin with white space, then white space, an
// optional '/' and '>'.
func htmlTagAt(s string) (int, []htmlAttr) {
	if strings.HasPrefix(s, "</") {
		p := tagNameEnd(s, 2)
		if p < 0 {
			return -1, nil
		}
		if p = skipSpace(s, p); p < len(s) && s[p] == '>' {
			return p + 1, nil
		}
		return -1, nil
	}
	if !strings.HasPrefix(s, "<") {
		return -1, nil
	}
	p := tagNameEnd(s, 1)
	if p < 0 {
		return -1, nil
	}
	var attrs []htmlAttr
	for {
		q := skipSpace(s, p)
		if q == p {
			break
		}
		n := attrNameEnd(s, q)
		if n < 0 {
			break
		}
		attr := htmlAttr{name: asciiLower(s[q:n])}
		p = n
		if e := attrSpecEnd(s, p); e >= 0 {
			eq := skipSpace(s, p)
			attr.value = attrText(s[skipSpace(s, eq+1):e])
			p = e
		} else if q := skipSpace(s, p); q < len(s) && s[q] == '=' {
			// A '=' that no value follows stays in the tag and ends it: no tag matches.
			return -1, nil
		}
		attrs = append(attrs, attr)
	}
	q := skipSpace(s, p)
	if q < len(s) && s[q] == '/' {
		q++
	}
	if q < len(s) && s[q] == '>' {
		return q + 1, attrs
	}
	return -1, nil
}

// attrText returns the value of an attribute as written: unquoted when it is quoted,
// with its character references decoded.
func attrText(v string) string {
	if n := len(v); n >= 2 && (v[0] == '"' || v[0] == '\'') && v[n-1] == v[0] {
		v = v[1 : n-1]
	}
	return htmlentity.Expand(v)
}

// inlineTail reads the inline target that follows a closing bracket at j: '(', an
// optional white space and destination, an optional title that white space precedes,
// an optional white space and ')'. It returns the index after the ')'.
func inlineTail(s string, j int) (int, bool) {
	if j >= len(s) || s[j] != '(' {
		return 0, false
	}
	p, ok := linkDest(s, skipTailSpace(s, j+1))
	if !ok {
		return 0, false
	}
	q := skipTailSpace(s, p)
	if q > p && q < len(s) && (s[q] == '"' || s[q] == '\'' || s[q] == '(') {
		e, ok := linkTitle(s, q)
		if !ok {
			return 0, false
		}
		q = skipTailSpace(s, e)
	}
	if q < len(s) && s[q] == ')' {
		return q + 1, true
	}
	return 0, false
}

// linkDest reads the destination of an inline target at p: either '<', the text up
// to the next '>' with no line ending or '<' in it, or a run without white space or
// control characters whose parentheses balance.
func linkDest(s string, p int) (int, bool) {
	if p < len(s) && s[p] == '<' {
		for k := p + 1; k < len(s); k++ {
			switch s[k] {
			case '\\':
				k++
			case '>':
				return k + 1, true
			case '<', '\n':
				return 0, false
			}
		}
		return 0, false
	}
	depth := 0
	k := p
	for ; k < len(s); k++ {
		c := s[k]
		if c == '\\' && k+1 < len(s) && isASCIIPunct(s[k+1]) {
			k++
			continue
		}
		if c <= 0x20 || c == 0x7f {
			break
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			if depth == 0 {
				break
			}
			depth--
		}
	}
	if depth != 0 {
		return 0, false
	}
	return k, true
}

// linkTitle reads the title that begins at q, in double quotes, single quotes or
// parentheses, and returns the index after it. A title holds no blank line, and a
// parenthesized title holds no '('.
func linkTitle(s string, q int) (int, bool) {
	closer := s[q]
	if closer == '(' {
		closer = ')'
	}
	for k := q + 1; k < len(s); k++ {
		c := s[k]
		switch {
		case c == '\\' && k+1 < len(s) && isASCIIPunct(s[k+1]):
			k++
		case c == closer:
			return k + 1, true
		case c == '(' && s[q] == '(':
			return 0, false
		case c == '\n' && blankLineAt(s, k+1):
			return 0, false
		}
	}
	return 0, false
}

// blankLineAt reports whether the line that begins at p holds only spaces and tabs
// and ends in a line feed.
func blankLineAt(s string, p int) bool {
	for p < len(s) && (s[p] == ' ' || s[p] == '\t') {
		p++
	}
	return p < len(s) && s[p] == '\n'
}

// skipTailSpace returns the index of the first character at or after p that is
// neither a space nor a tab, passing at most one line feed.
func skipTailSpace(s string, p int) int {
	lf := false
	for p < len(s) {
		switch {
		case s[p] == ' ' || s[p] == '\t':
		case s[p] == '\n' && !lf:
			lf = true
		default:
			return p
		}
		p++
	}
	return p
}

// isASCIIPunct reports whether c is an ASCII punctuation character.
func isASCIIPunct(c byte) bool { return c >= 0x21 && c <= 0x7e && !isASCIIAlnum(c) }

// isASCIIAlnum reports whether c is an ASCII letter or digit.
func isASCIIAlnum(c byte) bool {
	return isASCIILetter(c) || ('0' <= c && c <= '9')
}
