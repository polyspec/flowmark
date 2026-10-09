package codec

import (
	"fmt"
	"regexp"
	"strings"
)

// headerComment matches the header comments: doc-id (required, first), source-sha256 (the
// twin) and owner (the provenance of an adapter-owned document).
var headerComment = regexp.MustCompile(`^<!-- (doc-id|source-sha256|owner): (.+) -->$`)

// Parse reads one document strictly into its skeleton. A rule violation is a located
// diagnostic and the parse continues best-effort (the T2.1 scope of spec 10-canonical-format).
func Parse(text string) (*Document, []Diagnostic) {
	diagnostics := []Diagnostic{}
	report := func(line int, rule, message string) {
		diagnostics = append(diagnostics, Diagnostic{Line: line, Rule: rule, Message: message})
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
	if at > 0 && lines[at] != "" {
		report(at+1, RuleHeader, "a blank line separates the header comments from the title")
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

	// The preamble: the payload up to the frontmatter fence or the first H2 heading.
	for at < len(lines) && !isBoundary(lines[at]) {
		document.Preamble = append(document.Preamble, lines[at])
		at++
	}
	document.Preamble = trimBlank(document.Preamble)

	// The frontmatter block (optional): one pair of `---` fences.
	if at < len(lines) && lines[at] == "---" {
		at++
		for at < len(lines) && lines[at] != "---" {
			document.Frontmatter = append(document.Frontmatter, lines[at])
			at++
		}
		if at >= len(lines) {
			report(at, RuleFrontmatter, "the frontmatter block has no closing fence")
		} else {
			at++
		}
		for at < len(lines) && lines[at] == "" {
			at++
		}
	}

	// The H2 sections: each heading with the payload under it. A blank line inside a section
	// is payload when content follows it; the blank run before the next heading is skeleton.
	for at < len(lines) {
		if !strings.HasPrefix(lines[at], "## ") {
			report(at+1, RuleSection, fmt.Sprintf("the section heading %q is a level-2 ATX heading", lines[at]))
			at++
			continue
		}
		section := Section{Heading: lines[at][3:]}
		at++
		for at < len(lines) {
			for at < len(lines) && lines[at] != "" && !strings.HasPrefix(lines[at], "#") {
				section.Lines = append(section.Lines, lines[at])
				at++
			}
			if at >= len(lines) || strings.HasPrefix(lines[at], "#") {
				break
			}
			// A blank line: skeleton when a heading or the end follows it, payload otherwise.
			next := at
			for next < len(lines) && lines[next] == "" {
				next++
			}
			if next >= len(lines) || strings.HasPrefix(lines[next], "#") {
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

// isBoundary names where the preamble ends: the frontmatter fence or an H2 heading.
func isBoundary(line string) bool {
	return line == "---" || strings.HasPrefix(line, "## ")
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
