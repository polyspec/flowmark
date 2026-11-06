package writepath

import (
	"context"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
)

// TestGenerationOfARead pins that a corpus carries the commit generation it was read at (spec 40 The commit
// generation): another handle of the same corpus learns from the journal alone that a commit landed after its read,
// and its next read under the lock is at that commit's generation.
func TestGenerationOfARead(t *testing.T) {
	writer := baseFixture(t)
	reader := openFixture(t, writer.root)
	ws, rs := writer.session("writer"), reader.session("reader")
	if got := reader.c.Generation(); got != 0 {
		t.Fatalf("a corpus no commit wrote is read at generation %d", got)
	}
	update := func(title string) {
		t.Helper()
		m := writer.msg("update", "notes", map[string]any{"set": sets(0, "title"), "where": corpustest.WhereID(1)}, title, "N3")
		if _, err := ws.Exec(context.Background(), m); err != nil {
			t.Fatal(err)
		}
	}
	update("Generation one")
	g, err := reader.j.Generation()
	if err != nil {
		t.Fatal(err)
	}
	if g.N != 1 || reader.c.Generation() != 0 {
		t.Fatalf("after another handle's commit the journal is at %d and the reader read at %d; want 1 and 0", g.N, reader.c.Generation())
	}
	if paths, ok := g.Since(reader.c.Generation()); !ok || len(paths) != 1 || paths[0] != "notes/N3.flowmark.md" {
		t.Fatalf("the reader learns %v, %v; want the commit's file", paths, ok)
	}
	reader.noteTitles(rs)
	if got := reader.c.Generation(); got != 1 {
		t.Fatalf("the read after the commit is at generation %d, want 1", got)
	}
	update("Generation two")
	writer.noteTitles(ws)
	if got := writer.c.Generation(); got != 2 {
		t.Fatalf("the writer's read after its own commits is at generation %d, want 2", got)
	}
}
