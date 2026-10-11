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
	"sort"

	"github.com/polyspec/flowmark/catalog"
)

// Corpus validates the corpus at root against cat and returns its diagnostics,
// sorted by file, line, column, rule and message. A diagnostic that a rule reports
// over an unavailable table is not returned; the table's own diagnostic stands for
// it. An error is returned only when the corpus root cannot be read.
func Corpus(cat *catalog.Catalog, root string) ([]catalog.Diagnostic, error) {
	first, err := read(cat, root, noTargets{})
	if err != nil {
		return nil, err
	}
	idx, err := indexOf(cat, first)
	if err != nil {
		return nil, err
	}
	c, err := read(cat, root, idx)
	if err != nil {
		return nil, err
	}
	c.rules()
	sort.SliceStable(c.diags, func(i, j int) bool {
		a, b := c.diags[i], c.diags[j]
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
	return c.diags, nil
}
