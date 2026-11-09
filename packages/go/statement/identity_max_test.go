package statement_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/min-0-max/sqlite"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

// plainTables are the tables of the cases of the largest key: a plain INTEGER PRIMARY KEY, one with a unique title for
// an insert that meets a row, and a table keyed by its rowid alone.
var plainTables = []string{
	"CREATE TABLE plain (id INTEGER PRIMARY KEY, title TEXT)",
	"CREATE TABLE uplain (id INTEGER PRIMARY KEY, title TEXT UNIQUE)",
	"CREATE TABLE bare (title TEXT)",
}

// plainRows is the statement that reads the keys of the tables of the cases, as one text.
const plainRows = "SELECT COALESCE((SELECT group_concat(id, ',') FROM (SELECT id FROM plain ORDER BY id)), '')" +
	" || '|' || COALESCE((SELECT group_concat(id, ',') FROM (SELECT id FROM uplain ORDER BY id)), '')" +
	" || '|' || COALESCE((SELECT group_concat(rowid, ',') FROM (SELECT rowid FROM bare ORDER BY rowid)), '')"

// plainSession opens a corpus with the tables of the cases and returns its session and root.
func plainSession(t *testing.T) (*writepath.Session, string) {
	t.Helper()
	s, root := open(t)
	for _, text := range plainTables {
		run(t, s, text)
	}
	return s, root
}

// TestPlainIdentityLargest pins the key past the largest int64 of a table whose key is its rowid and that keeps no
// AUTOINCREMENT sequence (spec 40 SQL statements, The largest key): an insert gives one more than the largest key, the
// largest int64 included, as SQLite does; where SQLite would pick a random unused rowid, since the largest key is the
// largest int64, the insert fails as DRIVER (SQLITE_FULL), as an AUTOINCREMENT table fails and as MySQL and PostgreSQL
// refuse an identity past its maximum, and the statement leaves the corpus and its transaction as they were. A key the
// insert gives, and an insert that gives no row, are taken while the table holds the largest int64.
func TestPlainIdentityLargest(t *testing.T) {
	const most = "9223372036854775807"
	accepted := []struct {
		name  string
		steps []string
		rows  string
	}{
		{"the key after the largest but one", []string{
			"INSERT INTO plain (id, title) VALUES (9223372036854775806, 'a')",
			"INSERT INTO plain (title) VALUES ('b')",
		}, "9223372036854775806," + most + "||"},
		{"a key given beside the largest", []string{
			"INSERT INTO plain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO plain (id, title) VALUES (5, 'b')",
		}, "5," + most + "||"},
		{"the key -1 given beside the largest", []string{
			"INSERT INTO plain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO plain (id, title) VALUES (-1, 'b')",
		}, "-1," + most + "||"},
		{"an insert a conflict ignores, then a key given", []string{
			"INSERT INTO uplain (id, title) VALUES (" + most + ", 'a')",
			"INSERT OR IGNORE INTO uplain (title) VALUES ('a')",
			"INSERT INTO uplain (id, title) VALUES (7, 'b')",
		}, "|7," + most + "|"},
		{"an ignored row and a given key in one statement", []string{
			"INSERT INTO uplain (id, title) VALUES (" + most + ", 'a')",
			"INSERT OR IGNORE INTO uplain (id, title) VALUES (NULL, 'a'), (8, 'c')",
		}, "|8," + most + "|"},
		{"an insert that meets a row and updates it", []string{
			"INSERT INTO uplain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO uplain (title) VALUES ('a') ON CONFLICT (title) DO UPDATE SET title = 'a'",
		}, "|" + most + "|"},
		{"the rowid after the largest but one", []string{
			"INSERT INTO bare (rowid, title) VALUES (9223372036854775806, 'a')",
			"INSERT INTO bare (title) VALUES ('b')",
		}, "||9223372036854775806," + most},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			db := plainSQLite(t, tc.steps)
			if got := plainCells(t, db); got != fmt.Sprintf("[[%q]]", tc.rows) {
				t.Fatalf("SQLite holds %s, the case says %q", got, tc.rows)
			}
			s, root := plainSession(t)
			for _, text := range tc.steps {
				run(t, s, text)
			}
			for _, sess := range []*writepath.Session{s, sequenceSession(t, root, "new")} {
				if got := cells(run(t, sess, plainRows)); got != fmt.Sprintf("[[%q]]", tc.rows) {
					t.Fatalf("the corpus holds %s, SQLite %q", got, tc.rows)
				}
			}
		})
	}
	refused := []struct {
		name, setup, text string
	}{
		{"an insert past the largest", "INSERT INTO plain (id, title) VALUES (" + most + ", 'a')", "INSERT INTO plain (title) VALUES ('b')"},
		{"a key NULL past the largest", "INSERT INTO plain (id, title) VALUES (" + most + ", 'a')", "INSERT INTO plain (id, title) VALUES (NULL, 'b')"},
		{"the largest and the key after it in one statement", "INSERT INTO plain (id, title) VALUES (1, 'a')",
			"INSERT INTO plain (id, title) VALUES (" + most + ", 'b'), (NULL, 'c')"},
		{"an insert of a select past the largest", "INSERT INTO plain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO plain (title) SELECT title FROM plain"},
		{"a given key, then one past the largest", "INSERT INTO plain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO plain (id, title) VALUES (4, 'b'), (NULL, 'c')"},
		{"a rowid past the largest", "INSERT INTO bare (rowid, title) VALUES (" + most + ", 'a')", "INSERT INTO bare (title) VALUES ('b')"},
	}
	for _, tc := range refused {
		for _, inTx := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, in a transaction %v", tc.name, inTx), func(t *testing.T) {
				s, root := plainSession(t)
				run(t, s, tc.setup)
				want := cells(run(t, s, plainRows))
				if inTx {
					run(t, s, "BEGIN IMMEDIATE")
				}
				fails(t, s, tc.text, "DRIVER", "(SQLITE_FULL)")
				if got := cells(run(t, s, plainRows)); got != want {
					t.Fatalf("after the refusal the session holds %s, want %s", got, want)
				}
				if inTx {
					run(t, s, "INSERT INTO plain (id, title) VALUES (2, 'kept')")
					run(t, s, "DELETE FROM plain WHERE id = 2")
					run(t, s, "COMMIT")
				}
				if got := cells(run(t, sequenceSession(t, root, "new"), plainRows)); got != want {
					t.Fatalf("after the refusal the corpus holds %s, want %s", got, want)
				}
			})
		}
	}
}

// TestPlainIdentityLargestFace pins the same largest key on the face: a request that inserts into the plain table
// without its key takes one more than the largest key, the largest int64 included, and past it fails as DRIVER
// (SQLITE_FULL) and changes nothing.
func TestPlainIdentityLargestFace(t *testing.T) {
	s, _ := plainSession(t)
	run(t, s, "INSERT INTO plain (id, title) VALUES (9223372036854775806, 'a')")
	insert := func() (string, error) {
		var hash string
		if err := s.View(context.Background(), func(c *irexec.Corpus) error {
			hash = c.Catalog().ManifestHash
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		msg := corpustest.Message(t, hash, "2026-01-01 00:00:00.000000", "insert", "plain",
			map[string]any{"set": []map[string]any{{"column": "title", "p": 0}}}, "next")
		a, err := s.ExecFace(context.Background(), msg)
		if err != nil {
			return "", err
		}
		return fmt.Sprint(a.Keys[0]), nil
	}
	key, err := insert()
	if err != nil || key != "9223372036854775807" {
		t.Fatalf("the insert after the largest but one gives %s, %v", key, err)
	}
	if _, err := insert(); err == nil {
		t.Fatal("the insert past the largest is not refused")
	} else if e, ok := err.(interface{ Failure() (string, string) }); !ok {
		t.Fatalf("the insert past the largest fails with %v", err)
	} else if code, msg := e.Failure(); code != "DRIVER" {
		t.Fatalf("the insert past the largest fails as %s %s", code, msg)
	}
	if got := cells(run(t, s, "SELECT COUNT(*) FROM plain")); got != "[[2]]" {
		t.Fatalf("after the refusal the table holds %s rows", got)
	}
}

// plainSQLite runs the steps on a database of the pinned SQLite of its own with the tables of the cases.
func plainSQLite(t *testing.T, steps []string) *sql.DB {
	t.Helper()
	db, err := sql.Open(sqlite.DriverName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, text := range append(append([]string{}, plainTables...), steps...) {
		if _, err := db.ExecContext(context.Background(), text); err != nil {
			t.Fatalf("SQLite: %s: %v", text, err)
		}
	}
	return db
}

// plainCells is the answer of plainRows on db in the form of cells.
func plainCells(t *testing.T, db *sql.DB) string {
	t.Helper()
	return sqliteCells(t, db, plainRows)
}
