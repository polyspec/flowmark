// Tests of the md→db export: a derived, deterministic SQL dump of the corpus rows.
package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportSQL(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "plans", "list.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	text := "<!-- doc-id: list -->\n# List\n\n## G\n\n- [o] T1 One\n- [ ] T2 Two\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	dump, err := ExportSQL(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dump, "INSERT INTO documents (doc_id, title) VALUES ('list', 'List');") {
		t.Fatalf("documents: %s", dump)
	}
	if !strings.Contains(dump, "INSERT INTO items (id, state, title, doc_id) VALUES ('T1', '[o]', 'One', 'list');") {
		t.Fatalf("items: %s", dump)
	}
	again, _ := ExportSQL(root)
	if dump != again {
		t.Fatal("the dump is not deterministic")
	}
}
