package writepath

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store/journal"
)

// The benchmarks of spec 40's cost model: an operation on a long-lived corpus of the envelope's 10k documents within
// hundreds of milliseconds. Each runs on the base corpus alone and with 10k notes beside it, so the part of a cost
// that grows with the corpus shows apart from the part that does not (the lock, the journal's syncs). Beside ns/op
// each reports the CPU time of the process per operation (cpu-ms/op: the operation's own work, whatever else the
// machine runs), and the opening reports the heap the corpus keeps (heap-KB/doc).

// benchSizes are the numbers of notes the benchmarks add to the base corpus.
var benchSizes = []int{0, 10000}

// benchDocs is the base corpus with n notes in files.
func benchDocs(n int) map[string]string {
	docs := baseDocs()
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("B%d", i)
		docs["notes/"+id+".flowmark.md"] = lines(
			fmt.Sprintf("# Bench note %d", i),
			"- item: [T1](../items.flowmark.md#items-row-t1)",
			fmt.Sprintf("- position: %d", i),
			"",
			fmt.Sprintf("The body of the bench note %d.", i),
		)
	}
	return docs
}

// liveHeap returns the heap in use after a full collection.
func liveHeap() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// loop runs op b.N times and reports the 95th percentile of its wall time and the CPU time per operation.
func loop(b *testing.B, op func(i int)) {
	b.Helper()
	start := processCPU(b)
	var walls []time.Duration
	i := 0
	for b.Loop() {
		i++
		t0 := time.Now()
		op(i)
		walls = append(walls, time.Since(t0))
	}
	b.ReportMetric(float64((processCPU(b)-start)/time.Microsecond)/1000/float64(b.N), "cpu-ms/op")
	slices.Sort(walls)
	b.ReportMetric(float64(walls[(len(walls)*95+99)/100-1]/time.Microsecond)/1000, "p95-ms")
}

// sizes runs bench for each size of benchSizes.
func sizes(b *testing.B, bench func(b *testing.B, n int)) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("notes=%d", n), func(b *testing.B) { bench(b, n) })
	}
}

// nopObserver drops every event.
type nopObserver struct{}

func (nopObserver) Observe(journal.Event) {}

// openCorpus opens the corpus at root through the write path with an observer that keeps nothing; the caller closes
// it.
func openCorpus(t testing.TB, root string) *Corpus {
	t.Helper()
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("catalog of %s: %+v", root, diags)
	}
	j, err := journal.New(root, nopObserver{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Open(context.Background(), cat, j, 0)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// keptHeap returns the heap that a corpus opened at root keeps, in bytes per corpus document of docs.
func keptHeap(t testing.TB, root string, docs int) float64 {
	t.Helper()
	before := liveHeap()
	c := openCorpus(t, root)
	kept := liveHeap() - before
	runtime.KeepAlive(c)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	return float64(kept) / float64(docs)
}

// BenchmarkOpen is the opening of a corpus: its first full read, which arms the watch of every folder and document,
// reads every document and parses it.
func BenchmarkOpen(b *testing.B) {
	sizes(b, func(b *testing.B, n int) {
		f := newFixture(b, executorCatalog(b), benchDocs(n))
		docs := len(benchDocs(n))
		loop(b, func(int) {
			if err := openCorpus(b, f.root).Close(); err != nil {
				b.Fatal(err)
			}
		})
		b.ReportMetric(keptHeap(b, f.root, docs)/1024, "heap-KB/doc")
	})
}

// BenchmarkRead is a read of the notes of a corpus kept in memory.
func BenchmarkRead(b *testing.B) {
	sizes(b, func(b *testing.B, n int) {
		f := newFixture(b, executorCatalog(b), benchDocs(n))
		s := f.session("bench")
		m := f.msg("count", "notes", nil)
		ctx := context.Background()
		loop(b, func(int) {
			if _, err := s.Exec(ctx, m); err != nil {
				b.Fatal(err)
			}
		})
	})
}

// BenchmarkWrite is an update of one note of a corpus kept in memory: one transaction under the lock, committed
// through the journal.
func BenchmarkWrite(b *testing.B) {
	sizes(b, func(b *testing.B, n int) {
		f := newFixture(b, executorCatalog(b), benchDocs(n))
		s := f.session("bench")
		ctx := context.Background()
		loop(b, func(i int) {
			m := f.msg("update", "notes", map[string]any{"set": sets(0, "weight"), "where": whereID(1)}, float64(i), "N3")
			if _, err := s.Exec(ctx, m); err != nil {
				b.Fatal(err)
			}
		})
	})
}
