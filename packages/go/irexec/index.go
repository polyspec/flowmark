package irexec

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/internal/pmap"
	"github.com/polyspec/flowmark/packages/go/store"
)

// The index of a corpus finds the rows a write and its checks read without walking a table (plan, Ruling: an operation
// costs what it touches). It maps an index key to the rows that hold it, and it is persistent: a corpus read or written
// from another shares its index and changes it for the documents it changes, so keeping it costs the rows of those
// documents. An index key names what a row holds:
//
//   - its key, byte for byte (keyIndex), and without regard to case (foldIndex);
//   - the key each of its foreign keys names (refIndex), the
//     values of each of its unique keys (uniqueIndex) and its sibling group when its table has an order column
//     (groupIndex);
//   - the number of its key and of each identity column, and the decimal suffix of its key after the table's
//     key_prefix (numberIndex), in numeric order, so the largest is the last of its range;
//   - for each of the values above that is not a value of its column type, the fault of that value (faultIndex), so a
//     check that evaluates the values of a table finds the rows whose evaluation fails as the walk of the table did.
//
// A check reads the rows of the keys it needs (candidates) and judges each as it judged each row of the table: the rows
// the index leaves out are those the judgement passes over.

// locator is a row of the index: the document that holds it and the row.
type locator struct {
	path string
	row  *store.Row
}

// byDoc holds the locators of one index key by the document that holds their rows, so a change of the rows of one
// document writes the locators of that document, not those of every row that shares the key.
type byDoc = pmap.Map[string, []locator]

func keyIndex(table, key string) string { return "k\x00" + table + "\x00" + key }

func foldIndex(table, key string) string { return "f\x00" + table + "\x00" + store.FoldKey(key) }

// refIndex is the index key of the rows of table whose i-th foreign key names the key key.
func refIndex(table string, i int, key string) string {
	return "r\x00" + table + "\x00" + strconv.Itoa(i) + "\x00" + key
}

// uniqueIndex is the index key of the rows of table whose unique key u holds the joined values text (uniqueText).
func uniqueIndex(table string, u []string, text string) string {
	return "u\x00" + table + "\x00" + strings.Join(u, ",") + "\x00" + text
}

// groupIndex is the index key of the rows of a table with an order column whose sibling group is group (groupKey).
func groupIndex(table, group string) string { return "g\x00" + table + "\x00" + group }

// faultIndex is the index key of the rows of table whose value of what fails its evaluation: "ref <i>", "unique <columns>"
// or "group".
func faultIndex(table, what string) string { return "e\x00" + table + "\x00" + what }

func refFault(table string, i int) string { return faultIndex(table, "ref "+strconv.Itoa(i)) }

func uniqueFault(table string, u []string) string {
	return faultIndex(table, "unique "+strings.Join(u, ","))
}

func groupFault(table string) string { return faultIndex(table, "group") }

// numberIndex is the index key of the non-negative decimal digits n of column col of table: the length of the digits
// before them, so the keys of a column sort in numeric order. col "" is the key's suffix after the key prefix.
func numberIndex(table, col, n string) string {
	return fmt.Sprintf("n\x00%s\x00%s\x00%010d%s", table, col, len(n), n)
}

// numberRange is the first index key of the numbers of column col of table and the first after them.
func numberRange(table, col string) (string, string) {
	p := "n\x00" + table + "\x00" + col + "\x00"
	return p, p[:len(p)-1] + "\x01"
}

// indexedUnique reports whether the index holds the unique key u of tb: a unique key of a files table that holds its
// order column is read by its table, since a position is no stored value.
func indexedUnique(tb *catalog.Table, u []string) bool {
	return tb.Storage.Kind != catalog.StorageFiles || !slices.Contains(u, tb.Order)
}

// indexKeys returns the index keys of the record x of c.
func (c *Corpus) indexKeys(x *rec) []string {
	tb := x.tb
	keys := []string{keyIndex(tb.Name, x.row.Key), foldIndex(tb.Name, x.row.Key)}
	for i, fk := range tb.ForeignKeys {
		target, ok := c.cat.Table(fk.Table)
		if !ok {
			continue
		}
		key, named, err := c.referencedKey(x, fk, target)
		switch {
		case err != nil:
			keys = append(keys, refFault(tb.Name, i))
		case named:
			keys = append(keys, refIndex(tb.Name, i, key))
		}
	}

	for _, u := range tb.Uniques {
		if !indexedUnique(tb, u) {
			continue
		}
		text, ok, err := c.uniqueText(x, u)
		switch {
		case err != nil:
			keys = append(keys, uniqueFault(tb.Name, u))
		case ok:
			keys = append(keys, uniqueIndex(tb.Name, u, text))
		}
	}
	if tb.Order != "" {
		if g, err := c.groupKey(x); err != nil {
			keys = append(keys, groupFault(tb.Name))
		} else {
			keys = append(keys, groupIndex(tb.Name, g))
		}
	}
	if p := c.keyPrefix(tb.Name); p != "" && strings.HasPrefix(x.row.Key, p) {
		if s := x.row.Key[len(p):]; decimal(s) {
			keys = append(keys, numberIndex(tb.Name, "", s))
		}
	}
	for _, col := range numberColumns(tb) {
		if v, err := c.value(x, col); err == nil && !v.null {
			if n, perr := strconv.ParseInt(v.text, 10, 64); perr == nil && n > 0 {
				keys = append(keys, numberIndex(tb.Name, col, strconv.FormatInt(n, 10)))
			}
		}
	}
	return keys
}

// decimal reports whether s is a decimal number without a leading zero.
func decimal(s string) bool {
	return s != "" && (len(s) == 1 || s[0] != '0') && strings.Trim(s, "0123456789") == ""
}

// keyPrefix returns the key_prefix of table, "" when it has none.
func (c *Corpus) keyPrefix(table string) string {
	for _, d := range c.cat.Documents {
		for _, t := range d.Tables {
			if t.Name == table && t.Settings != nil && t.Settings.KeyPrefix != nil {
				return t.Settings.KeyPrefix.Prefix
			}
		}
	}
	return ""
}

// numberColumns returns the columns of tb whose numbers the index holds: its key and its identity columns, the columns a
// write allocates max + 1 over.
func numberColumns(tb *catalog.Table) []string {
	out := []string{tb.Key}
	for _, col := range tb.Columns {
		if col.Identity && col.Name != tb.Key {
			out = append(out, col.Name)
		}
	}
	return out
}

// groupKey names the sibling group of row x of a table with an order column: its placement parent and its parent.
func (c *Corpus) groupKey(x *rec) (string, *Error) {
	k := ""
	if x.parent != nil {
		k = fmt.Sprintf("%p", x.parent.row)
	}
	if pc := parentColumn(x.tb); pc != "" {
		v, err := c.value(x, pc)
		if err != nil {
			return "", err
		}
		if v.null {
			k += "\x00null"
		} else {
			k += "\x00=" + v.text
		}
	}
	return k, nil
}

// uniqueText returns the values of the unique key u of row x joined, false when one is NULL. A block row placed nowhere
// — its placement key names no row — holds no unique key that takes its placement key.
func (c *Corpus) uniqueText(x *rec, u []string) (string, bool, *Error) {
	parts := make([]string, len(u))
	for i, col := range u {
		if x.parent == nil && x.tb.Storage.Kind == catalog.StorageBlock && col == x.tb.Placement() {
			return "", false, nil
		}
		v, err := c.value(x, col)
		if err != nil {
			return "", false, err
		}
		if v.null {
			return "", false, nil
		}
		parts[i] = v.text
	}
	return strings.Join(parts, "\x00"), true, nil
}

// docKeys holds the index keys of each row of one document, as the index holds them. A corpus never changes the docKeys
// of a document it shares; a write changes those of the documents it made its own (ownFile).
type docKeys map[*store.Row][]string

// docIndex is the change of the index that bringing one document in makes: the rows that leave each key and the rows
// that enter it, which applyIndex writes to the index at once, so a key the rows of the document share is written once.
type docIndex struct {
	path string
	gone map[string]map[*store.Row]bool
	add  map[string]map[*store.Row]bool
}

// newDocIndex returns the empty change of the index for the document path.
func newDocIndex(path string) *docIndex {
	return &docIndex{path: path, gone: map[string]map[*store.Row]bool{}, add: map[string]map[*store.Row]bool{}}
}

// indexRow enters the record x of the document of b in the change b, recording its keys in dk.
func (c *Corpus) indexRow(b *docIndex, x *rec, dk docKeys) {
	c.cost.Add(cost.RowsIndexed, 1)
	keys := c.indexKeys(x)
	dk[x.row] = keys
	for _, k := range keys {
		if b.add[k] == nil {
			b.add[k] = map[*store.Row]bool{}
		}
		b.add[k][x.row] = true
	}
}

// unindexRow takes row r of the document of b out of the index in the change b, by the keys ks it entered the index
// with. The docKeys that recorded them are left as they are: a corpus read or written from another may share them.
func (c *Corpus) unindexRow(b *docIndex, r *store.Row, ks []string) {
	for _, k := range ks {
		if b.gone[k] == nil {
			b.gone[k] = map[*store.Row]bool{}
		}
		b.gone[k][r] = true
		delete(b.add[k], r)
	}
}

// applyIndex writes the change b to the index of c: for each key it changes, the locators of the document of b are
// those it held, without the rows that left, and the rows that entered.
func (c *Corpus) applyIndex(b *docIndex) {
	keys := slices.Collect(maps.Keys(b.add))
	for k := range b.gone {
		if b.add[k] == nil {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		docs, _ := c.idx.Get(k)
		old, _ := docs.Get(b.path)
		locs := make([]locator, 0, len(old)+len(b.add[k]))
		for _, l := range old {
			if !b.gone[k][l.row] {
				locs = append(locs, l)
			}
		}
		for r := range b.add[k] {
			locs = append(locs, locator{b.path, r})
		}
		c.cost.Add(cost.IndexEntries, len(locs))
		if len(locs) == 0 {
			docs = docs.Delete(b.path)
		} else {
			docs = docs.Set(b.path, locs)
		}
		if docs.Len() == 0 {
			c.idx = c.idx.Delete(k)
		} else {
			c.idx = c.idx.Set(k, docs)
		}
	}
}
