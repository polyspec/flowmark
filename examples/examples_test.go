// Package examples holds the example corpora of the engine (examples/README.md). Its tests read and change them through
// the engine only — the operation layer of the faces over the write path — on copies, so a lock, a journal or a write
// never lands in the repository.
package examples

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/introspect"
	"github.com/polyspec/flowmark/packages/go/ops"
)

// The tracker corpora: the rows of the application schema in three storages of the table items — every row in one
// list document, every row in one table document, and one file per row. They differ in that one store setting.
var trackerCorpora = []struct {
	root    string
	storage catalog.Storage
}{
	{"tracker/aggregate-list", catalog.Storage{Kind: catalog.StorageDocument, Shape: catalog.ShapeList}},
	{"tracker/aggregate-table", catalog.Storage{Kind: catalog.StorageDocument, Shape: catalog.ShapeTable}},
	{"tracker/split-file", catalog.Storage{Kind: catalog.StorageFiles}},
}

// copyCorpus copies the example corpus root into a temporary folder, so that the lock, the journal and every write of
// a test stay out of the repository, and returns the copy's root.
func copyCorpus(t *testing.T, root string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, "dbspec")); err != nil {
		t.Fatalf("the example corpus %s has no catalog: %v", root, err)
	}
	dst := filepath.Join(t.TempDir(), filepath.Base(root))
	if err := os.CopyFS(dst, os.DirFS(root)); err != nil {
		t.Fatalf("copying %s: %v", root, err)
	}
	return dst
}

// open opens the corpus at root through the operation layer of the faces.
func open(t *testing.T, root string) *ops.Corpus {
	t.Helper()
	c, err := ops.Open(context.Background(), root, 0)
	if err != nil {
		t.Fatalf("opening %s: %v", root, err)
	}
	return c
}

// run runs the operation op with members, given as one JSON object, and returns its answer as JSON.
func run(t *testing.T, c *ops.Corpus, op, members string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(members), &m); err != nil {
		t.Fatalf("members %s: %v", members, err)
	}
	r, err := ops.Parse(op, m)
	if err != nil {
		t.Fatalf("%s %s: %v", op, members, err)
	}
	a, err := c.Run(context.Background(), "examples", r)
	if err != nil {
		t.Fatalf("%s %s: %v", op, members, err)
	}
	b, err := ops.JSON(a)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tables returns the rows of every table of the corpus, read through the engine, as the JSON of each rows answer by
// table name, and fails when a table has no row.
func tables(t *testing.T, c *ops.Corpus, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tb := range c.Catalog().Tables {
		a := run(t, c, "rows", `{"table":"`+tb.Name+`"}`)
		if a == `{"rows":[]}` {
			t.Errorf("%s: the table %s has no row", root, tb.Name)
		}
		out[tb.Name] = a
	}
	return out
}

// equalTables fails for each table whose rows differ between the corpora a and b.
func equalTables(t *testing.T, what, a, b string, ra, rb map[string]string) {
	t.Helper()
	if len(ra) != len(rb) {
		t.Errorf("%s: %s has %d tables and %s %d", what, a, len(ra), b, len(rb))
	}
	for name, rows := range ra {
		if rb[name] != rows {
			t.Errorf("%s: the rows of %s differ\n%s: %s\n%s: %s", what, name, a, rows, b, rb[name])
		}
	}
}

// sound fails when the corpus at root has a diagnostic or is not in canonical form: introspection validates every
// document, and a canon of the whole corpus — P1, render(parse(file)) = file — rewrites and leaves nothing and changes
// no byte.
func sound(t *testing.T, root string) {
	t.Helper()
	res, err := introspect.Corpus(root)
	if err != nil {
		t.Fatalf("introspecting %s: %v", root, err)
	}
	for _, d := range res.Diagnostics {
		t.Errorf("%s: %s:%d:%d %s: %s", root, d.File, d.Line, d.Column, d.Rule, d.Message)
	}
	before := snapshot(t, root)
	c := open(t, root)
	s, err := c.Session("examples")
	if err != nil {
		t.Fatal(err)
	}
	canon, err := s.Canon(context.Background(), nil)
	if cerr := s.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatalf("canon of %s: %v", root, err)
	}
	if len(canon.Rewritten) > 0 || len(canon.Reported) > 0 {
		t.Errorf("%s is not canonical: canon rewrote %v and left %v", root, canon.Rewritten, canon.Reported)
	}
	after := snapshot(t, root)
	for p, b := range before {
		if after[p] != b {
			t.Errorf("%s: canon changed %s", root, p)
		}
	}
}

// snapshot returns the bytes of every corpus file under root by its corpus path, the engine's own state aside.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".flowmark" {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestTrackerSchema: each tracker catalog is the application schema (spec 100, packages/go/catalog/testdata/app) but
// for the store setting of items, so the split-file corpus is the application schema itself and the other two differ
// from it in that one line.
func TestTrackerSchema(t *testing.T) {
	app, err := os.ReadFile(filepath.Join("..", "packages", "go", "catalog", "testdata", "app", "dbspec", "flowmark.dbs"))
	if err != nil {
		t.Fatal(err)
	}
	stores := map[catalog.Storage]string{
		{Kind: catalog.StorageDocument, Shape: catalog.ShapeList}:  "    key_prefix 'T'\n    store document list\n",
		{Kind: catalog.StorageDocument, Shape: catalog.ShapeTable}: "    key_prefix 'T'\n    store document table\n",
		{Kind: catalog.StorageFiles}:                               "    key_prefix 'T'\n    store files\n",
	}
	for _, tc := range trackerCorpora {
		b, err := os.ReadFile(filepath.Join(tc.root, "dbspec", "flowmark.dbs"))
		if err != nil {
			t.Fatal(err)
		}
		text, line := string(b), stores[tc.storage]
		if !strings.Contains(text, line) {
			t.Fatalf("%s: no items store line %q", tc.root, line)
		}
		if got := strings.Replace(text, line, stores[catalog.Storage{Kind: catalog.StorageFiles}], 1); got != string(app) {
			t.Errorf("%s: the catalog differs from the application schema beyond the store setting of items", tc.root)
		}
	}
}

// TestTrackerWritesOneFile: in the application schema's storage (spec 100 Files and links, the split-file corpus) a
// transition, a link of an item and a move of an item each write that item's file alone, so no write touches a file
// that holds the whole project.
func TestTrackerWritesOneFile(t *testing.T) {
	root := copyCorpus(t, "tracker/split-file")
	c := open(t, root)
	steps := []struct {
		name, op, members, file string
	}{
		{"a transition writes its history row into the item's file", "transition", `{"table":"items","key":"T3","state":"doing"}`, "items/T3.flowmark.md"},
		{"a link is a block of its from item's file", "row.insert", `{"table":"item_links","values":{"from_item":"T3","to_item":"T1","kind":"relates"}}`, "items/T3.flowmark.md"},
		{"a move takes a rank between its new neighbours", "row.move", `{"table":"items","key":"T6","position":1}`, "items/T6.flowmark.md"},
	}
	for _, s := range steps {
		before := snapshot(t, root)
		run(t, c, s.op, s.members)
		after := snapshot(t, root)
		var changed []string
		for p, b := range after {
			if before[p] != b {
				changed = append(changed, p)
			}
		}
		for p := range before {
			if _, ok := after[p]; !ok {
				changed = append(changed, p)
			}
		}
		if len(changed) != 1 || changed[0] != s.file {
			t.Errorf("%s: the write changed %v, want %s alone", s.name, changed, s.file)
		}
	}
	if got := run(t, c, "row.get", `{"table":"items","key":"T6"}`); !strings.Contains(got, `"position":1,`) {
		t.Errorf("T6 after its move = %s, want position 1", got)
	}
	sound(t, root)
}

// TestTrackerStorages: the tracker corpora declare the three storages of items, each corpus is valid and canonical,
// and the three read back the same rows of every table through the engine.
func TestTrackerStorages(t *testing.T) {
	var first map[string]string
	for _, tc := range trackerCorpora {
		root := copyCorpus(t, tc.root)
		c := open(t, root)
		items, ok := c.Catalog().Table("items")
		if !ok {
			t.Fatalf("%s declares no table items", tc.root)
		}
		if items.Storage != tc.storage {
			t.Errorf("%s stores items as %+v, want %+v", tc.root, items.Storage, tc.storage)
		}
		rows := tables(t, c, tc.root)
		if first == nil {
			first = rows
		} else {
			equalTables(t, "the tracker", trackerCorpora[0].root, tc.root, first, rows)
		}
		sound(t, root)
	}
}
