// Package inventory holds the grep of the hardcoding inventory of the markdown database plan
// (docs/plans/markdown-database/plan.md, Hardcoding inventory): every place the table lists is removed, and no Go file
// of the repository holds a name or a literal of the behaviours it lists. The plugin row is the plugin repository's
// own item and is grepped there.
package inventory

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// root is the module root, the folder of go.mod, one folder above this package; the test checks that go.mod stands there.
const root = ".."

// removedPlaces are the files and packages of the table: each was read from the catalog or removed with the old
// engine, so none may stand again.
var removedPlaces = []string{
	"app/apply.go", "app/schema.go", "app",
	"corpus/validate.go", "corpus",
	"executor/executor.go", "executor",
	"codec/blocks.go", "codec",
	"sidecar/loop.go",
}

// names are the identifiers of the table, which no Go file may declare or use.
var names = []string{"WIPHeadroom", "taskIDLess", "firstFlowmarkFile", "ApplySchema"}

// literals are the hardcoded behaviours of the table, which no Go file outside the tests may hold: the WIP glyphs as
// string literals, the assignee and date sigils, the in-document Mermaid, the twin suffix and the `**/*.md` selection,
// and SQL rendered by hand.
var literals = []struct{ what, pattern string }{
	{"a WIP glyph", `"\[[~o]\]"`},
	{"the @assignee sigil", `@assignee`},
	{"the in-document Mermaid", `(?i)mermaid|stateDiagram`},
	{"the twin suffix", `\.ko\.md`},
	{"the **/*.md selection", `\*\*/\*\.md`},
	{"hand-rendered SQL", `(?i)create table|insert into`},
}

// exportTables are the table names sync/export.go once hardcoded.
var exportTables = regexp.MustCompile(`"(documents|items)"`)

func TestHardcodingInventory(t *testing.T) {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("the repository root %s holds no go.mod: %v", root, err)
	}
	for _, p := range removedPlaces {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			t.Errorf("%s stands; the hardcoding inventory lists it as removed", p)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	var patterns []*regexp.Regexp
	for _, l := range literals {
		patterns = append(patterns, regexp.MustCompile(l.pattern))
	}
	self, err := filepath.Abs("inventory_test.go")
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "var") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		if abs, _ := filepath.Abs(p); abs == self {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files++
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		text := string(b)
		for _, n := range names {
			if regexp.MustCompile(`\b` + n + `\b`).MatchString(text) {
				t.Errorf("%s names %s, which the hardcoding inventory lists", rel, n)
			}
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil
		}
		for i, re := range patterns {
			if loc := re.FindStringIndex(text); loc != nil {
				line := strings.Count(text[:loc[0]], "\n") + 1
				t.Errorf("%s:%d holds %s (%s), which the hardcoding inventory lists", rel, line, literals[i].what, text[loc[0]:loc[1]])
			}
		}
		if rel == "sync/export.go" && exportTables.MatchString(text) {
			t.Errorf("sync/export.go names the table %s, which the hardcoding inventory lists", exportTables.FindString(text))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("the grep read no Go file")
	}
}
