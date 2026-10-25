package validate

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/orm/packages/go/engine/runtimemodel"
)

// placed is a row with the corpus file that holds it and, for a block row, its
// placement parent row, nil for any other row: a block row is held by the file of
// its placement row, and its placement foreign key is that row's key.
type placed struct {
	row    *store.Row
	file   string
	parent *store.Row
}

// corpus is the corpus read under one link target function.
type corpus struct {
	cat   *catalog.Catalog
	diags []catalog.Diagnostic
	// rows holds the rows of each table in appearance order, the block rows of
	// a table included under the table's name.
	rows map[string][]placed
	// blocked names the tables that hold an unreadable file or a corpus.walk
	// entry at their place: a table whose rows are not all readable.
	blocked map[string]bool
	// files holds each corpus document read at the place of a table, by corpus
	// path: its table, its bytes and its top-level rows.
	files map[string]*File
	// model is orm's runtime model of the catalog, built for the view rule when a
	// view row is judged.
	model *runtimemodel.Model
}

// File is a corpus document read at the place of a files or document table: the
// table whose place it is, the bytes read, and its rows of that table in the order
// the store returns them, each with its block rows. A file that could not be read
// as rows holds none.
type File struct {
	Table string
	Data  []byte
	Rows  []*store.Row
}

// noTargets is the link target function of the first pass: it knows no row, so
// every foreign key is kept as written, and the first pass only locates the rows.
type noTargets struct{}

func (noTargets) Target(fromFile, table, key string) (string, error) {
	return "", fmt.Errorf("no %s row is known to the first pass", table)
}

// read reads the corpus fsys by storage, and reports the walk and the format
// diagnostics of its files. A row of a block table is read with its placement row.
func read(cat *catalog.Catalog, fsys store.FS, lk store.Linker, memo *store.Memo) (*corpus, error) {
	entries, err := fsys.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("cannot read the corpus root: %w", err)
	}
	c := &corpus{cat: cat, rows: map[string][]placed{}, blocked: map[string]bool{}, files: map[string]*File{}}
	c.diags = append(c.diags, walkRoot(cat, fsys, entries)...)
	for _, tb := range cat.Tables {
		switch tb.Storage.Kind {
		case catalog.StorageFiles:
			if !listed(entries, tb.Name) {
				continue
			}
			rows, data, ds := store.ParseFilesTable(cat, fsys, tb.Name, lk, memo)
			c.diags = append(c.diags, ds...)
			for path, b := range data {
				c.files[path] = &File{Table: tb.Name, Data: b}
			}
			for _, r := range rows {
				path := r.Path
				if f := c.files[path]; f != nil {
					f.Rows = append(f.Rows, r)
				}
				c.place(r, path, nil)
			}
		case catalog.StorageDocument:
			if tb.Storage.Shard != nil {
				if !listed(entries, tb.Name) {
					continue
				}
				docs, ds, err := store.ParseDocumentTable(cat, fsys, tb.Name, lk, memo)
				if err != nil {
					return nil, err
				}
				c.diags = append(c.diags, ds...)
				for _, d := range docs {
					c.files[d.Path] = &File{Table: tb.Name, Data: d.Data, Rows: d.Rows}
					for _, r := range d.Rows {
						c.place(r, d.Path, nil)
					}
				}
				continue
			}
			file := tb.Name + docSuffix
			if !regularListed(entries, file) {
				continue
			}
			data, err := fsys.ReadFile(file)
			if err != nil {
				c.diags = append(c.diags, walkDiag(file, "cannot read the document: "+err.Error()))
				continue
			}
			rows, ds, err := memo.ParseDocument(cat, tb.Name, file, data, lk)
			if err != nil {
				return nil, err
			}
			c.diags = append(c.diags, ds...)
			c.files[file] = &File{Table: tb.Name, Data: data, Rows: rows}
			for _, r := range rows {
				c.place(r, file, nil)
			}
		}
	}
	c.markBlocked()
	return c, nil
}

// listed reports whether the root holds the exact name.
func listed(entries []fs.DirEntry, name string) bool {
	for _, e := range entries {
		if e.Name() == name {
			return true
		}
	}
	return false
}

// regularListed reports whether the root holds the exact name as a regular file.
func regularListed(entries []fs.DirEntry, name string) bool {
	for _, e := range entries {
		if e.Name() == name {
			return e.Type().IsRegular()
		}
	}
	return false
}

// place records row r, whose placement parent row is parent (nil for none), and
// its block rows, held by file, in their tables.
func (c *corpus) place(r *store.Row, file string, parent *store.Row) {
	c.rows[r.Table] = append(c.rows[r.Table], placed{row: r, file: file, parent: parent})
	for _, name := range r.BlockNames() {
		for _, child := range r.Blocks[name] {
			c.place(child, file, r)
		}
	}
}

// markBlocked marks the tables that a diagnostic of an unreadable file or of a
// corpus.walk entry at their place makes unavailable.
func (c *corpus) markBlocked() {
	for _, d := range c.diags {
		if !blocksRule(d.Rule) {
			continue
		}
		if t, ok := tableAt(c.cat, d.File); ok {
			c.blocked[t] = true
		}
	}
}

// blocksRule reports whether a diagnostic of rule makes its table unavailable: an
// unreadable file, or an entry that cannot be read at a table's place.
func blocksRule(rule string) bool {
	switch rule {
	case store.RuleBlock, store.RuleSection, store.RuleEncoding, store.RuleParentCycle, store.RuleWalk:
		return true
	}
	return false
}

// tableAt returns the table whose place is the corpus path file: a files table's
// folder or a row of it, or a document table's file, compared without regard to
// case. A document in a subfolder of a table folder is at no place.
func tableAt(cat *catalog.Catalog, file string) (string, bool) {
	parts := strings.Split(file, "/")
	switch len(parts) {
	case 1:
		if t, ok := docPlaceOf(cat, file); ok {
			return t, true
		}
		for _, tb := range cat.Tables {
			if folderTable(tb) && strings.EqualFold(tb.Name, file) {
				return tb.Name, true
			}
		}
	case 2, 3:
		// A files row stands one folder deeper in a sharded table, in the folder of its period.
		for _, tb := range cat.Tables {
			if folderTable(tb) && strings.EqualFold(tb.Name, parts[0]) && (len(parts) == 2 || tb.Storage.Shard != nil) {
				return tb.Name, true
			}
		}
	}
	return "", false
}

// isBlocked reports whether table name is unavailable: it holds an unreadable file
// or a corpus.walk entry at its place, or it is a block table whose placement
// parent is. A block table holds its parent's files, recursively.
func (c *corpus) isBlocked(name string) bool {
	if c.blocked[name] {
		return true
	}
	tb, ok := c.cat.Table(name)
	if !ok || tb.Storage.Kind != catalog.StorageBlock {
		return false
	}
	return c.isBlocked(tb.Storage.Parent)
}

// target is where a row is linked to: the corpus file that holds it and the anchor
// of its detail section, "" when it has none. A key that two or more rows share
// byte for byte is shared and has no target.
type target struct {
	file, anchor string
	shared       bool
}

// index maps each row, by table and key, to where it is.
type index map[string]target

func indexKey(table, key string) string { return table + "\x00" + key }

// indexOf returns the index of the rows of c. A key that a second row holds byte
// for byte names no one row, so it is marked shared; the duplicate is a validator
// rule.
func indexOf(cat *catalog.Catalog, c *corpus) (index, error) {
	idx := index{}
	for _, tb := range cat.Tables {
		for _, p := range c.rows[tb.Name] {
			k := indexKey(p.row.Table, p.row.Key)
			if t, ok := idx[k]; ok {
				t.shared = true
				idx[k] = t
				continue
			}
			has, err := store.HasDetail(cat, p.row.Table, p.row)
			if err != nil {
				return nil, err
			}
			t := target{file: p.file}
			if has {
				t.anchor = store.DetailAnchor(p.row.Table, p.row.Key)
			}
			idx[k] = t
		}
	}
	return idx, nil
}

// Target derives the link to the row of table with key from the corpus file
// fromFile: the relative path to the row's file, with its anchor when it has one.
// A key that names no row has no target, so the link is dangling; a key that two
// or more rows share is store.ErrKeyShared.
func (ix index) Target(fromFile, table, key string) (string, error) {
	t, ok := ix[indexKey(table, key)]
	if !ok {
		return "", fmt.Errorf("no %s row has the key %s", table, key)
	}
	if t.shared {
		return "", store.ErrKeyShared
	}
	return store.RelLink(fromFile, t.file, t.anchor)
}
