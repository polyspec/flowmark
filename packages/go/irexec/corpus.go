package irexec

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/flowmark/packages/go/validate"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// docSuffix is the suffix of a corpus document.
const docSuffix = ".flowmark.md"

// Corpus is the corpus a request reads: every corpus document at the place of a
// table with its bytes and rows, the read diagnostics of each, and the tables that
// are unavailable (spec 10 Diagnostics, "Unavailable table"). A Corpus is never
// changed: a write computes the next Corpus beside it (see PostImage).
type Corpus struct {
	cat         *catalog.Catalog
	files       map[string]*file
	unavailable map[string]catalog.Diagnostic
	// walk holds the first corpus.walk diagnostic of each entry at a table's place
	// that is no corpus document, by its corpus path.
	walk map[string]catalog.Diagnostic
	// recs holds the rows of each table, by file path order and then the order
	// the store returns them in; a block row follows its placement row.
	recs map[string][]*rec
	// ordered caches the storage order of each table.
	ordered map[string][]*rec
	// positions caches the 1-based sibling position of each row of a files table
	// with an order column, which its stored rank gives (spec 20 Order).
	positions map[string]map[*rec]int64
	// byRow finds the record of a row.
	byRow map[*store.Row]*rec
}

// file is a corpus document at the place of a files or document table.
type file struct {
	table string
	// data is the bytes read; nil for a file that the corpus did not hold.
	data  []byte
	rows  []*store.Row
	diags []catalog.Diagnostic
}

// rec is a row of the corpus with the file that holds it and, for a block row,
// the row whose scope holds it: its placement parent.
type rec struct {
	row    *store.Row
	file   string
	parent *rec
	tb     *catalog.Table
}

// Open reads the corpus fsys as a request reads it: the corpus folder itself
// (store.DirFS), or the write path's mirror of it, parsing through the memo m of a
// long-lived corpus (nil to parse every document). An error is returned only when
// the corpus root cannot be read.
func Open(cat *catalog.Catalog, fsys store.FS, m *validate.Memo) (*Corpus, error) {
	s, err := validate.Read(cat, fsys, m)
	if err != nil {
		return nil, err
	}
	c := &Corpus{cat: cat, files: map[string]*file{}, unavailable: map[string]catalog.Diagnostic{}, walk: map[string]catalog.Diagnostic{}}
	for path, f := range s.Files {
		c.files[path] = &file{table: f.Table, data: f.Data, rows: f.Rows}
	}
	for _, d := range s.Diagnostics {
		if f := c.files[d.File]; f != nil {
			f.diags = append(f.diags, d)
			continue
		}
		if _, seen := c.walk[d.File]; !seen && d.Rule == store.RuleWalk {
			c.walk[d.File] = d
		}
	}
	for _, tb := range cat.Tables {
		if d, ok := s.Unavailable(tb.Name); ok {
			c.unavailable[tb.Name] = d
		}
	}
	c.index()
	return c, nil
}

// index builds the rows of each table from the files.
func (c *Corpus) index() {
	c.recs = map[string][]*rec{}
	c.reorder()
	c.byRow = map[*store.Row]*rec{}
	paths := make([]string, 0, len(c.files))
	for p := range c.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		for _, r := range c.files[p].rows {
			c.place(r, p, nil)
		}
	}
}

// reorder drops the cached storage orders and positions, which a changed rank or
// a changed set of rows invalidates.
func (c *Corpus) reorder() {
	c.ordered = map[string][]*rec{}
	c.positions = map[string]map[*rec]int64{}
}

// place records row r, held by the file path, and its block rows.
func (c *Corpus) place(r *store.Row, path string, parent *rec) {
	tb, _ := c.cat.Table(r.Table)
	x := &rec{row: r, file: path, parent: parent, tb: tb}
	c.recs[r.Table] = append(c.recs[r.Table], x)
	c.byRow[r] = x
	names := make([]string, 0, len(r.Blocks))
	for name := range r.Blocks {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, child := range r.Blocks[name] {
			c.place(child, path, x)
		}
	}
}

// Catalog returns the catalog the corpus was read with.
func (c *Corpus) Catalog() *catalog.Catalog { return c.cat }

// Rows returns every row of table in storage order (spec 20 Order), soft-deleted
// rows included, each as the value of every column of the table in declaration
// order, written in the runtime value model JSON a read answers with (spec 60 The
// wire): the export reads the corpus through it (spec 70 The export). It fails as
// CONSTRAINT, naming the diagnostic, when the table is unavailable or a value is not
// of its column type, and as ENTITY_UNKNOWN when the catalog has no such table.
func (c *Corpus) Rows(table string) ([][]any, error) {
	if _, ok := c.cat.Table(table); !ok {
		return nil, errf(codeEntityUnknown, "the catalog has no table %s", table)
	}
	recs, err := c.rows(table)
	if err != nil {
		return nil, err
	}
	out := make([][]any, 0, len(recs))
	for _, x := range recs {
		row := make([]any, len(x.tb.Columns))
		for i, col := range x.tb.Columns {
			v, err := c.value(x, col.Name)
			if err != nil {
				return nil, err
			}
			w, err := output(col.Type, v)
			if err != nil {
				return nil, err
			}
			row[i] = w
		}
		out = append(out, row)
	}
	return out, nil
}

// need fails as CONSTRAINT when table is unavailable: a request that needs any of
// its rows never answers from the other files' rows alone.
func (c *Corpus) need(table string) *Error {
	if d, ok := c.unavailable[table]; ok {
		return diagError("the table "+table+" is unavailable", d)
	}
	return nil
}

// Diagnostics returns the read diagnostics of the corpus file path: its format
// diagnostics, which the commit's rewrite refusal reads.
func (c *Corpus) Diagnostics(path string) []catalog.Diagnostic {
	if f := c.files[path]; f != nil {
		return slices.Clone(f.diags)
	}
	return nil
}

// Data returns a copy of the bytes the corpus document path was read with, nil when
// the corpus holds no document there: the write path checks a target against them
// and reads a file it rewrites outside the executor from them.
func (c *Corpus) Data(path string) []byte {
	if f := c.files[path]; f != nil && f.data != nil {
		return slices.Clone(f.data)
	}
	return nil
}

// Differs returns, sorted, the corpus paths at which o differs from c: each corpus
// document that one holds and the other does not or that the two hold with other
// bytes, and the file named by the diagnostic of each table whose unavailability
// differs — a walk entry has no bytes in the corpus, so its table's diagnostic stands
// for it. The write path reads it to know whether a corpus re-read under the lock is
// the one a transaction read, and to name a change that landed.
func (c *Corpus) Differs(o *Corpus) []string {
	set := map[string]bool{}
	for p, f := range c.files {
		if g := o.files[p]; g == nil || !bytes.Equal(f.data, g.data) || (f.data == nil) != (g.data == nil) {
			set[p] = true
		}
	}
	for p := range o.files {
		if c.files[p] == nil {
			set[p] = true
		}
	}
	for _, tb := range c.cat.Tables {
		a, aok := c.unavailable[tb.Name]
		b, bok := o.unavailable[tb.Name]
		if aok != bok || a != b {
			if aok {
				set[a.File] = true
			}
			if bok {
				set[b.File] = true
			}
		}
	}
	return sortedTables(set)
}

// cell is the value of a column of a row: NULL, or its canonical readable text.
type cell struct {
	null bool
	text string
}

// value returns the value of column col of row x. The key, the title, the body, a
// markdown section and the placement foreign key have their own places in a row;
// every other column is a stored scalar. A markdown value is its text without the
// line feed that ends its last line. A scalar that is not a value of its column type
// fails as CONSTRAINT, naming its type diagnostic: a request that evaluates such a
// value has no answer (spec 10 Readable values).
func (c *Corpus) value(x *rec, col string) (cell, *Error) {
	tb := x.tb
	r := x.row
	switch {
	case col == tb.Key:
		return cell{text: r.Key}, nil
	case col == tb.Title:
		return cell{text: r.Title}, nil
	case col == tb.Body:
		return cell{text: strings.TrimSuffix(r.Body, "\n")}, nil
	case isMarkdown(tb, col):
		v, ok := r.Sections[col]
		if !ok || v == nil {
			return cell{null: true}, nil
		}
		return cell{text: strings.TrimSuffix(*v, "\n")}, nil
	case tb.Storage.Kind == catalog.StorageBlock && col == placementColumn(tb):
		return cell{text: x.parent.row.Key}, nil
	case tb.Storage.Kind == catalog.StorageFiles && col == tb.Order:
		return c.position(x)
	}
	if _, raw := r.Raw[col]; raw {
		return cell{}, c.typeFault(x, col)
	}
	v := r.Scalars[col]
	if v == nil {
		return cell{null: true}, nil
	}
	return cell{text: *v}, nil
}

// typeFault is the CONSTRAINT error of a request that evaluates the value of col of
// row x, which is not a value of its column type, naming its type diagnostic.
func (c *Corpus) typeFault(x *rec, col string) *Error {
	line, column := x.row.At(col)
	d := catalog.Diagnostic{File: x.file, Line: line, Column: column, Rule: "type",
		Message: fmt.Sprintf("%s is not a value of type %s", col, columnOf(x.tb, col).Type.String())}
	return diagError("the request evaluates a value not of its column type", d)
}

// isMarkdown reports whether col is a markdown column of tb other than the body.
func isMarkdown(tb *catalog.Table, col string) bool {
	return col != tb.Body && slices.Contains(tb.Markdown, col)
}

// placementColumn returns the placement foreign key column of a block table.
func placementColumn(tb *catalog.Table) string {
	for _, fk := range tb.ForeignKeys {
		if fk.Name == tb.Storage.ParentForeignKey && len(fk.Columns) == 1 {
			return fk.Columns[0]
		}
	}
	return ""
}

// columnOf returns the column named col of tb; the request was validated against
// the runtime model, so every column it names exists.
func columnOf(tb *catalog.Table, col string) dbspec.Column {
	for _, c := range tb.Columns {
		if c.Name == col {
			return c
		}
	}
	panic("irexec: no column " + col + " in " + tb.Name)
}

// parentColumn returns the self foreign key "parent" of tb, "" when it has none.
func parentColumn(tb *catalog.Table) string { return tb.Hierarchy() }

// rows returns the rows of table in storage order (spec 20 Order): the pre-order of
// the tree of parents, each row's children in sibling order, and the rows of
// different placement parents in their parents' storage order. It fails as
// CONSTRAINT when the table is unavailable.
func (c *Corpus) rows(table string) ([]*rec, *Error) {
	if err := c.need(table); err != nil {
		return nil, err
	}
	if out, ok := c.ordered[table]; ok {
		return out, nil
	}
	tb, _ := c.cat.Table(table)
	var out []*rec
	switch tb.Storage.Kind {
	case catalog.StorageDocument:
		// The store returns a document's rows in storage order.
		out = slices.Clone(c.recs[table])
	case catalog.StorageBlock:
		parents, err := c.rows(tb.Storage.Parent)
		if err != nil {
			return nil, err
		}
		byParent := map[*store.Row][]*rec{}
		for _, x := range c.recs[table] {
			byParent[x.parent.row] = append(byParent[x.parent.row], x)
		}
		for _, p := range parents {
			// The store returns a block's rows in storage order.
			out = append(out, byParent[p.row]...)
		}
	case catalog.StorageFiles:
		o, err := c.filesOrder(tb)
		if err != nil {
			return nil, err
		}
		out = o
	}
	c.ordered[table] = out
	return out, nil
}

// filesOrder returns the rows of a files table in storage order (spec 20 Order). The
// tree of a files table runs through its files, so its order is computed over all
// its rows, and it is total whatever the validator reports on the rows: a row whose
// parent is NULL or names no row is a root; the roots stand by sibling group — the
// NULL parent group first, then each group of a parent value that names no row, in
// key order of that value — each root followed by its subtree in pre-order; the rows
// no root reaches (a parent not of its type, a cycle of parents across files, and
// every row under one of them) follow in corpus order. Within a sibling group, rows
// that share an order value stand in corpus order, and rows whose order value is
// not of its type stand after the others in corpus order.
func (c *Corpus) filesOrder(tb *catalog.Table) ([]*rec, *Error) {
	all := c.recs[tb.Name]
	parent := parentColumn(tb)
	keyInt := isInteger(columnOf(tb, tb.Key).Type.Kind)
	byKey := map[string]*rec{}
	for _, x := range all {
		byKey[x.row.Key] = x
	}
	children := map[*rec][]*rec{}
	// The root groups: the rows whose parent is NULL, then the rows of each parent
	// value that names no row.
	var nullRoots []*rec
	dangling := map[string][]*rec{}
	var danglingValues []string
	for _, x := range all {
		if parent == "" {
			nullRoots = append(nullRoots, x)
			continue
		}
		v, err := c.value(x, parent)
		if err != nil {
			continue // a parent not of its type: no root reaches the row
		}
		if v.null {
			nullRoots = append(nullRoots, x)
			continue
		}
		if p, ok := byKey[v.text]; ok {
			children[p] = append(children[p], x)
			continue
		}
		if _, seen := dangling[v.text]; !seen {
			danglingValues = append(danglingValues, v.text)
		}
		dangling[v.text] = append(dangling[v.text], x)
	}
	sort.Slice(danglingValues, func(i, j int) bool { return keyLess(keyInt, danglingValues[i], danglingValues[j]) })
	rootGroups := [][]*rec{nullRoots}
	for _, v := range danglingValues {
		rootGroups = append(rootGroups, dangling[v])
	}
	siblings := func(group []*rec) {
		if tb.Order == "" {
			sort.SliceStable(group, func(i, j int) bool { return keyLess(keyInt, group[i].row.Key, group[j].row.Key) })
			return
		}
		c.sortSiblings(tb, group)
	}
	for _, g := range children {
		siblings(g)
	}
	out := make([]*rec, 0, len(all))
	reached := make(map[*rec]bool, len(all))
	var walk func(x *rec)
	walk = func(x *rec) {
		out = append(out, x)
		reached[x] = true
		for _, ch := range children[x] {
			walk(ch)
		}
	}
	for _, group := range rootGroups {
		siblings(group)
		for _, r := range group {
			walk(r)
		}
	}
	var rest []*rec
	for _, x := range all {
		if !reached[x] {
			rest = append(rest, x)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].file < rest[j].file })
	return append(out, rest...), nil
}

// sortSiblings sorts a sibling group of a files table with an order column into its
// sibling order (spec 20 Order): by ascending rank, rows sharing a rank in corpus
// order, and the rows whose rank is not of its type after the others in corpus
// order.
func (c *Corpus) sortSiblings(tb *catalog.Table, group []*rec) {
	type rank struct {
		n  int64
		ok bool
	}
	at := make(map[*rec]rank, len(group))
	for _, x := range group {
		n, ok := c.orderValue(x, tb.Order)
		at[x] = rank{n, ok}
	}
	sort.SliceStable(group, func(i, j int) bool {
		a, b := at[group[i]], at[group[j]]
		switch {
		case a.ok != b.ok:
			return a.ok
		case a.ok && a.n != b.n:
			return a.n < b.n
		}
		return group[i].file < group[j].file
	})
}

// filesGroup names the sibling group of a row of a files table by its parent
// value; ok is false when the parent is not a value of its type, and the row then
// belongs to no known group.
func filesGroup(x *rec) (string, bool) {
	pc := parentColumn(x.tb)
	if pc == "" {
		return "", true
	}
	if _, raw := x.row.Raw[pc]; raw {
		return "", false
	}
	if v := x.row.Scalars[pc]; v != nil {
		return "=" + *v, true
	}
	return "null", true
}

// position returns the order value a read fills for row x of a files table: its
// 1-based position in its sibling order, which its stored rank gives (spec 20
// Order). A row whose rank, or whose parent, is not a value of its type fails as
// CONSTRAINT naming that type diagnostic: the rank has no comparison, and without
// its parent the row's sibling group is not known.
func (c *Corpus) position(x *rec) (cell, *Error) {
	tb := x.tb
	if _, raw := x.row.Raw[tb.Order]; raw {
		return cell{}, c.typeFault(x, tb.Order)
	}
	if _, ok := filesGroup(x); !ok {
		return cell{}, c.typeFault(x, parentColumn(tb))
	}
	at, ok := c.positions[tb.Name]
	if !ok {
		groups := map[string][]*rec{}
		var keys []string
		for _, y := range c.recs[tb.Name] {
			g, ok := filesGroup(y)
			if !ok {
				continue
			}
			if _, seen := groups[g]; !seen {
				keys = append(keys, g)
			}
			groups[g] = append(groups[g], y)
		}
		at = map[*rec]int64{}
		for _, g := range keys {
			members := groups[g]
			c.sortSiblings(tb, members)
			for i, y := range members {
				at[y] = int64(i + 1)
			}
		}
		c.positions[tb.Name] = at
	}
	return cell{text: strconv.FormatInt(at[x], 10)}, nil
}

// orderOf returns the stored order value of row x: its rank in files storage, its
// position in the others.
func (c *Corpus) orderOf(x *rec, col string) (int64, *Error) {
	n, ok := c.orderValue(x, col)
	if !ok {
		return 0, c.typeFault(x, col)
	}
	return n, nil
}

// orderValue returns the stored order value of row x; ok is false when it is not a
// value of its type.
func (c *Corpus) orderValue(x *rec, col string) (int64, bool) {
	if _, raw := x.row.Raw[col]; raw {
		return 0, false
	}
	v := x.row.Scalars[col]
	if v == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(*v, 10, 64)
	return n, err == nil
}

// keyLess orders two canonical keys: numerically for an integer key, else by code
// point, which is byte order in UTF-8.
// integerKey is the value of the key k of a table with integer keys, which the read of the corpus validated as an integer.
func integerKey(k string) int64 {
	n, err := strconv.ParseInt(k, 10, 64)
	if err != nil {
		panic("irexec: the integer key " + strconv.Quote(k) + " is no integer: " + err.Error())
	}
	return n
}

func keyLess(integer bool, a, b string) bool {
	if integer {
		return integerKey(a) < integerKey(b)
	}
	return a < b
}

// isInteger reports whether k is an integer type.
func isInteger(k dbspec.TypeKind) bool {
	return k == dbspec.TypeI16 || k == dbspec.TypeI32 || k == dbspec.TypeI64
}
