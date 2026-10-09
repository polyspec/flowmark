// Tests of the frontmatter profile: the YAML 1.2 core subset with a canonical emission.
package codec

import (
	"strings"
	"testing"
)

func parseFrontmatter(t *testing.T, text string) (*Document, []Diagnostic) {
	t.Helper()
	document := "<!-- doc-id: a -->\n# T\n\n---\n" + text + "---\n"
	return Parse(document)
}

func TestFrontmatterRoundTrip(t *testing.T) {
	const source = "table: checklists\n" +
		"version: 3\n" +
		"title: \"A quoted: title\"\n" +
		"tags:\n" +
		"  - spec\n" +
		"  - codec\n" +
		"fragment: |\n" +
		"  table checklists {}\n" +
		"  column title text\n"
	document, diagnostics := parseFrontmatter(t, source)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics: %v", diagnostics)
	}
	rendered := Render(document)
	if !strings.Contains(rendered, source) {
		t.Fatalf("round-trip failed:\n%q", rendered)
	}
	again, _ := Parse(rendered)
	if Render(again) != rendered {
		t.Fatal("P2 failed")
	}
}

func TestFrontmatterRules(t *testing.T) {
	cases := []struct {
		name string
		text string
		rule string
		line int
	}{
		{"comment", "# a comment\nkey: value\n", RuleFrontmatter, 5},
		{"duplicate key", "key: a\nkey: b\n", RuleFrontmatter, 6},
		{"tab indent", "key:\n\t- a\n", RuleFrontmatter, 6},
		{"flow style", "key: {a: 1}\n", RuleFrontmatter, 5},
		{"anchor", "key: &anchor value\n", RuleFrontmatter, 5},
	}
	for _, testCase := range cases {
		_, diagnostics := parseFrontmatter(t, testCase.text)
		found := false
		for _, diagnostic := range diagnostics {
			if diagnostic.Rule == testCase.rule && diagnostic.Line == testCase.line {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: diagnostics: %v", testCase.name, diagnostics)
		}
	}
}

func TestFrontmatterCanonicalizes(t *testing.T) {
	document, diagnostics := parseFrontmatter(t, "key:    value\n")
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics: %v", diagnostics)
	}
	if rendered := Render(document); !strings.Contains(rendered, "key: value\n") {
		t.Fatalf("not canonical: %q", rendered)
	}
}
