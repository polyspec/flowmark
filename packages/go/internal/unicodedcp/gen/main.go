// Command gen writes table.go of package unicodedcp from the
// Default_Ignorable_Code_Point lines of testdata/DerivedCoreProperties-17.0.0.txt,
// the Unicode 17.0.0 data file published at unicodedcp.Source. go generate runs
// it in the package directory.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/polyspec/flowmark/packages/go/internal/unicodedcp"
)

const (
	dataFile  = "testdata/DerivedCoreProperties-" + unicodedcp.UnicodeVersion + ".txt"
	tableFile = "table.go"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	f, err := os.Open(dataFile)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	ranges, err := unicodedcp.Parse(f)
	if err != nil {
		return fmt.Errorf("%s: %w", dataFile, err)
	}
	src, err := unicodedcp.Render(ranges)
	if err != nil {
		return err
	}
	return os.WriteFile(tableFile, src, 0o644)
}
