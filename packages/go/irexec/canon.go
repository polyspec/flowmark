package irexec

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
)

// CanonTarget is a corpus document that flowmark canon rewrites: its path and its canonical bytes, which differ from
// the bytes it was read with.
type CanonTarget struct {
	Path  string
	After []byte
}

// CanonReport is a corpus document or a corpus.walk entry that flowmark canon with no file named leaves as it is
// (spec 40 The canon operation). Diagnostic is its own diagnostic that canon cannot repair; when Needs is set, it is
// the diagnostic that makes the table Needs unavailable, a block or document table that a link of the file names.
type CanonReport struct {
	Path       string
	Needs      string
	Diagnostic catalog.Diagnostic
}

// Canon computes flowmark canon on the corpus (spec 40 The canon operation): the canonical bytes of each target whose
// bytes they change, sorted by path, and, with no file named, the reports of the files it leaves, sorted by path. The
// targets are the named files, each a corpus path, or with none named every corpus document. A named path that is
// neither a corpus document nor a corpus.walk entry is CONFIG; a named file that canon cannot rewrite — a corpus.walk
// entry, a document with format.block, format.section, format.encoding or a parent.cycle among its rows, a document
// with a link to a row of an unavailable block or document table — is CONSTRAINT naming each such file and its
// diagnostic. A target is rendered as its read rendered it, so canon changes no logical value.
func (c *Corpus) Canon(named []string) ([]CanonTarget, []CanonReport, *Error) {
	var paths []string
	if len(named) == 0 {
		for p := range c.files {
			paths = append(paths, p)
		}
	} else {
		for _, p := range named {
			if !slices.Contains(paths, p) {
				paths = append(paths, p)
			}
		}
	}
	sort.Strings(paths)
	var unknown []string
	for _, p := range paths {
		if c.files[p] == nil && !hasKey(c.walk, p) {
			unknown = append(unknown, fmt.Sprintf("%q", p))
		}
	}
	if len(unknown) > 0 {
		return nil, nil, errf(codeConfig, "flowmark canon names %s, which is no corpus document: name each file by its path relative to the corpus root, with / between its parts; nothing was written", strings.Join(unknown, ", "))
	}
	var reports []CanonReport
	if len(named) == 0 {
		for p, d := range c.walk {
			reports = append(reports, CanonReport{Path: p, Diagnostic: d})
		}
	}
	var targets []CanonTarget
	var lk *linker
	for _, p := range paths {
		if rp, left := c.canonLeaves(p); left {
			reports = append(reports, rp)
			continue
		}
		if lk == nil {
			l, err := newLinker(c)
			if err != nil {
				return nil, nil, errf(codeInternal, "%v", err)
			}
			lk = l
		}
		out, err := c.canonical(p, lk)
		if err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(out, c.files[p].data) {
			targets = append(targets, CanonTarget{Path: p, After: out})
		}
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Path < reports[j].Path })
	if len(named) > 0 && len(reports) > 0 {
		return nil, nil, canonRefusal(reports)
	}
	return targets, reports, nil
}

// hasKey reports whether m holds k.
func hasKey(m map[string]catalog.Diagnostic, k string) bool {
	_, ok := m[k]
	return ok
}

// canonLeaves reports why canon cannot rewrite the corpus path p, false when it can: p is a corpus.walk entry, or a
// document with a diagnostic of a file it cannot read — format.block, format.section, format.encoding, a parent.cycle
// among its rows — or with a link that needs an unavailable table.
func (c *Corpus) canonLeaves(p string) (CanonReport, bool) {
	f := c.files[p]
	if f == nil {
		return CanonReport{Path: p, Diagnostic: c.walk[p]}, true
	}
	for _, d := range f.diags {
		switch d.Rule {
		case store.RuleBlock, store.RuleSection, store.RuleEncoding, store.RuleParentCycle, store.RuleWalk:
			return CanonReport{Path: p, Diagnostic: d}, true
		}
	}
	for _, t := range c.linkTables(f) {
		if d, ok := c.unavailable[t]; ok {
			return CanonReport{Path: p, Needs: t, Diagnostic: d}, true
		}
	}
	return CanonReport{}, false
}

// linkTables returns, sorted, the tables whose rows the links of file f need for their derived targets: the table of
// each block or document row a written foreign key of its rows names. A link to a files row derives from its key
// alone and needs no table, and the placement foreign key of a block row is never written (spec 10 Foreign keys as
// links).
func (c *Corpus) linkTables(f *file) []string {
	tables := map[string]bool{}
	var visit func(r *store.Row)
	visit = func(r *store.Row) {
		tb, _ := c.cat.Table(r.Table)
		placement := ""
		if tb.Storage.Kind == catalog.StorageBlock {
			placement = placementColumn(tb)
		}
		for _, fk := range tb.ForeignKeys {
			col := fk.Columns[0]
			if col == placement || r.Scalars[col] == nil {
				continue
			}
			if ref, _ := c.cat.Table(fk.Table); ref.Storage.Kind != catalog.StorageFiles {
				tables[fk.Table] = true
			}
		}
		for _, n := range sortedBlocks(r) {
			for _, b := range r.Blocks[n] {
				visit(b)
			}
		}
	}
	for _, r := range f.rows {
		visit(r)
	}
	return sortedTables(tables)
}

// sortedBlocks returns the names of the child blocks of row r, sorted.
func sortedBlocks(r *store.Row) []string {
	names := make([]string, 0, len(r.Blocks))
	for n := range r.Blocks {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// canonical renders the corpus document p in its canonical form, as its read rendered it, with the link targets lk
// derives. The rendered bytes must read back without a diagnostic: anything else is internal, since the read of the
// file accepted its rows.
func (c *Corpus) canonical(p string, lk *linker) ([]byte, *Error) {
	f := c.files[p]
	tb, _ := c.cat.Table(f.table)
	var out []byte
	var rf []store.Refusal
	var err error
	if tb.Storage.Kind == catalog.StorageFiles {
		if len(f.rows) != 1 {
			return nil, errf(codeInternal, "the files document %s was read with %d rows, not one", p, len(f.rows))
		}
		out, rf, err = store.CanonicalFile(c.cat, f.table, p, f.rows[0], lk)
	} else {
		out, rf, err = store.CanonicalDocument(c.cat, f.table, p, f.rows, lk)
	}
	switch {
	case err != nil:
		return nil, errf(codeInternal, "rendering %s in its canonical form: %v", p, err)
	case len(rf) > 0:
		return nil, errf(codeInternal, "the canonical form of %s, whose rows its read accepted, refuses: %s %s, column %s: %s", p, rf[0].Table, rf[0].Key, rf[0].Column, rf[0].Message)
	}
	var diags []catalog.Diagnostic
	if tb.Storage.Kind == catalog.StorageFiles {
		_, diags = store.ParseFile(c.cat, f.table, p, out, lk)
	} else {
		_, ds, perr := store.ParseDocument(c.cat, f.table, p, out, lk)
		if perr != nil {
			return nil, errf(codeInternal, "reading back the canonical form of %s: %v", p, perr)
		}
		diags = ds
	}
	if len(diags) > 0 {
		d := diags[0]
		return nil, errf(codeInternal, "the canonical form of %s does not read back clean: %d:%d %s: %s", p, d.Line, d.Column, d.Rule, d.Message)
	}
	return out, nil
}

// canonRefusal is the CONSTRAINT of a named-file canon that names files it cannot rewrite: each file and its
// diagnostic, and for a link the table it needs; it carries the first diagnostic.
func canonRefusal(reports []CanonReport) *Error {
	msgs := make([]string, len(reports))
	for i, rp := range reports {
		d := rp.Diagnostic
		at := fmt.Sprintf("%s:%d:%d %s: %s", d.File, d.Line, d.Column, d.Rule, d.Message)
		if rp.Needs != "" {
			msgs[i] = fmt.Sprintf("%s holds a link to a row of the table %s, which is unavailable (%s), so its derived target is unknown", rp.Path, rp.Needs, at)
		} else {
			msgs[i] = fmt.Sprintf("%s cannot be read as rows (%s)", rp.Path, at)
		}
	}
	d := reports[0].Diagnostic
	return &Error{Code: codeConstraint, Diagnostic: &d,
		Msg: "flowmark canon cannot rewrite the files named: " + strings.Join(msgs, "; ") + "; repair each by editing the corpus, then run canon again; nothing was written"}
}
