package validate

import (
	"fmt"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"path/filepath"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
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
	corpustest.WriteFiles(t, root, map[string]string{rel: text})
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
			checkCorpus(t, cat, root, tc.want)
		})
	}
}

// TestScaleEnvelope pins the diagnostics of a corpus past the scale envelope (spec 40 The scale envelope): a file
// of more bytes than catalog.Envelope.FileBytes, a folder of more documents than Envelope.FolderFiles, and a corpus
// of more documents than Envelope.CorpusFiles are scale.file, scale.folder and scale.corpus, each naming what is
// past, its size, its table and the envelope. The rule reports; a corpus past the envelope is read and written all
// the same.
func TestScaleEnvelope(t *testing.T) {
	cat := shardCatalog(t)
	env := catalog.Envelope
	cases := []struct {
		name  string
		build func(root string)
		want  []string
	}{
		{name: "40#the-scale-envelope: a corpus within the envelope reports nothing", build: func(root string) {
			writeFile(t, root, "events/2026-10/E1.flowmark.md", event("A", "2026-10-11 09:30:00.000000"))
		}},
		{name: "40#the-scale-envelope: a file past the file size is scale.file", build: func(root string) {
			rows := make([][3]string, 0, env.FileBytes/30)
			for i := 0; len(rows) < cap(rows); i++ {
				rows = append(rows, [3]string{fmt.Sprintf("N%d", i+1), fmt.Sprintf("Note %d", i+1), "2026-03-04"})
			}
			writeShardNotes(t, cat, root, "notes/2026.flowmark.md", rows...)
		}, want: []string{"notes/2026.flowmark.md scale.file ~ notes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.build(root)
			checkCorpus(t, cat, root, tc.want)
		})
	}
}

// TestScaleEnvelopeCounts pins scale.folder and scale.corpus with an envelope of a few documents, since a corpus of
// the real limits holds twenty thousand: the rules read catalog.Envelope, which the test lowers and restores.
func TestScaleEnvelopeCounts(t *testing.T) {
	cat := shardCatalog(t)
	old := catalog.Envelope
	t.Cleanup(func() { catalog.Envelope = old })
	catalog.Envelope = catalog.EnvelopeLimits{FileBytes: old.FileBytes, FolderFiles: 2, CorpusFiles: 4}
	build := func(root string, folders ...int) {
		n := 0
		for m, count := range folders {
			for i := 0; i < count; i++ {
				n++
				writeFile(t, root, fmt.Sprintf("events/2026-%02d/E%d.flowmark.md", m+1, n), event("E", fmt.Sprintf("2026-%02d-15 09:00:00.000000", m+1)))
			}
		}
	}
	cases := []struct {
		name    string
		folders []int
		want    []string
	}{
		{name: "40#the-scale-envelope: a folder and a corpus at the envelope report nothing", folders: []int{2, 2}},
		{name: "40#the-scale-envelope: a folder past the envelope is scale.folder", folders: []int{3, 1}, want: []string{"events/2026-01 scale.folder ~ 3 documents"}},
		{name: "40#the-scale-envelope: a corpus past the envelope is scale.corpus", folders: []int{2, 2, 1}, want: []string{". scale.corpus ~ 5 documents"}},
		{name: "40#the-scale-envelope: both are reported", folders: []int{3, 2}, want: []string{"events/2026-01 scale.folder ~ 3 documents", ". scale.corpus ~ 5 documents"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			build(root, tc.folders...)
			checkCorpus(t, cat, root, tc.want)
		})
	}
}
