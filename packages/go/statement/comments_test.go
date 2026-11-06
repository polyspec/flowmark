package statement_test

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/statement"
)

// TestCommentsBeforeAStatement pins that a comment is whitespace, as SQLite's tokenizer reads it (spec 40, SQL
// statements): a read that a comment begins reads under query_only, in a deferred transaction and outside one, a
// transaction statement and a PRAGMA after a comment control the session, a schema statement after one is a schema
// statement, and a text that holds only comments runs nothing.
func TestCommentsBeforeAStatement(t *testing.T) {
	comments := []string{"-- query\n", "/* query */ ", "/* two\nlines */ -- and one\n", "  \t-- indented\n"}
	t.Run("a read under query_only", func(t *testing.T) {
		t.Parallel()
		s, _ := open(t)
		run(t, s, "PRAGMA query_only=ON")
		for _, c := range comments {
			if got := cells(run(t, s, c+"SELECT ';'")); got != `[[";"]]` {
				t.Errorf("%q outside a transaction: read %s", c, got)
			}
			run(t, s, "BEGIN")
			if got := cells(run(t, s, c+"SELECT ';'")); got != `[[";"]]` {
				t.Errorf("%q in a deferred transaction: read %s", c, got)
			}
			if got := cells(run(t, s, c+"WITH n(v) AS (SELECT 1) SELECT v FROM n")); got != `[[1]]` {
				t.Errorf("%q WITH in a deferred transaction: read %s", c, got)
			}
			run(t, s, "ROLLBACK")
		}
	})
	t.Run("transaction statements and a PRAGMA", func(t *testing.T) {
		t.Parallel()
		s, root := open(t)
		for i, c := range comments {
			run(t, s, c+"BEGIN IMMEDIATE")
			run(t, s, "INSERT INTO notes (title) VALUES (?)", `"note`+string(rune('a'+i))+`"`)
			run(t, s, c+"COMMIT")
		}
		if got := file(t, root, "notes.flowmark.md"); !strings.Contains(got, "noted") {
			t.Fatalf("the commits after a comment wrote:\n%s", got)
		}
		run(t, s, "-- the session reads only\nPRAGMA query_only = ON")
		refused(t, s, "INSERT INTO notes (title) VALUES ('refused')", "READ_ONLY")
		run(t, s, "/* writes again */ PRAGMA query_only = OFF")
		run(t, s, "INSERT INTO notes (title) VALUES ('written')")
	})
	t.Run("a schema statement", func(t *testing.T) {
		t.Parallel()
		s, _ := open(t)
		run(t, s, "-- a table\nCREATE TABLE IF NOT EXISTS commented (id INTEGER PRIMARY KEY, v TEXT)")
		run(t, s, "/* again */ CREATE TABLE IF NOT EXISTS commented (id INTEGER PRIMARY KEY, v TEXT)")
		run(t, s, "INSERT INTO commented (v) VALUES ('x')")
		if got := cells(run(t, s, "SELECT v FROM commented")); got != `[["x"]]` {
			t.Fatalf("the table that a commented statement created holds %s", got)
		}
	})
	t.Run("a text of only comments", func(t *testing.T) {
		t.Parallel()
		s, _ := open(t)
		for _, c := range comments {
			a, err := statement.Run(s, c, nil)
			if err != nil || len(a.Rows) != 0 {
				t.Errorf("%q: answered %+v, %v, want no rows and no error", c, a, err)
			}
		}
	})
}
