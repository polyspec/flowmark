// Tests of the codec over the repository corpus: every document config/documents.json selects
// parses without a diagnostic (P1 input) and re-renders byte for byte (P1), and a CRLF copy
// re-renders stably (P2). External package: the corpus package imports the codec.
package codec_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/codec"
	"github.com/polyspec/flowmark/corpus"
)

// The corpus of the repository: the documents config/documents.json selects, read from the
// repository root, the same selection `make validate` and `make documents-check` use.
func documents(t *testing.T) []string {
	t.Helper()
	const root = ".."
	raw, err := os.ReadFile(filepath.Join(root, "config", "documents.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Include []string `json:"include"`
		Exclude []string `json:"exclude"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	paths, err := corpus.SelectedPaths(root, config.Include, config.Exclude)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 10 {
		t.Fatalf("the corpus selection found %d documents", len(paths))
	}
	for i, path := range paths {
		paths[i] = filepath.Join(root, filepath.FromSlash(path))
	}
	return paths
}

func TestCorpusIsCanonical(t *testing.T) {
	for _, path := range documents(t) {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		document, diagnostics := codec.Parse(string(text))
		if len(diagnostics) != 0 {
			t.Errorf("%s: diagnostics: %v", path, diagnostics)
			continue
		}
		if rendered := codec.Render(document); rendered != string(text) {
			t.Errorf("%s: P1 failed", path)
		}
	}
}

func TestCorpusEmissionStability(t *testing.T) {
	for _, path := range documents(t) {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		crlf := strings.ReplaceAll(string(text), "\n", "\r\n")
		document, _ := codec.Parse(crlf)
		first := codec.Render(document)
		again, diagnostics := codec.Parse(first)
		if len(diagnostics) != 0 {
			t.Errorf("%s: second diagnostics: %v", path, diagnostics)
			continue
		}
		if second := codec.Render(again); second != first {
			t.Errorf("%s: P2 failed", path)
		}
	}
}
