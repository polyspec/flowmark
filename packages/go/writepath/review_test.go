package writepath

import (
	"context"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"strings"
	"testing"
	"time"

	"github.com/polyspec/flowmark/packages/go/store/journal"
)

// TestReleaseDropsLaterSavepoints pins that release removes the savepoints set after the one it names: rollback_to
// one of them is CONFIG.
func TestReleaseDropsLaterSavepoints(t *testing.T) {
	f := baseFixture(t)
	s := f.session("server")
	must(t, s.Begin(context.Background(), false))
	must(t, s.Savepoint("s1"))
	want(t, s, f.insertTag("a", "area"), `{"affected":1,"keys":[3]}`)
	must(t, s.Savepoint("s2"))
	must(t, s.Release("s1"))
	fails(t, s.RollbackTo("s2"), "CONFIG", "no savepoint s2")
	want(t, s, f.countTags(), `{"count":3}`)
	must(t, s.Rollback())
}

// TestRecheckDivergence pins each way the writes of a transaction diverge when they run again on the corpus re-read
// under the lock (spec 40 The commit protocol, step 2): the same answers with another net post-image, and another
// answer with the same net post-image, each WRITE_CONFLICT naming the file that changed.
func TestRecheckDivergence(t *testing.T) {
	t.Run("40#the-commit-protocol: the same answer, another net post-image", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		// T3 gains a detail section, so every link to it is rewritten; a note linking T3 lands by hand.
		want(t, s, f.msg("update", "items", map[string]any{"set": sets(0, "body"), "where": corpustest.WhereID(1)}, "Now a body.", "T3"), `{"affected":1,"keys":[]}`)
		f.write("notes/N4.flowmark.md", lines("# Third root", "- item: [T3](../items.flowmark.md)", "- position: 3"))
		err := s.Commit()
		fails(t, err, "WRITE_CONFLICT", "notes/N4.flowmark.md:1", "another result")
		if strings.Contains(f.file("items.flowmark.md"), "Now a body.") {
			t.Fatal("the refused commit wrote")
		}
	})
	t.Run("40#the-commit-protocol: another answer, the same net post-image", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		want(t, s, f.msg("update", "tags", map[string]any{"set": sets(0, "kind"), "where": corpustest.WhereID(1)}, "topic", 2), `{"affected":1,"keys":[]}`)
		// Matches no note; a note that it matches lands by hand, and setting its weight to the weight it holds
		// changes no byte.
		want(t, s, f.msg("update", "notes", map[string]any{"set": sets(0, "weight"), "where": map[string]any{"items": []any{map[string]any{"pred": map[string]any{"column": "position", "op": "eq", "p": 1}}}}}, 0.25, 3), `{"affected":0,"keys":[]}`)
		f.write("notes/N4.flowmark.md", lines("# Third root", "- position: 3", "- weight: 0.25"))
		err := s.Commit()
		fails(t, err, "WRITE_CONFLICT", "notes/N4.flowmark.md:1", "another result")
		if strings.Contains(f.file("tags.flowmark.md"), "| 2 | ops | topic |") {
			t.Fatal("the refused commit wrote")
		}
	})
}

// TestCloseEndsLockWait pins a Close that arrives while a request of its session waits for the lock, as a server
// closes a session whose connection closed: the wait ends at once as CANCELED, and Close returns without waiting for
// the holder.
func TestCloseEndsLockWait(t *testing.T) {
	f := baseFixture(t)
	f.c.wait = patience
	holder := f.session("server")
	must(t, holder.Begin(context.Background(), false))
	waiter, err := f.c.Session("cli")
	if err != nil {
		t.Fatal(err)
	}
	count := f.countTags()
	done := make(chan error, 1)
	go func() {
		_, err := waiter.Exec(context.Background(), count)
		done <- err
	}()
	f.rec.waitFor(t, journal.LockWaiting)
	closed := make(chan error, 1)
	go func() { closed <- waiter.Close() }()
	// The holder keeps the lock; a Close that waited for it would not return before this guard.
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close waited for the holder of the lock")
	}
	fails(t, <-done, "CANCELED", "closed")
	must(t, holder.Rollback())
}
