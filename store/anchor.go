package store

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// The collision rule of spec 10 (Detail headings and anchors): a heading that is not a
// detail heading takes the anchor of a detail heading when one of its slugs, S_GH, S_VS
// or S_GL, equals the anchor or is numbered to it (takesAnchor), S_GH reading the heading
// element as GitHub parses it, a raw HTML heading included (ghReadings); an HTML id or name that
// equals the anchor, or user-content- followed by it, once lowercased, in raw HTML as
// CommonMark 0.31.2 reads it, collides too. A detail heading is a
// top-level heading whose text is `<table> row <key>` at the detail level of its table,
// the row scope level less one, which the layouts of the file give.

// anchorClash is a collision of a file with a detail anchor: line is the 1-based line,
// and at the byte offset in that line, of the colliding text: the text where a heading
// starts, or the '<' of an HTML tag. text is the colliding heading's text or the
// attribute value that collides, and other is the text of the detail heading.
type anchorClash struct {
	line, at    int
	text, other string
}

// message is the diagnostic text of a collision, which names the other heading.
func (c anchorClash) message() string {
	return "the text " + c.text + " takes the anchor of the detail heading " + c.other
}

// detailAnchor is the anchor of a detail heading of the file, with its text.
type detailAnchor struct {
	text, anchor string
}

// textPiece is a run of an element's text on one line of the file: the 0-based index of
// the line, the byte offset in that line where the run starts, and the run's text.
type textPiece struct {
	line, start int
	text        string
}

// element is the text that the rule reads as one unit: a heading, a paragraph or an HTML
// block. Its pieces are its lines, and text joins their texts with line feeds, so an
// offset of text is located in the file by its piece.
type element struct {
	pieces []textPiece
	text   string
}

// newElement returns the element of pieces.
func newElement(pieces []textPiece) element {
	parts := make([]string, len(pieces))
	for i, p := range pieces {
		parts[i] = p.text
	}
	return element{pieces: pieces, text: strings.Join(parts, "\n")}
}

// locate returns the 0-based line of the file, and the byte offset in that line, of the
// character at off of the text of e.
func (e element) locate(off int) (int, int) {
	base := 0
	for _, p := range e.pieces {
		if off <= base+len(p.text) {
			return p.line, p.start + off - base
		}
		base += len(p.text) + 1
	}
	last := e.pieces[len(e.pieces)-1]
	return last.line, last.start
}

// clash returns the collision of the value at off of e with the anchor a.
func (e element) clash(off int, value string, a detailAnchor) anchorClash {
	line, at := e.locate(off)
	return anchorClash{line: line + 1, at: at, text: value, other: a.text}
}

// lineBase returns the byte offset of text in the file line raw, which holds it after
// the markers of its containers; 0 when the text is not found there.
func lineBase(raw, text string) int {
	if i := strings.Index(raw, text); i >= 0 {
		return i
	}
	return 0
}

// htmlElement returns the element of an HTML block of the file, whose lines are its text.
func htmlElement(file []string, u unit) element {
	pieces := make([]textPiece, len(u.lines))
	for k, l := range u.lines {
		pieces[k] = textPiece{line: u.at + k, start: lineBase(file[u.at+k], l), text: l}
	}
	return newElement(pieces)
}

// tableLevels returns the detail level of a table and of the block tables stored under
// it, by name: the row scope level less one, which their detail headings are at. A
// files row has no detail section of its own table, so its level is 0, which no heading
// has.
func tableLevels(table string, level int, children []*docLayout) map[string]int {
	levels := map[string]int{table: level}
	for _, c := range children {
		for n, l := range tableLevels(c.table, c.scope-1, c.children) {
			levels[n] = l
		}
	}
	return levels
}

// anchorClashes returns the collisions of the file lines with the anchors of its detail
// headings, in line order. Two detail headings never collide here: their anchors are
// equal only when their keys differ in case, which the read reports as key.duplicate.
func anchorClashes(lines []string, tables map[string]int) []anchorClash {
	sc := scanHeadings(lines, 0)
	g := linkDefs(sc.blocks)
	var anchors []detailAnchor
	var others []heading
	for _, h := range sc.all {
		if table, key, ok := detailHeading(h, tables); ok {
			anchors = append(anchors, detailAnchor{text: h.text, anchor: DetailAnchor(table, key)})
		} else {
			others = append(others, h)
		}
	}
	setext := setextStarts(sc.blocks)
	gh := map[int]ghRead{}
	var out []anchorClash
	for _, r := range ghReadings(lines, sc, g) {
		if r.head != nil {
			gh[r.head.at] = r
			continue
		}
		if r.anchored {
			out = append(out, rawHeadingClashes(r, anchors)...)
		}
	}
	for _, h := range others {
		out = append(out, headingClashes(lines, h, setext, anchors, g, gh[h.at])...)
	}
	for _, u := range sc.units {
		out = append(out, unitClashes(lines, u, anchors, g)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].line != out[j].line {
			return out[i].line < out[j].line
		}
		return out[i].at < out[j].at
	})
	return out
}

// setextStarts returns the setext headings among the blocks by the line where each
// begins: the first line of its paragraph that is text.
func setextStarts(blocks []unit) map[int]unit {
	setext := map[int]unit{}
	for _, u := range blocks {
		if u.heading {
			setext[u.at+textStart(u.lines)] = u
		}
	}
	return setext
}

// detailHeading reports whether h is a detail heading of one of the tables, and its
// table, as the layout names it, and its key. The table name and the word row are
// compared after lowercasing, as the viewers lowercase them.
func detailHeading(h heading, tables map[string]int) (string, string, bool) {
	if h.nested {
		return "", "", false
	}
	parts := detailFields(h.text)
	if len(parts) != 3 || strings.ToLower(parts[1]) != "row" {
		return "", "", false
	}
	table, ok := layoutTable(parts[0], tables)
	if !ok || tables[table] != h.level {
		return "", "", false
	}
	return table, parts[2], true
}

// detailFields splits the text of a heading into its words. The separators are the spaces,
// the tabs and the line endings of the heading; any other white space is part of a word.
func detailFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

// layoutTable returns the name of the table of the layout that name names once both are
// lowercased as the viewers lowercase them.
func layoutTable(name string, tables map[string]int) (string, bool) {
	for t := range tables {
		if lowerFull(t) == lowerFull(name) {
			return t, true
		}
	}
	return "", false
}

// headingClashes returns the collisions of a heading that is not a detail heading: its
// slugs against each anchor — S_GH by GitHub's reading of its element (gh), S_VS and
// S_GL by its rendered text as each reads it — and the HTML tags of its text, read with
// the definitions of the file.
func headingClashes(lines []string, h heading, setext map[int]unit, anchors []detailAnchor, g linkRefs, gh ghRead) []anchorClash {
	e := headingElement(lines, h, setext)
	read := viewerText(e.text, g)
	var out []anchorClash
	for _, a := range anchors {
		if gh.takes(a.anchor) || slugsTake(read, a.anchor) {
			out = append(out, e.clash(0, e.text, a))
		}
	}
	out = append(out, tagClashes(e, read.tags, read.tagAt, anchors)...)
	return out
}

// rawHeadingClashes returns the collisions of a raw HTML heading that GitHub anchors:
// its S_GH against each anchor, located at the '<' of its start tag. VS Code and GitLab
// give a raw HTML heading no anchor.
func rawHeadingClashes(r ghRead, anchors []detailAnchor) []anchorClash {
	var out []anchorClash
	for _, a := range anchors {
		if r.takes(a.anchor) {
			out = append(out, r.e.clash(r.at, strings.TrimSpace(r.el.text.String()), a))
		}
	}
	return out
}

// headingElement returns the element of a heading of the file that its text is read
// from: for an ATX heading its content, and for a setext heading the lines of its
// paragraph without the definitions that CommonMark reads at the start of the
// paragraph, each without its leading spaces and tabs and the last also without its
// trailing ones, as CommonMark forms a paragraph's raw content (§4.8). The spaces before
// a line ending stay for the inline reader, which reads them as part of a line break.
func headingElement(lines []string, h heading, setext map[int]unit) element {
	u, ok := setext[h.at]
	if h.last == h.at || !ok {
		lead, content := atxContent(h.text)
		return newElement([]textPiece{{line: h.at, start: lineBase(lines[h.at], h.text) + lead, text: content}})
	}
	_, cm := cmDefLines(u.lines)
	var g []textPiece
	for k, l := range u.lines {
		if cm[k] {
			continue
		}
		t := strings.TrimLeft(l, " \t")
		start := lineBase(lines[u.at+k], l) + len(l) - len(t)
		g = append(g, textPiece{line: u.at + k, start: start, text: t})
	}
	if n := len(g); n > 0 {
		g[n-1].text = strings.TrimRight(g[n-1].text, " \t")
	}
	return newElementOr(g, h, lines)
}

// atxContent returns the content of an ATX heading whose text after the separator is
// text, as CommonMark reads it (§4.2): without its leading and trailing spaces and tabs
// and without a closing run of '#' that spaces or tabs precede or that is the whole
// text, and the byte offset of the content in text.
func atxContent(text string) (int, string) {
	t := strings.TrimRight(text, " \t")
	j := strings.TrimRight(t, "#")
	if len(j) < len(t) && (j == "" || j[len(j)-1] == ' ' || j[len(j)-1] == '\t') {
		t = strings.TrimRight(j, " \t")
	}
	lead := len(t) - len(strings.TrimLeft(t, " \t"))
	return lead, t[lead:]
}

// newElementOr returns the element of pieces, or of the heading's own empty text when no
// line of the paragraph is left.
func newElementOr(pieces []textPiece, h heading, lines []string) element {
	if len(pieces) == 0 {
		return newElement([]textPiece{{line: h.at, start: lineBase(lines[h.at], h.text), text: ""}})
	}
	return newElement(pieces)
}

// linesElement returns the element of the lines of u that keep marks: the text of each
// kept line, untrimmed as an HTML block's text is, located in the file.
func linesElement(file []string, u unit, drop []bool) element {
	var pieces []textPiece
	for k, l := range u.lines {
		if drop[k] {
			continue
		}
		pieces = append(pieces, textPiece{line: u.at + k, start: lineBase(file[u.at+k], l), text: l})
	}
	if len(pieces) == 0 {
		return newElement([]textPiece{{line: u.at, start: 0, text: ""}})
	}
	return newElement(pieces)
}

// unitClashes returns the collisions of a paragraph or an HTML block: the HTML tags it
// holds, read from the lines that CommonMark reads as its text.
func unitClashes(lines []string, u unit, anchors []detailAnchor, g linkRefs) []anchorClash {
	if u.html {
		e := htmlElement(lines, u)
		tags, at := rawTags(e.text)
		return tagClashes(e, tags, at, anchors)
	}
	_, cm := cmDefLines(u.lines)
	ge := linesElement(lines, u, cm)
	r := inlineText(ge.text, g)
	return tagClashes(ge, r.tags, r.tagAt, anchors)
}

// tagClashes returns the collisions of the id and name attributes of the open tags with
// the anchors: a value that equals an anchor, or user-content- followed by it, once
// lowercased by the full mapping, as VS Code compares a fragment with an id.
// user-content- followed by the anchor is the form GitHub renders the heading's id in,
// and a raw id or name too, unless it already begins with user-content-
// (TestGitHubAttributes), and GitLab's form of the id; both compare exactly, so the
// lowercased comparison covers them.
func tagClashes(e element, tags [][]htmlAttr, at []int, anchors []detailAnchor) []anchorClash {
	var out []anchorClash
	for i, attrs := range tags {
		for _, attr := range attrs {
			if attr.name != "id" && attr.name != "name" {
				continue
			}
			v := lowerFull(attr.value)
			for _, a := range anchors {
				if v == a.anchor || v == "user-content-"+a.anchor {
					out = append(out, e.clash(at[i], attr.value, a))
				}
			}
		}
	}
	return out
}

// rawTags returns the attributes of the open tags in text, which an HTML block holds
// as raw text, and the offset of the '<' of each: its code spans are no code, and a
// comment or a declaration holds no tag.
func rawTags(text string) ([][]htmlAttr, []int) {
	var tags [][]htmlAttr
	var at []int
	for i := 0; i < len(text); {
		if text[i] == '<' {
			if n, attrs, open := htmlMarkupAt(text[i:]); n > 0 {
				if open {
					tags = append(tags, attrs)
					at = append(at, i)
				}
				i += n
				continue
			}
		}
		i++
	}
	return tags, at
}

// anchorRefusals returns the CONSTRAINT refusals of a write: one for each collision
// of the written bytes out with a detail anchor. Each is located at the row and the
// column whose text holds the colliding text, which the spans of its line record.
func anchorRefusals(lines [][]colSpan, out []byte, tables map[string]int) []Refusal {
	var rf []Refusal
	for _, c := range anchorClashes(splitLines(string(out)), tables) {
		o := originAt(lines[c.line-1], c.at)
		rf = append(rf, o.row.refusal(o.column, RuleConstraint, c.message()))
	}
	return rf
}

// clashColumn returns the 1-based column, in code points, of a collision in the file lines.
func clashColumn(lines []string, c anchorClash) int {
	return utf8.RuneCountInString(lines[c.line-1][:c.at]) + 1
}
