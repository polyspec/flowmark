package store

import (
	"strings"

	"github.com/polyspec/flowmark/internal/htmlentity"
)

// htmlAttr is an attribute of a raw HTML open tag: its name in lower case and its
// value, unquoted, with the character references decoded.
type htmlAttr struct {
	name, value string
}

// inlineResult is the inline text of a heading or a paragraph as the folds read it:
// text, where an image keeps no text, and alt, where an image keeps its alt text; and
// the attributes of each raw HTML open tag of the source, in order.
type inlineResult struct {
	text, alt string
	tags      [][]htmlAttr
	tagAt     []int // the offset in the source of the '<' of each tag
}

// inlineText returns the inline text of s, as the folds read it. The reader follows the
// precedence of CommonMark 0.31.2 where it decides what is text: a code span, an
// autolink and a raw HTML tag bind tighter than a link bracket, and a link may not
// contain another link, so the brackets around one turn inactive. A link keeps its text
// and loses its target and title. An image keeps no text in text, since the text of an
// img element is empty, and its alt text in alt. Markup that is not a letter or a digit,
// such as emphasis markers and escapes, is kept as characters, which the folds drop. A
// bracket pair that is followed by no inline target is a link when refs defines its
// label: a full, a collapsed or a shortcut reference, which keeps its text. Otherwise
// the brackets are text, and the text stays.
func inlineText(s string, refs linkRefs) inlineResult {
	r := inlineReader{s: s, refs: refs}
	r.run()
	return inlineResult{text: string(r.plain), alt: string(r.out), tags: r.tags, tagAt: r.tagAt}
}

// inlineReader holds the text kept so far, with and without the alt text of images, the
// tags read and their offsets, the open brackets, and the link reference definitions that
// the fold reads.
type inlineReader struct {
	s     string
	refs  linkRefs
	out   []byte // the kept text with the alt text of each image
	plain []byte // the kept text with no text of an image
	tags  [][]htmlAttr
	tagAt []int
	open  []bracket
}

// keep keeps s as text, in both readings.
func (r *inlineReader) keep(s string) {
	r.out = append(r.out, s...)
	r.plain = append(r.plain, s...)
}

// bracket is an open link or image bracket: the length of the kept text in each reading
// when it was read, the index of its '[' in the source, whether it opens an image, and
// whether it may still open a link.
type bracket struct {
	pos, plainPos int
	src           int
	image         bool
	active        bool
}

// run reads the whole of r.s, left to right.
func (r *inlineReader) run() {
	s := r.s
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]):
			i += 2
		case c == '`':
			i = r.codeSpan(i)
		case c == '<':
			i = r.angle(i)
		case c == '&':
			i = r.reference(i)
		case c == '!' && i+1 < len(s) && s[i+1] == '[':
			r.open = append(r.open, bracket{pos: len(r.out), plainPos: len(r.plain), src: i + 1, image: true, active: true})
			i += 2
		case c == '[':
			r.open = append(r.open, bracket{pos: len(r.out), plainPos: len(r.plain), src: i, active: true})
			i++
		case c == ']':
			i = r.closeBracket(i)
		default:
			r.keep(s[i : i+1])
			i++
		}
	}
}

// codeSpan reads the code span that opens at i: a run of backticks, closed by the
// next run of the same length, whose content is kept. An opening run with no closing
// run is text.
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
			r.keep(s[i+n : k])
			return k + m
		}
		k += m
	}
	r.keep(s[i : i+n])
	return i + n
}

// angle reads what begins with '<' at i: an autolink, whose text is kept, or a raw
// HTML construct, which is not text and whose open tag is recorded. Any other '<' is
// text.
func (r *inlineReader) angle(i int) int {
	s := r.s[i:]
	if n, text := autolinkAt(s); n > 0 {
		r.keep(text)
		return i + n
	}
	if n, attrs, open := htmlMarkupAt(s); n > 0 {
		if open {
			r.tags = append(r.tags, attrs)
			r.tagAt = append(r.tagAt, i)
		}
		return i + n
	}
	r.keep("<")
	return i + 1
}

// reference reads the character reference at i, whose decoded text is kept. An
// ampersand that begins no reference is text.
func (r *inlineReader) reference(i int) int {
	if text, n := htmlentity.Reference(r.s[i:]); n > 0 {
		r.keep(text)
		return i + n
	}
	r.keep("&")
	return i + 1
}

// closeBracket reads the ']' at i. The nearest open bracket forms a link or an image
// when an inline target or a reference that refs defines follows it; a link deactivates
// the brackets of links before it, and an image drops the text it encloses. Otherwise
// the bracket is text, its label kept.
func (r *inlineReader) closeBracket(i int) int {
	n := len(r.open)
	if n == 0 {
		return i + 1
	}
	o := r.open[n-1]
	r.open = r.open[:n-1]
	if !o.active {
		return i + 1
	}
	end, ok := inlineTail(r.s, i+1)
	if !ok {
		end, ok = r.refTail(o, i)
	}
	if !ok {
		return i + 1
	}
	if o.image {
		// The alt text stays in alt; the text of the image leaves the plain reading.
		r.plain = r.plain[:o.plainPos]
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
// Under F_M one white space may stand before the label. It returns the index after the
// reference, and false when none is defined.
func (r *inlineReader) refTail(o bracket, i int) (int, bool) {
	s := r.s
	text := s[o.src+1 : i]
	p := i + 1
	if r.refs.space && p+1 < len(s) && (s[p] == ' ' || s[p] == '\t' || s[p] == '\n') && s[p+1] == '[' {
		p++
	}
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
