package store

import "strings"

// The block structure that the sections of a row depend on: ATX headings and
// fenced code blocks, as CommonMark reads them at the top level. A line of
// four or more spaces of indent is code, never a heading or a fence.

// headingOf reads an ATX heading: up to three spaces, one to six '#', then a
// space and the text, or the end of the line. The text is the rest of the line.
func headingOf(line string) (level int, text string, ok bool) {
	s := indentOf(line)
	if s > 3 {
		return 0, "", false
	}
	h := s
	for h < len(line) && line[h] == '#' {
		h++
	}
	level = h - s
	if level < 1 || level > 6 {
		return 0, "", false
	}
	if h == len(line) {
		return level, "", true
	}
	if line[h] != ' ' {
		return 0, "", false
	}
	return level, line[h+1:], true
}

// isListItem reports whether line opens a list item: up to three spaces, a
// '-' and then a space or the end of the line.
func isListItem(line string) bool {
	s := indentOf(line)
	return s <= 3 && s < len(line) && line[s] == '-' && (s+1 == len(line) || line[s+1] == ' ')
}

// fenceMark is an open fenced block: its character and the length of its opening run.
type fenceMark struct {
	ch byte
	n  int
}

// fenceOpenAt reads an opening fence: up to three spaces, at least three
// backticks or tildes, and for backticks no backtick in the info string.
func fenceOpenAt(line string) (fenceMark, bool) {
	s := indentOf(line)
	if s > 3 || s >= len(line) || (line[s] != '`' && line[s] != '~') {
		return fenceMark{}, false
	}
	ch := line[s]
	n := runOf(line[s:], ch)
	if n < 3 {
		return fenceMark{}, false
	}
	if ch == '`' && strings.Contains(line[s+n:], "`") {
		return fenceMark{}, false
	}
	return fenceMark{ch: ch, n: n}, true
}

// closesFence reports whether line closes the open fence: up to three spaces,
// a run of its character at least as long, and nothing but spaces after it.
func closesFence(line string, f fenceMark) bool {
	s := indentOf(line)
	if s > 3 || s >= len(line) || line[s] != f.ch {
		return false
	}
	n := runOf(line[s:], f.ch)
	return n >= f.n && strings.Trim(line[s+n:], " ") == ""
}

// indentOf counts the leading spaces of line.
func indentOf(line string) int {
	s := 0
	for s < len(line) && line[s] == ' ' {
		s++
	}
	return s
}

// runOf counts the leading run of ch in s.
func runOf(s string, ch byte) int {
	n := 0
	for n < len(s) && s[n] == ch {
		n++
	}
	return n
}

// boundary returns the index of the first line at or after from that is an
// ATX heading of level 2 or less outside a fenced block, or len(lines) when
// there is none. It also reports whether a fenced block is still open at the
// returned index. A heading is never returned inside a fence.
func boundary(lines []string, from int) (int, bool) {
	var open fenceMark
	inFence := false
	for i := from; i < len(lines); i++ {
		line := lines[i]
		if inFence {
			if closesFence(line, open) {
				inFence = false
			}
			continue
		}
		if f, ok := fenceOpenAt(line); ok {
			open, inFence = f, true
			continue
		}
		if level, _, ok := headingOf(line); ok && level <= 2 {
			return i, false
		}
	}
	return len(lines), inFence
}

// splitLines splits text, which ends with a line feed or is empty, into its
// lines without the line feeds.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// joinLines writes lines as text, each one terminated by a line feed.
func joinLines(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}
