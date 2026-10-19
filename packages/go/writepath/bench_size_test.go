package writepath

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
)

// The benchmarks of the size of one file (spec 40 The scale envelope): a write parses, checks and renders its target
// whole, so its cost grows with the bytes of the target. The notes table of the shard golden is a document table, so
// all the rows of a year stand in one document; the benchmarks make that document of benchFileRows rows and report its
// bytes beside the p95 of an update and of a read.

// benchFileRows are the numbers of rows of the one document.
var benchFileRows = []int{1000, 10000, 40000}

// sizedNotes writes a corpus of the shard golden whose notes document of 2026 holds rows rows, and returns its root and
// the bytes of the document.
func sizedNotes(b testing.TB, rows int) (string, int) {
	b.Helper()
	root := b.TempDir()
	writeTree(b, root, map[string]string{"dbspec/test.dbs": shardDBS(b)})
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		b.Fatalf("catalog: %+v", diags)
	}
	notes := make([]*store.Row, rows)
	for i := range notes {
		written := fmt.Sprintf("2026-%02d-%02d", 1+i%12, 1+i%28)
		notes[i] = &store.Row{Table: "notes", Key: fmt.Sprintf("N%d", i+1), Title: fmt.Sprintf("Note %d", i+1), Scalars: map[string]*string{"written": &written}}
	}
	rel := "notes/2026.flowmark.md"
	out, refusals, err := store.RenderDocument(cat, "notes", rel, notes, nil)
	if err != nil || len(refusals) > 0 {
		b.Fatalf("render: %v %+v", err, refusals)
	}
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), out, 0o644); err != nil {
		b.Fatal(err)
	}
	return root, len(out)
}

// BenchmarkFileSizeWrite is the update of one note of a document of the given size, kept in memory.
func BenchmarkFileSizeWrite(b *testing.B) {
	for _, rows := range benchFileRows {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			root, bytes := sizedNotes(b, rows)
			f := openFixture(b, root)
			s := f.session("bench")
			ctx := context.Background()
			loop(b, func(i int) {
				m := f.msg("update", "notes", map[string]any{"set": []any{map[string]any{"column": "title", "p": 0}}, "where": whereID(1)}, fmt.Sprintf("Renamed %d", i), "N1")
				if _, err := s.Exec(ctx, m); err != nil {
					b.Fatal(err)
				}
			})
			b.ReportMetric(float64(bytes)/1024, "file-KB")
		})
	}
}

// BenchmarkFileSizeRead is a count of the notes of a document of the given size, kept in memory.
func BenchmarkFileSizeRead(b *testing.B) {
	for _, rows := range benchFileRows {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			root, bytes := sizedNotes(b, rows)
			f := openFixture(b, root)
			s := f.session("bench")
			m := f.msg("count", "notes", nil)
			ctx := context.Background()
			loop(b, func(int) {
				if _, err := s.Exec(ctx, m); err != nil {
					b.Fatal(err)
				}
			})
			b.ReportMetric(float64(bytes)/1024, "file-KB")
		})
	}
}

// BenchmarkSessions is the update of one note by several sessions of one corpus at once, the lock serializing them
// (spec 40 The lock): each session writes in turn, so the wall time of one write grows with the sessions that wait
// for it, while the work of a write does not.
func BenchmarkSessions(b *testing.B) {
	for _, sessions := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			f := newFixture(b, executorCatalog(b), benchDocs(0))
			// A corpus that waits for the lock, as the sessions of a server do: the one of the fixture refuses at once.
			c, err := Open(context.Background(), f.cat, f.j, 30*time.Second)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { c.Close() })
			ss := make([]*Session, sessions)
			for i := range ss {
				ss[i], err = c.Session(fmt.Sprintf("bench%d", i))
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() { ss[i].Close() })
			}
			ctx := context.Background()
			var turn atomic.Int64
			loop(b, func(i int) {
				var wg sync.WaitGroup
				for k := 0; k < sessions; k++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						m := f.msg("update", "notes", map[string]any{"set": sets(0, "weight"), "where": whereID(1)}, float64(turn.Add(1)), "N3")
						if _, err := ss[k].Exec(ctx, m); err != nil {
							b.Error(err)
						}
					}()
				}
				wg.Wait()
			})
		})
	}
}
