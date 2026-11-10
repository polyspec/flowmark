package statement_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/min-0-max/sqlite"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

// plainTables are the tables of the cases of the largest key: a plain INTEGER PRIMARY KEY, one with a unique title for
// an insert that meets a row, a table keyed by its rowid alone, and one keyed by its rowid since its primary key holds
// NULL.
var plainTables = []string{
	"CREATE TABLE plain (id INTEGER PRIMARY KEY, title TEXT)",
	"CREATE TABLE uplain (id INTEGER PRIMARY KEY, title TEXT UNIQUE)",
	"CREATE TABLE bare (title TEXT)",
	"CREATE TABLE npk (k TEXT PRIMARY KEY, title TEXT)",
}

// plainRows is the statement that reads the keys of the tables of the cases, as one text.
const plainRows = "SELECT COALESCE((SELECT group_concat(id, ',') FROM (SELECT id FROM plain ORDER BY id)), '')" +
	" || '|' || COALESCE((SELECT group_concat(id, ',') FROM (SELECT id FROM uplain ORDER BY id)), '')" +
	" || '|' || COALESCE((SELECT group_concat(rowid, ',') FROM (SELECT rowid FROM bare ORDER BY rowid)), '')" +
	" || '|' || COALESCE((SELECT group_concat(rowid, ',') FROM (SELECT rowid FROM npk ORDER BY rowid)), '')"

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
		}, "9223372036854775806," + most + "|||"},
		{"a key given beside the largest", []string{
			"INSERT INTO plain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO plain (id, title) VALUES (5, 'b')",
		}, "5," + most + "|||"},
		{"the key -1 given beside the largest", []string{
			"INSERT INTO plain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO plain (id, title) VALUES (-1, 'b')",
		}, "-1," + most + "|||"},
		{"an insert a conflict ignores, then a key given", []string{
			"INSERT INTO uplain (id, title) VALUES (" + most + ", 'a')",
			"INSERT OR IGNORE INTO uplain (title) VALUES ('a')",
			"INSERT INTO uplain (id, title) VALUES (7, 'b')",
		}, "|7," + most + "||"},
		{"an ignored row and a given key in one statement", []string{
			"INSERT INTO uplain (id, title) VALUES (" + most + ", 'a')",
			"INSERT OR IGNORE INTO uplain (id, title) VALUES (NULL, 'a'), (8, 'c')",
		}, "|8," + most + "||"},
		{"an insert that meets a row and updates it", []string{
			"INSERT INTO uplain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO uplain (title) VALUES ('a') ON CONFLICT (title) DO UPDATE SET title = 'a'",
		}, "|" + most + "||"},
		{"the rowid after the largest but one", []string{
			"INSERT INTO bare (rowid, title) VALUES (9223372036854775806, 'a')",
			"INSERT INTO bare (title) VALUES ('b')",
		}, "||9223372036854775806," + most + "|"},
		{"the rowid after the largest but one of a key that holds NULL", []string{
			"INSERT INTO npk (rowid, k, title) VALUES (9223372036854775806, 'a', 'a')",
			"INSERT INTO npk (k, title) VALUES ('b', 'b')",
		}, "|||9223372036854775806," + most},
		{"the largest and a given key after it in one statement", []string{
			"INSERT INTO plain (id, title) VALUES (" + most + ", 'a'), (5, 'b')",
		}, "5," + most + "|||"},
		{"an insert of a trigger with a key given under an insert with a key given", []string{
			"CREATE TRIGGER inner BEFORE INSERT ON plain WHEN NEW.title = 'outer' BEGIN INSERT INTO plain (id, title) VALUES (5, 'inner'); END",
			"INSERT INTO plain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO plain (id, title) VALUES (6, 'outer')",
		}, "5,6," + most + "|||"},
		{"an insert of a trigger that a conflict ignores under an insert with a key given", []string{
			"CREATE TRIGGER inner BEFORE INSERT ON plain BEGIN INSERT OR IGNORE INTO uplain (title) VALUES ('a'); END",
			"INSERT INTO uplain (id, title) VALUES (" + most + ", 'a')",
			"INSERT INTO plain (id, title) VALUES (6, 'outer')",
		}, "6|" + most + "||"},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			// Each case has its own corpus and database.
			t.Parallel()
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
	holds := "INSERT INTO plain (id, title) VALUES (" + most + ", 'a')"
	refused := []struct {
		name  string
		setup []string
		text  string
	}{
		{"an insert past the largest", []string{holds}, "INSERT INTO plain (title) VALUES ('b')"},
		{"a key NULL past the largest", []string{holds}, "INSERT INTO plain (id, title) VALUES (NULL, 'b')"},
		{"the largest and the key after it in one statement", []string{"INSERT INTO plain (id, title) VALUES (1, 'a')"},
			"INSERT INTO plain (id, title) VALUES (" + most + ", 'b'), (NULL, 'c')"},
		{"an insert of a select past the largest", []string{holds}, "INSERT INTO plain (title) SELECT title FROM plain"},
		{"a given key, then one past the largest", []string{holds}, "INSERT INTO plain (id, title) VALUES (4, 'b'), (NULL, 'c')"},
		{"a rowid past the largest", []string{"INSERT INTO bare (rowid, title) VALUES (" + most + ", 'a')"},
			"INSERT INTO bare (title) VALUES ('b')"},
		{"a rowid past the largest of a key that holds NULL", []string{"INSERT INTO npk (rowid, k, title) VALUES (" + most + ", 'a', 'a')"},
			"INSERT INTO npk (k, title) VALUES ('b', 'b')"},
		// The insert of the trigger, a level deeper, takes its own key; the note of the outer insert stays its own.
		{"an insert past the largest over the insert of its trigger with a key given", []string{
			"CREATE TRIGGER inner BEFORE INSERT ON plain WHEN NEW.title = 'outer' BEGIN INSERT INTO plain (id, title) VALUES (5, 'inner'); END",
			holds,
		}, "INSERT INTO plain (title) VALUES ('outer')"},
		{"an insert of a trigger past the largest under an insert with a key given", []string{
			"CREATE TRIGGER inner BEFORE INSERT ON plain WHEN NEW.title = 'outer' BEGIN INSERT INTO plain (title) VALUES ('inner'); END",
			holds,
		}, "INSERT INTO plain (id, title) VALUES (6, 'outer')"},
		{"an insert of a trigger past the largest of another table", []string{
			"CREATE TRIGGER inner AFTER INSERT ON plain WHEN NEW.title = 'outer' BEGIN INSERT INTO bare (title) VALUES (NEW.title); END",
			"INSERT INTO bare (rowid, title) VALUES (" + most + ", 'a')",
		}, "INSERT INTO plain (id, title) VALUES (6, 'outer')"},
	}
	for _, tc := range refused {
		for _, inTx := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, in a transaction %v", tc.name, inTx), func(t *testing.T) {
				// Each case has its own corpus and database.
				t.Parallel()
				s, root := plainSession(t)
				for _, text := range tc.setup {
					run(t, s, text)
				}
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

// TestLargestGuardsLeaveNoSchema pins that the guard of the largest key stands only while its statement runs: after an
// insert under the guard, after one it refuses and after an insert into a table without a rowid, alone and in a
// transaction, the temporary schema reads as that of SQLite after the same statements.
func TestLargestGuardsLeaveNoSchema(t *testing.T) {
	const most = "9223372036854775807"
	const temp = "SELECT COALESCE((SELECT group_concat(type || ' ' || name, ',') FROM (SELECT type, name FROM sqlite_temp_master ORDER BY name)), '')" +
		" || '|' || (SELECT group_concat(name, ',') FROM (SELECT name FROM pragma_table_list WHERE schema = 'temp' ORDER BY name))"
	steps := []struct {
		text string
		ok   bool
	}{
		{"INSERT INTO plain (id, title) VALUES (5, 'b')", true},
		{"INSERT INTO plain (title) VALUES ('c')", false},
		{"INSERT INTO wr (k, title) VALUES ('a', 'a')", true},
		{"UPDATE wr SET title = 'b'", true},
	}
	db := plainSQLite(t, []string{"CREATE TABLE wr (k TEXT PRIMARY KEY, title TEXT) WITHOUT ROWID", "INSERT INTO plain (id, title) VALUES (" + most + ", 'a')"})
	want := sqliteCells(t, db, temp)
	for _, inTx := range []bool{false, true} {
		t.Run(fmt.Sprintf("in a transaction %v", inTx), func(t *testing.T) {
			s, _ := plainSession(t)
			run(t, s, "CREATE TABLE wr (k TEXT PRIMARY KEY, title TEXT) WITHOUT ROWID")
			run(t, s, "INSERT INTO plain (id, title) VALUES ("+most+", 'a')")
			if inTx {
				run(t, s, "BEGIN IMMEDIATE")
			}
			for _, step := range steps {
				if step.ok {
					run(t, s, step.text)
				} else {
					fails(t, s, step.text, "DRIVER", "(SQLITE_FULL)")
				}
				if got := cells(run(t, s, temp)); got != want {
					t.Fatalf("after %s the temporary schema reads %s, SQLite %s", step.text, got, want)
				}
			}
			if inTx {
				run(t, s, "COMMIT")
				if got := cells(run(t, s, temp)); got != want {
					t.Fatalf("after the commit the temporary schema reads %s, SQLite %s", got, want)
				}
			}
		})
	}
}

// TestRowidColumnName pins the name of the rowid of a table keyed by it (spec 40 SQL statements, Tables): the corpus
// holds that rowid as the column rowid, so a schema statement that leaves such a table, one without a primary key or
// with one that holds NULL, a column of that name in any case is CONSTRAINT and changes nothing, and a table keyed
// otherwise names a column rowid as SQLite lets it.
func TestRowidColumnName(t *testing.T) {
	accepted := []struct {
		name  string
		steps []string
		read  string
		rows  string
	}{
		{"the INTEGER PRIMARY KEY rowid", []string{"CREATE TABLE r (rowid INTEGER PRIMARY KEY, b TEXT)",
			"INSERT INTO r (b) VALUES ('x'), ('y')", "UPDATE r SET b = 'z' WHERE b = 'x'"},
			"SELECT group_concat(rowid || ' ' || b, ',') FROM r", `[["1 z,2 y"]]`},
		{"a key that holds no NULL", []string{"CREATE TABLE r (rowid TEXT PRIMARY KEY NOT NULL, b TEXT)",
			"INSERT INTO r VALUES ('k', 'x'), ('j', 'y')", "UPDATE r SET b = 'z' WHERE rowid = 'k'", "DELETE FROM r WHERE rowid = 'j'"},
			"SELECT group_concat(rowid || ' ' || b, ',') FROM r", `[["k z"]]`},
		{"a key that holds no NULL and a column in another case", []string{"CREATE TABLE r (k TEXT PRIMARY KEY NOT NULL, RowId TEXT)",
			"INSERT INTO r VALUES ('k', 'x'), ('j', 'y')", "UPDATE r SET RowId = 'z' WHERE k = 'k'"},
			"SELECT group_concat(k || ' ' || rowid, ',') FROM (SELECT * FROM r ORDER BY k)", `[["j y,k z"]]`},
		{"every name of the rowid beside an INTEGER PRIMARY KEY", []string{"CREATE TABLE r (id INTEGER PRIMARY KEY, rowid TEXT, _rowid_ TEXT, oid TEXT)",
			"INSERT INTO r (rowid, _rowid_, oid) VALUES ('a', 'b', 'c')", "UPDATE r SET oid = 'd'"},
			"SELECT group_concat(id || ' ' || rowid || ' ' || _rowid_ || ' ' || oid, ',') FROM r", `[["1 a b d"]]`},
		{"a table without a rowid", []string{"CREATE TABLE r (rowid TEXT PRIMARY KEY, b TEXT) WITHOUT ROWID",
			"INSERT INTO r VALUES ('k', 'x')"}, "SELECT group_concat(rowid || ' ' || b, ',') FROM r", `[["k x"]]`},
		{"the other names of the rowid", []string{"CREATE TABLE r (_rowid_ TEXT, oid TEXT)",
			"INSERT INTO r VALUES ('a', 'b')", "UPDATE r SET oid = 'c'"},
			"SELECT group_concat(rowid || ' ' || _rowid_ || ' ' || oid, ',') FROM r", `[["1 a c"]]`},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			db := plainSQLite(t, tc.steps)
			if got := sqliteCells(t, db, tc.read); got != tc.rows {
				t.Fatalf("SQLite holds %s, the case says %s", got, tc.rows)
			}
			s, root := open(t)
			for _, text := range tc.steps {
				run(t, s, text)
			}
			for _, sess := range []*writepath.Session{s, sequenceSession(t, root, "new")} {
				if got := cells(run(t, sess, tc.read)); got != tc.rows {
					t.Fatalf("the corpus holds %s, SQLite %s", got, tc.rows)
				}
			}
		})
	}
	const keyed, unnamed = "is keyed by its rowid", "has a rowid that no statement names"
	// SQLite itself refuses a column that an ALTER TABLE would name after the rowid.
	refused := []struct {
		name       string
		setup      []string
		text       string
		code, part string
	}{
		{"a table without a primary key", nil, "CREATE TABLE r (rowid TEXT, b TEXT)", "CONSTRAINT", keyed},
		{"the name in another case", nil, "CREATE TABLE r (b TEXT, RowId TEXT)", "CONSTRAINT", keyed},
		{"every name of the rowid", nil, "CREATE TABLE r (rowid TEXT, _rowid_ TEXT, oid TEXT)", "CONSTRAINT", unnamed},
		{"every name of the rowid beside a key that holds no NULL", nil,
			"CREATE TABLE r (k TEXT PRIMARY KEY NOT NULL, rowid TEXT, _rowid_ TEXT, OID TEXT)", "CONSTRAINT", unnamed},
		{"every name of the rowid beside a key of an INT", nil,
			"CREATE TABLE r (k INT PRIMARY KEY, rowid TEXT, _rowid_ TEXT, oid TEXT)", "CONSTRAINT", unnamed},
		{"a key that holds NULL", nil, "CREATE TABLE r (k TEXT PRIMARY KEY, rowid TEXT)", "CONSTRAINT", keyed},
		{"an added column", []string{"CREATE TABLE r (b TEXT)", "INSERT INTO r VALUES ('x')"}, "ALTER TABLE r ADD COLUMN rowid TEXT",
			"DRIVER", "cannot add rowid alias"},
		{"a renamed column", []string{"CREATE TABLE r (b TEXT)", "INSERT INTO r VALUES ('x')"}, "ALTER TABLE r RENAME COLUMN b TO rowid",
			"DRIVER", "cannot rename to rowid alias"},
	}
	const schema = "SELECT COALESCE((SELECT group_concat(sql, ';') FROM (SELECT sql FROM sqlite_schema ORDER BY name)), '')"
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			s, root := open(t)
			for _, text := range tc.setup {
				run(t, s, text)
			}
			want := cells(run(t, s, schema))
			if tc.code == "DRIVER" {
				db := plainSQLite(t, tc.setup)
				if _, err := db.ExecContext(context.Background(), tc.text); err == nil || !strings.Contains(err.Error(), tc.part) {
					t.Fatalf("SQLite answers %s with %v, the case says %s", tc.text, err, tc.part)
				}
			}
			fails(t, s, tc.text, tc.code, tc.part)
			for _, sess := range []*writepath.Session{s, sequenceSession(t, root, "new")} {
				if got := cells(run(t, sess, schema)); got != want {
					t.Fatalf("after the refusal the schema reads %s, want %s", got, want)
				}
			}
			if len(tc.setup) > 0 {
				if got := cells(run(t, s, "SELECT rowid || ' ' || b FROM r")); got != `[["1 x"]]` {
					t.Fatalf("after the refusal the table holds %s", got)
				}
			}
		})
	}
}
