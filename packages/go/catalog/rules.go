package catalog

import (
	"fmt"
	"slices"
	"strings"

	"github.com/polyspec/orm/packages/go/engine/dbspec"
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
	for _, name := range t.PrimaryKey.Columns {
		// A table in table shape holds its key in a cell, so every type a cell reads may key it; a file name, a heading
		// and an anchor hold only the key types.
		inCell := (t.Settings == nil || t.Settings.Store == nil) || t.Settings.Store.Shape == string(ShapeTable)
		if c := columnOf(t, name); c != nil && !inCell && !isKeyType(c.Type.Kind) {
			report("primary key column " + c.Name + " of table " + t.Name + " has type " + c.Type.String() + ", which is not a key type (an integer, bool, varchar, uuid or date)")
		}
	}
	s := t.Settings
	if s == nil {
		s = &dbspec.Settings{}
	}
	for _, c := range s.Codecs {
		if len(c.Stages) > 0 && c.Stages[len(c.Stages)-1] == "yaml" {
			report("column " + c.Column + " ends its codec with the stage yaml, whose encoder ends every value with a line feed")
		}
	}
	if len(t.PrimaryKey.Columns) > 1 && s.Store != nil && s.Store.Kind == "files" {
		report("table " + t.Name + ": files storage names a file by one key column, and the primary key is composite (" + strings.Join(t.PrimaryKey.Columns, ", ") + ")")
	}
	if s.Store == nil {
		// A table that declares no storage is a table in one document of its own (spec 30).
		return Storage{Kind: StorageDocument, Shape: ShapeTable}, true
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
		if r.doc.external {
			continue
		}
		for _, fk := range r.table.ForeignKeys {
			target, ok := l.refs[fk.Table]
			if !ok {
				continue
			}
			pk := target.table.PrimaryKey.Columns
			references := slices.Sorted(slices.Values(fk.References))
			switch {
			case !slices.Equal(references, slices.Sorted(slices.Values(pk))):
				l.add(l.fkDiag(r.doc, r.table.Name, fk.Name, "foreign key "+fk.Name+" references "+fk.Table+" ("+strings.Join(fk.References, ", ")+"), which is not the primary key of "+fk.Table))
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
