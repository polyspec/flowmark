package store

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// FS is the corpus as a read sees it: its folders with the type of each entry, and its files, by slash-separated
// corpus path ("." is the root). A read goes through it and never through the working directory; the write path
// passes a mirror of the corpus kept current by the operating system's file notifications (spec 40 Opening a corpus),
// and a one-shot read passes the corpus folder itself (DirFS). ReadDir returns the entries sorted by name, each with
// its type as lstat gives it, so a symbolic link is an entry of its own type and is never followed. The bytes
// ReadFile returns are never changed by the caller.
type FS interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
}

// DirFS returns the corpus folder root as an FS, each call reading the disk.
func DirFS(root string) FS {
	return os.DirFS(root).(FS)
}

// ParseFilesTable reads the rows of the files table of the corpus fsys:
// the files <table>/<key>.flowmark.md. A missing folder has no rows.
// Rows whose keys differ only in case are all returned: key.duplicate is a
// validator rule, not a read fault.
// Only the documents of the folder are read; a symbolic link is never
// followed, and a document that is not of the folder is a walk diagnostic.
// The bytes of each document read are returned by corpus path, so that a
// caller holds the exact pre-image of every file its rows came from. Each
// document is parsed through memo, nil to parse every one.
func ParseFilesTable(cat *catalog.Catalog, fsys FS, table string, lk Linker, memo *Memo) ([]*Row, map[string][]byte, []Diagnostic) {
	lay, err := layoutOf(cat, table)
	if err != nil {
		return nil, nil, []Diagnostic{fileDiag(table, RuleWalk, err.Error())}
	}
	// The folder's type is its entry in the root, never a lookup of its path, which would follow a symbolic link.
	root, err := fsys.ReadDir(".")
	if err != nil {
		return nil, nil, []Diagnostic{fileDiag(table, RuleWalk, "cannot read the corpus root: "+err.Error())}
	}
	i := slices.IndexFunc(root, func(e fs.DirEntry) bool { return e.Name() == table })
	switch {
	case i < 0:
		return nil, nil, nil
	case root[i].Type()&fs.ModeSymlink != 0:
		return nil, nil, []Diagnostic{fileDiag(table, RuleWalk, "a symbolic link named as a files table is not followed")}
	case !root[i].IsDir():
		return nil, nil, []Diagnostic{fileDiag(table, RuleWalk, "the table folder is not a folder")}
	}
	w := &walker{cat: cat, fsys: fsys, lay: lay, lk: lk, memo: memo, data: map[string][]byte{}, shard: lay.shard}
	w.visit(table, 0)
	return w.rows, w.data, w.diags
}

// walker reads the documents under the folder of a files table.
type walker struct {
	cat   *catalog.Catalog
	fsys  FS
	lay   *filesLayout
	shard *catalog.Shard // the time shard of the table, nil when none: its rows stand one folder deeper
	lk    Linker
	memo  *Memo
	rows  []*Row
	data  map[string][]byte // the bytes of each document read, by corpus path
	diags []Diagnostic
}

// visit reads the entries of the folder rel of the corpus. A
// document at depth 0 is a row; a document in a subfolder or with another
// suffix case is a walk diagnostic; a folder is searched for such documents;
// any other entry is outside the database.
func (w *walker) visit(rel string, depth int) {
	entries, err := w.fsys.ReadDir(rel)
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
		if w.shard != nil && depth == 0 {
			// The table folder of a sharded table holds the folders of its periods and nothing else.
			switch {
			case e.IsDir() && e.Type()&fs.ModeSymlink == 0 && catalog.ValidPeriod(w.shard.Period, name):
				w.visit(file, depth+1)
			case e.IsDir() && e.Type()&fs.ModeSymlink == 0:
				w.diags = append(w.diags, fileDiag(file, RuleWalk, "a folder of a sharded table is named as a "+string(w.shard.Period)+", not "+name))
			case doc || looseDoc:
				w.diags = append(w.diags, fileDiag(file, RuleWalk, "a document directly in the table folder of a sharded table is not a row; a row stands in the folder of its "+string(w.shard.Period)))
			}
			continue
		}
		if !doc && !looseDoc {
			if e.IsDir() {
				w.visit(file, depth+1)
			}
			continue
		}
		rowDepth := depth
		if w.shard != nil {
			rowDepth = depth - 1
		}
		if reason := notARow(e, rowDepth, looseDoc); reason != "" {
			w.diags = append(w.diags, fileDiag(file, RuleWalk, reason))
			continue
		}
		data, err := w.fsys.ReadFile(file)
		if err != nil {
			w.diags = append(w.diags, fileDiag(file, RuleWalk, "cannot read the document: "+err.Error()))
			continue
		}
		w.data[file] = data
		row, diags := w.memo.ParseFile(w.cat, w.lay.table, file, data, w.lk)
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

// DocFile is one document file of a sharded document table: its corpus path, the bytes read, and its rows.
type DocFile struct {
	Path string
	Data []byte
	Rows []*Row
}

// ParseDocumentTable reads the documents of the sharded document table of the corpus fsys: the files
// <table>/<period>.flowmark.md, the period named at the grain of the table's shard (spec 20 Time shards). A
// missing folder has no documents. A document named as another grain or as no period, a folder, and a
// symbolic link are walk diagnostics; none is followed. A document that is read is returned with the
// diagnostics of its read, each document through memo, nil to parse every one.
func ParseDocumentTable(cat *catalog.Catalog, fsys FS, table string, lk Linker, memo *Memo) ([]DocFile, []Diagnostic, error) {
	tb, ok := cat.Table(table)
	if !ok || tb.Storage.Kind != catalog.StorageDocument || tb.Storage.Shard == nil {
		return nil, nil, fmt.Errorf("table %s is not a sharded document table", table)
	}
	root, err := fsys.ReadDir(".")
	if err != nil {
		return nil, []Diagnostic{fileDiag(table, RuleWalk, "cannot read the corpus root: "+err.Error())}, nil
	}
	i := slices.IndexFunc(root, func(e fs.DirEntry) bool { return e.Name() == table })
	switch {
	case i < 0:
		return nil, nil, nil
	case root[i].Type()&fs.ModeSymlink != 0:
		return nil, []Diagnostic{fileDiag(table, RuleWalk, "a symbolic link named as a sharded table is not followed")}, nil
	case !root[i].IsDir():
		return nil, []Diagnostic{fileDiag(table, RuleWalk, "the table folder is not a folder")}, nil
	}
	entries, err := fsys.ReadDir(table)
	if err != nil {
		return nil, []Diagnostic{fileDiag(table, RuleWalk, "cannot read the folder: "+err.Error())}, nil
	}
	var docs []DocFile
	var diags []Diagnostic
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		file := table + "/" + name
		doc := strings.HasSuffix(name, fileSuffix)
		looseDoc := !doc && strings.HasSuffix(strings.ToLower(name), fileSuffix)
		switch {
		case e.IsDir() && e.Type()&fs.ModeSymlink == 0:
			diags = append(diags, fileDiag(file, RuleWalk, "a folder in the folder of a sharded document table is not a period"))
		case !doc && !looseDoc:
		case looseDoc:
			diags = append(diags, fileDiag(file, RuleWalk, "the suffix of a document is .flowmark.md in lower case"))
		case e.Type()&fs.ModeSymlink != 0:
			diags = append(diags, fileDiag(file, RuleWalk, "a symbolic link is not followed"))
		case !e.Type().IsRegular():
			diags = append(diags, fileDiag(file, RuleWalk, "not a regular file"))
		case !catalog.ValidPeriod(tb.Storage.Shard.Period, strings.TrimSuffix(name, fileSuffix)):
			diags = append(diags, fileDiag(file, RuleWalk, "a document of a sharded document table is named as a "+string(tb.Storage.Shard.Period)+", not "+strings.TrimSuffix(name, fileSuffix)))
		default:
			data, err := fsys.ReadFile(file)
			if err != nil {
				diags = append(diags, fileDiag(file, RuleWalk, "cannot read the document: "+err.Error()))
				continue
			}
			rows, ds, err := memo.ParseDocument(cat, table, file, data, lk)
			if err != nil {
				return nil, nil, err
			}
			diags = append(diags, ds...)
			docs = append(docs, DocFile{Path: file, Data: data, Rows: rows})
		}
	}
	return docs, diags, nil
}
