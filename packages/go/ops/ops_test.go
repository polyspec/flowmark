package ops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/statement"
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
				Name     string `json:"name"`
				Key      string `json:"key"`
				Order    string `json:"order"`
				Parent   string `json:"parent"`
				Title    string `json:"title"`
				Storage  struct{ Kind, Shape, Parent string }
				Columns  []struct{ Name, Type string }
				Checkbox *struct {
					Column string            `json:"column"`
					Glyphs map[string]string `json:"glyphs"`
				} `json:"checkbox"`
			} `json:"tables"`
		}
		answer := ok(t, c, "schema", `{}`)
		if err := json.Unmarshal([]byte(answer), &s); err != nil {
			t.Fatal(err)
		}
		// The engine holds no state machine (spec 50 State columns), so the answer names none.
		if strings.Contains(answer, "state_machine") || strings.Contains(answer, "transitions") {
			t.Fatalf("schema answers a state machine: %s", answer)
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
		if b := items.Checkbox; b == nil || b.Column != "state" || b.Glyphs["doing"] != "~" || len(b.Glyphs) != 3 {
			t.Fatalf("items checkbox: %+v", b)
		}
		if h := s.Tables[1]; h.Storage.Kind != "block" || h.Storage.Parent != "items" {
			t.Fatalf("item_history storage: %+v", h)
		}
		if s.Tables[2].Checkbox != nil || s.Tables[2].Parent != "" {
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
		a := ok(t, c, "row.insert", `{"table":"items","values":{"id":"T1","title":"First","body":"","position":1,"owner":"P1"}}`)
		if want := `{"affected":1,"keys":["T1"],"rows":[{"id":"T1","parent":null,"state":"waiting","title":"First","body":"","position":1,"owner":"P1","evidence":null}]}`; a != want {
			t.Fatalf("row.insert = %s, want %s", a, want)
		}
		ok(t, c, "row.insert", `{"table":"items","values":{"id":"T2","title":"Second","body":"","position":2}}`)
		// A move writes the value it is given and moves no sibling (spec 20 Order).
		a = ok(t, c, "row.move", `{"table":"items","key":"T2","position":0}`)
		if !strings.Contains(a, `"rows":[{"id":"T2"`) || !strings.Contains(a, `"title":"Second","body":"","position":0`) || !strings.Contains(a, `"title":"First","body":"","position":1`) {
			t.Fatalf("row.move = %s", a)
		}
		a = ok(t, c, "row.move", `{"table":"items","key":"T1","position":1,"parent":"T2"}`)
		if !strings.Contains(a, `{"id":"T1","parent":"T2"`) {
			t.Fatalf("row.move with parent = %s", a)
		}
		ok(t, c, "row.update", `{"table":"items","key":"T1","values":{"title":"Renamed"}}`)
		// A state is a value: an update the declared machine would refuse, without the column its transition
		// requires, is accepted and writes no history row (spec 50 State columns).
		a = ok(t, c, "row.update", `{"table":"items","key":"T1","values":{"state":"done"}}`)
		if !strings.Contains(a, `"state":"done","title":"Renamed"`) {
			t.Fatalf("row.update of the state = %s", a)
		}
		if got := ok(t, c, "rows", `{"table":"item_history"}`); got != `{"rows":[]}` {
			t.Fatalf("item_history = %s, want no row", got)
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
		fails(t, c, "transition", `{"table":"items","key":"T1","state":"doing"}`, "CONFIG", "transition", "unknown")
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

// TestCatalogOfSchemaStatements pins that a face reads the catalog a schema statement of SQL left, in the process that
// ran it and in another process that holds the corpus open (spec 40 SQL statements): the table it created answers rows
// and stands in the schema, while an orm request, whose manifest names the set's tables, does not reach it.
func TestCatalogOfSchemaStatements(t *testing.T) {
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{"dbspec/board.dbs": corpustest.BoardDBS})
	c, other := openCorpus(t, root), openCorpus(t, root)
	ok(t, other, "rows", `{"table": "people"}`)
	s, err := c.Session("sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"CREATE TABLE made (id INTEGER PRIMARY KEY, title TEXT)", "INSERT INTO made (title) VALUES ('a')"} {
		if _, err := statement.Run(s, text, nil); err != nil {
			t.Fatalf("%s: %v", text, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for name, corpus := range map[string]*Corpus{"the process of the statement": c, "another process": other} {
		t.Run(name, func(t *testing.T) {
			if got := ok(t, corpus, "rows", `{"table": "made"}`); !strings.Contains(got, `"a"`) {
				t.Fatalf("the rows of the made table: %s", got)
			}
			if got := ok(t, corpus, "schema", `{}`); !strings.Contains(got, `"made"`) {
				t.Fatalf("the schema holds no made table: %s", got)
			}
			// An orm request reads the tables of the set its manifest names, which SQL did not add to.
			s, err := corpus.Session("orm")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := s.Close(); err != nil {
					t.Error(err)
				}
			}()
			_, err = s.Exec(context.Background(), corpustest.Message(t, corpus.Catalog().ManifestHash, corpustest.TestNow, "all", "made", nil))
			if code, _ := Failure(err); code != "ENTITY_UNKNOWN" {
				t.Fatalf("an orm request of the made table: %v, want ENTITY_UNKNOWN", err)
			}
		})
	}
}
