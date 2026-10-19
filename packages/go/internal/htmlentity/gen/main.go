// Command gen writes table.go of package htmlentity from the entity data of
// testdata/entities-3.0.1.json, the data file of the entities package 3.0.1 that
// htmlentity.Source names. go generate runs it in the package directory.
package main

import (
	"fmt"
	"os"

	"github.com/polyspec/flowmark/packages/go/internal/htmlentity"
)

const (
	dataFile  = "testdata/entities-" + htmlentity.Version + ".json"
	tableFile = "table.go"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() error {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return err
	}
	src, err := htmlentity.Render(data)
	if err != nil {
		return fmt.Errorf("%s: %w", dataFile, err)
	}
	return os.WriteFile(tableFile, src, 0o644)
}
