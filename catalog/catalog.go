// Package catalog loads the dbspec catalog of a corpus root.
package catalog

import (
	"fmt"
	"time"

	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// Diagnostic is one located catalog error. File is relative to the corpus
// root and slash-separated; Line and Column are 1-based, in code points.
type Diagnostic struct {
	File    string
	Line    int
	Column  int
	Rule    string
	Message string
}

// Catalog is a loaded, valid catalog.
type Catalog struct {
	Documents    []*dbspec.Document
	Files        []string // the corpus path of each document, in the order of Documents
	ManifestHash string
	Tables       []*Table
}

// Table is one table of the catalog with its markdown settings.
type Table struct {
	Name         string
	Document     string
	Columns      []dbspec.Column
	ForeignKeys  []dbspec.ForeignKey
	Uniques      [][]string     // the columns of each unique key, in declaration order
	Checks       []dbspec.Check // the checks, in declaration order, each predicate with its literals in canonical form
	Storage      Storage
	RowScope     int
	Key          string
	Title        string
	Body         string
	Order        string
	Markdown     []string
	StateMachine *StateMachine
	Codecs       map[string][]string // the codec stages of each column with a codec setting, in write order
	View         *View               // the view columns when the table is the view table, else nil
}

// The application schema (spec 100): the catalog document flowmark provides, and
// its table whose rows are views.
const (
	ApplicationDocument = "flowmark"
	ViewTable           = "views"
)

// View names the view columns of the view table (spec 100 Views): the view's
// kind, the entity it reads, its stored filter and sort, and the field it groups
// by.
type View struct {
	Kind   string
	Entity string
	Filter string
	Sort   string
	Group  string
}

// StorageKind is the storage of a table.
type StorageKind string

// The storage kinds.
const (
	StorageFiles    StorageKind = "files"
	StorageDocument StorageKind = "document"
	StorageBlock    StorageKind = "block"
)

// Shape is the list or table form of a document or block storage.
type Shape string

// The shapes.
const (
	ShapeList  Shape = "list"
	ShapeTable Shape = "table"
)

// Period is the length of a time shard (spec 20 Time shards).
type Period string

// The periods.
const (
	PeriodYear  Period = "year"
	PeriodMonth Period = "month"
	PeriodDay   Period = "day"
)

// Shard is the time shard of a document or files storage: the rows of a period stand in their own
// file or folder. Column is the non-null date or datetime column whose value (a datetime at its stored
// UTC value) names the period of a row.
type Shard struct {
	Column string
	Period Period
}

// Storage is where a table's rows live. Shape is empty for files storage;
// ParentForeignKey and Parent are set for block storage only. Shard is nil unless the table declares a
// time shard.
type Storage struct {
	Kind             StorageKind
	Shape            Shape
	ParentForeignKey string
	Parent           string
	Shard            *Shard
}

// StateMachine is the state machine of one column of a table.
type StateMachine struct {
	Column      string
	States      []string
	Initial     []string
	Transitions []Transition
	Terminals   []Terminal
	History     History
	Limits      []Limit
	Checkboxes  map[string]string
}

// Transition is a declared transition; Require lists the columns it records.
type Transition struct {
	From    string
	To      string
	Require []string
}

// Terminal is a terminal state; Require lists the columns entering it records.
type Terminal struct {
	State   string
	Require []string
}

// History names the history table and its columns.
type History struct {
	Table string
	Row   string
	From  string
	To    string
	At    string
}

// Limit is the maximum number of rows in a state.
type Limit struct {
	State string
	Count int64
}

// Hierarchy returns the hierarchy column of the table — `parent`, when a foreign key of the table references the
// table itself through the one column `parent` (spec 20 Hierarchy) — or "" when the table has none.
func (t *Table) Hierarchy() string {
	for _, fk := range t.ForeignKeys {
		if fk.Table == t.Name && len(fk.Columns) == 1 && fk.Columns[0] == "parent" {
			return "parent"
		}
	}
	return ""
}

// Column returns the named column of the table.
func (t *Table) Column(name string) (dbspec.Column, bool) {
	for _, c := range t.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return dbspec.Column{}, false
}

// Table returns the named table of the catalog.
func (c *Catalog) Table(name string) (*Table, bool) {
	for _, t := range c.Tables {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

// DocSuffix is the suffix of a corpus document.
const DocSuffix = ".flowmark.md"

// RowPath returns the corpus path of the file that holds the row with the key of a files table, or of the
// document that holds the rows of a document table (the key is then unused). A sharded table puts it in the
// folder or the file of the period of value, the stored text of the shard column: a datetime at its stored
// UTC value, a date as it is (spec 20 Time shards). A value that is no date or datetime has no period, so
// it is an error. A block table has no file of its own, so it has no path.
func (t *Table) RowPath(key, value string) (string, error) {
	var period string
	if sh := t.Storage.Shard; sh != nil {
		p, err := periodOf(sh.Period, value)
		if err != nil {
			return "", fmt.Errorf("the shard column %s of table %s holds %q, which is not a date or datetime", sh.Column, t.Name, value)
		}
		period = p
	}
	switch t.Storage.Kind {
	case StorageFiles:
		if period != "" {
			return t.Name + "/" + period + "/" + key + DocSuffix, nil
		}
		return t.Name + "/" + key + DocSuffix, nil
	case StorageDocument:
		if period != "" {
			return t.Name + "/" + period + DocSuffix, nil
		}
		return t.Name + DocSuffix, nil
	}
	return "", fmt.Errorf("table %s is stored as a block and has no file of its own", t.Name)
}

// ValidPeriod reports whether name is the name of a period at the grain p: 2026, 2026-10 or 2026-10-11,
// a date that exists.
func ValidPeriod(p Period, name string) bool {
	layout := map[Period]string{PeriodYear: "2006", PeriodMonth: "2006-01", PeriodDay: "2006-01-02"}[p]
	if layout == "" || len(name) != len(layout) {
		return false
	}
	_, err := time.Parse(layout, name)
	return err == nil
}

// periodOf returns the name of the period of p that the date or datetime text value falls in: 2026, 2026-10
// or 2026-10-11. The date is read by the standard parser, so a month past 12 or a day past the month fails.
func periodOf(p Period, value string) (string, error) {
	day := value
	if len(day) > 10 {
		day = day[:10]
	}
	if _, err := time.Parse("2006-01-02", day); err != nil || (len(value) > 10 && value[10] != ' ' && value[10] != 'T') {
		return "", fmt.Errorf("not a date or datetime")
	}
	switch p {
	case PeriodYear:
		return day[:4], nil
	case PeriodMonth:
		return day[:7], nil
	case PeriodDay:
		return day, nil
	}
	return "", fmt.Errorf("unknown period %q", p)
}

// EnvelopeLimits are the sizes past which a corpus is outside the scale envelope the engine states (spec 40 The scale
// envelope). A corpus past one of them is read and written all the same; the validator reports it (scale.file,
// scale.folder, scale.corpus) and a write that carries a file past the file size publishes an event.
type EnvelopeLimits struct {
	// FileBytes is the largest document: a write parses, checks and renders its target whole, and the update of one
	// row costs 0.7 ms of CPU for each KB of the document (83 ms at 32 KB, 239 ms at 340 KB, 757 ms at 1.4 MB).
	FileBytes int
	// FolderFiles is the most documents of one folder.
	FolderFiles int
	// CorpusFiles is the most documents of a corpus: the first read costs 0.06 ms for each, and on macOS the watch
	// holds a descriptor for each (20009 at 10000 documents).
	CorpusFiles int
}

// Envelope is the scale envelope: what the benchmarks of the write path measured within the cost model of spec 40, an
// operation within hundreds of milliseconds. Sessions are not limited here: the lock serializes them, and the wait of
// the last of sixteen writers is 1.3 s.
var Envelope = EnvelopeLimits{FileBytes: 1 << 20, FolderFiles: 20000, CorpusFiles: 20000}
