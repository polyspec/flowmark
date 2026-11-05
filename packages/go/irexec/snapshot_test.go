package irexec

import (
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/flowmark/packages/go/validate"
)

// sameCorpus requires got to answer as want does: the same documents and walk entries, the diagnostics of every path,
// the rows of every table as read and as stored, the files of every table, and the unavailable tables.
func sameCorpus(t *testing.T, step string, got, want *Corpus) {
	t.Helper()
	if d := got.Differs(want); len(d) != 0 {
		t.Fatalf("%s: the corpus differs from the read from the disk at %v", step, d)
	}
	if d := want.Differs(got); len(d) != 0 {
		t.Fatalf("%s: the read from the disk differs from the corpus at %v", step, d)
	}
	var paths []string
	for p := range want.files.All() {
		paths = append(paths, p)
	}
	for p := range want.walk.All() {
		paths = append(paths, p)
	}
	for _, p := range paths {
		if !reflect.DeepEqual(got.Diagnostics(p), want.Diagnostics(p)) || string(got.Data(p)) != string(want.Data(p)) {
			t.Fatalf("%s: %s reads %+v; from the disk %+v", step, p, got.Diagnostics(p), want.Diagnostics(p))
		}
	}
	for _, tb := range want.cat.Tables {
		for name, rows := range map[string]func(*Corpus, string) ([][]any, error){"Rows": (*Corpus).Rows, "StoredRows": (*Corpus).StoredRows} {
			g, gerr := rows(got, tb.Name)
			w, werr := rows(want, tb.Name)
			if !reflect.DeepEqual(g, w) || (gerr == nil) != (werr == nil) {
				t.Fatalf("%s: the %s of %s are %v, %v; from the disk %v, %v", step, name, tb.Name, g, gerr, w, werr)
			}
		}
		gf, gerr := got.FilesOf(tb.Name)
		wf, werr := want.FilesOf(tb.Name)
		if gerr != nil || werr != nil {
			t.Fatalf("%s: the files of %s: %v; from the disk %v", step, tb.Name, gerr, werr)
		}
		if !slices.Equal(gf, wf) {
			t.Fatalf("%s: the files of %s are %v; from the disk %v", step, tb.Name, gf, wf)
		}
	}
	if g, w := maps.Collect(got.walk.All()), maps.Collect(want.walk.All()); !reflect.DeepEqual(g, w) {
		t.Fatalf("%s: the walk entries are %v; from the disk %v", step, g, w)
	}
	if !reflect.DeepEqual(got.unavailable, want.unavailable) {
		t.Fatalf("%s: the unavailable tables are %v; from the disk %v", step, got.unavailable, want.unavailable)
	}
}

// TestFromSnapshot pins that the corpus of a Live read, built on the corpus of the read before it, answers as the
// corpus opened from the disk, and that the corpus it was built on still answers as it did (plan, Ruling: an
// operation costs what it touches): each golden and the test corpus is built a document at a time, each document cut
// and restored, and the corpus taken down a document at a time.
func TestFromSnapshot(t *testing.T) {
	srcs := []string{catalogRoot}
	for _, name := range goldenRoots {
		srcs = append(srcs, corpustest.GoldenDir(t, name))
	}
	for _, src := range srcs {
		t.Run(filepath.ToSlash(src), func(t *testing.T) {
			docs := map[string]string{}
			err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel, err := filepath.Rel(src, p)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if strings.HasPrefix(rel, "dbspec/") {
					return nil
				}
				b, err := os.ReadFile(p)
				docs[rel] = string(b)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			if err := corpustest.CopyTree(filepath.Join(src, "dbspec"), filepath.Join(root, "dbspec")); err != nil {
				t.Fatal(err)
			}
			cat, diags := catalog.Load(root)
			if len(diags) > 0 || cat == nil {
				t.Fatalf("catalog of %s: %+v", src, diags)
			}
			fsys := store.DirFS(root)
			live := validate.NewLive(cat, nil)
			var prev, prevWant *Corpus
			step := func(what string, changed ...string) {
				t.Helper()
				// A change names the path and every folder above it, as the corpus tree drops them.
				var dropped []string
				for _, p := range changed {
					for q := p; q != "."; q = path.Dir(q) {
						dropped = append(dropped, q)
					}
				}
				dropped = append(dropped, ".")
				s, err := live.Read(fsys, dropped, prev == nil)
				if err != nil {
					t.Fatal(err)
				}
				got := FromSnapshot(cat, s, prev, nil)
				want, err := Open(cat, fsys)
				if err != nil {
					t.Fatal(err)
				}
				sameCorpus(t, what, got, want)
				if prev != nil {
					sameCorpus(t, what+": the corpus before", prev, prevWant)
				}
				prev, prevWant = got, want
			}
			write := func(p, text string) {
				corpustest.WriteFiles(t, root, map[string]string{p: text})
			}
			step("the empty corpus")
			names := slices.Sorted(func(yield func(string) bool) {
				for p := range docs {
					if !yield(p) {
						return
					}
				}
			})
			for _, p := range names {
				write(p, docs[p])
				step("add "+p, p)
			}
			for _, p := range names {
				write(p, docs[p][:len(docs[p])/2])
				step("cut "+p, p)
				write(p, docs[p])
				step("restore "+p, p)
			}
			// A document outside every place, a folder too deep in a place and a file that is no document are walk entries,
			// until they are removed.
			strays := []string{"stray.flowmark.md", "outside/stray.flowmark.md"}
			for _, p := range names {
				if dir, _, ok := strings.Cut(p, "/"); ok && !slices.Contains(strays, dir+"/a/b/stray.flowmark.md") {
					strays = append(strays, dir+"/a/b/stray.flowmark.md", dir+"/stray.txt")
				}
			}
			for _, stray := range strays {
				write(stray, "# Stray\n")
				step("add "+stray, stray)
			}
			for _, stray := range strays {
				gone := stray
				if d := path.Dir(stray); strings.HasSuffix(d, "/a/b") || d == "outside" {
					gone = strings.TrimSuffix(d, "/b")
				}
				if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(gone))); err != nil {
					t.Fatal(err)
				}
				step("remove "+gone, gone)
			}
			for _, p := range names {
				if err := os.Remove(filepath.Join(root, filepath.FromSlash(p))); err != nil {
					t.Fatal(err)
				}
				step("remove "+p, p)
			}
		})
	}
}
