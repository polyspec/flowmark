// Tests of applying a transition to a corpus document.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const board = "<!-- doc-id: plans-execution-checklist -->\n# Checklist\n\n## Work\n\n- [ ] T1.1 Write the parser\n- [~] T2.1 Print the union\n  - Criteria: it prints.\n- [ ] T3.1 Keep the gate\n"

func rootWith(t *testing.T, text string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "plans", "list.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestApplyWaitingToDoing(t *testing.T) {
	root := rootWith(t, board)
	if err := Transition(root, "docs/plans/list.md", "T1.1", "doing", ""); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "<!-- doc-id: plans-execution-checklist -->\n# Checklist\n\n## Work\n\n- [~] T1.1 Write the parser\n- [~] T2.1 Print the union\n  - Criteria: it prints.\n- [ ] T3.1 Keep the gate\n"
	if string(text) != want {
		t.Fatalf("text:\n%q\nwant:\n%q", string(text), want)
	}
}

func TestApplyRequiresEvidence(t *testing.T) {
	root := rootWith(t, board)
	if err := Transition(root, "docs/plans/list.md", "T2.1", "done", ""); err == nil {
		t.Fatal("done without evidence passes")
	}
	if err := Transition(root, "docs/plans/list.md", "T2.1", "done", "make test passes"); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalIsRefused(t *testing.T) {
	root := rootWith(t, board)
	if err := Transition(root, "docs/plans/list.md", "T1.1", "done", "evidence"); err == nil {
		t.Fatal("waiting -> done passes the table")
	}
	if err := Transition(root, "docs/plans/list.md", "T1.1", "doing", ""); err != nil {
		t.Fatal(err)
	}
	if err := Transition(root, "docs/plans/list.md", "T1.1", "done", "make test passes"); err != nil {
		t.Fatal(err)
	}
	if err := Transition(root, "docs/plans/list.md", "T1.1", "waiting", ""); err == nil {
		t.Fatal("done -> waiting passes")
	}
}

func TestWIPGate(t *testing.T) {
	root := rootWith(t, board)
	if err := Transition(root, "docs/plans/list.md", "T1.1", "doing", ""); err != nil {
		t.Fatal(err)
	}
	if err := Transition(root, "docs/plans/list.md", "T3.1", "doing", ""); err == nil {
		t.Fatal("the WIP gate opens a third doing item")
	}
}

func TestAddItem(t *testing.T) {
	root := rootWith(t, board)
	if err := AddItem(root, "docs/plans/list.md", "T4.1", "A new issue arrives"); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(text), "- [ ] T4.1 A new issue arrives\n") {
		t.Fatalf("text ends with %q", string(text)[len(string(text))-60:])
	}
	if err := AddItem(root, "docs/plans/list.md", "T1.1", "A duplicate arrives"); err == nil {
		t.Fatal("a duplicate ID passes")
	}
}

func TestRemoveItem(t *testing.T) {
	root := rootWith(t, board)
	if err := RemoveItem(root, "docs/plans/list.md", "T1.1"); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), "T1.1") {
		t.Fatalf("text still holds T1.1")
	}
	if err := RemoveItem(root, "docs/plans/list.md", "T9.9"); err == nil {
		t.Fatal("an unknown ID passes")
	}
}

func TestRemoveItemKeepsChildrenRule(t *testing.T) {
	nested := "<!-- doc-id: plans-execution-checklist -->\n# Checklist\n\n## Work\n\n- [ ] T1 Parent\n- [ ] T1.1 Child\n"
	root := rootWith(t, nested)
	if err := RemoveItem(root, "docs/plans/list.md", "T1"); err == nil {
		t.Fatal("a parent with a child passes")
	}
	if err := RemoveItem(root, "docs/plans/list.md", "T1.1"); err != nil {
		t.Fatal(err)
	}
}

func TestNextID(t *testing.T) {
	root := rootWith(t, board) // T1.1, T2.1, T3.1 — the next top level is T4
	if id := NextID(root, "docs/plans/list.md"); id != "T4" {
		t.Fatalf("next: %q", id)
	}
	empty := "<!-- doc-id: plans-execution-checklist -->\n# Checklist\n\n## Work\n\n"
	root = rootWith(t, empty)
	if id := NextID(root, "docs/plans/list.md"); id != "T1" {
		t.Fatalf("first: %q", id)
	}
}

func TestAddItemAutoID(t *testing.T) {
	root := rootWith(t, board)
	if err := AddItem(root, "docs/plans/list.md", "", "An issue without an ID"); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "- [ ] T4 An issue without an ID") {
		t.Fatalf("text holds no auto T4")
	}
}

func TestMoveItem(t *testing.T) {
	root := rootWith(t, board) // T1.1, T2.1(with continuation), T3.1
	if err := MoveItem(root, "docs/plans/list.md", "T3.1", "T1.1"); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(text), "\n")
	// T3.1 should now be right after T1.1 (before T2.1)
	if !strings.Contains(lines[5], "T1.1") || !strings.Contains(lines[6], "T3.1") || !strings.Contains(lines[7], "T2.1") {
		t.Fatalf("order:\n%s", string(text))
	}
	// T2.1's continuation moved with it
	if !strings.Contains(string(text), "Criteria: it prints.") {
		t.Fatal("the continuation was dropped")
	}
}

func TestMoveToTop(t *testing.T) {
	root := rootWith(t, board)
	if err := MoveItem(root, "docs/plans/list.md", "T3.1", ""); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	lines := strings.Split(string(text), "\n")
	// T3.1 should be the first item (line 5 in this fixture, 0-indexed)
	if !strings.Contains(lines[5], "T3.1") {
		t.Fatalf("T3.1 is not first:\n%s", string(text))
	}
}
