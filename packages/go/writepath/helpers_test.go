package writepath

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store/journal"
	"github.com/polyspec/flowmark/packages/go/validate"
)

// patience bounds every wait of a test for something the code under test must publish; it is a guard against a hung
// test, never the condition a test checks.
const patience = 60 * time.Second

// testNow is the request clock of every request of the tests.
const testNow = "2026-10-11 09:00:00.123456"

// recorder is the observer of a test: it keeps every event in order.
type recorder struct {
	mu     sync.Mutex
	events []journal.Event
	notify chan struct{}
}

func (r *recorder) Observe(e journal.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	if r.notify != nil {
		close(r.notify)
		r.notify = nil
	}
}

// waitFor blocks until an event of kind has been published since the last reset.
func (r *recorder) waitFor(t testing.TB, kind journal.Kind) {
	t.Helper()
	deadline := time.After(patience)
	for {
		r.mu.Lock()
		for _, e := range r.events {
			if e.Kind == kind {
				r.mu.Unlock()
				return
			}
		}
		if r.notify == nil {
			r.notify = make(chan struct{})
		}
		ch := r.notify
		r.mu.Unlock()
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("waited %s for %s; got %v", patience, kind, r.kinds())
		}
	}
}

// kinds returns the kinds of the lock and journal events since the last reset; the events of the corpus tree are
// read by treeEvents.
func (r *recorder) kinds() []journal.Kind {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []journal.Kind{}
	for _, e := range r.events {
		if !treeKind(e.Kind) {
			out = append(out, e.Kind)
		}
	}
	return out
}

// treeKind reports whether kind is an event of the corpus tree.
func treeKind(kind journal.Kind) bool {
	return strings.HasPrefix(string(kind), "corpus.")
}

// treeEvents returns the corpus paths, relative to root and slash-separated, of the events of kind since the last
// reset, in order.
func (r *recorder) treeEvents(t testing.TB, root string, kind journal.Kind) []string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []string{}
	for _, e := range r.events {
		if e.Kind != kind {
			continue
		}
		rel, err := filepath.Rel(root, e.Path)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

// documentsRead returns the corpus documents read from the disk since the last reset, sorted.
func (r *recorder) documentsRead(t testing.TB, root string) []string {
	t.Helper()
	out := []string{}
	for _, p := range r.treeEvents(t, root, journal.CorpusRead) {
		if strings.HasSuffix(p, ".flowmark.md") {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

func (r *recorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

// wantKinds requires the events published since the last reset to be want, and resets.
func (r *recorder) wantKinds(t testing.TB, want ...journal.Kind) {
	t.Helper()
	if want == nil {
		want = []journal.Kind{}
	}
	if got := r.kinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	r.reset()
}

// The event sequences of the tests.
var (
	opened    = []journal.Kind{journal.LockAcquired, journal.LockReleased}
	committed = []journal.Kind{journal.LockAcquired, journal.Prepared, journal.Applied, journal.Collected, journal.LockReleased}
)

// moduleRoot returns the root of the module: the nearest ancestor of the test's working directory that holds go.mod.
// The go tool declares that working directory to be the package directory.
func moduleRoot(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; {
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && fi.Mode().IsRegular() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no ancestor of %s holds go.mod", wd)
		}
		dir = parent
	}
}

// lines joins lines with line feeds and a final one.
func lines(ls ...string) string { return strings.Join(ls, "\n") + "\n" }

// baseDocs is the corpus of the executor's test catalog that most cases start from: items with their history in
// blocks, people with a soft-deleted row, notes in files with a block of steps, and tags.
func baseDocs() map[string]string {
	return map[string]string{
		"items.flowmark.md": lines(
			"# items",
			"- [x] T1 Ship the parser owner:[P1](people.flowmark.md)",
			"  - [~] T2 Write the tests due:2026-10-20",
			"- [ ] T3 Review the design",
			"- [-] T4 Drop the old path",
			"",
			"## items row T1",
			"",
			"Parser body.",
			"",
			"### evidence",
			"",
			"Golden tests pass.",
			"",
			"### item_history",
			"| id | from_state | to_state | at |",
			"| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"| 2 | doing | done | 2026-10-01 09:00:00.000000 |",
			"",
			"#### item_history row 2",
			"",
			"##### evidence",
			"",
			"Golden tests pass.",
			"",
			"## items row T2",
			"",
			"### item_history",
			"| id | from_state | to_state | at |",
			"| --- | --- | --- | --- |",
			"| 3 | waiting | doing | 2026-10-02 08:00:00.000000 |",
			"",
			"## items row T4",
			"",
			"### cause",
			"",
			"Superseded.",
			"",
			"### item_history",
			"| id | from_state | to_state | at |",
			"| --- | --- | --- | --- |",
			"| 4 | waiting | dropped | 2026-10-03 08:00:00.000000 |",
			"",
			"#### item_history row 4",
			"",
			"##### cause",
			"",
			"Superseded.",
		),
		"people.flowmark.md": lines(
			"# people",
			"| id | name | email | age | active | score | joined | deleted_at | updated_at |",
			"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
			"| P1 | Ada | ada@x.io | 36 | true | 1.50 | 2026-10-01 09:00:00 |  |  |",
			"| P2 | Max |  |  | false | 0.00 | 2026-10-02 10:00:00 |  |  |",
			"| P3 | Gone | gone@x.io |  | true | 0.00 | 2026-10-03 00:00:00 | 2026-10-05 00:00:00.000000 |  |",
		),
		"notes/N1.flowmark.md": lines(
			"# First note",
			"- item: [T1](../items.flowmark.md#items-row-t1)",
			"- position: 1",
			"- weight: 0.5",
			"",
			"Note body.",
			"",
			"## steps",
			"- S1 Draft done:false",
			"- S2 Review done:true",
		),
		"notes/N2.flowmark.md": lines(
			"# Child note",
			"- parent: [N1](N1.flowmark.md)",
			"- position: 1",
		),
		"notes/N3.flowmark.md": lines(
			"# Second root",
			"- position: 2",
		),
		"tags.flowmark.md": lines(
			"# tags",
			"| id | label | kind |",
			"| --- | --- | --- |",
			"| 1 | go | topic |",
			"| 2 | ops | area |",
		),
	}
}

// cardsDBS is a catalog of a files table that a document table references: a whole-file delete whose restrict check
// needs a table other than its own.
const cardsDBS = `dbspec 1 cards

table cards {
  id varchar(16)
  title varchar(64)
  primary key (id)
  settings {
    store files
    title title
  }
}

table pins {
  id varchar(16)
  card varchar(16)
  primary key (id)
  index ix_pins_card (card)
  foreign key fk_pins_card (card) references cards (id) on delete restrict on update restrict
  settings {
    store document table
  }
}
`

// fixture is a corpus in a temporary root, opened through the write path, with the observer of its events.
type fixture struct {
	t    testing.TB
	root string
	cat  *catalog.Catalog
	j    *journal.Corpus
	c    *Corpus
	rec  *recorder
}

// writeTree writes docs (corpus path to text) under root.
func writeTree(t testing.TB, root string, docs map[string]string) {
	t.Helper()
	for p, text := range docs {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := corpustest.WriteSetFile(filepath.Join(root, "dbspec")); err != nil {
		t.Fatal(err)
	}
}

// executorCatalog returns the dbspec text of the executor's test catalog.
func executorCatalog(t testing.TB) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(moduleRoot(t), "irexec", "testdata", "catalog", "dbspec", "flowmark.dbs"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// newFixture writes the catalog dbs and the documents docs into a temporary root and opens it through the write path.
// The events of the opening are reset.
func newFixture(t testing.TB, dbs string, docs map[string]string) *fixture {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"dbspec/test.dbs": dbs})
	writeTree(t, root, docs)
	return openFixture(t, root)
}

// openFixture opens the corpus at root through the write path.
func openFixture(t testing.TB, root string) *fixture {
	t.Helper()
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("catalog of %s: %+v", root, diags)
	}
	rec := &recorder{}
	j, err := journal.New(root, rec)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Open(context.Background(), cat, j, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	c.clock = func() time.Time { return time.Date(2026, 10, 11, 10, 0, 0, 123456000, time.UTC) }
	f := &fixture{t: t, root: root, cat: cat, j: j, c: c, rec: rec}
	rec.reset()
	return f
}

// baseFixture is the base corpus of the executor's test catalog.
func baseFixture(t testing.TB) *fixture {
	t.Helper()
	return newFixture(t, executorCatalog(t), baseDocs())
}

// session returns a new session of the fixture's corpus, closed when the test ends.
func (f *fixture) session(label string) *Session {
	f.t.Helper()
	s, err := f.c.Session(label)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() {
		if err := s.Close(); err != nil {
			f.t.Errorf("Close: %v", err)
		}
	})
	return s
}

// msg builds a request message: the request of kind on entity with the fields of body, the parameters params and the
// test clock.
func (f *fixture) msg(kind, entity string, body map[string]any, params ...any) []byte {
	f.t.Helper()
	return buildMsg(f.t, f.c.exec.Model().ManifestHash, kind, entity, body, params...)
}

func buildMsg(t testing.TB, hash, kind, entity string, body map[string]any, params ...any) []byte {
	t.Helper()
	req := map[string]any{}
	for k, v := range body {
		req[k] = v
	}
	req["ir_version"] = 1
	req["manifest_hash"] = hash
	req["kind"] = kind
	req["entity"] = entity
	req["n_params"] = len(params)
	if params == nil {
		params = []any{}
	}
	b, err := json.Marshal(map[string]any{"ir_version": 1, "manifest_hash": hash, "request": req, "params": params, "now": testNow})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// whereID is the where of the row whose id is parameter i.
func whereID(i int) map[string]any {
	return map[string]any{"items": []any{map[string]any{"pred": map[string]any{"column": "id", "op": "eq", "p": i}}}}
}

// sets is the set of a request, each column assigned its parameter in order from first.
func sets(first int, cols ...string) []any {
	out := make([]any, len(cols))
	for i, c := range cols {
		out[i] = map[string]any{"column": c, "p": first + i}
	}
	return out
}

// insertTag is the insert of a tag.
func (f *fixture) insertTag(label, kind string) []byte {
	return f.msg("insert", "tags", map[string]any{"set": sets(0, "label", "kind")}, label, kind)
}

// countTags is the count of the tags.
func (f *fixture) countTags() []byte { return f.msg("count", "tags", nil) }

// file returns the text of a corpus file, "" when it is absent.
func (f *fixture) file(p string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(p)))
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

// write writes a corpus file as a human edit would.
func (f *fixture) write(p, text string) {
	f.t.Helper()
	writeTree(f.t, f.root, map[string]string{p: text})
}

// remove removes a corpus file as a human edit would.
func (f *fixture) remove(p string) {
	f.t.Helper()
	if err := os.Remove(filepath.Join(f.root, filepath.FromSlash(p))); err != nil {
		f.t.Fatal(err)
	}
}

// clean requires the corpus to validate without a diagnostic and to hold no stage.
func (f *fixture) clean() {
	f.t.Helper()
	diags, err := validate.Corpus(f.cat, f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, d := range diags {
		f.t.Errorf("%s:%d:%d %s: %s", d.File, d.Line, d.Column, d.Rule, d.Message)
	}
	entries, err := os.ReadDir(filepath.Join(f.root, ".flowmark", "tx"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.t.Fatal(err)
	}
	if len(entries) > 0 {
		f.t.Fatalf("a stage is left: %v", entries)
	}
}

// answerJSON returns the JSON of an answer.
func answerJSON(t testing.TB, a *irexec.Answer) string {
	t.Helper()
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// want requires the request m to answer the JSON text j.
func want(t testing.TB, s *Session, m []byte, j string) {
	t.Helper()
	a, err := s.Exec(context.Background(), m)
	if err != nil {
		t.Fatalf("request %s failed: %v", m, err)
	}
	if got := answerJSON(t, a); got != j {
		t.Fatalf("request %s answered %s, want %s", m, got, j)
	}
}

// codeOf returns the orm code of an error of the write path or of the executor, "" for nil.
func codeOf(err error) string {
	var we *Error
	var ee *irexec.Error
	switch {
	case err == nil:
		return ""
	case errors.As(err, &we):
		return we.Code
	case errors.As(err, &ee):
		return ee.Code
	}
	return "an error of no code: " + err.Error()
}

// fails requires err to carry code and its message to hold every text of has.
func fails(t testing.TB, err error, code string, has ...string) {
	t.Helper()
	if got := codeOf(err); got != code {
		t.Fatalf("error %v has the code %q, want %s", err, got, code)
	}
	for _, h := range has {
		if !strings.Contains(err.Error(), h) {
			t.Fatalf("error %q does not name %q", err.Error(), h)
		}
	}
}

// writeErr returns err as an *Error of the write path, failing the test when it is not one.
func writeErr(t testing.TB, err error) *Error {
	t.Helper()
	var we *Error
	if !errors.As(err, &we) {
		t.Fatalf("error %v is not an *Error of the write path", err)
	}
	return we
}
