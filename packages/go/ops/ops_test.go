package ops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/writepath"
)

// boardCorpus writes the board catalog with two people and no item, and opens it.
func boardCorpus(t *testing.T) *Corpus {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/board.dbs":   corpustest.BoardDBS,
		"people.flowmark.md": "# people\n| id | name | age |\n| --- | --- | --- |\n| P1 | Ada | 36 |\n| P2 | Max |  |\n",
	})
	return openCorpus(t, root)
}

// openCorpus opens the corpus at root, closed when the test ends.
func openCorpus(t *testing.T, root string) *Corpus {
	t.Helper()
	c, err := Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	return c
}

// run parses the members, given as one JSON object, of operation op and runs it; it returns the answer as JSON.
func run(t *testing.T, c *Corpus, op, members string) (string, error) {
	t.Helper()
	b, err := c.RunText(context.Background(), "test", op, members)
	return string(b), err
}

// ok runs an operation that must succeed and returns its answer.
func ok(t *testing.T, c *Corpus, op, members string) string {
	t.Helper()
	a, err := run(t, c, op, members)
	if err != nil {
		t.Fatalf("%s %s: %v", op, members, err)
	}
	return a
}

// fails runs an operation that must fail with the face code code, its message naming every text of has.
func fails(t *testing.T, c *Corpus, op, members, code string, has ...string) {
	t.Helper()
	a, err := run(t, c, op, members)
	if err == nil {
		t.Fatalf("%s %s answered %s; want %s", op, members, a, code)
	}
	got, msg := Failure(err)
	if got != code {
		t.Fatalf("%s %s failed with %s: %s; want %s", op, members, got, msg, code)
	}
	for _, h := range has {
		if !strings.Contains(msg, h) {
			t.Fatalf("%s %s: the message %q does not name %q", op, members, msg, h)
		}
	}
}

// TestOperations pins the operations of the faces over any table (spec 40 The faces).
func TestOperations(t *testing.T) {
	t.Run("40#the-faces: schema answers the catalog", func(t *testing.T) {
		c := boardCorpus(t)
		var s struct {
			ManifestHash string `json:"manifest_hash"`
			Tables       []struct {
				Name    string `json:"name"`
				Key     string `json:"key"`
				Order   string `json:"order"`
				Parent  string `json:"parent"`
				Title   string `json:"title"`
				Storage struct{ Kind, Shape, Parent string }
				Columns []struct{ Name, Type string }
				Machine *struct {
					Column      string            `json:"column"`
					States      []string          `json:"states"`
					Checkboxes  map[string]string `json:"checkboxes"`
					Transitions []struct {
						From, To string
						Require  []string
					} `json:"transitions"`
				} `json:"state_machine"`
			} `json:"tables"`
		}
		if err := json.Unmarshal([]byte(ok(t, c, "schema", `{}`)), &s); err != nil {
			t.Fatal(err)
		}
		if s.ManifestHash == "" || len(s.Tables) != 3 {
			t.Fatalf("schema: %+v", s)
		}
		items := s.Tables[0]
		if items.Name != "items" || items.Key != "id" || items.Order != "position" || items.Parent != "parent" || items.Title != "title" {
			t.Fatalf("items: %+v", items)
		}
		if items.Storage.Kind != "document" || items.Storage.Shape != "list" || items.Columns[4].Type != "text" {
			t.Fatalf("items storage and columns: %+v", items)
		}
		if m := items.Machine; m == nil || m.Column != "state" || m.Checkboxes["doing"] != "~" || m.Transitions[1].Require[0] != "evidence" {
			t.Fatalf("items state machine: %+v", m)
		}
		if h := s.Tables[1]; h.Storage.Kind != "block" || h.Storage.Parent != "items" {
			t.Fatalf("item_history storage: %+v", h)
		}
		if s.Tables[2].Machine != nil || s.Tables[2].Parent != "" {
			t.Fatalf("people: %+v", s.Tables[2])
		}
	})
	t.Run("40#the-faces: rows and row.get answer the rows of any table", func(t *testing.T) {
		c := boardCorpus(t)
		if got, want := ok(t, c, "rows", `{"table":"people"}`), `{"rows":[{"id":"P1","name":"Ada","age":36},{"id":"P2","name":"Max","age":null}]}`; got != want {
			t.Fatalf("rows = %s, want %s", got, want)
		}
		if got, want := ok(t, c, "row.get", `{"table":"people","key":"P2"}`), `{"row":{"id":"P2","name":"Max","age":null}}`; got != want {
			t.Fatalf("row.get = %s, want %s", got, want)
		}
		fails(t, c, "row.get", `{"table":"people","key":"P9"}`, "NO_ROWS", "people", "P9")
	})
	t.Run("40#the-faces: the writes answer their keys and the table's rows", func(t *testing.T) {
		c := boardCorpus(t)
		a := ok(t, c, "row.insert", `{"table":"items","values":{"title":"First","body":"","owner":"P1"}}`)
		if want := `{"affected":1,"keys":["T1"],"rows":[{"id":"T1","parent":null,"state":"waiting","title":"First","body":"","position":1,"owner":"P1","evidence":null}]}`; a != want {
			t.Fatalf("row.insert = %s, want %s", a, want)
		}
		ok(t, c, "row.insert", `{"table":"items","values":{"title":"Second","body":""}}`)
		a = ok(t, c, "row.move", `{"table":"items","key":"T2","position":1}`)
		if !strings.Contains(a, `"rows":[{"id":"T2"`) || !strings.Contains(a, `"title":"First","body":"","position":2`) {
			t.Fatalf("row.move = %s", a)
		}
		a = ok(t, c, "row.move", `{"table":"items","key":"T1","position":1,"parent":"T2"}`)
		if !strings.Contains(a, `{"id":"T1","parent":"T2"`) {
			t.Fatalf("row.move with parent = %s", a)
		}
		ok(t, c, "row.update", `{"table":"items","key":"T1","values":{"title":"Renamed"}}`)
		ok(t, c, "transition", `{"table":"items","key":"T1","state":"doing"}`)
		fails(t, c, "transition", `{"table":"items","key":"T1","state":"done"}`, "CONSTRAINT", "evidence")
		a = ok(t, c, "transition", `{"table":"items","key":"T1","state":"done","values":{"evidence":"Tests pass."}}`)
		if !strings.Contains(a, `"state":"done","title":"Renamed"`) {
			t.Fatalf("transition = %s", a)
		}
		fails(t, c, "row.delete", `{"table":"items","key":"T2"}`, "FOREIGN_KEY")
		fails(t, c, "row.update", `{"table":"people","key":"P9","values":{"name":"X"}}`, "NO_ROWS", "people", "P9")
		if got, want := ok(t, c, "row.delete", `{"table":"people","key":"P2"}`), `{"affected":1,"keys":[],"rows":[{"id":"P1","name":"Ada","age":36}]}`; got != want {
			t.Fatalf("row.delete = %s, want %s", got, want)
		}
	})
	t.Run("40#the-faces: an operation the face cannot form is CONFIG", func(t *testing.T) {
		c := boardCorpus(t)
		fails(t, c, "row.rename", `{}`, "CONFIG", "row.rename")
		fails(t, c, "rows", `{}`, "CONFIG", "table")
		fails(t, c, "row.get", `{"table":"people"}`, "CONFIG", "key")
		fails(t, c, "rows", `{"table":"people","key":"P1"}`, "CONFIG", "key")
		fails(t, c, "row.insert", `{"table":"people","values":["P3"]}`, "CONFIG", "values")
		fails(t, c, "row.move", `{"table":"people","key":"P1","position":1}`, "CONFIG", "people", "order")
		fails(t, c, "row.move", `{"table":"item_history","key":1,"position":1,"parent":null}`, "CONFIG", "item_history")
		fails(t, c, "transition", `{"table":"people","key":"P1","state":"done"}`, "CONFIG", "people", "state machine")
		fails(t, c, "transition", `{"table":"items","key":"T1","state":7}`, "CONFIG", "state")
		fails(t, c, "transition", `{"table":"items","key":"T1","state":"doing","values":{"state":"done"}}`, "CONFIG", "state column")
	})
	t.Run("40#the-faces: a table or column the catalog does not declare is the request's code", func(t *testing.T) {
		c := boardCorpus(t)
		fails(t, c, "rows", `{"table":"posts"}`, "ENTITY_UNKNOWN", "posts")
		fails(t, c, "row.insert", `{"table":"people","values":{"id":"P3","nick":"x"}}`, "COLUMN_UNKNOWN", "nick")
	})
}

// TestFailure pins the failure answer: the face name of the failure vocabulary, else the orm code (spec 40 The faces).
func TestFailure(t *testing.T) {
	for _, c := range []struct {
		err  error
		code string
	}{
		{&writepath.Error{Code: "CANCELED", Face: "LOCK_TIMEOUT", Msg: "held"}, "LOCK_TIMEOUT"},
		{&writepath.Error{Code: "WRITE_CONFLICT", Face: "CORPUS_CONFLICT", Msg: "changed"}, "CORPUS_CONFLICT"},
		{&Error{Code: "CONFIG", Msg: "x"}, "CONFIG"},
		{errors.New("plain"), "INTERNAL"},
	} {
		if got, _ := Failure(c.err); got != c.code {
			t.Errorf("Failure(%v) = %s, want %s", c.err, got, c.code)
		}
	}
}

// TestTextValue pins the CLI's text values: the wire value of the column's type (spec 40 The faces).
func TestTextValue(t *testing.T) {
	c := boardCorpus(t)
	for _, v := range []struct{ table, column, text, want string }{
		{"people", "age", "36", `36`},
		{"people", "age", "x", `"x"`},
		{"people", "name", "36", `"36"`},
		{"items", "position", "2", `2`},
		{"people", "nick", "x", `"x"`},
		{"posts", "id", "x", `"x"`},
	} {
		if got := string(c.TextValue(v.table, v.column, v.text)); got != v.want {
			t.Errorf("TextValue(%s.%s, %q) = %s, want %s", v.table, v.column, v.text, got, v.want)
		}
	}
}
