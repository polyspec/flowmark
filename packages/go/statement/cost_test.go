package statement_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/ops"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

// costDBS is the catalog of the cost cases (plan, Ruling: an operation costs what it touches): tasks is an order table
// with a key_prefix key, whose sibling groups are the block lists of the projects, of costGroup rows each; notes is a
// document table with a key_prefix key and no order, sharded by day into documents of costGroup rows.
const costDBS = `dbspec 1 cost

table projects {
  id varchar(16)
  name varchar(64)
  primary key (id)
  settings {
    key_prefix 'P'
    store files
    title name
  }
}

table tasks {
  id varchar(16)
  project varchar(16)
  title varchar(64)
  position i32
  primary key (id)
  index ix_tasks_project (project)
  foreign key fk_tasks_project (project) references projects (id) on delete restrict on update restrict
  settings {
    key_prefix 'T'
    store block fk_tasks_project list
    title title
    order position
  }
}

table notes {
  id varchar(16)
  title varchar(64)
  written date
  primary key (id)
  settings {
    key_prefix 'N'
    store document table shard written day
    title title
  }
}
`

// costGroup is the size of a sibling group of tasks and the rows of a notes document, the same at every size of the
// corpus.
const costGroup = 100

// costSizes are the rows of the corpora the cost cases measure, half of them tasks and half notes.
var costSizes = []int{1000, 100000}

// openCost writes a corpus of costDBS of rows rows, half of them tasks in sibling groups of costGroup under as many
// projects and half notes in documents of costGroup rows, and opens a session on it. The files are rendered by the store,
// as a commit writes them, so the corpus is canonical whatever its size.
func openCost(t *testing.T, rows int) (*writepath.Session, string) {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["cost.dbs"]}`,
		"dbspec/cost.dbs":        costDBS,
	})
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("catalog: %+v", diags)
	}
	write := func(rel string, data []byte, refusals []store.Refusal, err error) {
		t.Helper()
		if err != nil || len(refusals) > 0 {
			t.Fatalf("render %s: %v %+v", rel, err, refusals)
		}
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	groups := rows / 2 / costGroup
	for g := range groups {
		project := &store.Row{Table: "projects", Key: fmt.Sprintf("P%d", g+1), Title: fmt.Sprintf("Project %d", g+1), Blocks: map[string][]*store.Row{}}
		for i := range costGroup {
			n := g*costGroup + i + 1
			position := fmt.Sprint(i + 1)
			project.Blocks["tasks"] = append(project.Blocks["tasks"], &store.Row{Table: "tasks", Key: fmt.Sprintf("T%d", n), Title: fmt.Sprintf("Task %d", n), Scalars: map[string]*string{"position": &position}})
		}
		rel := "projects/" + project.Key + ".flowmark.md"
		out, refusals, err := store.RenderFile(cat, "projects", rel, project, nil)
		write(rel, out, refusals, err)
	}
	day := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for g := range groups {
		written := day.AddDate(0, 0, g).Format(time.DateOnly)
		notes := make([]*store.Row, costGroup)
		for i := range notes {
			n := g*costGroup + i + 1
			notes[i] = &store.Row{Table: "notes", Key: fmt.Sprintf("N%d", n), Title: fmt.Sprintf("Note %d", n), Scalars: map[string]*string{"written": &written}}
		}
		rel := "notes/" + written + ".flowmark.md"
		out, refusals, err := store.RenderDocument(cat, "notes", rel, notes, nil)
		write(rel, out, refusals, err)
	}
	c, err := ops.Open(context.Background(), root, 0)
	corpustest.Opened(t, c, err)
	s, err := c.Session("statement-cost")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(t, s) })
	return s, root
}

// costOp is one operation the cost cases measure.
type costOp struct {
	name string
	// do runs the operation on the session of a corpus at root; i tells the repetitions apart.
	do func(t *testing.T, s *writepath.Session, root string)
}

// costOps are the point operations of the cost cases: a read, an insert, an update and a delete on each table, and an
// external edit of one file, answered by the read after it.
var costOps = []costOp{
	{"read task", func(t *testing.T, s *writepath.Session, _ string) { run(t, s, "SELECT title FROM tasks WHERE id = 'T7'") }},
	{"read note", func(t *testing.T, s *writepath.Session, _ string) { run(t, s, "SELECT title FROM notes WHERE id = 'N7'") }},
	{"insert task", func(t *testing.T, s *writepath.Session, _ string) {
		run(t, s, "INSERT INTO tasks (project, title) VALUES ('P2', 'added')")
	}},
	{"insert note", func(t *testing.T, s *writepath.Session, _ string) {
		run(t, s, "INSERT INTO notes (title, written) VALUES ('added', '2020-01-02')")
	}},
	{"update task", func(t *testing.T, s *writepath.Session, _ string) {
		run(t, s, "UPDATE tasks SET title = 'renamed' WHERE id = 'T150'")
	}},
	{"update note", func(t *testing.T, s *writepath.Session, _ string) {
		run(t, s, "UPDATE notes SET title = 'renamed' WHERE id = 'N150'")
	}},
	{"delete task", func(t *testing.T, s *writepath.Session, _ string) { run(t, s, "DELETE FROM tasks WHERE id = 'T151'") }},
	{"delete note", func(t *testing.T, s *writepath.Session, _ string) { run(t, s, "DELETE FROM notes WHERE id = 'N151'") }},
	{"external edit", func(t *testing.T, s *writepath.Session, root string) {
		p := filepath.Join(root, "notes", "2020-01-03.flowmark.md")
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		edited := strings.Replace(string(b), "Note 201", "Edited 201", 1)
		if edited == string(b) {
			t.Fatalf("%s holds no Note 201", p)
		}
		if err := os.WriteFile(p, []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := cells(run(t, s, "SELECT title FROM notes WHERE id = 'N201'")); got != `[["Edited 201"]]` {
			t.Fatalf("the read after the edit answers %s", got)
		}
	}},
}

// measure runs each operation of costOps on a new corpus of rows rows and returns the counts of each.
func measure(t *testing.T, rows int) map[string]cost.Snapshot {
	t.Helper()
	s, root := openCost(t, rows)
	out := map[string]cost.Snapshot{}
	for _, op := range costOps {
		before := s.Cost().Snapshot()
		op.do(t, s, root)
		out[op.name] = s.Cost().Snapshot().Since(before)
	}
	return out
}

// equalCost names, for each operation, the counters whose count is equal at every size of costSizes. A row of the plan
// that makes an operation cost what it touches adds its counters here, red before the row.
var equalCost = map[string][]cost.Counter{}

// TestCost measures what each point operation touches at each size of costSizes (plan, Ruling: an operation costs what
// it touches): the counts are the engine's own, so two corpora of the same rows count the same, every operation reads
// the corpus it answers from, and each count equalCost names is equal at every size.
func TestCost(t *testing.T) {
	t.Run("two corpora of the same rows count the same", func(t *testing.T) {
		first, second := measure(t, costSizes[0]), measure(t, costSizes[0])
		for _, op := range costOps {
			if first[op.name] != second[op.name] {
				t.Errorf("%s counts %v, then %v", op.name, first[op.name], second[op.name])
			}
		}
	})
	counts := map[int]map[string]cost.Snapshot{}
	for _, rows := range costSizes {
		counts[rows] = measure(t, rows)
	}
	for _, op := range costOps {
		for _, rows := range costSizes {
			c := counts[rows][op.name]
			t.Logf("%s at %d rows: %s", op.name, rows, describe(c))
			if c == (cost.Snapshot{}) {
				t.Errorf("%s at %d rows counts nothing", op.name, rows)
			}
		}
		for _, k := range equalCost[op.name] {
			small, large := counts[costSizes[0]][op.name].Get(k), counts[costSizes[len(costSizes)-1]][op.name].Get(k)
			if small != large {
				t.Errorf("%s counts %d %s at %d rows and %d at %d rows", op.name, small, k, costSizes[0], large, costSizes[len(costSizes)-1])
			}
		}
	}
}

// describe writes the counts of c that are not zero.
func describe(c cost.Snapshot) string {
	var b strings.Builder
	for _, k := range cost.Counters() {
		if n := c.Get(k); n != 0 {
			fmt.Fprintf(&b, " %s=%d", k, n)
		}
	}
	return strings.TrimSpace(b.String())
}
