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
	f, ok := os.DirFS(root).(FS)
	if !ok {
		panic("store: the os.DirFS of " + root + " is no FS")
	}
	return f
}

// ParseFilesTable reads the rows of the files table of the corpus fsys:
// the files <table>/<key>.flowmark.md. A missing folder has no rows.
// Rows whose keys differ only in case are all returned: key.duplicate is a
// validator rule, not a read fault.
// Only the documents of the folder are read; a symbolic link is never
// followed, and a document that is not of the folder is a walk diagnostic.
// The bytes of each document read are returned by corpus path, so that a
// caller holds the exact pre-image of every file its rows came from.
func ParseFilesTable(cat *catalog.Catalog, fsys FS, table string, lk Linker) ([]*Row, map[string][]byte, []Diagnostic) {
	lay, err := layoutOf(cat, table)
	if err != nil {
		return nil, nil, []Diagnostic{fileDiag(table, RuleWalk, err.Error())}
	}
	if found, diags := tableFolder(fsys, table, FilesTableWhat); !found {
		return nil, nil, diags
	}
	w := &walker{cat: cat, fsys: fsys, lay: lay, lk: lk, data: map[string][]byte{}, shard: lay.shard}
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
		w.diags = append(w.diags, fileDiag(rel, RuleWalk, FolderFailure(err)))
		return
	}
	for _, e := range entries {
		file := rel + "/" + e.Name()
		switch en := FilesEntry(w.shard, e, depth); en.Kind {
		case EntryFolder:
			w.visit(file, depth+1)
		case EntryDiag:
			w.diags = append(w.diags, fileDiag(file, RuleWalk, en.Reason))
		case EntryRow:
			data, err := w.fsys.ReadFile(file)
			if err != nil {
				w.diags = append(w.diags, fileDiag(file, RuleWalk, ReadFailure(err)))
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
}

// EntryKind is what a corpus read makes of an entry of a folder at a table's place.
type EntryKind int

// The kinds of an entry.
const (
	// EntrySkip is an entry outside the database: a hidden entry, or one that is no document.
	EntrySkip EntryKind = iota
	// EntryFolder is a folder that the read searches for documents.
	EntryFolder
	// EntryRow is a document the read parses: a row of a files table, or the rows of a document table.
	EntryRow
	// EntryDiag is an entry that is a corpus.walk diagnostic, for its Reason.
	EntryDiag
)

// Entry is the kind of an entry of a folder at a table's place and, for EntryDiag, the message of its diagnostic.
type Entry struct {
	Kind   EntryKind
	Reason string
}

// entryName reports whether the entry name is hidden, a document, and a document whose suffix differs only in case.
func entryName(name string) (hidden, doc, looseDoc bool) {
	doc = strings.HasSuffix(name, fileSuffix)
	return strings.HasPrefix(name, "."), doc, !doc && strings.HasSuffix(strings.ToLower(name), fileSuffix)
}

// FilesEntry classifies the entry e of a folder of a files table whose time shard is shard (nil for none), the folder
// depth levels under the table folder: a document at the level of the rows is a row, a document elsewhere or of
// another suffix case is a walk diagnostic, a folder is searched for such documents, and any other entry is outside
// the database. The table folder of a sharded table holds the folders of its periods and nothing else.
func FilesEntry(shard *catalog.Shard, e fs.DirEntry, depth int) Entry {
	name := e.Name()
	hidden, doc, looseDoc := entryName(name)
	if hidden {
		return Entry{}
	}
	if shard != nil && depth == 0 {
		switch {
		case e.IsDir() && e.Type()&fs.ModeSymlink == 0 && catalog.ValidPeriod(shard.Period, name):
			return Entry{Kind: EntryFolder}
		case e.IsDir() && e.Type()&fs.ModeSymlink == 0:
			return Entry{EntryDiag, "a folder of a sharded table is named as a " + string(shard.Period) + ", not " + name}
		case doc || looseDoc:
			return Entry{EntryDiag, "a document directly in the table folder of a sharded table is not a row; a row stands in the folder of its " + string(shard.Period)}
		}
		return Entry{}
	}
	if !doc && !looseDoc {
		if e.IsDir() {
			return Entry{Kind: EntryFolder}
		}
		return Entry{}
	}
	rowDepth := depth
	if shard != nil {
		rowDepth = depth - 1
	}
	if reason := notARow(e, rowDepth, looseDoc); reason != "" {
		return Entry{EntryDiag, reason}
	}
	return Entry{Kind: EntryRow}
}

// ReadFailure is the message of the walk diagnostic of a document the read cannot read.
func ReadFailure(err error) string { return "cannot read the document: " + err.Error() }

// FolderFailure is the message of the walk diagnostic of a folder the read cannot read.
func FolderFailure(err error) string { return "cannot read the folder: " + err.Error() }

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

// tableFolder reports whether the corpus root fsys holds the folder of table, what kind of table it is named as (such as
// "a files table"). The folder's type is its entry in the root, never a lookup of its path, which would follow a symbolic
// link: a root that cannot be read, a symbolic link and an entry that is no folder are not found, each with its walk
// diagnostic, and a root without the entry is not found with none.
func tableFolder(fsys FS, table, what string) (bool, []Diagnostic) {
	root, err := fsys.ReadDir(".")
	if err != nil {
		return false, []Diagnostic{fileDiag(table, RuleWalk, "cannot read the corpus root: "+err.Error())}
	}
	i := slices.IndexFunc(root, func(e fs.DirEntry) bool { return e.Name() == table })
	if i < 0 {
		return false, nil
	}
	if reason := TableFolderFault(root[i], what); reason != "" {
		return false, []Diagnostic{fileDiag(table, RuleWalk, reason)}
	}
	return true, nil
}

// TableFolderFault returns why the root entry e named as a table folder of what kind (such as "a files table") is not
// read as one — a symbolic link, or an entry that is no folder — or "" when it is a folder.
func TableFolderFault(e fs.DirEntry, what string) string {
	switch {
	case e.Type()&fs.ModeSymlink != 0:
		return "a symbolic link named as " + what + " is not followed"
	case !e.IsDir():
		return "the table folder is not a folder"
	}
	return ""
}

// FilesTableWhat and DocumentTableWhat name the kind of a table folder in the walk diagnostics of its root entry.
const (
	FilesTableWhat    = "a files table"
	DocumentTableWhat = "a sharded table"
)

// CheckFilesTable returns the error of a files table whose layout the store cannot build from the catalog, which a
// read reports as the walk diagnostic of the table.
func CheckFilesTable(cat *catalog.Catalog, table string) error {
	_, err := layoutOf(cat, table)
	return err
}

// Diag returns the diagnostic of rule at the first line of file, as the store reports a walk diagnostic.
func Diag(file, rule, msg string) Diagnostic { return fileDiag(file, rule, msg) }

// DocumentEntry classifies the entry e of the folder of a document table sharded by shard: a document named as a period
// of its grain is read; a folder, a document of another suffix case, a symbolic link, an entry that is no regular file
// and a document named as no period are walk diagnostics; any other entry is outside the database.
func DocumentEntry(shard *catalog.Shard, e fs.DirEntry) Entry {
	name := e.Name()
	hidden, doc, looseDoc := entryName(name)
	if hidden {
		return Entry{}
	}
	switch {
	case e.IsDir() && e.Type()&fs.ModeSymlink == 0:
		return Entry{EntryDiag, "a folder in the folder of a sharded document table is not a period"}
	case !doc && !looseDoc:
		return Entry{}
	case looseDoc:
		return Entry{EntryDiag, "the suffix of a document is .flowmark.md in lower case"}
	case e.Type()&fs.ModeSymlink != 0:
		return Entry{EntryDiag, "a symbolic link is not followed"}
	case !e.Type().IsRegular():
		return Entry{EntryDiag, "not a regular file"}
	case !catalog.ValidPeriod(shard.Period, strings.TrimSuffix(name, fileSuffix)):
		return Entry{EntryDiag, "a document of a sharded document table is named as a " + string(shard.Period) + ", not " + strings.TrimSuffix(name, fileSuffix)}
	}
	return Entry{Kind: EntryRow}
}

// ParseDocumentTable reads the documents of the sharded document table of the corpus fsys: the files
// <table>/<period>.flowmark.md, the period named at the grain of the table's shard (spec 20 Time shards). A
// missing folder has no documents. A document named as another grain or as no period, a folder, and a
// symbolic link are walk diagnostics; none is followed. A document that is read is returned with the
// diagnostics of its read.
func ParseDocumentTable(cat *catalog.Catalog, fsys FS, table string, lk Linker) ([]DocFile, []Diagnostic, error) {
	tb, ok := cat.Table(table)
	if !ok || tb.Storage.Kind != catalog.StorageDocument || tb.Storage.Shard == nil {
		return nil, nil, fmt.Errorf("table %s is not a sharded document table", table)
	}
	if found, diags := tableFolder(fsys, table, DocumentTableWhat); !found {
		return nil, diags, nil
	}
	entries, err := fsys.ReadDir(table)
	if err != nil {
		return nil, []Diagnostic{fileDiag(table, RuleWalk, FolderFailure(err))}, nil
	}
	var docs []DocFile
	var diags []Diagnostic
	for _, e := range entries {
		file := table + "/" + e.Name()
		switch en := DocumentEntry(tb.Storage.Shard, e); en.Kind {
		case EntryDiag:
			diags = append(diags, fileDiag(file, RuleWalk, en.Reason))
		case EntryRow:
			data, err := fsys.ReadFile(file)
			if err != nil {
				diags = append(diags, fileDiag(file, RuleWalk, ReadFailure(err)))
				continue
			}
			rows, ds, err := ParseDocument(cat, table, file, data, lk)
			if err != nil {
				return nil, nil, err
			}
			diags = append(diags, ds...)
			docs = append(docs, DocFile{Path: file, Data: data, Rows: rows})
		}
	}
	return docs, diags, nil
}
