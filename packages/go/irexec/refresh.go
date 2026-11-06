package irexec

import (
	"bytes"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/internal/pmap"
)

// TableChange is how the stored rows of a table differ between a corpus and the corpus before it: the rows of Gone leave
// a database of the corpus before, by their keys, and the rows of Rows enter it, so it holds the stored rows of the
// corpus; when Whole is true, every row of the table leaves it and the table is filled again.
type TableChange struct {
	Table string
	Whole bool
	Gone  [][]any
	Rows  [][]any
}

// Changed returns, in catalog order, how the stored rows of each table of c differ from those of before, a corpus under
// the same catalog: the rows of each document the two hold with other bytes, the rows of the sibling groups of a files
// table with an order column that those rows join or leave, since a position is derived from the group, and every row of
// a table whose availability differs. It costs the rows of the documents that differ (plan, Ruling: an operation costs
// what it touches).
func (c *Corpus) Changed(before *Corpus) ([]TableChange, error) {
	var paths []string
	same := func(f, g *file) bool {
		return f == g || bytes.Equal(f.data, g.data) && (f.data == nil) == (g.data == nil)
	}
	c.cost.Add(cost.FilesCompared, pmap.Diff(before.files, c.files, same, func(p string) { paths = append(paths, p) }))
	whole := map[string]bool{}
	for _, tb := range c.cat.Tables {
		a, aok := before.unavailable[tb.Name]
		b, bok := c.unavailable[tb.Name]
		if aok != bok || a != b {
			whole[tb.Name] = true
		}
	}
	// A group is taken again in both corpora when a row of a document that differs belongs to it in either.
	groups := groupsOf(before, before.changedRecs(paths, nil))
	for table, keys := range groupsOf(c, c.changedRecs(paths, nil)) {
		if groups[table] == nil {
			groups[table] = map[string]bool{}
		}
		for g := range keys {
			groups[table][g] = true
		}
	}
	gone, rows := before.changedRecs(paths, groups), c.changedRecs(paths, groups)
	var err error
	var out []TableChange
	for _, tb := range c.cat.Tables {
		if whole[tb.Name] {
			out = append(out, TableChange{Table: tb.Name, Whole: true})
			continue
		}
		if len(gone[tb.Name]) == 0 && len(rows[tb.Name]) == 0 {
			continue
		}
		ch := TableChange{Table: tb.Name}
		if ch.Gone, err = before.storedRowsOf(tb, gone[tb.Name]); err != nil {
			return nil, err
		}
		if ch.Rows, err = c.storedRowsOf(tb, rows[tb.Name]); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, nil
}

// changedRecs returns, by table, the records of the documents paths of c, and of each sibling group of groups (by table),
// once each, in corpus order.
func (c *Corpus) changedRecs(paths []string, groups map[string]map[string]bool) map[string][]*rec {
	out := map[string][]*rec{}
	seen := map[*rec]bool{}
	add := func(x *rec) {
		if !seen[x] {
			seen[x] = true
			out[x.tb.Name] = append(out[x.tb.Name], x)
		}
	}
	for _, p := range paths {
		if d := c.recsOf(p); d != nil {
			for _, x := range d.walk {
				add(x)
			}
		}
	}
	for table, keys := range groups {
		for g := range keys {
			for _, x := range c.lookup(groupIndex(table, g)) {
				add(x)
			}
		}
	}
	for _, xs := range out {
		sortCorpusOrder(xs)
	}
	return out
}

// groupsOf returns, by table, the sibling groups of the records recs of c in a files table with an order column, whose
// positions their group derives.
func groupsOf(c *Corpus, recs map[string][]*rec) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for table, xs := range recs {
		tb, ok := c.cat.Table(table)
		if !ok || tb.Storage.Kind != catalog.StorageFiles || tb.Order == "" {
			continue
		}
		for _, x := range xs {
			g, err := c.groupKey(x)
			if err != nil {
				continue // a parent not of its type: the row has no position, and its stored row fails
			}
			if out[table] == nil {
				out[table] = map[string]bool{}
			}
			out[table][g] = true
		}
	}
	return out
}

// storedRowsOf returns the stored rows of the records xs of table tb, as StoredRows returns those of the table. The
// table is needed.
func (c *Corpus) storedRowsOf(tb *catalog.Table, xs []*rec) ([][]any, error) {
	if len(xs) == 0 {
		return nil, nil
	}
	if err := c.need(tb.Name); err != nil {
		return nil, err
	}
	c.cost.Add(cost.RowsStored, len(xs))
	out := make([][]any, 0, len(xs))
	for _, x := range xs {
		row := make([]any, len(tb.Columns))
		for i, col := range tb.Columns {
			v, err := c.storedValue(x, col)
			if err != nil {
				return nil, err
			}
			row[i] = v
		}
		out = append(out, row)
	}
	return out, nil
}
