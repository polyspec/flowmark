// Package introspect validates every document of a corpus against its catalog
// and returns the catalog (spec 30 Introspection).
package introspect

import (
	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/validate"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// Document is one dbspec/ document of the catalog: its corpus path, its
// document name and its text in dbspec canonical form.
type Document struct {
	File string `json:"file"`
	Name string `json:"name"`
	Text string `json:"text"`
}

// Catalog is the returned catalog: its documents in file name order and the
// manifest hash of the set.
type Catalog struct {
	ManifestHash string     `json:"manifest_hash"`
	Documents    []Document `json:"documents"`
}

// Result is the answer of introspection. Catalog is nil when the catalog does
// not load, and Diagnostics then holds its catalog diagnostics only.
type Result struct {
	Catalog     *Catalog
	Diagnostics []catalog.Diagnostic
}

// Corpus introspects the corpus at root: it loads the catalog, validates every
// document against it and returns the catalog with every diagnostic, sorted as
// the validator sorts them. A catalog that does not load returns no catalog and
// its catalog diagnostics. An error is returned only when the corpus root
// cannot be read.
func Corpus(root string) (*Result, error) {
	cat, diags := catalog.Load(root)
	if cat == nil || len(diags) > 0 {
		return &Result{Diagnostics: diags}, nil
	}
	diags, err := validate.Corpus(cat, root)
	if err != nil {
		return nil, err
	}
	out := &Catalog{ManifestHash: cat.ManifestHash, Documents: make([]Document, len(cat.Documents))}
	for i, d := range cat.Documents {
		out.Documents[i] = Document{File: cat.Files[i], Name: d.Name, Text: dbspec.Emit(d)}
	}
	return &Result{Catalog: out, Diagnostics: diags}, nil
}
