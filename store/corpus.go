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
// Rows whose keys differ only in case are all returned: key.duplicate is a
// validator rule, not a read fault.
// Only the documents of the folder are read; a symbolic link is never
// followed, and a document that is not of the folder is a walk diagnostic.
// The bytes of each document read are returned by corpus path, so that a
// caller holds the exact pre-image of every file its rows came from.
func ParseFilesTable(cat *catalog.Catalog, root, table string, lk Linker) ([]*Row, map[string][]byte, []Diagnostic) {
	lay, err := layoutOf(cat, table)
	if err != nil {
		return nil, nil, []Diagnostic{fileDiag(table, RuleWalk, err.Error())}
	}
	dir := filepath.Join(root, table)
	fi, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil, nil
	case err != nil:
		return nil, nil, []Diagnostic{fileDiag(table, RuleWalk, "cannot read the table folder: "+err.Error())}
	case fi.Mode()&fs.ModeSymlink != 0:
		return nil, nil, []Diagnostic{fileDiag(table, RuleWalk, "a symbolic link named as a files table is not followed")}
	case !fi.IsDir():
		return nil, nil, []Diagnostic{fileDiag(table, RuleWalk, "the table folder is not a folder")}
	}
	w := &walker{cat: cat, lay: lay, lk: lk, data: map[string][]byte{}}
	w.visit(dir, table, 0)
	return w.rows, w.data, w.diags
}

// walker reads the documents under the folder of a files table.
type walker struct {
	cat   *catalog.Catalog
	lay   *filesLayout
	lk    Linker
	rows  []*Row
	data  map[string][]byte // the bytes of each document read, by corpus path
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
		w.data[file] = data
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
