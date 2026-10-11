package validate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store"
)

// shardCatalog is the catalog of the store's shard golden: events, a files table sharded by the month of a
// datetime, and notes, a document table sharded by the year of a date.
func shardCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, diags := catalog.Load(filepath.Join("..", "store", "testdata", "shard"))
	if len(diags) > 0 || cat == nil {
		t.Fatalf("shard catalog: %+v", diags)
	}
	return cat
}

// writeShardNotes renders the notes document of a period, each note key, title, written.
func writeShardNotes(t *testing.T, cat *catalog.Catalog, root, rel string, notes ...[3]string) {
	t.Helper()
	var rows []*store.Row
	for _, n := range notes {
		w := n[2]
		rows = append(rows, &store.Row{Table: "notes", Key: n[0], Title: n[1], Scalars: map[string]*string{"written": &w}})
	}
	out, rf, err := store.RenderDocument(cat, "notes", rel, rows, nil)
	if err != nil || len(rf) > 0 {
		t.Fatalf("render %s: %v %+v", rel, err, rf)
	}
	writeFile(t, root, rel, string(out))
}

func writeFile(t *testing.T, root, rel, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func event(title, at string) string { return "# " + title + "\n- at: " + at + "\n" }

// TestShardPeriod pins the shard.period rule (spec 20 Time shards): a row stands in the file of the period its
// column value names.
func TestShardPeriod(t *testing.T) {
	cat := shardCatalog(t)
	cases := []struct {
		name  string
		build func(root string)
		want  []string
	}{
		{name: "20#time-shards: rows in the files of their periods report nothing", build: func(root string) {
			writeFile(t, root, "events/2026-10/E1.flowmark.md", event("A", "2026-10-11 09:30:00.000000"))
			writeFile(t, root, "events/2026-11/E2.flowmark.md", event("B", "2026-11-02 08:00:00.000000"))
			writeShardNotes(t, cat, root, "notes/2026.flowmark.md", [3]string{"N1", "One", "2026-03-04"})
			writeShardNotes(t, cat, root, "notes/2027.flowmark.md", [3]string{"N2", "Two", "2027-01-02"})
		}},
		{name: "20#time-shards: a datetime counts at its stored UTC value", build: func(root string) {
			writeFile(t, root, "events/2026-10/E1.flowmark.md", event("A", "2026-10-31 23:59:59.999999"))
			writeFile(t, root, "events/2026-11/E2.flowmark.md", event("B", "2026-11-01 00:00:00.000000"))
		}},
		{name: "20#time-shards: a files row in another period's folder", build: func(root string) {
			writeFile(t, root, "events/2026-11/E1.flowmark.md", event("A", "2026-10-11 09:30:00.000000"))
		}, want: []string{"events/2026-11/E1.flowmark.md:2:3 shard.period ~ 2026-10"}},
		{name: "20#time-shards: a document row in another period's document", build: func(root string) {
			writeShardNotes(t, cat, root, "notes/2026.flowmark.md", [3]string{"N1", "One", "2026-03-04"}, [3]string{"N2", "Two", "2027-01-02"})
		}, want: []string{"notes/2026.flowmark.md shard.period ~ 2027"}},
		{name: "20#time-shards: a key shared across two periods is a duplicate key", build: func(root string) {
			writeFile(t, root, "events/2026-10/E1.flowmark.md", event("A", "2026-10-11 09:30:00.000000"))
			writeFile(t, root, "events/2026-11/E1.flowmark.md", event("B", "2026-11-02 08:00:00.000000"))
		}, want: []string{"events/2026-11/E1.flowmark.md key.duplicate ~ byte for byte"}},
		{name: "20#time-shards: a document directly in a sharded files folder is a walk diagnostic", build: func(root string) {
			writeFile(t, root, "events/E1.flowmark.md", event("A", "2026-10-11 09:30:00.000000"))
		}, want: []string{"events/E1.flowmark.md corpus.walk"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.build(root)
			diags, err := Corpus(cat, root)
			if err != nil {
				t.Fatal(err)
			}
			check(t, diags, tc.want)
		})
	}
}
