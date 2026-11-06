package statement_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/ops"
	"github.com/polyspec/flowmark/packages/go/statement"
)

// compareDiffs installs the seam that forms the difference of each write statement from every row as well, and fails
// t when the difference of the rows the pre-update hook reported differs from it; it returns the count of the
// statements compared.
func compareDiffs(t *testing.T) *int {
	t.Helper()
	n := new(int)
	t.Cleanup(statement.SetFullDiff(func(text string, captured, full [][]byte) {
		*n++
		if !slices.EqualFunc(captured, full, sameWrite) {
			t.Errorf("%s: the difference of the changed rows is\n%s\nand of every row\n%s", text, joined(captured), joined(full))
		}
	}))
	return n
}

func joined(msgs [][]byte) string { return string(bytes.Join(msgs, []byte("\n"))) }

// sameWrite reports whether two request messages are the same write: equal but for the clock reading each carries.
func sameWrite(a, b []byte) bool {
	strip := func(msg []byte) string {
		var m map[string]any
		if err := json.Unmarshal(msg, &m); err != nil {
			return "not a message: " + string(msg)
		}
		delete(m, "now")
		out, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		return string(out)
	}
	return strip(a) == strip(b)
}

// captureSchema are the tables SQL adds to the board corpus for the cases of TestCapture: a table with no primary key,
// whose key is the rowid; a table with a text key, which a REPLACE and an upsert meet; a trigger that writes a row of
// one for each update of the other; and a table without a rowid of a descending key under a collation.
var captureSchema = []string{
	"CREATE TABLE plain (name TEXT)",
	"CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT)",
	"CREATE TRIGGER kv_log AFTER UPDATE ON kv BEGIN INSERT INTO plain (name) VALUES (NEW.k); END",
	"CREATE TABLE wk (k TEXT COLLATE NOCASE, n INTEGER, v BLOB, PRIMARY KEY (k, n DESC)) WITHOUT ROWID",
}

// TestCapture pins the difference of a write statement (spec 40 SQL statements): formed from the rows SQLite's
// pre-update hook reports the statement changed, it equals the difference formed from every row before and after it,
// for an insert, an update and a delete of many rows, a key that changes, a REPLACE, an upsert, the rows of a foreign key
// action and of a trigger, a table whose key is the rowid, a table whose integer key is its rowid, a table without a
// rowid, whose rows the hook reports by their primary key, of one column and of a descending key under a collation,
// texts and keys holding a NUL, and a statement of a transaction.
func TestCapture(t *testing.T) {
	cases := []struct {
		name  string
		setup []string
		texts []string
	}{
		{"inserts of many rows", nil, []string{
			"INSERT INTO notes (title) VALUES ('a'), ('b'), ('c')",
			"INSERT INTO tags (note) SELECT id FROM notes",
		}},
		{"updates of many rows and of a key", []string{"INSERT INTO notes (title) VALUES ('a'), ('b'), ('c')"}, []string{
			"UPDATE notes SET title = title || '!', done = true",
			"UPDATE notes SET id = id + 100 WHERE title = 'c!'",
			"UPDATE notes SET title = title",
		}},
		{"a delete and the rows of its foreign key action", []string{
			"INSERT INTO notes (title) VALUES ('a'), ('b')",
			"INSERT INTO tags (note) SELECT id FROM notes",
			"INSERT INTO tags (note) SELECT id FROM notes",
		}, []string{
			"DELETE FROM notes WHERE title = 'a'",
			"DELETE FROM notes",
		}},
		{"a table whose key is the rowid", captureSchema, []string{
			"INSERT INTO plain (name) VALUES ('x'), ('y'), ('z')",
			"UPDATE plain SET name = 'w' WHERE name = 'y'",
			"DELETE FROM plain WHERE name <> 'w'",
		}},
		{"a REPLACE, an upsert and a trigger", captureSchema, []string{
			"INSERT INTO kv (k, v) VALUES ('a', '1'), ('b', '2')",
			"REPLACE INTO kv (k, v) VALUES ('a', '3'), ('c', '4')",
			"INSERT INTO kv (k, v) VALUES ('b', '5'), ('d', '6') ON CONFLICT (k) DO UPDATE SET v = excluded.v",
			"UPDATE kv SET v = v || v",
		}},
		{"texts and keys holding a NUL", captureSchema, []string{
			"INSERT INTO kv (k, v) VALUES ('a' || char(0) || 'b', 'x' || char(0)), (char(0), '')",
			"UPDATE kv SET v = v || char(0) || 'y'",
			"REPLACE INTO kv (k, v) VALUES ('a' || char(0) || 'b', 'z')",
			"UPDATE notes SET title = 'n' || char(0) WHERE id IN (SELECT id FROM notes LIMIT 1)",
			"DELETE FROM kv WHERE k = char(0)",
		}},
		{"a table without a rowid", []string{"CREATE TABLE wr (k TEXT PRIMARY KEY, v TEXT) WITHOUT ROWID"}, []string{
			"INSERT INTO wr (k, v) VALUES ('c', '1'), ('a', '2'), ('b', '3')",
			"UPDATE wr SET v = v || '!' WHERE k <> 'b'",
			"UPDATE wr SET k = '0' || k WHERE k <> 'a'",
			"REPLACE INTO wr (k, v) VALUES ('a', '4'), ('0b', '5')",
			"INSERT INTO wr (k, v) VALUES ('d', '6'), ('a', '7') ON CONFLICT (k) DO UPDATE SET v = excluded.v",
			"DELETE FROM wr WHERE k <> '0c'",
		}},
		{"a table without a rowid of a descending key under a collation", []string{
			"CREATE TABLE wc (a TEXT COLLATE NOCASE, b INTEGER, v BLOB, PRIMARY KEY (a, b DESC)) WITHOUT ROWID",
		}, []string{
			"INSERT INTO wc (a, b, v) VALUES ('B', 1, x'00'), ('a', 2, x'01'), ('a', 3, NULL), ('b', 2, x'0200')",
			"UPDATE wc SET v = x'ff' WHERE b >= 2",
			"UPDATE wc SET b = b + 10 WHERE a = 'A' AND b = 3",
			"REPLACE INTO wc (a, b, v) VALUES ('a', 2, x'aa'), ('c', 9, x'bb')",
			"DELETE FROM wc WHERE a = 'b'",
		}},
		{"a key of a table without a rowid changed only in its case", []string{
			"CREATE TABLE wp (k TEXT PRIMARY KEY, v TEXT) WITHOUT ROWID",
			"CREATE TABLE wq (id INTEGER PRIMARY KEY, p TEXT REFERENCES wp (k) ON UPDATE CASCADE ON DELETE RESTRICT)",
			"INSERT INTO wp (k, v) VALUES ('a', '1'), ('b', '2'), ('c', '3')",
			"INSERT INTO wq (p) VALUES ('a'), ('b')",
		}, []string{
			"UPDATE wp SET k = upper(k) WHERE k <> 'c'",
			"UPDATE wp SET k = 'c', v = '4' WHERE k = 'C'",
			"REPLACE INTO wp (k, v) VALUES ('c', '5')",
			"UPDATE wp SET k = lower(k)",
		}},
		{"a table whose integer key is its rowid", []string{"CREATE TABLE ip (id INTEGER PRIMARY KEY, v TEXT)"}, []string{
			"INSERT INTO ip (id, v) VALUES (5, 'a'), (2, 'b'), (NULL, 'c')",
			"UPDATE ip SET id = id + 10 WHERE v <> 'b'",
			"REPLACE INTO ip (id, v) VALUES (2, 'd')",
			"DELETE FROM ip WHERE id > 15",
		}},
		{"statements of a transaction", captureSchema, []string{
			"BEGIN",
			"INSERT INTO kv (k, v) VALUES ('t', '1')",
			"UPDATE kv SET v = '2' WHERE k = 't'",
			"SAVEPOINT s",
			"DELETE FROM kv WHERE k = 't'",
			"ROLLBACK TO s",
			"REPLACE INTO kv (k, v) VALUES ('t', '3')",
			"COMMIT",
		}},
	}
	// held are, by case, a read whose rows a corpus opened again from the files must answer as the database does.
	held := map[string]string{
		"a table without a rowid": "SELECT k, v FROM wr ORDER BY k",
		"a table without a rowid of a descending key under a collation": "SELECT a, b, hex(v) FROM wc ORDER BY a, b",
		"a key of a table without a rowid changed only in its case":     "SELECT k, v, (SELECT group_concat(p) FROM wq WHERE p = wp.k) FROM wp ORDER BY k",
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, root := open(t)
			run(t, s, "INSERT INTO notes (title) VALUES ('seed')")
			for _, text := range tc.setup {
				run(t, s, text)
			}
			n := compareDiffs(t)
			writes := 0
			for _, text := range tc.texts {
				run(t, s, text)
				if !strings.Contains("BEGIN COMMIT SAVEPOINT ROLLBACK", strings.Fields(text)[0]) {
					writes++
				}
			}
			if *n != writes {
				t.Fatalf("%d statements compared, want %d", *n, writes)
			}
			if text := held[tc.name]; text != "" {
				c, err := ops.Open(context.Background(), root, 0)
				corpustest.Opened(t, c, err)
				other, err := c.Session("again")
				if err != nil {
					t.Fatal(err)
				}
				defer closeSession(t, other)
				if got, want := fmt.Sprint(run(t, other, text).Rows), fmt.Sprint(run(t, s, text).Rows); got != want {
					t.Fatalf("the corpus read again holds %s, and the database %s", got, want)
				}
			}
		})
	}
}

// TestCaptureKeys pins the keys a statement gives the NULL keys of a key_prefix table from the rows it changed (spec 20
// Keys): past the largest number of the corpus, of the keys the statement wrote and of none it took away, as SQLite would
// give them over the keys the table then holds.
func TestCaptureKeys(t *testing.T) {
	cases := []struct {
		name  string
		setup []string
		text  string
		want  string
	}{
		{"past the largest key of the corpus", nil, "INSERT INTO notes (title, written) VALUES ('a', '2020-01-02'), ('b', '2020-01-02') RETURNING id", "[[N1001] [N1002]]"},
		{"past a key the statement wrote", nil, "INSERT INTO notes (id, title, written) VALUES ('N5000', 'a', '2020-01-02'), (NULL, 'b', '2020-01-02') RETURNING id", "[[N5000] [N5001]]"},
		{"past the largest key a statement did not take away", []string{
			"CREATE TRIGGER notes_take AFTER INSERT ON notes WHEN NEW.title = 'take' BEGIN DELETE FROM notes WHERE id = 'N1000'; END",
		}, "INSERT INTO notes (title, written) VALUES ('take', '2020-01-02') RETURNING id", "[[N1000]]"},
		{"past a key a statement moved", []string{
			"CREATE TRIGGER notes_move AFTER INSERT ON notes WHEN NEW.title = 'move' BEGIN UPDATE notes SET id = 'N7000' WHERE id = 'N1000'; END",
		}, "INSERT INTO notes (title, written) VALUES ('move', '2020-01-02') RETURNING id", "[[N7001]]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := openCost(t, 2000)
			for _, text := range tc.setup {
				run(t, s, text)
			}
			compareDiffs(t)
			if got := fmt.Sprint(run(t, s, tc.text).Rows); got != tc.want {
				t.Fatalf("%s returns %s, want %s", tc.text, got, tc.want)
			}
		})
	}
}

// captureTexts are the statements FuzzCapture forms, each with a value of the input in place of %s.
var captureTexts = []string{
	"INSERT INTO kv (k, v) VALUES (%s, %s)",
	"REPLACE INTO kv (k, v) VALUES (%s, %s)",
	"INSERT INTO kv (k, v) VALUES (%s, %s) ON CONFLICT (k) DO UPDATE SET v = excluded.v || kv.v",
	"UPDATE kv SET v = %s WHERE k >= %s",
	"DELETE FROM kv WHERE k < %s OR v = %s",
	"INSERT INTO plain (name) VALUES (%s), (%s)",
	"UPDATE plain SET name = %s WHERE name > %s",
	"DELETE FROM plain WHERE name = %s OR rowid %% 3 = length(%s) %% 3",
	"INSERT INTO notes (title) VALUES (substr(%s, 1, 32)), (substr(%s, 1, 32))",
	"UPDATE notes SET title = substr(%s, 1, 32), done = NOT done WHERE title > %s",
	"DELETE FROM notes WHERE title = %s OR id %% 4 = length(%s) %% 4",
	"INSERT INTO tags (note) SELECT id FROM notes WHERE title >= %s OR title < %s",
	"INSERT OR IGNORE INTO wk (k, n, v) VALUES (%s, 1, x'00'), (upper(%s), 2, NULL)",
	"REPLACE INTO wk (k, n, v) VALUES (%s, 2, x'01'), (%s, 3, x'02')",
	"UPDATE wk SET v = CAST(%s AS BLOB) WHERE k >= %s",
	"DELETE FROM wk WHERE k = %s OR n = length(%s)",
}

// FuzzCapture pins TestCapture's equality for any sequence of the statements of captureTexts: each byte pair of the
// input names a statement and the value it binds, a text of the input that may hold a NUL. Each input runs in a
// transaction of one corpus, rolled back after it, so every input starts from the same rows.
func FuzzCapture(f *testing.F) {
	f.Add([]byte{0, 'a', 1, 'a', 2, 'b', 3, 0, 4, 'b'})
	f.Add([]byte{5, 'x', 6, 'y', 7, 'z', 8, 'n', 9, 'm', 10, 'n', 11, 0})
	f.Add([]byte{0, 0, 1, 0, 3, 0, 2, 1, 4, 0, 8, 0, 11, 1, 10, 0})
	f.Add([]byte{12, 'a', 12, 'b', 13, 'a', 14, 'c', 15, 'b', 12, 0})
	s, _ := open(f)
	run(f, s, "INSERT INTO notes (title) VALUES ('seed'), ('a'), ('n')")
	for _, text := range captureSchema {
		run(f, s, text)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 24 {
			return
		}
		compareDiffs(t)
		run(t, s, "BEGIN")
		defer run(t, s, "ROLLBACK")
		for i := 0; i+1 < len(input); i += 2 {
			value := fmt.Sprintf("'%c' || char(%d) || '%c'", 'a'+input[i+1]%26, input[i+1]%3, 'a'+input[i+1]%7)
			text := fmt.Sprintf(captureTexts[int(input[i])%len(captureTexts)], value, value)
			// A statement SQLite or the corpus refuses changes nothing, and the next one goes on from the same rows; an
			// internal failure, such as an image the hook could not read, fails the input.
			if _, err := statement.Run(s, text, nil); err != nil {
				var f interface{ Failure() (code, msg string) }
				if !errors.As(err, &f) {
					t.Fatalf("%s: %v", text, err)
				}
				if code, _ := f.Failure(); code == "INTERNAL" {
					t.Fatalf("%s: %v", text, err)
				}
			}
		}
	})
}
