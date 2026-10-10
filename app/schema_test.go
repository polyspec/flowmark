// Tests of the schema as a first-class entity: reading the Mermaid block, the Mermaid
// rendering, and the cascading effects of a schema change on the items.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const schemaBoard = "# Demo\n\n## State machine\n\n```mermaid\nstateDiagram-v2\n    direction LR\n    [*] --> waiting\n    waiting : [ ] 대기\n    waiting --> doing\n    doing : [~] 진행\n    doing --> done: evidence\n    doing --> waiting\n    done : [o] 완료\n    done --> [*]\n```\n\n## Work\n\n- [ ] T1 First\n- [~] T2 Second\n- [o] T3 Third\n- [ ] T1.1 Child of first\n- [-] T4 Gone\n"

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

func TestSchemaFromMermaidBlock(t *testing.T) {
	schema, err := SchemaFromMermaidBlock(schemaRoot(t), "docs/plans/checklist.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.States) != 3 || schema.States[0].Glyph != "[ ]" || schema.States[2].Label != "완료" {
		t.Fatalf("states: %+v", schema.States)
	}
	if schema.Initial != "waiting" {
		t.Fatalf("initial: %q", schema.Initial)
	}
	if len(schema.Transitions) != 3 || schema.Transitions[1].Requires[0] != "evidence" {
		t.Fatalf("transitions: %+v", schema.Transitions)
	}
}

func TestSchemaFromDocumentWithoutMermaid(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plain.md")
	os.WriteFile(path, []byte("# P\n\n## G\n\n- [ ] T1 One\n"), 0o644)
	if _, err := SchemaFromMermaidBlock(root, "plain.md"); err == nil {
		t.Fatal("a document without a Mermaid block holds no state machine")
	}
}

func TestSchemaMermaidRoundTrip(t *testing.T) {
	schema, err := SchemaFromMermaidBlock(schemaRoot(t), "docs/plans/checklist.md")
	if err != nil {
		t.Fatal(err)
	}
	first := SchemaMermaid(schema)
	again, err := ParseMermaid(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.States) != len(schema.States) || len(again.Transitions) != len(schema.Transitions) || again.Initial != schema.Initial {
		t.Fatalf("round-trip:\n%+v\n%+v", schema, again)
	}
	second := SchemaMermaid(again)
	if first != second {
		t.Fatalf("emission is not stable:\n%s\n---\n%s", first, second)
	}
}

func TestApplySchemaRename(t *testing.T) {
	root := schemaRoot(t)
	old, _ := SchemaFromMermaidBlock(root, "docs/plans/checklist.md")
	// rename [~] → [%]
	renewed := StateSchema{States: make([]StateDef, len(old.States)), Transitions: old.Transitions, Initial: old.Initial}
	copy(renewed.States, old.States)
	renewed.States[1].Glyph = "[%]"
	if err := ApplySchema(root, "docs/plans/checklist.md", old, renewed); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(root, "docs", "plans", "checklist.md"))
	if strings.Contains(string(text), "- [~]") {
		t.Fatal("the old glyph is still there")
	}
	if !strings.Contains(string(text), "- [%] T2 Second") {
		t.Fatal("the new glyph is not there")
	}
	// the schema itself moved into the Mermaid block
	after, err := SchemaFromMermaidBlock(root, "docs/plans/checklist.md")
	if err != nil {
		t.Fatal(err)
	}
	if after.States[1].Glyph != "[%]" {
		t.Fatalf("the Mermaid block was not rewritten: %+v", after.States)
	}
}

func TestApplySchemaRemoveState(t *testing.T) {
	root := schemaRoot(t)
	// swap the fixture: put T4 in a declared state, then remove that state
	path := filepath.Join(root, "docs", "plans", "checklist.md")
	text, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(text), "- [-] T4 Gone", "- [o] T4 Gone", 1)), 0o644)
	old, _ := SchemaFromMermaidBlock(root, "docs/plans/checklist.md")
	var states []StateDef
	for _, s := range old.States {
		if s.Glyph != "[o]" {
			states = append(states, s)
		}
	}
	renewed := old
	renewed.States = states
	if err := ApplySchema(root, "docs/plans/checklist.md", old, renewed); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "T4") || strings.Contains(string(after), "T3 Third") {
		t.Fatal("an item of the removed state is still there")
	}
	if !strings.Contains(string(after), "T1 First") || !strings.Contains(string(after), "T1.1 Child of first") {
		t.Fatal("an item of a kept state was removed")
	}
}
