package irexec

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/internal/pmap"
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
	cat *catalog.Catalog
	// files holds the corpus documents by corpus path, shared with the corpora read or written from this one.
	files       pmap.Map[string, *file]
	unavailable map[string]catalog.Diagnostic
	// walk holds the first corpus.walk diagnostic of each entry at a table's place
	// that is no corpus document, by its corpus path.
	walk pmap.Map[string, catalog.Diagnostic]
	// idx is the index of the rows (index.go), the locators of each key by the document that holds them, and keys the
	// index keys of each document's rows, both shared with the corpora read or written from this one.
	idx  pmap.Map[string, byDoc]
	keys pmap.Map[string, docKeys]
	// docs holds the records of each document this corpus built, guarded by docsMu, since the requests that read a
	// committed corpus may run at once.
	docs map[string]*docRecs
	// dirty marks the documents whose rows changed since they entered the index (docs.go).
	dirty map[string]*mark
	// ownKeys holds the documents whose index keys this corpus made and shares with no other corpus.
	ownKeys map[string]bool
	docsMu  sync.Mutex
	// ordered caches the storage order of each table.
	ordered map[string][]*rec
	// cost counts the work of the corpus's operations; nil counts nothing.
	cost *cost.Counts
	// seq holds the AUTOINCREMENT counters of the corpus (sequences.go), nil when it holds none.
	seq *sequences
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
	// at is the place of the record in the walk of its document, gen the walk that placed it, moved the walk that found it
	// new or under another parent, and again the walk that entered it in the index again (docs.go).
	at, gen, moved, again int
}

// Open reads the corpus fsys as a request reads it, from nothing: the corpus folder itself (store.DirFS), or the
// write path's mirror of it. An error is returned only when the corpus root cannot be read.
func Open(cat *catalog.Catalog, fsys store.FS) (*Corpus, error) {
	s, err := validate.Read(cat, fsys)
	if err != nil {
		return nil, err
	}
	data, err := fsys.ReadFile(catalog.SequencesFile)
	if errors.Is(err, fs.ErrNotExist) {
		data, err = nil, nil
	}
	if err != nil {
		return nil, err
	}
	c := FromSnapshot(cat, s, nil, nil)
	if err := c.SetSequences(data); err != nil {
		return nil, err
	}
	return c, nil
}

// FromSnapshot returns the corpus of the read s of the catalog cat, counting its work in k. When prev is the corpus of
// the read before s of the same validate.Live, under the same catalog, the corpus shares every document of prev that
// the read did not change, and sets only the paths it did (plan, Ruling: an operation costs what it touches).
func FromSnapshot(cat *catalog.Catalog, s *validate.Snapshot, prev *Corpus, k *cost.Counts) *Corpus {
	c := &Corpus{cat: cat, unavailable: map[string]catalog.Diagnostic{}, cost: k}
	if prev != nil && prev.cat == cat {
		// The counters are no document of the read: the reader sets them again when their file changed (SetSequences).
		c.seq = prev.seq
	}
	changed, full := s.Changed()
	if prev == nil || prev.cat != cat || full {
		for path := range s.Files() {
			c.readPath(s, path)
		}
		for path := range s.AllDiagnostics() {
			c.readPath(s, path)
		}
	} else {
		c.files, c.walk = prev.files, prev.walk
		prev.docsMu.Lock()
		c.idx, c.keys = prev.shareKeysLocked()
		for p := range prev.dirty {
			c.markLocked(p).walk = true
		}
		prev.docsMu.Unlock()
		for _, path := range changed {
			c.readPath(s, path)
		}
	}
	for _, tb := range cat.Tables {
		if d, ok := s.Unavailable(tb.Name); ok {
			c.unavailable[tb.Name] = d
		}
	}
	return c
}

// readPath sets the document and the walk entry of the corpus path from the read s: the document with its
// diagnostics, or, for a path that is no document, the first corpus.walk diagnostic of the entry.
func (c *Corpus) readPath(s *validate.Snapshot, path string) {
	diags := s.DiagnosticsAt(path)
	c.walk = c.walk.Delete(path)
	if f := s.File(path); f != nil {
		c.setFile(path, &file{table: f.Table, data: f.Data, rows: f.Rows, diags: diags})
		return
	}
	c.dropFile(path)
	for _, d := range diags {
		if d.Rule == store.RuleWalk {
			c.walk = c.walk.Set(path, d)
			return
		}
	}
}

// Cost returns the counts of the work of the corpus's operations, nil when it counts none.
func (c *Corpus) Cost() *cost.Counts { return c.cost }

// FilesOf returns the paths of the files that hold rows of table.
func (c *Corpus) FilesOf(table string) ([]string, error) {
	var out []string
	for p, f := range c.files.All() {
		if f.table == table {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Rebased returns the corpus under the catalog cat of the schema statements that stand next, without the files of drop:
// the tables a schema statement changed, whose rows come again from the statement's database.
func (c *Corpus) Rebased(cat *catalog.Catalog, drop []string) *Corpus {
	n := &Corpus{cat: cat, files: c.files, walk: c.walk, unavailable: map[string]catalog.Diagnostic{}, cost: c.cost, seq: c.seq}
	for _, p := range drop {
		n.dropFile(p)
	}
	// The index keys of a row follow its table's declaration, so the index under cat is built from nothing.
	for p := range n.files.All() {
		n.changed(p)
	}
	return n
}

// reorder drops the cached storage orders, which a changed order value or a changed
// set of rows invalidates.
func (c *Corpus) reorder() {
	c.ordered = map[string][]*rec{}
}

// file returns the corpus document at path, nil when the corpus holds none.
func (c *Corpus) file(path string) *file {
	f, _ := c.files.Get(path)
	return f
}

// setFile makes f the document at path of c, a corpus that a read or a write is computing and no other reads, and
// marks it.
func (c *Corpus) setFile(path string, f *file) {
	c.files = c.files.Set(path, f)
	c.changed(path)
}

// dropFile removes the document at path from c, a corpus that a read or a write is computing and no other reads, and
// marks it.
func (c *Corpus) dropFile(path string) {
	c.files = c.files.Delete(path)
	c.changed(path)
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
	return c.tableRows(table, func(x *rec, col dbspec.Column) (any, error) {
		v, err := c.value(x, col.Name)
		if err != nil {
			return nil, err
		}
		return cellOutput(col, v)
	})
}

// StoredRows returns every row of table as Rows does, each value that is not of its column's type as the file holds it: a
// store.Literal, or the text. The database of a SQL statement holds them as a statement stored them (spec 40 SQL statements).
func (c *Corpus) StoredRows(table string) ([][]any, error) { return c.tableRows(table, c.storedValue) }

// storedValue is the stored value of column col of x, as a database of the corpus holds it.
func (c *Corpus) storedValue(x *rec, col dbspec.Column) (any, error) {
	if raw, ok := x.row.Raw[col.Name]; ok {
		if store.IsLiteral(raw) {
			return store.Literal(raw), nil
		}
		return raw, nil
	}
	v, err := c.value(x, col.Name)
	if err != nil {
		return nil, err
	}
	if !v.null && (col.Type.Kind == dbspec.TypeDatetime || col.Type.Kind == dbspec.TypeTime || col.Type.Kind == dbspec.TypeDate) {
		// SQLite holds these as text, at the precision of the type, which is how the file holds them.
		return v.text, nil
	}
	return cellOutput(col, v)
}

// cellOutput is the wire value of v in column col, its failure an error only when there is one: output answers a typed
// *Error, which as an error is never nil.
func cellOutput(col dbspec.Column, v cell) (any, error) {
	w, err := output(col.Type, v)
	if err != nil {
		return nil, err
	}
	return w, nil
}

// tableRows returns every row of table in storage order, each the cells that of gives for the columns of the table in
// declaration order; it fails as ENTITY_UNKNOWN when the catalog has no such table.
func (c *Corpus) tableRows(table string, of func(*rec, dbspec.Column) (any, error)) ([][]any, error) {
	if _, ok := c.cat.Table(table); !ok {
		return nil, errf(codeEntityUnknown, "the catalog has no table %s", table)
	}
	recs, err := c.rows(table)
	if err != nil {
		return nil, err
	}
	out := make([][]any, 0, len(recs))
	c.cost.Add(cost.RowsStored, len(recs))
	for _, x := range recs {
		row := make([]any, len(x.tb.Columns))
		for i, col := range x.tb.Columns {
			w, err := of(x, col)
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
	if f := c.file(path); f != nil {
		return slices.Clone(f.diags)
	}
	return nil
}

// Data returns a copy of the bytes the corpus document path was read with, nil when
// the corpus holds no document there: the write path checks a target against them
// and reads a file it rewrites outside the executor from them.
func (c *Corpus) Data(path string) []byte {
	if path == catalog.SequencesFile {
		return slices.Clone(c.sequenceData())
	}
	if f := c.file(path); f != nil && f.data != nil {
		return slices.Clone(f.data)
	}
	return nil
}

// Differs returns, sorted, the corpus paths at which o differs from c: each corpus
// document that one holds and the other does not or that the two hold with other
// bytes, and the file named by the diagnostic of each table whose unavailability
// differs — a walk entry has no bytes in the corpus, so its table's diagnostic stands
// for it —, the file of the AUTOINCREMENT counters when theirs differ, and the file of
// the schema statements when their catalogs differ. The write path reads it to know whether a corpus re-read under the lock is
// the one a transaction read, and to name a change that landed. The documents the two
// corpora share are not compared, so corpora read or written one from the other cost
// the paths they changed (plan, Ruling: an operation costs what it touches).
func (c *Corpus) Differs(o *Corpus) []string {
	set := map[string]bool{}
	same := func(f, g *file) bool {
		return f == g || bytes.Equal(f.data, g.data) && (f.data == nil) == (g.data == nil)
	}
	c.cost.Add(cost.FilesCompared, pmap.Diff(c.files, o.files, same, func(p string) { set[p] = true }))
	if a, b := c.sequenceData(), o.sequenceData(); !bytes.Equal(a, b) || (a == nil) != (b == nil) {
		set[catalog.SequencesFile] = true
	}
	// Corpora under two catalogs differ at the schema statements, which give a corpus its catalog.
	if c.cat != o.cat {
		set[catalog.StatementsFile] = true
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
	// raw is a text that is no value of its column's type, which a statement of SQL may store: SQLite's types do not
	// bind a column, and the file holds it as written.
	raw bool
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
	if i := tb.KeyPart(col); i >= 0 {
		values := catalog.SplitKey(r.Key)
		if i >= len(values) {
			return cell{}, errf(codeConstraint, "the key %s of %s has no value of %s", r.Key, tb.Name, col)
		}
		return cell{text: values[i]}, nil
	}
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
	case tb.Storage.Kind == catalog.StorageBlock && col == tb.Placement():
		return cell{text: x.parent.row.Key}, nil
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
		out = slices.Clone(c.tableRecs(table))
	case catalog.StorageBlock:
		parents, err := c.rows(tb.Storage.Parent)
		if err != nil {
			return nil, err
		}
		byParent := map[*store.Row][]*rec{}
		for _, x := range c.tableRecs(table) {
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
	all := c.tableRecs(tb.Name)
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
		c.sortSiblings(tb, keyInt, group)
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
// sibling order (spec 20 Order): by ascending value, the rows whose value is not of
// its type after the others, and the rows that share a value, or whose value is not
// of its type, in key order; keyInt reports an integer key.
func (c *Corpus) sortSiblings(tb *catalog.Table, keyInt bool, group []*rec) {
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
		return keyLess(keyInt, group[i].row.Key, group[j].row.Key)
	})
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
