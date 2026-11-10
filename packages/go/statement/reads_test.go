package statement_test

import (
	"context"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/ops"
	"github.com/polyspec/flowmark/packages/go/statement"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

// answers runs text in s and requires the answer want. A read that waited for another's cursor or lock would wait until
// that one ends, which the case never lets happen before it, so it would never answer: the deadline of the runner
// (make test-go) stops it and writes the stack of every goroutine into the test's output.
func answers(t *testing.T, s *writepath.Session, text, want string) {
	t.Helper()
	a, err := statement.Run(s, text, nil)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	if got := cells(a); got != want {
		t.Fatalf("%s answers %s, want %s", text, got, want)
	}
}

// TestConcurrentReads pins the reads of an opening (plan, Reads without the lock): a read runs on a reader connection
// of its own, so it never waits for another read's rows, while another session holds the lock or not, and it holds no
// corpus lock, so a write never waits for a read's rows.
func TestConcurrentReads(t *testing.T) {
	c, _ := openCorpus(t)
	session := func(name string) *writepath.Session {
		s, err := c.Session(name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { closeSession(t, s) })
		return s
	}
	writer, first, second := session("writer"), session("first"), session("second")
	run(t, writer, "CREATE TABLE numbers (n INTEGER PRIMARY KEY)")
	run(t, writer, "INSERT INTO numbers (n) VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(10)")
	t.Run("a read does not wait for another read's rows while a writer holds the lock", func(t *testing.T) {
		run(t, writer, "BEGIN IMMEDIATE")
		run(t, writer, "INSERT INTO numbers (n) VALUES (11)")
		if a := run(t, first, "SELECT a.n FROM numbers a, numbers b, numbers c, numbers d"); !a.More {
			t.Fatal("the first read's rows do not go on")
		}
		answers(t, second, "SELECT COUNT(*) FROM numbers", `[[10]]`)
		if err := statement.Close(first); err != nil {
			t.Fatal(err)
		}
		run(t, writer, "ROLLBACK")
	})
	t.Run("a read whose rows go on holds no lock", func(t *testing.T) {
		run(t, writer, "PRAGMA busy_timeout = 0")
		if a := run(t, first, "SELECT a.n FROM numbers a, numbers b, numbers c, numbers d"); !a.More {
			t.Fatal("the first read's rows do not go on")
		}
		answers(t, writer, "INSERT INTO numbers (n) VALUES (12) RETURNING n", `[[12]]`)
		answers(t, second, "SELECT COUNT(*) FROM numbers", `[[11]]`)
		if err := statement.Close(first); err != nil {
			t.Fatal(err)
		}
	})
}

// TestReadRecovery pins a read of a corpus another process holds (plan, Reads without the lock): while that process
// holds the lock after its commit, a read answers the commit without the lock, and a read of a corpus whose own session
// holds the lock answers what is committed.
func TestReadRecovery(t *testing.T) {
	c, root := openCorpus(t)
	s, err := c.Session("reader")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, s) })
	run(t, s, "CREATE TABLE numbers (n INTEGER PRIMARY KEY)")
	run(t, s, "INSERT INTO numbers (n) VALUES (1)")
	oc, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, oc, err)
	other, err := oc.Session("other")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, other) })
	answers(t, s, "SELECT n FROM numbers ORDER BY n", `[[1]]`)
	t.Run("a read while another process holds the lock after its commit answers that commit", func(t *testing.T) {
		run(t, other, "INSERT INTO numbers (n) VALUES (2)")
		run(t, other, "BEGIN IMMEDIATE")
		defer run(t, other, "ROLLBACK")
		run(t, other, "INSERT INTO numbers (n) VALUES (3)")
		answers(t, s, "SELECT n FROM numbers ORDER BY n", `[[1],[2]]`)
	})
	t.Run("a read while a session of its own process holds the lock answers what is committed", func(t *testing.T) {
		holder, err := c.Session("holder")
		if err != nil {
			t.Fatal(err)
		}
		defer closeSession(t, holder)
		run(t, holder, "BEGIN IMMEDIATE")
		run(t, holder, "INSERT INTO numbers (n) VALUES (4)")
		answers(t, s, "SELECT n FROM numbers ORDER BY n", `[[1],[2]]`)
		run(t, holder, "COMMIT")
		answers(t, s, "SELECT n FROM numbers ORDER BY n", `[[1],[2],[4]]`)
	})
}
