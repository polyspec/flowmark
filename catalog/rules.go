package catalog

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

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

// tableDiag locates a diagnostic at the name of a table declaration.
func (l *loader) tableDiag(d *document, table, msg string) Diagnostic {
	line, column := locateDecl(d.text, `table +`, table, ` *\{`)
	return Diagnostic{File: d.file, Line: line, Column: column, Rule: rule, Message: msg}
}

// fkDiag locates a diagnostic at the name of a foreign key declaration.
func (l *loader) fkDiag(d *document, fk, msg string) Diagnostic {
	line, column := locateDecl(d.text, `foreign key +`, fk, ` *\(`)
	return Diagnostic{File: d.file, Line: line, Column: column, Rule: rule, Message: msg}
}

// locateDecl returns the line and column of a declaration's name in the text:
// the name follows the keyword pattern and is followed by the tail pattern.
// The parser accepted the declaration, so it is in the text; the location is
// line 1, column 1 only for a name the text does not declare.
func locateDecl(text, keyword, name, tail string) (int, int) {
	re := regexp.MustCompile(`(?m)^( *` + keyword + `)` + regexp.QuoteMeta(name) + tail)
	m := re.FindStringSubmatchIndex(text)
	if m == nil {
		return 1, 1
	}
	// m[3] is the end of the prefix group, where the name starts.
	before := []byte(text[:m[3]])
	line := 1 + bytes.Count(before, []byte("\n"))
	lineStart := bytes.LastIndexByte(before, '\n') + 1
	return line, utf8.RuneCount(before[lineStart:]) + 1
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
				l.add(l.fkDiag(r.doc, fk.Name, "foreign key "+fk.Name+" is composite ("+strings.Join(fk.Columns, ", ")+"); a link needs one column"))
			case len(pk) != 1 || pk[0] != fk.References[0]:
				l.add(l.fkDiag(r.doc, fk.Name, "foreign key "+fk.Name+" references "+fk.Table+" ("+strings.Join(fk.References, ", ")+"), which is not the one-column primary key of "+fk.Table))
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
