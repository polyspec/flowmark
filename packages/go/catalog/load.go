package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// The corpus layout: the catalog is the dbspec/ directory of the root.
const (
	dbspecDir = "dbspec"
	dbsExt    = ".dbs"
	rule      = "catalog"
)

// Load reads the catalog of the corpus root: every dbspec/*.dbs file as one
// document set, and the corpus rules. It returns a catalog only when there
// are no diagnostics; otherwise it returns nil and every diagnostic found.
func Load(root string) (*Catalog, []Diagnostic) {
	entries, err := os.ReadDir(filepath.Join(root, dbspecDir))
	if err != nil {
		msg := "cannot read dbspec/: " + err.Error()
		if errors.Is(err, fs.ErrNotExist) {
			msg = "no dbspec/ directory"
		}
		return nil, []Diagnostic{fileDiagnostic(dbspecDir, msg)}
	}
	l := &loader{}
	docs := l.readDocuments(root, entries)
	allParsed := l.parseSet(docs)
	var hash string
	if allParsed {
		hash = l.manifest(docs)
	}
	l.collectTables(docs)
	l.checkForeignKeys()
	l.resolveLayout()
	l.checkNames()
	l.checkHistoryMarks()
	l.checkHistoryStorage()
	l.checkViewTable()
	if len(l.diags) > 0 {
		return nil, l.diags
	}
	cat := &Catalog{ManifestHash: hash}
	for _, d := range docs {
		cat.Documents = append(cat.Documents, d.parsed)
		cat.Files = append(cat.Files, d.file)
	}
	for _, r := range l.order {
		cat.Tables = append(cat.Tables, buildTable(r))
	}
	return cat, nil
}

// document is one dbspec/*.dbs file of the set.
type document struct {
	file   string // relative to the root, slash-separated
	name   string // the header name, which the document set keys it by
	text   string
	parsed *dbspec.Document
	// positions are the declaration positions of the parsed document, in source order.
	positions []dbspec.Position
}

// tableRef is a table of the set with the facts its layout needs.
type tableRef struct {
	doc     *document
	table   *dbspec.Table
	storage Storage
	scope   int
	phase   int  // phaseNew, phaseVisiting or phaseDone
	ok      bool // the storage is valid
	linked  bool // the row scope is resolved: the storage is valid and its block chain reaches a root
}

const (
	phaseNew = iota
	phaseVisiting
	phaseDone
)

// loader collects the diagnostics of one Load and the tables of the set.
type loader struct {
	diags []Diagnostic
	refs  map[string]*tableRef
	order []*tableRef
}

func (l *loader) add(d Diagnostic) {
	l.diags = append(l.diags, d)
}

// fileDiagnostic locates a diagnostic at the first line of a file.
func fileDiagnostic(file, msg string) Diagnostic {
	return Diagnostic{File: file, Line: 1, Column: 1, Rule: rule, Message: msg}
}

// readDocuments reads the documents that the set file dbspec/dbspec-set.json lists, with orm's set reader, in file
// name order (docs/spec/30-schema.md, The catalog). An error of the set file, a listed document outside dbspec/ or in a
// folder of it, an external entry, a .dbs file of dbspec/ that the set does not list and any other entry of dbspec/
// are diagnostics, never dropped; a document that fails the byte check leaves the set with its diagnostics. Hidden
// entries are skipped, as the corpus walk skips them.
func (l *loader) readDocuments(root string, entries []fs.DirEntry) []*document {
	setFile := dbspecDir + "/" + dbspec.SetFileName
	listed := map[string]bool{}
	var docs []*document
	hasSet := false
	for _, e := range entries {
		if e.Name() == dbspec.SetFileName && !e.IsDir() {
			hasSet = true
		}
	}
	if !hasSet {
		l.add(fileDiagnostic(setFile, "no "+setFile))
	} else {
		set, errs := dbspec.ReadSet(filepath.Join(root, dbspecDir, dbspec.SetFileName))
		for _, e := range errs {
			l.add(fileDiagnostic(setFile, "dbspec "+e.Rule+": "+e.Message))
		}
		for _, f := range set.External {
			l.add(fileDiagnostic(setFile, "external document "+corpusPath(root, f.Path)+": the catalog uses no document outside its set"))
		}
		for _, f := range set.Documents {
			file, ok := l.documentFile(root, f.Path)
			if !ok {
				l.add(fileDiagnostic(setFile, "listed document "+file+" is not directly in dbspec/"))
				continue
			}
			listed[file] = true
			text, ds := dbspec.ReadBytes(file, f.Bytes)
			for _, d := range ds {
				l.add(Diagnostic{File: file, Line: d.Line, Column: d.Column, Rule: rule, Message: "dbspec " + d.Rule + ": " + d.Message})
			}
			if len(ds) > 0 {
				continue
			}
			docs = append(docs, &document{file: file, text: text})
		}
	}
	for _, e := range entries {
		name := e.Name()
		file := dbspecDir + "/" + name
		switch {
		case strings.HasPrefix(name, "."), name == dbspec.SetFileName:
		case e.IsDir():
			l.add(fileDiagnostic(file, "unexpected directory in dbspec/"))
		case strings.HasSuffix(name, dbsExt):
			if hasSet && !listed[file] {
				l.add(fileDiagnostic(file, file+" is not listed in "+setFile))
			}
		default:
			l.add(fileDiagnostic(file, "unexpected file in dbspec/: "+name))
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].file < docs[j].file })
	return docs
}

// documentFile is the corpus path of a listed document, and whether it lies directly in dbspec/.
func (l *loader) documentFile(root, joined string) (string, bool) {
	file := corpusPath(root, joined)
	return file, path.Dir(file) == dbspecDir && strings.HasSuffix(file, dbsExt)
}

// corpusPath is the corpus path of a path that orm's set reader joined with the root: relative to the root and
// slash-separated, as every corpus diagnostic names its file.
func corpusPath(root, joined string) string {
	rel, err := filepath.Rel(root, joined)
	if err != nil {
		panic(fmt.Sprintf("catalog: the set reader joined %s with the root %s", joined, root))
	}
	return filepath.ToSlash(rel)
}

// parseSet parses the documents with orm's set parse (docs/dbspec.md "Parsing a set"), which names each document by its
// header, parses it with the others as the documents its use lines name, and reports a repeated name on the later
// document. It reports false when a document has a diagnostic.
func (l *loader) parseSet(docs []*document) bool {
	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.text
	}
	ok := true
	for i, r := range dbspec.ParseSet(texts, nil) {
		d := docs[i]
		d.name = r.Name
		for _, x := range r.Diagnostics {
			l.add(Diagnostic{File: d.file, Line: x.Line, Column: x.Column, Rule: rule, Message: "dbspec " + x.Rule + ": " + x.Message})
		}
		if len(r.Diagnostics) > 0 {
			ok = false
		}
		d.parsed, d.positions = r.Document, r.Positions
	}
	return ok
}

// manifest checks the set as a whole and returns its manifest hash. Its
// diagnostics concern the set, not one file, so they are located at dbspec/.
func (l *loader) manifest(docs []*document) string {
	parsed := make([]*dbspec.Document, 0, len(docs))
	for _, d := range docs {
		parsed = append(parsed, d.parsed)
	}
	m, ds := dbspec.ManifestOf(parsed)
	for _, d := range ds {
		l.add(Diagnostic{File: dbspecDir, Line: d.Line, Column: d.Column, Rule: rule, Message: "dbspec " + d.Rule + ": " + d.Message})
	}
	if m == nil {
		return ""
	}
	return m.ManifestHash
}

// collectTables registers every table of the set in declaration order and
// runs the rules that a single table decides. A table name that two documents
// declare is a diagnostic at the later one.
func (l *loader) collectTables(docs []*document) {
	l.refs = map[string]*tableRef{}
	for _, d := range docs {
		if d.parsed == nil {
			continue
		}
		for i := range d.parsed.Tables {
			t := &d.parsed.Tables[i]
			if first, dup := l.refs[t.Name]; dup {
				l.add(l.tableDiag(d, t.Name, "table "+t.Name+" is also declared by "+first.doc.file))
				continue
			}
			storage, ok := l.checkTable(d, t)
			r := &tableRef{doc: d, table: t, storage: storage, ok: ok}
			l.refs[t.Name] = r
			l.order = append(l.order, r)
		}
	}
}

// resolveLayout computes the row scope of every table and reports a layout
// that needs a heading past H6.
func (l *loader) resolveLayout() {
	for _, r := range l.order {
		l.scopeOf(r)
	}
	for _, r := range l.order {
		if !r.ok {
			continue
		}
		if level := l.deepestHeading(r); level > maxHeading {
			l.add(l.tableDiag(r.doc, r.table.Name, "the layout of table "+r.table.Name+" needs heading level "+strconv.Itoa(level)+", past H6"))
		}
	}
}

// deepestHeading returns the deepest heading level that the rows of r write, 0
// when they write none. The rows write their sections at their scope when they
// can have a markdown column other than the body or a child block; they write
// their detail heading one level above their scope when they can have a body.
// A table with neither writes no heading below its block heading, so a scope of
// 7 or more is no layout that needs a heading past H6.
func (l *loader) deepestHeading(r *tableRef) int {
	var s dbspec.Settings
	if r.table.Settings != nil {
		s = *r.table.Settings
	}
	for _, m := range s.Marks {
		if s.Body == nil || m.Column != s.Body.Column {
			return r.scope
		}
	}
	for _, c := range l.order {
		if c.storage.Kind == StorageBlock && c.storage.Parent == r.table.Name {
			return r.scope
		}
	}
	if s.Body != nil {
		return r.scope - 1
	}
	return 0
}

// scopeOf returns the row scope level of r, or false when it has none: its
// block chain is a cycle or names a parent outside the set, and that is
// reported once, at the table where it is found.
func (l *loader) scopeOf(r *tableRef) (int, bool) {
	switch r.phase {
	case phaseDone:
		return r.scope, r.ok
	case phaseVisiting:
		l.add(l.tableDiag(r.doc, r.table.Name, "the block chain of table "+r.table.Name+" is a cycle"))
		return 0, false
	}
	r.phase = phaseVisiting
	switch r.storage.Kind {
	case StorageFiles:
		r.scope, r.ok, r.linked = 2, true, true
	case StorageDocument:
		r.scope, r.ok, r.linked = 3, true, true
	case StorageBlock:
		if parent := l.parentOf(r); parent != nil {
			if level, ok := l.scopeOf(parent); ok {
				r.scope, r.ok, r.linked = level+2, true, true
			}
		}
	}
	r.phase = phaseDone
	return r.scope, r.ok
}

// parentOf resolves the parent table of a block table through the foreign
// key its storage names, and records it in the storage. A nullable column of
// that key is reported at the table's name (spec 30 Block placement): a block
// row stands inside the scope of the row the key references, and NULL
// references none; the layout still resolves, so the other rules run.
func (l *loader) parentOf(r *tableRef) *tableRef {
	var fk *dbspec.ForeignKey
	for i := range r.table.ForeignKeys {
		if r.table.ForeignKeys[i].Name == r.storage.ParentForeignKey {
			fk = &r.table.ForeignKeys[i]
		}
	}
	if fk == nil {
		l.add(l.tableDiag(r.doc, r.table.Name, "store block names "+r.storage.ParentForeignKey+", which is not a foreign key of table "+r.table.Name))
		return nil
	}
	for _, name := range fk.Columns {
		if c := columnOf(r.table, name); c != nil && c.Null {
			l.add(l.tableDiag(r.doc, r.table.Name, "block table "+r.table.Name+" is placed by foreign key "+fk.Name+", whose column "+name+" is nullable"))
		}
	}
	parent, ok := l.refs[fk.Table]
	if !ok {
		l.add(l.tableDiag(r.doc, r.table.Name, "the block parent "+fk.Table+" of table "+r.table.Name+" is not a table of the set"))
		return nil
	}
	r.storage.Parent = fk.Table
	return parent
}

// buildTable converts a checked table into its catalog form.
func buildTable(r *tableRef) *Table {
	t := r.table
	s := t.Settings
	if s == nil {
		s = &dbspec.Settings{}
	}
	out := &Table{
		Name:        t.Name,
		Document:    r.doc.parsed.Name,
		Columns:     t.Columns,
		ForeignKeys: t.ForeignKeys,
		Storage:     r.storage,
		RowScope:    r.scope,
	}
	for _, u := range t.Uniques {
		out.Uniques = append(out.Uniques, slices.Clone(u.Columns))
	}
	out.Checks = slices.Clone(t.Checks)
	if len(t.PrimaryKey.Columns) == 1 {
		out.Key = t.PrimaryKey.Columns[0]
	}
	if s.Title != nil {
		out.Title = s.Title.Column
	}
	if s.Body != nil {
		out.Body = s.Body.Column
	}
	if s.Order != nil {
		out.Order = s.Order.Column
	}
	for _, m := range s.Marks {
		out.Markdown = append(out.Markdown, m.Column)
	}
	if sm := s.StateMachine; sm != nil {
		out.StateMachine = stateMachine(sm, s.Checkboxes)
	}
	for _, c := range s.Codecs {
		if out.Codecs == nil {
			out.Codecs = map[string][]string{}
		}
		out.Codecs[c.Column] = slices.Clone(c.Stages)
	}
	if isViewTable(r) {
		v := viewColumns
		out.View = &v
	}
	return out
}

// stateMachine converts a state machine setting; its state set is the states
// its lines name, in the order the lines name them.
func stateMachine(sm *dbspec.StateMachineSetting, checkboxes []dbspec.CheckboxSetting) *StateMachine {
	out := &StateMachine{Column: sm.Column}
	add := func(state string) {
		for _, s := range out.States {
			if s == state {
				return
			}
		}
		out.States = append(out.States, state)
	}
	for _, line := range sm.Lines {
		switch {
		case line.Initial:
			out.Initial = append(out.Initial, line.State)
			add(line.State)
		case line.Terminal:
			out.Terminals = append(out.Terminals, Terminal{State: line.State, Require: line.Requires})
			add(line.State)
		default:
			out.Transitions = append(out.Transitions, Transition{From: line.From, To: line.To, Require: line.Requires})
			add(line.From)
			add(line.To)
		}
	}
	if h := sm.History; h != nil {
		out.History = History{Table: h.Table, Row: h.Row, From: h.From, To: h.To, At: h.At}
	}
	for _, l := range sm.Limits {
		out.Limits = append(out.Limits, Limit{State: l.State, Count: l.Count})
	}
	for _, c := range checkboxes {
		if c.Column != sm.Column {
			continue
		}
		if out.Checkboxes == nil {
			out.Checkboxes = map[string]string{}
		}
		out.Checkboxes[c.State] = c.Glyph
	}
	return out
}
