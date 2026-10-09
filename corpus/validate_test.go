// Tests of the corpus validator: unique keys and resolved references across the corpus.
package corpus

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

const clean = "<!-- doc-id: a -->\n# A\n\nThe item T1.1 lands after T1.2 prints.\n"
const checklist = "<!-- doc-id: list -->\n# List\n\n## Group\n\n- [o] T1.1 Land the item\n- [ ] T1.2 Print the union\n"

func findingsOf(t *testing.T, root string) []Finding {
	t.Helper()
	found, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestCleanCorpus(t *testing.T) {
	root := write(t, map[string]string{"a.md": clean, "list.md": checklist})
	if found := findingsOf(t, root); len(found) != 0 {
		t.Fatalf("findings: %v", found)
	}
}

func TestDuplicateKeys(t *testing.T) {
	root := write(t, map[string]string{"a.md": clean, "b.md": clean, "list.md": checklist})
	found := findingsOf(t, root)
	if len(found) != 1 || found[0].Rule != "key.duplicate" {
		t.Fatalf("findings: %v", found)
	}
}

func TestDuplicateItems(t *testing.T) {
	duplicated := "<!-- doc-id: other -->\n# Other\n\n## Group\n\n- [ ] T1.2 Appears twice\n"
	root := write(t, map[string]string{"list.md": checklist, "other.md": duplicated})
	found := findingsOf(t, root)
	if len(found) != 1 || found[0].Rule != "key.duplicate" {
		t.Fatalf("findings: %v", found)
	}
}

func TestDanglingReference(t *testing.T) {
	dangling := "<!-- doc-id: b -->\n# B\n\nRefines T9.9, which names no item.\n"
	root := write(t, map[string]string{"a.md": clean, "list.md": checklist, "b.md": dangling})
	found := findingsOf(t, root)
	if len(found) != 1 || found[0].Rule != "reference.dangling" || found[0].Line != 4 {
		t.Fatalf("findings: %v", found)
	}
}

func TestFencedAndInlineExcluded(t *testing.T) {
	excluded := "<!-- doc-id: b -->\n# B\n\n```\nT9.9 fenced\n```\n\nThe shape `T9.9` is an example.\n"
	root := write(t, map[string]string{"a.md": clean, "list.md": checklist, "b.md": excluded})
	if found := findingsOf(t, root); len(found) != 0 {
		t.Fatalf("findings: %v", found)
	}
}

func TestSelectionExcludes(t *testing.T) {
	vendored := "<!-- doc-id: vendored -->\n# V\n\n## Group\n\n- [o] K1 Fixture item\n"
	root := t.TempDir()
	write(t, map[string]string{"docs/a.md": clean, "docs/list.md": checklist, "tests/kit/fixture.md": vendored})
	found, err := ValidateSelected(root, []string{"**/*.md"}, []string{"tests/kit/**"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("findings: %v", found)
	}
}
