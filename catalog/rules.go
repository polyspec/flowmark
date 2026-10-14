package catalog

import (
	"fmt"
	"slices"
	"strings"

	"github.com/polyspec/orm/engine/dbspec"
)

// maxHeading is the deepest heading level of markdown.
const maxHeading = 6

// checkTable runs the corpus rules that one table decides and returns its
// storage, which is valid only when the boolean is true.
func (l *loader) checkTable(d *document, t *dbspec.Table) (Storage, bool) {
	report := func(msg string) { l.add(l.tableDiag(d, t.Name, msg)) }
	if reservedName(t.Name) {
		report("table " + t.Name + " is a reserved file name on Windows")
	}
	if len(t.PrimaryKey.Columns) > 1 {
		report("table " + t.Name + " has a composite primary key (" + strings.Join(t.PrimaryKey.Columns, ", ") + ")")
	}
	if len(t.PrimaryKey.Columns) == 1 {
		if c := columnOf(t, t.PrimaryKey.Columns[0]); c != nil && !isKeyType(c.Type.Kind) {
			report("primary key column " + c.Name + " of table " + t.Name + " has type " + c.Type.String() + ", which is not a key type (an integer, bool, varchar, uuid or date)")
		}
	}
	for _, fk := range t.ForeignKeys {
		for _, action := range []dbspec.Action{fk.OnDelete, fk.OnUpdate} {
			if action == dbspec.ActionCascade || action == dbspec.ActionSetNull {
				report("foreign key " + fk.Name + " uses " + string(action))
			}
		}
	}
	s := t.Settings
	if s == nil {
		s = &dbspec.Settings{}
	}
	for _, c := range s.Codecs {
		if slices.Contains(c.Stages, "aes") {
			report("column " + c.Column + " uses aes")
		}
		if len(c.Stages) > 0 && c.Stages[len(c.Stages)-1] == "yaml" {
			report("column " + c.Column + " ends its codec with the stage yaml, whose encoder ends every value with a line feed")
		}
	}
	if s.AESVersion != nil {
		report("table " + t.Name + " has an aes_version setting")
	}
	if len(s.BlindIndexes) > 0 {
		report("table " + t.Name + " has a blind_index setting")
	}
	if s.Audit != nil {
		report("table " + t.Name + " has an audit setting")
	}
	if s.Immutable != nil {
		report("table " + t.Name + " has an immutable setting")
	}
	if sm := s.StateMachine; sm != nil {
		if sm.History == nil {
			report("state machine on column " + sm.Column + " has no history")
		}
		if !slices.ContainsFunc(sm.Lines, func(line dbspec.StateLineSetting) bool { return line.Initial }) {
			report("state machine on column " + sm.Column + " has no initial state")
		}
	}
	if s.Store == nil {
		report("table " + t.Name + " has no store setting")
		return Storage{}, false
	}
	var storage Storage
	switch s.Store.Kind {
	case "files":
		storage = Storage{Kind: StorageFiles}
	case "document":
		storage = Storage{Kind: StorageDocument, Shape: Shape(s.Store.Shape)}
	case "block":
		storage = Storage{Kind: StorageBlock, Shape: Shape(s.Store.Shape), ParentForeignKey: s.Store.Foreign}
	default:
		report("table " + t.Name + " has an unknown storage " + s.Store.Kind)
		return Storage{}, false
	}
	if storage.Kind != StorageFiles && storage.Shape != ShapeList && storage.Shape != ShapeTable {
		report("table " + t.Name + " has an unknown shape " + s.Store.Shape)
		return Storage{}, false
	}
	if sh := s.Store.Shard; sh != nil {
		storage.Shard = &Shard{Column: sh.Column, Period: Period(sh.Period)}
	}
	if s.Title == nil {
		switch {
		case storage.Kind == StorageFiles:
			report("table " + t.Name + ": files storage needs a title")
		case storage.Shape == ShapeList:
			report("table " + t.Name + ": list storage needs a title")
		}
	}
	// A list row carries its state as its glyph, so a list storage of a state
	// machine declares a checkbox; dbspec covers every state once one exists.
	if sm := s.StateMachine; sm != nil && storage.Shape == ShapeList && len(s.Checkboxes) == 0 {
		report("table " + t.Name + ": list storage of the state machine on column " + sm.Column + " declares no checkbox")
	}
	return storage, true
}

// columnOf returns the column of t with the name, or nil when t has none.
func columnOf(t *dbspec.Table, name string) *dbspec.Column {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
}

// isKeyType reports whether a one-column primary key may have the type: an
// integer, bool, varchar, uuid or date. The other types cannot be written in
// the key shape of a file name and anchor: time, datetime, decimal and f64
// hold characters the shape refuses, and text and bytes are refused by
// dbspec's own key rule.
func isKeyType(k dbspec.TypeKind) bool {
	switch k {
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64, dbspec.TypeBool, dbspec.TypeVarchar, dbspec.TypeUUID, dbspec.TypeDate:
		return true
	}
	return false
}

// tableDiag locates a diagnostic at the name of a table declaration.
func (l *loader) tableDiag(d *document, table, msg string) Diagnostic {
	line, column := d.position(dbspec.PositionTable, table)
	return Diagnostic{File: d.file, Line: line, Column: column, Rule: rule, Message: msg}
}

// fkDiag locates a diagnostic at the name of a foreign key declaration of a table.
func (l *loader) fkDiag(d *document, table, fk, msg string) Diagnostic {
	line, column := d.position(dbspec.PositionConstraint, table, fk)
	return Diagnostic{File: d.file, Line: line, Column: column, Rule: rule, Message: msg}
}

// position returns the line and column of a declaration from the positions that the parser reports
// (docs/dbspec.md "Declaration positions"). The rules locate only declarations of a parsed document, so a
// declaration without a position is a broken invariant of the catalog, not a place to guess.
func (d *document) position(kind string, path ...string) (int, int) {
	for _, p := range d.positions {
		if p.Kind == kind && slices.Equal(p.Path, path) {
			return p.Line, p.Column
		}
	}
	panic(fmt.Sprintf("catalog: %s has no %s position for %v", d.file, kind, path))
}

// checkForeignKeys runs the rule on the foreign keys of the set, once every
// table of the set is registered: a foreign key is written as a link to the
// referenced row, and a row has a link only through its one-column primary
// key, so a foreign key must be one column that references the one-column
// primary key of its table. A foreign key to a referenced table outside the
// registered set is not checked here: the parse of the set reports it.
func (l *loader) checkForeignKeys() {
	for _, r := range l.order {
		for _, fk := range r.table.ForeignKeys {
			target, ok := l.refs[fk.Table]
			if !ok {
				continue
			}
			pk := target.table.PrimaryKey.Columns
			switch {
			case len(fk.Columns) != 1 || len(fk.References) != 1:
				l.add(l.fkDiag(r.doc, r.table.Name, fk.Name, "foreign key "+fk.Name+" is composite ("+strings.Join(fk.Columns, ", ")+"); a link needs one column"))
			case len(pk) != 1 || pk[0] != fk.References[0]:
				l.add(l.fkDiag(r.doc, r.table.Name, fk.Name, "foreign key "+fk.Name+" references "+fk.Table+" ("+strings.Join(fk.References, ", ")+"), which is not the one-column primary key of "+fk.Table))
			}
		}
	}
}

// reservedName reports whether a table name is a Windows device name, which
// is reserved as a file name: CON, PRN, AUX, NUL, COM0-COM9 and LPT0-LPT9,
// without regard to case.
func reservedName(name string) bool {
	up := strings.ToUpper(name)
	switch up {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(up) != 4 || (!strings.HasPrefix(up, "COM") && !strings.HasPrefix(up, "LPT")) {
		return false
	}
	return up[3] >= '0' && up[3] <= '9'
}

// checkNames runs the name rule that needs the storage tree, once every
// table's storage and parent are resolved: a child block table may not be
// named like a markdown column of its parent. The comparison ignores case.
func (l *loader) checkNames() {
	for _, r := range l.order {
		if !r.linked || r.storage.Kind != StorageBlock {
			continue
		}
		parent := l.refs[r.storage.Parent]
		for _, m := range markdownOf(parent.table) {
			if strings.EqualFold(m, r.table.Name) {
				l.add(l.tableDiag(r.doc, r.table.Name, "block table "+r.table.Name+" is named like markdown column "+m+" of its parent "+parent.table.Name))
			}
		}
	}
}

// checkHistoryMarks reports, at the history table's name, a column that a
// require list names and that the state table and the history table do not
// both declare markdown or both declare without it: the history row holds a
// copy of the value, and a section and a single-line place accept different
// values.
func (l *loader) checkHistoryMarks() {
	for _, r := range l.order {
		s := r.table.Settings
		if s == nil || s.StateMachine == nil || s.StateMachine.History == nil {
			continue
		}
		h := l.refs[s.StateMachine.History.Table]
		if h == nil {
			continue
		}
		seen := map[string]bool{}
		for _, line := range s.StateMachine.Lines {
			for _, c := range line.Requires {
				if seen[c] {
					continue
				}
				seen[c] = true
				if slices.Contains(markdownOf(r.table), c) != slices.Contains(markdownOf(h.table), c) {
					l.add(l.tableDiag(h.doc, h.table.Name, "history column "+c+" of table "+h.table.Name+" and required column "+c+" of table "+r.table.Name+" differ in their markdown setting"))
				}
			}
		}
	}
}

// checkHistoryStorage reports, at the history table's name, a history table that
// is stored neither `block <row fk> table` under its state table — a block whose
// placement foreign key is the history's row column and references the state table
// — nor `document table` (spec 30 The history table): the executor and the
// validator place a history row only there.
func (l *loader) checkHistoryStorage() {
	for _, r := range l.order {
		s := r.table.Settings
		if s == nil || s.StateMachine == nil || s.StateMachine.History == nil {
			continue
		}
		hist := s.StateMachine.History
		h := l.refs[hist.Table]
		if h == nil {
			continue
		}
		st := h.storage
		ok := st.Kind == StorageDocument && st.Shape == ShapeTable && st.Shard == nil
		if st.Kind == StorageBlock && st.Shape == ShapeTable && st.Parent == r.table.Name {
			for _, fk := range h.table.ForeignKeys {
				if fk.Name == st.ParentForeignKey && len(fk.Columns) == 1 && fk.Columns[0] == hist.Row {
					ok = true
				}
			}
		}
		if !ok {
			l.add(l.tableDiag(h.doc, h.table.Name, "table "+h.table.Name+": a history table is stored `block <row fk> table` under its state table or `document table`"))
		}
	}
}

// viewColumns is the view of the view table, its columns as spec 100 names them.
var viewColumns = View{Kind: "kind", Entity: "entity", Filter: "filter", Sort: "sort", Group: "group_by"}

// isViewTable reports whether a table is the view table: the table views of the
// application schema's document (spec 100 Views).
func isViewTable(r *tableRef) bool {
	return r.doc.name == ApplicationDocument && r.table.Name == ViewTable
}

// checkViewTable reports, at the view table's name, a view column of another
// shape than spec 100 gives it: kind and entity non-null varchar or text without
// a codec; filter and sort non-null varchar or text with exactly the codec
// ordered_json; group_by a varchar or text without a codec.
func (l *loader) checkViewTable() {
	for _, r := range l.order {
		if !isViewTable(r) {
			continue
		}
		stages := map[string][]string{}
		if s := r.table.Settings; s != nil {
			for _, c := range s.Codecs {
				stages[c.Column] = c.Stages
			}
		}
		check := func(name string, nullable bool, codec []string, want string) {
			c := columnOf(r.table, name)
			ok := c != nil && (c.Type.Kind == dbspec.TypeVarchar || c.Type.Kind == dbspec.TypeText) &&
				(nullable || !c.Null) && slices.Equal(stages[name], codec)
			if !ok {
				l.add(l.tableDiag(r.doc, r.table.Name, "the view table "+r.table.Name+" declares "+name+" "+want))
			}
		}
		check(viewColumns.Kind, false, nil, "as a non-null varchar or text without a codec")
		check(viewColumns.Entity, false, nil, "as a non-null varchar or text without a codec")
		check(viewColumns.Filter, false, []string{"ordered_json"}, "as a non-null varchar or text with exactly the codec ordered_json")
		check(viewColumns.Sort, false, []string{"ordered_json"}, "as a non-null varchar or text with exactly the codec ordered_json")
		check(viewColumns.Group, true, nil, "as a varchar or text without a codec")
	}
}

// markdownOf returns the names of the markdown columns of a table.
func markdownOf(t *dbspec.Table) []string {
	if t.Settings == nil {
		return nil
	}
	var out []string
	for _, m := range t.Settings.Marks {
		out = append(out, m.Column)
	}
	return out
}
