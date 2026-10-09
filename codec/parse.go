package codec

import (
	"fmt"
	"regexp"
	"strings"
)

// headerComment matches the header comments: doc-id (required, first), source-sha256 (the
// twin) and owner (the provenance of an adapter-owned document).
var headerComment = regexp.MustCompile(`^<!-- (doc-id|source-sha256|owner): (.+) -->$`)

// fenceLine matches the opening or closing line of a fenced code block.
var fenceLine = regexp.MustCompile(`^\s*(` + "```" + `|~~~)`)

// Parse reads one document strictly into its skeleton. A rule violation is a located
// diagnostic and the parse continues best-effort (spec 10-canonical-format). A fenced block
// is payload: the structural rules look at unfenced lines only.
func Parse(text string) (*Document, []Diagnostic) {
	diagnostics := []Diagnostic{}
	report := func(line int, rule, message string) {
		diagnostics = append(diagnostics, Diagnostic{Line: line, Column: 1, Rule: rule, Message: message})
	}

	if strings.HasPrefix(text, "\uFEFF") {
		text = strings.TrimPrefix(text, "\uFEFF")
		report(1, RuleEncoding, "the document starts with a byte order mark")
	}
	// P2: CRLF is valid input; it breaks P1, so the parse reports it as non-canonical.
	if i := strings.IndexByte(text, '\r'); i >= 0 {
		line := 1 + strings.Count(text[:i], "\n")
		report(line, RuleNonCanonical, "the document carries a carriage return; the canonical form uses LF")
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}

	lines := strings.Split(text, "\n")
	fenced := scanFences(lines)
	document := &Document{}
	at := 0

	// The header comments: a contiguous run at the top of the file.
	for at < len(lines) {
		match := headerComment.FindStringSubmatch(lines[at])
		if match == nil {
			break
		}
		kind, value := match[1], match[2]
		switch kind {
		case "doc-id":
			if document.DocID != "" {
				report(at+1, RuleHeader, fmt.Sprintf("the doc-id %q appears twice", value))
			} else {
				document.DocID = value
			}
		case "source-sha256":
			document.TwinSHA = value
		case "owner":
			document.Owner = value
		}
		at++
	}
	if document.DocID == "" {
		report(1, RuleHeader, "the document names no doc-id; the header comment is required")
	}
	if at > 0 && at < len(lines) && lines[at] == "" {
		report(at+1, RuleHeader, "the title follows the header comments without a blank line")
	}
	for at < len(lines) && lines[at] == "" {
		at++
	}

	// The title: exactly one level-1 ATX heading.
	if at >= len(lines) || !strings.HasPrefix(lines[at], "# ") {
		report(at+1, RuleSection, "the document title is a level-1 ATX heading")
	} else {
		document.Title = lines[at][2:]
		at++
	}
	for at < len(lines) && lines[at] == "" {
		at++
	}

	// The preamble: the payload up to the frontmatter fence or the first section heading. A
	// level-1 heading here is a second title and is reported where it stands.
	for at < len(lines) && (fenced[at] || !isBoundary(lines[at])) {
		if !fenced[at] && strings.HasPrefix(lines[at], "# ") {
			report(at+1, RuleSection, "a level-1 heading appears after the title")
		}
		document.Preamble = append(document.Preamble, lines[at])
		at++
	}
	document.Preamble = trimBlank(document.Preamble)

	// The frontmatter block (optional): one pair of `---` fences. An unclosed fence reports
	// the line of its opening.
	if at < len(lines) && lines[at] == "---" {
		open := at
		at++
		for at < len(lines) && lines[at] != "---" {
			document.Frontmatter = append(document.Frontmatter, lines[at])
			at++
		}
		if at >= len(lines) {
			report(open+1, RuleFrontmatter, "the frontmatter block has no closing fence")
		} else {
			at++
		}
		for at < len(lines) && lines[at] == "" {
			at++
		}
	}

	// The sections: each H2 or H3 heading with the payload under it. A blank line inside a
	// section is payload when content follows it; the blank run before the next heading is
	// skeleton.
	for at < len(lines) {
		level, heading, ok := sectionHeading(lines[at])
		if !ok {
			report(at+1, RuleSection, fmt.Sprintf("the section heading %q is a level-2 or level-3 ATX heading", lines[at]))
			at++
			continue
		}
		section := Section{Level: level, Heading: heading}
		at++
		for at < len(lines) {
			for at < len(lines) && (fenced[at] || (lines[at] != "" && !isHeadingLine(lines[at]))) {
				section.Lines = append(section.Lines, lines[at])
				at++
			}
			if at >= len(lines) || (!fenced[at] && isHeadingLine(lines[at])) {
				break
			}
			// A blank line: skeleton when a heading or the end follows it, payload otherwise.
			next := at
			for next < len(lines) && lines[next] == "" {
				next++
			}
			if next >= len(lines) || (!fenced[next] && isHeadingLine(lines[next])) {
				at = next
				break
			}
			section.Lines = append(section.Lines, "")
			at++
		}
		section.Lines = trimBlank(section.Lines)
		document.Sections = append(document.Sections, section)
	}
	return document, diagnostics
}

// scanFences marks every line that is a fence line or lies inside a fenced block. A closing
// fence carries no information string; an opening fence may.
func scanFences(lines []string) []bool {
	fenced := make([]bool, len(lines))
	open := ""
	for i, line := range lines {
		if open == "" {
			if match := fenceLine.FindStringSubmatch(line); match != nil {
				open = match[1]
				fenced[i] = true
			}
		} else if match := fenceLine.FindStringSubmatch(line); match != nil && match[1][0] == open[0] && len(match[1]) >= len(open) && strings.TrimSpace(line) == match[1] {
			open = ""
			fenced[i] = true
		} else {
			fenced[i] = true
		}
	}
	return fenced
}

// isBoundary names where the preamble ends: the frontmatter fence or a section heading.
func isBoundary(line string) bool {
	_, _, ok := sectionHeading(line)
	return line == "---" || ok
}

// sectionHeading reads a level-2 or level-3 ATX heading.
func sectionHeading(line string) (level int, heading string, ok bool) {
	if strings.HasPrefix(line, "## ") {
		return 2, line[3:], true
	}
	if strings.HasPrefix(line, "### ") {
		return 3, line[4:], true
	}
	return 0, "", false
}

// isHeadingLine names any line the section content stops at.
func isHeadingLine(line string) bool {
	return strings.HasPrefix(line, "#")
}

// trimBlank drops the leading and trailing blank lines and keeps the inner ones.
func trimBlank(lines []string) []string {
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
