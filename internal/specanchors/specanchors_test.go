// Package specanchors checks that every test case naming the rule it pins as
// `<spec>#<anchor>: …` names a heading of that spec.
package specanchors

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// caseName is a test case name that cites a rule: the spec number, then the
// anchor of the heading that states the rule, then ": " and the case.
var caseName = regexp.MustCompile(`^([0-9]+)#([^:]*): `)

// citesSpec is the prefix that makes a string literal a rule citation.
var citesSpec = regexp.MustCompile(`^[0-9]+#`)

// TestCaseNamesCiteSpecHeadings reads the string literals of every test file
// of the module and fails on each one citing a spec anchor that is not a
// heading of that spec, listing them all.
func TestCaseNamesCiteSpecHeadings(t *testing.T) {
	root := moduleRoot(t)
	files := testFiles(t, root)
	anchors := map[string]map[string]bool{} // spec number to its anchors
	var bad []string
	for _, file := range files {
		for _, lit := range citations(t, file) {
			rel, err := filepath.Rel(root, lit.pos.Filename)
			if err != nil {
				t.Fatal(err)
			}
			at := fmt.Sprintf("%s:%d", filepath.ToSlash(rel), lit.pos.Line)
			m := caseName.FindStringSubmatch(lit.value)
			if m == nil {
				bad = append(bad, fmt.Sprintf("%s: %q is not of the form <spec>#<anchor>: …", at, lit.value))
				continue
			}
			spec, anchor := m[1], m[2]
			set, ok := anchors[spec]
			if !ok {
				set = specAnchors(t, root, spec)
				anchors[spec] = set
			}
			if !set[anchor] {
				bad = append(bad, fmt.Sprintf("%s: %s#%s is not a heading of spec %s", at, spec, anchor, spec))
			}
		}
	}
	if len(bad) > 0 {
		t.Fatalf("%d case names cite no spec heading:\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

// TestAnchor pins the anchor rule to headings of the specs whose anchors the
// specs' own links use, and to the punctuation those headings carry.
func TestAnchor(t *testing.T) {
	cases := []struct{ heading, want string }{
		{"Encoding and line structure", "encoding-and-line-structure"},
		{"Encoding과 줄 구조", "encoding과-줄-구조"},
		{"상세 heading과 anchor", "상세-heading과-anchor"},
		{"The canon operation", "the-canon-operation"},
		{"`files` 행의 field list", "files-행의-field-list"},
		{"This repository's checklists", "this-repositorys-checklists"},
		{"Sync (scoped skeleton)", "sync-scoped-skeleton"},
		{"Corpus documents and the walk", "corpus-documents-and-the-walk"},
	}
	for _, c := range cases {
		if got := anchor(c.heading); got != c.want {
			t.Errorf("anchor(%q) = %q, want %q", c.heading, got, c.want)
		}
	}
}

// TestSpecAnchorsRejects pins that a citation the spec has no heading for is
// absent from its anchors, and that a duplicate heading takes a suffix.
func TestSpecAnchorsRejects(t *testing.T) {
	set := specAnchors(t, moduleRoot(t), "10")
	for _, a := range []string{"detail-headings-and-anchors", "readable-values", "document-order"} {
		if !set[a] {
			t.Errorf("spec 10 lacks anchor %q", a)
		}
	}
	for _, a := range []string{"order", "storage-order", "readable-value", "Readable-values"} {
		if set[a] {
			t.Errorf("spec 10 has anchor %q, which none of its headings gives", a)
		}
	}
	got, err := headingAnchors(strings.NewReader("# A\n\n```\n# B\n```\n\n## A\n### A ##\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "a-1", "a-2"}
	if !slices.Equal(got, want) {
		t.Errorf("headingAnchors = %q, want %q", got, want)
	}
}

// moduleRoot is the nearest ancestor of the package directory, where go test
// runs the test, that holds go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil {
			return dir
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod in any ancestor of the package directory")
		}
		dir = parent
	}
}

// testFiles lists the *_test.go files of the module rooted at root, skipping
// what the go command skips: testdata, directories whose name begins with "."
// or "_", and nested modules.
func testFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no test files under %s", root)
	}
	return files
}

type literal struct {
	pos   token.Position
	value string
}

// citations returns the string literals of a Go file whose value begins with
// a spec number and "#", as the code declares them.
func citations(t *testing.T, file string) []literal {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var out []literal
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("%s: %v", fset.Position(lit.Pos()), err)
		}
		if citesSpec.MatchString(value) {
			out = append(out, literal{fset.Position(lit.Pos()), value})
		}
		return true
	})
	return out
}

// specAnchors returns the anchors of the headings of spec number spec: the one
// English file docs/spec/<spec>-<name>.md, whose Korean twin is <…>.ko.md.
func specAnchors(t *testing.T, root, spec string) map[string]bool {
	t.Helper()
	dir := filepath.Join(root, "docs", "spec")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	name := regexp.MustCompile(`^` + regexp.QuoteMeta(spec) + `-[^.]+\.md$`)
	var found []string
	for _, e := range entries {
		if name.MatchString(e.Name()) {
			found = append(found, e.Name())
		}
	}
	if len(found) != 1 {
		t.Fatalf("spec %s: want one file docs/spec/%s-*.md, found %q", spec, spec, found)
	}
	f, err := os.Open(filepath.Join(dir, found[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	list, err := headingAnchors(f)
	if err != nil {
		t.Fatalf("%s: %v", found[0], err)
	}
	set := map[string]bool{}
	for _, a := range list {
		set[a] = true
	}
	return set
}

var (
	atxHeading = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+(.*?))?[ \t]*$`)
	closingRun = regexp.MustCompile(`(?:^|[ \t]+)#+$`)
	fenceOpen  = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
)

// headingAnchors returns the anchors of the ATX headings of a markdown
// document outside fenced code, in order. A repeated anchor takes the suffix
// -1, -2, … as GitHub gives it. The specs use no setext heading.
func headingAnchors(r io.Reader) ([]string, error) {
	var out []string
	seen := map[string]int{}
	fence := ""
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if fence != "" {
			if strings.HasPrefix(strings.TrimLeft(line, " "), fence) && strings.Trim(strings.TrimLeft(line, " "), fence[:1]+" \t") == "" {
				fence = ""
			}
			continue
		}
		if m := fenceOpen.FindStringSubmatch(line); m != nil {
			fence = m[1]
			continue
		}
		m := atxHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		a := anchor(closingRun.ReplaceAllString(m[1], ""))
		n := seen[a]
		seen[a] = n + 1
		if n > 0 {
			a = fmt.Sprintf("%s-%d", a, n)
		}
		out = append(out, a)
	}
	return out, s.Err()
}

// anchor is GitHub's anchor of a heading's text: lowercase, characters other
// than letters, digits, spaces and hyphens dropped, spaces turned to hyphens.
func anchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}
