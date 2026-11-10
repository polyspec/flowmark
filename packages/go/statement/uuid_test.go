package statement_test

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/ops"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

const uuidSet = `dbspec 1 board

table keys {
  id i64 identity
  u uuid
  v uuid null
  primary key (id)
}
`

// openUUID opens a new corpus whose one table has a uuid column and a nullable one.
func openUUID(t *testing.T) (*writepath.Session, string) {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["board.dbs"]}`,
		"dbspec/board.dbs":       uuidSet,
	})
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	s, err := c.Session("statement-uuid")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, s) })
	return s, root
}

// uuidBytes is the parameter of the 16 bytes of the canonical text u, as the client binds a uuid (orm DIA-27).
func uuidBytes(t *testing.T, u string) string {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(u, "-", ""))
	if err != nil || len(b) != 16 {
		t.Fatalf("%s is no uuid: %v", u, err)
	}
	return `{"bytes":"` + base64.StdEncoding.EncodeToString(b) + `"}`
}

// TestUUIDColumn pins a uuid column of the set (orm DIA-27): SQL holds its value as the 16 bytes of the canonical text,
// in the order of the text, and the document holds the canonical text; a corpus read again from its files answers the
// same 16 bytes. A value that is not 16 bytes is refused by the column's CHECK.
func TestUUIDColumn(t *testing.T) {
	const a, b = "0e2b5e8c-3b0f-4c59-9a0f-6a2f2c2e1b10", "ff000000-0000-4000-8000-000000000001"
	s, root := openUUID(t)
	run(t, s, "INSERT INTO keys (u, v) VALUES (?, NULL)", uuidBytes(t, b))
	run(t, s, "INSERT INTO keys (u, v) VALUES (?, ?)", uuidBytes(t, a), uuidBytes(t, b))
	read := func(s *writepath.Session) string {
		return cells(run(t, s, "SELECT u, typeof(u), v, typeof(v) FROM keys ORDER BY u"))
	}
	want := `[[` + uuidBytes(t, a) + `,"blob",` + uuidBytes(t, b) + `,"blob"],[` + uuidBytes(t, b) + `,"blob",null,"null"]]`
	if got := read(s); got != want {
		t.Errorf("the session read %s, want %s", got, want)
	}
	doc := file(t, root, "keys.flowmark.md")
	for _, u := range []string{a, b} {
		if !strings.Contains(doc, "| "+u+" ") {
			t.Errorf("the document does not hold the canonical text %s:\n%s", u, doc)
		}
	}
	if strings.Contains(doc, "x'") {
		t.Errorf("the document holds a uuid as a stored literal:\n%s", doc)
	}
	fresh := freshSession(t, root)
	if got := read(fresh); got != want {
		t.Errorf("read %s from the files, want %s", got, want)
	}
	if got := cells(run(t, fresh, "SELECT id FROM keys WHERE u = ?", uuidBytes(t, a))); got != `[[2]]` {
		t.Errorf("the row of %s by its bytes: %s, want [[2]]", a, got)
	}
	for name, value := range map[string]string{
		"the canonical text": "'" + a + "'",
		"15 bytes":           "X'0e2b5e8c3b0f4c599a0f6a2f2c2e1b'",
		"17 bytes":           "X'0e2b5e8c3b0f4c599a0f6a2f2c2e1b1000'",
		"an integer":         "1",
	} {
		t.Run(name, func(t *testing.T) {
			refused(t, fresh, "INSERT INTO keys (u) VALUES ("+value+")", "CONSTRAINT")
		})
	}
}
