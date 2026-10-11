package writepath

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/irexec"
	"github.com/polyspec/flowmark/store/journal"
)

// TestSingleRequest pins one request outside a transaction (spec 40 The lock, Transactions): it is one transaction
// under the lock — a read takes and releases the lock, a write commits through the journal before the release, and a
// refused write writes nothing.
func TestSingleRequest(t *testing.T) {
	t.Run("40#the-lock: a read takes the lock and releases it", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("cli")
		want(t, s, f.countTags(), `{"count":2}`)
		f.rec.wantKinds(t, opened...)
	})
	t.Run("40#the-commit-protocol: a write commits through the journal under the lock", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("cli")
		want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[3]}`)
		f.rec.wantKinds(t, committed...)
		if !strings.HasSuffix(f.file("tags.flowmark.md"), "| 3 | web | area |\n") {
			t.Fatalf("tags:\n%s", f.file("tags.flowmark.md"))
		}
		f.clean()
	})
	t.Run("40#the-commit-protocol: a refused write writes nothing", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("cli")
		_, err := s.Exec(context.Background(), f.insertTag("new", "other"))
		fails(t, err, "CONSTRAINT", "ck_tags_kind")
		f.rec.wantKinds(t, opened...)
		if f.file("tags.flowmark.md") != baseDocs()["tags.flowmark.md"] {
			t.Fatalf("tags changed:\n%s", f.file("tags.flowmark.md"))
		}
	})
	t.Run("40#the-commit-protocol: a write that changes no byte writes nothing", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("cli")
		want(t, s, f.msg("restore", "people", map[string]any{"where": whereID(0)}, "P1"), `{"affected":0,"keys":[]}`)
		f.rec.wantKinds(t, opened...)
	})
	t.Run("40#transactions: a history row and an order move commit with their write", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("cli")
		want(t, s, f.msg("update", "items", map[string]any{"set": sets(0, "state"), "where": whereID(1)}, "doing", "T3"), `{"affected":1,"keys":[]}`)
		if !strings.Contains(f.file("items.flowmark.md"), "## items row T3\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n| 5 | waiting | doing | 2026-10-11 09:00:00.123456 |\n") {
			t.Fatalf("items:\n%s", f.file("items.flowmark.md"))
		}
		want(t, s, f.msg("update", "notes", map[string]any{"set": sets(0, "position"), "where": whereID(1)}, 1, "N3"), `{"affected":1,"keys":[]}`)
		if !strings.Contains(f.file("notes/N1.flowmark.md"), "- position: 2\n") || !strings.Contains(f.file("notes/N3.flowmark.md"), "- position: 1\n") {
			t.Fatalf("N1:\n%s\nN3:\n%s", f.file("notes/N1.flowmark.md"), f.file("notes/N3.flowmark.md"))
		}
		f.clean()
	})
}

// TestCreateMissing pins the insert that creates a missing folder or file (spec 40 The commit protocol, step 6): a
// missing table folder or document is an empty table, and the commit creates it.
func TestCreateMissing(t *testing.T) {
	f := newFixture(t, executorCatalog(t), nil)
	s := f.session("cli")
	want(t, s, f.msg("insert", "notes", map[string]any{"set": sets(0, "id", "title", "body")}, "N1", "First", "Text."), `{"affected":1,"keys":["N1"]}`)
	f.rec.wantKinds(t, committed...)
	if got := f.file("notes/N1.flowmark.md"); got != lines("# First", "- position: 1", "", "Text.") {
		t.Fatalf("N1:\n%s", got)
	}
	want(t, s, f.insertTag("go", "topic"), `{"affected":1,"keys":[1]}`)
	if got := f.file("tags.flowmark.md"); got != lines("# tags", "| id | label | kind |", "| --- | --- | --- |", "| 1 | go | topic |") {
		t.Fatalf("tags:\n%s", got)
	}
	f.clean()
}

// TestTransaction pins a transaction (spec 40 The lock, Transactions): begin takes the lock until commit or rollback,
// a request inside reads the transaction's post-image, the corpus changes only at commit, and another session's
// request meets the held lock.
func TestTransaction(t *testing.T) {
	t.Run("40#transactions: requests read the post-image and commit writes it once", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		if err := s.Begin(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[3]}`)
		want(t, s, f.insertTag("db", "area"), `{"affected":1,"keys":[4]}`)
		want(t, s, f.countTags(), `{"count":4}`)
		if f.file("tags.flowmark.md") != baseDocs()["tags.flowmark.md"] {
			t.Fatal("the corpus changed before the commit")
		}
		other := f.session("cli")
		_, err := other.Exec(context.Background(), f.countTags())
		fails(t, err, "CANCELED", "LOCK_TIMEOUT", s.Name())
		if we := writeErr(t, err); we.Face != "LOCK_TIMEOUT" {
			t.Fatalf("face %s, want LOCK_TIMEOUT", we.Face)
		}
		if err := s.Commit(); err != nil {
			t.Fatal(err)
		}
		f.rec.wantKinds(t, journal.LockAcquired, journal.Prepared, journal.Applied, journal.Collected, journal.LockReleased)
		if !strings.HasSuffix(f.file("tags.flowmark.md"), "| 3 | web | area |\n| 4 | db | area |\n") {
			t.Fatalf("tags:\n%s", f.file("tags.flowmark.md"))
		}
		want(t, other, f.countTags(), `{"count":4}`)
		f.clean()
	})
	t.Run("40#transactions: rollback writes nothing and releases the lock", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		if err := s.Begin(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[3]}`)
		if err := s.Rollback(); err != nil {
			t.Fatal(err)
		}
		f.rec.wantKinds(t, opened...)
		want(t, s, f.countTags(), `{"count":2}`)
	})
	t.Run("40#transactions: a failed request changes nothing and the transaction stays open", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		if err := s.Begin(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[3]}`)
		_, err := s.Exec(context.Background(), f.msg("insert", "tags", map[string]any{"set": sets(0, "label", "kind"), "rows": [][]int{{2, 3}}}, "db", "area", "bad", "other"))
		fails(t, err, "CONSTRAINT", "ck_tags_kind")
		want(t, s, f.countTags(), `{"count":3}`)
		if err := s.Commit(); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(f.file("tags.flowmark.md"), "| 2 | ops | area |\n| 3 | web | area |\n") {
			t.Fatalf("tags:\n%s", f.file("tags.flowmark.md"))
		}
	})
	t.Run("40#transactions: a read-only transaction takes the lock and refuses a write", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		if err := s.Begin(context.Background(), true); err != nil {
			t.Fatal(err)
		}
		want(t, s, f.countTags(), `{"count":2}`)
		_, err := s.Exec(context.Background(), f.insertTag("web", "area"))
		fails(t, err, "READ_ONLY", "read-only")
		_, err = s.Repair(context.Background(), "items", "T3", "")
		fails(t, err, "READ_ONLY", "read-only")
		if err := s.Commit(); err != nil {
			t.Fatal(err)
		}
		f.rec.wantKinds(t, opened...)
	})
	t.Run("40#transactions: a closed session rolls its transaction back and releases the lock", func(t *testing.T) {
		f := baseFixture(t)
		s, err := f.c.Session("server")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Begin(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[3]}`)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		f.rec.wantKinds(t, opened...)
		_, err = s.Exec(context.Background(), f.countTags())
		fails(t, err, "CONFIG", "closed")
		if err := s.Close(); err != nil {
			t.Fatalf("a second Close: %v", err)
		}
	})
	t.Run("40#the-lock: a closed connection ends the wait", func(t *testing.T) {
		f := baseFixture(t)
		f.c.wait = patience
		holder := f.session("server")
		if err := holder.Begin(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		waiter := f.session("cli")
		count := f.countTags()
		go func() {
			_, err := waiter.Exec(ctx, count)
			done <- err
		}()
		f.rec.waitFor(t, journal.LockWaiting)
		cancel()
		err := <-done
		fails(t, err, "CANCELED", "ended")
		if err := holder.Rollback(); err != nil {
			t.Fatal(err)
		}
	})
}

// TestInTransaction pins what the server's idle bound reads: whether a transaction is open, from its begin to its end
// by commit, rollback, a failed commit or Close.
func TestInTransaction(t *testing.T) {
	f := baseFixture(t)
	s := f.session("server")
	ctx := context.Background()
	open := func(want bool) {
		t.Helper()
		if got := s.InTransaction(); got != want {
			t.Fatalf("InTransaction() = %v, want %v", got, want)
		}
	}
	open(false)
	must(t, s.Begin(ctx, false))
	open(true)
	must(t, s.Commit())
	open(false)
	must(t, s.Begin(ctx, true))
	open(true)
	must(t, s.Rollback())
	open(false)
	must(t, s.Begin(ctx, false))
	must(t, s.Close())
	open(false)
}

// TestTransactionControlMisuse pins the refusals of the control messages (spec 40 Transactions): CONFIG, orm's code of
// a transaction misuse.
func TestTransactionControlMisuse(t *testing.T) {
	f := baseFixture(t)
	s := f.session("server")
	ctx := context.Background()
	fails(t, s.Commit(), "CONFIG", "no transaction")
	fails(t, s.Rollback(), "CONFIG", "no transaction")
	fails(t, s.Savepoint("a"), "CONFIG", "no transaction")
	fails(t, s.Release("a"), "CONFIG", "no transaction")
	fails(t, s.RollbackTo("a"), "CONFIG", "no transaction")
	if err := s.Begin(ctx, false); err != nil {
		t.Fatal(err)
	}
	fails(t, s.Begin(ctx, false), "CONFIG", "already")
	fails(t, s.Release("a"), "CONFIG", "no savepoint a")
	fails(t, s.RollbackTo("a"), "CONFIG", "no savepoint a")
	fails(t, s.Savepoint(""), "CONFIG", "name")
	_, err := s.Rewrite(ctx, func(*irexec.Corpus) ([]Rewrite, error) { return nil, nil }, nil)
	fails(t, err, "CONFIG", "own transaction")
	if err := s.Rollback(); err != nil {
		t.Fatal(err)
	}
	// In a read-only transaction a rewrite is CONFIG, never READ_ONLY: it never runs in a transaction it did not
	// begin.
	must(t, s.Begin(ctx, true))
	_, err = s.Rewrite(ctx, func(*irexec.Corpus) ([]Rewrite, error) { return nil, nil }, nil)
	fails(t, err, "CONFIG", "own transaction")
	must(t, s.Rollback())
	_, err = f.c.Session("a/b")
	fails(t, err, "CONFIG", "a/b")
}

// TestSavepoints pins the savepoints of a transaction (spec 40 Transactions): a savepoint is a nested level,
// rollback_to discards the levels above it and keeps it, release keeps their writes, and a savepoint of a name that
// exists replaces it (the SQL standard).
func TestSavepoints(t *testing.T) {
	f := baseFixture(t)
	s := f.session("server")
	ctx := context.Background()
	if err := s.Begin(ctx, false); err != nil {
		t.Fatal(err)
	}
	labels := func(j string) {
		t.Helper()
		want(t, s, f.msg("all", "tags", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"label"}}}), j)
	}
	want(t, s, f.insertTag("a", "area"), `{"affected":1,"keys":[3]}`)
	must(t, s.Savepoint("s1"))
	want(t, s, f.insertTag("b", "area"), `{"affected":1,"keys":[4]}`)
	must(t, s.Savepoint("s2"))
	want(t, s, f.insertTag("c", "area"), `{"affected":1,"keys":[5]}`)
	must(t, s.RollbackTo("s1"))
	labels(`{"rows":[{"id":1,"label":"go"},{"id":2,"label":"ops"},{"id":3,"label":"a"}]}`)
	fails(t, s.Release("s2"), "CONFIG", "no savepoint s2")
	want(t, s, f.insertTag("d", "area"), `{"affected":1,"keys":[4]}`)
	must(t, s.RollbackTo("s1"))
	labels(`{"rows":[{"id":1,"label":"go"},{"id":2,"label":"ops"},{"id":3,"label":"a"}]}`)
	want(t, s, f.insertTag("e", "area"), `{"affected":1,"keys":[4]}`)
	must(t, s.Release("s1"))
	fails(t, s.RollbackTo("s1"), "CONFIG", "no savepoint s1")
	// A savepoint of an existing name replaces it.
	must(t, s.Savepoint("x"))
	want(t, s, f.insertTag("f", "area"), `{"affected":1,"keys":[5]}`)
	must(t, s.Savepoint("x"))
	want(t, s, f.insertTag("g", "area"), `{"affected":1,"keys":[6]}`)
	must(t, s.RollbackTo("x"))
	labels(`{"rows":[{"id":1,"label":"go"},{"id":2,"label":"ops"},{"id":3,"label":"a"},{"id":4,"label":"e"},{"id":5,"label":"f"}]}`)
	must(t, s.Release("x"))
	fails(t, s.RollbackTo("x"), "CONFIG", "no savepoint x")
	must(t, s.Commit())
	if !strings.HasSuffix(f.file("tags.flowmark.md"), "| 3 | a | area |\n| 4 | e | area |\n| 5 | f | area |\n") {
		t.Fatalf("tags:\n%s", f.file("tags.flowmark.md"))
	}
	f.clean()
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestConflict pins step 1 of the commit protocol: a target changed since it was read fails WRITE_CONFLICT
// (CORPUS_CONFLICT), naming the file and the line, and is never overwritten; a change elsewhere that the write does
// not depend on commits.
func TestConflict(t *testing.T) {
	begin := func(t *testing.T) (*fixture, *Session) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		want(t, s, f.msg("update", "tags", map[string]any{"set": sets(0, "kind"), "where": whereID(1)}, "topic", 2), `{"affected":1,"keys":[]}`)
		return f, s
	}
	t.Run("40#the-commit-protocol: a target edited by hand", func(t *testing.T) {
		f, s := begin(t)
		edited := strings.Replace(baseDocs()["tags.flowmark.md"], "| 2 | ops | area |", "| 2 | ops | topic |", 1)
		f.write("tags.flowmark.md", edited)
		err := s.Commit()
		fails(t, err, "WRITE_CONFLICT", "tags.flowmark.md:5")
		we := writeErr(t, err)
		if we.Face != "CORPUS_CONFLICT" || len(we.Conflicts) != 1 || we.Conflicts[0] != (Conflict{File: "tags.flowmark.md", Line: 5}) {
			t.Fatalf("error %+v", we)
		}
		if f.file("tags.flowmark.md") != edited {
			t.Fatal("the edit was overwritten")
		}
		f.rec.wantKinds(t, opened...)
		fails(t, s.Commit(), "CONFIG", "no transaction")
	})
	t.Run("40#the-commit-protocol: a target removed by hand", func(t *testing.T) {
		f, s := begin(t)
		f.remove("tags.flowmark.md")
		err := s.Commit()
		fails(t, err, "WRITE_CONFLICT", "tags.flowmark.md:1")
	})
	t.Run("40#the-commit-protocol: a target created by hand", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		want(t, s, f.msg("insert", "notes", map[string]any{"set": sets(0, "id", "title", "body")}, "N4", "Fourth", ""), `{"affected":1,"keys":["N4"]}`)
		f.write("notes/N4.flowmark.md", lines("# Mine", "- position: 3"))
		fails(t, s.Commit(), "WRITE_CONFLICT", "notes/N4.flowmark.md:1")
		if f.file("notes/N4.flowmark.md") != lines("# Mine", "- position: 3") {
			t.Fatal("the edit was overwritten")
		}
	})
	t.Run("40#the-commit-protocol: an edit of another file that the write does not depend on", func(t *testing.T) {
		f, s := begin(t)
		f.write("notes/N3.flowmark.md", lines("# Second root, edited", "- position: 2"))
		must(t, s.Commit())
		if !strings.Contains(f.file("tags.flowmark.md"), "| 2 | ops | topic |") || !strings.Contains(f.file("notes/N3.flowmark.md"), "edited") {
			t.Fatal("the commit lost a change")
		}
		f.rec.wantKinds(t, committed...)
		f.clean()
	})
}

// TestCommitChecks pins step 2 of the commit protocol: the constraint checks run on the corpus re-read under the lock
// with the post-image applied, by running the transaction's writes again on it; a write they refuse fails the commit
// with its code, and a write that the change makes another write fails WRITE_CONFLICT naming the changed file.
func TestCommitChecks(t *testing.T) {
	t.Run("40#the-commit-protocol: a referenced row removed by hand fails the commit", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		want(t, s, f.msg("insert", "items", map[string]any{"set": sets(0, "title", "body", "owner")}, "Owned", "", "P2"), `{"affected":1,"keys":["T5"]}`)
		f.write("people.flowmark.md", strings.Replace(baseDocs()["people.flowmark.md"], "| P2 | Max |  |  | false | 0.00 | 2026-10-02 10:00:00 |  |  |\n", "", 1))
		fails(t, s.Commit(), "FOREIGN_KEY", "P2")
		if f.file("items.flowmark.md") != baseDocs()["items.flowmark.md"] {
			t.Fatal("the refused commit wrote")
		}
		f.rec.wantKinds(t, opened...)
	})
	t.Run("40#the-commit-protocol: a key taken by hand fails the commit", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		want(t, s, f.msg("insert", "steps", map[string]any{"set": sets(0, "id", "note", "title", "done")}, "S3", "N1", "Ship", false), `{"affected":1,"keys":["S3"]}`)
		f.write("notes/N3.flowmark.md", lines("# Second root", "- position: 2", "", "## steps", "- s3 Mine done:false"))
		fails(t, s.Commit(), "DUPLICATE_KEY", "s3")
	})
	t.Run("40#the-commit-protocol: a change that makes the write another write", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		upd := f.msg("update", "notes", map[string]any{"set": sets(0, "weight"), "where": map[string]any{"items": []any{map[string]any{"pred": map[string]any{"column": "position", "op": "gte", "p": 1}}}}}, 1.5, 2)
		want(t, s, upd, `{"affected":1,"keys":[]}`)
		f.write("notes/N4.flowmark.md", lines("# Third root", "- position: 3"))
		err := s.Commit()
		fails(t, err, "WRITE_CONFLICT", "notes/N4.flowmark.md:1")
		if we := writeErr(t, err); we.Face != "CORPUS_CONFLICT" {
			t.Fatalf("face %s", we.Face)
		}
		if strings.Contains(f.file("notes/N3.flowmark.md"), "weight") {
			t.Fatal("the refused commit wrote")
		}
	})
	t.Run("40#the-commit-protocol: a change the writes do not depend on commits them", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		want(t, s, f.msg("insert", "items", map[string]any{"set": sets(0, "title", "body", "owner")}, "Owned", "", "P2"), `{"affected":1,"keys":["T5"]}`)
		f.write("people.flowmark.md", baseDocs()["people.flowmark.md"]+"| P4 | Lin |  |  | true | 0.00 | 2026-10-04 10:00:00 |  |  |\n")
		must(t, s.Commit())
		f.clean()
	})
}

// TestSingleRequestRetry pins the retry of a single-operation write (spec 40 Transactions): a commit that meets
// CORPUS_CONFLICT is retried once after a re-read, still under the lock; a second conflict is returned.
func TestSingleRequestRetry(t *testing.T) {
	edit := func(f *fixture, attempts ...int) {
		f.c.readHook = func(attempt int) {
			for _, a := range attempts {
				if a == attempt {
					text := f.file("tags.flowmark.md")
					f.write("tags.flowmark.md", text+"| "+string(rune('7'+attempt))+" | hand"+string(rune('0'+attempt))+" | area |\n")
				}
			}
		}
	}
	t.Run("40#transactions: one conflict is retried after a re-read", func(t *testing.T) {
		f := baseFixture(t)
		edit(f, 0)
		s := f.session("cli")
		want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[8]}`)
		if !strings.HasSuffix(f.file("tags.flowmark.md"), "| 7 | hand0 | area |\n| 8 | web | area |\n") {
			t.Fatalf("tags:\n%s", f.file("tags.flowmark.md"))
		}
		f.rec.wantKinds(t, committed...)
	})
	t.Run("40#transactions: a second conflict is returned", func(t *testing.T) {
		f := baseFixture(t)
		edit(f, 0, 1)
		s := f.session("cli")
		_, err := s.Exec(context.Background(), f.insertTag("web", "area"))
		fails(t, err, "WRITE_CONFLICT", "tags.flowmark.md:7")
		f.rec.wantKinds(t, opened...)
	})
}

// TestRewriteRefusal pins the refusal of a commit that must rewrite a file with a format.* diagnostic (spec 40 The
// commit protocol, step 2; spec 10 Encoding and line structure): CONSTRAINT naming the file and the diagnostic, and
// flowmark canon for format.noncanonical — the row's own file, an order sibling, a file holding a link to a row whose
// detail section appears — at the request, inside a transaction too.
func TestRewriteRefusal(t *testing.T) {
	noncanonical := func(p string) map[string]string {
		docs := baseDocs()
		docs[p] += "\n"
		return docs
	}
	cases := []struct {
		name string
		docs map[string]string
		msg  func(f *fixture) []byte
		file string
	}{
		{"10#encoding-and-line-structure: the row's own file", noncanonical("tags.flowmark.md"),
			func(f *fixture) []byte { return f.insertTag("web", "area") }, "tags.flowmark.md"},
		{"10#encoding-and-line-structure: an order sibling", noncanonical("notes/N1.flowmark.md"),
			func(f *fixture) []byte {
				return f.msg("update", "notes", map[string]any{"set": sets(0, "position"), "where": whereID(1)}, 1, "N3")
			}, "notes/N1.flowmark.md"},
		{"10#encoding-and-line-structure: a file holding a link to a row whose detail section appears", func() map[string]string {
			docs := baseDocs()
			docs["notes/N3.flowmark.md"] = lines("# Second root", "- item: [T3](../items.flowmark.md)", "- position: 2") + "\n"
			return docs
		}(), func(f *fixture) []byte {
			return f.msg("update", "items", map[string]any{"set": sets(0, "body"), "where": whereID(1)}, "Now a body.", "T3")
		}, "notes/N3.flowmark.md"},
		{"10#encoding-and-line-structure: a file holding a dangling link to the row an insert gives", func() map[string]string {
			docs := baseDocs()
			docs["notes/N3.flowmark.md"] = lines("# Second root", "- item: [T9](x.md)", "- position: 2") + "\n"
			return docs
		}(), func(f *fixture) []byte {
			return f.msg("insert", "items", map[string]any{"set": sets(0, "id", "title", "body")}, "T9", "Ninth", "")
		}, "notes/N3.flowmark.md"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, executorCatalog(t), c.docs)
			s := f.session("cli")
			_, err := s.Exec(context.Background(), c.msg(f))
			fails(t, err, "CONSTRAINT", c.file, "format.noncanonical", "flowmark canon")
			if we := writeErr(t, err); we.Diagnostic == nil || we.Diagnostic.File != c.file || we.Diagnostic.Rule != "format.noncanonical" {
				t.Fatalf("error %+v", we)
			}
			f.rec.wantKinds(t, opened...)
			if f.file(c.file) != c.docs[c.file] {
				t.Fatal("the refused write rewrote the file")
			}
			// Inside a transaction the request fails and the transaction stays open.
			must(t, s.Begin(context.Background(), false))
			_, err = s.Exec(context.Background(), c.msg(f))
			fails(t, err, "CONSTRAINT", c.file)
			want(t, s, f.countTags(), `{"count":2}`)
			must(t, s.Rollback())
		})
	}
	t.Run("10#encoding-and-line-structure: the same writes on canonical files commit", func(t *testing.T) {
		docs := baseDocs()
		docs["notes/N3.flowmark.md"] = lines("# Second root", "- item: [T3](../items.flowmark.md)", "- position: 2")
		f := newFixture(t, executorCatalog(t), docs)
		s := f.session("cli")
		want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[3]}`)
		want(t, s, f.msg("update", "notes", map[string]any{"set": sets(0, "position"), "where": whereID(1)}, 1, "N3"), `{"affected":1,"keys":[]}`)
		want(t, s, f.msg("update", "items", map[string]any{"set": sets(0, "body"), "where": whereID(1)}, "Now a body.", "T3"), `{"affected":1,"keys":[]}`)
		if !strings.Contains(f.file("notes/N3.flowmark.md"), "[T3](../items.flowmark.md#items-row-t3)") {
			t.Fatalf("N3:\n%s", f.file("notes/N3.flowmark.md"))
		}
		f.clean()
	})
	t.Run("10#diagnostics: a write into an unavailable table writes nothing", func(t *testing.T) {
		docs := baseDocs()
		docs["tags.flowmark.md"] = "no heading\n"
		f := newFixture(t, executorCatalog(t), docs)
		s := f.session("cli")
		_, err := s.Exec(context.Background(), f.insertTag("web", "area"))
		fails(t, err, "CONSTRAINT", "tags.flowmark.md", "unavailable")
		f.rec.wantKinds(t, opened...)
	})
}

// TestWholeFileDelete pins the delete that removes a files row's whole file (spec 40 The commit protocol, step 2): it
// is exempt from the rewrite refusal, and its restrict check needs the rows of every table that may reference the row.
func TestWholeFileDelete(t *testing.T) {
	cards := func(t *testing.T, docs map[string]string) (*fixture, *Session) {
		f := newFixture(t, cardsDBS, docs)
		return f, f.session("cli")
	}
	del := func(f *fixture, key string) []byte {
		return f.msg("delete", "cards", map[string]any{"where": whereID(0)}, key)
	}
	t.Run("10#encoding-and-line-structure: a whole-file delete of a noncanonical file", func(t *testing.T) {
		f, s := cards(t, map[string]string{"cards/C1.flowmark.md": "# One\n\n"})
		want(t, s, del(f, "C1"), `{"affected":1,"keys":[]}`)
		f.rec.wantKinds(t, committed...)
		if f.file("cards/C1.flowmark.md") != "" {
			t.Fatal("the file stays")
		}
		f.clean()
	})
	t.Run("50#keys-and-references: a whole-file delete of a referenced row", func(t *testing.T) {
		f, s := cards(t, map[string]string{
			"cards/C1.flowmark.md": "# One\n\n",
			"pins.flowmark.md":     lines("# pins", "| id | card |", "| --- | --- |", "| X1 | [C1](cards/C1.flowmark.md) |"),
		})
		_, err := s.Exec(context.Background(), del(f, "C1"))
		fails(t, err, "FOREIGN_KEY", "pins X1")
		f.rec.wantKinds(t, opened...)
	})
	t.Run("10#encoding-and-line-structure: a whole-file delete whose restrict check needs an unavailable table", func(t *testing.T) {
		f, s := cards(t, map[string]string{
			"cards/C1.flowmark.md": "# One\n",
			"pins.flowmark.md":     "no heading\n",
		})
		_, err := s.Exec(context.Background(), del(f, "C1"))
		fails(t, err, "CONSTRAINT", "pins.flowmark.md")
		f.rec.wantKinds(t, opened...)
		if f.file("cards/C1.flowmark.md") != "# One\n" {
			t.Fatal("the refused delete removed the file")
		}
	})
}

// TestRewrite pins the commit of files rewritten outside the executor — flowmark canon and the add-column migration —
// and their exemptions (spec 40 The commit protocol, step 2): canon's targets are exempt, the migration's for the
// diagnostics the added column causes, and any other format.* diagnostic refuses.
func TestRewrite(t *testing.T) {
	ctx := context.Background()
	canonical := baseDocs()["people.flowmark.md"]
	docs := baseDocs()
	docs["people.flowmark.md"] = canonical + "\n"
	// The tags table without its kind column: the format.block of a missing header that the migration of an added
	// column is exempt for.
	docs["tags.flowmark.md"] = lines("# tags", "| id | label |", "| --- | --- |", "| 1 | go |", "| 2 | ops |")
	docs["notes/N3.flowmark.md"] = "# Second root\n- position: \xff2\n"
	rewrite := func(p, text string) func(*irexec.Corpus) ([]Rewrite, error) {
		return func(c *irexec.Corpus) ([]Rewrite, error) { return []Rewrite{{Path: p, After: []byte(text)}}, nil }
	}
	every := func(string, catalog.Diagnostic) bool { return true }
	blockOnly := func(_ string, d catalog.Diagnostic) bool { return d.Rule == "format.block" }
	t.Run("40#the-canon-operation: canon's targets are exempt", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		s := f.session("canon")
		n, err := s.Rewrite(ctx, rewrite("people.flowmark.md", canonical), every)
		if err != nil || n != 1 {
			t.Fatalf("Rewrite = %d, %v", n, err)
		}
		f.rec.wantKinds(t, committed...)
		if f.file("people.flowmark.md") != canonical {
			t.Fatal("not rewritten")
		}
	})
	t.Run("40#the-commit-protocol: a rewrite without the exemption is refused, naming canon", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		s := f.session("canon")
		_, err := s.Rewrite(ctx, rewrite("people.flowmark.md", canonical), nil)
		fails(t, err, "CONSTRAINT", "people.flowmark.md", "format.noncanonical", "flowmark canon")
		f.rec.wantKinds(t, opened...)
	})
	t.Run("80#scope: the migration's targets are exempt for the diagnostics the added column causes", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		s := f.session("migrate")
		if _, err := s.Rewrite(ctx, rewrite("tags.flowmark.md", baseDocs()["tags.flowmark.md"]), blockOnly); err != nil {
			t.Fatal(err)
		}
		if f.file("tags.flowmark.md") != baseDocs()["tags.flowmark.md"] {
			t.Fatal("not rewritten")
		}
	})
	t.Run("80#scope: any other format diagnostic in a migration target refuses it", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		s := f.session("migrate")
		_, err := s.Rewrite(ctx, rewrite("people.flowmark.md", canonical), blockOnly)
		fails(t, err, "CONSTRAINT", "people.flowmark.md", "format.noncanonical", "flowmark canon")
		_, err = s.Rewrite(ctx, rewrite("notes/N3.flowmark.md", lines("# Second root", "- position: 2")), blockOnly)
		fails(t, err, "CONSTRAINT", "notes/N3.flowmark.md", "format.encoding")
		if strings.Contains(err.Error(), "flowmark canon") {
			t.Fatalf("a format.encoding refusal names canon: %v", err)
		}
	})
	t.Run("40#the-commit-protocol: a rewrite whose target changed during it is retried once", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		s := f.session("canon")
		f.c.readHook = func(attempt int) {
			if attempt == 0 {
				f.write("people.flowmark.md", canonical+"\n\n")
			}
		}
		calls := 0
		n, err := s.Rewrite(ctx, func(c *irexec.Corpus) ([]Rewrite, error) {
			calls++
			return []Rewrite{{Path: "people.flowmark.md", After: []byte(canonical)}}, nil
		}, every)
		if err != nil || n != 1 || f.file("people.flowmark.md") != canonical {
			t.Fatalf("Rewrite = %d, %v", n, err)
		}
		if calls != 2 {
			t.Fatalf("compute ran %d times, want 2: the read, and the retry after the commit met the edit", calls)
		}
	})
	t.Run("40#the-commit-protocol: a computation that fails writes nothing", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), docs)
		s := f.session("canon")
		boom := errors.New("boom")
		_, err := s.Rewrite(ctx, func(*irexec.Corpus) ([]Rewrite, error) { return nil, boom }, every)
		if !errors.Is(err, boom) {
			t.Fatalf("Rewrite = %v, want the computation's error", err)
		}
		f.rec.wantKinds(t, opened...)
	})
}

// TestRepairCommit pins the history repair as a write (spec 50 Repair): a normal transaction that commits its history
// row, inside a transaction too, refused when it must rewrite a noncanonical file.
func TestRepairCommit(t *testing.T) {
	gapped := func() map[string]string {
		docs := baseDocs()
		docs["items.flowmark.md"] = strings.Replace(docs["items.flowmark.md"], "- [ ] T3 Review the design", "- [~] T3 Review the design", 1)
		return docs
	}
	t.Run("50#a-state-changed-outside-the-engine: the repair commits its history row", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), gapped())
		s := f.session("cli")
		a, err := s.Repair(context.Background(), "items", "T3", "")
		if err != nil {
			t.Fatal(err)
		}
		if got := answerJSON(t, a); got != `{"affected":1,"keys":[5]}` {
			t.Fatalf("answer %s", got)
		}
		f.rec.wantKinds(t, committed...)
		if !strings.Contains(f.file("items.flowmark.md"), "| 5 | waiting | doing | 2026-10-11 10:00:00.123456 |") {
			t.Fatalf("items:\n%s", f.file("items.flowmark.md"))
		}
		f.clean()
	})
	t.Run("50#a-state-changed-outside-the-engine: inside a transaction, then a transition of the repaired row", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), gapped())
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		if _, err := s.Repair(context.Background(), "items", "T3", ""); err != nil {
			t.Fatal(err)
		}
		want(t, s, f.msg("update", "items", map[string]any{"set": sets(0, "state", "evidence"), "where": whereID(2)}, "done", "Checked.", "T3"), `{"affected":1,"keys":[]}`)
		must(t, s.Commit())
		f.clean()
	})
	t.Run("50#a-state-changed-outside-the-engine: a repair that must rewrite a noncanonical file", func(t *testing.T) {
		docs := gapped()
		docs["items.flowmark.md"] += "\n"
		f := newFixture(t, executorCatalog(t), docs)
		s := f.session("cli")
		_, err := s.Repair(context.Background(), "items", "T3", "")
		fails(t, err, "CONSTRAINT", "items.flowmark.md", "flowmark canon")
	})
	t.Run("50#a-state-changed-outside-the-engine: a refused repair", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("cli")
		_, err := s.Repair(context.Background(), "items", "T1", "")
		fails(t, err, "CONSTRAINT", "no history gap")
		f.rec.wantKinds(t, opened...)
	})
}
