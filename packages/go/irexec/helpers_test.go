package irexec

import (
	"encoding/json"
	"errors"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/flowmark/packages/go/validate"
)

// catalogRoot is the corpus root of the executor's test catalog; a case writes its
// corpus documents beside a copy of its dbspec/ set.
const catalogRoot = "testdata/catalog"

// testNow is the request clock of every case.
const testNow = "2026-10-11 09:00:00.123456"

// goldenRoots are the corpora of the store goldens, each a corpus root with its own
// dbspec/ set: the read cases and the fuzz seeds read them.
var goldenRoots = []string{
	"files",
	"document",
	"equivalence/block",
	"equivalence/block_table",
	"equivalence/document",
	"equivalence/document_table",
	"equivalence/files",
}

// fixture is a corpus in a temporary root, its executor, and the corpus as read.
type fixture struct {
	t    testing.TB
	root string
	cat  *catalog.Catalog
	e    *Executor
	c    *Corpus
}

// newFixture copies the corpus root src into a temporary root, writes the
// documents docs (corpus path to text) there, and opens it.
func newFixture(t testing.TB, src string, docs map[string]string) *fixture {
	t.Helper()
	return newFixtureIn(t, t.TempDir(), src, docs)
}

// newFixtureIn is newFixture with the temporary root root.
func newFixtureIn(t testing.TB, root, src string, docs map[string]string) *fixture {
	t.Helper()
	if err := corpustest.CopyTree(src, root); err != nil {
		t.Fatal(err)
	}
	corpustest.WriteFiles(t, root, docs)
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("catalog of %s: %+v", src, diags)
	}
	e, err := New(cat)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, root: root, cat: cat, e: e}
	f.reopen()
	return f
}

// catalogFixture is a corpus of the test catalog holding docs.
func catalogFixture(t testing.TB, docs map[string]string) *fixture {
	t.Helper()
	return newFixture(t, catalogRoot, docs)
}

// goldenFixture is a copy of a store golden corpus.
func goldenFixture(t testing.TB, name string) *fixture {
	t.Helper()
	return newFixture(t, corpustest.GoldenDir(t, name), nil)
}

// on returns the fixture reporting to t, the test of a case.
func (f *fixture) on(t testing.TB) *fixture {
	c := *f
	c.t = t
	return &c
}

// sub runs a case of the fixture as a subtest of t. The case reads the fixture's
// corpus; a case that commits commits to its own copy of the fixture.
func (f *fixture) sub(t *testing.T, name string, fn func(f *fixture)) {
	t.Helper()
	t.Run(name, func(t *testing.T) { fn(f.on(t)) })
}

// reopen reads the corpus of the fixture again.
func (f *fixture) reopen() {
	f.t.Helper()
	c, err := Open(f.cat, store.DirFS(f.root), nil)
	if err != nil {
		f.t.Fatal(err)
	}
	f.c = c
}

// msg builds a request message: the request of kind on entity with the fields of
// body, the parameters params and the test clock.
func (f *fixture) msg(kind, entity string, body map[string]any, params ...any) []byte {
	f.t.Helper()
	return f.msgAt(testNow, kind, entity, body, params...)
}

// msgAt builds a request message as msg does, at the request clock now.
func (f *fixture) msgAt(now, kind, entity string, body map[string]any, params ...any) []byte {
	f.t.Helper()
	return corpustest.Message(f.t, f.e.Model().ManifestHash, now, kind, entity, body, params...)
}

// run executes a message against the fixture's corpus.
func (f *fixture) run(m []byte) (*Result, *Error) {
	res, err := f.e.Execute(f.c, m)
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			f.t.Fatalf("an error that is not an *Error: %v", err)
		}
		return nil, e
	}
	return res, nil
}

// ok executes a message that must succeed.
func (f *fixture) ok(m []byte) *Result {
	f.t.Helper()
	res, err := f.run(m)
	if err != nil {
		f.t.Fatalf("request %s failed: %v", m, err)
	}
	return res
}

// fails executes a message that must fail with code, and returns the error.
// fails asserts that the request m fails with code.
func (f *fixture) fails(m []byte, code string) {
	f.t.Helper()
	res, err := f.run(m)
	f.checkFailure(m, res, err, code)
}

// failure asserts that the request m fails with code and returns that failure.
func (f *fixture) failure(m []byte, code string) *Error {
	f.t.Helper()
	res, err := f.run(m)
	f.checkFailure(m, res, err, code)
	return err
}

// checkFailure fails the test unless the run that answered res with err failed with code.
func (f *fixture) checkFailure(m []byte, res *Result, err *Error, code string) {
	f.t.Helper()
	if err == nil {
		b, jsonErr := json.Marshal(res.Answer)
		if jsonErr != nil {
			f.t.Fatal(jsonErr)
		}
		f.t.Fatalf("request %s answered %s; want %s", m, b, code)
	}
	if err.Code != code {
		f.t.Fatalf("request %s failed with %v; want %s", m, err, code)
	}
}

// answer returns the JSON of an answer.
func answer(t testing.TB, res *Result) string {
	t.Helper()
	b, err := json.Marshal(res.Answer)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// want requires the answer of m to be the JSON text j.
func (f *fixture) want(m []byte, j string) *Result {
	f.t.Helper()
	res := f.ok(m)
	if got := answer(f.t, res); got != j {
		f.t.Fatalf("request %s answered\n%s\nwant\n%s", m, got, j)
	}
	return res
}

// commit writes the post-image of a write to the fixture's root, as the write path
// would: each target's file written or removed. The corpus must then validate clean,
// read back as the post-image's next corpus, and becomes the fixture's corpus.
func (f *fixture) commit(res *Result) {
	f.t.Helper()
	if res.Post == nil {
		f.t.Fatal("the result has no post-image")
	}
	for _, tg := range res.Post.Targets {
		full := filepath.Join(f.root, filepath.FromSlash(tg.Path))
		cur, err := os.ReadFile(full)
		if tg.Before == nil {
			if err == nil {
				f.t.Fatalf("target %s is new to the post-image, but the corpus holds it", tg.Path)
			}
		} else if string(cur) != string(tg.Before) {
			f.t.Fatalf("target %s does not hold its Before bytes", tg.Path)
		}
		if tg.After == nil {
			if err := os.Remove(full); err != nil {
				f.t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(full, tg.After, 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	diags, err := validate.Corpus(f.cat, f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, d := range diags {
		f.t.Errorf("after the commit: %s", d)
	}
	f.reopen()
	next := res.Post.Next
	if len(next.files) != len(f.c.files) {
		f.t.Fatalf("the next corpus holds %d files; the committed corpus %d", len(next.files), len(f.c.files))
	}
	for p, file := range f.c.files {
		nf := next.files[p]
		if nf == nil || string(nf.data) != string(file.data) || len(nf.rows) != len(file.rows) {
			f.t.Fatalf("the next corpus differs from the committed corpus at %s", p)
		}
	}
}

// file returns the text of a corpus file of the fixture.
func (f *fixture) file(p string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(p)))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

// lines joins lines with line feeds and a final one.
func lines(ls ...string) string { return strings.Join(ls, "\n") + "\n" }

// p is a parameter index for a request body.
func p(i int) map[string]any { return map[string]any{"p": i} }

// pred is a predicate item.
func pred(column, op string, extra map[string]any) map[string]any {
	m := map[string]any{"column": column, "op": op}
	for k, v := range extra {
		m[k] = v
	}
	return map[string]any{"pred": m}
}

// where is a top-level group of items.
func where(items ...map[string]any) map[string]any {
	return map[string]any{"items": items}
}

// set is an assignment of column to parameter i.
func set(column string, i int) map[string]any { return map[string]any{"column": column, "p": i} }

// rowIDs returns the id of each row of the answer of res, in order; an id that is no string fails the test.
func rowIDs(t testing.TB, res *Result) []string {
	t.Helper()
	var ids []string
	for _, r := range res.Answer.Rows {
		v, _ := r.Get("id")
		s, ok := v.(string)
		if !ok {
			t.Fatalf("the value %v is no string", v)
		}
		ids = append(ids, s)
	}
	return ids
}
