package catalog

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/polyspec/orm/engine/dbspec"
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
	if len(l.diags) > 0 {
		return nil, l.diags
	}
	cat := &Catalog{ManifestHash: hash}
	for _, d := range docs {
		cat.Documents = append(cat.Documents, d.parsed)
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

// readDocuments reads the .dbs files of the set in name order. A file that
// cannot be read or fails the byte check is a diagnostic and leaves the set.
// Hidden entries are skipped, as the corpus walk skips them; any other entry
// that is not a .dbs file is a diagnostic, never dropped.
func (l *loader) readDocuments(root string, entries []fs.DirEntry) []*document {
	var docs []*document
	found := false
	for _, e := range entries {
		name := e.Name()
		file := dbspecDir + "/" + name
		switch {
		case strings.HasPrefix(name, "."):
		case e.IsDir():
			l.add(fileDiagnostic(file, "unexpected directory in dbspec/"))
		case strings.HasSuffix(name, dbsExt):
			found = true
			text, ds, err := dbspec.ReadFile(filepath.Join(root, dbspecDir, name))
			if err != nil {
				l.add(fileDiagnostic(file, "cannot read "+file+": "+err.Error()))
				continue
			}
			for _, d := range ds {
				l.add(Diagnostic{File: file, Line: d.Line, Column: d.Column, Rule: rule, Message: "dbspec " + d.Rule + ": " + d.Message})
			}
			if len(ds) > 0 {
				continue
			}
			docs = append(docs, &document{file: file, name: headerName(text, name), text: text})
		default:
			l.add(fileDiagnostic(file, "unexpected file in dbspec/: "+name))
		}
	}
	if !found {
		l.add(fileDiagnostic(dbspecDir, "no .dbs document in dbspec/"))
	}
	return docs
}

// headerName is the document name of a file's header `dbspec 1 <document>`;
// when the header is not in that form, the file name stands for the document
// and the parser reports the header.
func headerName(text, fileName string) string {
	line, _, _ := strings.Cut(text, "\n")
	if f := strings.Fields(line); len(f) >= 3 && f[0] == "dbspec" {
		return f[2]
	}
	return strings.TrimSuffix(fileName, dbsExt)
}

// parseSet parses every document with the others of the set as its declared
// document set, which is what its use lines name. It reports false when a
// document failed or two documents share a name.
func (l *loader) parseSet(docs []*document) bool {
	ok := true
	firstFile := map[string]string{}
	for _, d := range docs {
		if first, dup := firstFile[d.name]; dup {
			l.add(Diagnostic{File: d.file, Line: 1, Column: len("dbspec 1 ") + 1, Rule: rule, Message: "document " + d.name + " is also declared by " + first})
			ok = false
			continue
		}
		firstFile[d.name] = d.file
	}
	for i, d := range docs {
		others := map[string]string{}
		for j, o := range docs {
			if j != i && firstFile[o.name] == o.file {
				others[o.name] = o.text
			}
		}
		parsed, ds := dbspec.Parse(d.text, others)
		for _, x := range ds {
			l.add(Diagnostic{File: d.file, Line: x.Line, Column: x.Column, Rule: rule, Message: "dbspec " + x.Rule + ": " + x.Message})
		}
		if len(ds) > 0 {
			ok = false
			continue
		}
		d.parsed = parsed
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
// key its storage names, and records it in the storage.
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
