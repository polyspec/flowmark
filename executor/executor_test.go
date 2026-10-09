// Tests of the executor: the first IR kinds over the corpus rows.
package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const docs = "<!-- doc-id: b-doc -->\n# B\n\n## Group\n\n- [o] T1.1 First\n"
const other = "<!-- doc-id: a-doc -->\n# A\n\n## Group\n\n- [ ] T1.2 Second\n- [~] T2.1 Third\n"

func load(t *testing.T) *Corpus {
	t.Helper()
	corpus, err := Open(write(t, map[string]string{"b.md": docs, "a.md": other}))
	if err != nil {
		t.Fatal(err)
	}
	return corpus
}

func TestAllOrdersByDocID(t *testing.T) {
	rows, err := load(t).All("documents", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["doc_id"] != "a-doc" || rows[1]["doc_id"] != "b-doc" {
		t.Fatalf("rows: %v", rows)
	}
}

func TestOneWithWhere(t *testing.T) {
	row, found, err := load(t).One("documents", Query{Where: []Condition{{Column: "doc_id", Op: "=", Value: "b-doc"}}})
	if err != nil || !found {
		t.Fatalf("found: %v, err: %v", found, err)
	}
	if row["title"] != "B" {
		t.Fatalf("row: %v", row)
	}
}

func TestCountOverItems(t *testing.T) {
	corpus := load(t)
	count, err := corpus.Count("items", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("count: %d", count)
	}
	done, err := corpus.Count("items", Query{Where: []Condition{{Column: "state", Op: "=", Value: "[o]"}}})
	if err != nil {
		t.Fatal(err)
	}
	if done != 1 {
		t.Fatalf("done: %d", done)
	}
}

func TestOrderAndLimit(t *testing.T) {
	rows, err := load(t).All("items", Query{Order: "id", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["id"] != "T1.1" || rows[1]["id"] != "T1.2" {
		t.Fatalf("rows: %v", rows)
	}
}

func TestTypeDirectedOrdering(t *testing.T) {
	root := t.TempDir()
	deep := "<!-- doc-id: deep -->\n# D\n\n## Group\n\n- [ ] T8.0.10 Tenth\n- [ ] T8.0.9 Ninth\n- [ ] T8.0.14-1 Spawn\n"
	if err := os.WriteFile(filepath.Join(root, "deep.md"), []byte(deep), 0o644); err != nil {
		t.Fatal(err)
	}
	corpus, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := corpus.All("items", Query{})
	if err != nil {
		t.Fatal(err)
	}
	order := []string{rows[0]["id"], rows[1]["id"], rows[2]["id"]}
	if order[0] != "T8.0.9" || order[1] != "T8.0.10" || order[2] != "T8.0.14-1" {
		t.Fatalf("order: %v", order)
	}
}
