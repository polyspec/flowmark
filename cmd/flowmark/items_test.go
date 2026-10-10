package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestItemsColumns(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "list.md")
	document := "# L\n\n## People\n\n| id | name |\n| --- | --- |\n| max | Kim |\n\n## Work\n\n- [~] T1 An item @max 2026-10-12T18:00\n"
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := itemsTo(&out, root); code != 0 {
		t.Fatalf("code: %d", code)
	}
	fields := strings.Split(strings.TrimSpace(out.String()), "\t")
	if len(fields) != 6 {
		t.Fatalf("fields: %q", fields)
	}
	if fields[4] != "max" || fields[5] != "2026-10-12T18:00" {
		t.Fatalf("assignee/due: %q", fields)
	}
	if fields[3] != "list.md" {
		t.Fatalf("path: %q", fields)
	}
}
