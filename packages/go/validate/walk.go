package validate

import (
	"io/fs"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
)

// docSuffix is the suffix of a corpus document.
const docSuffix = ".flowmark.md"

// walkRoot returns the corpus.walk diagnostics of the entries of the corpus root.
// The store reads the folder of each files table whose exact name the root holds;
// a document of a document table is read by the validator when it is a regular
// file. Every other document is outside its place, or is a case variant of one.
// The names are compared as the listing holds them, never as a path lookup, so a
// case-folding file system reports a case variant as a case variant.
func walkRoot(cat *catalog.Catalog, fsys store.FS, entries []fs.DirEntry) []catalog.Diagnostic {
	var diags []catalog.Diagnostic
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == dbspecDir {
			continue
		}
		if hasTableNamed(cat, name) {
			continue
		}
		if t, ok := docPlaceOf(cat, name); ok && name == t+docSuffix {
			if !e.Type().IsRegular() {
				diags = append(diags, walkDiag(name, "a document of the table "+t+" is not a regular file"))
			}
			continue
		}
		if t, ok := foldedPlace(cat, name); ok && (e.IsDir() || e.Type()&fs.ModeSymlink != 0 || hasDocSuffixFold(name)) {
			diags = append(diags, walkDiag(name, "the name differs in case from the place of the table "+t))
			continue
		}
		if hasDocSuffixFold(name) {
			diags = append(diags, walkDiag(name, "a document that matches no table place is not a row"))
			continue
		}
		if e.IsDir() {
			diags = append(diags, walkFolder(fsys, name)...)
		}
	}
	return diags
}

// dbspecDir is the folder of the catalog, which the corpus walk does not read.
const dbspecDir = "dbspec"

// walkFolder returns the diagnostics of the documents in the folder of the corpus
// path rel: no folder outside the table places is a table folder, so a document in
// it is outside every place. Symbolic links are not followed.
func walkFolder(fsys store.FS, rel string) []catalog.Diagnostic {
	entries, err := fsys.ReadDir(rel)
	if err != nil {
		return []catalog.Diagnostic{walkDiag(rel, "cannot read the folder: "+err.Error())}
	}
	var diags []catalog.Diagnostic
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := rel + "/" + name
		switch {
		case hasDocSuffixFold(name):
			diags = append(diags, walkDiag(path, "a document outside every table place is not a row"))
		case e.IsDir():
			diags = append(diags, walkFolder(fsys, path)...)
		}
	}
	return diags
}

// walkDiag is a corpus.walk diagnostic of an entry, located at its first line.
func walkDiag(file, msg string) catalog.Diagnostic {
	return catalog.Diagnostic{File: file, Line: 1, Column: 1, Rule: store.RuleWalk, Message: msg}
}

// hasTableNamed reports whether name is the exact name of a files table folder or
// of a document table's file, the places the root holds.
func hasTableNamed(cat *catalog.Catalog, name string) bool {
	for _, tb := range cat.Tables {
		if folderTable(tb) && tb.Name == name {
			return true
		}
	}
	return false
}

// folderTable reports whether the place of table tb is a folder of its name: a files table, or a document
// table sharded by time, whose documents stand in the folder by period (spec 20 Time shards).
func folderTable(tb *catalog.Table) bool {
	return tb.Storage.Kind == catalog.StorageFiles || (tb.Storage.Kind == catalog.StorageDocument && tb.Storage.Shard != nil)
}

// fileTable reports whether the place of table tb is the one document <table>.flowmark.md.
func fileTable(tb *catalog.Table) bool {
	return tb.Storage.Kind == catalog.StorageDocument && tb.Storage.Shard == nil
}

// docPlaceOf returns the document table whose file name, <table>.flowmark.md
// without regard to case, is name.
func docPlaceOf(cat *catalog.Catalog, name string) (string, bool) {
	for _, tb := range cat.Tables {
		if fileTable(tb) && strings.EqualFold(tb.Name+docSuffix, name) {
			return tb.Name, true
		}
	}
	return "", false
}

// foldedPlace returns the table whose folder or document name equals name without
// regard to case, when name is a folder or a document of that case: the place of a
// files table in another case, or the file of a document table in another case.
func foldedPlace(cat *catalog.Catalog, name string) (string, bool) {
	for _, tb := range cat.Tables {
		switch {
		case folderTable(tb):
			if strings.EqualFold(tb.Name, name) {
				return tb.Name, true
			}
		case fileTable(tb):
			if strings.EqualFold(tb.Name+docSuffix, name) {
				return tb.Name, true
			}
		}
	}
	return "", false
}

// hasDocSuffixFold reports whether name ends with the document suffix, without
// regard to case.
func hasDocSuffixFold(name string) bool {
	return len(name) >= len(docSuffix) && strings.EqualFold(name[len(name)-len(docSuffix):], docSuffix)
}
