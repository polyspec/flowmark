package store

import "strings"

// The block structure that the sections of a row depend on, as CommonMark reads
// it at the top level: ATX and setext headings, fenced code blocks, paragraphs,
// and the HTML blocks of types 1 to 5. A line of four or more spaces of indent is
// code, never a heading or a fence.

// heading is an ATX or a setext heading: the first line and the last line of it
// (the underline of a setext heading), its level, and its text.
type heading struct {
	at, last, level int
	text            string
}

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

// setextLevel reads a setext underline: up to three spaces, then only '=' (level 1)
// or only '-' (level 2), then any spaces or tabs. It returns 0 for any other line.
func setextLevel(line string) int {
	s := indentOf(line)
	if s > 3 {
		return 0
	}
	t := strings.TrimRight(line[s:], " \t")
	switch {
	case t == "":
		return 0
	case strings.Trim(t, "=") == "":
		return 1
	case strings.Trim(t, "-") == "":
		return 2
	}
	return 0
}

// isListItem reports whether line opens a list item: up to three spaces, a
// '-' and then a space or the end of the line.
func isListItem(line string) bool {
	s := indentOf(line)
	return s <= 3 && s < len(line) && line[s] == '-' && (s+1 == len(line) || line[s+1] == ' ')
}

// isBlank reports whether line holds only spaces and tabs, or nothing.
func isBlank(line string) bool { return trimWhite(line) == "" }

// indented reports whether line begins with two or more columns of white space,
// spaces or a tab.
func indented(line string) bool {
	return strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")
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

// scanHeadings returns the headings of lines from index from on, outside fenced
// blocks, and whether a fenced block is still open at the end. A setext heading
// is the whole paragraph before its underline; a paragraph is a run of lines
// that are neither blank, nor a heading, a fence or a list item.
func scanHeadings(lines []string, from int) ([]heading, bool) {
	var hs []heading
	var open fenceMark
	inFence := false
	para := -1 // the first line of the open paragraph, or -1
	for i := from; i < len(lines); i++ {
		line := lines[i]
		if inFence {
			if closesFence(line, open) {
				inFence = false
			}
			continue
		}
		if f, ok := fenceOpenAt(line); ok {
			open, inFence, para = f, true, -1
			continue
		}
		if level, text, ok := headingOf(line); ok {
			hs = append(hs, heading{at: i, last: i, level: level, text: text})
			para = -1
			continue
		}
		if para >= 0 {
			if level := setextLevel(line); level > 0 {
				hs = append(hs, heading{at: para, last: i, level: level, text: setextText(lines[para:i])})
				para = -1
				continue
			}
		}
		switch {
		case isBlank(line), isListItem(line):
			para = -1
		case para < 0 && indentOf(line) < 4:
			para = i
		}
	}
	return hs, inFence
}

// setextText is the text of a setext heading: its lines without their indent and
// their trailing white space, joined by line feeds.
func setextText(lines []string) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = strings.TrimRight(strings.TrimLeft(l, " "), " \t")
	}
	return strings.Join(parts, "\n")
}

// boundary returns the index of the first heading of level 2 or less at or after
// from, outside fenced blocks, or len(lines) when there is none. When there is
// none, it also reports whether a fenced block is still open at the end.
func boundary(lines []string, from int) (int, bool) {
	hs, open := scanHeadings(lines, from)
	for _, h := range hs {
		if h.level <= 2 {
			return h.at, false
		}
	}
	return len(lines), open
}

// htmlStart reads the start of an HTML block of CommonMark types 1 to 5 and
// returns its type, or 0 when line starts none.
func htmlStart(line string) int {
	s := indentOf(line)
	if s > 3 || s >= len(line) || line[s] != '<' {
		return 0
	}
	rest := line[s:]
	lower := strings.ToLower(rest)
	for _, name := range []string{"script", "pre", "style", "textarea"} {
		if strings.HasPrefix(lower, "<"+name) {
			after := lower[len(name)+1:]
			if after == "" || after[0] == ' ' || after[0] == '\t' || after[0] == '>' {
				return 1
			}
		}
	}
	switch {
	case strings.HasPrefix(rest, "<!--"):
		return 2
	case strings.HasPrefix(rest, "<?"):
		return 3
	case strings.HasPrefix(rest, "<![CDATA["):
		return 5
	case strings.HasPrefix(rest, "<!") && len(rest) > 2 && isAlnum(rest[2]):
		return 4
	}
	return 0
}

// htmlEnds reports whether line holds the end condition of an HTML block of type kind.
func htmlEnds(kind int, line string) bool {
	switch kind {
	case 1:
		lower := strings.ToLower(line)
		for _, end := range []string{"</script>", "</pre>", "</style>", "</textarea>"} {
			if strings.Contains(lower, end) {
				return true
			}
		}
	case 2:
		return strings.Contains(line, "-->")
	case 3:
		return strings.Contains(line, "?>")
	case 4:
		return strings.Contains(line, ">")
	case 5:
		return strings.Contains(line, "]]>")
	}
	return false
}

// htmlOpen reports whether the lines leave an HTML block of types 1 to 5 open at
// their end. Such a block takes in the element that follows it.
func htmlOpen(lines []string) bool {
	kind := 0
	var fence fenceMark
	inFence := false
	for _, line := range lines {
		switch {
		case kind != 0:
			if htmlEnds(kind, line) {
				kind = 0
			}
		case inFence:
			if closesFence(line, fence) {
				inFence = false
			}
		default:
			if k := htmlStart(line); k != 0 {
				if !htmlEnds(k, line) {
					kind = k
				}
			} else if f, ok := fenceOpenAt(line); ok {
				fence, inFence = f, true
			}
		}
	}
	return kind != 0
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
