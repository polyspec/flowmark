package store

import "github.com/polyspec/flowmark/packages/go/catalog"

// CanonicalFile returns the canonical form of the file of row, a files row read from its file: the bytes against
// which the read judged the file format.noncanonical, which flowmark canon writes (spec 40 The canon operation). It
// renders as the read does, so a value read with a validator diagnostic keeps its written text, and a dangling or
// shared link keeps its written target. A row the read returned never refuses; a refusal is returned as RenderFile
// returns it.
func CanonicalFile(cat *catalog.Catalog, table, rel string, row *Row, lk Linker) ([]byte, []Refusal, error) {
	lay, err := layoutOf(cat, table)
	if err != nil {
		return nil, nil, err
	}
	out, _, rf := renderRow(lay, rel, row, lk)
	if len(rf) > 0 {
		return nil, rf, nil
	}
	return out, nil, nil
}

// CanonicalDocument returns the canonical form of the document file of rows, the rows a read of it returned: the
// bytes against which the read judged the file format.noncanonical, which flowmark canon writes (spec 40 The canon
// operation). It renders as the read does — keys that rows of two blocks of the file share byte for byte are the
// validator's key.duplicate, never a refusal as they are for a write — so a value read with a validator diagnostic keeps its written text, and a dangling or shared link keeps
// its written target. A row set the read returned never refuses; a refusal is returned as RenderDocument returns it.
func CanonicalDocument(cat *catalog.Catalog, table, rel string, rows []*Row, lk Linker) ([]byte, []Refusal, error) {
	lay, err := docLayoutOf(cat, table)
	if err != nil {
		return nil, nil, err
	}
	out, _, rf := renderDocument(lay, rel, rows, lk, false)
	if len(rf) > 0 {
		return nil, rf, nil
	}
	return out, nil, nil
}
