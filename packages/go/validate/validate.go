// Package validate reports the diagnostics of a corpus over its catalog: the
// walk, the format rules of the canonical format, and the validator rules of the
// constraints (spec 10 Diagnostics and spec 50 Diagnostics).
//
// The rows of a corpus are read twice. The first pass locates every row, and the
// second reads the corpus with the index of the first, so that each foreign key
// is checked against the rows the corpus holds and each link target is derived
// the way the store writes it.
package validate

import (
	"fmt"
	"sort"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
)

// Corpus validates the corpus at root against cat and returns its diagnostics,
// sorted by file, line, column, rule and message. A diagnostic that a rule reports
// over an unavailable table is not returned; the table's own diagnostic stands for
// it. An error is returned only when the corpus root cannot be read, or when orm
// builds no runtime model of the loaded catalog that the view rule needs.
func Corpus(cat *catalog.Catalog, root string) ([]catalog.Diagnostic, error) {
	c, err := load(cat, store.DirFS(root), nil)
	if err != nil {
		return nil, err
	}
	if err := c.rules(); err != nil {
		return nil, err
	}
	sortDiags(c.diags)
	return c.diags, nil
}

// load reads the corpus fsys in its two passes: the first locates the rows,
// the second reads them with the index of the first. Each pass parses through its
// memo of m; a nil m parses every document.
func load(cat *catalog.Catalog, fsys store.FS, m *Memo) (*corpus, error) {
	var first, second *store.Memo
	if m != nil {
		first, second = m.first, m.second
	}
	first.Begin(cat)
	c, err := read(cat, fsys, noTargets{}, first)
	if err != nil {
		return nil, err
	}
	first.End()
	idx, err := indexOf(cat, c)
	if err != nil {
		return nil, err
	}
	second.Begin(cat)
	c, err = read(cat, fsys, idx, second)
	if err != nil {
		return nil, err
	}
	second.End()
	return c, nil
}

// Memo keeps the parses of both passes of a read across the reads of a
// long-lived corpus (store.Memo): a read parses only the documents whose parse
// can differ from the last read's.
type Memo struct {
	first, second *store.Memo
}

// NewMemo returns an empty memo.
func NewMemo() *Memo { return &Memo{first: store.NewMemo(), second: store.NewMemo()} }

// sortDiags sorts diagnostics by file, line, column, rule and message.
func sortDiags(diags []catalog.Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
}

// Snapshot is a corpus read once, as a request reads it: every corpus document at
// the place of a table with its bytes and rows, the read diagnostics (the walk and
// the format rules), and the unavailable tables (spec 10 Diagnostics, "Unavailable
// table"). It holds no validator rule but the byte-for-byte key duplicate that makes
// a table unavailable.
type Snapshot struct {
	// Files holds each corpus document read at the place of a table, by corpus path.
	Files map[string]*File
	// Diagnostics holds the walk and format diagnostics of the read, sorted.
	Diagnostics []catalog.Diagnostic
	// unavailable holds, for each unavailable table, the diagnostic a request
	// that needs its rows names.
	unavailable map[string]catalog.Diagnostic
}

// Read reads the corpus fsys as a request reads it, parsing through m (nil to
// parse every document). An error is returned only when the corpus root cannot be
// read.
func Read(cat *catalog.Catalog, fsys store.FS, m *Memo) (*Snapshot, error) {
	c, err := load(cat, fsys, m)
	if err != nil {
		return nil, err
	}
	sortDiags(c.diags)
	s := &Snapshot{Files: c.files, Diagnostics: c.diags, unavailable: map[string]catalog.Diagnostic{}}
	for _, tb := range cat.Tables {
		if d, ok := c.unavailableBy(tb.Name); ok {
			s.unavailable[tb.Name] = d
		}
	}
	return s, nil
}

// Unavailable returns the diagnostic that makes table unavailable, and false when
// the table is available.
func (s *Snapshot) Unavailable(table string) (catalog.Diagnostic, bool) {
	d, ok := s.unavailable[table]
	return d, ok
}

// unavailableBy returns the diagnostic that makes table unavailable: the first, in
// diagnostic order, of a file with format.block, format.section, format.encoding or
// a parent.cycle among its rows, or of a corpus.walk entry, at its place or at the
// place of its placement parent's table, recursively; otherwise, when two of its
// rows have byte-for-byte equal keys, the key.duplicate of the second row in corpus
// order, at its file.
func (c *corpus) unavailableBy(table string) (catalog.Diagnostic, bool) {
	for _, d := range c.diags {
		if !blocksRule(d.Rule) {
			continue
		}
		if t, ok := tableAt(c.cat, d.File); ok && c.holds(table, t) {
			return d, true
		}
	}
	tb, ok := c.cat.Table(table)
	if !ok {
		return catalog.Diagnostic{}, false
	}
	first := map[string]placed{}
	for _, p := range corpusOrder(c.rows[table]) {
		f, seen := first[p.row.Key]
		if !seen {
			first[p.row.Key] = p
			continue
		}
		line, col := p.row.At(tb.Key)
		return catalog.Diagnostic{File: p.file, Line: line, Column: col, Rule: ruleDuplicate,
			Message: fmt.Sprintf("the key %s equals the key %s of %s byte for byte", p.row.Key, f.row.Key, f.file)}, true
	}
	return catalog.Diagnostic{}, false
}

// holds reports whether table holds the files at the place of placeTable: the table
// itself, or a block table whose placement chain reaches it.
func (c *corpus) holds(table, placeTable string) bool {
	for name := table; ; {
		if name == placeTable {
			return true
		}
		tb, ok := c.cat.Table(name)
		if !ok || tb.Storage.Kind != catalog.StorageBlock {
			return false
		}
		name = tb.Storage.Parent
	}
}
