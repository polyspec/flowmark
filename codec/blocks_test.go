// Tests of the child blocks: checklist items and markdown tables extracted as rows.
package codec

import "testing"

const blockDoc = "<!-- doc-id: plans-execution-checklist -->\n" +
	"# Checklist\n" +
	"\n" +
	"## T1 Group\n" +
	"\n" +
	"- [o] T1.1 Write the parser\n" +
	"  - Criteria: it parses.\n" +
	"- [ ] T1.2 Print the union\n" +
	"\n" +
	"## Status table\n" +
	"\n" +
	"| ID | Task | State |\n" +
	"|---|---|---|\n" +
	"| K1 | List the tools | [o] |\n" +
	"| K2 | Merge the gates | [~] |\n"

func TestChecklistItems(t *testing.T) {
	document, diagnostics := Parse(blockDoc)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics: %v", diagnostics)
	}
	if len(document.Items) != 2 {
		t.Fatalf("items: %+v", document.Items)
	}
	first := document.Items[0]
	if first.ID != "T1.1" || first.State != "[o]" || first.Title != "Write the parser" || first.Line != 6 {
		t.Fatalf("first: %+v", first)
	}
	if len(first.Continuations) != 1 || first.Continuations[0] != "  - Criteria: it parses." {
		t.Fatalf("continuations: %+v", first.Continuations)
	}
	if document.Items[1].ID != "T1.2" || document.Items[1].State != "[ ]" {
		t.Fatalf("second: %+v", document.Items[1])
	}
}

func TestStatusTable(t *testing.T) {
	document, diagnostics := Parse(blockDoc)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics: %v", diagnostics)
	}
	if len(document.Tables) != 1 || len(document.Tables[0].Rows) != 2 {
		t.Fatalf("tables: %+v", document.Tables)
	}
	if document.Tables[0].Headers[0] != "ID" || document.Tables[0].Rows[1][2] != "[~]" {
		t.Fatalf("table: %+v", document.Tables[0])
	}
}

func TestFencedItemIsNotARow(t *testing.T) {
	text := "<!-- doc-id: a -->\n# T\n\n## S\n\n```\n- [o] T1 fenced\n```\n"
	document, diagnostics := Parse(text)
	if len(diagnostics) != 0 || len(document.Items) != 0 {
		t.Fatalf("diagnostics: %v, items: %+v", diagnostics, document.Items)
	}
}

func TestMalformedItem(t *testing.T) {
	text := "<!-- doc-id: a -->\n# T\n\n## S\n\n- [o]\n"
	_, diagnostics := Parse(text)
	if len(diagnostics) != 1 || diagnostics[0].Rule != RuleBlock {
		t.Fatalf("diagnostics: %v", diagnostics)
	}
}
