// Tests of applying a transition to a corpus document.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const board = "# Checklist\n\n## State machine\n\n```mermaid\nstateDiagram-v2\n    direction LR\n    [*] --> waiting\n    waiting : [ ] waiting\n    waiting --> doing\n    waiting --> impossible: cause\n    doing : [~] doing\n    doing --> done: evidence\n    doing --> bypassed: cause, retry\n    doing --> waiting\n    bypassed : [!] bypassed\n    bypassed --> doing\n    done : [o] done\n    impossible : [-] impossible\n    done --> [*]\n    impossible --> [*]\n```\n\n## Work\n\n- [ ] T1.1 Write the parser\n- [~] T2.1 Print the union\n  - Criteria: it prints.\n- [ ] T3.1 Keep the gate\n"

// customBoard declares its own state names and edges: the document, not a built-in table,
// is the only authority.
const customBoard = "# Custom\n\n## State machine\n\n```mermaid\nstateDiagram-v2\n    direction LR\n    [*] --> draft\n    draft : [ ] draft\n    draft --> review\n    review : [~] review\n    review --> shipped: evidence\n    shipped : [o] shipped\n    shipped --> [*]\n```\n\n## Work\n\n- [ ] T1 First item\n"

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

func TestTransitionFollowsTheDocument(t *testing.T) {
	root := rootWith(t, customBoard)
	// the document declares draft -> review with no required field
	if err := Transition(root, "docs/plans/list.md", "T1", "review", ""); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if !strings.Contains(string(text), "- [~] T1 First item") {
		t.Fatalf("text:\n%s", string(text))
	}
	// the document declares no review -> draft edge: it is refused
	if err := Transition(root, "docs/plans/list.md", "T1", "draft", ""); err == nil {
		t.Fatal("an undeclared transition passes")
	}
	// the document requires evidence on review -> shipped
	if err := Transition(root, "docs/plans/list.md", "T1", "shipped", ""); err == nil {
		t.Fatal("shipped without evidence passes")
	}
	if err := Transition(root, "docs/plans/list.md", "T1", "shipped", "make test passes"); err != nil {
		t.Fatal(err)
	}
}

func TestAddItemTakesTheInitialGlyph(t *testing.T) {
	// the initial state of this document is review ([~]), not waiting
	initial := "# Custom\n\n## State machine\n\n```mermaid\nstateDiagram-v2\n    direction LR\n    [*] --> review\n    review : [~] review\n    review --> shipped: evidence\n    shipped : [o] shipped\n    shipped --> [*]\n```\n\n## Work\n"
	root := rootWith(t, initial)
	if err := AddItem(root, "docs/plans/list.md", "", "A new issue"); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if !strings.Contains(string(text), "- [~] T1 A new issue") {
		t.Fatalf("text:\n%s", string(text))
	}
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
	want := strings.Replace(board, "- [ ] T1.1 Write the parser", "- [~] T1.1 Write the parser", 1)
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
	// T3.1 should now sit right after T1.1 (before T2.1)
	at := 0
	for i, line := range lines {
		if strings.Contains(line, "T1.1") {
			at = i
			break
		}
	}
	if !strings.Contains(lines[at+1], "T3.1") || !strings.Contains(lines[at+2], "T2.1") {
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
	// T3.1 should be the first item of the Work section
	first := true
	for _, line := range lines {
		if strings.HasPrefix(line, "- ") {
			if first && !strings.Contains(line, "T3.1") {
				t.Fatalf("T3.1 is not first:\n%s", string(text))
			}
			first = false
		}
	}
}

func TestEditTitle(t *testing.T) {
	root := rootWith(t, board)
	if err := EditTitle(root, "docs/plans/list.md", "T1.1", "A renamed parser"); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if !strings.Contains(string(text), "T1.1 A renamed parser") {
		t.Fatal("the title was not rewritten")
	}
	if strings.Contains(string(text), "Write the parser") {
		t.Fatal("the old title is still there")
	}
}

const tableBoard = "# Checklist\n\n## State machine\n\n```mermaid\nstateDiagram-v2\n    direction LR\n    [*] --> waiting\n    waiting : [ ] waiting\n    waiting --> doing\n    doing : [~] doing\n    doing --> done: evidence\n    done : [o] done\n    done --> [*]\n```\n\n## Work\n\n| state | id | title |\n| --- | --- | --- |\n| [ ] | T1 | First |\n| [~] | T2 | Second |\n"

func TestTransitionRewritesTheTableCell(t *testing.T) {
	root := rootWith(t, tableBoard)
	if err := Transition(root, "docs/plans/list.md", "T2", "done", "make test passes"); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if !strings.Contains(string(text), "| [o] | T2 | Second |") || strings.Contains(string(text), "| [~] | T2 |") {
		t.Fatalf("text:\n%s", string(text))
	}
	// the document declares no waiting -> done edge
	if err := Transition(root, "docs/plans/list.md", "T1", "done", "make test passes"); err == nil {
		t.Fatal("an undeclared transition passes")
	}
}

func TestTableWritePath(t *testing.T) {
	root := rootWith(t, tableBoard)
	if err := AddItem(root, "docs/plans/list.md", "T3", "A new row"); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if !strings.Contains(string(text), "| [ ] | T3 | A new row |") {
		t.Fatalf("add:\n%s", string(text))
	}
	if err := EditTitle(root, "docs/plans/list.md", "T3", "A renamed row"); err != nil {
		t.Fatal(err)
	}
	text, _ = os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if !strings.Contains(string(text), "| [ ] | T3 | A renamed row |") {
		t.Fatalf("edit:\n%s", string(text))
	}
	if err := MoveItem(root, "docs/plans/list.md", "T1", "T2"); err != nil {
		t.Fatal(err)
	}
	text, _ = os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if strings.Index(string(text), "| T2") < strings.Index(string(text), "| T1") {
		t.Fatalf("move:\n%s", string(text))
	}
	if err := RemoveItem(root, "docs/plans/list.md", "T1"); err != nil {
		t.Fatal(err)
	}
	text, _ = os.ReadFile(filepath.Join(root, "docs", "plans", "list.md"))
	if strings.Contains(string(text), "T1 |") {
		t.Fatalf("remove:\n%s", string(text))
	}
}
