package store

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

// tree writes files (a trailing slash makes a folder) and symlinks, each
// relative to a fresh corpus root, and returns the root.
func tree(t *testing.T, files map[string]string, links map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		p := filepath.Join(root, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range links {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// keysAndRules returns the sorted keys of the rows and the sorted rules of the
// diagnostics, for comparison.
func keysAndRules(rows []*Row, diags []Diagnostic) ([]string, []string) {
	var keys, rules []string
	for _, r := range rows {
		keys = append(keys, r.Key)
	}
	for _, d := range diags {
		rules = append(rules, d.Rule)
	}
	sort.Strings(keys)
	sort.Strings(rules)
	return keys, rules
}

func TestParseFilesTableWalk(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	good := withBase()
	cases := []struct {
		name  string
		files map[string]string
		links map[string]string
		keys  []string
		rules []string
	}{
		{
			name:  "20#corpus-documents-and-the-walk: only flowmark documents are rows",
			files: map[string]string{"tasks/T1.flowmark.md": good, "tasks/README.md": "# x\n", "tasks/notes.txt": "x"},
			keys:  []string{"T1"},
		},
		{
			name:  "20#corpus-documents-and-the-walk: hidden entries are outside the walk",
			files: map[string]string{"tasks/T1.flowmark.md": good, "tasks/.T9.flowmark.md": "junk", "tasks/.git/x.flowmark.md": "junk"},
			keys:  []string{"T1"},
		},
		{
			name:  "20#corpus-documents-and-the-walk: suffix in another case is a walk diagnostic",
			files: map[string]string{"tasks/T1.flowmark.md": good, "tasks/T2.FLOWMARK.md": good},
			keys:  []string{"T1"},
			rules: []string{RuleWalk},
		},
		{
			name:  "20#corpus-documents-and-the-walk: a document in a subfolder is a walk diagnostic",
			files: map[string]string{"tasks/T1.flowmark.md": good, "tasks/sub/T3.flowmark.md": good, "tasks/sub/x.md": "x"},
			keys:  []string{"T1"},
			rules: []string{RuleWalk},
		},
		{
			name:  "20#corpus-documents-and-the-walk: a symbolic link named as a document is a walk diagnostic and is not followed",
			files: map[string]string{"tasks/T1.flowmark.md": good},
			links: map[string]string{"tasks/L.flowmark.md": "T1.flowmark.md"},
			keys:  []string{"T1"},
			rules: []string{RuleWalk},
		},
		{
			name:  "20#corpus-documents-and-the-walk: a dangling symbolic link named as a document is a walk diagnostic",
			files: map[string]string{"tasks/T1.flowmark.md": good},
			links: map[string]string{"tasks/D.flowmark.md": "missing.flowmark.md"},
			keys:  []string{"T1"},
			rules: []string{RuleWalk},
		},
		{
			name:  "20#corpus-documents-and-the-walk: any other symbolic link is outside the database",
			files: map[string]string{"tasks/T1.flowmark.md": good},
			links: map[string]string{"tasks/L.md": "T1.flowmark.md", "tasks/other": "T1.flowmark.md"},
			keys:  []string{"T1"},
		},
		{
			name:  "20#corpus-documents-and-the-walk: a folder named as a document is a walk diagnostic",
			files: map[string]string{"tasks/T1.flowmark.md": good, "tasks/Dir.flowmark.md/": ""},
			keys:  []string{"T1"},
			rules: []string{RuleWalk},
		},
		{
			name:  "20#corpus-documents-and-the-walk: an invalid document is reported and is not a row",
			files: map[string]string{"tasks/T1.flowmark.md": good, "tasks/T4.flowmark.md": "# Title\n"},
			keys:  []string{"T1"},
			rules: []string{RuleBlock, RuleBlock},
		},
		{
			name:  "20#corpus-documents-and-the-walk: a symbolic link named as the table folder is a walk diagnostic",
			files: map[string]string{"elsewhere/T1.flowmark.md": good},
			links: map[string]string{"tasks": "elsewhere"},
			rules: []string{RuleWalk},
		},
		{
			name:  "20#corpus-documents-and-the-walk: a table folder that is a file is a walk diagnostic",
			files: map[string]string{"tasks": "x"},
			rules: []string{RuleWalk},
		},
		{
			name: "20#storage: a missing table folder has no rows and no diagnostic",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := tree(t, c.files, c.links)
			rows, diags := ParseFilesTable(cat, root, "tasks", lk)
			keys, rules := keysAndRules(rows, diags)
			if !reflect.DeepEqual(keys, c.keys) {
				t.Fatalf("keys = %v; want %v (diagnostics %+v)", keys, c.keys, diags)
			}
			if !reflect.DeepEqual(rules, c.rules) {
				t.Fatalf("diagnostic rules = %v; want %v: %+v", rules, c.rules, diags)
			}
			for _, d := range diags {
				if d.Line < 1 || d.Column < 1 || d.File == "" {
					t.Fatalf("diagnostic not located: %+v", d)
				}
			}
		})
	}
}

// writeCorpus writes a corpus root with the dbspec set and the files, and
// returns the root.
func writeCorpus(t *testing.T, dbs string, files map[string]string) string {
	t.Helper()
	files["dbspec/flowmark.dbs"] = dbs
	return tree(t, files, nil)
}

// notesDBS is a files table with an integer key and a non-null markdown section.
const notesDBS = `dbspec 1 flowmark

table notes {
  id i64 identity
  title varchar(64)
  body text
  summary text
  primary key (id)
  settings {
    store files
    title title
    body body
    markdown body
    markdown summary
  }
}
`

func TestIntegerKeysAndMandatorySection(t *testing.T) {
	root := writeCorpus(t, notesDBS, map[string]string{
		"notes/7.flowmark.md": "# Seven\n\n## summary\n\nx\n",
	})
	cat, diags := catalog.Load(root)
	if len(diags) > 0 {
		t.Fatalf("corpus: %+v", diags)
	}
	lk := stubLinker{}
	rows, diags := ParseFilesTable(cat, root, "notes", lk)
	if len(diags) > 0 || len(rows) != 1 || rows[0].Key != "7" || rows[0].Title != "Seven" || *rows[0].Sections["summary"] != "x\n" {
		t.Fatalf("rows = %+v, diagnostics %+v", rows, diags)
	}
	cases := []struct {
		name, rel, text, rule string
	}{
		{"20#keys: canonical integer name", "notes/7.flowmark.md", "# Seven\n\n## summary\n\nx\n", ""},
		{"10#sections-and-their-values: present empty mandatory section", "notes/8.flowmark.md", "# Eight\n\n## summary\n", ""},
		{"20#keys: integer name with a leading zero", "notes/007.flowmark.md", "# Seven\n\n## summary\n", RuleBlock},
		{"20#keys: negative integer name", "notes/-7.flowmark.md", "# Seven\n\n## summary\n", RuleBlock},
		{"20#keys: text name for an integer key", "notes/seven.flowmark.md", "# Seven\n\n## summary\n", RuleBlock},
		{"10#sections-and-their-values: mandatory section missing", "notes/9.flowmark.md", "# Nine\n", RuleBlock},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row, diags := ParseFile(cat, "notes", c.rel, []byte(c.text), lk)
			if c.rule == "" {
				if len(diags) > 0 || row == nil {
					t.Fatalf("refused: %+v", diags)
				}
				checkP2(t, cat, row, lk)
				return
			}
			if len(diags) == 0 || diags[0].Rule != c.rule || row != nil {
				t.Fatalf("row %v, diagnostics %+v; want rule %s and no row", row, diags, c.rule)
			}
		})
	}
	t.Run("the integer key writes its canonical name", func(t *testing.T) {
		row := &Row{Table: "notes", Key: "007", Title: "Seven", Sections: map[string]*string{"summary": sp("x\n")}}
		if _, diags := RenderFile(cat, "notes", row, lk); len(diags) == 0 || diags[0].Rule != RuleConstraint {
			t.Fatalf("a key 007 is refused on write; diagnostics %+v", diags)
		}
	})
	t.Run("a non-null section is written", func(t *testing.T) {
		row := &Row{Table: "notes", Key: "7", Title: "Seven", Sections: map[string]*string{}}
		if _, diags := RenderFile(cat, "notes", row, lk); len(diags) == 0 || diags[0].Rule != RuleConstraint {
			t.Fatalf("a missing non-null section is refused on write; diagnostics %+v", diags)
		}
	})
}

// TestKeysCollideWithoutRegardToCase pins 20#keys and 10#diagnostics: two rows
// whose keys differ only in case are both rows of the codec with no diagnostic;
// key.duplicate is the validator's rule, so the codec never makes a file
// unreadable for it. A case-insensitive file system holds one of the two files,
// so the test runs only where both names can be written.
func TestKeysCollideWithoutRegardToCase(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	root := tree(t, map[string]string{"tasks/T1.flowmark.md": withBase(), "tasks/t1.flowmark.md": withBase()}, nil)
	entries, err := os.ReadDir(filepath.Join(root, "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Skip("the file system is case-insensitive; the two names cannot coexist")
	}
	rows, diags := ParseFilesTable(cat, root, "tasks", lk)
	var keys []string
	for _, r := range rows {
		keys = append(keys, r.Key)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"T1", "t1"}) || len(diags) > 0 {
		t.Fatalf("rows = %v, diagnostics = %+v; want both rows and no diagnostic", keys, diags)
	}
}
