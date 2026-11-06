package statement_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/ops"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

const temporalSet = `dbspec 1 clock

table moments {
  id i64 identity
  day date null
  at0 datetime(0) null
  at3 datetime(3) null
  at6 datetime(6) null
  hour time(2) null
  primary key (id)
}
`

// openTemporal opens a new session of a corpus whose table holds a column of each temporal type.
func openTemporal(t *testing.T, root string) *writepath.Session {
	t.Helper()
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	s, err := c.Session("temporal-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, s) })
	return s
}

// TestTemporalText pins that a statement reads the text of a date, datetime or time column as SQLite holds it, as a
// SQLite connection does (spec 40, SQL statements): the text a statement wrote, in the session and in a session of a
// new opening that reads the corpus's files, and the text of a column a statement declared DATE, DATETIME or
// TIMESTAMP, which no time parse rewrites.
func TestTemporalText(t *testing.T) {
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["clock.dbs"]}`,
		"dbspec/clock.dbs":       temporalSet,
	})
	s := openTemporal(t, root)
	declared := []struct{ name, column, text string }{
		{"a date", "day", "2026-10-22"},
		{"a datetime of no fraction", "at0", "2020-01-02 03:04:05"},
		{"a datetime of three digits", "at3", "2020-01-02 03:04:05.500"},
		{"a datetime of six digits", "at6", "2020-01-02 03:04:05.123456"},
		{"a time of two digits", "hour", "03:04:05.25"},
	}
	for i, v := range declared {
		run(t, s, fmt.Sprintf("INSERT INTO moments (id, %s) VALUES (%d, ?)", v.column, i+1), fmt.Sprintf("%q", v.text))
	}
	fresh := openTemporal(t, root)
	for i, v := range declared {
		want := fmt.Sprintf(`[[%q]]`, v.text)
		for _, session := range []struct {
			name string
			s    *writepath.Session
		}{{"the session", s}, {"a new opening", fresh}} {
			if got := cells(run(t, session.s, fmt.Sprintf("SELECT %s FROM moments WHERE id = %d", v.column, i+1))); got != want {
				t.Errorf("%s in %s: read %s, want %s", v.name, session.name, got, want)
			}
		}
	}
	run(t, s, "CREATE TABLE stamps (id INTEGER PRIMARY KEY, d DATE, dt DATETIME, ts TIMESTAMP)")
	undeclared := []struct{ name, insert, want string }{
		{"a date", "'2026-10-22', NULL, NULL", `[["2026-10-22",null,null]]`},
		{"a datetime of one digit", "NULL, '2020-01-02 03:04:05.5', NULL", `[[null,"2020-01-02 03:04:05.5",null]]`},
		{"a timestamp of RFC 3339", "NULL, NULL, '2020-01-02T03:04:05Z'", `[[null,null,"2020-01-02T03:04:05Z"]]`},
		{"a text that is no time", "'soon', 'later', 'never'", `[["soon","later","never"]]`},
	}
	for i, v := range undeclared {
		run(t, s, fmt.Sprintf("INSERT INTO stamps (id, d, dt, ts) VALUES (%d, %s)", i+1, v.insert))
		if got := cells(run(t, s, fmt.Sprintf("SELECT d, dt, ts FROM stamps WHERE id = %d", i+1))); got != v.want {
			t.Errorf("%s: read %s, want %s", v.name, got, v.want)
		}
	}
}
