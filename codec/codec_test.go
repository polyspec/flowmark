// Tests of the codec package: the skeleton round-trip (P1, P2) and the header diagnostics.
package codec

import (
	"strings"
	"testing"
)

// Canonical input: P1 — emit(parse(s)) == s.
const canonical = "<!-- doc-id: plans-design-plan -->\n" +
	"# Design Plan\n" +
	"\n" +
	"[Korean](design-plan.ko.md)\n" +
	"\n" +
	"The preamble prose, preserved as payload.\n" +
	"Second line of the preamble.\n" +
	"\n" +
	"## Scope\n" +
	"\n" +
	"One section.\n" +
	"\n" +
	"```\nfenced payload\n```\n" +
	"\n" +
	"## Rulings\n" +
	"\n" +
	"Another section.\n"

func TestCanonicalRoundTrip(t *testing.T) {
	document, diagnostics := Parse(canonical)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics: %v", diagnostics)
	}
	if rendered := Render(document); rendered != canonical {
		t.Fatalf("P1 failed:\n%q\n%q", canonical, rendered)
	}
}

// Valid input, non-canonical: P2 — emit(parse(emit(d))) == emit(d).
func TestEmissionStability(t *testing.T) {
	crlf := strings.ReplaceAll(canonical, "\n", "\r\n")
	document, diagnostics := Parse(crlf)
	if len(diagnostics) != 1 || diagnostics[0].Rule != RuleNonCanonical {
		t.Fatalf("diagnostics: %v", diagnostics)
	}
	first := Render(document)
	documentAgain, secondDiagnostics := Parse(first)
	if len(secondDiagnostics) != 0 {
		t.Fatalf("second diagnostics: %v", secondDiagnostics)
	}
	if second := Render(documentAgain); second != first {
		t.Fatalf("P2 failed:\n%q\n%q", first, second)
	}
	if strings.Contains(first, "\r") {
		t.Fatal("canonical emission keeps CR")
	}
}

func TestHeaderRules(t *testing.T) {
	missing := strings.ReplaceAll(canonical, "<!-- doc-id: plans-design-plan -->\n", "")
	_, diagnostics := Parse(missing)
	if len(diagnostics) != 1 || diagnostics[0].Rule != RuleHeader {
		t.Fatalf("diagnostics: %v", diagnostics)
	}

	bom := "\uFEFF" + canonical
	_, diagnostics = Parse(bom)
	if len(diagnostics) != 1 || diagnostics[0].Rule != RuleEncoding {
		t.Fatalf("diagnostics: %v", diagnostics)
	}

	twin := "<!-- doc-id: plans-design-plan -->\n<!-- source-sha256: abc -->\n" + strings.SplitN(canonical, "\n", 2)[1]
	document, diagnostics := Parse(twin)
	if len(diagnostics) != 0 || document.TwinSHA != "abc" {
		t.Fatalf("diagnostics: %v, twin: %q", diagnostics, document.TwinSHA)
	}
	if rendered := Render(document); rendered != twin {
		t.Fatalf("twin round-trip failed:\n%q\n%q", twin, rendered)
	}
}
