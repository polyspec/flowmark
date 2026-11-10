package statement_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/ops"
	"github.com/polyspec/flowmark/packages/go/statement"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

const set = `dbspec 1 board

table notes {
  id i64 identity
  title varchar(32)
  price decimal(13,3) null
  done bool default false
  at datetime(6) null
  body bytes null
  primary key (id)
}

table tags {
  id i64 identity
  note i64
  primary key (id)
  index ix_tags_note (note)
  foreign key fk_tags_note (note) references notes (id) on delete cascade on update restrict
}
`

// openCorpus opens a new corpus of the set of this file.
func openCorpus(t testing.TB) (*ops.Corpus, string) {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["board.dbs"]}`,
		"dbspec/board.dbs":       set,
	})
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	return c, root
}

func open(t testing.TB) (*writepath.Session, string) {
	t.Helper()
	c, root := openCorpus(t)
	s, err := c.Session("statement-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, s) })
	return s, root
}

func run(t testing.TB, s *writepath.Session, text string, params ...string) *statement.Answer {
	t.Helper()
	raw := make([]json.RawMessage, len(params))
	for i, p := range params {
		raw[i] = json.RawMessage(p)
	}
	a, err := statement.Run(s, text, raw)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return a
}

func refused(t *testing.T, s *writepath.Session, text, code string) {
	t.Helper()
	_, err := statement.Run(s, text, nil)
	if err == nil || !strings.Contains(err.Error(), code) {
		t.Fatalf("%s: %v, want %s", text, err, code)
	}
}

func cells(a *statement.Answer) string {
	b, err := json.Marshal(a.Rows)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func file(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestRowStatements holds the statements that read and change rows: every change reaches the corpus's files.
func TestRowStatements(t *testing.T) {
	s, root := open(t)
	if a := run(t, s, "INSERT INTO notes (title, price, done) VALUES (?, ?, ?), ('b', 2500, 1)", `"a"`, `1000`, `0`); a.Affected != 2 || a.LastInsertID != 2 {
		t.Fatalf("insert answered %+v", a)
	}
	if got := file(t, root, "notes.flowmark.md"); !strings.Contains(got, "2.500") || !strings.Contains(got, "1.000") {
		t.Fatalf("a decimal is the scaled integer in SQL and its text in the file:\n%s", got)
	}
	if got := cells(run(t, s, "SELECT title, price FROM notes ORDER BY id")); got != `[["a",1000],["b",2500]]` {
		t.Fatalf("select: %s", got)
	}
	run(t, s, "UPDATE notes SET title = 'c' WHERE id = 2")
	if got := file(t, root, "notes.flowmark.md"); !strings.Contains(got, "| c ") {
		t.Fatalf("update:\n%s", got)
	}
	run(t, s, "WITH g(i) AS (VALUES (1)) INSERT INTO tags (note) SELECT i FROM g")
	run(t, s, "DELETE FROM notes WHERE id = 1")
	if got := cells(run(t, s, "SELECT COUNT(*) FROM tags")); got != `[[0]]` {
		t.Fatalf("a foreign key action of SQLite reaches the corpus: %s", got)
	}
	refused(t, s, "INSERT INTO tags (note) VALUES (99)", "FOREIGN_KEY")
	refused(t, s, "SELECT * FROM missing", "DRIVER")
	refused(t, s, "UPDATE notes SET title = NULL", "CONSTRAINT")
}

// TestStatementsInATransaction holds a statement in a transaction: it reads and writes the transaction's image.
func TestStatementsInATransaction(t *testing.T) {
	s, root := open(t)
	run(t, s, "BEGIN IMMEDIATE")
	run(t, s, "INSERT INTO notes (title) VALUES ('a')")
	if got := cells(run(t, s, "SELECT COUNT(*) FROM notes")); got != `[[1]]` {
		t.Fatalf("own write: %s", got)
	}
	refused(t, s, "INSERT INTO tags (note) VALUES (99)", "FOREIGN_KEY")
	if got := cells(run(t, s, "SELECT COUNT(*) FROM notes")); got != `[[1]]` {
		t.Fatalf("after a refused statement: %s", got)
	}
	run(t, s, "ROLLBACK")
	if got := cells(run(t, s, "SELECT COUNT(*) FROM notes")); got != `[[0]]` {
		t.Fatalf("after the rollback: %s", got)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.flowmark.md")); err == nil {
		t.Fatal("a rolled back write reached the corpus")
	}
	run(t, s, "PRAGMA query_only=ON")
	refused(t, s, "INSERT INTO notes (title) VALUES ('a')", "READ_ONLY")
	run(t, s, "PRAGMA query_only=OFF")
}

// TestSchemaStatements holds the schema statements: they stand across statements and across a new opening of the corpus.
func TestSchemaStatements(t *testing.T) {
	s, root := open(t)
	run(t, s, "CREATE TABLE plain (name TEXT NOT NULL, n INTEGER)")
	run(t, s, "INSERT INTO plain (name, n) VALUES ('x', 1), ('y', 2)")
	if got := cells(run(t, s, "SELECT name, n FROM plain ORDER BY n")); got != `[["x",1],["y",2]]` {
		t.Fatalf("a table without a primary key: %s", got)
	}
	if got := file(t, root, "plain.flowmark.md"); !strings.Contains(got, "| x ") {
		t.Fatalf("its rows reach the corpus:\n%s", got)
	}
	run(t, s, "ALTER TABLE plain ADD COLUMN extra TEXT")
	run(t, s, "UPDATE plain SET extra = 'e' WHERE n = 2")
	if got := cells(run(t, s, "SELECT name, extra FROM plain ORDER BY n")); got != `[["x",null],["y","e"]]` {
		t.Fatalf("an added column: %s", got)
	}
	run(t, s, "CREATE VIEW plain_names AS SELECT name FROM plain")
	run(t, s, "CREATE TRIGGER plain_guard BEFORE DELETE ON plain WHEN OLD.name = 'x' BEGIN SELECT RAISE(ABORT, 'kept'); END")
	refused(t, s, "DELETE FROM plain WHERE name = 'x'", "kept")
	if got := cells(run(t, s, "SELECT name FROM plain_names ORDER BY name")); got != `[["x"],["y"]]` {
		t.Fatalf("a view: %s", got)
	}
	run(t, s, "BEGIN IMMEDIATE")
	run(t, s, "CREATE TABLE inside (id INTEGER PRIMARY KEY)")
	run(t, s, "INSERT INTO inside (id) VALUES (1)")
	run(t, s, "COMMIT")
	reopened, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, reopened, err)
	again, err := reopened.Session("again")
	if err != nil {
		t.Fatal(err)
	}
	defer closeSession(t, again)
	if got := cells(run(t, again, "SELECT COUNT(*) FROM inside")); got != `[[1]]` {
		t.Fatalf("after a new opening: %s", got)
	}
	run(t, again, "DROP TABLE plain")
	if _, err := os.Stat(filepath.Join(root, "plain.flowmark.md")); err == nil {
		t.Fatal("a dropped table left its file")
	}
	refused(t, again, "NOT SQL", "DRIVER")
	refused(t, again, "CREATE TABLE notes (id INTEGER)", "DRIVER")
	run(t, again, "CREATE TABLE money (id INTEGER PRIMARY KEY, amount DECIMALINT(10,2))")
	run(t, again, "INSERT INTO money (id, amount) VALUES (1, 1.25)")
	refused(t, again, "CREATE TABLE huge (amount DECIMALINT(99999999999999999999,2))", "its precision")
	refused(t, again, "CREATE TABLE wide (amount DECIMALINT(10,99999999999999999999))", "its scale")
}

// A result larger than a batch goes in batches, and ending it early frees the corpus for the next statement.
func TestRowsGoInBatches(t *testing.T) {
	s, _ := open(t)
	run(t, s, "CREATE TABLE numbers (n INTEGER PRIMARY KEY)")
	run(t, s, "INSERT INTO numbers (n) VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(10)")
	first := run(t, s, "SELECT a.n FROM numbers a, numbers b, numbers c, numbers d")
	if !first.More || len(first.Rows) != 1000 {
		t.Fatalf("first batch: more %v, %d rows", first.More, len(first.Rows))
	}
	total := len(first.Rows)
	for more := first.More; more; {
		next, err := statement.Fetch(s)
		if err != nil {
			t.Fatal(err)
		}
		total += len(next.Rows)
		more = next.More
	}
	if total != 10000 {
		t.Fatalf("all batches hold %d rows, want 2500", total)
	}
	if !run(t, s, "SELECT a.n FROM numbers a, numbers b, numbers c, numbers d").More {
		t.Fatal("the second read has no more rows")
	}
	if err := statement.Close(s); err != nil {
		t.Fatal(err)
	}
	run(t, s, "INSERT INTO numbers (n) VALUES (11)")
	if got := cells(run(t, s, "SELECT COUNT(*) FROM numbers")); got != `[[11]]` {
		t.Fatalf("after the closed rows: %s", got)
	}
}

// A statement of WITH is a read when its common table expressions lead to a read.
func TestWithIsRead(t *testing.T) {
	s, _ := open(t)
	run(t, s, "CREATE TABLE numbers (n INTEGER PRIMARY KEY)")
	a := run(t, s, "WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM c WHERE n < 2500) SELECT n FROM c")
	if !a.More {
		t.Fatal("a large WITH read is not in batches")
	}
	run(t, s, "WITH v(n) AS (VALUES (7)) INSERT INTO numbers (n) SELECT n FROM v")
	if got := cells(run(t, s, "SELECT n FROM numbers")); got != `[[7]]` {
		t.Fatalf("a WITH write: %s", got)
	}
}

// openTwo opens two sessions of one corpus.
func openTwo(t *testing.T) (*writepath.Session, *writepath.Session, string) {
	t.Helper()
	c, root := openCorpus(t)
	a, err := c.Session("statement-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Session("statement-b")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeSession(t, a)
		closeSession(t, b)
	})
	return a, b, root
}

func fails(t *testing.T, s *writepath.Session, text, code, part string) {
	t.Helper()
	_, err := statement.Run(s, text, nil)
	if err == nil {
		t.Fatalf("%s: no error; want %s", text, code)
	}
	var f interface{ Failure() (string, string) }
	if !errors.As(err, &f) {
		t.Fatalf("%s: %v has no code", text, err)
	}
	if got, msg := f.Failure(); got != code || !strings.Contains(err.Error(), part) {
		t.Fatalf("%s: %s %s (%v), want %s holding %q", text, got, msg, err, code, part)
	}
}

// TestDeferredTransaction pins a plain BEGIN: it takes the lock at its first write, reads the corpus its first read
// fixes, and a write after a newer commit is refused as SQLite refuses it.
func TestDeferredTransaction(t *testing.T) {
	a, b, _ := openTwo(t)
	count := func(s *writepath.Session) string {
		t.Helper()
		return fmt.Sprint(run(t, s, "SELECT COUNT(*) FROM notes").Rows[0][0])
	}
	run(t, b, "PRAGMA busy_timeout = 0")
	t.Run("a read inside it does not wait for a writer", func(t *testing.T) {
		run(t, a, "BEGIN IMMEDIATE")
		run(t, a, "INSERT INTO notes (title) VALUES ('pending')")
		run(t, b, "BEGIN")
		if got := count(b); got != "0" {
			t.Fatalf("read %s rows of a writer's open transaction", got)
		}
		fails(t, b, "INSERT INTO notes (title) VALUES ('other')", "CANCELED", "(SQLITE_BUSY)")
		run(t, b, "ROLLBACK")
		run(t, a, "ROLLBACK")
	})
	t.Run("its snapshot holds across a commit", func(t *testing.T) {
		run(t, b, "BEGIN")
		before := count(b)
		run(t, a, "INSERT INTO notes (title) VALUES ('committed')")
		if got := count(b); got != before {
			t.Fatalf("read %s rows, its snapshot held %s", got, before)
		}
		fails(t, b, "INSERT INTO notes (title) VALUES ('stale')", "CANCELED", "(SQLITE_BUSY_SNAPSHOT)")
		run(t, b, "ROLLBACK")
		if got := count(b); got == before {
			t.Fatalf("a new read still reads %s rows", got)
		}
	})
	t.Run("its first write takes the lock, with its savepoints", func(t *testing.T) {
		before := count(a)
		run(t, a, "BEGIN")
		run(t, a, "SAVEPOINT s")
		run(t, a, "INSERT INTO notes (title) VALUES ('kept')")
		fails(t, b, "INSERT INTO notes (title) VALUES ('waits')", "CANCELED", "(SQLITE_BUSY)")
		run(t, a, "ROLLBACK TO s")
		run(t, a, "COMMIT")
		if got := count(a); got != before {
			t.Fatalf("%s rows after the rollback to a savepoint, want %s", got, before)
		}
	})
	t.Run("a savepoint it does not hold is refused", func(t *testing.T) {
		run(t, a, "BEGIN")
		fails(t, a, "RELEASE nothing", "DRIVER", "no such savepoint")
		fails(t, a, "BEGIN", "DRIVER", "within a transaction")
		run(t, a, "COMMIT")
	})
}

// TestExclusiveLockingMode pins locking_mode: the lock outlives the transaction until the mode is normal again.
func TestExclusiveLockingMode(t *testing.T) {
	a, b, _ := openTwo(t)
	run(t, b, "PRAGMA busy_timeout = 0")
	run(t, a, "PRAGMA locking_mode = EXCLUSIVE")
	run(t, a, "BEGIN EXCLUSIVE")
	run(t, a, "COMMIT")
	fails(t, b, "BEGIN IMMEDIATE", "CANCELED", "(SQLITE_BUSY)")
	run(t, a, "INSERT INTO notes (title) VALUES ('mine')")
	fails(t, b, "INSERT INTO notes (title) VALUES ('other')", "CANCELED", "(SQLITE_BUSY)")
	run(t, a, "PRAGMA locking_mode = NORMAL")
	run(t, b, "INSERT INTO notes (title) VALUES ('other')")
}

// freshSession is a session of a new corpus that reads the files of root, so nothing of another session's database stands.
func freshSession(t *testing.T, root string) *writepath.Session {
	t.Helper()
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	fresh, err := c.Session("statement-fresh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, fresh) })
	return fresh
}

// TestStoredLiterals pins that a value SQLite holds comes back the same from the corpus, its storage class included,
// whatever its column declares and whether or not a cell can hold it as text.
func TestStoredLiterals(t *testing.T) {
	s, root := open(t)
	run(t, s, "CREATE TABLE mix (id INTEGER PRIMARY KEY, note TEXT, data BLOB, wide)")
	values := []struct{ name, insert, want string }{
		{"a blob in a text column", "X'6162', NULL, NULL", `[{"bytes":"YWI="},"blob",null,"null",null,"null"]`},
		{"a text in a blob column", "NULL, 'abcd', NULL", `[null,"null","abcd","text",null,"null"]`},
		{"a text that is not UTF-8", "CAST(X'FF' AS TEXT), NULL, NULL", `[{"text":"/w=="},"text",null,"null",null,"null"]`},
		{"a text of two lines", "'a' || char(10) || 'b', NULL, NULL", `["a\nb","text",null,"null",null,"null"]`},
		{"a text of raw HTML", "'<b>hi</b>', NULL, NULL", `["\u003cb\u003ehi\u003c/b\u003e","text",null,"null",null,"null"]`},
		{"a text that looks like a literal", "'x''6162''', NULL, NULL", `["x'6162'","text",null,"null",null,"null"]`},
		{"a number with no affinity", "NULL, NULL, 7", `[null,"null",null,"null",7,"integer"]`},
		{"a number in a blob column", "NULL, 7, NULL", `[null,"null",7,"integer",null,"null"]`},
		{"a fraction with no affinity", "NULL, NULL, 1.5", `[null,"null",null,"null",1.5,"real"]`},
		{"a text of digits with no affinity", "NULL, NULL, '7'", `[null,"null",null,"null","7","text"]`},
		{"an infinity", "NULL, NULL, 9e999", `[null,"null",null,"null",{"float":"+Inf"},"real"]`},
	}
	read := func(s *writepath.Session, id int) string {
		a := run(t, s, "SELECT note, typeof(note), data, typeof(data), wide, typeof(wide) FROM mix WHERE id = ?", fmt.Sprint(id))
		return strings.TrimSuffix(strings.TrimPrefix(cells(a), "["), "]")
	}
	for i, v := range values {
		run(t, s, fmt.Sprintf("INSERT INTO mix (id, note, data, wide) VALUES (%d, %s)", i+1, v.insert))
		if got := read(s, i+1); got != v.want {
			t.Errorf("%s: read %s, want %s", v.name, got, v.want)
		}
	}
	// A new corpus reads the files: nothing of the session's database stands.
	fresh := freshSession(t, root)
	for i, v := range values {
		if got := read(fresh, i+1); got != v.want {
			t.Errorf("%s: read %s from the files, want %s", v.name, got, v.want)
		}
	}
}

// affinityValues are the values TestAffinityRoundTrip inserts into a column of each affinity.
var affinityValues = []string{"2", "2.5", "'abc'", "'2'", "X'00'", "NULL", "1e300", "5", "'true'", "'x''6162'''", "9223372036854775807"}

// The answers of affinityValues in a column of each affinity, as SQLite gives them.
var (
	integerAffinity = []string{`[2,"integer"]`, `[2.5,"real"]`, `["abc","text"]`, `[2,"integer"]`, `[{"bytes":"AA=="},"blob"]`, `[null,"null"]`, `[1e+300,"real"]`, `[5,"integer"]`, `["true","text"]`, `["x'6162'","text"]`, `[9223372036854775807,"integer"]`}
	realAffinity    = []string{`[2.0,"real"]`, `[2.5,"real"]`, `["abc","text"]`, `[2.0,"real"]`, `[{"bytes":"AA=="},"blob"]`, `[null,"null"]`, `[1e+300,"real"]`, `[5.0,"real"]`, `["true","text"]`, `["x'6162'","text"]`, `[9223372036854776000.0,"real"]`}
	textAffinity    = []string{`["2","text"]`, `["2.5","text"]`, `["abc","text"]`, `["2","text"]`, `[{"bytes":"AA=="},"blob"]`, `[null,"null"]`, `["1.0e+300","text"]`, `["5","text"]`, `["true","text"]`, `["x'6162'","text"]`, `["9223372036854775807","text"]`}
	blobAffinity    = []string{`[2,"integer"]`, `[2.5,"real"]`, `["abc","text"]`, `["2","text"]`, `[{"bytes":"AA=="},"blob"]`, `[null,"null"]`, `[1e+300,"real"]`, `[5,"integer"]`, `["true","text"]`, `["x'6162'","text"]`, `[9223372036854775807,"integer"]`}
)

// TestAffinityRoundTrip pins that a corpus read again from its files gives each value of a table that a statement made the
// storage class and the value its session gave it, for a column of each affinity SQLite assigns a declared type: the
// session answers what SQLite's affinity gives, and a fresh corpus answers the same. Its rejecting cases are the answers
// the column's type made of a value its affinity kept, before the stored form followed the affinity.
func TestAffinityRoundTrip(t *testing.T) {
	columns := []struct {
		declared string
		want     []string
		wrong    []string
	}{
		{"INTEGER", integerAffinity, []string{`[{"bytes":"YWI="},"blob"]`}},
		{"BIGINT", integerAffinity, nil},
		{"DECIMALINT(5,2)", integerAffinity, []string{`[{"bytes":"YWI="},"blob"]`}},
		{"REAL", realAffinity, []string{`[{"bytes":"YWI="},"blob"]`}},
		{"DOUBLE", realAffinity, nil},
		{"NUMERIC", integerAffinity, nil},
		{"DATETIME", integerAffinity, nil},
		{"BOOLEAN", integerAffinity, []string{`[1,"integer"]`, `[0,"integer"]`, `[{"bytes":"YWI="},"blob"]`}},
		{"TEXT", textAffinity, nil},
		{"VARCHAR(8)", textAffinity, nil},
		{"CHAR BLOB", textAffinity, nil},
		{"BLOB", blobAffinity, nil},
		{"", blobAffinity, []string{`["5","text"]`, `["9223372036854775807","text"]`}},
	}
	for _, col := range columns {
		t.Run(fmt.Sprintf("%q", col.declared), func(t *testing.T) {
			s, root := open(t)
			run(t, s, "CREATE TABLE kept (id INTEGER PRIMARY KEY, c "+col.declared+")")
			read := func(s *writepath.Session, id int) string {
				a := run(t, s, "SELECT c, typeof(c) FROM kept WHERE id = ?", fmt.Sprint(id))
				return strings.TrimSuffix(strings.TrimPrefix(cells(a), "["), "]")
			}
			for i, v := range affinityValues {
				run(t, s, fmt.Sprintf("INSERT INTO kept (id, c) VALUES (%d, %s)", i+1, v))
				if got := read(s, i+1); got != col.want[i] {
					t.Errorf("%s: the session read %s, want %s", v, got, col.want[i])
				}
			}
			fresh := freshSession(t, root)
			for i, v := range affinityValues {
				got := read(fresh, i+1)
				if got != col.want[i] {
					t.Errorf("%s: read %s from the files, want %s", v, got, col.want[i])
				}
				if slices.Contains(col.wrong, got) && got != col.want[i] {
					t.Errorf("%s: read %s from the files, a form its affinity did not give", v, got)
				}
			}
			// A text of a column of TEXT affinity is its cell's text, not a literal, whatever else its type names.
			if col.declared == "CHAR BLOB" {
				if got := file(t, root, "kept.flowmark.md"); !strings.Contains(got, "| abc ") || strings.Contains(got, "t'616263'") {
					t.Errorf("the text abc is not written as its text:\n%s", got)
				}
			}
		})
	}
}

// TestAffinityKeys pins the keys of a table that a statement made: a key cell has no stored literal, so a key value whose
// storage class its column's text does not give back is CONSTRAINT, and the statement leaves no row; every other key
// value reads back from the files with the storage class its session gave it.
func TestAffinityKeys(t *testing.T) {
	accepted := []struct{ declared, value, want string }{
		{"NUMERIC", "5", `[[5,"integer"]]`},
		{"DATETIME", "5", `[[5,"integer"]]`},
		{"NUMERIC", "'abc'", `[["abc","text"]]`},
		{"TEXT", "5", `[["5","text"]]`},
		{"", "'abc'", `[["abc","text"]]`},
		{"", "'5'", `[["5","text"]]`},
		{"INTEGER", "5", `[[5,"integer"]]`},
		{"REAL", "5", `[[5.0,"real"]]`},
		{"REAL", "2.5", `[[2.5,"real"]]`},
		{"BOOLEAN", "1", `[[1,"integer"]]`},
	}
	for _, c := range accepted {
		s, root := open(t)
		run(t, s, "CREATE TABLE k (id "+c.declared+" NOT NULL PRIMARY KEY, v) WITHOUT ROWID")
		run(t, s, "INSERT INTO k (id, v) VALUES ("+c.value+", 1)")
		run(t, s, "UPDATE k SET v = 2 WHERE v = 1")
		for _, session := range []*writepath.Session{s, freshSession(t, root)} {
			if got := cells(run(t, session, "SELECT id, typeof(id) FROM k WHERE v = 2")); got != c.want {
				t.Errorf("%q key %s: read %s, want %s", c.declared, c.value, got, c.want)
			}
		}
	}
	needsLiteral := []struct{ declared, value string }{
		{"", "5"},
		{"", "2.5"},
		{"", "X'00'"},
		{"BLOB", "5"},
		{"BOOLEAN", "5"},
		{"BOOLEAN", "'true'"},
		{"INTEGER", "'abc'"},
		{"REAL", "'abc'"},
		{"TEXT", "X'00'"},
	}
	for _, c := range needsLiteral {
		s, _ := open(t)
		run(t, s, "CREATE TABLE k (id "+c.declared+" NOT NULL PRIMARY KEY, v) WITHOUT ROWID")
		refused(t, s, "INSERT INTO k (id, v) VALUES ("+c.value+", 1)", "CONSTRAINT")
		if got := cells(run(t, s, "SELECT COUNT(*) FROM k")); got != "[[0]]" {
			t.Errorf("%q key %s: the refused insert left %s rows", c.declared, c.value, got)
		}
	}
}

// affinityDeclared are the declared types FuzzAffinityRoundTrip makes a column of: one or more of each affinity.
var affinityDeclared = []string{"INTEGER", "DECIMALINT(5,2)", "REAL", "NUMERIC", "BOOLEAN", "DATETIME", "TEXT", "CHAR BLOB", "BLOB", ""}

// FuzzAffinityRoundTrip pins TestAffinityRoundTrip's equality for any value bound into a column of any affinity: an
// INTEGER, a REAL, a text or a blob, which a fresh corpus reads back as the session answered it.
func FuzzAffinityRoundTrip(f *testing.F) {
	f.Add(uint8(9), uint8(0), int64(2), 0.0, "")
	f.Add(uint8(4), uint8(0), int64(5), 0.0, "")
	f.Add(uint8(4), uint8(2), int64(0), 0.0, "true")
	f.Add(uint8(1), uint8(2), int64(0), 0.0, "abc")
	f.Add(uint8(0), uint8(2), int64(0), 0.0, "x'6162'")
	f.Add(uint8(9), uint8(1), int64(0), 2.0, "")
	f.Add(uint8(2), uint8(1), int64(0), math.Copysign(0, -1), "")
	f.Add(uint8(3), uint8(3), int64(0), 0.0, "ab")
	f.Add(uint8(3), uint8(0), int64(5), 0.0, "")
	f.Add(uint8(7), uint8(2), int64(0), 0.0, "abc")
	f.Fuzz(func(t *testing.T, declared, kind uint8, n int64, x float64, text string) {
		var param string
		switch kind % 4 {
		case 0:
			param = strconv.FormatInt(n, 10)
		case 1:
			if math.IsNaN(x) {
				return
			}
			b, err := statement.AnswerJSON(x)
			if err != nil {
				t.Fatal(err)
			}
			param = string(b)
		case 2:
			b, err := json.Marshal(text)
			if err != nil {
				t.Fatal(err)
			}
			param = string(b)
			if !utf8.ValidString(text) {
				param = `{"text":"` + base64.StdEncoding.EncodeToString([]byte(text)) + `"}`
			}
		default:
			param = `{"bytes":"` + base64.StdEncoding.EncodeToString([]byte(text)) + `"}`
		}
		s, root := open(t)
		run(t, s, "CREATE TABLE kept (id INTEGER PRIMARY KEY, c "+affinityDeclared[int(declared)%len(affinityDeclared)]+")")
		run(t, s, "INSERT INTO kept (id, c) VALUES (1, ?)", param)
		const read = "SELECT c, typeof(c) FROM kept"
		live := cells(run(t, s, read))
		if got := cells(run(t, freshSession(t, root), read)); got != live {
			t.Fatalf("%s in %q: read %s from the files, the session read %s", param, affinityDeclared[int(declared)%len(affinityDeclared)], got, live)
		}
	})
}

// TestParameterForms holds the bound values that a statement takes: each value form that an answer writes binds the
// value it stands for, so a value read from a row binds back unchanged, and every other object is refused.
func TestParameterForms(t *testing.T) {
	s, _ := open(t)
	run(t, s, "CREATE TABLE vals (id INTEGER PRIMARY KEY, v)")
	accepted := []struct{ name, param, want string }{
		{"an integer", `7`, `[[7,"integer"]]`},
		{"a fraction", `1.5`, `[[1.5,"real"]]`},
		{"a text", `"abc"`, `[["abc","text"]]`},
		{"a null", `null`, `[[null,"null"]]`},
		{"a blob", `{"bytes":"YWI="}`, `[[{"bytes":"YWI="},"blob"]]`},
		{"a text that is not UTF-8", `{"text":"/w=="}`, `[[{"text":"/w=="},"text"]]`},
		{"an infinity", `{"float":"+Inf"}`, `[[{"float":"+Inf"},"real"]]`},
		{"a negative infinity", `{"float":"-Inf"}`, `[[{"float":"-Inf"},"real"]]`},
	}
	for i, v := range accepted {
		run(t, s, fmt.Sprintf("INSERT INTO vals (id, v) VALUES (%d, ?)", i+1), v.param)
		a := run(t, s, "SELECT v, typeof(v) FROM vals WHERE id = ?", fmt.Sprint(i+1))
		if got := cells(a); got != v.want {
			t.Errorf("%s: read %s, want %s", v.name, got, v.want)
		}
	}
	refusedForms := []struct{ name, param string }{
		{"an object of no form", `{"x":"1"}`},
		{"a blob that is not base64", `{"bytes":"!"}`},
		{"a text that is not base64", `{"text":"!"}`},
		{"a float that is not an infinity", `{"float":"1"}`},
		{"two forms in one object", `{"bytes":"YQ==","text":"YQ=="}`},
		{"an array", `[1]`},
	}
	for _, v := range refusedForms {
		_, err := statement.Run(s, "SELECT ?", []json.RawMessage{json.RawMessage(v.param)})
		if err == nil || !strings.Contains(err.Error(), "parameter 0") {
			t.Errorf("%s: %v, want a refusal of parameter 0", v.name, err)
		}
	}
}

// TestRealForm pins that an answer tells a REAL from an INTEGER: a finite REAL is a JSON number with a fraction or an
// exponent, an INTEGER an integer literal and an infinity its object form. Each binds back as the value it stands for, and
// a corpus read again from its files answers the same, a REAL stored as a literal included.
func TestRealForm(t *testing.T) {
	s, root := open(t)
	run(t, s, "CREATE TABLE nums (id INTEGER PRIMARY KEY, v, w)")
	values := []struct{ name, insert, want string }{
		{"a whole REAL", "2.0", `[2.0,"real"]`},
		{"a negative zero REAL", "-0.0", `[-0.0,"real"]`},
		{"a REAL of exponent 21", "1e21", `[1e+21,"real"]`},
		{"a REAL with a fraction", "2.5", `[2.5,"real"]`},
		{"a REAL below one millionth", "1e-7", `[1e-7,"real"]`},
		{"a large whole REAL", "123456789.0", `[123456789.0,"real"]`},
		{"an INTEGER", "2", `[2,"integer"]`},
		{"a negative zero INTEGER", "-0", `[0,"integer"]`},
		{"a negative INTEGER", "-3", `[-3,"integer"]`},
		{"an infinity", "9e999", `[{"float":"+Inf"},"real"]`},
		{"a negative infinity", "-9e999", `[{"float":"-Inf"},"real"]`},
	}
	read := func(t *testing.T, s *writepath.Session, column string, id int) string {
		t.Helper()
		a := run(t, s, fmt.Sprintf("SELECT %s, typeof(%s) FROM nums WHERE id = ?", column, column), fmt.Sprint(id))
		return strings.TrimSuffix(strings.TrimPrefix(cells(a), "["), "]")
	}
	for i, v := range values {
		run(t, s, fmt.Sprintf("INSERT INTO nums (id, v) VALUES (%d, %s)", i+1, v.insert))
		if got := read(t, s, "v", i+1); got != v.want {
			t.Errorf("%s: read %s, want %s", v.name, got, v.want)
		}
		// The value the answer wrote binds back as the same value of the same type.
		var row []json.RawMessage
		if err := json.Unmarshal([]byte(v.want), &row); err != nil {
			t.Fatal(err)
		}
		run(t, s, "UPDATE nums SET w = ? WHERE id = ?", string(row[0]), fmt.Sprint(i+1))
		if got := read(t, s, "w", i+1); got != v.want {
			t.Errorf("%s: %s binds back as %s", v.name, row[0], got)
		}
	}
	// A new corpus reads the files: nothing of the session's database stands. A REAL or an INTEGER that a cell cannot hold
	// as its column's type is a stored literal, which reads back as itself.
	fresh := freshSession(t, root)
	for i, v := range values {
		for _, column := range []string{"v", "w"} {
			if got := read(t, fresh, column, i+1); got != v.want {
				t.Errorf("%s: read %s of %s from the files, want %s", v.name, got, column, v.want)
			}
		}
	}
	// The forms an answer never writes: a REAL as an integer literal, an INTEGER with a fraction, an infinity as a number.
	wrong := []struct{ name, value, form string }{
		{"a whole REAL as an integer", "2.0", `[[2,"real"]]`},
		{"a negative zero REAL as an integer", "-0.0", `[[-0,"real"]]`},
		{"a REAL of exponent 21 as an integer", "1e21", `[[1000000000000000000000,"real"]]`},
		{"an INTEGER with a fraction", "2", `[[2.0,"integer"]]`},
		{"a negative zero INTEGER as a REAL", "-0", `[[-0.0,"integer"]]`},
		{"an infinity as a number", "9e999", `[[1e999,"real"]]`},
	}
	for _, v := range wrong {
		if got := cells(run(t, s, "SELECT "+v.value+", typeof("+v.value+")")); got == v.form {
			t.Errorf("%s: answered %s", v.name, got)
		}
	}
}

// TestAnswerNumbers pins the JSON of each number a row holds: a finite REAL is the shortest text that reads back as it,
// with .0 when that text has neither a fraction nor an exponent, an INTEGER its integer literal, an infinity its object
// form, and a NaN, which SQLite never stores, an error.
func TestAnswerNumbers(t *testing.T) {
	accepted := []struct {
		name string
		v    any
		want string
	}{
		{"a whole REAL", 2.0, `2.0`},
		{"a negative zero REAL", math.Copysign(0, -1), `-0.0`},
		{"a zero REAL", 0.0, `0.0`},
		{"a REAL of exponent 21", 1e21, `1e+21`},
		{"the largest REAL written without an exponent", 1e20, `100000000000000000000.0`},
		{"a REAL with a fraction", 2.5, `2.5`},
		{"a REAL below one millionth", 1e-7, `1e-7`},
		{"a negative whole REAL", -3.0, `-3.0`},
		{"the largest finite REAL", math.MaxFloat64, `1.7976931348623157e+308`},
		{"the smallest REAL", math.SmallestNonzeroFloat64, `5e-324`},
		{"an INTEGER", int64(2), `2`},
		{"a negative zero INTEGER", int64(-0), `0`},
		{"the largest INTEGER", int64(math.MaxInt64), `9223372036854775807`},
		{"an infinity", math.Inf(1), `{"float":"+Inf"}`},
		{"a negative infinity", math.Inf(-1), `{"float":"-Inf"}`},
	}
	for _, c := range accepted {
		got, err := statement.AnswerJSON(c.v)
		if err != nil || string(got) != c.want {
			t.Errorf("%s: wrote %s (%v), want %s", c.name, got, err, c.want)
		}
	}
	refused := []struct {
		name string
		v    any
	}{
		{"a NaN", math.NaN()},
	}
	for _, c := range refused {
		if got, err := statement.AnswerJSON(c.v); err == nil {
			t.Errorf("%s: wrote %s, want an error", c.name, got)
		}
	}
}

// FuzzAnswerReal pins that every finite REAL an answer writes is a JSON number with a fraction or an exponent that reads
// back as the same bits, and that a client reading it as a parameter takes a REAL, never an INTEGER.
func FuzzAnswerReal(f *testing.F) {
	for _, seed := range []float64{2.0, math.Copysign(0, -1), 1e21, 2.5, 1e-7, 123456789.0, math.MaxFloat64} {
		f.Add(math.Float64bits(seed))
	}
	f.Fuzz(func(t *testing.T, bits uint64) {
		x := math.Float64frombits(bits)
		if math.IsInf(x, 0) || math.IsNaN(x) {
			return
		}
		got, err := statement.AnswerJSON(x)
		if err != nil {
			t.Fatalf("%v: %v", x, err)
		}
		if !strings.ContainsAny(string(got), ".eE") {
			t.Fatalf("%v: wrote %s, an integer literal", x, got)
		}
		dec := json.NewDecoder(strings.NewReader(string(got)))
		dec.UseNumber()
		var n json.Number
		if err := dec.Decode(&n); err != nil {
			t.Fatalf("%v: wrote %s, no JSON number: %v", x, got, err)
		}
		if _, err := n.Int64(); err == nil {
			t.Fatalf("%v: wrote %s, which reads as an integer", x, got)
		}
		back, err := n.Float64()
		if err != nil || math.Float64bits(back) != bits {
			t.Fatalf("%v: wrote %s, which reads back as %v", x, got, back)
		}
	})
}

// TestLiveTables pins what a table takes from its statement: a key that holds no NULL, the type it declares, and rows of
// any key text.
func TestLiveTables(t *testing.T) {
	s, _ := open(t)
	t.Run("a primary key that may hold NULL keys by the rowid", func(t *testing.T) {
		run(t, s, "CREATE TABLE open_key (label VARCHAR(8) PRIMARY KEY, n INTEGER)")
		run(t, s, "INSERT INTO open_key VALUES (NULL, 1), (NULL, 2), ('a', 3)")
		if got := fmt.Sprint(run(t, s, "SELECT COUNT(*) FROM open_key WHERE label IS NULL").Rows[0][0]); got != "2" {
			t.Fatalf("%s rows of a NULL key, want 2", got)
		}
	})
	t.Run("a table made again takes its declared types", func(t *testing.T) {
		run(t, s, "DROP TABLE tags")
		run(t, s, "DROP TABLE notes")
		run(t, s, "CREATE TABLE notes (id REAL NOT NULL PRIMARY KEY, title TEXT, price TEXT, done TEXT, at TEXT, body TEXT)")
		run(t, s, "INSERT INTO notes (id, title) VALUES (1.5, 'a')")
		if got := fmt.Sprint(run(t, s, "SELECT id FROM notes").Rows[0][0]); got != "1.5" {
			t.Fatalf("read %s, want 1.5", got)
		}
	})
	t.Run("a key is any single-line text, or a number of either sign", func(t *testing.T) {
		run(t, s, "CREATE TABLE named (name TEXT NOT NULL PRIMARY KEY)")
		for _, key := range []string{"é", "a b", "a/b", "007", "-_", "[x]", "a|b"} {
			run(t, s, "INSERT INTO named VALUES ('"+key+"')")
		}
		run(t, s, "CREATE TABLE numbered (n INTEGER PRIMARY KEY)")
		run(t, s, "INSERT INTO numbered VALUES (-1), (1)")
		if got := fmt.Sprint(run(t, s, "SELECT COUNT(*) FROM named").Rows[0][0]); got != "7" {
			t.Fatalf("%s keys, want 7", got)
		}
		if got := fmt.Sprint(run(t, s, "SELECT MIN(n) FROM numbered").Rows[0][0]); got != "-1" {
			t.Fatalf("least key %s, want -1", got)
		}
	})
}

// TestManyRowsOfOneStatement pins that the rows of one statement go to the corpus at once: this many rows one request at
// a time, each rendering the document, does not end within the test's deadline.
func TestManyRowsOfOneStatement(t *testing.T) {
	s, _ := open(t)
	run(t, s, "WITH RECURSIVE g(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM g WHERE i < 5000) INSERT INTO notes (title) SELECT 'n-' || i FROM g")
	run(t, s, "UPDATE notes SET done = 1 WHERE id % 2 = 0")
	if got := fmt.Sprint(run(t, s, "SELECT COUNT(*) FROM notes WHERE done = 1").Rows[0][0]); got != "2500" {
		t.Fatalf("%s rows updated, want 2500", got)
	}
	run(t, s, "DELETE FROM notes")
	if got := fmt.Sprint(run(t, s, "SELECT COUNT(*) FROM notes").Rows[0][0]); got != "0" {
		t.Fatalf("%s rows left", got)
	}
}

// openBoard opens a session on a corpus of the board catalog, whose items have a text key and an order column.
func openBoard(t *testing.T) (*writepath.Session, string) {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["board.dbs"]}`,
		"dbspec/board.dbs":       corpustest.BoardDBS,
	})
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	s, err := c.Session("statement-board")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, s) })
	return s, root
}

// TestNoAssignedColumns pins that the corpus assigns no value SQLite does not assign (spec 20 Keys and Order): an insert
// that leaves out a text key or an order column fails as SQLite fails a NOT NULL column without a default, the key
// column reads as NOT NULL, and an order value is what the statement wrote, in the session and in a new corpus alike.
func TestNoAssignedColumns(t *testing.T) {
	s, root := openBoard(t)
	fails(t, s, "INSERT INTO items (title, body, position) VALUES ('a', '', 1)", "DRIVER", "NOT NULL constraint failed: items.id")
	fails(t, s, "INSERT INTO items (id, title, body) VALUES ('T1', 'a', '')", "DRIVER", "NOT NULL constraint failed: items.position")
	if got := fmt.Sprint(run(t, s, "INSERT INTO items (id, title, body, position) VALUES ('T2', 'b', '', 20), ('T1', 'a', '', -5) RETURNING id").Rows); got != "[[T2] [T1]]" {
		t.Fatalf("returned %s, want [[T2] [T1]]", got)
	}
	run(t, s, "UPDATE items SET position = 7 WHERE id = 'T1'")
	if got := fmt.Sprint(run(t, s, `SELECT "notnull", pk FROM pragma_table_info('items') WHERE name = 'id'`).Rows[0]); got != "[1 1]" {
		t.Fatalf("the key column reads %s, want [1 1]", got)
	}
	// A new corpus reads the files: the values are those the statements wrote.
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	fresh, err := c.Session("statement-board-fresh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, fresh) })
	const positions = "SELECT id, position, title FROM items ORDER BY position"
	want := "[[T1 7 a] [T2 20 b]]"
	if got := fmt.Sprint(run(t, fresh, positions).Rows); got != want {
		t.Fatalf("the files hold %s, want %s", got, want)
	}
	if got := fmt.Sprint(run(t, s, positions).Rows); got != want {
		t.Fatalf("the session reads %s, want %s", got, want)
	}
}

// closeSession closes the session s and fails the test when the close fails.
func closeSession(t testing.TB, s io.Closer) {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Error(err)
	}
}

// TestSequences pins that an AUTOINCREMENT key is never given twice, as SQLite's AUTOINCREMENT gives none (plan, A
// refresh of the changed rows): the counter of each AUTOINCREMENT table is part of the corpus, in dbspec/sequences.json,
// so a database filled after a new opening, a database another process refreshes to the commit, and the database of the
// session itself give the next key after the largest key the table gave or held, the deleted ones included. A table
// whose key is a plain INTEGER PRIMARY KEY reuses the largest key a delete freed, as SQLite does; a renamed table keeps
// its counter, and a dropped table takes its counter with it.
func TestSequences(t *testing.T) {
	counted := "CREATE TABLE counted (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT)"
	cases := []struct {
		name string
		// create creates the tables of the case that the set does not declare.
		create []string
		// steps run in the first session; table is the table the inserts after them insert into.
		steps []string
		table string
		// opened is the number of the steps after which the other process opens the corpus: a process takes the schema
		// statements its opening found (spec 40 Opening a corpus).
		opened int
		// want is the key the first insert after the steps gives.
		want int
	}{
		{"a declared identity key after a delete of the largest", nil, []string{
			"INSERT INTO notes (title) VALUES ('a'), ('b'), ('c')",
			"DELETE FROM notes WHERE id = 3",
		}, "notes", 0, 4},
		{"an AUTOINCREMENT key of SQL after a delete of the largest", []string{counted}, []string{
			"INSERT INTO counted (title) VALUES ('a'), ('b')",
			"DELETE FROM counted WHERE id = 2",
		}, "counted", 0, 3},
		{"a key inserted and deleted in one transaction", nil, []string{
			"INSERT INTO notes (title) VALUES ('a')",
			"BEGIN IMMEDIATE",
			"INSERT INTO notes (title) VALUES ('b')",
			"DELETE FROM notes WHERE id = 2",
			"COMMIT",
		}, "notes", 0, 3},
		{"a key inserted and deleted by one statement", []string{counted,
			"CREATE TRIGGER counted_gone AFTER INSERT ON counted BEGIN DELETE FROM counted WHERE id = NEW.id; END"}, []string{
			"INSERT INTO counted (title) VALUES ('a'), ('b')",
		}, "counted", 0, 3},
		{"a key written by hand and deleted", nil, []string{
			"INSERT INTO notes (id, title) VALUES (10, 'a')",
			"DELETE FROM notes WHERE id = 10",
		}, "notes", 0, 11},
		{"every row deleted", []string{counted}, []string{
			"INSERT INTO counted (title) VALUES ('a'), ('b'), ('c')",
			"DELETE FROM counted",
		}, "counted", 0, 4},
		{"a renamed table", []string{counted}, []string{
			"INSERT INTO counted (title) VALUES ('a'), ('b')",
			"DELETE FROM counted WHERE id = 2",
			"ALTER TABLE counted RENAME TO renamed",
		}, "renamed", 3, 3},
		{"a dropped table made again", []string{counted}, []string{
			"INSERT INTO counted (title) VALUES ('a'), ('b')",
			"DELETE FROM counted WHERE id = 2",
			"DROP TABLE counted",
			counted,
		}, "counted", 4, 1},
		{"a plain INTEGER PRIMARY KEY reuses a freed key", []string{"CREATE TABLE plain (id INTEGER PRIMARY KEY, title TEXT)"}, []string{
			"INSERT INTO plain (title) VALUES ('a'), ('b')",
			"DELETE FROM plain WHERE id = 2",
		}, "plain", 0, 2},
	}
	session := openSession
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, root := open(t)
			for _, text := range tc.create {
				run(t, s, text)
			}
			// Another process holds a database of the corpus from before the steps that write rows.
			var other *writepath.Session
			for i, step := range tc.steps {
				if i == tc.opened {
					other = session(t, root, "other")
					run(t, other, "SELECT COUNT(*) FROM notes")
				}
				run(t, s, step)
			}
			if other == nil {
				other = session(t, root, "other")
				run(t, other, "SELECT COUNT(*) FROM notes")
			}
			insert := "INSERT INTO " + tc.table + " (title) VALUES ('next') RETURNING id"
			for i, sess := range []*writepath.Session{other, session(t, root, "new"), s} {
				if got, want := cells(run(t, sess, insert)), fmt.Sprintf("[[%d]]", tc.want+i); got != want {
					t.Fatalf("insert %d, of %s, gives %s, want %s", i+1, []string{"the other process", "a new opening", "the first session"}[i], got, want)
				}
			}
		})
	}
}

// TestSchemaOfAnotherProcess pins that a process takes the schema statements another process committed, as a SQLite
// connection takes the schema another connection changed (spec 40 SQL statements): after a rename by one process, the
// other refuses a statement on the old name as SQLite does, reads the rows under the new name, and writes none of its
// rows to a document of a table the corpus no longer holds.
func TestSchemaOfAnotherProcess(t *testing.T) {
	s, root := open(t)
	run(t, s, "CREATE TABLE counted (id INTEGER PRIMARY KEY, title TEXT)")
	run(t, s, "INSERT INTO counted (title) VALUES ('a')")
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	other, err := c.Session("other")
	if err != nil {
		t.Fatal(err)
	}
	defer closeSession(t, other)
	run(t, other, "SELECT COUNT(*) FROM counted")
	run(t, s, "ALTER TABLE counted RENAME TO renamed")
	refused(t, other, "INSERT INTO counted (title) VALUES ('b')", "no such table: counted")
	if got := cells(run(t, other, "SELECT title FROM renamed")); got != `[["a"]]` {
		t.Fatalf("the other process reads the renamed table as %s", got)
	}
	run(t, other, "INSERT INTO renamed (title) VALUES ('b')")
	if _, err := os.Stat(filepath.Join(root, "counted.flowmark.md")); err == nil {
		t.Fatal("a write of the other process made a document of the table the rename took away")
	}
	if got := cells(run(t, s, "SELECT title FROM renamed ORDER BY id")); got != `[["a"],["b"]]` {
		t.Fatalf("the first process reads the renamed table as %s", got)
	}
	// A deferred transaction whose snapshot is older than a schema statement that changed no document cannot write.
	run(t, other, "BEGIN")
	run(t, other, "SELECT COUNT(*) FROM renamed")
	run(t, s, "CREATE TABLE extra (id INTEGER PRIMARY KEY)")
	fails(t, other, "INSERT INTO renamed (title) VALUES ('stale')", "CANCELED", "(SQLITE_BUSY_SNAPSHOT)")
	run(t, other, "ROLLBACK")
	run(t, other, "INSERT INTO extra (id) VALUES (1)")
	if got := cells(run(t, s, "SELECT id FROM extra")); got != `[[1]]` {
		t.Fatalf("the first process reads the table the other filled as %s", got)
	}
}
