package irexec

import (
	"cmp"
	"maps"
	"slices"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/internal/pmap"
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

// mark is what changed in a document since its rows entered the index: the rows whose values changed, and whether rows
// joined, left or moved within it (walk).
type mark struct {
	rows map[*store.Row]bool
	walk bool
}

// changed marks the document path of c: rows joined it, left it or moved within it.
func (c *Corpus) changed(path string) {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	c.markLocked(path).walk = true
}

// changedRow marks the row r of the document path of c, whose values changed.
func (c *Corpus) changedRow(path string, r *store.Row) {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	m := c.markLocked(path)
	if m.rows == nil {
		m.rows = map[*store.Row]bool{}
	}
	m.rows[r] = true
}

func (c *Corpus) markLocked(path string) *mark {
	if c.dirty == nil {
		c.dirty = map[string]*mark{}
	}
	m := c.dirty[path]
	if m == nil {
		m = &mark{}
		c.dirty[path] = m
	}
	return m
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

// shareKeysLocked returns the index and the index keys of c for a corpus read or written from it, which shares them: c
// copies the index keys of a document again before it changes them.
func (c *Corpus) shareKeysLocked() (pmap.Map[string, []locator], pmap.Map[string, docKeys]) {
	c.ownKeys = nil
	return c.idx, c.keys
}

// freshDoc brings the document path of c in. A document whose rows joined, left or moved is walked again, and its rows
// enter the index again when they are new to it, marked, under another parent, or under a parent that entered again;
// in a document whose rows only changed their values, the marked rows and their block rows enter it again.
func (c *Corpus) freshDoc(path string, m *mark) {
	old, _ := c.keys.Get(path)
	f := c.file(path)
	if f == nil {
		for r, ks := range old {
			c.unindexRow(path, r, ks)
		}
		c.keys = c.keys.Delete(path)
		delete(c.docs, path)
		delete(c.ownKeys, path)
		return
	}
	dk := old
	if !c.ownKeys[path] {
		dk = make(docKeys, len(old))
		maps.Copy(dk, old)
		if c.ownKeys == nil {
			c.ownKeys = map[string]bool{}
		}
		c.ownKeys[path] = true
	}
	again := func(x *rec) {
		if ks, ok := dk[x.row]; ok {
			c.unindexRow(path, x.row, ks)
		}
		c.indexRow(path, x, dk)
	}
	d := c.docs[path]
	if d == nil || m.walk {
		if d == nil {
			d = &docRecs{byRow: map[*store.Row]*rec{}}
			c.docs[path] = d
		}
		c.rewalk(path, f, d)
		for _, x := range d.walk {
			_, keyed := dk[x.row]
			if !keyed || x.moved == d.gen || m.rows[x.row] || (x.parent != nil && x.parent.again == d.gen) {
				x.again = d.gen
				again(x)
			}
		}
		for r, ks := range dk {
			if x := d.byRow[r]; x == nil || x.gen != d.gen {
				c.unindexRow(path, r, ks)
				delete(dk, r)
			}
		}
	} else {
		var down func(x *rec)
		down = func(x *rec) {
			again(x)
			for _, name := range x.row.BlockNames() {
				for _, child := range x.row.Blocks[name] {
					if y := d.byRow[child]; y != nil {
						down(y)
					}
				}
			}
		}
		var xs []*rec
		for r := range m.rows {
			if x := d.byRow[r]; x != nil {
				xs = append(xs, x)
			}
		}
		slices.SortFunc(xs, func(a, b *rec) int { return cmp.Compare(a.at, b.at) })
		for _, x := range xs {
			down(x)
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

// recsOf returns the records of the document path of c, nil when c holds no such document.
func (c *Corpus) recsOf(path string) *docRecs {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	c.freshLocked()
	return c.docLocked(path)
}

// rewalk builds the walk of the document path from its rows, keeping the record of each row that has one.
func (c *Corpus) rewalk(path string, f *file, d *docRecs) {
	d.walk = d.walk[:0]
	var place func(r *store.Row, parent *rec)
	place = func(r *store.Row, parent *rec) {
		x := d.byRow[r]
		if x == nil {
			tb, _ := c.cat.Table(r.Table)
			x = &rec{row: r, tb: tb, parent: parent, moved: d.gen + 1}
			d.byRow[r] = x
		}
		if x.parent != parent {
			x.moved = d.gen + 1
		}
		x.file, x.parent, x.at, x.gen = path, parent, len(d.walk), d.gen+1
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
	d.gen++
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
	c.markLocked(path).walk = true
}

// lookup returns the records of c that hold the index key k, in corpus order: by document path, then in the walk of
// their document.
func (c *Corpus) lookup(k string) []*rec {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	c.freshLocked()
	locs, _ := c.idx.Get(k)
	if len(locs) == 0 {
		return nil
	}
	out := make([]*rec, 0, len(locs))
	for _, l := range locs {
		if d := c.docLocked(l.path); d != nil {
			if x := d.byRow[l.row]; x != nil {
				out = append(out, x)
			}
		}
	}
	c.cost.Add(cost.RowsScanned, len(out))
	sortCorpusOrder(out)
	return out
}

// candidates returns the records of c that hold any of the index keys keys, once each, in corpus order: the rows a check
// judges in the order of the walk of their table.
func (c *Corpus) candidates(keys ...string) []*rec {
	if len(keys) == 1 {
		return c.lookup(keys[0])
	}
	var out []*rec
	seen := map[*rec]bool{}
	for _, k := range keys {
		for _, x := range c.lookup(k) {
			if !seen[x] {
				seen[x] = true
				out = append(out, x)
			}
		}
	}
	sortCorpusOrder(out)
	return out
}

// maxNumber returns the largest number of column col of table that the index holds (numberIndex), "" when it holds
// none.
func (c *Corpus) maxNumber(table, col string) string {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	c.freshLocked()
	first, end := numberRange(table, col)
	for k := range c.idx.Before(end) {
		if k < first {
			return ""
		}
		c.cost.Add(cost.RowsScanned, 1)
		return k[len(first)+10:]
	}
	return ""
}

// sortCorpusOrder sorts records in corpus order: by document path, then in the walk of their document.
func sortCorpusOrder(xs []*rec) {
	slices.SortFunc(xs, func(a, b *rec) int {
		if a.file != b.file {
			return cmp.Compare(a.file, b.file)
		}
		return cmp.Compare(a.at, b.at)
	})
}

// lookupOne returns the first record of c, in corpus order, that holds the index key k, nil when none does.
func (c *Corpus) lookupOne(k string) *rec {
	if xs := c.lookup(k); len(xs) > 0 {
		return xs[0]
	}
	return nil
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
