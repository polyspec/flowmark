package statement_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/min-0-max/sqlite"

	"github.com/polyspec/flowmark/packages/go/statement"
)

// TestWriterFollowsTransaction pins the writer connection's transaction, which follows the session's (spec 40 SQL
// statements): a deferred foreign key SQLite has not resolved refuses the COMMIT as SQLite does and leaves the
// transaction open; a write of the session's transaction outside its statements, an IR request, is in what the next
// statement reads; a savepoint the session set before its first statement is one the statements roll back to; and no
// reader connection reads what the transaction has not committed.
func TestWriterFollowsTransaction(t *testing.T) {
	t.Run("a deferred foreign key refuses the commit and leaves the transaction open", func(t *testing.T) {
		s, _ := open(t)
		schema := []string{
			"CREATE TABLE parents (id INTEGER PRIMARY KEY)",
			"CREATE TABLE children (id INTEGER PRIMARY KEY, parent INTEGER REFERENCES parents (id) DEFERRABLE INITIALLY DEFERRED)",
		}
		for _, text := range schema {
			run(t, s, text)
		}
		run(t, s, "BEGIN IMMEDIATE")
		run(t, s, "INSERT INTO children (id, parent) VALUES (1, 7)")
		_, err := statement.Run(s, "COMMIT", nil)
		want := plainCommitFailure(t, schema, "INSERT INTO children (id, parent) VALUES (1, 7)")
		var f interface{ Failure() (string, string) }
		if !errors.As(err, &f) {
			t.Fatalf("COMMIT = %v, want the refusal of a deferred foreign key", err)
		}
		if code, msg := f.Failure(); code != "FOREIGN_KEY" || !strings.HasPrefix(msg, want+" (") {
			t.Fatalf("COMMIT = %s %s, want FOREIGN_KEY %s, as SQLite refuses it", code, msg, want)
		}
		if !s.InTransaction() {
			t.Fatal("the refused COMMIT ended the transaction")
		}
		run(t, s, "INSERT INTO parents (id) VALUES (7)")
		run(t, s, "COMMIT")
		if got := cells(run(t, s, "SELECT c.id, p.id FROM children c JOIN parents p ON p.id = c.parent")); got != `[[1,7]]` {
			t.Fatalf("after the commit: %s", got)
		}
	})
	t.Run("an IR request of the transaction is in what the next statement reads", func(t *testing.T) {
		c, _ := openCorpus(t)
		s, err := c.Session("writer")
		if err != nil {
			t.Fatal(err)
		}
		defer closeSession(t, s)
		other, err := c.Session("other")
		if err != nil {
			t.Fatal(err)
		}
		defer closeSession(t, other)
		run(t, s, "BEGIN")
		run(t, s, "INSERT INTO notes (id, title) VALUES (1, 'by a statement')")
		cat := c.Catalog()
		tb, _ := cat.Table("notes")
		msg, err := statement.InsertMessage(cat, tb, [][]any{{int64(2), "by a request", nil, int64(0), nil, nil}}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ExecFace(context.Background(), msg); err != nil {
			t.Fatal(err)
		}
		if got := cells(run(t, s, "SELECT id, title FROM notes ORDER BY id")); got != `[[1,"by a statement"],[2,"by a request"]]` {
			t.Fatalf("the transaction reads %s", got)
		}
		run(t, s, "UPDATE notes SET title = 'both' WHERE id IN (1, 2)")
		if got := cells(run(t, other, "SELECT COUNT(*) FROM notes")); got != `[[0]]` {
			t.Fatalf("another session reads %s rows of an open transaction", got)
		}
		run(t, s, "COMMIT")
		if got := cells(run(t, other, "SELECT id, title FROM notes ORDER BY id")); got != `[[1,"both"],[2,"both"]]` {
			t.Fatalf("after the commit another session reads %s", got)
		}
	})
	t.Run("a savepoint set before the first statement is one the statements roll back to", func(t *testing.T) {
		s, _ := open(t)
		if err := s.Begin(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		if err := s.Savepoint("before"); err != nil {
			t.Fatal(err)
		}
		run(t, s, "INSERT INTO notes (id, title) VALUES (1, 'rolled back')")
		run(t, s, "ROLLBACK TO before")
		run(t, s, "INSERT INTO notes (id, title) VALUES (2, 'kept')")
		run(t, s, "COMMIT")
		if got := cells(run(t, s, "SELECT id FROM notes")); got != `[[2]]` {
			t.Fatalf("after the rollback to the savepoint: %s", got)
		}
	})
}

// plainCommitFailure runs schema and then text in one transaction of a plain SQLite database and returns the message of
// the failure of its COMMIT, without its result code.
func plainCommitFailure(t *testing.T, schema []string, text string) string {
	t.Helper()
	db, err := sql.Open(sqlite.DriverName, ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	db.SetMaxOpenConns(1)
	for _, q := range append(append(schema, "BEGIN"), text) {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec("COMMIT")
	if err == nil {
		t.Fatal("SQLite commits the transaction")
	}
	if _, rerr := db.Exec("ROLLBACK"); rerr != nil {
		t.Fatal(rerr)
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, " ("); i >= 0 {
		msg = msg[:i]
	}
	return msg
}
