// Tests of the codec over the repository corpus: every selected document parses without a
// diagnostic (P1 input) and re-renders byte for byte (P1), and a CRLF copy re-renders stably (P2).
package codec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The corpus selection of config/documents.json: every tracked markdown document except the
// vendored kit tests.
func documents(t *testing.T) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir("..", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "tests" || entry.Name() == ".git" || entry.Name() == ".kit" || entry.Name() == "scripts" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) < 10 {
		t.Fatalf("the corpus walk found %d documents", len(found))
	}
	return found
}

func TestCorpusIsCanonical(t *testing.T) {
	for _, path := range documents(t) {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		document, diagnostics := Parse(string(text))
		if len(diagnostics) != 0 {
			t.Errorf("%s: diagnostics: %v", path, diagnostics)
			continue
		}
		if rendered := Render(document); rendered != string(text) {
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
		document, _ := Parse(crlf)
		first := Render(document)
		again, diagnostics := Parse(first)
		if len(diagnostics) != 0 {
			t.Errorf("%s: second diagnostics: %v", path, diagnostics)
			continue
		}
		if second := Render(again); second != first {
			t.Errorf("%s: P2 failed", path)
		}
	}
}
