// Package store parses and renders the rows of the markdown storages of a
// corpus. A Row is storage-neutral: the files storage maps it to one file per
// row, and the document and block storages map the same Row to their list and
// table forms.
package store

import "github.com/polyspec/flowmark/catalog"

// Diagnostic is one located format error, shaped as catalog.Diagnostic.
type Diagnostic = catalog.Diagnostic

// The rule names of the specifications. Write refusals are CONSTRAINT.
const (
	RuleNoncanonical = "format.noncanonical"
	RuleSection      = "format.section"
	RuleBlock        = "format.block"
	RuleEncoding     = "format.encoding"
	RuleWalk         = "corpus.walk"
	RuleConstraint   = "CONSTRAINT"
	RuleForeignKey   = "FOREIGN_KEY"
)

// Row is one row of a table in its readable form.
//
// Key is the key as its file name holds it. Title and Body are never NULL.
// Scalars holds each stored scalar column, the foreign keys as the referenced
// key; a nil entry, or a missing one, is NULL and the empty text is a non-nil
// entry holding "". Every value is in its canonical readable form (see the
// canonical format, "Readable values"). Sections holds each nullable markdown
// column other than the body: a nil entry is an absent section and a non-nil
// empty entry a present empty section.
type Row struct {
	Table    string
	Key      string
	Title    string
	Body     string
	Scalars  map[string]*string
	Sections map[string]*string
	// Raw holds the text of each scalar that is not a value of its column type,
	// as the file writes it. A scalar still holding its Raw text is written as
	// it is; any other value is checked against its type.
	Raw map[string]string
	// Dangling holds the written target of each foreign key that names no row,
	// keyed by its column.
	Dangling map[string]DanglingLink
}

// DanglingLink is the link target a dangling foreign key is written with, and
// the key its link text names.
type DanglingLink struct {
	Key    string
	Target string
}

// Linker derives the link target of a foreign key: the relative path, with
// "/" separators, from the corpus file fromFile to the file that holds the row
// of table with key, followed by "#<anchor>" when that row has a detail
// section. It returns an error when no row has the key; a link to such a row
// is dangling.
type Linker interface {
	Target(fromFile, table, key string) (string, error)
}
