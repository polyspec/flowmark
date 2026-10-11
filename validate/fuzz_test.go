package validate

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

// goldenRoots are the corpora of the store goldens, each a corpus root with its own
// dbspec/ set. The fuzz seeds are their documents, so the seeds are the goldens.
var goldenRoots = []string{
	"files",
	"document",
	"equivalence/block",
	"equivalence/block_table",
	"equivalence/document",
	"equivalence/document_table",
	"equivalence/files",
}

// moduleRoot returns the root of the module: the nearest ancestor of the test's
// working directory that holds go.mod. The go tool declares that working directory
// to be the package directory, and go.mod declares the module root, so another
// package's testdata is found from a declared root rather than from where this
// package stands. It fails the test when no ancestor holds go.mod.
func moduleRoot(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	for dir := wd; ; {
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && fi.Mode().IsRegular() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("module root: no ancestor of %s holds go.mod", wd)
		}
		dir = parent
	}
}

// goldenDir returns the directory of a golden corpus of the store package,
// <module root>/store/testdata/<name>.
func goldenDir(t testing.TB, name string) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "store", "testdata", filepath.FromSlash(name))
}

// goldenDocs returns the corpus paths of the documents of a golden corpus, sorted.
func goldenDocs(t testing.TB, name string) []string {
	t.Helper()
	dir := goldenDir(t, name)
	var docs []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), docSuffix) {
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			docs = append(docs, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(docs)
	return docs
}

// catalogCache holds the catalog of each golden corpus, loaded once.
var (
	catalogMu    sync.Mutex
	catalogCache = map[string]*catalog.Catalog{}
)

func goldenCatalog(t testing.TB, name string) *catalog.Catalog {
	t.Helper()
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if cat, ok := catalogCache[name]; ok {
		return cat
	}
	cat, diags := catalog.Load(goldenDir(t, name))
	if len(diags) > 0 || cat == nil {
		t.Fatalf("golden catalog %s: %+v", name, diags)
	}
	catalogCache[name] = cat
	return cat
}

// copyTree copies the regular files of the golden corpus src into dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// TestGoldensValidateClean validates every store golden corpus: a golden is an
// accepting case, so a golden the engine's own validator rejects is not one.
func TestGoldensValidateClean(t *testing.T) {
	for _, name := range goldenRoots {
		t.Run(name, func(t *testing.T) {
			diags, err := Corpus(goldenCatalog(t, name), goldenDir(t, name))
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range diags {
				t.Errorf("%s:%d:%d %s: %s", d.File, d.Line, d.Column, d.Rule, d.Message)
			}
		})
	}
}

// FuzzCorpus replaces one document of a golden corpus with the fuzzed bytes and
// validates the corpus: the validator must not panic, must report located
// diagnostics with a rule, and must report the same diagnostics on a second run.
// The seeds are the golden documents themselves, so the fuzzer starts from valid
// corpora.
func FuzzCorpus(f *testing.F) {
	for g, name := range goldenRoots {
		for i, doc := range goldenDocs(f, name) {
			data, err := os.ReadFile(filepath.Join(goldenDir(f, name), doc))
			if err != nil {
				f.Fatal(err)
			}
			f.Add(uint8(g), uint8(i), data)
		}
	}
	f.Fuzz(func(t *testing.T, g, i uint8, data []byte) {
		name := goldenRoots[int(g)%len(goldenRoots)]
		docs := goldenDocs(t, name)
		doc := docs[int(i)%len(docs)]
		cat := goldenCatalog(t, name)
		root := t.TempDir()
		if err := copyTree(goldenDir(t, name), root); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(doc)), data, 0o644); err != nil {
			t.Fatal(err)
		}
		diags, err := Corpus(cat, root)
		if err != nil {
			t.Fatalf("corpus: %v", err)
		}
		for _, d := range diags {
			if d.File == "" || d.Rule == "" || d.Line < 1 || d.Column < 1 {
				t.Fatalf("diagnostic not located: %+v", d)
			}
		}
		again, err := Corpus(cat, root)
		if err != nil {
			t.Fatalf("second run: %v", err)
		}
		if !reflect.DeepEqual(diags, again) {
			t.Fatalf("the diagnostics differ between two runs:\n%+v\n%+v", diags, again)
		}
	})
}
