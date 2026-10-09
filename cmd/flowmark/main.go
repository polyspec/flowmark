// Command flowmark validates a corpus directory. The selection comes from the
// config/documents.json of the root when it exists: include minus exclude.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/polyspec/flowmark/corpus"
)

type documentsConfig struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func main() {
	if len(os.Args) != 3 || os.Args[1] != "validate" {
		fmt.Fprintln(os.Stderr, "usage: flowmark validate <root>")
		os.Exit(2)
	}
	root := os.Args[2]
	config := documentsConfig{Include: []string{"**/*.md"}}
	if text, err := os.ReadFile(filepath.Join(root, "config", "documents.json")); err == nil {
		if err := json.Unmarshal(text, &config); err != nil {
			fmt.Fprintf(os.Stderr, "flowmark validate: config/documents.json: %v\n", err)
			os.Exit(2)
		}
	}
	findings, err := corpus.ValidateSelected(root, config.Include, config.Exclude)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flowmark validate: %v\n", err)
		os.Exit(2)
	}
	for _, finding := range findings {
		fmt.Printf("[validate] %s: line %d: %s: %s\n", finding.File, finding.Line, finding.Rule, finding.Message)
	}
	if len(findings) > 0 {
		fmt.Printf("[validate] %d findings\n", len(findings))
		os.Exit(1)
	}
	fmt.Println("[validate] the corpus passes")
}
