package writepath

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/store/journal"
)

// The benchmarks of a corpus of the same documents stored flat and sharded by month (spec 20 Time shards): the
// events table of the shard golden holds events events, a tenth of them in each of ten months, once as a files
// table in one folder and once with its shard clause. A sharded table keeps the number of documents; what its folders
// change is what the benchmarks show.

// benchFolderEvents are the numbers of events of the benchmark corpus that the folder benchmarks sweep: the files of
// one folder.
var benchFolderEvents = []int{5000, 10000, 20000}

// benchMonths is the number of months the events are spread over.
const benchMonths = 10

// eventDocs returns the given number of events in files: in one folder when sharded is false, in the folder of their month
// when it is true. Event i is in month 1 + i%benchMonths of 2026.
func eventDocs(events int, sharded bool) map[string]string {
	docs := map[string]string{}
	for i := 0; i < events; i++ {
		month := 1 + i%benchMonths
		at := fmt.Sprintf("2026-%02d-15 09:00:00.000000", month)
		path := fmt.Sprintf("events/E%d.flowmark.md", i+1)
		if sharded {
			path = fmt.Sprintf("events/2026-%02d/E%d.flowmark.md", month, i+1)
		}
		docs[path] = lines(fmt.Sprintf("# Event %d", i+1), "- at: "+at)
	}
	return docs
}

// eventCatalog returns the catalog of the shard golden with the shard clause of events when sharded is true and
// without it otherwise.
func eventCatalog(b testing.TB, sharded bool) string {
	b.Helper()
	dbs := shardDBS(b)
	flat := "    store files shard at month\n"
	if !strings.Contains(dbs, flat) {
		b.Fatal("the shard golden no longer declares the shard of events")
	}
	if sharded {
		return dbs
	}
	return strings.Replace(dbs, flat, "    store files\n", 1)
}

// shardLayouts runs bench for the flat and the sharded layout, each at every number of events of
// benchFolderEvents, so the files of a folder show as a sweep.
func shardLayouts(b *testing.B, bench func(b *testing.B, f *fixture)) {
	for _, sharded := range []bool{false, true} {
		name := "flat"
		if sharded {
			name = "sharded"
		}
		for _, events := range benchFolderEvents {
			b.Run(fmt.Sprintf("%s/events=%d", name, events), func(b *testing.B) {
				bench(b, newFixture(b, eventCatalog(b, sharded), eventDocs(events, sharded)))
			})
		}
	}
}

// BenchmarkShardOpen is the opening of the event corpus, flat and sharded, with the descriptors its watch holds.
func BenchmarkShardOpen(b *testing.B) {
	shardLayouts(b, func(b *testing.B, f *fixture) {
		loop(b, func(int) {
			if err := openCorpus(b, f.root).Close(); err != nil {
				b.Fatal(err)
			}
		})
		c := openCorpus(b, f.root)
		b.ReportMetric(float64(openDescriptorCount(b)), "descriptors")
		if err := c.Close(); err != nil {
			b.Fatal(err)
		}
	})
}

// BenchmarkShardInsert is the insert of one event into the corpus kept in memory, flat and sharded.
func BenchmarkShardInsert(b *testing.B) {
	shardLayouts(b, benchShardInsert)
}

// benchShardInsert inserts one event per op into the corpus of f, and reports per op the overflows of the corpus tree
// (corpus.overflow) and the reads it made of the disk (corpus.read).
func benchShardInsert(b *testing.B, f *fixture) {
	s := f.session("bench")
	ctx := context.Background()
	f.rec.reset()
	loop(b, func(i int) {
		m := f.msg("insert", "events", map[string]any{"set": sets(0, "title", "at")}, fmt.Sprintf("Bench %d", i), "2026-10-20 10:00:00.000000")
		if _, err := s.Exec(ctx, m); err != nil {
			b.Fatal(err)
		}
	})
	events := f.rec.Events()
	b.ReportMetric(eventsPerOp(events, journal.CorpusOverflow, b.N), "overflows/op")
	b.ReportMetric(eventsPerOp(events, journal.CorpusRead, b.N), "tree-reads/op")
}

// eventsPerOp returns the number of events of kind among events, divided by ops.
func eventsPerOp(events []journal.Event, kind journal.Kind, ops int) float64 {
	n := 0
	for _, e := range events {
		if e.Kind == kind {
			n++
		}
	}
	return float64(n) / float64(ops)
}

// TestShardInsertMetrics: BenchmarkShardInsert reports the overflows and the tree reads of each insert, so a Windows
// run shows whether the corpus tree is read again whole. A forced overflow (the test seam of the tree's notices) is
// counted, and without one there is none; the count per insert is not pinned, since an insert refreshes the tree
// once before its lock and once under it (writepath.go read).
func TestShardInsertMetrics(t *testing.T) {
	for _, forced := range []bool{false, true} {
		r := testing.Benchmark(func(b *testing.B) {
			f := newFixture(b, eventCatalog(b, true), eventDocs(10, true))
			if forced {
				f.c.tree.SetNotices(func(journal.Notices) journal.Notices { return journal.Notices{Overflow: true} })
			}
			benchShardInsert(b, f)
		})
		overflows := r.Extra["overflows/op"]
		if forced && overflows <= 0 {
			t.Errorf("forced overflow: overflows/op = %v, want more than 0", overflows)
		}
		if !forced && overflows != 0 {
			t.Errorf("no forced overflow: overflows/op = %v, want 0", overflows)
		}
		if got := r.Extra["tree-reads/op"]; got <= 0 {
			t.Errorf("forced overflow %v: tree-reads/op = %v, want more than 0", forced, got)
		}
	}
}

// BenchmarkShardRead is a count of the events of the corpus kept in memory, flat and sharded.
func BenchmarkShardRead(b *testing.B) {
	shardLayouts(b, func(b *testing.B, f *fixture) {
		benchCount(b, f, "events")
	})
}
