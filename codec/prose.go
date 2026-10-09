package codec

import "strings"

// Line is one prose line of a document: outside fenced blocks, with the inline code spans
// blanked so columns keep their place.
type Line struct {
	Number int
	Text   string
}

// Prose returns the prose lines of `text`: the unfenced lines with the inline code spans
// replaced by spaces. The reference grammar reads these lines (spec 10-canonical-format).
func Prose(text string) []Line {
	lines := strings.Split(text, "\n")
	fenced := scanFences(lines)
	out := make([]Line, 0, len(lines))
	for i, line := range lines {
		if fenced[i] {
			continue
		}
		out = append(out, Line{Number: i + 1, Text: blankInlineCode(line)})
	}
	return out
}

// blankInlineCode replaces each inline code span — an opening backtick run, its content and
// the closing run of at least the same length — with spaces, keeping the line length.
func blankInlineCode(line string) string {
	out := []byte(line)
	at := 0
	for at < len(out) {
		if out[at] != '`' {
			at++
			continue
		}
		open := at
		for at < len(out) && out[at] == '`' {
			at++
		}
		run := at - open
		closing := -1
		for scan := at; scan < len(out); {
			if out[scan] != '`' {
				scan++
				continue
			}
			closeStart := scan
			for scan < len(out) && out[scan] == '`' {
				scan++
			}
			if scan-closeStart >= run {
				closing = scan
				break
			}
		}
		if closing < 0 {
			continue // an unmatched run stays as it is
		}
		for b := open; b < closing; b++ {
			out[b] = ' '
		}
		at = closing
	}
	return string(out)
}
