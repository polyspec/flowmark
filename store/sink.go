package store

import (
	"bytes"
	"unicode/utf8"
)

// sink collects the diagnostics of one file. A fatal diagnostic makes the file
// unreadable; a warning marks a noncanonical file whose rows are still returned.
type sink struct {
	rel   string
	diags []Diagnostic
	fatal bool
}

func (s *sink) fail(line, col int, rule, msg string) {
	s.fatal = true
	s.diags = append(s.diags, Diagnostic{File: s.rel, Line: line, Column: col, Rule: rule, Message: msg})
}

func (s *sink) warn(line, col int, rule, msg string) {
	s.diags = append(s.diags, Diagnostic{File: s.rel, Line: line, Column: col, Rule: rule, Message: msg})
}

// normalize checks the encoding and returns the text with LF line endings and
// a final line feed. Each departure from the canonical bytes is a diagnostic.
func (s *sink) normalize(data []byte) (string, bool) {
	data = bytes.Clone(data)
	if !utf8.Valid(data) {
		line, col := invalidAt(data)
		s.fail(line, col, RuleEncoding, "the file is not UTF-8")
		return "", false
	}
	if i := bytes.IndexByte(data, 0); i >= 0 {
		s.fail(lineOf(data, i), colOf(data, i), RuleEncoding, "the file holds a U+0000")
		return "", false
	}
	if i := bytes.IndexByte(data, '\r'); i >= 0 {
		s.warn(lineOf(data, i), colOf(data, i), RuleNoncanonical, "a CR; the canonical line ending is LF")
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	}
	if bytes.HasPrefix(data, bom) {
		s.warn(1, 1, RuleNoncanonical, "the file begins with a byte order mark; the canonical file has none")
		data = data[len(bom):]
	}
	if n := len(data); n > 0 && data[n-1] != '\n' {
		s.warn(lineOf(data, n), colOf(data, n), RuleNoncanonical, "the last line has no line feed")
		data = append(data, '\n')
	}
	return string(data), true
}

// valueLines returns the lines of the value between from and to, and the index
// of its first line. The element that follows the value is at to, unless to is
// the end of the file. A value that stands after a heading (framed) has one
// blank line after the heading; every value has one blank line before the
// element that follows it. Any other blank line at either end of the value is
// not part of it and is noncanonical.
func (s *sink) valueLines(lines []string, from, to int, framed bool) ([]string, int) {
	end := to
	if to < len(lines) && end > from && lines[end-1] == "" {
		end--
	}
	if framed && end-from > 1 && lines[from] == "" {
		from++
	}
	start := from
	for start < end && isBlank(lines[start]) {
		start++
	}
	if start > from {
		s.warn(from+1, 1, RuleNoncanonical, "a blank line at the start of the value")
	}
	stop := end
	for stop > start && isBlank(lines[stop-1]) {
		stop--
	}
	if stop < end {
		s.warn(stop+1, 1, RuleNoncanonical, "a blank line at the end of the value")
	}
	return lines[start:stop], start
}
