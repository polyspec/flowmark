package store

import (
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// equivalenceCatalog loads the catalog of the equivalence corpus name, whose entries table has the order column
// position in the storage that the name gives.
func equivalenceCatalog(t testing.TB, name string) *catalog.Catalog {
	t.Helper()
	cat, diags := catalog.Load("testdata/equivalence/" + name)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("%s catalog: %+v", name, diags)
	}
	return cat
}

// entryList is a document of the entries list with the given rows.
func entryList(rows ...string) string { return raw(append([]string{"# entries"}, rows...)...) }

// entryTable is a document of the entries table with the given rows.
func entryTable(rows ...string) string {
	return raw(append([]string{"# entries", "| id | parent | title | position | kind | label | count | done | due |", "| --- | --- | --- | --- | --- | --- | --- | --- | --- |"}, rows...)...)
}

// entryCells is a row of the entries table: its key, its parent, its order value and the cells that follow.
func entryCells(key, parent, position string) string {
	if parent != "" {
		parent = "[" + parent + "](entries.flowmark.md)"
	}
	return row(key, parent, "Row "+key, position, "alpha", "", "1", "false", "")
}

// TestOrderStored pins the order column in block and document storages (spec 20 Order): the column is written where any
// scalar is, a field of a list row or a cell of a table, as any integer of its type; the siblings stand by ascending
// value, those that share a value in key order, so a document in another order is format.noncanonical; a row without
// its value is format.block, as any non-null column is.
func TestOrderStored(t *testing.T) {
	cases := []struct {
		name    string
		storage string
		text    string
		rule    string // "" when the document is accepted
		line    int
	}{
		{"20#order: a list in ascending order of its values", "document",
			entryList("- E2 B position:-3 kind:a count:1 done:false", "- E1 A position:7 kind:a count:1 done:false"), "", 0},
		{"20#order: list siblings that share a value in key order", "document",
			entryList("- E1 A position:2 kind:a count:1 done:false", "- E2 B position:2 kind:a count:1 done:false"), "", 0},
		{"20#order: the children of a list row by their own values", "document",
			entryList("- E1 A position:1 kind:a count:1 done:false", "  - E3 C position:1 kind:a count:1 done:false", "  - E2 B position:5 kind:a count:1 done:false"), "", 0},
		{"20#order: a list out of the order of its values", "document",
			entryList("- E1 A position:7 kind:a count:1 done:false", "- E2 B position:-3 kind:a count:1 done:false"), RuleNoncanonical, 0},
		{"20#order: list siblings that share a value out of key order", "document",
			entryList("- E2 B position:2 kind:a count:1 done:false", "- E1 A position:2 kind:a count:1 done:false"), RuleNoncanonical, 0},
		{"20#order: a list row without its order value", "document",
			entryList("- E1 A kind:a count:1 done:false"), RuleBlock, 2},
		{"20#order: a table in ascending order of its values", "document_table",
			entryTable(entryCells("E2", "", "1"), entryCells("E1", "", "2")), "", 0},
		{"20#order: table siblings that share a value in key order", "document_table",
			entryTable(entryCells("E1", "", "2"), entryCells("E3", "E1", "9"), entryCells("E2", "", "2")), "", 0},
		{"20#order: a table out of the order of its values", "document_table",
			entryTable(entryCells("E1", "", "2"), entryCells("E2", "", "1")), RuleNoncanonical, 0},
		{"20#order: a table row without its order value", "document_table",
			entryTable(entryCells("E1", "", "")), RuleBlock, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cat := equivalenceCatalog(t, c.storage)
			rows, diags, err := ParseDocument(cat, "entries", "entries.flowmark.md", []byte(c.text), stubLinker{})
			if err != nil {
				t.Fatal(err)
			}
			if c.rule == "" {
				if len(diags) > 0 {
					t.Fatalf("accepted input has diagnostics: %+v", diags)
				}
				out := checkDocumentP2(t, cat, "entries", rows, stubLinker{})
				wantP1(t, out, c.text)
				return
			}
			if len(diags) == 0 || diags[0].Rule != c.rule {
				t.Fatalf("diagnostics %+v; want %s first", diags, c.rule)
			}
			if c.line != 0 && diags[0].Line != c.line {
				t.Fatalf("first diagnostic at line %d; want %d: %+v", diags[0].Line, c.line, diags[0])
			}
		})
	}
}

// TestOrderUnreadable pins an order value not of its column's type (spec 20 Order): the row keeps it as written, the
// validator's type diagnostic, and stands after the siblings whose value is readable.
func TestOrderUnreadable(t *testing.T) {
	cat := equivalenceCatalog(t, "document")
	text := entryList("- E2 B position:9 kind:a count:1 done:false", "- E1 A position:x kind:a count:1 done:false")
	rows, diags, err := ParseDocument(cat, "entries", "entries.flowmark.md", []byte(text), stubLinker{})
	if err != nil || len(diags) > 0 {
		t.Fatalf("parse: %v %+v", err, diags)
	}
	if len(rows) != 2 || rows[1].Key != "E1" || rows[1].Raw["position"] != "x" {
		t.Fatalf("rows %+v; want E1 last, holding x as written", rows)
	}
}

// TestOrderRendered pins the writer of an order column (spec 20 Order): rows given in any order are written by
// ascending value, those that share a value in key order, and a value not of the column's type is refused.
func TestOrderRendered(t *testing.T) {
	entry := func(key, position string) *Row {
		sc := map[string]*string{"position": sp(position), "kind": sp("a"), "count": sp("1"), "done": sp("false")}
		return &Row{Table: "entries", Key: key, Title: "Row " + key, Scalars: sc, Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
	}
	cat := equivalenceCatalog(t, "document")
	t.Run("20#order: the rows written by value, then by key", func(t *testing.T) {
		out, refused, err := RenderDocument(cat, "entries", "entries.flowmark.md", []*Row{entry("E3", "2"), entry("E1", "9"), entry("E2", "2"), entry("E4", "-1")}, stubLinker{})
		if err != nil || len(refused) > 0 {
			t.Fatalf("render: %v %+v", err, refused)
		}
		want := entryList("- E4 Row E4 position:-1 kind:a count:1 done:false", "- E2 Row E2 position:2 kind:a count:1 done:false",
			"- E3 Row E3 position:2 kind:a count:1 done:false", "- E1 Row E1 position:9 kind:a count:1 done:false")
		if string(out) != want {
			t.Fatalf("written\n%s\nwant\n%s", out, want)
		}
	})
	t.Run("20#order: a value not of the column's type is refused", func(t *testing.T) {
		_, refused, err := RenderDocument(cat, "entries", "entries.flowmark.md", []*Row{entry("E1", "3000000000")}, stubLinker{})
		if err != nil {
			t.Fatal(err)
		}
		if len(refused) != 1 || refused[0].Column != "position" || refused[0].Rule != RuleConstraint {
			t.Fatalf("refusals %+v; want one CONSTRAINT at position", refused)
		}
	})
}
