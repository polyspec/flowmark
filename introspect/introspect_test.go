package introspect_test

import (
	"github.com/polyspec/flowmark/internal/corpustest"
	"os"
	"path/filepath"
	"testing"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/introspect"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// coreDbs and flowmarkDbs are a two-document catalog in dbspec canonical form:
// flowmark uses core, and both carry a comment, which canonical form keeps.
const coreDbs = `dbspec 1 core

# the people of the team
table people {
  id varchar(16)
  name varchar(64)
  primary key (id)
  settings {
    store document table
    title name
  }
}
`

const flowmarkDbs = `dbspec 1 flowmark

use core { people }

table notes {
  id varchar(16)
  owner varchar(16)
  title varchar(255)
  primary key (id)
  index ix_notes_owner (owner)
  foreign key fk_notes_owner (owner) references people (id) on delete restrict on update restrict
  settings {
    store document list
    title title
  }
}
`

const peopleDoc = "# people\n| id | name |\n| --- | --- |\n| P1 | Ada |\n"

const notesDoc = "# notes\n- N1 First owner:[P1](people.flowmark.md)\n"

// writeTree writes files, keyed by their slash-separated corpus path, under a
// fresh root and returns the root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, text := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := corpustest.WriteSetFile(filepath.Join(root, "dbspec")); err != nil {
		t.Fatal(err)
	}
	return root
}

func validTree() map[string]string {
	return map[string]string{
		"dbspec/core.dbs":     coreDbs,
		"dbspec/flowmark.dbs": flowmarkDbs,
		"people.flowmark.md":  peopleDoc,
		"notes.flowmark.md":   notesDoc,
	}
}

// requireCatalog checks the returned catalog of validTree: both documents in
// file name order with their paths and names, each in canonical form, and a
// manifest hash that the returned texts read back to.
func requireCatalog(t *testing.T, res *introspect.Result) {
	t.Helper()
	if res.Catalog == nil {
		t.Fatalf("no catalog; diagnostics %+v", res.Diagnostics)
	}
	want := []introspect.Document{
		{File: "dbspec/core.dbs", Name: "core", Text: coreDbs},
		{File: "dbspec/flowmark.dbs", Name: "flowmark", Text: flowmarkDbs},
	}
	got := res.Catalog.Documents
	if len(got) != len(want) {
		t.Fatalf("documents = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("document %d = %+v,\nwant %+v", i, got[i], want[i])
		}
	}
	texts := map[string]string{}
	for _, d := range got {
		texts[d.Name] = d.Text
	}
	var docs []*dbspec.Document
	for _, d := range got {
		others := map[string]string{}
		for n, x := range texts {
			if n != d.Name {
				others[n] = x
			}
		}
		parsed, _, diags := dbspec.Parse(d.Text, others)
		if len(diags) > 0 {
			t.Fatalf("returned %s does not parse: %+v", d.File, diags)
		}
		docs = append(docs, parsed)
	}
	m, diags := dbspec.ManifestOf(docs)
	if len(diags) > 0 || m == nil {
		t.Fatalf("returned set has no manifest: %+v", diags)
	}
	if res.Catalog.ManifestHash == "" || m.ManifestHash != res.Catalog.ManifestHash {
		t.Fatalf("manifest hash %q, the returned set reads back to %q", res.Catalog.ManifestHash, m.ManifestHash)
	}
}

func TestValidCorpusReturnsItsCatalog(t *testing.T) {
	res, err := introspect.Corpus(writeTree(t, validTree()))
	if err != nil {
		t.Fatal(err)
	}
	requireCatalog(t, res)
	if len(res.Diagnostics) != 0 {
		t.Fatalf("diagnostics for a valid corpus: %+v", res.Diagnostics)
	}
}

func TestNonCanonicalCatalogReturnsCanonicalForm(t *testing.T) {
	files := validTree()
	files["dbspec/core.dbs"] = "dbspec 1 core\n\n\n# the people of the team\ntable people {\n  id varchar(16)\n  name varchar(64)\n  primary key (id)\n  settings {\n    store document table\n    title name\n  }\n}"
	res, err := introspect.Corpus(writeTree(t, files))
	if err != nil {
		t.Fatal(err)
	}
	requireCatalog(t, res)
}

func TestFailingDocumentKeepsTheCatalog(t *testing.T) {
	files := validTree()
	files["people.flowmark.md"] = peopleDoc + "| P1 | Bea |\n"
	res, err := introspect.Corpus(writeTree(t, files))
	if err != nil {
		t.Fatal(err)
	}
	requireCatalog(t, res)
	found := false
	for _, d := range res.Diagnostics {
		if d.File == "people.flowmark.md" && d.Rule == "key.duplicate" && d.Line > 0 && d.Column > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no located key.duplicate in %+v", res.Diagnostics)
	}
}

func TestCatalogThatDoesNotLoad(t *testing.T) {
	files := validTree()
	files["dbspec/core.dbs"] = coreDbs + "table broken {\n"
	files["people.flowmark.md"] = "not a document\n"
	res, err := introspect.Corpus(writeTree(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if res.Catalog != nil {
		t.Fatalf("a catalog that does not load returned %+v", res.Catalog)
	}
	if len(res.Diagnostics) == 0 {
		t.Fatal("no diagnostic for a catalog that does not load")
	}
	for _, d := range res.Diagnostics {
		if d.Rule != "catalog" {
			t.Fatalf("a %s diagnostic beside a catalog that does not load: %+v", d.Rule, d)
		}
	}
}

func TestCorpusWithoutCatalog(t *testing.T) {
	res, err := introspect.Corpus(writeTree(t, map[string]string{"people.flowmark.md": peopleDoc}))
	if err != nil {
		t.Fatal(err)
	}
	want := []catalog.Diagnostic{{File: "dbspec", Line: 1, Column: 1, Rule: "catalog", Message: "no dbspec/ directory"}}
	if res.Catalog != nil || len(res.Diagnostics) != 1 || res.Diagnostics[0] != want[0] {
		t.Fatalf("catalog %+v, diagnostics %+v; want none and %+v", res.Catalog, res.Diagnostics, want)
	}
}
