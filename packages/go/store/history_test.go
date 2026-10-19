package store

import (
	"bytes"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// TestHistoryDocumentTable requires that a history table stored `document table`
// reads and writes its rows: a history row with the link to its state row and a
// required markdown column as its evidence section round-trips (P1, P2).
func TestHistoryDocumentTable(t *testing.T) {
	cat, diags := catalog.Load("testdata/historydoc")
	if len(diags) > 0 || cat == nil {
		t.Fatalf("historydoc fixture catalog: %+v", diags)
	}
	lk := stubLinker{"items/T1": {file: "items.flowmark.md", anchor: "items-row-t1"}}
	text := raw("# item_history",
		"| id | item | from_state | to_state | at |",
		"| --- | --- | --- | --- | --- |",
		"| 1 | [T1](items.flowmark.md#items-row-t1) | waiting | done | 2026-10-01 08:00:00.000000 |",
		"", "## item_history row 1", "", "### evidence", "", "Checked.")
	t.Run("30#the-history-table: a history row of a document table round-trips with its link and its evidence section", func(t *testing.T) {
		if tb, _ := cat.Table("item_history"); tb.RowScope != 3 {
			t.Fatalf("history row scope %d; want 3", tb.RowScope)
		}
		rows, diags, err := ParseDocument(cat, "item_history", "item_history.flowmark.md", []byte(text), lk)
		if err != nil || len(diags) > 0 || len(rows) != 1 {
			t.Fatalf("read: %v %+v rows %d", err, diags, len(rows))
		}
		if ev := rows[0].Sections["evidence"]; ev == nil || *ev != "Checked.\n" || *rows[0].Scalars["item"] != "T1" {
			t.Fatalf("history row %+v", rows[0])
		}
		out := checkDocumentP2(t, cat, "item_history", rows, lk)
		if !bytes.Equal(out, []byte(text)) {
			t.Fatalf("P1: written %q; want %q", out, text)
		}
	})
}
