// Command gen writes table.go of package unicodedcp from the
// Default_Ignorable_Code_Point lines of testdata/DerivedCoreProperties-17.0.0.txt,
// the Unicode 17.0.0 data file published at unicodedcp.Source. go generate runs
// it in the package directory.
package main

import (
	"github.com/polyspec/flowmark/packages/go/internal/ucd"
	"github.com/polyspec/flowmark/packages/go/internal/unicodedcp"
)

func main() { ucd.Generate("table.go", unicodedcp.Table) }
