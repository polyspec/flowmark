package validate

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/store"
)

// memFS is a corpus in memory for the Live cases: folders, documents, symbolic links and entries whose read fails,
// the same on every platform. Each change records the paths a corpus tree drops for it: the path itself, and its
// folder when the folder's entries change.
type memFS struct {
	nodes map[string]*memNode
	dirty map[string]bool
}

type memKind int

const (
	memDir memKind = iota
	memFile
	memLink
)

type memNode struct {
	kind memKind
	data []byte
	fail bool
}

func newMemFS(files map[string]string) *memFS {
	m := &memFS{nodes: map[string]*memNode{".": {kind: memDir}}, dirty: map[string]bool{}}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		m.write(p, files[p])
	}
	m.dirty = map[string]bool{}
	return m
}

// mkdir makes the folder p and the folders above it.
func (m *memFS) mkdir(p string) {
	if p == "." {
		return
	}
	if n, ok := m.nodes[p]; ok && n.kind == memDir {
		return
	}
	m.mkdir(path.Dir(p))
	m.set(p, &memNode{kind: memDir})
}

// set binds p to n, replacing what p held and removing what lay under it.
func (m *memFS) set(p string, n *memNode) {
	old, had := m.nodes[p]
	m.removeUnder(p)
	m.nodes[p] = n
	m.dirty[p] = true
	if !had || old.kind != n.kind {
		m.dirty[path.Dir(p)] = true
	}
}

func (m *memFS) removeUnder(p string) {
	for q := range m.nodes {
		if strings.HasPrefix(q, p+"/") {
			delete(m.nodes, q)
			m.dirty[q] = true
		}
	}
}

func (m *memFS) write(p, text string) {
	m.mkdir(path.Dir(p))
	if n, ok := m.nodes[p]; ok && n.kind == memFile {
		n.data = []byte(text)
		m.dirty[p] = true
		return
	}
	m.set(p, &memNode{kind: memFile, data: []byte(text)})
}

func (m *memFS) link(p string) {
	m.mkdir(path.Dir(p))
	m.set(p, &memNode{kind: memLink})
}

func (m *memFS) remove(p string) {
	if _, ok := m.nodes[p]; !ok || p == "." {
		return
	}
	m.removeUnder(p)
	delete(m.nodes, p)
	m.dirty[p] = true
	m.dirty[path.Dir(p)] = true
}

// fail makes the read of p fail, or succeed again.
func (m *memFS) fail(p string, on bool) {
	if n, ok := m.nodes[p]; ok && p != "." && n.fail != on {
		n.fail = on
		m.dirty[p] = true
	}
}

// changes returns the paths dropped since the last call, sorted.
func (m *memFS) changes() []string {
	out := slices.Sorted(maps.Keys(m.dirty))
	m.dirty = map[string]bool{}
	return out
}

// open returns the error of a read of name that needs a node of kind, nil when the read succeeds.
func (m *memFS) open(name string, kind memKind) error {
	n, ok := m.nodes[name]
	switch {
	case !ok:
		return &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	case n.fail:
		return &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	case n.kind != kind:
		return &fs.PathError{Op: "read", Path: name, Err: errors.New("not of the kind the read needs")}
	}
	return nil
}

func (m *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := m.open(name, memDir); err != nil {
		return nil, err
	}
	var out []fs.DirEntry
	for q, c := range m.nodes {
		if q != "." && path.Dir(q) == name {
			out = append(out, memEntry{name: path.Base(q), kind: c.kind})
		}
	}
	slices.SortFunc(out, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return out, nil
}

func (m *memFS) ReadFile(name string) ([]byte, error) {
	if err := m.open(name, memFile); err != nil {
		return nil, err
	}
	return slices.Clone(m.nodes[name].data), nil
}

type memEntry struct {
	name string
	kind memKind
}

func (e memEntry) Name() string { return e.name }
func (e memEntry) IsDir() bool  { return e.kind == memDir }
func (e memEntry) Type() fs.FileMode {
	switch e.kind {
	case memDir:
		return fs.ModeDir
	case memLink:
		return fs.ModeSymlink
	}
	return 0
}
func (e memEntry) Info() (fs.FileInfo, error) { return memInfo(e), nil }

type memInfo memEntry

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return 0 }
func (i memInfo) Mode() fs.FileMode  { return memEntry(i).Type() }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return i.kind == memDir }
func (i memInfo) Sys() any           { return nil }

// liveRead is what a read of the corpus answers: its documents, its read diagnostics and its unavailable tables.
type liveRead struct {
	files       map[string]*File
	diags       []catalog.Diagnostic
	unavailable map[string]catalog.Diagnostic
}

// oracleRead reads fsys from nothing in the two passes of load, the read Live must equal.
func oracleRead(cat *catalog.Catalog, fsys store.FS) (liveRead, error) {
	c, err := load(cat, fsys)
	if err != nil {
		return liveRead{}, err
	}
	sortDiags(c.diags)
	r := liveRead{files: c.files, diags: c.diags, unavailable: map[string]catalog.Diagnostic{}}
	for _, tb := range cat.Tables {
		if d, ok := c.unavailableBy(tb.Name); ok {
			r.unavailable[tb.Name] = d
		}
	}
	return r, nil
}

func snapshotRead(s *Snapshot) liveRead {
	return liveRead{files: maps.Collect(s.Files()), diags: s.Diagnostics(), unavailable: s.unavailable}
}

// differs returns the paths whose document or diagnostics differ between a and b.
func (a liveRead) differs(b liveRead) map[string]bool {
	out := map[string]bool{}
	for p, f := range a.files {
		if !reflect.DeepEqual(f, b.files[p]) {
			out[p] = true
		}
	}
	for p := range b.files {
		if a.files[p] == nil {
			out[p] = true
		}
	}
	byFile := func(ds []catalog.Diagnostic) map[string][]catalog.Diagnostic {
		m := map[string][]catalog.Diagnostic{}
		for _, d := range ds {
			m[d.File] = append(m[d.File], d)
		}
		return m
	}
	da, db := byFile(a.diags), byFile(b.diags)
	for p := range da {
		if !reflect.DeepEqual(da[p], db[p]) {
			out[p] = true
		}
	}
	for p := range db {
		if da[p] == nil {
			out[p] = true
		}
	}
	return out
}

// liveRun keeps a Live read of a memFS beside the reads of the oracle and requires them equal after every change.
type liveRun struct {
	t    testing.TB
	cat  *catalog.Catalog
	fsys *memFS
	live *Live
	k    *cost.Counts
	last liveRead
	ok   bool
}

func newLiveRun(t testing.TB, cat *catalog.Catalog, files map[string]string) *liveRun {
	r := &liveRun{t: t, cat: cat, fsys: newMemFS(files), k: &cost.Counts{}}
	r.live = NewLive(cat, r.k)
	r.step("the first read")
	return r
}

// step reads the corpus again with the paths its changes dropped, and requires the read to equal the oracle's from
// nothing, the paths it names as changed to cover every path whose read changed, and a read with nothing changed to
// read nothing and answer the same.
func (r *liveRun) step(what string) {
	t := r.t
	t.Helper()
	want, werr := oracleRead(r.cat, r.fsys)
	s, err := r.live.Read(r.fsys, r.fsys.changes(), false)
	if werr != nil || err != nil {
		if (werr == nil) != (err == nil) {
			t.Fatalf("%s: the read fails with %v, the read from nothing with %v", what, err, werr)
		}
		r.ok = false
		return
	}
	got := snapshotRead(s)
	if d := got.differs(want); len(d) > 0 {
		t.Fatalf("%s: the read differs from the read from nothing at %v\ngot diags  %v\nwant diags %v", what, slices.Sorted(maps.Keys(d)), got.diags, want.diags)
	}
	if !reflect.DeepEqual(got.unavailable, want.unavailable) {
		t.Fatalf("%s: the unavailable tables are %v, want %v", what, got.unavailable, want.unavailable)
	}
	if changed, full := s.Changed(); r.ok && !full {
		named := map[string]bool{}
		for _, p := range changed {
			named[p] = true
		}
		for p := range r.last.differs(want) {
			if !named[p] {
				t.Fatalf("%s: the read of %s changed, but the read names %v as changed", what, p, changed)
			}
		}
	}
	before := r.k.Snapshot()
	again, err := r.live.Read(r.fsys, nil, false)
	if err != nil {
		t.Fatalf("%s: the read with nothing changed: %v", what, err)
	}
	if n := r.k.Snapshot().Since(before); n != (cost.Snapshot{}) {
		t.Fatalf("%s: the read with nothing changed counts %v", what, n)
	}
	if changed, full := again.Changed(); full || len(changed) > 0 {
		t.Fatalf("%s: the read with nothing changed names %v, %v", what, changed, full)
	}
	if d := snapshotRead(again).differs(want); len(d) > 0 {
		t.Fatalf("%s: the read with nothing changed differs at %v", what, slices.Sorted(maps.Keys(d)))
	}
	r.last, r.ok = want, true
}

// liveCorpus is a corpus the Live cases start from: its catalog and its files.
type liveCorpus struct {
	name  string
	cat   *catalog.Catalog
	files map[string]string
}

var (
	liveCorporaOnce sync.Once
	liveCorporaList []liveCorpus
	liveCorporaErr  error
)

// liveCorpora returns every corpus of the validator cases, with the test catalog, and every store golden with a
// catalog of its own.
func liveCorpora(t testing.TB) []liveCorpus {
	t.Helper()
	liveCorporaOnce.Do(func() {
		readTree := func(dir string) (map[string]string, error) {
			files := map[string]string{}
			err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(dir, p)
				files[filepath.ToSlash(rel)] = string(b)
				return err
			})
			return files, err
		}
		cat, diags := catalog.Load(catalogRoot)
		if len(diags) > 0 || cat == nil {
			liveCorporaErr = fmt.Errorf("test catalog: %+v", diags)
			return
		}
		cases, err := os.ReadDir(filepath.Join("testdata", "cases"))
		if err != nil {
			liveCorporaErr = err
			return
		}
		for _, e := range cases {
			files, err := readTree(filepath.Join("testdata", "cases", e.Name()))
			if err != nil {
				liveCorporaErr = err
				return
			}
			liveCorporaList = append(liveCorporaList, liveCorpus{"cases/" + e.Name(), cat, files})
		}
		for _, name := range goldenRoots {
			dir := filepath.Join("..", "store", "testdata", filepath.FromSlash(name))
			gcat, diags := catalog.Load(dir)
			if len(diags) > 0 || gcat == nil {
				liveCorporaErr = fmt.Errorf("golden catalog %s: %+v", name, diags)
				return
			}
			files, err := readTree(dir)
			if err != nil {
				liveCorporaErr = err
				return
			}
			liveCorporaList = append(liveCorporaList, liveCorpus{"golden/" + name, gcat, files})
		}
	})
	if liveCorporaErr != nil {
		t.Fatal(liveCorporaErr)
	}
	return liveCorporaList
}

// docPaths returns the corpus documents of files, sorted.
func docPaths(files map[string]string) []string {
	var out []string
	for p := range files {
		if !strings.HasPrefix(p, dbspecDir+"/") {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// TestLive pins that a Live read equals the read from nothing after every change (plan, Ruling: an operation costs
// what it touches): each corpus of the cases and the goldens is built a document at a time, each document is cut and
// restored, and the corpus is taken down a document at a time and then by its folders, so every rule's diagnostics
// appear and disappear and every link meets its target missing, found and gone; and each named case changes what a
// unit is — a link, a failing read, a folder that becomes a document — or what a key answers.
func TestLive(t *testing.T) {
	for _, c := range liveCorpora(t) {
		t.Run(c.name, func(t *testing.T) {
			catalogOnly := map[string]string{}
			for p, b := range c.files {
				if strings.HasPrefix(p, dbspecDir+"/") {
					catalogOnly[p] = b
				}
			}
			r := newLiveRun(t, c.cat, catalogOnly)
			docs := docPaths(c.files)
			for _, p := range docs {
				r.fsys.write(p, c.files[p])
				r.step("add " + p)
			}
			for _, p := range docs {
				r.fsys.write(p, c.files[p][:len(c.files[p])/2])
				r.step("cut " + p)
				r.fsys.write(p, c.files[p])
				r.step("restore " + p)
			}
			for _, p := range docs[:len(docs)/2] {
				r.fsys.remove(p)
				r.step("remove " + p)
			}
			for _, p := range docs[len(docs)/2:] {
				r.fsys.remove(strings.SplitN(p, "/", 2)[0])
				r.step("remove the place of " + p)
			}
		})
	}
	cat := loadCatalog(t)
	plan := "# Plan one\n"
	entry := func(title string) string { return "# " + title + "\n- plan: [P1](../plans/P1.flowmark.md)\n" }
	for _, tc := range []struct {
		name  string
		start map[string]string
		edits []func(m *memFS)
	}{
		{"a symbolic link in a table folder and at the root, then a document in its stead",
			map[string]string{"plans/P1.flowmark.md": plan},
			[]func(m *memFS){
				func(m *memFS) { m.link("plans/P2.flowmark.md") },
				func(m *memFS) { m.link("items.flowmark.md") },
				func(m *memFS) { m.link("plans") },
				func(m *memFS) { m.write("plans/P2.flowmark.md", plan) },
				func(m *memFS) { m.write("items.flowmark.md", "") },
				func(m *memFS) { m.remove("items.flowmark.md") },
			}},
		{"a document whose read fails makes its table unavailable until it reads",
			map[string]string{"plans/P1.flowmark.md": plan, "entries/E1.flowmark.md": entry("One")},
			[]func(m *memFS){
				func(m *memFS) { m.fail("plans/P1.flowmark.md", true) },
				func(m *memFS) { m.fail("entries/E1.flowmark.md", true) },
				func(m *memFS) { m.fail("plans/P1.flowmark.md", false) },
				func(m *memFS) { m.write("entries/E1.flowmark.md", entry("Two")) },
				func(m *memFS) { m.fail("entries/E1.flowmark.md", false) },
			}},
		{"a folder whose read fails, at a table place and outside every place",
			map[string]string{"plans/P1.flowmark.md": plan, "notes/a/x.flowmark.md": plan},
			[]func(m *memFS){
				func(m *memFS) { m.fail("plans", true) },
				func(m *memFS) { m.fail("notes/a", true) },
				func(m *memFS) { m.write("notes/a/y.flowmark.md", plan) },
				func(m *memFS) { m.fail("notes/a", false) },
				func(m *memFS) { m.fail("plans", false) },
			}},
		{"a folder that becomes a document and a document that becomes a folder",
			map[string]string{"plans/P1.flowmark.md": plan, "entries/E1.flowmark.md": entry("One")},
			[]func(m *memFS){
				func(m *memFS) { m.remove("plans"); m.write("plans", plan) },
				func(m *memFS) {
					m.remove("entries/E1.flowmark.md")
					m.write("entries/E1.flowmark.md/x.flowmark.md", plan)
				},
				func(m *memFS) { m.remove("plans"); m.write("plans/P1.flowmark.md", plan) },
				func(m *memFS) { m.remove("entries/E1.flowmark.md"); m.write("entries/E1.flowmark.md", entry("Again")) },
			}},
		{"a link whose target is missing, found, renamed and found again",
			map[string]string{"entries/E1.flowmark.md": entry("One"), "entries/E2.flowmark.md": entry("Two")},
			[]func(m *memFS){
				func(m *memFS) { m.write("plans/P1.flowmark.md", plan) },
				func(m *memFS) { m.remove("plans/P1.flowmark.md"); m.write("plans/P2.flowmark.md", plan) },
				func(m *memFS) { m.write("plans/P1.flowmark.md", plan) },
				func(m *memFS) { m.mkdir("plans/2026") },
			}},
		{"a case variant of a place and a document outside every place",
			map[string]string{"plans/P1.flowmark.md": plan},
			[]func(m *memFS){
				func(m *memFS) { m.write("Plans/P1.flowmark.md", plan) },
				func(m *memFS) { m.write("Items.flowmark.md", "") },
				func(m *memFS) { m.write("stray.flowmark.md", plan) },
				func(m *memFS) { m.write("deep/er/stray.flowmark.md", plan) },
				func(m *memFS) { m.remove("Plans"); m.remove("deep") },
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newLiveRun(t, cat, tc.start)
			for i, e := range tc.edits {
				e(r.fsys)
				r.step(fmt.Sprintf("edit %d", i+1))
			}
		})
	}
	t.Run("a key two shard documents hold makes the table unavailable until one lets it go", func(t *testing.T) {
		scat := shardCatalog(t)
		root := t.TempDir()
		writeShardNotes(t, scat, root, "notes/2026.flowmark.md", [3]string{"N1", "One", "2026-03-04"})
		writeShardNotes(t, scat, root, "notes/2027.flowmark.md", [3]string{"N2", "Two", "2027-01-02"})
		writeShardNotes(t, scat, root, "notes/2028.flowmark.md", [3]string{"N1", "Again", "2028-01-02"})
		read := func(rel string) string {
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
		r := newLiveRun(t, scat, map[string]string{"notes/2026.flowmark.md": read("notes/2026.flowmark.md")})
		r.fsys.write("notes/2027.flowmark.md", read("notes/2027.flowmark.md"))
		r.step("add a second key")
		r.fsys.write("notes/2028.flowmark.md", read("notes/2028.flowmark.md"))
		r.step("add the key again")
		if _, ok := r.last.unavailable["notes"]; !ok {
			t.Fatal("a key two documents hold leaves the table available")
		}
		r.fsys.remove("notes/2026.flowmark.md")
		r.step("remove the first holder")
		if _, ok := r.last.unavailable["notes"]; ok {
			t.Fatal("the table stays unavailable after the duplicate is gone")
		}
		r.fsys.link("notes/2026.flowmark.md")
		r.step("a link in the shard folder")
	})
}

// liveOps decodes fuzzed bytes into changes of a memFS: each three bytes are an action, a path of paths and a text of
// texts.
func liveOps(m *memFS, paths, texts []string, ops []byte, step func(string)) {
	for i := 0; i+2 < len(ops); i += 3 {
		p := paths[int(ops[i+1])%len(paths)]
		text := texts[int(ops[i+2])%len(texts)]
		var what string
		switch ops[i] % 8 {
		case 0, 1, 2:
			m.write(p, text)
			what = "write"
		case 3:
			m.remove(p)
			what = "remove"
		case 4:
			m.remove(path.Dir(p))
			what = "remove the folder of"
		case 5:
			m.link(p)
			what = "link"
		case 6:
			_, failing := m.nodes[p]
			m.fail(p, failing && !m.nodes[p].fail)
			what = "toggle the failure of"
		case 7:
			m.mkdir(p)
			what = "make the folder"
		}
		step(fmt.Sprintf("op %d: %s %s", i/3+1, what, p))
	}
}

// FuzzLive applies fuzzed changes to a corpus of the cases or the goldens and requires the Live read after each to
// equal the read from nothing: the paths are the corpus's documents and folders and their neighbours, and the texts
// its documents, their halves, and their keys and links renamed.
func FuzzLive(f *testing.F) {
	f.Add(uint8(0), []byte{})
	f.Add(uint8(1), []byte{3, 0, 0, 0, 0, 1, 6, 1, 0, 6, 1, 0})
	f.Add(uint8(2), []byte{5, 2, 0, 0, 2, 3, 4, 0, 0, 7, 4, 0, 0, 4, 2})
	f.Add(uint8(25), []byte{0, 0, 5, 0, 1, 6, 3, 1, 0, 0, 1, 1})
	f.Add(uint8(28), []byte{0, 0, 2, 0, 3, 1, 4, 1, 0, 1, 3, 7, 6, 0, 0})
	f.Fuzz(func(t *testing.T, which uint8, ops []byte) {
		corpora := liveCorpora(t)
		c := corpora[int(which)%len(corpora)]
		docs := docPaths(c.files)
		seen := map[string]bool{}
		var paths []string
		add := func(p string) {
			if p != "" && p != "." && !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
		texts := []string{""}
		for _, p := range docs {
			add(p)
			add(path.Dir(p))
			add(path.Join(path.Dir(p), "X9"+docSuffix))
			add(path.Join(path.Dir(p), "2026", "X8"+docSuffix))
			b := c.files[p]
			texts = append(texts, b, b[:len(b)/2], strings.ReplaceAll(b, "1", "2"), strings.ReplaceAll(b, "](", "]("+"x"))
		}
		add("stray" + docSuffix)
		r := newLiveRun(t, c.cat, c.files)
		liveOps(r.fsys, paths, texts, ops, r.step)
	})
}
