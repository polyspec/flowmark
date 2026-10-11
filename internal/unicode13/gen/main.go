// Command gen writes table.go of package unicode13 from
// testdata/DerivedGeneralCategory-13.0.0.txt and the Alphabetic lines of
// testdata/DerivedCoreProperties-13.0.0.txt, the Unicode 13.0.0 data files published
// at unicode13.CategorySource and unicode13.AlphabeticSource. go generate runs it in
// the package directory.
package main

import (
	"fmt"
	"os"

	"github.com/polyspec/flowmark/internal/unicode13"
)

const (
	categoryFile   = "testdata/DerivedGeneralCategory-" + unicode13.UnicodeVersion + ".txt"
	alphabeticFile = "testdata/DerivedCoreProperties-" + unicode13.UnicodeVersion + ".txt"
	tableFile      = "table.go"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() error {
	cf, err := os.Open(categoryFile)
	if err != nil {
		return err
	}
	defer cf.Close()
	categories, err := unicode13.ParseCategories(cf)
	if err != nil {
		return fmt.Errorf("%s: %w", categoryFile, err)
	}
	af, err := os.Open(alphabeticFile)
	if err != nil {
		return err
	}
	defer af.Close()
	alphabetic, err := unicode13.ParseAlphabetic(af)
	if err != nil {
		return fmt.Errorf("%s: %w", alphabeticFile, err)
	}
	src, err := unicode13.Render(categories, alphabetic)
	if err != nil {
		return err
	}
	return os.WriteFile(tableFile, src, 0o644)
}
