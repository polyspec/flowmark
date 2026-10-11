package store

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// The collision rule of spec 10 (Detail headings and anchors): a heading that is not a
// detail heading takes the anchor of a detail heading when its fold equals the anchor's
// fold once a final run of digits is removed from each; an attribute list that sets the
// id of an anchor, and an HTML id or name equal to an anchor, collide too. A detail
// heading is a top-level heading whose text is `<table> row <key>` at the detail level
// of its table, the row scope level less one, which the layouts of the file give.

// anchorClash is a collision of a file with a detail anchor: line is the 1-based line,
// and at the byte offset in that line, of the colliding text: the text where a heading
// starts, the '{' of an attribute list, or the '<' of an HTML tag. text is the colliding
// heading's text or the id or the attribute value that collides, and other is the text
// of the detail heading.
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
	g, m := linkDefs(sc.blocks)
	var anchors []detailAnchor
	var others []heading
	for _, h := range sc.all {
		if table, key, ok := detailHeading(h, tables); ok {
			anchors = append(anchors, detailAnchor{text: h.text, anchor: DetailAnchor(table, key)})
		} else {
			others = append(others, h)
		}
	}
	setext := map[int]unit{}
	for _, u := range sc.blocks {
		if u.heading {
			setext[u.at] = u
		}
	}
	var out []anchorClash
	for _, h := range others {
		out = append(out, headingClashes(lines, h, setext, anchors, g, m)...)
	}
	for _, u := range sc.units {
		out = append(out, unitClashes(lines, u, anchors, g)...)
	}
	out = append(out, attrClashes(lines, anchors)...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].line != out[j].line {
			return out[i].line < out[j].line
		}
		return out[i].at < out[j].at
	})
	return out
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
// lowercased.
func layoutTable(name string, tables map[string]int) (string, bool) {
	for t := range tables {
		if strings.ToLower(t) == strings.ToLower(name) {
			return t, true
		}
	}
	return "", false
}

// headingClashes returns the collisions of a heading that is not a detail heading: its
// fold against each anchor, and the HTML tags of its text. The text is read under F_G with
// the definitions of F_G, and under F_M with those of F_M. The attribute lists of the file
// are compared by attrClashes.
func headingClashes(lines []string, h heading, setext map[int]unit, anchors []detailAnchor, g, m linkRefs) []anchorClash {
	eg, em := headingElements(lines, h, setext)
	readG := inlineText(eg.text, g)
	keptM := inlineText(em.text, m).text
	var out []anchorClash
	for _, a := range anchors {
		if foldsCollide(readG.text, readG.alt, keptM, a.anchor) {
			out = append(out, eg.clash(0, eg.text, a))
		}
	}
	out = append(out, tagClashes(eg, readG.tags, readG.tagAt, anchors)...)
	return out
}

// headingElements returns the elements of a heading of the file, the one its text is read
// from under F_G and the one under F_M: for an ATX heading both are its text, and for a
// setext heading they are the lines of its paragraph. The F_G element drops only the
// definitions that CommonMark reads at the start of the paragraph, and the F_M element
// drops the lines of the definitions that Python-Markdown reads.
func headingElements(lines []string, h heading, setext map[int]unit) (element, element) {
	u, ok := setext[h.at]
	if h.last == h.at || !ok {
		e := newElement([]textPiece{{line: h.at, start: lineBase(lines[h.at], h.text), text: h.text}})
		return e, e
	}
	_, cm := cmDefLines(u.lines)
	md := mDefLines(u.lines)
	var g, m []textPiece
	for k, l := range u.lines {
		t := strings.TrimRight(strings.TrimLeft(l, " \t"), " \t")
		start := lineBase(lines[u.at+k], l) + len(l) - len(strings.TrimLeft(l, " \t"))
		piece := textPiece{line: u.at + k, start: start, text: t}
		if !cm[k] {
			g = append(g, piece)
		}
		if !md[k] {
			m = append(m, piece)
		}
	}
	return newElementOr(g, h, lines), newElementOr(m, h, lines)
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
// holds, read under F_G from the lines that CommonMark reads as its text. The attribute
// lists of the file are compared by attrClashes.
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

// foldsCollide reports whether the heading folds to the anchor once a final run of
// digits is removed from each: under F_G by its text or by its text with the alt text of
// its images, and under F_M by its text read with no text of an image. The folds of an
// anchor are the same under both, since its letters and digits are ASCII.
func foldsCollide(textG, altG, textM, anchor string) bool {
	a := stripDigits(foldG(anchor))
	return stripDigits(foldG(textG)) == a || stripDigits(foldG(altG)) == a ||
		stripDigits(foldM(textM)) == stripDigits(foldM(anchor))
}

// tagClashes returns the collisions of the id and name attributes of the open tags with
// the anchors, which compare without regard to case.
func tagClashes(e element, tags [][]htmlAttr, at []int, anchors []detailAnchor) []anchorClash {
	var out []anchorClash
	for i, attrs := range tags {
		for _, attr := range attrs {
			if attr.name != "id" && attr.name != "name" {
				continue
			}
			for _, a := range anchors {
				if strings.EqualFold(attr.value, a.anchor) {
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
