package codec

import (
	"fmt"
	"regexp"
	"strings"
)

// Entry is one frontmatter key with its value: a scalar, a sequence of scalars or a literal
// block (the YAML 1.2 core subset of spec 10-canonical-format — no comments, no flow style,
// no anchors).
type Entry struct {
	Key     string
	Scalar  string
	Quoted  bool // the source wrote the scalar in double quotes
	Items   []string
	Literal []string
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ParseFrontmatter reads the lines inside the frontmatter fence. `baseLine` is the 1-based
// document line of the first inside line, so the diagnostics carry document coordinates.
func ParseFrontmatter(lines []string, baseLine int) ([]Entry, []Diagnostic) {
	entries := []Entry{}
	diagnostics := []Diagnostic{}
	report := func(i int, message string) {
		diagnostics = append(diagnostics, Diagnostic{Line: baseLine + i, Column: 1, Rule: RuleFrontmatter, Message: message})
	}
	seen := map[string]bool{}
	at := 0
	for at < len(lines) {
		line := lines[at]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			at++
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			report(at, "the frontmatter profile rejects comments")
			at++
			continue
		}
		if strings.Contains(line[:len(line)-len(strings.TrimLeft(line, " "))], "\t") || strings.HasPrefix(line, "\t") {
			report(at, "the frontmatter indents with spaces, not tabs")
			at++
			continue
		}
		key, rest, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if !ok || !keyPattern.MatchString(key) {
			report(at, fmt.Sprintf("the line %q is a mapping `key: value`", trimmed))
			at++
			continue
		}
		if seen[key] {
			report(at, fmt.Sprintf("the key %q appears twice", key))
			at++
			continue
		}
		seen[key] = true
		rest = strings.TrimSpace(rest)
		entry := Entry{Key: key}
		switch {
		case rest == "|":
			// A literal block: the indented lines below, dedented by their common indent.
			at++
			block := []string{}
			for at < len(lines) {
				next := lines[at]
				if strings.TrimSpace(next) == "" {
					block = append(block, "")
					at++
					continue
				}
				if !strings.HasPrefix(next, "  ") {
					break
				}
				block = append(block, next)
				at++
			}
			entry.Literal = dedent(block)
		case rest == "":
			// A sequence: `- item` lines indented by two spaces.
			at++
			for at < len(lines) {
				next := lines[at]
				if strings.HasPrefix(next, "  - ") {
					entry.Items = append(entry.Items, scalarOf(strings.TrimSpace(next[4:])))
					at++
					continue
				}
				if strings.TrimSpace(next) == "" {
					at++
					continue
				}
				break
			}
		default:
			if strings.HasPrefix(rest, "&") || strings.HasPrefix(rest, "*") {
				report(at, "the frontmatter profile rejects anchors and aliases")
			} else if strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "[") {
				report(at, "the frontmatter profile rejects the flow style")
			} else {
				entry.Quoted = strings.HasPrefix(rest, `"`)
				entry.Scalar = scalarOf(rest)
			}
			at++
		}
		entries = append(entries, entry)
	}
	return entries, diagnostics
}

// scalarOf unwraps a double-quoted scalar and leaves the rest as written.
func scalarOf(value string) string {
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) && len(value) >= 2 {
		return value[1 : len(value)-1]
	}
	return value
}

// dedent removes the common leading spaces of a literal block.
func dedent(lines []string) []string {
	first := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		first = len(line) - len(strings.TrimLeft(line, " "))
		break
	}
	if first <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if len(line) >= first {
			out[i] = line[first:]
		}
	}
	return out
}

// RenderFrontmatter emits the canonical form: one space after the colon, two-space sequence
// indents and two-space literal indents.
func RenderFrontmatter(entries []Entry) string {
	var out strings.Builder
	for _, entry := range entries {
		switch {
		case entry.Items != nil:
			out.WriteString(entry.Key + ":\n")
			for _, item := range entry.Items {
				out.WriteString("  - " + emitScalar(item, false) + "\n")
			}
		case entry.Literal != nil:
			out.WriteString(entry.Key + ": |\n")
			for _, line := range entry.Literal {
				if line == "" {
					out.WriteString("\n")
					continue
				}
				out.WriteString("  " + line + "\n")
			}
		default:
			out.WriteString(entry.Key + ": " + emitScalar(entry.Scalar, entry.Quoted) + "\n")
		}
	}
	return out.String()
}

// emitScalar writes a scalar plainly, or double-quoted when the source quoted it or the plain
// form would not survive a re-parse.
func emitScalar(value string, quoted bool) string {
	needsQuote := quoted || value == "" || strings.ContainsAny(value, ":#") || strings.TrimSpace(value) != value
	if !needsQuote {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
