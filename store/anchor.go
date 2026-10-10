package store

import (
	"sort"
	"strings"
)

// The collision rule of spec 10 (Detail headings and anchors): a heading that is not a
// detail heading takes the anchor of a detail heading when its fold equals the anchor's
// fold once a final run of digits is removed from each; an attribute list that sets the
// id of an anchor, and an HTML id or name equal to an anchor, collide too. A detail
// heading is a top-level heading whose text is `<table> row <key>` at the detail level
// of its table, the row scope level less one, which the layouts of the file give.

// anchorClash is a collision of a file with a detail anchor: line is the line where the
// colliding heading or block starts, text is the colliding heading's text or the id or
// the attribute value that collides, and other is the text of the detail heading.
type anchorClash struct {
	line        int
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
	var anchors []detailAnchor
	var others []heading
	for _, h := range sc.all {
		if table, key, ok := detailHeading(h, tables); ok {
			anchors = append(anchors, detailAnchor{text: h.text, anchor: DetailAnchor(table, key)})
		} else {
			others = append(others, h)
		}
	}
	var out []anchorClash
	for _, h := range others {
		out = append(out, headingClashes(h, anchors)...)
	}
	for _, u := range sc.units {
		out = append(out, unitClashes(u, anchors)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].line < out[j].line })
	return out
}

// detailHeading reports whether h is a detail heading of one of the tables, and its
// table and key.
func detailHeading(h heading, tables map[string]int) (string, string, bool) {
	if h.nested {
		return "", "", false
	}
	parts := strings.Fields(h.text)
	if len(parts) != 3 || tables[parts[0]] != h.level || !strings.EqualFold(parts[1], "row") {
		return "", "", false
	}
	return parts[0], parts[2], true
}

// headingClashes returns the collisions of a heading that is not a detail heading: its
// fold against each anchor, and the attribute list and the HTML tags of its text.
func headingClashes(h heading, anchors []detailAnchor) []anchorClash {
	kept, tags := inlineText(h.text)
	var out []anchorClash
	for _, a := range anchors {
		if foldsCollide(kept, a.anchor) {
			out = append(out, anchorClash{line: h.at + 1, text: h.text, other: a.text})
		}
	}
	out = append(out, tagClashes(h.at+1, tags, anchors)...)
	if id, ok := attrID(h.text); ok {
		out = append(out, idClashes(h.at+1, id, anchors)...)
	}
	return out
}

// unitClashes returns the collisions of a paragraph or an HTML block: the attribute list
// that ends it, and the HTML tags it holds.
func unitClashes(u unit, anchors []detailAnchor) []anchorClash {
	line := u.at + 1
	if u.html {
		return tagClashes(line, rawTags(strings.Join(u.lines, "\n")), anchors)
	}
	text := unitText(u)
	_, tags := inlineText(text)
	out := tagClashes(line, tags, anchors)
	if id, ok := attrID(text); ok {
		out = append(out, idClashes(line, id, anchors)...)
	}
	return out
}

// foldsCollide reports whether the text folds to the anchor under F_G or under F_M once
// a final run of digits is removed from each. The fold of an anchor is the same under
// both folds, since its letters and digits are ASCII.
func foldsCollide(text, anchor string) bool {
	return stripDigits(foldG(text)) == stripDigits(foldG(anchor)) ||
		stripDigits(foldM(text)) == stripDigits(foldM(anchor))
}

// tagClashes returns the collisions of the id and name attributes of the open tags with
// the anchors, which compare without regard to case.
func tagClashes(line int, tags [][]htmlAttr, anchors []detailAnchor) []anchorClash {
	var out []anchorClash
	for _, attrs := range tags {
		for _, at := range attrs {
			if at.name != "id" && at.name != "name" {
				continue
			}
			for _, a := range anchors {
				if strings.EqualFold(at.value, a.anchor) {
					out = append(out, anchorClash{line: line, text: at.value, other: a.text})
				}
			}
		}
	}
	return out
}

// idClashes returns the collisions of an id that an attribute list sets with the anchors.
func idClashes(line int, id string, anchors []detailAnchor) []anchorClash {
	var out []anchorClash
	for _, a := range anchors {
		if strings.EqualFold(id, a.anchor) {
			out = append(out, anchorClash{line: line, text: id, other: a.text})
		}
	}
	return out
}

// attrID returns the id that the attribute list at the end of the last line of text
// sets, and false when that line ends in no attribute list. The list is read the way
// Python-Markdown's attr_list reads one: braces around white space separated tokens,
// an optional colon after the opening brace, and a '#' token or an id= token that sets
// the id, the last of them winning.
func attrID(text string) (string, bool) {
	line := text[strings.LastIndexByte(text, '\n')+1:]
	t := strings.TrimRight(line, " \t")
	if !strings.HasSuffix(t, "}") {
		return "", false
	}
	end := len(t) - 1
	start := strings.LastIndexByte(t[:end], '}') + 1
	open := strings.IndexByte(t[start:end], '{')
	if open < 0 {
		return "", false
	}
	body := strings.TrimLeft(strings.TrimPrefix(t[start+open+1:end], ":"), " ")
	if body == "" {
		return "", false
	}
	id, found := "", false
	for _, tok := range strings.Fields(body) {
		switch {
		case strings.HasPrefix(tok, "#") && len(tok) > 1:
			id, found = tok[1:], true
		case strings.HasPrefix(tok, "id="):
			id, found = unquoted(tok[3:]), true
		}
	}
	return id, found
}

// unquoted returns v without the quotes around it, when it has a matching pair.
func unquoted(v string) string {
	if n := len(v); n >= 2 && (v[0] == '"' || v[0] == '\'') && v[n-1] == v[0] {
		return v[1 : n-1]
	}
	return v
}

// rawTags returns the attributes of the open tags in text, which an HTML block holds
// as raw text: its code spans are no code, and a comment or a declaration holds no tag.
func rawTags(text string) [][]htmlAttr {
	var tags [][]htmlAttr
	for i := 0; i < len(text); {
		if text[i] == '<' {
			if n, attrs, open := htmlMarkupAt(text[i:]); n > 0 {
				if open {
					tags = append(tags, attrs)
				}
				i += n
				continue
			}
		}
		i++
	}
	return tags
}

// anchorRefusals returns the CONSTRAINT diagnostics of a write: one for each collision
// of the written file rel with a detail anchor.
func anchorRefusals(rel string, out []byte, tables map[string]int) []Diagnostic {
	var diags []Diagnostic
	for _, c := range anchorClashes(splitLines(string(out)), tables) {
		diags = append(diags, Diagnostic{File: rel, Line: c.line, Column: 1, Rule: RuleConstraint, Message: c.message()})
	}
	return diags
}
