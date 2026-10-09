// Tests of the db→md materialization with the provenance header and the drift check.
package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeAndDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docs", "board.md")
	rows := []map[string]string{
		{"id": "T1", "state": "[ ]", "title": "One"},
		{"id": "T2", "state": "[o]", "title": "Two"},
	}
	if err := Materialize(path, "board", "Board", rows); err != nil {
		t.Fatal(err)
	}
	drift, err := Drift(path)
	if err != nil {
		t.Fatal(err)
	}
	if drift != "" {
		t.Fatalf("drift: %q", drift)
	}
}

func TestTamperingReports(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docs", "board.md")
	rows := []map[string]string{{"id": "T1", "state": "[ ]", "title": "One"}}
	if err := Materialize(path, "board", "Board", rows); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(text, []byte("A human edit.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	drift, err := Drift(path)
	if err != nil {
		t.Fatal(err)
	}
	if drift == "" {
		t.Fatal("the drift check misses a human edit")
	}
}
