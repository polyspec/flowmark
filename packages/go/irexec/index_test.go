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
		b := newDocIndex(p)
		for _, x := range d.walk {
			n.indexRow(b, x, dk)
		}
		n.applyIndex(b)
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
	for k, docs := range c.idx.All() {
		set := map[locator]bool{}
		for _, locs := range docs.All() {
			for _, l := range locs {
				set[l] = true
			}
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
	c.markLocked(path).walk = true
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

// TestIndexMarks pins each mark a change of a write's corpus makes (docs.go): after the change and its mark, the index
// and the records of the next corpus equal those built from nothing, and without its mark they differ. Each change is
// made after the write owned the document and the corpus brought it in, so it takes the path of a document already the
// write's own.
func TestIndexMarks(t *testing.T) {
	check := checkFresh
	checkFresh = nil
	defer func() { checkFresh = check }()
	rowOf := func(w *writer, table, key string) *rec {
		x := w.next.lookupOne(keyIndex(table, key))
		if x == nil {
			t.Fatalf("no %s row %s", table, key)
		}
		return x
	}
	// ownedDoc makes the document of the row of table and key the write's own and brings the corpus in.
	ownedDoc := func(w *writer, table, key string) *rec {
		x := w.own(rowOf(w, table, key))
		w.next.recsOf(x.file)
		return rowOf(w, table, key)
	}
	cases := []struct {
		name string
		// change changes the next corpus of w; marked tells it to mark what it changed.
		change func(w *writer, marked bool)
	}{
		{"the value of a row of a document", func(w *writer, marked bool) {
			x := ownedDoc(w, "items", "T3")
			if marked {
				x = w.own(x)
			}
			setValue(x.tb, x.row, "owner", cell{text: "P1"})
		}},
		{"the key of a placement row, which its block rows name", func(w *writer, marked bool) {
			x := ownedDoc(w, "notes", "N1")
			if marked {
				x = w.own(x)
			}
			x.row.Key = "N9"
		}},
		{"a row added to a block", func(w *writer, marked bool) {
			x := w.own(ownedDoc(w, "notes", "N1"))
			x.row.Blocks["steps"] = append(slices.Clip(x.row.Blocks["steps"]), &store.Row{Table: "steps", Key: "S9", Title: "Added", Scalars: map[string]*string{}})
			if marked {
				w.next.changed(x.file)
			}
		}},
		{"a row removed from a document", func(w *writer, marked bool) {
			x := ownedDoc(w, "items", "T3")
			f := w.next.file(x.file)
			if marked {
				f = w.ownFile(x.file)
			}
			f.rows = slices.DeleteFunc(slices.Clone(f.rows), func(r *store.Row) bool { return r == x.row })
		}},
		{"a block row moved to another placement row", func(w *writer, marked bool) {
			from := w.own(ownedDoc(w, "notes", "N1"))
			to := w.own(ownedDoc(w, "notes", "N3"))
			s := from.row.Blocks["steps"][0]
			from.row.Blocks["steps"] = slices.Clone(from.row.Blocks["steps"][1:])
			if to.row.Blocks == nil {
				to.row.Blocks = map[string][]*store.Row{}
			}
			to.row.Blocks["steps"] = append(slices.Clip(to.row.Blocks["steps"]), s)
			w.next.changed(from.file)
			if marked {
				w.next.changed(to.file)
			}
		}},
		{"a document dropped", func(w *writer, marked bool) {
			x := ownedDoc(w, "notes", "N3")
			if marked {
				w.next.dropFile(x.file)
			} else {
				w.next.files = w.next.files.Delete(x.file)
			}
		}},
	}
	for _, tc := range cases {
		for _, marked := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s, marked %v", tc.name, marked), func(t *testing.T) {
				c := baseFixture(t).c
				w := &writer{c: c, next: c.fork(), touched: map[string]bool{}}
				before := imageOf(c)
				tc.change(w, marked)
				w.next.docsMu.Lock()
				w.next.freshLocked()
				fault := indexFault(w.next)
				w.next.docsMu.Unlock()
				if marked && fault != "" {
					t.Fatalf("the index after the marked change: %s", fault)
				}
				if !marked && fault == "" {
					t.Fatal("the index after the change without its mark equals the index built from nothing")
				}
				wantImage(t, c, before)
			})
		}
	}
}
