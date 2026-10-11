package store

import (
	"strings"
)

// The attribute lists of the collision rule (spec 10, Detail headings and anchors).
// Python-Markdown's attr_list extension reads one list from the text of an element, at
// the place the rule names, and the ids that its tokens set are compared with the anchors.
// Python-Markdown expands the tabs before it parses, so a tab reads as a space here.

// textPiece is a run of an element's text on one line of the file: the 0-based index of
// the line, the byte offset in that line where the run starts, and the run's text.
type textPiece struct {
	line, start int
	text        string
}

// element is the text that the rule reads as one unit: a heading, a paragraph segment, a
// table cell or an HTML block. Its pieces are its lines, and text joins their texts with
// line feeds, so an offset of text is located in the file by its piece.
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

// attrToken is a token of an attribute list that sets a key, spelled as written.
type attrToken struct {
	key, value string
}

// readList reads the attribute list that begins at s[k], a '{', and returns its ids and
// the index of its closing '}'. The list is '{', an optional ':', optional spaces, and
// then tokens separated by spaces up to a '}' on the same line, its first token beginning
// with neither a space nor a '}'. A token '#A' sets the key id to A; a token key=A, key="A"
// or key='A' sets its key, where a quoted value holds every character up to its closing
// quote. For each key spelling the last token wins, and the values of the keys id and name,
// in any case, are the ids the list sets.
func readList(s string, k int) ([]string, int, bool) {
	p := k + 1
	if p < len(s) && s[p] == ':' {
		p++
	}
	for p < len(s) && s[p] == ' ' {
		p++
	}
	if p >= len(s) || s[p] == '}' || s[p] == '\n' {
		return nil, 0, false
	}
	var toks []attrToken
	for p < len(s) {
		switch c := s[p]; {
		case c == '}':
			return idValues(toks), p, true
		case c == '\n':
			return nil, 0, false
		case c == ' ':
			p++
		case c == '#':
			q := tokenEnd(s, p)
			if q-p > 1 {
				toks = append(toks, attrToken{key: "id", value: s[p+1 : q]})
			}
			p = q
		default:
			q := p
			for q < len(s) && !strings.ContainsRune(" }\n=", rune(s[q])) {
				q++
			}
			if q == p {
				p++
				continue
			}
			if q == len(s) || s[q] != '=' {
				p = q
				continue
			}
			key := s[p:q]
			q++
			var value string
			// A quoted value closes before the last '}' of the line, as Python-Markdown's
			// pattern for the list reads; a quote that no quote closes is an ordinary character.
			end := strings.LastIndexByte(s, '}')
			if q < len(s) && (s[q] == '"' || s[q] == '\'') && q+1 <= end {
				if e := strings.IndexByte(s[q+1:end], s[q]); e >= 0 {
					value = s[q+1 : q+1+e]
					q += e + 2
				} else {
					v := q
					q = tokenEnd(s, q)
					value = s[v:q]
				}
			} else {
				v := q
				q = tokenEnd(s, q)
				value = s[v:q]
			}
			toks = append(toks, attrToken{key: key, value: value})
			p = q
		}
	}
	return nil, 0, false
}

// tokenEnd returns the index of the first space, '}' or line feed of s at or after p.
func tokenEnd(s string, p int) int {
	for p < len(s) && s[p] != ' ' && s[p] != '}' && s[p] != '\n' {
		p++
	}
	return p
}

// idValues returns the values that toks set for the keys id and name, in any case: the
// last token of each key spelling sets its value, and the values come in token order.
func idValues(toks []attrToken) []string {
	last := map[string]int{}
	for i, t := range toks {
		if strings.EqualFold(t.key, "id") || strings.EqualFold(t.key, "name") {
			last[t.key] = i
		}
	}
	var ids []string
	for i, t := range toks {
		if j, ok := last[t.key]; ok && j == i {
			ids = append(ids, t.value)
		}
	}
	return ids
}

// attrClashes returns the collisions of the attribute lists of the file with the anchors,
// over every line, code included. Every unescaped `{` on a line that a list reads from
// sets its ids: the lists are compared whatever element Python-Markdown would attach them
// to, so a list in code is refused as well, and a `{` that an odd run of backslashes
// precedes is an escaped brace, which is no list.
func attrClashes(lines []string, anchors []detailAnchor) []anchorClash {
	var out []anchorClash
	for i, l := range lines {
		t := strings.ReplaceAll(l, "\t", " ")
		for k := 0; k < len(t); k++ {
			if t[k] != '{' || oddBackslashesBefore(t, k) {
				continue
			}
			ids, _, ok := readList(t, k)
			if !ok {
				continue
			}
			for _, id := range ids {
				for _, a := range anchors {
					if strings.EqualFold(id, a.anchor) {
						out = append(out, anchorClash{line: i + 1, at: k, text: id, other: a.text})
					}
				}
			}
		}
	}
	return out
}

// oddBackslashesBefore reports whether an odd number of backslashes stands directly before
// the byte at k of t, so that the byte is escaped.
func oddBackslashesBefore(t string, k int) bool {
	n := 0
	for j := k - 1; j >= 0 && t[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// htmlElement returns the element of an HTML block of the file, whose lines are its text.
func htmlElement(file []string, u unit) element {
	pieces := make([]textPiece, len(u.lines))
	for k, l := range u.lines {
		pieces[k] = textPiece{line: u.at + k, start: lineBase(file[u.at+k], l), text: l}
	}
	return newElement(pieces)
}
