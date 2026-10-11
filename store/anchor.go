package store

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// The collision rule of spec 10 (Detail headings and anchors): a heading that is not a
// detail heading takes the anchor of a detail heading when one of its slugs, S_GH, S_VS
// or S_GL, equals the anchor or is numbered to it (takesAnchor), each slug reading the
// headings of its viewer's blocks (readViewers) and S_GH reading the heading
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
// headings, in line order. The detail headings are those of the file's CommonMark
// reading; every other heading that a viewer anchors, in that viewer's reading of the
// file's blocks (readViewers), is compared under that viewer's slug. Two detail headings
// never collide here: their anchors are equal only when their keys differ in case, which
// the read reports as key.duplicate. A file without a detail heading has no anchor to
// take.
func anchorClashes(lines []string, tables map[string]int) []anchorClash {
	sc := scanHeadings(lines, 0)
	var anchors []detailAnchor
	details := map[int]heading{}
	for _, h := range sc.all {
		if table, key, ok := detailHeading(h, tables); ok {
			anchors = append(anchors, detailAnchor{text: h.text, anchor: DetailAnchor(table, key)})
			details[h.at] = h
		}
	}
	if len(anchors) == 0 {
		return nil
	}
	x := newAnchorSet(anchors)
	// isDetail reports whether a viewer's heading is a detail heading of the file: the
	// heading of the same line, level and text.
	isDetail := func(h heading) bool {
		d, ok := details[h.at]
		return ok && d.level == h.level && d.text == h.text
	}
	v := readViewers(lines, sc)
	var out []anchorClash
	ghSetext := setextStarts(v.gh.sc.blocks)
	for _, r := range ghReadings(lines, v.gh.sc, v.gh.refs) {
		switch {
		case r.head == nil:
			if r.anchored {
				out = append(out, rawHeadingClashes(r, x)...)
			}
		case !isDetail(*r.head):
			out = append(out, slugClashes(headingElement(lines, *r.head, ghSetext), r.taken(x), x)...)
		}
	}
	vsSetext := setextStarts(v.vs.sc.blocks)
	for _, h := range v.vs.sc.all {
		if !isDetail(h) {
			e := headingElement(lines, h, vsSetext)
			s := slugVS(strings.Join(vsText(e.text, v.vs.refs), ""))
			out = append(out, slugClashes(e, x.takenBy(s), x)...)
		}
	}
	for _, h := range v.gl.sc.all {
		if !isDetail(h) && v.gl.refs.shows(h.note) {
			e := headingElement(lines, h, ghSetext)
			s := slugGL(strings.Join(glText(e.text, v.gl.refs), " "))
			out = append(out, slugClashes(e, x.takenBy(s), x)...)
		}
	}
	out = append(out, v.gh.tagClashes(lines, x, v.gl.refs)...)
	if v.math {
		out = append(out, v.vs.tagClashes(lines, x, v.vs.refs)...)
	}
	return sortedClashes(out)
}

// anchorSet is the detail anchors of a file, in the order of their headings, indexed
// by the slugs that take them (takesAnchor): exact by the anchor itself, and numbered by
// the anchor without a final '-' and decimal number of no leading zero, which is the one
// slug that the numbering rule numbers to it. A slug finds the anchors it takes by two
// lookups, so the rule costs no comparison of every heading with every anchor.
type anchorSet struct {
	list            []detailAnchor
	exact, numbered map[string][]int
}

// newAnchorSet returns the anchor set of anchors.
func newAnchorSet(anchors []detailAnchor) anchorSet {
	x := anchorSet{list: anchors, exact: map[string][]int{}, numbered: map[string][]int{}}
	for i, a := range anchors {
		x.exact[a.anchor] = append(x.exact[a.anchor], i)
		if k := strings.LastIndexByte(a.anchor, '-'); k >= 0 && takesAnchor(a.anchor[:k], a.anchor) {
			x.numbered[a.anchor[:k]] = append(x.numbered[a.anchor[:k]], i)
		}
	}
	return x
}

// takenBy returns the indices of the anchors that a heading of the slug s takes.
func (x anchorSet) takenBy(s string) []int {
	return mergeIndices(x.exact[s], x.numbered[s])
}

// mergeIndices returns the distinct indices of a and b, ascending; each is ascending.
func mergeIndices(a, b []int) []int {
	if len(b) == 0 {
		return a
	}
	if len(a) == 0 {
		return b
	}
	out := append(append([]int(nil), a...), b...)
	sort.Ints(out)
	return slices.Compact(out)
}

// slugClashes returns the collisions of the heading element e with the anchors of x at
// the indices taken.
func slugClashes(e element, taken []int, x anchorSet) []anchorClash {
	var out []anchorClash
	for _, i := range taken {
		out = append(out, e.clash(0, e.text, x.list[i]))
	}
	return out
}

// sortedClashes returns the distinct collisions of out in line order, then in the order
// of their offsets: a heading that more than one viewer's slug takes the same anchor by
// is one collision.
func sortedClashes(out []anchorClash) []anchorClash {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].line != out[j].line {
			return out[i].line < out[j].line
		}
		return out[i].at < out[j].at
	})
	kept := out[:0]
	seen := map[anchorClash]bool{}
	for _, c := range out {
		if !seen[c] {
			seen[c] = true
			kept = append(kept, c)
		}
	}
	return kept
}

// viewerReading is a viewer's reading of the blocks of a file and its definitions.
type viewerReading struct {
	sc   scan
	refs linkRefs
}

// viewerReadings are the readings of a file by GitHub, VS Code and GitLab, each of the
// blocks its viewer reads (spec 10, Detail headings and anchors): GitHub and GitLab read
// footnote definitions, VS Code math blocks, besides CommonMark. A reading is a scan of
// its own only when a line of the file may open its construct; otherwise it is the
// CommonMark reading, which it then equals, and notes and math are false.
type viewerReadings struct {
	gh, vs, gl  viewerReading
	notes, math bool
}

// readViewers returns the viewers' readings of the file lines, whose CommonMark reading
// is cm. GitHub's and GitLab's footnote definitions are numbered by their first
// reference in the document (numberNotes).
func readViewers(lines []string, cm scan) viewerReadings {
	var v viewerReadings
	refs := linkDefs(cm.blocks)
	notes := viewerReading{sc: cm, refs: refs}
	if mayOpen(lines, "[^", "]:") {
		v.notes = true
		notes.sc = scanWith(lines, 0, blockSyntax{notes: true})
		notes.refs = linkDefs(notes.sc.blocks)
	}
	v.gh, v.gl = notes, notes
	v.gh.refs.notes = newNoteRefs(notes.sc.notes, true)
	v.gl.refs.notes = newNoteRefs(notes.sc.notes, false)
	if len(notes.sc.notes) > 0 {
		numberNotes(lines, v.gh.sc, v.gh.refs, true)
		numberNotes(lines, v.gl.sc, v.gl.refs, false)
	}
	v.vs = viewerReading{sc: cm, refs: refs}
	if mayOpen(lines, "$$") {
		v.math = true
		v.vs.sc = scanWith(lines, 0, blockSyntax{math: true})
		v.vs.refs = linkDefs(v.vs.sc.blocks)
	}
	return v
}

// mayOpen reports whether a line of the file holds every one of marks, which a line
// that opens the construct holds: a reading of that construct differs from CommonMark's
// only where such a line stands.
func mayOpen(lines []string, marks ...string) bool {
	for _, l := range lines {
		all := true
		for _, m := range marks {
			all = all && strings.Contains(l, m)
		}
		if all {
			return true
		}
	}
	return false
}

// numberNotes reads the inline content of the scan sc, its headings and paragraphs, in
// document order, as GitHub (gh) or GitLab reads it with refs, so that refs.notes numbers
// each footnote definition by its first reference, as cmark-gfm's and comrak's
// process_footnotes number them over the whole document, a definition's content and an
// image's description included.
func numberNotes(lines []string, sc scan, refs linkRefs, gh bool) {
	setext := setextStarts(sc.blocks)
	type content struct {
		at   int
		text string
	}
	var all []content
	for _, h := range sc.all {
		all = append(all, content{h.at, headingElement(lines, h, setext).text})
	}
	for _, u := range sc.units {
		if !u.html {
			_, cm := cmDefLines(u.lines)
			all = append(all, content{u.at, linesElement(lines, u, cm).text})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].at < all[j].at })
	for _, c := range all {
		r := inlineReader{s: c.text, refs: refs, gh: gh}
		if !gh {
			r.math = glMath
		}
		r.run()
	}
}

// shows reports whether the viewer of refs renders the content of the footnote
// definition d, none for content outside every definition (noteRefs.home).
func (r linkRefs) shows(d *noteDef) bool {
	if d == nil || r.notes == nil {
		return true
	}
	_, shown := r.notes.home(d)
	return shown
}

// tagClashes returns the collisions of the HTML tags of the reading's headings and
// paragraphs and HTML blocks with the anchors, but those of the footnote definitions
// that neither the reading's viewer nor the viewer of other renders.
func (v viewerReading) tagClashes(lines []string, x anchorSet, other linkRefs) []anchorClash {
	setext := setextStarts(v.sc.blocks)
	var out []anchorClash
	for _, h := range v.sc.all {
		if v.refs.shows(h.note) || other.shows(h.note) {
			e := headingElement(lines, h, setext)
			tags, at := inlineTags(e.text, v.refs)
			out = append(out, tagClashes(e, tags, at, x)...)
		}
	}
	for _, u := range v.sc.units {
		if v.refs.shows(u.note) || other.shows(u.note) {
			out = append(out, unitClashes(lines, u, x, v.refs)...)
		}
	}
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

// rawHeadingClashes returns the collisions of a raw HTML heading that GitHub anchors:
// its S_GH against each anchor, located at the '<' of its start tag. VS Code and GitLab
// give a raw HTML heading no anchor.
func rawHeadingClashes(r ghRead, x anchorSet) []anchorClash {
	var out []anchorClash
	for _, i := range r.taken(x) {
		out = append(out, r.e.clash(r.at, strings.TrimSpace(r.el.text.String()), x.list[i]))
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
func unitClashes(lines []string, u unit, x anchorSet, g linkRefs) []anchorClash {
	if u.html {
		e := htmlElement(lines, u)
		tags, at := rawTags(e.text)
		return tagClashes(e, tags, at, x)
	}
	_, cm := cmDefLines(u.lines)
	ge := linesElement(lines, u, cm)
	tags, at := inlineTags(ge.text, g)
	return tagClashes(ge, tags, at, x)
}

// tagClashes returns the collisions of the id and name attributes of the open tags with
// the anchors: a value that equals an anchor, or user-content- followed by it, once
// lowercased by the full mapping, as VS Code compares a fragment with an id.
// user-content- followed by the anchor is the form GitHub renders the heading's id in,
// and a raw id or name too, unless it already begins with user-content-
// (TestGitHubAttributes), and GitLab's form of the id; both compare exactly, so the
// lowercased comparison covers them.
func tagClashes(e element, tags [][]htmlAttr, at []int, x anchorSet) []anchorClash {
	var out []anchorClash
	for i, attrs := range tags {
		for _, attr := range attrs {
			if attr.name != "id" && attr.name != "name" {
				continue
			}
			v := lowerFull(attr.value)
			var prefixed []int
			if rest, ok := strings.CutPrefix(v, "user-content-"); ok {
				prefixed = x.exact[rest]
			}
			for _, k := range mergeIndices(x.exact[v], prefixed) {
				out = append(out, e.clash(at[i], attr.value, x.list[k]))
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
