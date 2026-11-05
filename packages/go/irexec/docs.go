package irexec

import (
	"maps"
	"slices"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/store"
)

// The records and the index of a corpus are kept for the documents it changes. A document whose rows changed is marked
// (changed): setFile and dropFile mark the document they set, and a write marks each document it makes its own before
// every change to it (writer.ownFile). Before it answers, an access of the records or of the index brings the marked
// documents in (fresh): each one's rows leave the index by the keys recorded when they entered it, and its rows as they
// stand now enter it, so the index costs the rows of the documents that changed (plan, Ruling: an operation costs what it
// touches).

// docRecs are the records of one document of a corpus: each row and its block rows in the document's walk, and the
// record of each row. A record belongs to one corpus; the rows are shared until a write makes the document its own.
type docRecs struct {
	walk  []*rec
	byRow map[*store.Row]*rec
	// gen is the number of the last walk, which each record of it holds.
	gen int
}

// checkFresh, when set, is called with a corpus at each access of its records or its index, after it brought its marked
// documents in, so a test can compare its index with the index built from nothing.
var checkFresh func(*Corpus)

// changed marks the document path of c: its rows changed since they entered the index.
func (c *Corpus) changed(path string) { c.changedRow(path, nil) }

// changedRow marks the document path of c and, when r is not nil, its row r, whose values changed: a document's rows
// enter the index again when they are new to it, marked, under another parent, or under a parent that entered again.
func (c *Corpus) changedRow(path string, r *store.Row) {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	c.changedLocked(path, r)
}

func (c *Corpus) changedLocked(path string, r *store.Row) {
	if c.dirty == nil {
		c.dirty = map[string]map[*store.Row]bool{}
	}
	m := c.dirty[path]
	if m == nil {
		m = map[*store.Row]bool{}
		c.dirty[path] = m
	}
	if r != nil {
		m[r] = true
	}
}

// freshLocked brings the marked documents of c in: their records follow their rows, and the index holds their rows as
// they stand now.
func (c *Corpus) freshLocked() {
	if checkFresh != nil {
		defer checkFresh(c)
	}
	if c.ordered == nil {
		c.reorder()
	}
	if len(c.dirty) == 0 {
		return
	}
	if c.docs == nil {
		c.docs = map[string]*docRecs{}
	}
	for _, path := range slices.Sorted(maps.Keys(c.dirty)) {
		c.freshDoc(path, c.dirty[path])
	}
	c.dirty = nil
	c.reorder()
}

// freshDoc brings the document path of c in, re-entering the rows of marked.
func (c *Corpus) freshDoc(path string, marked map[*store.Row]bool) {
	old, _ := c.keys.Get(path)
	f := c.file(path)
	if f == nil {
		for r, ks := range old {
			c.unindexRow(path, r, ks)
		}
		c.keys = c.keys.Delete(path)
		delete(c.docs, path)
		return
	}
	d := c.docs[path]
	if d == nil {
		d = &docRecs{byRow: map[*store.Row]*rec{}}
		c.docs[path] = d
	}
	parents := make(map[*rec]*rec, len(d.byRow))
	for _, x := range d.byRow {
		parents[x] = x.parent
	}
	c.rewalk(path, f, d)
	dk := make(docKeys, len(d.walk))
	again := map[*rec]bool{}
	for _, x := range d.walk {
		prev, had := parents[x]
		ks, keyed := old[x.row]
		if !keyed || !had || marked[x.row] || prev != x.parent || (x.parent != nil && again[x.parent]) {
			if keyed {
				c.unindexRow(path, x.row, ks)
			}
			again[x] = true
			c.indexRow(path, x, dk)
			continue
		}
		dk[x.row] = ks
	}
	for r, ks := range old {
		if _, live := dk[r]; !live {
			c.unindexRow(path, r, ks)
		}
	}
	c.keys = c.keys.Set(path, dk)
}

// docLocked returns the records of the document path of c, building them at the first call, nil when c holds no such
// document. The document is not marked.
func (c *Corpus) docLocked(path string) *docRecs {
	d := c.docs[path]
	if d != nil {
		return d
	}
	f := c.file(path)
	if f == nil {
		return nil
	}
	d = &docRecs{byRow: map[*store.Row]*rec{}}
	c.rewalk(path, f, d)
	if c.docs == nil {
		c.docs = map[string]*docRecs{}
	}
	c.docs[path] = d
	return d
}

// rewalk builds the walk of the document path from its rows, keeping the record of each row that has one.
func (c *Corpus) rewalk(path string, f *file, d *docRecs) {
	d.walk = d.walk[:0]
	d.gen++
	var place func(r *store.Row, parent *rec)
	place = func(r *store.Row, parent *rec) {
		x := d.byRow[r]
		if x == nil {
			tb, _ := c.cat.Table(r.Table)
			x = &rec{row: r, tb: tb}
			d.byRow[r] = x
		}
		x.file, x.parent, x.at, x.gen = path, parent, len(d.walk), d.gen
		d.walk = append(d.walk, x)
		for _, name := range r.BlockNames() {
			for _, child := range r.Blocks[name] {
				place(child, x)
			}
		}
	}
	for _, r := range f.rows {
		place(r, nil)
	}
	if len(d.byRow) != len(d.walk) {
		for r, x := range d.byRow {
			if x.gen != d.gen {
				delete(d.byRow, r)
			}
		}
	}
}

// repoint makes the records of the document path of c hold the copies of their rows, so a record a write holds stays
// the record of its row after the write made the document its own; the document is marked.
func (c *Corpus) repoint(path string, copies map[*store.Row]*store.Row) {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	if d := c.docs[path]; d != nil {
		for old, row := range copies {
			if x := d.byRow[old]; x != nil {
				x.row = row
				delete(d.byRow, old)
				d.byRow[row] = x
			}
		}
	}
	c.changedLocked(path, nil)
}

// recOf returns the record of row r of c, nil when no document of c holds r.
func (c *Corpus) recOf(r *store.Row) *rec {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	c.freshLocked()
	locs, _ := c.idx.Get(keyIndex(r.Table, r.Key))
	for _, l := range locs {
		if l.row == r {
			if d := c.docLocked(l.path); d != nil {
				return d.byRow[r]
			}
		}
	}
	return nil
}

// tableRecs returns every record of table in corpus order, walking every document: a read of a table's storage order
// reads it through here.
func (c *Corpus) tableRecs(table string) []*rec {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	c.freshLocked()
	var out []*rec
	for p := range c.files.All() {
		if d := c.docLocked(p); d != nil {
			for _, x := range d.walk {
				if x.row.Table == table {
					out = append(out, x)
				}
			}
		}
	}
	return out
}

// scan returns every record of table of c as tableRecs does, for a write or its checks, counting each as scanned: what a
// write reads by walking a table and not by the index.
func scan(c *Corpus, table string) []*rec {
	out := c.tableRecs(table)
	c.cost.Add(cost.RowsScanned, len(out))
	return out
}
