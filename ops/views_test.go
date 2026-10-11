package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// viewsDBS is an application schema document flowmark with a view table over tasks.
const viewsDBS = `dbspec 1 flowmark

table tasks {
  id varchar(16)
  title varchar(64)
  body text
  rank i32
  primary key (id)
  settings {
    store document table
    title title
    body body
  }
}

table views {
  id i64 identity
  title varchar(255)
  kind varchar(16)
  entity varchar(63)
  filter text
  sort text
  group_by varchar(63) null
  primary key (id)
  check ck_views_kind (kind in ('kanban', 'list', 'gantt'))
  settings {
    store document table
    title title
    codec filter ordered_json
    codec sort ordered_json
  }
}
`

// viewsCorpus writes the views catalog with three tasks and three views — high ranks first, every task in key order,
// and one whose filter names a column tasks lacks — and opens it.
func viewsCorpus(t *testing.T, dbs string) *Corpus {
	t.Helper()
	root := t.TempDir()
	docs := map[string]string{
		"dbspec/flowmark.dbs": dbs,
		"tasks.flowmark.md":   "# tasks\n| id | title | rank |\n| --- | --- | --- |\n| A | Alpha | 1 |\n| B | Beta | 3 |\n| C | Gamma | 2 |\n",
		"views.flowmark.md": "# views\n| id | title | kind | entity | filter | sort | group_by |\n| --- | --- | --- | --- | --- | --- | --- |\n" +
			`| 1 | High | list | tasks | {"where":{"items":[{"pred":{"column":"rank","op":"gt","p":0}}]},"params":[1]} | [{"column":"rank","desc":true}] |  |` + "\n" +
			`| 2 | All | kanban | tasks | {"where":{"items":[]},"params":[]} | [] | rank |` + "\n" +
			`| 3 | Broken | list | tasks | {"where":{"items":[{"pred":{"column":"size","op":"gt","p":0}}]},"params":[1]} | [] |  |` + "\n",
	}
	for p, text := range docs {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Open(context.Background(), root, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return c
}

// TestViews pins the views operation (spec 40 The faces, spec 100 Views): the view rows, and a view's rows through its
// stored filter and sort.
func TestViews(t *testing.T) {
	t.Run("40#the-faces: views without a key answers the rows of the view table", func(t *testing.T) {
		c := viewsCorpus(t, viewsDBS)
		got := ok(t, c, "views", `{}`)
		if !strings.HasPrefix(got, `{"rows":[{"id":1,"title":"High","kind":"list","entity":"tasks"`) || strings.Count(got, `"kind"`) != 3 {
			t.Fatalf("views = %s", got)
		}
	})
	t.Run("40#the-faces: views with a key answers the view and its rows through its filter and sort", func(t *testing.T) {
		c := viewsCorpus(t, viewsDBS)
		got := ok(t, c, "views", `{"key":1}`)
		want := `{"view":{"id":1,"title":"High","kind":"list","entity":"tasks","filter":"{\"where\":{\"items\":[{\"pred\":{\"column\":\"rank\",\"op\":\"gt\",\"p\":0}}]},\"params\":[1]}","sort":"[{\"column\":\"rank\",\"desc\":true}]","group_by":null},` +
			`"rows":[{"id":"B","title":"Beta","body":"","rank":3},{"id":"C","title":"Gamma","body":"","rank":2}]}`
		if got != want {
			t.Fatalf("views 1 = %s\nwant %s", got, want)
		}
		if got := ok(t, c, "views", `{"key":2}`); !strings.HasSuffix(got, `"rows":[{"id":"A","title":"Alpha","body":"","rank":1},{"id":"B","title":"Beta","body":"","rank":3},{"id":"C","title":"Gamma","body":"","rank":2}]}`) {
			t.Fatalf("views 2 = %s", got)
		}
	})
	t.Run("40#the-faces: an invalid view is refused, a missing one is NO_ROWS", func(t *testing.T) {
		c := viewsCorpus(t, viewsDBS)
		fails(t, c, "views", `{"key":3}`, "CONSTRAINT", "view", "3", "filter")
		fails(t, c, "views", `{"key":9}`, "NO_ROWS", "views", "9")
		fails(t, c, "views", `{"table":"tasks"}`, "CONFIG", "table")
	})
	t.Run("40#the-faces: a catalog without a view table refuses views", func(t *testing.T) {
		c := viewsCorpus(t, strings.Replace(viewsDBS, "dbspec 1 flowmark", "dbspec 1 board", 1))
		fails(t, c, "views", `{}`, "CONFIG", "view table")
	})
}
