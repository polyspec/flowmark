// Package catalog loads the dbspec catalog of a corpus root.
package catalog

import "github.com/polyspec/orm/engine/dbspec"

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
	ManifestHash string
	Translations []string
	Tables       []*Table
}

// Table is one table of the catalog with its markdown settings.
type Table struct {
	Name         string
	Document     string
	Columns      []dbspec.Column
	ForeignKeys  []dbspec.ForeignKey
	Storage      Storage
	RowScope     int
	Key          string
	Title        string
	Body         string
	Order        string
	Markdown     []string
	StateMachine *StateMachine
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

// Storage is where a table's rows live. Shape is empty for files storage;
// ParentForeignKey and Parent are set for block storage only.
type Storage struct {
	Kind             StorageKind
	Shape            Shape
	ParentForeignKey string
	Parent           string
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

// Table returns the named table of the catalog.
func (c *Catalog) Table(name string) (*Table, bool) {
	for _, t := range c.Tables {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}
