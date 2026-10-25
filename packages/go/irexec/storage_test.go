package irexec

import (
	"slices"
	"strings"
	"testing"
)

// TestGoldenReads reads every table of every store golden corpus: a golden is an
// accepting case, so the executor answers each table's rows, as many as the store
// read, and counts them alike.
func TestGoldenReads(t *testing.T) {
	for _, name := range goldenRoots {
		f := goldenFixture(t, name)
		for ename, ent := range f.e.Model().Entities {
			f.sub(t, "60#surfaces: "+name+" "+ename, func(f *fixture) {
				res := f.ok(f.msg("all", ename, nil))
				if got, want := len(res.Answer.Rows), len(f.c.recs[ent.Table]); got != want {
					f.t.Fatalf("%d rows; the store read %d", got, want)
				}
				count := f.ok(f.msg("count", ename, nil))
				if count.Answer.Count != int64(len(res.Answer.Rows)) {
					f.t.Fatalf("count %d; all answered %d rows", count.Answer.Count, len(res.Answer.Rows))
				}
			})
		}
	}
}

// TestFilesStorageOrder pins the storage order of a files table whose tree is not
// whole (spec 20 Order, "Storage order"): it is total, so a read without an IR order
// never fails on a validator diagnostic of another row. The roots stand by sibling
// group, NULL parent first, then each dangling parent value in key order; the rows
// no root reaches follow in corpus order; tied or untyped order values fall back to
// corpus order.
func TestFilesStorageOrder(t *testing.T) {
	note := func(key, parent string, position string) string {
		ls := []string{"# " + key}
		if parent != "" {
			ls = append(ls, "- parent: ["+parent+"]("+parent+".flowmark.md)")
		}
		ls = append(ls, "- position: "+position)
		return lines(ls...)
	}
	corpus := func(notes map[string][2]string) map[string]string {
		docs := baseDocs()
		for p := range docs {
			if strings.HasPrefix(p, "notes/") {
				delete(docs, p)
			}
		}
		for k, v := range notes {
			docs["notes/"+k+".flowmark.md"] = note(k, v[0], v[1])
		}
		return docs
	}
	cases := []struct {
		name  string
		notes map[string][2]string
		want  []string
	}{
		{"20#order: a row whose parent names no row is a root after the NULL parent group",
			map[string][2]string{"A": {"ghost", "1"}, "B": {"A", "1"}, "C": {"", "1"}}, []string{"C", "A", "B"}},
		{"20#order: dangling parent groups stand in the key order of their parent value",
			map[string][2]string{"D": {"zz", "1"}, "E": {"aa", "1"}}, []string{"E", "D"}},
		{"20#order: rows on a cycle of parents across files follow the reached rows in corpus order",
			map[string][2]string{"X": {"Y", "1"}, "Y": {"X", "1"}, "Z": {"X", "2"}, "R": {"", "1"}}, []string{"R", "X", "Y", "Z"}},
		{"20#order: siblings sharing an order value stand in corpus order",
			map[string][2]string{"S1": {"", "2"}, "S2": {"", "1"}, "S3": {"", "1"}}, []string{"S2", "S3", "S1"}},
		{"20#order: a sibling whose order value is not of its type stands after the others",
			map[string][2]string{"S1": {"", "x"}, "S2": {"", "2"}, "S3": {"", "1"}}, []string{"S3", "S2", "S1"}},
		{"20#order: a row whose parent is not of its type and its subtree follow the reached rows",
			map[string][2]string{"A": {"ABCDEFGHIJKLMNOPQ", "1"}, "B": {"A", "1"}, "C": {"", "1"}}, []string{"C", "A", "B"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := catalogFixture(t, corpus(c.notes)).on(t)
			// The keys alone: a parent not of its type fails a read that returns it.
			res := f.ok(f.msg("all", "notes", map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"parent", "item"}}}))
			var got []string
			for _, r := range res.Answer.Rows {
				v, _ := r.Get("id")
				s, ok := v.(string)
				if !ok {
					t.Fatalf("the value %v is no string", v)
				}
				got = append(got, s)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("ids %v; want %v", got, c.want)
			}
		})
	}
}

// TestLimitCountsSoftDeleted pins a state limit over a soft-deleted row (spec 50
// The state machine, Limit): the soft-deleted row still counts in its state, so a
// soft delete frees no place and a restore never exceeds the limit.
func TestLimitCountsSoftDeleted(t *testing.T) {
	docs := map[string]string{
		"tasks.flowmark.md": lines("# tasks", "| id | state | title | deleted_at |", "| --- | --- | --- | --- |",
			"| A | doing | Alpha | 2026-10-05 00:00:00.000000 |", "| B | todo | Beta |  |"),
		"task_history.flowmark.md": lines("# task_history", "| id | task | from_state | to_state | at |", "| --- | --- | --- | --- | --- |",
			"| 1 | [A](tasks.flowmark.md) | todo | doing | 2026-10-01 00:00:00.000000 |"),
	}
	t.Run("50#the-state-machine: a restore into a state at its limit succeeds", func(t *testing.T) {
		f := catalogFixture(t, docs).on(t)
		f.commit(f.want(f.msg("restore", "tasks", map[string]any{"where": where(pred("id", "eq", p(0)))}, "A"), `{"affected":1,"keys":[]}`))
	})
	t.Run("50#the-state-machine: a soft-deleted row holds its place under the limit", func(t *testing.T) {
		f := catalogFixture(t, docs).on(t)
		err := f.failure(f.msg("update", "tasks", map[string]any{"set": sets("state"), "where": where(pred("id", "eq", p(1)))}, "doing", "B"), "CONSTRAINT")
		wantFaults(t, err, []dupFault{{"tasks", "B", -1, "state", "CONSTRAINT", "limit 1"}})
	})
}

// TestDocumentLifecycle pins a document table from its first insert to its last
// delete (spec 20 Storage): a missing document is an empty table, an insert creates
// it, and a delete of its last row leaves # <table> and one LF.
func TestDocumentLifecycle(t *testing.T) {
	f := catalogFixture(t, nil).on(t)
	f.want(f.msg("count", "tags", nil), `{"count":0}`)
	res := f.want(f.msg("insert", "tags", map[string]any{"set": sets("label", "kind")}, "go", "topic"), `{"affected":1,"keys":[1]}`)
	if tg := res.Post.Targets; len(tg) != 1 || tg[0].Before != nil || tg[0].Path != "tags.flowmark.md" {
		t.Fatalf("targets %+v", tg)
	}
	f.commit(res)
	f.commit(f.ok(f.msg("delete", "tags", map[string]any{"where": where(pred("id", "eq", p(0)))}, 1)))
	if got := f.file("tags.flowmark.md"); got != "# tags\n" {
		t.Fatalf("tags: %q", got)
	}
}

// TestDocumentHistory pins a state table whose history table is a document table
// (spec 30 The history table): a transition appends its history row there, and the
// history row restricts the delete of its state row.
func TestDocumentHistory(t *testing.T) {
	f := catalogFixture(t, nil).on(t)
	f.commit(f.want(f.msg("insert", "tickets", map[string]any{"set": sets("id", "state", "title")}, "K1", "open", "First"), `{"affected":1,"keys":["K1"]}`))
	res := f.ok(f.msg("update", "tickets", map[string]any{"set": sets("state"), "where": where(pred("id", "eq", p(1)))}, "closed", "K1"))
	f.commit(res)
	if got := f.file("ticket_history.flowmark.md"); !strings.Contains(got, "| 1 | [K1](tickets.flowmark.md) | open | closed | 2026-10-11 09:00:00.123456 |\n") {
		t.Fatalf("ticket_history:\n%s", got)
	}
	err := f.failure(f.msg("delete", "tickets", map[string]any{"where": where(pred("id", "eq", p(0)))}, "K1"), "FOREIGN_KEY")
	wantFaults(t, err, []dupFault{{"tickets", "K1", -1, "", "FOREIGN_KEY", "ticket_history 1"}})
	f.fails(f.msg("update", "tickets", map[string]any{"set": sets("state"), "where": where(pred("id", "eq", p(1)))}, "open", "K1"), "CONSTRAINT")
}
