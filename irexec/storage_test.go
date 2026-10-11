package irexec

import (
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
	err := f.fails(f.msg("delete", "tickets", map[string]any{"where": where(pred("id", "eq", p(0)))}, "K1"), "FOREIGN_KEY")
	wantFaults(t, err, []dupFault{{"tickets", "K1", -1, "", "FOREIGN_KEY", "ticket_history 1"}})
	f.fails(f.msg("update", "tickets", map[string]any{"set": sets("state"), "where": where(pred("id", "eq", p(1)))}, "open", "K1"), "CONSTRAINT")
}
