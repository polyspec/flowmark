package irexec

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/polyspec/flowmark/packages/go/store"
)

// Every corpus of the tests of this package that brings marked documents in compares its index and its records with
// those built from nothing (indexFault), so a change to a row that no mark announced fails the test that made it.
func init() {
	checkFresh = func(c *Corpus) {
		if fault := indexFault(c); fault != "" {
			panic("irexec: the index is not the index built from nothing: " + fault)
		}
	}
}

// indexFault returns how the index and the records of c, all of whose documents are brought in, differ from those of
// its documents built from nothing, "" when they do not.
func indexFault(c *Corpus) string {
	n := &Corpus{cat: c.cat, files: c.files, walk: c.walk, unavailable: c.unavailable, docs: map[string]*docRecs{}}
	for p, f := range c.files.All() {
		d := &docRecs{byRow: map[*store.Row]*rec{}}
		n.rewalk(p, f, d)
		n.docs[p] = d
		dk := docKeys{}
		for _, x := range d.walk {
			n.indexRow(p, x, dk)
		}
		n.keys = n.keys.Set(p, dk)
	}
	if g, w := indexSet(c), indexSet(n); !maps.EqualFunc(g, w, maps.Equal) {
		return fmt.Sprintf("the index holds %v, built from nothing %v", g, w)
	}
	for p := range c.files.All() {
		gk, _ := c.keys.Get(p)
		wk, _ := n.keys.Get(p)
		if !maps.EqualFunc(gk, wk, slices.Equal) {
			return fmt.Sprintf("the keys of %s are %v, built from nothing %v", p, gk, wk)
		}
	}
	for p := range c.keys.All() {
		if c.file(p) == nil {
			return fmt.Sprintf("the index records keys of %s, which the corpus does not hold", p)
		}
	}
	for p, d := range c.docs {
		w := n.docLocked(p)
		if w == nil {
			return fmt.Sprintf("the corpus holds records of %s, which it does not hold", p)
		}
		if len(d.walk) != len(w.walk) {
			return fmt.Sprintf("the walk of %s has %d records, built from nothing %d", p, len(d.walk), len(w.walk))
		}
		for i, x := range d.walk {
			y := w.walk[i]
			if x.row != y.row || x.file != y.file || (x.parent == nil) != (y.parent == nil) ||
				(x.parent != nil && x.parent.row != y.parent.row) || d.byRow[x.row] != x || x.at != i {
				return fmt.Sprintf("record %d of %s is %+v, built from nothing %+v", i, p, *x, *y)
			}
		}
		if len(d.byRow) != len(d.walk) {
			return fmt.Sprintf("%s finds %d rows, of %d records", p, len(d.byRow), len(d.walk))
		}
	}
	return ""
}

// indexSet returns the index of c as a set of locators by index key.
func indexSet(c *Corpus) map[string]map[locator]bool {
	out := map[string]map[locator]bool{}
	for k, locs := range c.idx.All() {
		set := map[locator]bool{}
		for _, l := range locs {
			set[l] = true
		}
		out[k] = set
	}
	return out
}

// TestIndexFault pins that the comparison of indexFault finds a row changed without a mark, a row added without a mark
// and a record that holds no row of its document.
func TestIndexFault(t *testing.T) {
	c := goldenFixture(t, "document").c
	var path string
	var f *file
	for p, g := range c.files.All() {
		if len(g.rows) > 0 {
			path, f = p, g
			break
		}
	}
	if f == nil {
		t.Fatal("the test corpus holds no row")
	}
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	c.changedLocked(path, nil)
	c.freshLocked()
	if fault := indexFault(c); fault != "" {
		t.Fatalf("the corpus as read: %s", fault)
	}
	cases := map[string]func(*Corpus) func(){
		"a key changed without a mark": func(c *Corpus) func() {
			r := f.rows[0]
			old := r.Key
			r.Key += "-changed"
			return func() { r.Key = old }
		},
		"a row added without a mark": func(c *Corpus) func() {
			old := f.rows
			f.rows = append(slices.Clip(f.rows), &store.Row{Table: f.table, Key: "added"})
			return func() { f.rows = old }
		},
		"a record of a row its document does not hold": func(c *Corpus) func() {
			d := c.docLocked(path)
			x := d.walk[0]
			old := x.row
			x.row = &store.Row{Table: f.table, Key: old.Key}
			return func() { x.row = old }
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			undo := change(c)
			defer undo()
			if indexFault(c) == "" {
				t.Fatal("indexFault finds no fault")
			}
		})
	}
}
