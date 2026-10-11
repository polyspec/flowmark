package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

// The equivalence dataset holds one table, entries, of five rows in five corpora
// under testdata/equivalence: files (one file per row), a document list and a
// document table (one file, entries.flowmark.md), and a block list and a block table
// (the entries under the plans of a files table). A block corpus adds the placement
// foreign key plan, which no row holds and which the shared columns do not include.
const (
	equivFiles         = "testdata/equivalence/files"
	equivDocument      = "testdata/equivalence/document"
	equivDocumentTable = "testdata/equivalence/document_table"
	equivBlock         = "testdata/equivalence/block"
	equivBlockTable    = "testdata/equivalence/block_table"
)

// equivKind is the storage family of a corpus: one file per row, one document file
// holding the rows, or a block of the rows inside the files of a parent table.
type equivKind int

const (
	kindFiles equivKind = iota
	kindDocument
	kindBlock
)

// equivCorpus is one corpus of the dataset: its root, its storage family, and its
// shape, list or table, for the document and block families.
type equivCorpus struct {
	name  string
	root  string
	kind  equivKind
	shape catalog.Shape
}

// equivCorpora are the five storages of the dataset. The files corpus comes first:
// its rows are the reference that every other corpus must equal.
var equivCorpora = []equivCorpus{
	{"files", equivFiles, kindFiles, ""},
	{"document list", equivDocument, kindDocument, catalog.ShapeList},
	{"document table", equivDocumentTable, kindDocument, catalog.ShapeTable},
	{"block list", equivBlock, kindBlock, catalog.ShapeList},
	{"block table", equivBlockTable, kindBlock, catalog.ShapeTable},
}

// equivCorpusAt returns the corpus whose root is root.
func equivCorpusAt(root string) equivCorpus {
	for _, c := range equivCorpora {
		if c.root == root {
			return c
		}
	}
	panic("no equivalence corpus at " + root)
}

// equivKeys are the keys of the shared rows.
var equivKeys = []string{"E1", "E2", "E3", "E4", "E5"}

// equivColumns are the stored scalar columns of entries, the order column and the
// parent foreign key among them. They cover text, NULL, int, bool and date.
var equivColumns = []string{"parent", "position", "kind", "label", "count", "done", "due"}

// equivShape is the value of a row on every column its three storages share: the
// title, the body, the scalar columns and the summary section. nil is NULL.
type equivShape struct {
	Title, Body string
	Scalars     map[string]*string
	Summary     *string
}

func shapeOf(r *Row) equivShape {
	s := equivShape{Title: r.Title, Body: r.Body, Scalars: map[string]*string{}, Summary: r.Sections["summary"]}
	for _, c := range equivColumns {
		s.Scalars[c] = r.Scalars[c]
	}
	return s
}

func (s equivShape) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "title=%q body=%q", s.Title, s.Body)
	for _, c := range equivColumns {
		fmt.Fprintf(&b, " %s=%s", c, nullable(s.Scalars[c]))
	}
	fmt.Fprintf(&b, " summary=%s", nullable(s.Summary))
	return b.String()
}

func nullable(p *string) string {
	if p == nil {
		return "NULL"
	}
	return fmt.Sprintf("%q", *p)
}

// equivLinker derives the link targets of the files corpus. The dataset has no
// detail section in that storage, so every target has no anchor; the document and
// block corpora write no link to another file.
func equivLinker() stubLinker {
	lk := stubLinker{}
	for _, k := range equivKeys {
		lk["entries/"+k] = stubRow{file: "entries/" + k + ".flowmark.md"}
	}
	return lk
}

// equivDocLinker derives the link targets of a document corpus whose rows are in
// file. A row with a detail section has the anchor of its heading; E4 has none, and
// no row links to it.
func equivDocLinker(file string) stubLinker {
	lk := stubLinker{}
	for _, k := range equivKeys {
		anchor := DetailAnchor("entries", k)
		if k == "E4" {
			anchor = ""
		}
		lk["entries/"+k] = stubRow{file: file, anchor: anchor}
	}
	return lk
}

// equivBlockLinker derives the link targets of a block corpus: all of its rows are
// in plans/P1.flowmark.md.
func equivBlockLinker() stubLinker { return equivDocLinker("plans/P1.flowmark.md") }

// equivGoldens are the documents of a corpus, relative to its root.
func equivGoldens(c equivCorpus) []string {
	switch c.kind {
	case kindFiles:
		var out []string
		for _, k := range equivKeys {
			out = append(out, "entries/"+k+".flowmark.md")
		}
		return out
	case kindDocument:
		return []string{"entries.flowmark.md"}
	default:
		return []string{"plans/P1.flowmark.md", "plans/P2.flowmark.md"}
	}
}

// renderGolden parses the golden document rel of corpus c with no diagnostic and
// returns the bytes its rows render to.
func renderGolden(t testing.TB, c equivCorpus, rel string, data []byte) []byte {
	t.Helper()
	cat := equivCatalog(t, c.root)
	switch c.kind {
	case kindFiles:
		lk := equivLinker()
		row, diags := ParseFile(cat, "entries", rel, data, lk)
		mustClean(t, rel, diags)
		out, rf := renderFileOf(t, cat, "entries", row, lk)
		mustClean(t, rel, rf)
		return out
	case kindDocument:
		lk := equivDocLinker("entries.flowmark.md")
		rows, diags, err := ParseDocument(cat, "entries", rel, data, lk)
		if err != nil {
			t.Fatal(err)
		}
		mustClean(t, rel, diags)
		out, rf, err := RenderDocument(cat, "entries", rows, lk)
		if err != nil {
			t.Fatal(err)
		}
		mustClean(t, rel, rf)
		return out
	default:
		lk := equivBlockLinker()
		row, diags := ParseFile(cat, "plans", rel, data, lk)
		mustClean(t, rel, diags)
		out, rf := renderFileOf(t, cat, "plans", row, lk)
		mustClean(t, rel, rf)
		return out
	}
}

func equivCatalog(t testing.TB, root string) *catalog.Catalog {
	t.Helper()
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("%s catalog: %+v", root, diags)
	}
	return cat
}

func mustClean[T Diagnostic | Refusal](t testing.TB, what string, diags []T) {
	t.Helper()
	if len(diags) > 0 {
		t.Fatalf("%s: %+v", what, diags)
	}
}

func readEquiv(t testing.TB, root, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// equivLoaded is one corpus read to its rows: its catalog, its linker, its rows of
// entries and, for a block corpus, its plans, the files whose child block holds the rows.
type equivLoaded struct {
	cat   *catalog.Catalog
	lk    Linker
	rows  []*Row
	plans []*Row
}

// loadCorpus parses the corpus c with no diagnostic. A files corpus has one row per
// file; a document corpus has the rows of its one file; a block corpus has the
// entries of its plans.
func loadCorpus(t testing.TB, c equivCorpus) equivLoaded {
	t.Helper()
	cat := equivCatalog(t, c.root)
	switch c.kind {
	case kindFiles:
		lk := equivLinker()
		rows, _, diags := ParseFilesTable(cat, DirFS(c.root), "entries", lk)
		mustClean(t, c.name, diags)
		return equivLoaded{cat: cat, lk: lk, rows: rows}
	case kindDocument:
		lk := equivDocLinker("entries.flowmark.md")
		data := readEquiv(t, c.root, "entries.flowmark.md")
		rows, diags, err := ParseDocument(cat, "entries", "entries.flowmark.md", data, lk)
		if err != nil {
			t.Fatal(err)
		}
		mustClean(t, c.name, diags)
		return equivLoaded{cat: cat, lk: lk, rows: rows}
	default:
		lk := equivBlockLinker()
		plans, _, diags := ParseFilesTable(cat, DirFS(c.root), "plans", lk)
		mustClean(t, c.name, diags)
		return equivLoaded{cat: cat, lk: lk, rows: blockRows(plans), plans: plans}
	}
}

// blockRows returns the entries of the plans, in plan order and then storage order.
func blockRows(plans []*Row) []*Row {
	var rows []*Row
	for _, p := range plans {
		rows = append(rows, p.Blocks["entries"]...)
	}
	return rows
}

// byKey returns the shape of each row by its key, failing on a row of another table
// and on two rows that share a key.
func byKey(t testing.TB, what string, rows []*Row) map[string]equivShape {
	t.Helper()
	out := map[string]equivShape{}
	for _, r := range rows {
		if r.Table != "entries" {
			t.Fatalf("%s: row %s is of table %s, want entries", what, r.Key, r.Table)
		}
		if _, dup := out[r.Key]; dup {
			t.Fatalf("%s: key %s names two rows", what, r.Key)
		}
		out[r.Key] = shapeOf(r)
	}
	return out
}

// requireSameRows requires got to hold the rows of want by key, each with an equal
// shape on every shared column, NULL included, and no other row.
func requireSameRows(t testing.TB, what string, want, got map[string]equivShape) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s holds %d rows, want %d", what, len(got), len(want))
	}
	for _, key := range equivKeys {
		w, wok := want[key]
		g, gok := got[key]
		switch {
		case !wok:
			t.Errorf("row %s is missing from the reference", key)
		case !gok:
			t.Errorf("%s: row %s is missing", what, key)
		case !reflect.DeepEqual(g, w):
			t.Errorf("%s: row %s differs\n got  %v\n want %v", what, key, g, w)
		}
	}
}

// TestEquivalenceCatalogs requires the five catalogs to declare the same columns of
// entries, the placement foreign key plan aside, each in the storage its corpus names.
func TestEquivalenceCatalogs(t *testing.T) {
	signature := func(tb *catalog.Table) []string {
		var out []string
		for _, c := range tb.Columns {
			if c.Name == "plan" {
				continue
			}
			out = append(out, fmt.Sprintf("%s %s null=%t identity=%t", c.Name, c.Type, c.Null, c.Identity))
		}
		return out
	}
	var want []string
	for i, c := range equivCorpora {
		cat := equivCatalog(t, c.root)
		tb, ok := cat.Table("entries")
		if !ok {
			t.Fatalf("%s: the catalog has no table entries", c.name)
		}
		got := signature(tb)
		if i == 0 {
			want = got
		} else if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: columns differ from the files corpus\n got  %q\n want %q", c.name, got, want)
		}
		switch c.kind {
		case kindFiles:
			if tb.Storage.Kind != catalog.StorageFiles {
				t.Errorf("%s: storage %s, want files", c.name, tb.Storage.Kind)
			}
		case kindDocument:
			if tb.Storage.Kind != catalog.StorageDocument || tb.Storage.Shape != c.shape {
				t.Errorf("%s: storage %s %s, want document %s", c.name, tb.Storage.Kind, tb.Storage.Shape, c.shape)
			}
		default:
			if tb.Storage.Kind != catalog.StorageBlock || tb.Storage.Shape != c.shape || tb.Storage.Parent != "plans" {
				t.Errorf("%s: storage %+v, want block %s under plans", c.name, tb.Storage, c.shape)
			}
		}
	}
}

// TestEquivalenceRowsAgree requires the five corpora to parse to the same rows: the
// same keys, and on every shared column the same value, NULL included.
func TestEquivalenceRowsAgree(t *testing.T) {
	var want map[string]equivShape
	for i, c := range equivCorpora {
		got := byKey(t, c.name, loadCorpus(t, c).rows)
		if i == 0 {
			want = got
			if len(want) != len(equivKeys) {
				t.Fatalf("the files corpus holds %d rows, want %d", len(want), len(equivKeys))
			}
			continue
		}
		requireSameRows(t, c.name, want, got)
	}
}

// roundTrip renders the rows of corpus c, checks that the rendering is stable
// (checkP2 for each file, checkDocumentP2 for the document), and returns the rows
// that the rendered bytes parse back to.
func roundTrip(t *testing.T, c equivCorpus, l equivLoaded) []*Row {
	t.Helper()
	switch c.kind {
	case kindFiles:
		back := make([]*Row, 0, len(l.rows))
		for _, row := range l.rows {
			out := checkP2(t, l.cat, row, l.lk)
			r, diags := ParseFile(l.cat, "entries", "entries/"+row.Key+".flowmark.md", out, l.lk)
			mustClean(t, "round trip of "+row.Key, diags)
			back = append(back, r)
		}
		return back
	case kindDocument:
		out := checkDocumentP2(t, l.cat, "entries", l.rows, l.lk)
		back, diags, err := ParseDocument(l.cat, "entries", "entries.flowmark.md", out, l.lk)
		if err != nil {
			t.Fatal(err)
		}
		mustClean(t, "round trip", diags)
		return back
	default:
		back := make([]*Row, 0, len(l.plans))
		for _, plan := range l.plans {
			out := checkP2(t, l.cat, plan, l.lk)
			r, diags := ParseFile(l.cat, "plans", "plans/"+plan.Key+".flowmark.md", out, l.lk)
			mustClean(t, "round trip of "+plan.Key, diags)
			back = append(back, r)
		}
		return blockRows(back)
	}
}

// TestEquivalenceRoundTrip requires that the rows of each corpus render and parse
// back to the same rows, and that the rendered bytes are emitted again unchanged.
func TestEquivalenceRoundTrip(t *testing.T) {
	for _, c := range equivCorpora {
		t.Run(c.name, func(t *testing.T) {
			l := loadCorpus(t, c)
			requireSameRows(t, c.name+" round trip", byKey(t, c.name, l.rows), byKey(t, c.name, roundTrip(t, c, l)))
		})
	}
}

// TestEquivalenceGoldens requires each document of each corpus to parse without a
// diagnostic and to render back to its own bytes.
func TestEquivalenceGoldens(t *testing.T) {
	for _, c := range equivCorpora {
		t.Run(c.name, func(t *testing.T) {
			for _, rel := range equivGoldens(c) {
				data := readEquiv(t, c.root, rel)
				if out := renderGolden(t, c, rel, data); !bytes.Equal(out, data) {
					t.Errorf("%s: render(parse(golden)) != golden\n got: %q\nwant: %q", rel, out, data)
				}
			}
		})
	}
}
