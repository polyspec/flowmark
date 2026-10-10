package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/polyspec/flowmark/catalog"
)

// ParseFilesTable reads the rows of the files table under the corpus root:
// the files <root>/<table>/<key>.flowmark.md. A missing folder has no rows.
// Only the documents of the folder are read; a symbolic link is never
// followed, and a document that is not of the folder is a walk diagnostic.
func ParseFilesTable(cat *catalog.Catalog, root, table string, lk Linker) ([]*Row, []Diagnostic) {
	lay, err := layoutOf(cat, table)
	if err != nil {
		return nil, []Diagnostic{fileDiag(table, RuleWalk, err.Error())}
	}
	dir := filepath.Join(root, table)
	fi, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, []Diagnostic{fileDiag(table, RuleWalk, "cannot read the table folder: "+err.Error())}
	case fi.Mode()&fs.ModeSymlink != 0:
		return nil, []Diagnostic{fileDiag(table, RuleWalk, "a symbolic link named as a files table is not followed")}
	case !fi.IsDir():
		return nil, []Diagnostic{fileDiag(table, RuleWalk, "the table folder is not a folder")}
	}
	w := &walker{cat: cat, lay: lay, lk: lk}
	w.visit(dir, table, 0)
	rows, diags := keepFirstKeys(table, w.rows)
	return rows, append(w.diags, diags...)
}

// keepFirstKeys keeps the rows whose keys differ from every earlier key without
// regard to case; a later row that collides is a diagnostic and is not kept,
// since case-insensitive file systems and lowercased anchors would confuse it.
func keepFirstKeys(table string, rows []*Row) ([]*Row, []Diagnostic) {
	var kept []*Row
	var diags []Diagnostic
	seen := map[string]string{}
	for _, r := range rows {
		low := strings.ToLower(r.Key)
		if first, dup := seen[low]; dup {
			diags = append(diags, fileDiag(table+"/"+r.Key+fileSuffix, RuleBlock, "the key "+r.Key+" collides with "+first+" without regard to case"))
			continue
		}
		seen[low] = r.Key
		kept = append(kept, r)
	}
	return kept, diags
}

// walker reads the documents under the folder of a files table.
type walker struct {
	cat   *catalog.Catalog
	lay   *filesLayout
	lk    Linker
	rows  []*Row
	diags []Diagnostic
}

// visit reads the entries of dir, which is the folder rel of the corpus. A
// document at depth 0 is a row; a document in a subfolder or with another
// suffix case is a walk diagnostic; a folder is searched for such documents;
// any other entry is outside the database.
func (w *walker) visit(dir, rel string, depth int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		w.diags = append(w.diags, fileDiag(rel, RuleWalk, "cannot read the folder: "+err.Error()))
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		file := rel + "/" + name
		doc := strings.HasSuffix(name, fileSuffix)
		looseDoc := !doc && strings.HasSuffix(strings.ToLower(name), fileSuffix)
		if !doc && !looseDoc {
			if e.IsDir() {
				w.visit(filepath.Join(dir, name), file, depth+1)
			}
			continue
		}
		if reason := notARow(e, depth, looseDoc); reason != "" {
			w.diags = append(w.diags, fileDiag(file, RuleWalk, reason))
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			w.diags = append(w.diags, fileDiag(file, RuleWalk, "cannot read the document: "+err.Error()))
			continue
		}
		row, diags := ParseFile(w.cat, w.lay.table, file, data, w.lk)
		w.diags = append(w.diags, diags...)
		if row != nil {
			w.rows = append(w.rows, row)
		}
	}
}

// notARow returns the reason an entry named like a document is not a row, or
// "" when it is a regular document of the table folder.
func notARow(e fs.DirEntry, depth int, looseDoc bool) string {
	switch {
	case depth > 0:
		return "a document outside the top level of the table folder is not a row"
	case looseDoc:
		return "the suffix of a document is .flowmark.md in lower case"
	case e.Type()&fs.ModeSymlink != 0:
		return "a symbolic link is not followed"
	case e.IsDir():
		return "a folder named as a document is not a row"
	case !e.Type().IsRegular():
		return "not a regular file"
	}
	return ""
}
