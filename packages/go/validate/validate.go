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
	"iter"
	"sort"

	"github.com/polyspec/flowmark/packages/go/internal/pmap"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
)

// Corpus validates the corpus at root against cat and returns its diagnostics,
// sorted by file, line, column, rule and message. A diagnostic that a rule reports
// over an unavailable table is not returned; the table's own diagnostic stands for
// it. An error is returned only when the corpus root cannot be read, or when orm
// builds no runtime model of the loaded catalog that the view rule needs.
func Corpus(cat *catalog.Catalog, root string) ([]catalog.Diagnostic, error) {
	c, err := load(cat, store.DirFS(root))
	if err != nil {
		return nil, err
	}
	if err := c.rules(); err != nil {
		return nil, err
	}
	sortDiags(c.diags)
	return c.diags, nil
}

// load reads the corpus fsys in its two passes: the first locates the rows, the second reads them with the index of
// the first. It is the read from nothing, which validate.Corpus reports on and every Live read equals.
func load(cat *catalog.Catalog, fsys store.FS) (*corpus, error) {
	c, err := read(cat, fsys, noTargets{})
	if err != nil {
		return nil, err
	}
	idx, err := indexOf(cat, c)
	if err != nil {
		return nil, err
	}
	return read(cat, fsys, idx)
}

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

// Snapshot is a corpus read as a request reads it: every corpus document at the
// place of a table with its bytes and rows, the read diagnostics (the walk and
// the format rules) by file, and the unavailable tables (spec 10 Diagnostics,
// "Unavailable table"). It holds no validator rule but the byte-for-byte key
// duplicate that makes a table unavailable. Its maps are shared with the
// snapshots read before and after it, and never changed.
type Snapshot struct {
	files pmap.Map[string, *File]
	diags pmap.Map[string, []catalog.Diagnostic]
	// unavailable holds, for each unavailable table, the diagnostic a request
	// that needs its rows names.
	unavailable map[string]catalog.Diagnostic
	// full is whether the read read the whole corpus, and changed the paths whose
	// file or diagnostics changed since the snapshot before, when it did not.
	full    bool
	changed []string
}

// Read reads the corpus fsys as a request reads it, from nothing. An error is
// returned only when the corpus root cannot be read.
func Read(cat *catalog.Catalog, fsys store.FS) (*Snapshot, error) {
	return NewLive(cat, nil).Read(fsys, nil, true)
}

// File returns the corpus document at path, nil when the corpus holds none.
func (s *Snapshot) File(path string) *File {
	f, _ := s.files.Get(path)
	return f
}

// Files yields every corpus document by corpus path, in byte order of the paths.
func (s *Snapshot) Files() iter.Seq2[string, *File] { return s.files.All() }

// DiagnosticsAt returns the read diagnostics at the corpus path file, sorted.
func (s *Snapshot) DiagnosticsAt(file string) []catalog.Diagnostic {
	d, _ := s.diags.Get(file)
	return d
}

// AllDiagnostics yields the read diagnostics of each corpus path that has any, by path in byte order, each sorted.
func (s *Snapshot) AllDiagnostics() iter.Seq2[string, []catalog.Diagnostic] { return s.diags.All() }

// Diagnostics returns every read diagnostic, sorted.
func (s *Snapshot) Diagnostics() []catalog.Diagnostic {
	var out []catalog.Diagnostic
	for _, ds := range s.diags.All() {
		out = append(out, ds...)
	}
	return out
}

// Changed returns the paths whose document or diagnostics changed since the
// snapshot the same Live read before, and false; or true when the read read the
// whole corpus, so every path may have changed.
func (s *Snapshot) Changed() ([]string, bool) { return s.changed, s.full }

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
func (c *corpus) holds(table, placeTable string) bool { return holds(c.cat, table, placeTable) }

// holds reports whether table of cat holds the files at the place of placeTable: the table itself, or a block table
// whose placement chain reaches it.
func holds(cat *catalog.Catalog, table, placeTable string) bool {
	for name := table; ; {
		if name == placeTable {
			return true
		}
		tb, ok := cat.Table(name)
		if !ok || tb.Storage.Kind != catalog.StorageBlock {
			return false
		}
		name = tb.Storage.Parent
	}
}
