package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

// TestMemo requires a memoized parse to be the parse, reused only while its bytes and every link answer it asked for
// are the same (spec 40 Opening a corpus).
func TestMemo(t *testing.T) {
	cat := loadFixture(t)
	data, err := os.ReadFile(filepath.Join(fixtureRoot, "tasks", "T2.flowmark.md"))
	if err != nil {
		t.Fatal(err)
	}
	const rel = "tasks/T2.flowmark.md"
	parse := func(m *Memo, c *catalog.Catalog, b []byte, lk Linker) *Row {
		t.Helper()
		m.Begin(c)
		row, diags := m.ParseFile(c, "tasks", rel, b, lk)
		m.End()
		wantRow, wantDiags := ParseFile(c, "tasks", rel, b, lk)
		if !reflect.DeepEqual(row, wantRow) || !reflect.DeepEqual(diags, wantDiags) {
			t.Fatalf("the memo answered %+v %+v, the parse %+v %+v", row, diags, wantRow, wantDiags)
		}
		return row
	}
	lk := fixtureLinker()
	t.Run("40#opening-a-corpus: the same bytes and answers reuse the parse", func(t *testing.T) {
		m := NewMemo()
		if a, b := parse(m, cat, data, lk), parse(m, cat, data, lk); a != b {
			t.Fatal("the parse was not reused")
		}
	})
	t.Run("40#opening-a-corpus: other bytes are parsed again", func(t *testing.T) {
		m := NewMemo()
		a := parse(m, cat, data, lk)
		if b := parse(m, cat, append([]byte{}, append(data, "\nMore.\n"...)...), lk); a == b {
			t.Fatal("other bytes reused the parse")
		}
	})
	t.Run("40#opening-a-corpus: a link answered otherwise is parsed again", func(t *testing.T) {
		m := NewMemo()
		gone := stubLinker{}
		a := parse(m, cat, data, gone)
		if _, ok := a.Dangling["parent"]; !ok {
			t.Fatalf("the parent of T2 is not dangling without T1: %+v", a)
		}
		b := parse(m, cat, data, lk)
		if _, ok := b.Dangling["parent"]; ok || a == b {
			t.Fatalf("the row of T1 appeared and the parse was reused: %+v", b)
		}
		moved := stubLinker{"tasks/T1": {file: "tasks/T1.flowmark.md", anchor: "x"}}
		if c := parse(m, cat, data, moved); c == b {
			t.Fatal("a link target that changed reused the parse")
		}
	})
	t.Run("40#opening-a-corpus: a document a read did not parse is dropped", func(t *testing.T) {
		m := NewMemo()
		a := parse(m, cat, data, lk)
		m.Begin(cat)
		m.End()
		if b := parse(m, cat, data, lk); a == b {
			t.Fatal("a document dropped by its read was reused")
		}
	})
	t.Run("40#opening-a-corpus: another catalog drops every parse", func(t *testing.T) {
		m := NewMemo()
		a := parse(m, cat, data, lk)
		if b := parse(m, loadFixture(t), data, lk); a == b {
			t.Fatal("another catalog reused the parse")
		}
	})
	t.Run("40#opening-a-corpus: a nil memo parses every time", func(t *testing.T) {
		var m *Memo
		if a, b := parse(m, cat, data, lk), parse(m, cat, data, lk); a == b {
			t.Fatal("a nil memo reused a parse")
		}
	})
	t.Run("40#opening-a-corpus: a document's parse is reused and parsed again as a file's", func(t *testing.T) {
		hcat, diags := catalog.Load("testdata/historydoc")
		if len(diags) > 0 || hcat == nil {
			t.Fatalf("historydoc fixture catalog: %+v", diags)
		}
		text := []byte(raw("# item_history",
			"| id | item | from_state | to_state | at |",
			"| --- | --- | --- | --- | --- |",
			"| 1 | [T1](items.flowmark.md#items-row-t1) | waiting | done | 2026-10-01 08:00:00.000000 |",
			"", "## item_history row 1", "", "### evidence", "", "Checked."))
		doc := func(m *Memo, lk Linker) ([]*Row, []Diagnostic) {
			t.Helper()
			m.Begin(hcat)
			rows, diags, err := m.ParseDocument(hcat, "item_history", "item_history.flowmark.md", text, lk)
			m.End()
			if err != nil {
				t.Fatal(err)
			}
			wantRows, wantDiags, _ := ParseDocument(hcat, "item_history", "item_history.flowmark.md", text, lk)
			if !reflect.DeepEqual(rows, wantRows) || !reflect.DeepEqual(diags, wantDiags) {
				t.Fatalf("the memo answered %+v %+v, the parse %+v %+v", rows, diags, wantRows, wantDiags)
			}
			return rows, diags
		}
		m := NewMemo()
		withAnchor := stubLinker{"items/T1": {file: "items.flowmark.md", anchor: "items-row-t1"}}
		a, ad := doc(m, withAnchor)
		b, _ := doc(m, withAnchor)
		if len(ad) != 0 || a[0] != b[0] {
			t.Fatalf("the parse was not reused: %+v", ad)
		}
		// The row T1 lost its detail section: its link loses the anchor, so the written link is noncanonical.
		c, cd := doc(m, stubLinker{"items/T1": {file: "items.flowmark.md"}})
		if c[0] == a[0] || len(cd) == 0 {
			t.Fatalf("a link answered otherwise reused the parse: %+v", cd)
		}
	})
}
