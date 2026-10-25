package writepath

import (
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shardDBS is the shard golden's catalog: events, a files table sharded by the month of a datetime, notes, a
// document table sharded by the year of a date, and refs, which link to events.
func shardDBS(t testing.TB) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(corpustest.ModuleRoot(t), "store", "testdata", "shard", "dbspec", "flowmark.dbs"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestShard pins the write path over a sharded corpus (spec 20 Time shards, spec 40): the journaled commit
// creates the period folders and documents, a move removes and creates files in one transaction, and a period
// folder that a hand edit adds is read by the next operation.
func TestShard(t *testing.T) {
	ins := func(f *fixture, entity string, cols []string, params ...any) []byte {
		return f.msg("insert", entity, map[string]any{"set": sets(0, cols...)}, params...)
	}
	t.Run("40#the-faces: install makes the folder of a sharded document table", func(t *testing.T) {
		f := newFixture(t, shardDBS(t), nil)
		r := f.installOf()
		if !f.isFolder("notes") || f.file("notes.flowmark.md") != "" {
			t.Fatalf("install %+v: the place of a sharded document table is a folder", r)
		}
		f.clean()
	})
	t.Run("20#time-shards: an insert creates the period folder, a move changes it, and the link follows", func(t *testing.T) {
		f := newFixture(t, shardDBS(t), nil)
		s := f.session("shard")
		want(t, s, ins(f, "events", []string{"title", "at"}, "First", "2026-10-11 09:30:00.000000"), `{"affected":1,"keys":["E1"]}`)
		want(t, s, ins(f, "refs", []string{"title", "event"}, "Ref one", "E1"), `{"affected":1,"keys":["R1"]}`)
		if f.file("events/2026-10/E1.flowmark.md") == "" {
			t.Fatal("the event is not in the folder of its month")
		}
		want(t, s, f.msg("update", "events", map[string]any{"set": []any{map[string]any{"column": "at", "p": 0}}, "where": corpustest.WhereID(1)},
			"2026-12-01 00:00:00.000000", "E1"), `{"affected":1,"keys":[]}`)
		if f.file("events/2026-10/E1.flowmark.md") != "" || f.file("events/2026-12/E1.flowmark.md") == "" {
			t.Fatal("the event did not move to the folder of its new month")
		}
		if got := f.file("refs/R1.flowmark.md"); !strings.Contains(got, "../events/2026-12/E1.flowmark.md") {
			t.Fatalf("the link did not follow the move:\n%s", got)
		}
		f.clean()
	})
	t.Run("20#time-shards: a period folder added by hand is read by the next operation", func(t *testing.T) {
		f := newFixture(t, shardDBS(t), nil)
		s := f.session("shard")
		want(t, s, ins(f, "events", []string{"title", "at"}, "First", "2026-10-11 09:30:00.000000"), `{"affected":1,"keys":["E1"]}`)
		f.write("events/2027-01/E7.flowmark.md", "# By hand\n- at: 2027-01-05 10:00:00.000000\n")
		want(t, s, f.msg("count", "events", nil), `{"count":2}`)
		f.clean()
	})
}
