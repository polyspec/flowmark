// Command flowmark works a corpus: validate, items and transition. The document selection of
// validate comes from the config/documents.json of the root when it exists.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/polyspec/flowmark/app"
	"github.com/polyspec/flowmark/corpus"
	"github.com/polyspec/flowmark/executor"
	"github.com/polyspec/flowmark/mcp"
	"github.com/polyspec/flowmark/sidecar"
)

type documentsConfig struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "mcp" {
		mcp.Serve(os.Stdin, os.Stdout)
		os.Exit(0)
	}
	if len(os.Args) == 2 && os.Args[1] == "sidecar" {
		sidecar.Serve(os.Stdin, os.Stdout)
		os.Exit(0)
	}
	if len(os.Args) < 3 {
		usage()
	}
	root := os.Args[2]
	switch os.Args[1] {
	case "validate":
		os.Exit(validate(root))
	case "items":
		os.Exit(items(root))
	case "add":
		if len(os.Args) < 5 {
			usage()
		}
		path := os.Args[3]
		id := "-"
		title := ""
		if os.Args[4] != "-" && len(os.Args) >= 6 && !strings.HasPrefix(os.Args[4], "-") && len(os.Args) > 5 && false {
		}
		// add <root> <path> [- | <id>] <title...> — "-" allocates the next ID.
		if len(os.Args) >= 6 {
			id = os.Args[4]
			title = strings.Join(os.Args[5:], " ")
		} else {
			title = strings.Join(os.Args[4:], " ")
		}
		if id == "-" {
			id = ""
		}
		if err := app.AddItem(root, path, id, title); err != nil {
			fmt.Fprintf(os.Stderr, "flowmark add: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[add] %s %s\n", id, title)
		os.Exit(0)
	case "rm":
		if len(os.Args) != 5 {
			usage()
		}
		path, id := os.Args[3], os.Args[4]
		if err := app.RemoveItem(root, path, id); err != nil {
			fmt.Fprintf(os.Stderr, "flowmark rm: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[rm] %s\n", id)
		os.Exit(0)
	case "transition":
		if len(os.Args) < 6 {
			usage()
		}
		path, id, target := os.Args[3], os.Args[4], os.Args[5]
		evidence := ""
		if len(os.Args) == 7 {
			evidence = os.Args[6]
		}
		if err := app.Transition(root, path, id, target, evidence); err != nil {
			fmt.Fprintf(os.Stderr, "flowmark transition: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[transition] %s is %s\n", id, target)
		os.Exit(0)
	}
	usage()
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: flowmark mcp | sidecar | validate <root> | items <root> | add <root> <path> <id> <title> | rm <root> <path> <id> | transition <root> <path> <id> <state> [evidence]")
	os.Exit(2)
}

func validate(root string) int {
	config := documentsConfig{Include: []string{"**/*.md"}}
	if text, err := os.ReadFile(filepath.Join(root, "config", "documents.json")); err == nil {
		if err := json.Unmarshal(text, &config); err != nil {
			fmt.Fprintf(os.Stderr, "flowmark validate: config/documents.json: %v\n", err)
			return 2
		}
	}
	findings, err := corpus.ValidateSelected(root, config.Include, config.Exclude)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flowmark validate: %v\n", err)
		return 2
	}
	for _, finding := range findings {
		fmt.Printf("[validate] %s: line %d: %s: %s\n", finding.File, finding.Line, finding.Rule, finding.Message)
	}
	if len(findings) > 0 {
		fmt.Printf("[validate] %d findings\n", len(findings))
		return 1
	}
	fmt.Println("[validate] the corpus passes")
	return 0
}

func items(root string) int {
	opened, err := executor.Open(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flowmark items: %v\n", err)
		return 2
	}
	rows, err := opened.All("items", executor.Query{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "flowmark items: %v\n", err)
		return 2
	}
	for _, row := range rows {
		fmt.Printf("%s\t%s\t%s\n", row["state"], row["id"], row["title"])
	}
	return 0
}
