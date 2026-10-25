package validate

import (
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
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

// goldenDocs returns the corpus paths of the documents of a golden corpus, sorted.
func goldenDocs(t testing.TB, name string) []string {
	t.Helper()
	dir := corpustest.GoldenDir(t, name)
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
	cat, diags := catalog.Load(corpustest.GoldenDir(t, name))
	if len(diags) > 0 || cat == nil {
		t.Fatalf("golden catalog %s: %+v", name, diags)
	}
	catalogCache[name] = cat
	return cat
}

// TestGoldensValidateClean validates every store golden corpus: a golden is an
// accepting case, so a golden the engine's own validator rejects is not one.
func TestGoldensValidateClean(t *testing.T) {
	for _, name := range goldenRoots {
		t.Run(name, func(t *testing.T) {
			diags, err := Corpus(goldenCatalog(t, name), corpustest.GoldenDir(t, name))
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range diags {
				t.Error(d.String())
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
			data, err := os.ReadFile(filepath.Join(corpustest.GoldenDir(f, name), doc))
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
		if err := corpustest.CopyTree(corpustest.GoldenDir(t, name), root); err != nil {
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
