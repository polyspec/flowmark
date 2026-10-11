package store

import (
	"strings"

	"golang.org/x/text/cases"
)

// The link reference definitions that decide the rendered text of a heading, whose
// slugs decide a detail-anchor collision (spec 10, Detail headings and anchors), read as CommonMark 0.31.2 reads them: a block that
// starts a paragraph, inside a container or not, and that never interrupts one. A label
// is compared by CommonMark's case folding once its white space is collapsed.

// linkRefs are the labels that the definitions of a file define, in one viewer's reading:
// keys holds each defined label under its folded key; notes is the footnote definitions
// of a viewer that renders footnotes, GitHub's or GitLab's, and nil for one that does not.
type linkRefs struct {
	keys  map[string]bool
	notes *noteRefs
}

// noteRefs are the footnote definitions of a file as GitHub or GitLab reads them, and the
// number of each label by its first reference, which the reading of the file's inline
// content in document order assigns (numberNotes).
type noteRefs struct {
	// github is GitHub's reading (cmark-gfm): the first definition of a label, at any
	// depth, defines it. GitLab's (comrak 0.56.0's find_footnote_definitions) reads only a
	// definition that no other holds, and the last of a label defines it.
	github bool
	owner  map[string]*noteDef
	number map[string]int
	next   int
}

// newNoteRefs returns the footnote definitions of defs, which are in the order they
// open, as GitHub reads them when github is true, else as GitLab reads them.
func newNoteRefs(defs []*noteDef, github bool) *noteRefs {
	n := &noteRefs{github: github, owner: map[string]*noteDef{}, number: map[string]int{}}
	for _, d := range defs {
		k := foldLabel(d.label)
		switch {
		case github:
			if n.owner[k] == nil {
				n.owner[k] = d
			}
		case d.parent == nil:
			n.owner[k] = d
		}
	}
	return n
}

// ref reads a footnote reference to label and returns the number its definition takes,
// the next number when this is the definition's first reference, or 0 when no definition
// defines label. A label is compared as a link label is, case-folded with its white space
// collapsed and trimmed.
func (n *noteRefs) ref(label string) int {
	k := foldLabel(label)
	if n.owner[k] == nil {
		return 0
	}
	if n.number[k] == 0 {
		n.next++
		n.number[k] = n.next
	}
	return n.number[k]
}

// home returns where the viewer renders the content of the footnote definition d, the
// innermost that holds it: moved is the number of the definition it is rendered under at
// the end of the document, 0 when it stays in place; shown is false when the viewer drops
// it. GitHub renders a definition that defines its label and is referenced, at the end
// in the order of the numbers, whatever holds it, and drops any other. GitLab keeps a
// definition that another holds inside that one, so the outermost decides: one that
// defines its label is rendered at the end when it is referenced and dropped when it is
// not, and one that does not, a duplicate, stays in place.
func (n *noteRefs) home(d *noteDef) (moved int, shown bool) {
	if !n.github {
		d = d.outermost()
	}
	k := foldLabel(d.label)
	switch {
	case n.owner[k] == d && n.number[k] > 0:
		return n.number[k], true
	case n.owner[k] == d || n.github:
		return 0, false
	}
	return 0, true
}

// defined reports whether a definition of the file defines label.
func (r linkRefs) defined(label string) bool {
	if r.keys == nil || strings.IndexByte(label, '[') >= 0 {
		return false
	}
	return r.keys[foldLabel(label)]
}

// collapseSpace makes each run of white space of s one space, and trims the ends.
func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// foldLabel is CommonMark's case folding of a label, its white space collapsed.
func foldLabel(s string) string { return cases.Fold().String(collapseSpace(s)) }

// lineStarts returns the byte offset of the start of each line of lines joined by line feeds.
func lineStarts(lines []string) []int {
	starts := make([]int, len(lines))
	off := 0
	for i, l := range lines {
		starts[i] = off
		off += len(l) + 1
	}
	return starts
}

// cmDefLines reads the CommonMark definitions at the start of a block, which are the only
// ones CommonMark reads, and returns the labels they define, and for each line whether the
// definitions consume it. A definition is a label, a colon, white space with at most one line
// ending, a destination, then an optional title preceded by white space, and nothing else on
// its line; without a title that reads, the destination ends the line.
func cmDefLines(lines []string) ([]string, []bool) {
	joined := strings.Join(lines, "\n")
	starts := lineStarts(lines)
	consumed := make([]bool, len(lines))
	var labels []string
	for p := 0; p < len(joined); {
		label, end, ok := cmDefAt(joined, p)
		if !ok {
			break
		}
		labels = append(labels, label)
		for i, st := range starts {
			if st >= p && st < end {
				consumed[i] = true
			}
		}
		p = end
	}
	return labels, consumed
}

// cmDefAt reads the definition that begins at p, the start of a line of s, and returns its
// label and the offset after its line ending.
func cmDefAt(s string, p int) (string, int, bool) {
	q := p
	for q-p < 3 && q < len(s) && s[q] == ' ' {
		q++
	}
	if q >= len(s) || s[q] != '[' {
		return "", 0, false
	}
	r := q + 1
	for r < len(s) && s[r] != ']' {
		switch s[r] {
		case '\\':
			r++
		case '[':
			return "", 0, false
		}
		r++
	}
	if r >= len(s) || r+1 >= len(s) || s[r+1] != ':' {
		return "", 0, false
	}
	label := s[q+1 : r]
	if collapseSpace(label) == "" {
		return "", 0, false
	}
	dest := skipLineSpace(s, r+2)
	dend := destinationEnd(s, dest)
	if dend < 0 {
		return "", 0, false
	}
	if p2 := skipLineSpace(s, dend); p2 > dend && p2 < len(s) && (s[p2] == '"' || s[p2] == '\'' || s[p2] == '(') {
		if tend := titleEnd(s, p2); tend >= 0 {
			if e := lineEndAfter(s, tend); e >= 0 {
				return label, e, true
			}
		}
	}
	if e := lineEndAfter(s, dend); e >= 0 {
		return label, e, true
	}
	return "", 0, false
}

// skipLineSpace returns the offset of the first character at or after p that is neither a
// space nor a tab, passing at most one line feed as well.
func skipLineSpace(s string, p int) int {
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

// destinationEnd returns the offset after the destination that begins at p, or -1 when none
// reads: an angle-bracketed one with no line ending in it, or a run of non-space characters.
func destinationEnd(s string, p int) int {
	if p < len(s) && s[p] == '<' {
		for k := p + 1; k < len(s); k++ {
			switch s[k] {
			case '\\':
				k++
			case '>':
				return k + 1
			case '<', '\n':
				return -1
			}
		}
		return -1
	}
	k := p
	for k < len(s) && s[k] > 0x20 && s[k] != 0x7f {
		k++
	}
	if k == p {
		return -1
	}
	return k
}

// titleEnd returns the offset after the title that begins at p, in quotes or parentheses,
// or -1 when it does not close.
func titleEnd(s string, p int) int {
	closer := s[p]
	if closer == '(' {
		closer = ')'
	}
	for k := p + 1; k < len(s); k++ {
		switch s[k] {
		case '\\':
			k++
		case closer:
			return k + 1
		}
	}
	return -1
}

// lineEndAfter returns the offset after the line feed that ends the line of p when only
// spaces and tabs stand between p and it, the end of s counting as a line end, or -1.
func lineEndAfter(s string, p int) int {
	k := p
	for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
		k++
	}
	switch {
	case k == len(s):
		return k
	case s[k] == '\n':
		return k + 1
	}
	return -1
}

// linkDefs returns the labels that the blocks of a file define: a definition is one that
// CommonMark reads at the start of a paragraph, inside a container or not, and is never
// blank: its label has a character that is not white space. The blocks are those of a
// viewer's reading, so under GitHub's and GitLab's a footnote definition is a container
// (blockSyntax.notes), never a link reference definition.
func linkDefs(blocks []unit) linkRefs {
	g := linkRefs{keys: map[string]bool{}}
	for _, u := range blocks {
		if u.html {
			continue
		}
		joined := strings.Join(u.lines, "\n")
		for p := 0; p < len(joined); {
			label, end, ok := cmDefAt(joined, p)
			if !ok {
				break
			}
			g.keys[foldLabel(label)] = true
			p = end
		}
	}
	return g
}
