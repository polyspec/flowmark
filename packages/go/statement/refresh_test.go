package statement

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store"
)

// refreshRoots are the golden corpora of the store that hold documents, each with its own dbspec/ set; the other golden
// folders hold a set and no document.
var refreshRoots = []string{"files", "document", "equivalence/block", "equivalence/block_table", "equivalence/document",
	"equivalence/document_table", "equivalence/files"}

// refreshCorpus is a corpus root in a temporary folder with its catalog and the text of each of its documents.
type refreshCorpus struct {
	root string
	cat  *catalog.Catalog
	docs map[string]string
}

// refreshSources returns the sources of the refresh cases, each copying its corpus into a root: the store's goldens and
// the executor's test corpus.
func refreshSources(t testing.TB) map[string]func(t testing.TB, root string) {
	copyFrom := func(src string) func(t testing.TB, root string) {
		return func(t testing.TB, root string) {
			if err := corpustest.CopyTree(src, root); err != nil {
				t.Fatal(err)
			}
		}
	}
	out := map[string]func(t testing.TB, root string){}
	for _, name := range refreshRoots {
		out[name] = copyFrom(corpustest.GoldenDir(t, name))
	}
	catalogDir := copyFrom(filepath.Join(corpustest.ModuleRoot(t), "irexec", "testdata", "catalog"))
	out["executor base"] = func(t testing.TB, root string) {
		catalogDir(t, root)
		corpustest.WriteFiles(t, root, corpustest.BaseDocs())
	}
	return out
}

// newRefreshCorpus copies the corpus of copyTo into a temporary root and loads its catalog.
func newRefreshCorpus(t testing.TB, copyTo func(t testing.TB, root string)) *refreshCorpus {
	t.Helper()
	root := t.TempDir()
	copyTo(t, root)
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("catalog of %s: %+v", root, diags)
	}
	docs := corpustest.Documents(t, root)
	if len(docs) == 0 {
		t.Fatalf("the corpus of %s holds no document to change", root)
	}
	return &refreshCorpus{root: root, cat: cat, docs: docs}
}

// open reads the corpus as it stands.
func (r *refreshCorpus) open(t testing.TB) *irexec.Corpus {
	t.Helper()
	c, err := irexec.Open(r.cat, store.DirFS(r.root))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// write sets the document p to text, and removes it when text is nil.
func (r *refreshCorpus) write(t testing.TB, p string, text *string) {
	t.Helper()
	full := filepath.Join(r.root, filepath.FromSlash(p))
	if text == nil {
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return
	}
	corpustest.WriteFiles(t, r.root, map[string]string{p: *text})
}

// dump returns the rows of every table of cat in db, by key, and the sequence SQLite keeps of each AUTOINCREMENT table.
func dump(t testing.TB, db *sql.DB, cat *catalog.Catalog) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, tb := range cat.Tables {
		names := columnNames(tb)
		for i := range names {
			names[i] = "+" + names[i]
		}
		rows, err := db.Query("SELECT " + strings.Join(names, ", ") + " FROM " + quote(tb.Name))
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for rows.Next() {
			row, err := scan(rows, len(names))
			if err != nil {
				t.Fatal(err)
			}
			lines = append(lines, fmt.Sprintf("%#v", row))
		}
		if err := firstErr(rows.Err(), rows.Close()); err != nil {
			t.Fatal(err)
		}
		slices.Sort(lines)
		out[tb.Name] = lines
	}
	seqs, err := sequencesOf(context.Background(), db, cat)
	if err != nil {
		t.Fatal(err)
	}
	for table, n := range seqs {
		out["sqlite_sequence"] = append(out["sqlite_sequence"], fmt.Sprintf("%s %d", table, n))
	}
	slices.Sort(out["sqlite_sequence"])
	return out
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// refreshStep brings the database db of the corpus prev to the corpus of r as it stands and requires it to hold what a
// fill of that corpus from nothing holds; a refresh fails exactly when that fill fails. It returns the corpus and its
// database, nil when the fill failed.
func refreshStep(t testing.TB, r *refreshCorpus, step string, prev *irexec.Corpus, db *sql.DB) (*irexec.Corpus, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	next := r.open(t)
	want, wantErr := materialize(ctx, next)
	if want != nil {
		defer closeDB(t, want)
	}
	if db == nil {
		if wantErr != nil {
			return next, nil
		}
		db, err := materialize(ctx, next)
		if err != nil {
			t.Fatal(err)
		}
		return next, db
	}
	err := refresh(ctx, db, prev, next)
	if (err == nil) != (wantErr == nil) {
		t.Fatalf("%s: the refresh fails with %v, the fill from nothing with %v", step, err, wantErr)
	}
	if err != nil {
		closeDB(t, db)
		return next, nil
	}
	if g, w := dump(t, db, r.cat), dump(t, want, r.cat); !reflect.DeepEqual(g, w) {
		for _, name := range append(tableNames(r.cat), "sqlite_sequence") {
			if !reflect.DeepEqual(g[name], w[name]) {
				t.Fatalf("%s: the refreshed %s holds\n%s\nthe fill from nothing\n%s", step, name, strings.Join(g[name], "\n"), strings.Join(w[name], "\n"))
			}
		}
	}
	return next, db
}

func tableNames(cat *catalog.Catalog) []string {
	out := make([]string, len(cat.Tables))
	for i, tb := range cat.Tables {
		out[i] = tb.Name
	}
	return out
}

func closeDB(t testing.TB, db *sql.DB) {
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestRefresh pins that a database refreshed from the corpus it held to the next one holds what a fill of the next one
// from nothing holds, and fails exactly when that fill fails (plan, Ruling: an operation costs what it touches): each
// document of each source is cut, restored, removed and restored, two documents are exchanged, a document is added
// outside every place, and the AUTOINCREMENT counters are set past every key and removed; the sequences SQLite keeps are
// compared too.
func TestRefresh(t *testing.T) {
	for name, src := range refreshSources(t) {
		t.Run(name, func(t *testing.T) {
			r := newRefreshCorpus(t, src)
			c, db := refreshStep(t, r, "the corpus", nil, nil)
			if db == nil {
				t.Fatalf("the corpus %s fills no database", name)
			}
			defer func() {
				if db != nil {
					closeDB(t, db)
				}
			}()
			step := func(what string, changes map[string]*string) {
				t.Helper()
				for p, text := range changes {
					r.write(t, p, text)
				}
				if db == nil {
					c, db = refreshStep(t, r, what, nil, nil)
					return
				}
				c, db = refreshStep(t, r, what, c, db)
			}
			paths := slices.Sorted(func(yield func(string) bool) {
				for p := range r.docs {
					if !yield(p) {
						return
					}
				}
			})
			text := func(s string) *string { return &s }
			for _, p := range paths {
				body := r.docs[p]
				step("cut "+p, map[string]*string{p: text(body[:len(body)/2])})
				step("restore "+p, map[string]*string{p: text(body)})
				step("remove "+p, map[string]*string{p: nil})
				step("restore "+p+" again", map[string]*string{p: text(body)})
			}
			if len(paths) >= 2 {
				a, b := paths[0], paths[len(paths)-1]
				step("swap "+a+" and "+b, map[string]*string{a: text(r.docs[b]), b: text(r.docs[a])})
				step("swap them back", map[string]*string{a: text(r.docs[a]), b: text(r.docs[b])})
			}
			step("a document outside every place", map[string]*string{"stray.flowmark.md": text("# Stray\n")})
			step("its removal", map[string]*string{"stray.flowmark.md": nil})
			counters := map[string]int64{}
			for i, tb := range r.cat.Tables {
				if tb.Autoincrement {
					counters[tb.Name] = int64(1000 + i)
				}
			}
			if data := catalog.RenderSequences(counters); data != nil {
				step("counters past every key", map[string]*string{catalog.SequencesFile: text(string(data))})
				step("the removal of the counters", map[string]*string{catalog.SequencesFile: nil})
			}
		})
	}
}

// FuzzRefresh pins the equality of TestRefresh for any text of any document of the sources: the seeds are their
// documents as they are and cut.
func FuzzRefresh(f *testing.F) {
	sources := refreshSources(f)
	names := slices.Sorted(func(yield func(string) bool) {
		for n := range sources {
			if !yield(n) {
				return
			}
		}
	})
	corpora := make([]*refreshCorpus, len(names))
	for i, n := range names {
		corpora[i] = newRefreshCorpus(f, sources[n])
		for j, p := range slices.Sorted(func(yield func(string) bool) {
			for p := range corpora[i].docs {
				if !yield(p) {
					return
				}
			}
		}) {
			body := corpora[i].docs[p]
			f.Add(uint8(i), uint8(j), body)
			f.Add(uint8(i), uint8(j), body[:len(body)/2])
		}
	}
	f.Fuzz(func(t *testing.T, source, doc uint8, text string) {
		r := newRefreshCorpus(t, sources[names[int(source)%len(names)]])
		paths := slices.Sorted(func(yield func(string) bool) {
			for p := range r.docs {
				if !yield(p) {
					return
				}
			}
		})
		c, db := refreshStep(t, r, "the corpus", nil, nil)
		if db == nil {
			t.Fatal("the source fills no database")
		}
		defer func() {
			if db != nil {
				closeDB(t, db)
			}
		}()
		p := paths[int(doc)%len(paths)]
		r.write(t, p, &text)
		_, db = refreshStep(t, r, "the text of "+p, c, db)
	})
}
