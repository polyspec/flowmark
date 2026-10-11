package writepath

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// The benchmarks of a corpus of the same documents stored flat and sharded by month (spec 20 Time shards): the
// events table of the shard golden holds benchEvents events, a tenth of them in each of ten months, once as a files
// table in one folder and once with its shard clause. A sharded table keeps the number of documents; what its folders
// change is what the benchmarks show.

// benchEvents is the number of events of a benchmark corpus.
const benchEvents = 10000

// benchMonths is the number of months the events are spread over.
const benchMonths = 10

// eventDocs returns benchEvents events in files: in one folder when sharded is false, in the folder of their month
// when it is true. Event i is in month 1 + i%benchMonths of 2026.
func eventDocs(sharded bool) map[string]string {
	docs := map[string]string{}
	for i := 0; i < benchEvents; i++ {
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

// shardLayouts runs bench for the flat and the sharded layout.
func shardLayouts(b *testing.B, bench func(b *testing.B, f *fixture)) {
	for _, sharded := range []bool{false, true} {
		name := "flat"
		if sharded {
			name = "sharded"
		}
		b.Run(name, func(b *testing.B) {
			bench(b, newFixture(b, eventCatalog(b, sharded), eventDocs(sharded)))
		})
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
	shardLayouts(b, func(b *testing.B, f *fixture) {
		s := f.session("bench")
		ctx := context.Background()
		loop(b, func(i int) {
			m := f.msg("insert", "events", map[string]any{"set": sets(0, "title", "at")}, fmt.Sprintf("Bench %d", i), "2026-10-20 10:00:00.000000")
			if _, err := s.Exec(ctx, m); err != nil {
				b.Fatal(err)
			}
		})
	})
}

// BenchmarkShardRead is a count of the events of the corpus kept in memory, flat and sharded.
func BenchmarkShardRead(b *testing.B) {
	shardLayouts(b, func(b *testing.B, f *fixture) {
		s := f.session("bench")
		m := f.msg("count", "events", nil)
		ctx := context.Background()
		loop(b, func(int) {
			if _, err := s.Exec(ctx, m); err != nil {
				b.Fatal(err)
			}
		})
	})
}
