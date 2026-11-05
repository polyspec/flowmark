package irexec

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/store"
)

// The index of a corpus finds the rows a request needs without walking a table (plan, Ruling: an operation costs what it
// touches). It maps an index key to the rows that hold it, and it is persistent: a corpus read or written from another
// shares its index and changes it for the documents it changes, so keeping it costs the rows of those documents. An
// index key names what a row holds:
//
//   - its key, byte for byte (keyIndex), and without regard to case (foldIndex);
//   - the value of a column that a lookup reads by value (valueIndex): a foreign key, a placement or parent key, a
//     unique key's columns, a state machine's column, and the group of an ordered row;
//   - the number of an identity column and the decimal suffix of a key of the table's key prefix (numberIndex), in
//     numeric order, so the largest is the last of its range.
//
// A row is found by its document and its row; the records of a document are built for each corpus that asks for them.

// locator is a row of the index: the document that holds it and the row.
type locator struct {
	path string
	row  *store.Row
}

func keyIndex(table, key string) string { return "k\x00" + table + "\x00" + key }

func foldIndex(table, key string) string { return "f\x00" + table + "\x00" + store.FoldKey(key) }

func valueIndex(table, col, v string) string { return "v\x00" + table + "\x00" + col + "\x00" + v }

// numberIndex is the index key of the non-negative decimal digits n of column col of table: the length of the digits
// before them, so the keys of a column sort in numeric order. col "" is the key's suffix after the key prefix.
func numberIndex(table, col, n string) string {
	return fmt.Sprintf("n\x00%s\x00%s\x00%010d%s", table, col, len(n), n)
}

// groupIndex is the value index of the sibling group of a row of an ordered table.
func groupIndex(table, group string) string { return valueIndex(table, "\x00group", group) }

// valueColumns returns the columns of tb whose values the index holds: every foreign key column, the placement key, the
// parent column, the columns of every unique key and the state machine's column.
func valueColumns(cat *catalog.Catalog, tb *catalog.Table) []string {
	var cols []string
	add := func(c string) {
		if c != "" && c != tb.Key && !slices.Contains(cols, c) && (tb.Storage.Kind != catalog.StorageFiles || c != tb.Order) {
			cols = append(cols, c)
		}
	}
	for _, fk := range tb.ForeignKeys {
		for _, c := range fk.Columns {
			add(c)
		}
	}
	if tb.Storage.Kind == catalog.StorageBlock {
		add(tb.Placement())
	}
	add(parentColumn(tb))
	if sm := tb.StateMachine; sm != nil {
		add(sm.Column)
	}
	return cols
}

// indexKeys returns the index keys of the record x of c.
func (c *Corpus) indexKeys(x *rec) []string {
	tb := x.tb
	keys := []string{keyIndex(tb.Name, x.row.Key), foldIndex(tb.Name, x.row.Key)}
	for _, col := range valueColumns(c.cat, tb) {
		if v, err := c.value(x, col); err == nil && !v.null {
			keys = append(keys, valueIndex(tb.Name, col, v.text))
		}
	}
	for _, u := range tb.Uniques {
		if tb.Storage.Kind == catalog.StorageFiles && slices.Contains(u, tb.Order) {
			continue // a position is no stored value: such a unique key is read by its table
		}
		if k, ok := c.uniqueKey(x, u); ok {
			keys = append(keys, valueIndex(tb.Name, "\x00unique "+strings.Join(u, ","), k))
		}
	}
	if g, ok := c.groupOf(x); ok {
		keys = append(keys, groupIndex(tb.Name, g))
	}
	if p := c.keyPrefix(tb.Name); p != "" && strings.HasPrefix(x.row.Key, p) {
		if s := x.row.Key[len(p):]; decimal(s) {
			keys = append(keys, numberIndex(tb.Name, "", s))
		}
	}
	for _, col := range identityColumns(tb) {
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

// identityColumns returns the identity columns of tb other than its key.
func identityColumns(tb *catalog.Table) []string {
	var out []string
	for _, col := range tb.Columns {
		if col.Identity && col.Name != tb.Key {
			out = append(out, col.Name)
		}
	}
	return out
}

// groupOf names the sibling group of row x of a table with an order column: its placement parent and its parent; the
// group "\x00fault" holds the rows whose parent is not a value of its type. It is false for a table without an order
// column.
func (c *Corpus) groupOf(x *rec) (string, bool) {
	if x.tb.Order == "" {
		return "", false
	}
	k := ""
	if x.parent != nil {
		k = fmt.Sprintf("%p", x.parent.row)
	}
	if pc := parentColumn(x.tb); pc != "" {
		v, err := c.value(x, pc)
		switch {
		case err != nil:
			return "\x00fault", true
		case v.null:
			k += "\x00null"
		default:
			k += "\x00=" + v.text
		}
	}
	return k, true
}

// uniqueKey returns the canonical values of the unique key u of x, joined, and false when one of them is NULL or not of
// its type.
func (c *Corpus) uniqueKey(x *rec, u []string) (string, bool) {
	parts := make([]string, len(u))
	for i, col := range u {
		v, err := c.value(x, col)
		if err != nil || v.null {
			return "", false
		}
		parts[i] = v.text
	}
	return catalog.JoinKey(parts), true
}

// docKeys holds the index keys of each row of one document, as the index holds them. A corpus never changes the docKeys
// of a document it shares; a write changes those of the documents it made its own (ownFile).
type docKeys map[*store.Row][]string

// indexRow adds the record x, held by the document path, to the index of c, recording its keys in dk.
func (c *Corpus) indexRow(path string, x *rec, dk docKeys) {
	c.cost.Add(cost.RowsIndexed, 1)
	keys := c.indexKeys(x)
	dk[x.row] = keys
	for _, k := range keys {
		locs, _ := c.idx.Get(k)
		c.idx = c.idx.Set(k, append(slices.Clip(locs), locator{path, x.row}))
	}
}

// unindexRow removes row r of the document path from the index of c, by the keys ks it entered the index with. The
// docKeys that recorded them are left as they are: a corpus read or written from another may share them.
func (c *Corpus) unindexRow(path string, r *store.Row, ks []string) {
	for _, k := range ks {
		locs, _ := c.idx.Get(k)
		locs = slices.DeleteFunc(slices.Clone(locs), func(l locator) bool { return l.path == path && l.row == r })
		if len(locs) == 0 {
			c.idx = c.idx.Delete(k)
		} else {
			c.idx = c.idx.Set(k, locs)
		}
	}
}
