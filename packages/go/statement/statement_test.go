package statement_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func open(t *testing.T) (*writepath.Session, string) {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["board.dbs"]}`,
		"dbspec/board.dbs":       set,
	})
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	s, err := c.Session("statement-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, root
}

func run(t *testing.T, s *writepath.Session, text string, params ...string) *statement.Answer {
	t.Helper()
	raw := make([]json.RawMessage, len(params))
	for i, p := range params {
		raw[i] = json.RawMessage(p)
	}
	a, err := statement.Run(context.Background(), s, text, raw)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return a
}

func refused(t *testing.T, s *writepath.Session, text, code string) {
	t.Helper()
	_, err := statement.Run(context.Background(), s, text, nil)
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
	defer again.Close()
	if got := cells(run(t, again, "SELECT COUNT(*) FROM inside")); got != `[[1]]` {
		t.Fatalf("after a new opening: %s", got)
	}
	run(t, again, "DROP TABLE plain")
	if _, err := os.Stat(filepath.Join(root, "plain.flowmark.md")); err == nil {
		t.Fatal("a dropped table left its file")
	}
	refused(t, again, "NOT SQL", "DRIVER")
	refused(t, again, "CREATE TABLE notes (id INTEGER)", "DRIVER")
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
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["board.dbs"]}`,
		"dbspec/board.dbs":       set,
	})
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	a, err := c.Session("statement-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Session("statement-b")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})
	return a, b, root
}

func fails(t *testing.T, s *writepath.Session, text, code, part string) {
	t.Helper()
	_, err := statement.Run(context.Background(), s, text, nil)
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

// TestStoredLiterals pins that a value SQLite holds comes back the same from the corpus, whatever its column declares and
// whether or not a cell can hold it as text.
func TestStoredLiterals(t *testing.T) {
	s, root := open(t)
	run(t, s, "CREATE TABLE mix (id INTEGER PRIMARY KEY, note TEXT, data BLOB, wide)")
	values := []struct{ name, insert, note, data, wide string }{
		{"a blob in a text column", "X'6162', NULL, NULL", `map[bytes:YWI=]`, `<nil>`, `<nil>`},
		{"a text in a blob column", "NULL, 'abcd', NULL", `<nil>`, `abcd`, `<nil>`},
		{"a text that is not UTF-8", "CAST(X'FF' AS TEXT), NULL, NULL", `map[text:/w==]`, `<nil>`, `<nil>`},
		{"a text of two lines", "'a' || char(10) || 'b', NULL, NULL", "a\nb", `<nil>`, `<nil>`},
		{"a text of raw HTML", "'<b>hi</b>', NULL, NULL", `<b>hi</b>`, `<nil>`, `<nil>`},
		{"a text that looks like a literal", "'x''6162''', NULL, NULL", `x'6162'`, `<nil>`, `<nil>`},
		{"a number with no affinity", "NULL, NULL, 7", `<nil>`, `<nil>`, `7`},
		{"a fraction with no affinity", "NULL, NULL, 1.5", `<nil>`, `<nil>`, `1.5`},
		{"an infinity", "NULL, NULL, 9e999", `<nil>`, `<nil>`, `map[float:+Inf]`},
	}
	for i, v := range values {
		run(t, s, fmt.Sprintf("INSERT INTO mix (id, note, data, wide) VALUES (%d, %s)", i+1, v.insert))
	}
	// A new corpus reads the files: nothing of the session's database stands.
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	fresh, err := c.Session("statement-fresh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	for i, v := range values {
		a := run(t, fresh, fmt.Sprintf("SELECT note, data, wide FROM mix WHERE id = %d", i+1))
		got := fmt.Sprint(a.Rows[0][0]) + "|" + fmt.Sprint(a.Rows[0][1]) + "|" + fmt.Sprint(a.Rows[0][2])
		if want := v.note + "|" + v.data + "|" + v.wide; got != want {
			t.Errorf("%s: read %q, want %q", v.name, got, want)
		}
	}
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
		_, err := statement.Run(context.Background(), s, "SELECT ?", []json.RawMessage{json.RawMessage(v.param)})
		if err == nil || !strings.Contains(err.Error(), "parameter 0") {
			t.Errorf("%s: %v, want a refusal of parameter 0", v.name, err)
		}
	}
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

// openBoard opens a session on a corpus of the board catalog, whose items take a key_prefix key and an order position.
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
	t.Cleanup(func() { _ = s.Close() })
	return s, root
}

// TestAssignedColumns pins that an insert may leave out what the corpus assigns: a key_prefix key, allocated <p><n + 1>
// over the keys of the form <p><n>, and an order position, the next one under the row's parent. A RETURNING of the key
// reads the key the row was given, and the key column reads as one that may hold NULL, as SQLite's INTEGER PRIMARY KEY
// does.
func TestAssignedColumns(t *testing.T) {
	s, root := openBoard(t)
	returned := func(text string) string {
		t.Helper()
		return fmt.Sprint(run(t, s, text).Rows)
	}
	if got := returned("INSERT INTO items (title, body) VALUES ('a', '') RETURNING id"); got != "[[T1]]" {
		t.Fatalf("returned %s, want [[T1]]", got)
	}
	if got := returned("INSERT INTO items (title, body) VALUES ('b', ''), ('c', '') RETURNING id, title"); got != "[[T2 b] [T3 c]]" {
		t.Fatalf("returned %s, want [[T2 b] [T3 c]]", got)
	}
	run(t, s, "INSERT INTO items (id, title, body) VALUES ('T10', 'd', ''), ('T011', 'e', ''), ('Tx', 'f', '')")
	if got := returned("INSERT INTO items (id, title, body) VALUES (NULL, 'g', '') RETURNING id"); got != "[[T11]]" {
		t.Fatalf("returned %s, want [[T11]]", got)
	}
	// An upsert that meets a row returns that row's key.
	if got := returned("INSERT INTO items (id, title, body) VALUES ('T10', 'd2', '') ON CONFLICT (id) DO UPDATE SET title = excluded.title RETURNING id"); got != "[[T10]]" {
		t.Fatalf("returned %s, want [[T10]]", got)
	}
	run(t, s, "INSERT INTO items (id, title, body) VALUES ('T20', 'h', '')")
	run(t, s, "BEGIN")
	run(t, s, "INSERT INTO items (title, body) VALUES ('i', '')")
	if got := returned("INSERT INTO items (title, body) VALUES ('j', '') RETURNING id"); got != "[[T22]]" {
		t.Fatalf("returned %s in a transaction, want [[T22]]", got)
	}
	run(t, s, "COMMIT")
	if got := fmt.Sprint(run(t, s, `SELECT "notnull", pk FROM pragma_table_info('items') WHERE name = 'id'`).Rows[0]); got != "[0 1]" {
		t.Fatalf("the key column reads %s, want [0 1]", got)
	}
	// A new corpus reads the files: the keys and the positions are the corpus's own.
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	fresh, err := c.Session("statement-board-fresh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	const positions = "SELECT id, position, title FROM items ORDER BY position"
	want := "[[T1 1 a] [T2 2 b] [T3 3 c] [T10 4 d2] [T011 5 e] [Tx 6 f] [T11 7 g] [T20 8 h] [T21 9 i] [T22 10 j]]"
	if got := fmt.Sprint(run(t, fresh, positions).Rows); got != want {
		t.Fatalf("the files hold %s, want %s", got, want)
	}
	if got := fmt.Sprint(run(t, s, positions).Rows); got != want {
		t.Fatalf("the session reads %s, want %s", got, want)
	}
}
