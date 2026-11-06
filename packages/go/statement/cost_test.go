package statement_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/ops"
	"github.com/polyspec/flowmark/packages/go/statement"
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
var costSizes = []int{1000, 10000, 100000}

// openCost writes a corpus of costDBS of rows rows, half of them tasks in sibling groups of costGroup under as many
// projects and half notes in documents of costGroup rows, and opens a session on it. The files are rendered by the store,
// as a commit writes them, so the corpus is canonical whatever its size.
func openCost(t testing.TB, rows int) (*writepath.Session, string) {
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
	// do runs the operation on the session of a corpus at root; i, from 0, tells the repetitions of a benchmark apart.
	do func(t testing.TB, s *writepath.Session, root string, i int)
}

// costOps are the point operations of the cost cases: a read, an insert, an update and a delete on each table, and an
// external edit of one file, answered by the read after it. The reads come first, on the corpus the warming statement
// read, so nothing changed before them.
var costOps = []costOp{
	{"read task", func(t testing.TB, s *writepath.Session, _ string, _ int) {
		run(t, s, "SELECT title FROM tasks WHERE id = 'T7'")
	}},
	{"read note", func(t testing.TB, s *writepath.Session, _ string, _ int) {
		run(t, s, "SELECT title FROM notes WHERE id = 'N7'")
	}},
	{"insert task", func(t testing.TB, s *writepath.Session, _ string, _ int) {
		run(t, s, "INSERT INTO tasks (project, title) VALUES ('P2', 'added')")
	}},
	{"insert note", func(t testing.TB, s *writepath.Session, _ string, _ int) {
		run(t, s, "INSERT INTO notes (title, written) VALUES ('added', '2020-01-02')")
	}},
	{"update task", func(t testing.TB, s *writepath.Session, _ string, i int) {
		run(t, s, fmt.Sprintf("UPDATE tasks SET title = 'renamed %d' WHERE id = 'T150'", i))
	}},
	{"update note", func(t testing.TB, s *writepath.Session, _ string, i int) {
		run(t, s, fmt.Sprintf("UPDATE notes SET title = 'renamed %d' WHERE id = 'N150'", i))
	}},
	{"delete task", func(t testing.TB, s *writepath.Session, _ string, i int) {
		run(t, s, fmt.Sprintf("DELETE FROM tasks WHERE id = 'T%d'", 151+i))
	}},
	{"delete note", func(t testing.TB, s *writepath.Session, _ string, i int) {
		run(t, s, fmt.Sprintf("DELETE FROM notes WHERE id = 'N%d'", 151+i))
	}},
	{"face read note", func(t testing.TB, s *writepath.Session, _ string, _ int) {
		face(t, s, "one", "notes", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"title"}}, "where": corpustest.WhereID(0)}, "N7")
	}},
	{"face insert note", func(t testing.TB, s *writepath.Session, _ string, i int) {
		face(t, s, "insert", "notes", map[string]any{"set": []any{map[string]any{"column": "title", "p": 0}, map[string]any{"column": "written", "p": 1}}},
			fmt.Sprintf("face %d", i), "2020-01-02")
	}},
	{"face update note", func(t testing.TB, s *writepath.Session, _ string, i int) {
		face(t, s, "update", "notes", map[string]any{"set": []any{map[string]any{"column": "title", "p": 0}}, "where": corpustest.WhereID(1)},
			fmt.Sprintf("face renamed %d", i), "N160")
	}},
	{"face delete note", func(t testing.TB, s *writepath.Session, _ string, i int) {
		face(t, s, "delete", "notes", map[string]any{"where": corpustest.WhereID(0)}, fmt.Sprintf("N%d", 301+i))
	}},
	{"refused write", func(t testing.TB, s *writepath.Session, _ string, _ int) {
		// SQLite takes the key; the rule of keys refuses it, as equal to T7 without regard to case.
		if _, err := statement.Run(s, "INSERT INTO tasks (id, project, title) VALUES ('t7', 'P2', 'folded')", nil); err == nil {
			t.Fatal("a key equal to another without regard to case is taken")
		}
		if got := cells(run(t, s, "SELECT title FROM tasks WHERE id = 'T7'")); got != `[["Task 7"]]` {
			t.Fatalf("the read after the refused write answers %s", got)
		}
	}},
	{"external edit", func(t testing.TB, s *writepath.Session, root string, i int) {
		p := filepath.Join(root, "notes", "2020-01-03.flowmark.md")
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		before, after := editedTitle(i-1), editedTitle(i)
		edited := strings.Replace(string(b), before, after, 1)
		if edited == string(b) {
			t.Fatalf("%s holds no %s", p, before)
		}
		if err := os.WriteFile(p, []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := cells(run(t, s, "SELECT title FROM notes WHERE id = 'N201'")); got != `[["`+after+`"]]` {
			t.Fatalf("the read after the edit answers %s", got)
		}
	}},
}

// face runs a request of a face, an orm request message of kind on entity, and fails t when it fails.
func face(t testing.TB, s *writepath.Session, kind, entity string, body map[string]any, params ...any) {
	t.Helper()
	msg := corpustest.Message(t, s.Executor().Model().ManifestHash, "2026-01-01 00:00:00.000000", kind, entity, body, params...)
	if _, err := s.Exec(context.Background(), msg); err != nil {
		t.Fatalf("%s %s: %v", kind, entity, err)
	}
}

// editedTitle is the title of N201 after the external edit of repetition i, and before the first, at -1, the one the
// corpus is written with.
func editedTitle(i int) string {
	switch i {
	case -1:
		return "Note 201"
	case 0:
		return "Edited 201"
	}
	return fmt.Sprintf("Edited 201 %d", i)
}

// warm runs the statements after which the operations are measured: the first fills the database of the session from
// the corpus, and the first write builds the index of the corpus, once.
func warm(t testing.TB, s *writepath.Session) {
	t.Helper()
	run(t, s, "SELECT COUNT(*) FROM tasks")
	run(t, s, "UPDATE notes SET title = 'warm' WHERE id = 'N1'")
	run(t, s, "SELECT COUNT(*) FROM tasks")
}

// measure runs each operation of costOps on a new corpus of rows rows and returns the counts of each.
func measure(t *testing.T, rows int) map[string]cost.Snapshot {
	t.Helper()
	s, root := openCost(t, rows)
	warm(t, s)
	out := map[string]cost.Snapshot{}
	for _, op := range costOps {
		before := s.Cost().Snapshot()
		op.do(t, s, root, 0)
		out[op.name] = s.Cost().Snapshot().Since(before)
	}
	return out
}

// equalCost names, for each operation, the counters whose count is equal at every size of costSizes. A row of the plan
// that makes an operation cost what it touches adds its counters here, red before the row.
var equalCost = map[string][]cost.Counter{
	// T38: a read reads what changed since the read before it.
	"read task":     readCost,
	"read note":     readCost,
	"external edit": editReadCost,
	"insert task":   writeReadCost,
	"insert note":   writeReadCost,
	"update task":   writeReadCost,
	"update note":   writeReadCost,
	"delete task":   writeReadCost,
	"delete note":   writeReadCost,
	// T46: a request of a face costs what it touches, as a statement does.
	"face read note":   faceReadCost,
	"face insert note": writeReadCost,
	"face update note": writeReadCost,
	"face delete note": writeReadCost,
	// T43-2: a refused write rolls the database back, and fills nothing again.
	"refused write": refusedCost,
}

// refusedCost are the counts of a write the corpus refuses and the read after it: the rows the database takes again, and
// the rows of the database it reads for its difference (T41).
var refusedCost = []cost.Counter{cost.RowsStored, cost.DatabaseRowsWritten, cost.DatabaseRowsRead}

// readCost are the counts of a corpus read: the documents it visits and parses, and the rows it places and indexes.
var readCost = []cost.Counter{cost.FilesVisited, cost.FilesParsed, cost.RowsPlaced, cost.RowsIndexed, cost.IndexEntries}

// faceReadCost are the counts of a read of a face: a corpus read, and the rows the read reads (T46).
var faceReadCost = []cost.Counter{cost.FilesVisited, cost.FilesParsed, cost.RowsPlaced, cost.RowsIndexed, cost.IndexEntries, cost.RowsScanned}

// quietCost names the operations that count nothing: a read of a corpus nothing changed since the statement before it,
// which takes no lock (T43-3).
var quietCost = map[string]bool{"read task": true, "read note": true}

// editReadCost are the counts of the read after an external edit, the rows the corpus enters in its index (T40-1), and the
// rows its database refresh takes from the corpus and writes (T42-1).
var editReadCost = []cost.Counter{cost.FilesVisited, cost.FilesParsed, cost.RowsPlaced, cost.RowsIndexed, cost.IndexEntries, cost.RowsStored,
	cost.DatabaseRowsWritten}

// writeReadCost are the counts of the reads of a write, at its start and under the lock at its commit, the rows its
// post-image copies (T39), the rows the corpus enters in its index (T40-1), and the rows the write and its checks read
// (T40-2), the rows whose positions it gives the database back (T42-1), and the rows of the database it reads for its
// difference, the rows it changed (T41).
var writeReadCost = []cost.Counter{cost.FilesVisited, cost.FilesParsed, cost.RowsPlaced, cost.RowsCopied, cost.RowsIndexed, cost.IndexEntries,
	cost.RowsScanned, cost.RowsStored, cost.DatabaseRowsRead}

// logCost names, for each operation, the counters whose count grows at most with the logarithm of the documents of
// the corpus: a comparison of two corpora walks the paths of the ordered map of their documents to the documents that
// differ, as a lookup in a B-tree walks its depth (T39). Each count is at most logFactor times the base-2 logarithm of
// the documents, at every size of costSizes.
var logCost = map[string][]cost.Counter{
	"insert task": {cost.FilesCompared}, "insert note": {cost.FilesCompared},
	"update task": {cost.FilesCompared}, "update note": {cost.FilesCompared},
	"delete task": {cost.FilesCompared}, "delete note": {cost.FilesCompared},
	"external edit": {cost.FilesCompared},
}

// logFactor bounds a logarithmic count: a comparison walks at most the depth of the map, which the treap of pmap keeps
// within three times the base-2 logarithm of its keys with high probability, along the paths of at most two changed
// documents and their neighbours.
const logFactor = 8

// costDocuments is the number of documents of a corpus of costDBS of rows rows: a project file per sibling group of
// tasks and a notes document per group of notes.
func costDocuments(rows int) int { return rows / costGroup }

// TestCost measures what each point operation touches at each size of costSizes (plan, Ruling: an operation costs what
// it touches): the counts are the engine's own, so two corpora of the same rows count the same; an operation counts
// nothing only when nothing changed before it; each count equalCost names is equal at every size; and each count
// logCost names stays within its logarithmic bound.
func TestCost(t *testing.T) {
	// Each corpus is its own folder and session, so the measures run in parallel; the group returns when all have.
	measured := make([]map[string]cost.Snapshot, len(costSizes))
	var again map[string]cost.Snapshot
	t.Run("measures", func(t *testing.T) {
		for i, rows := range costSizes {
			t.Run(fmt.Sprintf("%d rows", rows), func(t *testing.T) {
				t.Parallel()
				measured[i] = measure(t, rows)
			})
		}
		t.Run(fmt.Sprintf("%d rows again", costSizes[0]), func(t *testing.T) {
			t.Parallel()
			again = measure(t, costSizes[0])
		})
	})
	if t.Failed() {
		return
	}
	for _, op := range costOps {
		if first := measured[0][op.name]; first != again[op.name] {
			t.Errorf("two corpora of the same rows count differently: %s counts %v, then %v", op.name, first, again[op.name])
		}
	}
	counts := map[int]map[string]cost.Snapshot{}
	for i, rows := range costSizes {
		counts[rows] = measured[i]
	}
	for _, op := range costOps {
		for _, rows := range costSizes {
			c := counts[rows][op.name]
			t.Logf("%s at %d rows: %s", op.name, rows, describe(c))
			if quiet := c == (cost.Snapshot{}); quiet != quietCost[op.name] {
				t.Errorf("%s at %d rows counts %s, and it counts nothing only when nothing changed before it", op.name, rows, describe(c))
			}
			if got, want := c.Get(cost.Flushes), flushCost(op.name); got != want {
				t.Errorf("%s at %d rows makes %d flushes, want %d", op.name, rows, got, want)
			}
			for _, k := range logCost[op.name] {
				if bound := logFactor * math.Log2(float64(costDocuments(rows))); float64(c.Get(k)) > bound {
					t.Errorf("%s at %d rows counts %d %s, more than %.0f, logFactor times the logarithm of its documents", op.name, rows, c.Get(k), k, bound)
				}
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

// flushCost is the flushes of the operation op: a write is an acquisition and a commit of one file; an operation that
// takes the lock for its read, after an external edit or to refresh its database, is an acquisition; a read of a
// corpus nothing changed makes none.
func flushCost(op string) int64 {
	switch {
	case quietCost[op]:
		return 0
	case op == "external edit", op == "face read note", op == "refused write":
		return lockFlushes
	}
	return lockFlushes + commitFlushes
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
