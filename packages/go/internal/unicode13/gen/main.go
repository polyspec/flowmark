// Command gen writes table.go of package unicode13 from
// testdata/DerivedGeneralCategory-13.0.0.txt and the Alphabetic lines of
// testdata/DerivedCoreProperties-13.0.0.txt, the Unicode 13.0.0 data files published
// at unicode13.CategorySource and unicode13.AlphabeticSource. go generate runs it in
// the package directory.
package main

import (
	"github.com/polyspec/flowmark/packages/go/internal/ucd"
	"github.com/polyspec/flowmark/packages/go/internal/unicode13"
)

func main() { ucd.Generate("table.go", unicode13.Table) }
