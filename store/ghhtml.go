package store

import (
	"sort"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/internal/htmlentity"
)

// GitHub's reading of HTML headings (spec 10, Detail headings and anchors): GitHub
// renders the markdown with GFM's tagfilter, parses the HTML with a parser whose
// behaviour the slug vectors record (testdata/slug/github-headings.json: closers,
// nested, htmlEntities, rawHeadings), and gives every heading element h1 to h6 that no
// other heading holds an anchor from its text, a raw HTML heading too. The text of a
// heading element ends where the parser ends the element, which a later tag can do; a
// ghElement reads it.

// ghFiltered are the tag names of GFM's tagfilter, whose tags GitHub renders as text.
var ghFiltered = setOf("title textarea style xmp iframe noembed noframes script plaintext")

// ghHTML4 are the named character references that GitHub's parser decodes in raw HTML:
// those of HTML 4.01 and apos (the vector htmlEntities); any other name is text.
var ghHTML4 = setOf(`Aacute aacute Acirc acirc acute AElig aelig Agrave agrave alefsym Alpha alpha amp
	and ang apos Aring aring asymp Atilde atilde Auml auml bdquo Beta beta brvbar bull cap
	Ccedil ccedil cedil cent Chi chi circ clubs cong copy crarr cup curren dagger Dagger
	darr dArr deg Delta delta diams divide Eacute eacute Ecirc ecirc Egrave egrave empty
	emsp ensp Epsilon epsilon equiv Eta eta ETH eth Euml euml euro exist fnof forall
	frac12 frac14 frac34 frasl Gamma gamma ge gt harr hArr hearts hellip Iacute iacute
	Icirc icirc iexcl Igrave igrave image infin int Iota iota iquest isin Iuml iuml Kappa
	kappa Lambda lambda lang laquo larr lArr lceil ldquo le lfloor lowast loz lrm lsaquo
	lsquo lt macr mdash micro middot minus Mu mu nabla nbsp ndash ne ni not notin nsub
	Ntilde ntilde Nu nu Oacute oacute Ocirc ocirc OElig oelig Ograve ograve oline Omega
	omega Omicron omicron oplus or ordf ordm Oslash oslash Otilde otilde otimes Ouml ouml
	para part permil perp Phi phi Pi pi piv plusmn pound prime Prime prod prop Psi psi
	quot radic rang raquo rarr rArr rceil rdquo real reg rfloor Rho rho rlm rsaquo rsquo
	sbquo Scaron scaron sdot sect shy Sigma sigma sigmaf sim spades sub sube sum sup1 sup2
	sup3 sup supe szlig Tau tau there4 Theta theta thetasym thinsp THORN thorn tilde times
	trade Uacute uacute uarr uArr Ucirc ucirc Ugrave ugrave uml upsih Upsilon upsilon Uuml
	uuml weierp Xi xi Yacute yacute yen yuml Yuml Zeta zeta zwj zwnj`)

// setOf returns the set of the words of s.
func setOf(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// ghIsindexText is the text that GitHub's parser writes for an isindex start tag (the
// vector closers).
const ghIsindexText = "This is a searchable index. Enter search keywords: "

// tagOf returns the lowercased name of the start or end tag t, the whole text of an
// HTML construct, whether it is an end tag, and whether it is a start tag closed by
// "/>"; ok is false for a comment, a processing instruction, a declaration or a CDATA
// section.
func tagOf(t string) (name string, end, self, ok bool) {
	p := 1
	if strings.HasPrefix(t, "</") {
		p, end = 2, true
	}
	q := tagNameEnd(t, p)
	if q < 0 {
		return "", false, false, false
	}
	return asciiLower(t[p:q]), end, !end && strings.HasSuffix(t, "/>"), true
}

// ghRawRef reads the character reference at the start of s, raw HTML text, as GitHub's
// parser does, and returns its text and length, or the length 0 when s starts with
// none: '&#' and decimal digits, or '&#x' or '&#X' and hexadecimal digits, as many as
// follow, then an optional ';', renders its code point, and nothing when it has no
// digit or is no Unicode scalar value; '&', a name of ghHTML4 and ';' renders the
// name's character, which is HTML 4.01's: lang and rang are U+2329 and U+232A, where
// HTML5 reads U+27E8 and U+27E9. Any other '&' is text.
func ghRawRef(s string) (string, int) {
	if len(s) < 2 || s[0] != '&' {
		return "", 0
	}
	if s[1] == '#' {
		p, base := 2, 10
		if p < len(s) && (s[p] == 'x' || s[p] == 'X') {
			p, base = 3, 16
		}
		start := p
		for p < len(s) && digitOf(s[p], base) {
			p++
		}
		digits := s[start:p]
		if p < len(s) && s[p] == ';' {
			p++
		}
		v, err := strconv.ParseUint(digits, base, 32)
		if digits == "" || err != nil || v == 0 || v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF) {
			return "", p
		}
		return string(rune(v)), p
	}
	q := 1
	for q < len(s) && (isASCIILetter(s[q]) || s[q] >= '0' && s[q] <= '9') {
		q++
	}
	if q < len(s) && s[q] == ';' && ghHTML4[s[1:q]] {
		switch s[1:q] {
		case "lang":
			return "〈", q + 1
		case "rang":
			return "〉", q + 1
		}
		if text, n := htmlentity.Reference(s[:q+1]); n == q+1 {
			return text, n
		}
	}
	return "", 0
}

// digitOf reports whether c is a digit of base 10 or 16.
func digitOf(c byte, base int) bool {
	switch {
	case c >= '0' && c <= '9':
		return true
	case base == 16:
		return c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	}
	return false
}

// ghEvent is a piece of the HTML that GitHub parses: text, or a tag. at is the offset
// of the tag's '<' in the text it was read from, and spans the rendered inline
// elements (emphasis, links) that enclose it.
type ghEvent struct {
	text      string
	name      string
	end, self bool
	at        int
	spans     []int
}

// tag reports whether e is a tag.
func (e ghEvent) tag() bool { return e.name != "" }

// htmlEvents returns the HTML of the raw text s of an HTML block as GitHub parses it:
// a tag is an event, except a tag of the tagfilter, which is text; a comment, a
// processing instruction, a declaration and a CDATA section are nothing; a character
// reference is read by ghRawRef.
func htmlEvents(s string) []ghEvent {
	var out []ghEvent
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, ghEvent{text: b.String()})
			b.Reset()
		}
	}
	for i := 0; i < len(s); {
		switch s[i] {
		case '<':
			if n, _, _ := htmlMarkupAt(s[i:]); n > 0 {
				if name, end, self, ok := tagOf(s[i : i+n]); ok && !ghFiltered[name] {
					flush()
					out = append(out, ghEvent{name: name, end: end, self: self, at: i})
				} else if ok {
					b.WriteString(s[i : i+n])
				}
				i += n
				continue
			}
		case '&':
			if text, n := ghRawRef(s[i:]); n > 0 {
				b.WriteString(text)
				i += n
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	flush()
	return out
}

// inlineEvents returns the HTML of inline content s as GitHub renders and parses it:
// its rendered text, an image's description excluded, with the tags of its raw HTML
// where they stand, each with the emphasis and links that enclose it; a tag inside an
// image's description is none, since GitHub writes the description into an alt
// attribute.
func inlineEvents(s string, refs linkRefs) []ghEvent {
	r := inlineReader{s: s, refs: refs, gh: true}
	r.run()
	inside := func(ranges [][2]int, at int) bool {
		for _, rg := range ranges {
			if rg[0] < at && at < rg[1] {
				return true
			}
		}
		return false
	}
	var out []ghEvent
	m := 0
	emit := func(upto int) {
		for ; m < len(r.marks) && r.marks[m].node <= upto; m++ {
			t := r.marks[m]
			if inside(r.hidden, t.at) {
				continue
			}
			var spans []int
			for k, sp := range r.spans {
				if sp[0] < t.at && t.at < sp[1] {
					spans = append(spans, k)
				}
			}
			out = append(out, ghEvent{name: t.name, end: t.end, self: t.self, at: t.at, spans: spans})
		}
	}
	for k, n := range r.nodes {
		emit(k)
		if n.image || n.brk || n.text == "" {
			continue
		}
		if len(out) > 0 && !out[len(out)-1].tag() {
			out[len(out)-1].text += n.text
			continue
		}
		out = append(out, ghEvent{text: n.text})
	}
	emit(len(r.nodes))
	return out
}

// ghStarts are the start tags that end a heading element when no element is open
// inside it (the vector closers): fieldset, form, li, p, table and every heading.
var ghStarts = setOf("fieldset form li p table h1 h2 h3 h4 h5 h6")

// ghThrough holds, for each element name the vector nested records, the start tags of
// ghStarts that end a heading element while that element is the one open inside it.
// The names of ghNoPush are no element left open, and the tags of ghFiltered no
// element at all.
var ghThrough = func() map[string]map[string]bool {
	m := map[string]map[string]bool{}
	for through, names := range map[string]string{
		"fieldset h1 h2 h3 h4 h5 h6 table": "a",
		"": `abbr acronym applet article aside audio bdi bdo blink blockquote button canvas center
			cite code data datalist dd del details dfn dialog div dt em figcaption figure font footer
			header hgroup ins kbd label main map mark marquee meter nav nobr noscript object optgroup
			option output picture progress q rb rp rt rtc ruby samp search section select slot span
			strong sub summary sup svg math template time var video foo custom-el`,
		"form li":                "address dl",
		"p":                      "b big i s small strike tt u",
		"h1 h2 h3 h4 h5 h6":      "caption colgroup embed frameset keygen menuitem source tbody td tfoot th thead tr track wbr",
		"form":                   "dir menu ol ul",
		"fieldset":               "legend",
		"fieldset form li table": "listing pre",
	} {
		for _, o := range strings.Fields(names) {
			m[o] = setOf(through)
		}
	}
	return m
}()

// ghNoPush are the start tags after which GitHub's parser leaves no element open
// inside a heading: every tag of ghStarts ends the heading after them as at its start
// (the vector nested).
var ghNoPush = setOf("area base basefont body br col frame head hr html img input isindex link meta param")

// ghPriority is the end priority of libxml2's HTML parser, 100 for a name it does not
// list: a misplaced end tag closes no element of a higher priority than its own (the
// vector rawHeadings: a div open inside a heading keeps the heading's end tag from
// ending it).
func ghPriority(name string) int {
	switch name {
	case "div":
		return 150
	case "td", "th":
		return 160
	case "tr":
		return 170
	case "thead", "tbody", "tfoot":
		return 180
	case "table":
		return 190
	case "head", "body":
		return 200
	case "html":
		return 220
	}
	return 100
}

// ghAutoClose are the pairs old>new of libxml2's htmlStartClose, versions 2.9.14 and
// 2.13.5 together: a start tag new may close an open element old. The engine reads no
// such closing; where a start tag may close an element open inside a heading or
// outside it, it no longer knows what is open there.
var ghAutoClose = setOf(`a>a a>fieldset a>table a>td a>th address>dd address>dl address>dt address>form
	address>li address>ul b>center b>p b>td b>th big>p caption>col caption>colgroup
	caption>tbody caption>tfoot caption>thead caption>tr col>col col>colgroup col>tbody
	col>tfoot col>thead col>tr colgroup>colgroup colgroup>tbody colgroup>tfoot
	colgroup>thead colgroup>tr dd>dt dir>dd dir>dl dir>dt dir>form dir>ul dl>form dl>li
	dt>dd dt>dl font>center font>td font>th form>form h1>fieldset h1>form h1>li h1>p
	h1>table h2>fieldset h2>form h2>li h2>p h2>table h3>fieldset h3>form h3>li h3>p
	h3>table h4>fieldset h4>form h4>li h4>p h4>table h5>fieldset h5>form h5>li h5>p
	h5>table h6>fieldset h6>form h6>li h6>p h6>table head>a head>abbr head>acronym
	head>address head>b head>bdo head>big head>blockquote head>body head>br head>center
	head>cite head>code head>dd head>dfn head>dir head>div head>dl head>dt head>em
	head>fieldset head>font head>form head>frameset head>h1 head>h2 head>h3 head>h4
	head>h5 head>h6 head>hr head>i head>iframe head>img head>kbd head>li head>listing
	head>map head>menu head>ol head>p head>pre head>q head>s head>samp head>small
	head>span head>strike head>strong head>sub head>sup head>table head>tt head>u head>ul
	head>var head>xmp hr>form i>center i>p i>td i>th legend>fieldset li>li link>body
	link>frameset listing>dd listing>dl listing>dt listing>fieldset listing>form
	listing>li listing>table listing>ul menu>dd menu>dl menu>dt menu>form menu>ul ol>form
	ol>ul option>optgroup option>option p>address p>blockquote p>body p>caption p>center
	p>col p>colgroup p>dd p>dir p>div p>dl p>dt p>fieldset p>form p>frameset p>h1 p>h2
	p>h3 p>h4 p>h5 p>h6 p>head p>hr p>li p>listing p>menu p>ol p>p p>pre p>table p>tbody
	p>td p>tfoot p>th p>title p>tr p>ul p>xmp pre>dd pre>dl pre>dt pre>fieldset pre>form
	pre>li pre>table pre>ul s>p script>noscript small>p span>td span>th strike>p
	style>body style>frameset tbody>tbody tbody>tfoot td>tbody td>td td>tfoot td>th td>tr
	tfoot>tbody th>tbody th>td th>tfoot th>th th>tr thead>tbody thead>tfoot title>body
	title>frameset tr>tbody tr>tfoot tr>tr tt>p u>p u>td u>th ul>address ul>form ul>menu
	ul>ol ul>pre xmp>dd xmp>dl xmp>dt xmp>fieldset xmp>form xmp>li xmp>table xmp>ul`)

// ghOuter is what the engine reads of the elements open in the HTML GitHub parses, a
// heading's or the document's: their names, innermost last, and whether the engine
// knows them, which it does not once a tag may have closed one of them by a rule it
// does not read, or content it does not read stood between.
type ghOuter struct {
	stack []string
	known bool
}

// start reads a start tag, which opens its element unless closed by "/>" or of ghNoPush.
func (o *ghOuter) start(name string, self bool) {
	if n := len(o.stack); n > 0 && (ghAutoClose[o.stack[n-1]+">"+name] || isHeadingName(name) && isHeadingName(o.stack[n-1])) {
		o.known = false
	}
	if !self && !ghNoPush[name] {
		o.stack = append(o.stack, name)
	}
}

// end reads an end tag, which closes its innermost open element and those above it,
// unless one above it outranks it, or nothing when none is open.
func (o *ghOuter) end(name string) {
	for k := len(o.stack) - 1; k >= 0; k-- {
		if o.stack[k] == name {
			for _, above := range o.stack[k+1:] {
				if ghPriority(above) > ghPriority(name) {
					o.known = false
				}
			}
			o.stack = o.stack[:k]
			return
		}
	}
}

// copyOuter returns a copy of o.
func (o ghOuter) copyOuter() ghOuter {
	return ghOuter{stack: append([]string(nil), o.stack...), known: o.known}
}

// isHeadingName reports whether name is h1 to h6.
func isHeadingName(name string) bool {
	return len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6'
}

// ghElement reads the text of one heading element as GitHub's parser does: from its
// start tag, its text until an event ends it. ends are the texts at which it may end,
// each where an event can end it; done is whether it has ended; prefix whether it
// continues past what the engine reads, its text so far then being a prefix of its
// text. inner holds the elements open inside it, outer those open outside it when it
// started, and spans the rendered inline elements that enclose its start tag; maybe
// records that the last event may have ended it.
type ghElement struct {
	level  int
	text   strings.Builder
	inner  ghOuter
	outer  ghOuter
	spans  []int
	ends   []string
	done   bool
	prefix bool
	maybe  bool
}

// end records that e may end here, and has ended when sure; maybe records that the last
// event may have ended it.
func (e *ghElement) end(sure bool) {
	e.ends = append(e.ends, e.text.String())
	if sure {
		e.done = true
	} else {
		e.maybe = true
	}
}

// cont records that e continues into content the engine does not read.
func (e *ghElement) cont() {
	e.prefix = true
	e.done = true
}

// feed reads the event ev, which follows the text read so far.
func (e *ghElement) feed(ev ghEvent) {
	if e.done {
		return
	}
	e.maybe = false
	switch {
	case !ev.tag():
		e.text.WriteString(ev.text)
	case ev.end:
		e.feedEnd(ev.name)
	default:
		e.feedStart(ev)
	}
}

// feedStart reads a start tag: a tag of ghStarts ends e where the vectors record that it
// does, and may end it where the engine does not know; any other start tag opens an
// element inside e, unless it is closed by "/>" or of ghNoPush.
func (e *ghElement) feedStart(ev ghEvent) {
	if ev.name == "isindex" {
		e.text.WriteString(ghIsindexText)
	}
	if ghStarts[ev.name] {
		cut, sure := e.startEnds(ev)
		if cut || !sure {
			e.end(sure)
			if e.done {
				return
			}
		}
	}
	e.inner.start(ev.name, ev.self)
}

// startEnds reports whether the start tag ev of ghStarts ends e, and whether that is
// sure. It closes the elements open inside e from the innermost, each one that
// ghThrough records it closing, and ends e when it closes them all: with none open it
// ends e; at one that ghThrough records it not closing it ends nothing. The engine does
// not know at an element ghThrough does not record, when it no longer knows what is open
// inside e, or inside an emphasis or a link that does not enclose e's start tag.
func (e *ghElement) startEnds(ev ghEvent) (bool, bool) {
	for _, s := range ev.spans {
		if !containsInt(e.spans, s) {
			return false, false
		}
	}
	if !e.inner.known {
		return false, false
	}
	for k := len(e.inner.stack) - 1; k >= 0; k-- {
		through, ok := ghThrough[e.inner.stack[k]]
		switch {
		case !ok:
			return false, false
		case !through[ev.name]:
			return false, true
		}
	}
	return true, true
}

// feedEnd reads an end tag: e's own ends it unless an element that outranks it is open
// inside it; </body> and </html> end it; one of an element open inside it closes that
// element; another heading's is nothing; one of an element open outside it ends it,
// unless an element that outranks it is open inside; where the engine does not know
// what is open, either may hold.
func (e *ghElement) feedEnd(name string) {
	outranked := func() bool {
		for _, o := range e.inner.stack {
			if ghPriority(o) > ghPriority(name) {
				return true
			}
		}
		return false
	}
	switch {
	case name == "h"+strconv.Itoa(e.level):
		switch {
		case !e.inner.known:
			e.end(false)
		case !outranked():
			e.end(true)
		}
	case name == "body" || name == "html":
		e.end(true)
	case containsString(e.inner.stack, name):
		known := e.inner.known
		e.inner.end(name)
		if !known {
			e.end(false)
		}
	case isHeadingName(name):
	case !e.outer.known || !e.inner.known:
		e.end(false)
	case containsString(e.outer.stack, name) && !outranked():
		e.end(true)
	}
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ghRead is GitHub's reading of a heading element: a markdown heading (head) or a raw
// HTML start tag (at, the offset of its '<' in e's text). anchored is false for a
// heading that is surely inside another, which GitHub gives no anchor; ends and prefix
// are its element's (ghElement).
type ghRead struct {
	head     *heading
	e        element
	at       int
	anchored bool
	el       *ghElement
}

// slugs returns the slugs at which the heading may end, and the slug of its text so far
// when it continues past what the engine reads.
func (g ghRead) slugs() ([]string, string, bool) {
	var out []string
	for _, t := range g.el.ends {
		out = append(out, slugGH(t))
	}
	if g.el.prefix {
		return out, slugGH(g.el.text.String()), true
	}
	return out, "", false
}

// takes reports whether the heading takes the anchor a under S_GH: a slug at which it may
// end takes a, or its slug so far, when it continues past what the engine reads,
// begins a.
func (g ghRead) takes(a string) bool {
	ends, so, cont := g.slugs()
	for _, s := range ends {
		if takesAnchor(s, a) {
			return true
		}
	}
	return cont && strings.HasPrefix(a, so)
}

// ghItem is a block of the file as GitHub renders it: the tag it opens with ("p" for a
// paragraph, "hN" for a heading, none for an HTML block), its events, its element, its
// heading, its lines, and whether it is inside a container.
type ghItem struct {
	open     string
	events   []ghEvent
	e        element
	head     *heading
	at, last int
	nested   bool
}

// ghReadings returns GitHub's reading of every heading element of the file, markdown
// headings and raw HTML start tags h1 to h6, in their order (spec 10, Detail headings
// and anchors). The HTML GitHub parses is read block by block, each top-level block
// opening with its tag and closing with its end tag. An element its block does not end
// continues into the next top-level block: a paragraph, a heading or an HTML block,
// whose events it reads, or a thematic break, an empty element; at the end of the file
// it ends. Before any other content — a block inside a container, a code block, a list
// marker — and at the end of a block inside a container, it continues past what the
// engine reads.
func ghReadings(lines []string, sc scan, g linkRefs) []ghRead {
	items := ghItems(lines, sc, g)
	var out []ghRead
	var live []*ghElement
	doc := ghOuter{known: true}
	feedAll := func(ev ghEvent) {
		for _, el := range live {
			el.feed(ev)
		}
		switch {
		case !ev.tag():
		case ev.end:
			doc.end(ev.name)
		default:
			doc.start(ev.name, ev.self)
		}
	}
	contAll := func() {
		doc.known = false
		for _, el := range live {
			if !el.done {
				el.cont()
			}
		}
	}
	// open feeds the start tag of a heading element and starts reading it, unless it is
	// no element: GitHub keeps no heading element inside a select (the vector nested).
	open := func(it ghItem, ev ghEvent, level int) {
		before := doc.copyOuter()
		feedAll(ev)
		inside, sure := false, true
		for _, el := range live {
			if el.done {
				continue
			}
			if containsString(el.inner.stack[:max(len(el.inner.stack)-1, 0)], "select") {
				return
			}
			inside = true
			if el.maybe {
				sure = false
			}
		}
		if !it.nested {
			before.known = before.known && doc.known
		} else {
			before.known = false
		}
		el := &ghElement{level: level, spans: ev.spans, inner: ghOuter{known: true}, outer: before}
		if ev.self {
			el.end(true)
		}
		out = append(out, ghRead{head: it.head, e: it.e, at: ev.at, anchored: !(inside && sure), el: el})
		live = append(live, el)
	}
	for k, it := range items {
		if k > 0 {
			for i := items[k-1].last + 1; i < it.at; i++ {
				switch {
				case isBlank(lines[i]):
				case isThematicBreak(lines[i]):
					feedAll(ghEvent{name: "hr", self: true})
				default:
					contAll()
				}
			}
		}
		if it.nested {
			contAll()
		}
		switch {
		case it.head != nil:
			open(it, ghEvent{name: it.open}, it.head.level)
		case it.open != "":
			feedAll(ghEvent{name: it.open})
		}
		for _, ev := range it.events {
			if ev.tag() && !ev.end && isHeadingName(ev.name) {
				open(ghItem{e: it.e, nested: it.nested}, ev, int(ev.name[1]-'0'))
				continue
			}
			feedAll(ev)
		}
		if it.open != "" {
			feedAll(ghEvent{name: it.open, end: true})
		}
		if it.nested {
			contAll()
		}
		kept := live[:0]
		for _, el := range live {
			if !el.done {
				kept = append(kept, el)
			}
		}
		live = kept
	}
	for _, el := range live {
		el.end(true)
	}
	return out
}

// ghItems returns the blocks of the file that GitHub renders with text, in order: its
// headings, and its paragraphs and HTML blocks that are not headings; a paragraph that is
// all link reference definitions renders nothing, and neither does the content of a
// footnote definition that GitHub drops. The content of one it renders is inside a
// container, rendered at the end of the document.
func ghItems(lines []string, sc scan, g linkRefs) []ghItem {
	setext := setextStarts(sc.blocks)
	var items []ghItem
	for i := range sc.all {
		h := &sc.all[i]
		if !g.shows(h.note) {
			continue
		}
		e := headingElement(lines, *h, setext)
		items = append(items, ghItem{open: "h" + strconv.Itoa(h.level), events: inlineEvents(e.text, g), e: e, head: h, at: h.at, last: h.last, nested: h.nested})
	}
	for _, u := range sc.units {
		if !g.shows(u.note) {
			continue
		}
		top := true
		for k, l := range u.lines {
			if lines[u.at+k] != l {
				top = false
			}
		}
		last := u.at + len(u.lines) - 1
		if u.html {
			e := htmlElement(lines, u)
			items = append(items, ghItem{events: htmlEvents(e.text), e: e, at: u.at, last: last, nested: !top})
			continue
		}
		_, cm := cmDefLines(u.lines)
		all := true
		for _, d := range cm {
			all = all && d
		}
		if all {
			continue
		}
		e := linesElement(lines, u, cm)
		items = append(items, ghItem{open: "p", events: inlineEvents(e.text, g), e: e, at: u.at, last: last, nested: !top})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].at < items[j].at })
	return items
}
