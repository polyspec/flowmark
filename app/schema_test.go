// Tests of the schema as a first-class entity: reading from the frontmatter, the Mermaid
// rendering, and the cascading effects of a schema change on the items.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const schemaBoard = "<!-- doc-id: demo -->\n" +
	"---\n" +
	"state_machine: |\n" +
	"  {\"states\": [\n" +
	"    {\"glyph\": \"[ ]\", \"name\": \"waiting\", \"label\": \"대기\"},\n" +
	"    {\"glyph\": \"[~]\", \"name\": \"doing\", \"label\": \"진행\"},\n" +
	"    {\"glyph\": \"[o]\", \"name\": \"done\", \"label\": \"완료\"},\n" +
	"    {\"glyph\": \"[-]\", \"name\": \"impossible\", \"label\": \"불가\"}\n" +
	"  ], \"transitions\": [\n" +
	"    {\"from\": \"waiting\", \"to\": \"doing\"},\n" +
	"    {\"from\": \"doing\", \"to\": \"done\", \"requires\": [\"evidence\"]},\n" +
	"    {\"from\": \"doing\", \"to\": \"waiting\"}\n" +
	"  ]}\n" +
	"---\n" +
	"# Demo\n" +
	"\n" +
	"## Work\n" +
	"\n" +
	"- [ ] T1 First\n" +
	"- [~] T2 Second\n" +
	"- [o] T3 Third\n" +
	"- [ ] T1.1 Child of first\n" +
	"- [-] T4 Gone\n"

func schemaRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "docs", "plans", "checklist.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(schemaBoard), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSchemaFromDocument(t *testing.T) {
	schema, err := SchemaFromDocument(schemaRoot(t), "docs/plans/checklist.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.States) != 4 || schema.States[0].Glyph != "[ ]" || schema.States[2].Label != "완료" {
		t.Fatalf("states: %+v", schema.States)
	}
	if len(schema.Transitions) != 3 || schema.Transitions[1].Requires[0] != "evidence" {
		t.Fatalf("transitions: %+v", schema.Transitions)
	}
}

func TestSchemaFromDocumentDefault(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plain.md")
	os.WriteFile(path, []byte("<!-- doc-id: p -->\n# P\n\n## G\n\n- [ ] T1 One\n"), 0o644)
	schema, err := SchemaFromDocument(root, "plain.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.States) != 5 {
		t.Fatalf("default states: %+v", schema.States)
	}
}

func TestSchemaMermaid(t *testing.T) {
	schema, _ := SchemaFromDocument(schemaRoot(t), "docs/plans/checklist.md")
	mermaid := SchemaMermaid(schema)
	for _, part := range []string{"stateDiagram-v2", "[*] --> waiting", "doing --> done: evidence", "done --> [*]"} {
		if !strings.Contains(mermaid, part) {
			t.Fatalf("mermaid holds no %q:\n%s", part, mermaid)
		}
	}
}

func TestApplySchemaRename(t *testing.T) {
	root := schemaRoot(t)
	old, _ := SchemaFromDocument(root, "docs/plans/checklist.md")
	// rename [~] → [%]
	renewed := StateSchema{States: make([]StateDef, len(old.States)), Transitions: old.Transitions}
	copy(renewed.States, old.States)
	renewed.States[1].Glyph = "[%]"
	if err := ApplySchema(root, "docs/plans/checklist.md", old, renewed); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(root, "docs/plans", "checklist.md"))
	if strings.Contains(string(text), "- [~]") {
		t.Fatal("the old glyph is still there")
	}
	if !strings.Contains(string(text), "- [%] T2 Second") {
		t.Fatal("the new glyph is not there")
	}
}

func TestApplySchemaRemoveState(t *testing.T) {
	root := schemaRoot(t)
	old, _ := SchemaFromDocument(root, "docs/plans/checklist.md")
	// remove the [-] state (T4 Gone is in it)
	renewed := StateSchema{States: make([]StateDef, len(old.States)), Transitions: old.Transitions}
	copy(renewed.States, old.States)
	var states []StateDef
	for _, s := range renewed.States {
		if s.Glyph != "[-]" {
			states = append(states, s)
		}
	}
	renewed.States = states
	if err := ApplySchema(root, "docs/plans/checklist.md", old, renewed); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(root, "docs/plans", "checklist.md"))
	if strings.Contains(string(text), "T4") {
		t.Fatal("the removed item is still there")
	}
	if !strings.Contains(string(text), "T1 First") {
		t.Fatal("an unrelated item was removed")
	}
}

func TestWriteSchemaRoundTrip(t *testing.T) {
	root := schemaRoot(t)
	schema, _ := SchemaFromDocument(root, "docs/plans/checklist.md")
	// write the same schema back — the document should not change
	text, _ := os.ReadFile(filepath.Join(root, "docs/plans", "checklist.md"))
	if _, err := writeSchema(string(text), schema); err != nil {
		t.Fatal(err)
	}
	again, err := SchemaFromDocument(root, "docs/plans/checklist.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(again.States) != len(schema.States) || len(again.Transitions) != len(schema.Transitions) {
		t.Fatalf("round-trip:\n%+v\n%+v", schema, again)
	}
}
