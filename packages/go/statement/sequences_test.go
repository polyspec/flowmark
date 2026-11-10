package statement_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/min-0-max/sqlite"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/ops"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

// sequenceTables are the AUTOINCREMENT tables of the cases of the sequences: one keyed alone and one with a unique
// title, for an insert that meets a row.
var sequenceTables = []string{
	"CREATE TABLE counted (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT)",
	"CREATE TABLE uq (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT UNIQUE)",
}

// sequenceRows is the statement that reads the rows of sqlite_sequence of the tables of the cases, as one text.
const sequenceRows = "SELECT group_concat(name || '=' || seq, ',') FROM (SELECT name, seq FROM sqlite_sequence WHERE name IN ('counted', 'uq') ORDER BY name, seq)"

// sqliteRun runs the statements on a database of the pinned SQLite of its own, with the tables of the cases, and returns
// it open.
func sqliteRun(t *testing.T, steps []string) *sql.DB {
	t.Helper()
	return sqliteWith(t, sequenceTables, steps)
}

// sqliteWith runs the tables and then the steps on a database of the pinned SQLite of its own, and returns it open.
func sqliteWith(t *testing.T, tables, steps []string) *sql.DB {
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
	for _, text := range append(append([]string{}, tables...), steps...) {
		if _, err := db.ExecContext(context.Background(), text); err != nil {
			t.Fatalf("SQLite: %s: %v", text, err)
		}
	}
	return db
}

// sqliteCells is the answer of the query text on db in the form of cells: one row of one text, or of NULL.
func sqliteCells(t *testing.T, db *sql.DB, text string) string {
	t.Helper()
	var v sql.NullString
	if err := db.QueryRowContext(context.Background(), text).Scan(&v); err != nil {
		t.Fatalf("SQLite: %s: %v", text, err)
	}
	if !v.Valid {
		return "[[null]]"
	}
	return fmt.Sprintf("[[%q]]", v.String)
}

// sequenceSession opens the corpus root in a session of a process of its own, which has read its rows.
func sequenceSession(t *testing.T, root, name string) *writepath.Session {
	t.Helper()
	s := openSession(t, root, name)
	run(t, s, "SELECT COUNT(*) FROM notes")
	return s
}

// openSession opens the corpus root in a session of a process of its own, closed at the end of the test.
func openSession(t *testing.T, root, name string) *writepath.Session {
	t.Helper()
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	s, err := c.Session(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, s) })
	return s
}

// TestExplicitSequences pins that the sequence of an AUTOINCREMENT table is the one SQLite keeps in sqlite_sequence
// (spec 40 SQL statements, AUTOINCREMENT counters): an explicit write of sqlite_sequence — an UPDATE, a lower value, zero
// and a negative value included, an INSERT of the row of a table and a DELETE of it — sets the sequence of its table; a
// delete of rows and a change of a key leave it as it is; an insert raises it to each key it gives or meets, and the key
// an insert gives is one more than the larger of the sequence and the largest key. Each case runs on a database of the
// pinned SQLite of its own as well, and the rows of sqlite_sequence and the keys of the inserts after the steps are
// SQLite's, in another process whose database the commits refresh, in a new opening and in the session itself.
func TestExplicitSequences(t *testing.T) {
	three := "INSERT INTO counted (title) VALUES ('a'), ('b'), ('c')"
	cases := []struct {
		name  string
		steps []string
		// table is the table the inserts after the steps insert into; want is the key of the first of them, and rows the
		// rows of sqlite_sequence after the steps.
		table string
		want  int
		rows  string
	}{
		{"a restart of the identity after a delete of the largest key", []string{
			three,
			"DELETE FROM counted WHERE id = 3",
			"UPDATE sqlite_sequence SET seq = (SELECT COALESCE(MAX(id), 0) FROM counted) WHERE name = 'counted'",
		}, "counted", 3, `[["counted=2"]]`},
		{"a sequence below the largest key", []string{
			three,
			"UPDATE sqlite_sequence SET seq = 1 WHERE name = 'counted'",
		}, "counted", 4, `[["counted=1"]]`},
		{"a delete after a lower sequence keeps the sequence", []string{
			three,
			"UPDATE sqlite_sequence SET seq = 1 WHERE name = 'counted'",
			"DELETE FROM counted WHERE id = 3",
		}, "counted", 3, `[["counted=1"]]`},
		{"a sequence past the largest key", []string{
			three,
			"UPDATE sqlite_sequence SET seq = 10 WHERE name = 'counted'",
		}, "counted", 11, `[["counted=10"]]`},
		{"a zero sequence of an empty table", []string{
			three,
			"DELETE FROM counted",
			"UPDATE sqlite_sequence SET seq = 0 WHERE name = 'counted'",
		}, "counted", 1, `[["counted=0"]]`},
		{"a negative sequence", []string{
			three,
			"UPDATE sqlite_sequence SET seq = -5 WHERE name = 'counted'",
		}, "counted", 4, `[["counted=-5"]]`},
		{"the row of the sequence deleted", []string{
			three,
			"DELETE FROM counted WHERE id = 3",
			"DELETE FROM sqlite_sequence WHERE name = 'counted'",
		}, "counted", 3, `[[null]]`},
		{"every row of sqlite_sequence deleted from an empty table", []string{
			three,
			"DELETE FROM counted",
			"DELETE FROM sqlite_sequence",
		}, "counted", 1, `[[null]]`},
		{"the row of a sequence inserted", []string{
			"INSERT INTO sqlite_sequence (name, seq) VALUES ('counted', 5)",
		}, "counted", 6, `[["counted=5"]]`},
		{"the row of a sequence inserted again after its delete", []string{
			three,
			"DELETE FROM sqlite_sequence WHERE name = 'counted'",
			"INSERT INTO sqlite_sequence (name, seq) VALUES ('counted', 1)",
		}, "counted", 4, `[["counted=1"]]`},
		{"a change of a key leaves the sequence", []string{
			"INSERT INTO counted (title) VALUES ('a'), ('b')",
			"UPDATE counted SET id = 10 WHERE id = 2",
			"UPDATE counted SET id = 3 WHERE id = 10",
		}, "counted", 4, `[["counted=2"]]`},
		{"a key given below the sequence", []string{
			three,
			"DELETE FROM counted WHERE id = 2",
			"INSERT INTO counted (id, title) VALUES (2, 'again')",
		}, "counted", 4, `[["counted=3"]]`},
		{"a key given after the row of the sequence is deleted", []string{
			three,
			"DELETE FROM sqlite_sequence",
			"DELETE FROM counted WHERE id = 2",
			"INSERT INTO counted (id, title) VALUES (2, 'again')",
		}, "counted", 4, `[["counted=2"]]`},
		{"an insert that meets a row on its unique key takes the key it gave", []string{
			"INSERT INTO uq (title) VALUES ('a'), ('b')",
			"INSERT INTO uq (title) VALUES ('a') ON CONFLICT (title) DO UPDATE SET title = 'a'",
		}, "uq", 4, `[["uq=3"]]`},
		{"an insert that a conflict ignores takes the key it gave", []string{
			"INSERT INTO uq (title) VALUES ('a'), ('b')",
			"INSERT OR IGNORE INTO uq (title) VALUES ('b')",
		}, "uq", 4, `[["uq=3"]]`},
		{"an insert that meets a row by a key past the sequence", []string{
			"INSERT INTO uq (title) VALUES ('a')",
			"UPDATE sqlite_sequence SET seq = 0",
			"INSERT INTO uq (id, title) VALUES (1, 'b') ON CONFLICT (id) DO UPDATE SET title = 'b'",
		}, "uq", 2, `[["uq=1"]]`},
		{"an explicit write inside a transaction", []string{
			three,
			"BEGIN IMMEDIATE",
			"DELETE FROM counted WHERE id = 3",
			"UPDATE sqlite_sequence SET seq = 2 WHERE name = 'counted'",
			"COMMIT",
		}, "counted", 3, `[["counted=2"]]`},
		{"an explicit write rolled back", []string{
			three,
			"BEGIN IMMEDIATE",
			"UPDATE sqlite_sequence SET seq = 1 WHERE name = 'counted'",
			"ROLLBACK",
		}, "counted", 4, `[["counted=3"]]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := sqliteRun(t, tc.steps)
			if got := sqliteCells(t, db, sequenceRows); got != tc.rows {
				t.Fatalf("SQLite holds the sequences %s, the case says %s", got, tc.rows)
			}
			s, root := open(t)
			for _, text := range sequenceTables {
				run(t, s, text)
			}
			other := sequenceSession(t, root, "other")
			for _, text := range tc.steps {
				run(t, s, text)
			}
			fresh := sequenceSession(t, root, "new")
			for _, sess := range []struct {
				name string
				s    *writepath.Session
			}{{"the other process", other}, {"a new opening", fresh}, {"the first session", s}} {
				if got := cells(run(t, sess.s, sequenceRows)); got != tc.rows {
					t.Fatalf("%s holds the sequences %s, SQLite %s", sess.name, got, tc.rows)
				}
			}
			for i, sess := range []struct {
				name string
				s    *writepath.Session
			}{{"the other process", other}, {"a new opening", fresh}, {"the first session", s}} {
				insert := fmt.Sprintf("INSERT INTO %s (title) VALUES ('next %d') RETURNING id", tc.table, i)
				var key int64
				if err := db.QueryRowContext(context.Background(), insert).Scan(&key); err != nil {
					t.Fatalf("SQLite: %s: %v", insert, err)
				}
				if key != int64(tc.want+i) {
					t.Fatalf("SQLite gives the key %d to insert %d, the case says %d", key, i+1, tc.want+i)
				}
				if got, want := cells(run(t, sess.s, insert)), fmt.Sprintf("[[%d]]", key); got != want {
					t.Fatalf("insert %d, of %s, gives %s, SQLite %s", i+1, sess.name, got, want)
				}
			}
			if got, want := cells(run(t, s, sequenceRows)), sqliteCells(t, db, sequenceRows); got != want {
				t.Fatalf("after the inserts the corpus holds the sequences %s, SQLite %s", got, want)
			}
		})
	}
}

// TestExplicitSequenceRefused pins the explicit writes of sqlite_sequence that leave a state the corpus does not hold
// (spec 40 SQL statements, AUTOINCREMENT counters): the corpus holds one integer sequence of each AUTOINCREMENT table or
// none, so a sequence that is no integer, a second row of a table and a row of a name that is no AUTOINCREMENT table
// are CONSTRAINT, and the statement leaves the corpus, its sequences and its next key as they were, in a transaction
// as well as alone.
func TestExplicitSequenceRefused(t *testing.T) {
	cases := []struct{ name, text, part string }{
		{"a text", "UPDATE sqlite_sequence SET seq = 'abc' WHERE name = 'counted'", "is no integer"},
		{"the text of an integer", "UPDATE sqlite_sequence SET seq = '7' WHERE name = 'counted'", "is no integer"},
		{"a real", "UPDATE sqlite_sequence SET seq = 2.5 WHERE name = 'counted'", "is no integer"},
		{"NULL", "UPDATE sqlite_sequence SET seq = NULL WHERE name = 'counted'", "is no integer"},
		{"a second row of a table", "INSERT INTO sqlite_sequence (name, seq) VALUES ('counted', 9)", "two rows"},
		{"a row of no table", "INSERT INTO sqlite_sequence (name, seq) VALUES ('nosuch', 5)", "no AUTOINCREMENT table"},
		{"a row of a table without AUTOINCREMENT", "INSERT INTO sqlite_sequence (name, seq) VALUES ('notes_plain', 5)", "no AUTOINCREMENT table"},
		{"a row renamed to no table", "UPDATE sqlite_sequence SET name = 'renamed' WHERE name = 'counted'", "no AUTOINCREMENT table"},
		{"a row of a name in another case", "UPDATE sqlite_sequence SET name = 'COUNTED' WHERE name = 'counted'", "no AUTOINCREMENT table"},
	}
	for _, tc := range cases {
		for _, inTx := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, in a transaction %v", tc.name, inTx), func(t *testing.T) {
				s, root := open(t)
				run(t, s, sequenceTables[0])
				run(t, s, "CREATE TABLE notes_plain (id INTEGER PRIMARY KEY, title TEXT)")
				run(t, s, "INSERT INTO counted (title) VALUES ('a'), ('b'), ('c')")
				run(t, s, "DELETE FROM counted WHERE id = 3")
				if inTx {
					run(t, s, "BEGIN IMMEDIATE")
				}
				fails(t, s, tc.text, "CONSTRAINT", tc.part)
				if inTx {
					if got := cells(run(t, s, sequenceRows)); got != `[["counted=3"]]` {
						t.Fatalf("the transaction holds the sequences %s after the refusal", got)
					}
					run(t, s, "COMMIT")
				}
				fresh := sequenceSession(t, root, "new")
				for _, sess := range []*writepath.Session{s, fresh} {
					if got := cells(run(t, sess, sequenceRows)); got != `[["counted=3"]]` {
						t.Fatalf("the corpus holds the sequences %s after the refusal", got)
					}
				}
				if got := cells(run(t, fresh, "INSERT INTO counted (title) VALUES ('next') RETURNING id")); got != "[[4]]" {
					t.Fatalf("the insert after the refusal gives %s", got)
				}
			})
		}
	}
}
