package store

import (
	"regexp"
	"strings"

	"golang.org/x/text/cases"
)

// The link reference definitions that decide the text of a heading under each fold
// (spec 10, Detail headings and anchors). F_G reads a definition as CommonMark 0.31.2
// does: a block that starts a paragraph, inside a container or not, and that never
// interrupts one. F_M reads a definition as Python-Markdown does: a line of any block
// that matches its pattern. A label is compared once its white space is collapsed,
// by CommonMark's case folding under F_G and by lowercasing under F_M.

// pmDefRE is the definition that Python-Markdown's reference processor searches a block
// for: up to three spaces, a label with no bracket in it, a colon, an optional line ending,
// a destination, an optional title in quotes or parentheses, and nothing else on its line.
var pmDefRE = regexp.MustCompile(`(?m)^ {0,3}\[([^\[\]]*)\]: *\n? *(\S+) *(?:\n *)?(?:"[^\n]*" *|'[^\n]*' *|\([^\n]*\) *)?$`)

// wsRE is a run of white space, which Python-Markdown collapses in a reference label.
var wsRE = regexp.MustCompile(`\s+`)

// linkRefs are the labels that the definitions of a file define under one fold: keys
// holds each defined label under the fold's key, and lookup gives the key of a reference
// label. Under F_M one white space may stand between a text and its label.
type linkRefs struct {
	keys   map[string]bool
	lookup func(string) string
	space  bool
}

// defined reports whether a definition of the fold defines label.
func (r linkRefs) defined(label string) bool {
	if r.keys == nil || strings.IndexByte(label, '[') >= 0 {
		return false
	}
	return r.keys[r.lookup(label)]
}

// collapseSpace makes each run of white space of s one space, and trims the ends.
func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// foldLabelG is CommonMark's case folding of a label, its white space collapsed.
func foldLabelG(s string) string { return cases.Fold().String(collapseSpace(s)) }

// lookupM is Python-Markdown's reference lookup: each run of white space of the label
// becomes one space, and the label is lowercased, without any trimming of its ends.
func lookupM(s string) string { return strings.ToLower(wsRE.ReplaceAllString(s, " ")) }

// defKeyM is the key of a definition under F_M: the label without the white space at its
// ends, lowercased.
func defKeyM(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

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

// mDefLines reports, for each line of a block, whether Python-Markdown reads the line as
// part of a definition, which removes it from the text of the block. A definition may run
// over several lines, and each line it starts is one that it removes.
func mDefLines(lines []string) []bool {
	out := make([]bool, len(lines))
	joined := strings.Join(lines, "\n")
	starts := lineStarts(lines)
	for _, loc := range pmDefRE.FindAllStringIndex(joined, -1) {
		for i, st := range starts {
			if st >= loc[0] && st < loc[1] {
				out[i] = true
			}
		}
	}
	return out
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

// linkDefs returns the labels that the blocks of a file define under F_G and under F_M.
// Under F_G a definition is one that CommonMark reads at the start of a paragraph, inside
// a container or not; under F_M it is one that Python-Markdown's pattern finds in any block
// line. A definition is never blank: its label has a character that is not white space.
func linkDefs(blocks []unit) (linkRefs, linkRefs) {
	g := linkRefs{keys: map[string]bool{}, lookup: foldLabelG}
	m := linkRefs{keys: map[string]bool{}, lookup: lookupM, space: true}
	for _, u := range blocks {
		if u.html {
			continue
		}
		cm, _ := cmDefLines(u.lines)
		for _, label := range cm {
			g.keys[foldLabelG(label)] = true
		}
		joined := strings.Join(u.lines, "\n")
		for _, mm := range pmDefRE.FindAllStringSubmatch(joined, -1) {
			if key := defKeyM(mm[1]); key != "" {
				m.keys[key] = true
			}
		}
	}
	return g, m
}
