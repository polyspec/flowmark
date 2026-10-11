package irexec

import (
	"strings"
	"testing"
)

// TestShardInsert pins where an insert places a row of a sharded table (spec 20 Time shards): in the file
// or the document of the period its shard column names, created when the period has none.
func TestShardInsert(t *testing.T) {
	t.Run("20#time-shards: a files row in the folder of its month", func(t *testing.T) {
		f := goldenFixture(t, "shard")
		res := f.want(f.msg("insert", "events", map[string]any{"set": sets("title", "at")}, "First", "2026-10-11 09:30:00.000000"), `{"affected":1,"keys":["E1"]}`)
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].Path != "events/2026-10/E1.flowmark.md" {
			t.Fatalf("targets %+v", res.Post.Targets)
		}
		f.commit(res)
		res = f.want(f.msg("insert", "events", map[string]any{"set": sets("title", "at")}, "Second", "2026-11-02 08:00:00.000000"), `{"affected":1,"keys":["E2"]}`)
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].Path != "events/2026-11/E2.flowmark.md" {
			t.Fatalf("targets %+v", res.Post.Targets)
		}
		f.commit(res)
	})
	t.Run("20#time-shards: a document row in the document of its year, created when the year has none", func(t *testing.T) {
		f := goldenFixture(t, "shard")
		res := f.want(f.msg("insert", "notes", map[string]any{"set": sets("title", "written")}, "One", "2026-03-04"), `{"affected":1,"keys":["N1"]}`)
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].Path != "notes/2026.flowmark.md" || res.Post.Targets[0].Before != nil {
			t.Fatalf("targets %+v", res.Post.Targets)
		}
		f.commit(res)
		res = f.want(f.msg("insert", "notes", map[string]any{"set": sets("title", "written")}, "Two", "2026-09-30"), `{"affected":1,"keys":["N2"]}`)
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].Path != "notes/2026.flowmark.md" || res.Post.Targets[0].Before == nil {
			t.Fatalf("a second note of the year rewrites its document: %+v", res.Post.Targets)
		}
		f.commit(res)
		res = f.want(f.msg("insert", "notes", map[string]any{"set": sets("title", "written")}, "Three", "2027-01-02"), `{"affected":1,"keys":["N3"]}`)
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].Path != "notes/2027.flowmark.md" {
			t.Fatalf("targets %+v", res.Post.Targets)
		}
		f.commit(res)
		if !strings.HasPrefix(f.file("notes/2027.flowmark.md"), "# notes\n") {
			t.Fatalf("the document of a new period begins with the table's H1:\n%s", f.file("notes/2027.flowmark.md"))
		}
	})
	t.Run("20#time-shards: a value that is no time is refused before the row is placed", func(t *testing.T) {
		f := goldenFixture(t, "shard")
		err := f.fails(f.msg("insert", "events", map[string]any{"set": sets("title", "at")}, "Bad", "soon"), "CONSTRAINT")
		if len(err.Faults) != 1 || err.Faults[0].Column != "at" {
			t.Fatalf("faults %+v; want the one fault of the column at", err.Faults)
		}
	})
	t.Run("20#time-shards: a row without its shard column is refused as a required column is", func(t *testing.T) {
		f := goldenFixture(t, "shard")
		f.fails(f.msg("insert", "events", map[string]any{"set": sets("title")}, "No time"), "IR_INVALID")
	})
}

// shardEvents inserts two events of the shard golden, E1 in October and E2 in November of 2026, and a ref to E1.
func shardEvents(t *testing.T) *fixture {
	t.Helper()
	f := goldenFixture(t, "shard")
	f.commit(f.ok(f.msg("insert", "events", map[string]any{"set": sets("title", "at")}, "First", "2026-10-11 09:30:00.000000")))
	f.commit(f.ok(f.msg("insert", "events", map[string]any{"set": sets("title", "at")}, "Second", "2026-11-02 08:00:00.000000")))
	f.commit(f.ok(f.msg("insert", "refs", map[string]any{"set": sets("title", "event")}, "Ref one", "E1")))
	return f
}

// TestShardUpdate pins the update of a shard column (spec 20 Time shards): the row moves to the file of its new
// period, and every file that links to it is rewritten in the same write.
func TestShardUpdate(t *testing.T) {
	update := func(f *fixture, entity, key, column string, value any) *Result {
		return f.ok(f.msg("update", entity, map[string]any{"set": []any{set(column, 0)}, "where": where(pred("id", "eq", map[string]any{"p": 1}))}, value, key))
	}
	paths := func(res *Result) map[string]bool {
		out := map[string]bool{}
		for _, tg := range res.Post.Targets {
			out[tg.Path] = tg.After != nil
		}
		return out
	}
	t.Run("20#time-shards: an update of another column leaves the row where it is", func(t *testing.T) {
		f := shardEvents(t)
		res := update(f, "events", "E1", "title", "Renamed")
		if got := paths(res); len(got) != 1 || !got["events/2026-10/E1.flowmark.md"] {
			t.Fatalf("targets %v", got)
		}
		f.commit(res)
	})
	t.Run("20#time-shards: an update within the period leaves the row where it is", func(t *testing.T) {
		f := shardEvents(t)
		res := update(f, "events", "E1", "at", "2026-10-30 10:00:00.000000")
		if got := paths(res); len(got) != 1 || !got["events/2026-10/E1.flowmark.md"] {
			t.Fatalf("targets %v", got)
		}
		f.commit(res)
	})
	t.Run("20#time-shards: an update into another month moves the file and rewrites the link to it", func(t *testing.T) {
		f := shardEvents(t)
		res := update(f, "events", "E1", "at", "2026-12-01 00:00:00.000000")
		want := map[string]bool{"events/2026-10/E1.flowmark.md": false, "events/2026-12/E1.flowmark.md": true, "refs/R1.flowmark.md": true}
		if got := paths(res); len(got) != len(want) || got["events/2026-10/E1.flowmark.md"] || !got["events/2026-12/E1.flowmark.md"] || !got["refs/R1.flowmark.md"] {
			t.Fatalf("targets %v; want %v", got, want)
		}
		f.commit(res)
		if !strings.Contains(f.file("refs/R1.flowmark.md"), "../events/2026-12/E1.flowmark.md") {
			t.Fatalf("the link to the moved row:\n%s", f.file("refs/R1.flowmark.md"))
		}
	})
	t.Run("20#time-shards: an update into the folder of a month that has a row joins it", func(t *testing.T) {
		f := shardEvents(t)
		res := update(f, "events", "E1", "at", "2026-11-20 12:00:00.000000")
		if got := paths(res); !got["events/2026-11/E1.flowmark.md"] || got["events/2026-10/E1.flowmark.md"] {
			t.Fatalf("targets %v", got)
		}
		f.commit(res)
	})
	t.Run("20#time-shards: an update moves a document row, and the document it empties is removed", func(t *testing.T) {
		f := goldenFixture(t, "shard")
		f.commit(f.ok(f.msg("insert", "notes", map[string]any{"set": sets("title", "written")}, "One", "2026-03-04")))
		f.commit(f.ok(f.msg("insert", "notes", map[string]any{"set": sets("title", "written")}, "Two", "2026-09-30")))
		res := update(f, "notes", "N1", "written", "2027-02-01")
		got := paths(res)
		if !got["notes/2026.flowmark.md"] || !got["notes/2027.flowmark.md"] {
			t.Fatalf("a move between documents rewrites both: %v", got)
		}
		f.commit(res)
		res = update(f, "notes", "N2", "written", "2027-05-05")
		got = paths(res)
		if got["notes/2026.flowmark.md"] || !got["notes/2027.flowmark.md"] || len(got) != 2 {
			t.Fatalf("the emptied document is removed: %v", got)
		}
		f.commit(res)
	})
}

// TestShardDelete pins the delete of a row of a sharded table: a files row's file is removed, and the last row of
// a sharded document removes the document, since a period without rows has no file.
func TestShardDelete(t *testing.T) {
	del := func(f *fixture, entity, key string) *Result {
		return f.ok(f.msg("delete", entity, map[string]any{"where": where(pred("id", "eq", map[string]any{"p": 0}))}, key))
	}
	t.Run("20#time-shards: a files row's file is removed and its folder stays empty of files", func(t *testing.T) {
		f := goldenFixture(t, "shard")
		f.commit(f.ok(f.msg("insert", "events", map[string]any{"set": sets("title", "at")}, "First", "2026-10-11 09:30:00.000000")))
		res := del(f, "events", "E1")
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].Path != "events/2026-10/E1.flowmark.md" || res.Post.Targets[0].After != nil {
			t.Fatalf("targets %+v", res.Post.Targets)
		}
		f.commit(res)
	})
	t.Run("20#time-shards: the last row of a sharded document removes the document, an earlier one does not", func(t *testing.T) {
		f := goldenFixture(t, "shard")
		f.commit(f.ok(f.msg("insert", "notes", map[string]any{"set": sets("title", "written")}, "One", "2026-03-04")))
		f.commit(f.ok(f.msg("insert", "notes", map[string]any{"set": sets("title", "written")}, "Two", "2026-09-30")))
		res := del(f, "notes", "N1")
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].After == nil {
			t.Fatalf("the document keeps its other row: %+v", res.Post.Targets)
		}
		f.commit(res)
		res = del(f, "notes", "N2")
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].Path != "notes/2026.flowmark.md" || res.Post.Targets[0].After != nil {
			t.Fatalf("the emptied document is removed: %+v", res.Post.Targets)
		}
		f.commit(res)
	})
}

// TestShardOrder pins the storage order of a sharded table (spec 20 Time shards): a shard changes where the rows
// stand and never the order the table reads in. A files table keeps its key order whatever the periods are; a
// document table reads its periods ascending, each document in its own order.
func TestShardOrder(t *testing.T) {
	ids := func(f *fixture, entity string) string {
		return answer(f.t, f.ok(f.msg("all", entity, map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"id"}}})))
	}
	t.Run("20#time-shards: a files table reads in key order across periods", func(t *testing.T) {
		f := goldenFixture(t, "shard")
		// E1 is in November and E2 in October, so period order would give E2 first.
		f.commit(f.ok(f.msg("insert", "events", map[string]any{"set": sets("title", "at")}, "First", "2026-11-02 08:00:00.000000")))
		f.commit(f.ok(f.msg("insert", "events", map[string]any{"set": sets("title", "at")}, "Second", "2026-10-11 09:30:00.000000")))
		if got, want := ids(f, "events"), `{"rows":[{"id":"E1"},{"id":"E2"}]}`; got != want {
			t.Fatalf("events read %s; want %s", got, want)
		}
	})
	t.Run("20#time-shards: a document table reads its periods ascending", func(t *testing.T) {
		f := goldenFixture(t, "shard")
		// N1 is in 2027 and N2 in 2026, so the document of 2026 reads first.
		f.commit(f.ok(f.msg("insert", "notes", map[string]any{"set": sets("title", "written")}, "One", "2027-01-02")))
		f.commit(f.ok(f.msg("insert", "notes", map[string]any{"set": sets("title", "written")}, "Two", "2026-03-04")))
		if got, want := ids(f, "notes"), `{"rows":[{"id":"N2"},{"id":"N1"}]}`; got != want {
			t.Fatalf("notes read %s; want %s", got, want)
		}
	})
}
