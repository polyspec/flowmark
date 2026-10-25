package store

import (
	"bytes"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// tree writes files (a trailing slash makes a folder) and symlinks, each
// relative to a fresh corpus root, and returns the root.
func tree(t *testing.T, files map[string]string, links map[string]string) string {
	t.Helper()
	return treeIn(t, t.TempDir(), files, links)
}

// treeIn is tree in the empty folder root.
func treeIn(t *testing.T, root string, files map[string]string, links map[string]string) string {
	t.Helper()
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
	if err := corpustest.WriteSetFile(filepath.Join(root, "dbspec")); err != nil {
		t.Fatal(err)
	}
	return root
}

// wantRoundTrip requires the properties of a fuzz input data that emits out, whose parse emits again: P2, again is out
// and the second emit has no fault (faults), and, for an input that the parse reports nothing on (diags), P1, out is data.
func wantRoundTrip(t *testing.T, data, out, again []byte, faults, diags int) {
	t.Helper()
	if faults > 0 || !bytes.Equal(again, out) {
		t.Fatalf("P2 failed\ninput: %q\nemit: %q\nagain: %q", data, out, again)
	}
	if diags == 0 && !bytes.Equal(out, data) {
		t.Fatalf("P1 failed\ninput: %q\nemit: %q", data, out)
	}
}

// wantP1 requires the emit out of the parse of text to be text.
func wantP1(t *testing.T, out []byte, text string) {
	t.Helper()
	if !bytes.Equal(out, []byte(text)) {
		t.Fatalf("P1: emit(parse(s)) != s\n got: %q\nwant: %q", out, text)
	}
}

// wantKeysAndRules requires the sorted keys of rows to be keys and the sorted rules of diags to be rules.
func wantKeysAndRules(t *testing.T, rows []*Row, diags []Diagnostic, keys, rules []string) {
	t.Helper()
	gotKeys, gotRules := keysAndRules(rows, diags)
	if !reflect.DeepEqual(gotKeys, keys) {
		t.Fatalf("keys = %v; want %v (diagnostics %+v)", gotKeys, keys, diags)
	}
	if !reflect.DeepEqual(gotRules, rules) {
		t.Fatalf("diagnostic rules = %v; want %v: %+v", gotRules, rules, diags)
	}
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
			rows, _, diags := ParseFilesTable(cat, DirFS(root), "tasks", lk, nil)
			wantKeysAndRules(t, rows, diags, c.keys, c.rules)
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
	rows, _, diags := ParseFilesTable(cat, DirFS(root), "notes", lk, nil)
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
		if _, diags := renderFileOf(t, cat, "notes", row, lk); len(diags) == 0 || diags[0].Rule != RuleConstraint {
			t.Fatalf("a key 007 is refused on write; diagnostics %+v", diags)
		}
	})
	t.Run("a non-null section is written", func(t *testing.T) {
		row := &Row{Table: "notes", Key: "7", Title: "Seven", Sections: map[string]*string{}}
		if _, diags := renderFileOf(t, cat, "notes", row, lk); len(diags) == 0 || diags[0].Rule != RuleConstraint {
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
	root := treeIn(t, corpustest.CaseSensitiveRoot(t), map[string]string{"tasks/T1.flowmark.md": withBase(), "tasks/t1.flowmark.md": withBase()}, nil)
	entries, err := os.ReadDir(filepath.Join(root, "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("the folder holds %d entries; the two names differ only in case and must coexist", len(entries))
	}
	rows, _, diags := ParseFilesTable(cat, DirFS(root), "tasks", lk, nil)
	var keys []string
	for _, r := range rows {
		keys = append(keys, r.Key)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"T1", "t1"}) || len(diags) > 0 {
		t.Fatalf("rows = %v, diagnostics = %+v; want both rows and no diagnostic", keys, diags)
	}
}

// shardEvent is the file of an event at the time at.
func shardEvent(title, at string) string {
	return "# " + title + "\n- at: " + at + "\n"
}

// TestShardMissingFolder: a sharded table whose folder is missing has no rows and no diagnostic, read as a files table
// and as a document table (spec 20 Time shards).
func TestShardMissingFolder(t *testing.T) {
	cat, diags := catalog.Load("testdata/shard")
	if len(diags) > 0 {
		t.Fatalf("catalog: %+v", diags)
	}
	root := t.TempDir()
	rows, _, diags := ParseFilesTable(cat, DirFS(root), "events", stubLinker{}, nil)
	wantKeysAndRules(t, rows, diags, nil, nil)
	docs, diags, err := ParseDocumentTable(cat, DirFS(root), "notes", stubLinker{}, nil)
	if err != nil || len(docs) > 0 || len(diags) > 0 {
		t.Fatalf("documents %+v, diagnostics %+v, %v; want none", docs, diags, err)
	}
}

// TestParseFilesTableShard pins the walk of a sharded files table (spec 20 Time shards): its rows stand in
// the folder of their period, <table>/<period>/<key>.flowmark.md, the period named at the grain of the shard.
func TestParseFilesTableShard(t *testing.T) {
	cat, diags := catalog.Load("testdata/shard")
	if len(diags) > 0 || cat == nil {
		t.Fatalf("shard catalog: %+v", diags)
	}
	e1 := shardEvent("First", "2026-10-11 09:30:00.000000")
	cases := []struct {
		name  string
		files map[string]string
		keys  []string
		paths []string
		rules []string
	}{
		{name: "20#time-shards: a row in the folder of a period", files: map[string]string{"events/2026-10/E1.flowmark.md": e1},
			keys: []string{"E1"}, paths: []string{"events/2026-10/E1.flowmark.md"}},
		{name: "20#time-shards: the rows of two periods", files: map[string]string{"events/2026-10/E1.flowmark.md": e1, "events/2026-11/E2.flowmark.md": shardEvent("Second", "2026-11-02 08:00:00.000000")},
			keys: []string{"E1", "E2"}, paths: []string{"events/2026-10/E1.flowmark.md", "events/2026-11/E2.flowmark.md"}},
		{name: "20#time-shards: a row in another period's folder is still read, the validator judges it", files: map[string]string{"events/2026-11/E1.flowmark.md": e1},
			keys: []string{"E1"}, paths: []string{"events/2026-11/E1.flowmark.md"}},
		{name: "20#time-shards: a document directly in the table folder is a walk diagnostic", files: map[string]string{"events/E1.flowmark.md": e1},
			rules: []string{RuleWalk}},
		{name: "20#time-shards: a folder named as another grain is a walk diagnostic", files: map[string]string{"events/2026/E1.flowmark.md": e1, "events/2026-10-11/E2.flowmark.md": e1},
			rules: []string{RuleWalk, RuleWalk}},
		{name: "20#time-shards: a folder that is no period is a walk diagnostic", files: map[string]string{"events/2026-13/E1.flowmark.md": e1, "events/misc/E2.flowmark.md": e1},
			rules: []string{RuleWalk, RuleWalk}},
		{name: "20#time-shards: a document below a period folder is a walk diagnostic", files: map[string]string{"events/2026-10/sub/E1.flowmark.md": e1},
			rules: []string{RuleWalk}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := tree(t, c.files, nil)
			rows, _, diags := ParseFilesTable(cat, DirFS(root), "events", stubLinker{}, nil)
			wantKeysAndRules(t, rows, diags, c.keys, c.rules)
			var paths []string
			for _, r := range rows {
				paths = append(paths, r.Path)
			}
			sort.Strings(paths)
			if !reflect.DeepEqual(paths, c.paths) {
				t.Fatalf("row paths = %v; want %v", paths, c.paths)
			}
		})
	}
}

// shardNotes renders the document of notes at the corpus path rel; each note is key, title, written.
func shardNotes(t *testing.T, cat *catalog.Catalog, rel string, notes ...[3]string) string {
	t.Helper()
	var rows []*Row
	for _, n := range notes {
		w := n[2]
		rows = append(rows, &Row{Table: "notes", Key: n[0], Title: n[1], Scalars: map[string]*string{"written": &w}})
	}
	out, rf, err := RenderDocument(cat, "notes", rel, rows, stubLinker{})
	if err != nil || len(rf) > 0 {
		t.Fatalf("render %s: %v %+v", rel, err, rf)
	}
	return string(out)
}

// TestParseDocumentTableShard pins the walk of a sharded document table (spec 20 Time shards): the rows of a
// period stand in <table>/<period>.flowmark.md, the period named at the grain of the shard.
func TestParseDocumentTableShard(t *testing.T) {
	cat, diags := catalog.Load("testdata/shard")
	if len(diags) > 0 || cat == nil {
		t.Fatalf("shard catalog: %+v", diags)
	}
	y26 := shardNotes(t, cat, "notes/2026.flowmark.md", [3]string{"N1", "One", "2026-03-04"}, [3]string{"N2", "Two", "2026-09-30"})
	y27 := shardNotes(t, cat, "notes/2027.flowmark.md", [3]string{"N3", "Three", "2027-01-02"})
	cases := []struct {
		name  string
		files map[string]string
		links map[string]string
		keys  []string
		paths []string
		rules []string
	}{
		{name: "20#time-shards: the rows of a period in its document", files: map[string]string{"notes/2026.flowmark.md": y26},
			keys: []string{"N1", "N2"}, paths: []string{"notes/2026.flowmark.md", "notes/2026.flowmark.md"}},
		{name: "20#time-shards: the documents of two periods", files: map[string]string{"notes/2026.flowmark.md": y26, "notes/2027.flowmark.md": y27},
			keys: []string{"N1", "N2", "N3"}, paths: []string{"notes/2026.flowmark.md", "notes/2026.flowmark.md", "notes/2027.flowmark.md"}},
		{name: "20#time-shards: a document named as another grain is a walk diagnostic", files: map[string]string{"notes/2026-03.flowmark.md": y26},
			rules: []string{RuleWalk}},
		{name: "20#time-shards: a document named as no period is a walk diagnostic", files: map[string]string{"notes/misc.flowmark.md": y26, "notes/N1.flowmark.md": y26},
			rules: []string{RuleWalk, RuleWalk}},
		{name: "20#time-shards: a folder in the table folder is a walk diagnostic", files: map[string]string{"notes/2026/N1.flowmark.md": y26},
			rules: []string{RuleWalk}},
		{name: "20#time-shards: a symbolic link named as a period is a walk diagnostic and is not followed", files: map[string]string{"notes/2027.flowmark.md": y27},
			links: map[string]string{"notes/2026.flowmark.md": "2027.flowmark.md"}, keys: []string{"N3"}, paths: []string{"notes/2027.flowmark.md"}, rules: []string{RuleWalk}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := tree(t, c.files, c.links)
			docs, diags, err := ParseDocumentTable(cat, DirFS(root), "notes", stubLinker{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var rows []*Row
			var paths []string
			for _, d := range docs {
				for _, r := range d.Rows {
					rows = append(rows, r)
					paths = append(paths, d.Path)
				}
			}
			wantKeysAndRules(t, rows, diags, c.keys, c.rules)
			sort.Strings(paths)
			if !reflect.DeepEqual(paths, c.paths) {
				t.Fatalf("paths = %v; want %v", paths, c.paths)
			}
		})
	}
}
