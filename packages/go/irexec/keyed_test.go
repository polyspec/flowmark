package irexec

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
)

// TestKeyedRows pins the rows a where that fixes the key reads (read.go keyedRows, spec 40 Cost model): a conjunction
// one of whose conditions compares the single-column key with a parameter for equality reads the rows of that key from
// the index, and every other where reads the table; either way the rows answered are the rows the same where answers
// when it reads the table, which an OR with a condition no row meets forces. A key of an integer type is met by the
// canonical text of its value, the only text an available table holds.
func TestKeyedRows(t *testing.T) {
	docs := baseDocs()
	docs["tags.flowmark.md"] = strings.Replace(docs["tags.flowmark.md"], "| 2 | ops | area |\n", "| 2 | ops | area |\n| 10 | ten | topic |\n", 1)
	eq := func(col string, i int) map[string]any { return pred(col, "eq", p(i)) }
	cases := []struct {
		name   string
		entity string
		items  []map[string]any
		params []any
		keyed  bool
	}{
		{"40#cost-model: the key of a text type", "items", []map[string]any{eq("id", 0)}, []any{"T2"}, true},
		{"40#cost-model: a key of another case meets no key", "items", []map[string]any{eq("id", 0)}, []any{"t2"}, true},
		{"40#cost-model: a key no row holds", "items", []map[string]any{eq("id", 0)}, []any{"T9"}, true},
		{"40#cost-model: the key and another condition", "items", []map[string]any{eq("id", 0), eq("state", 1)}, []any{"T1", "x"}, true},
		{"40#cost-model: the key after another condition", "items", []map[string]any{eq("state", 1), eq("id", 0)}, []any{"T1", " "}, true},
		{"40#cost-model: a NULL key meets no row", "items", []map[string]any{eq("id", 0)}, []any{nil}, true},
		{"40#cost-model: the key of a table with a soft delete", "people", []map[string]any{eq("id", 0)}, []any{"P1"}, true},
		{"40#cost-model: an integer key", "tags", []map[string]any{eq("id", 0)}, []any{2}, true},
		{"40#cost-model: an integer key of two digits", "tags", []map[string]any{eq("id", 0)}, []any{10}, true},
		{"40#cost-model: an integer key no row holds", "tags", []map[string]any{eq("id", 0)}, []any{1.0e1 + 1}, true},
		{"40#cost-model: an OR reads the table", "items", []map[string]any{eq("id", 0), {"pred": map[string]any{"column": "id", "op": "eq", "p": 1, "conn": "or"}}}, []any{"T1", "T3"}, false},
		{"40#cost-model: a key in a nested group reads the table", "items", []map[string]any{{"group": map[string]any{"items": []any{eq("id", 0)}}}}, []any{"T1"}, false},
		{"40#cost-model: a condition of another column reads the table", "items", []map[string]any{eq("state", 0)}, []any{"x"}, false},
		{"40#cost-model: a comparison of the key other than equality reads the table", "items", []map[string]any{pred("id", "gt", p(0))}, []any{"T2"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := catalogFixture(t, docs).on(t)
			rows := len(f.c.tableRecs(tc.entity))
			columns := map[string]any{"mode": "none", "add": []string{"id"}}
			items := make([]any, len(tc.items))
			for i, it := range tc.items {
				items[i] = it
			}
			f.c.cost = &cost.Counts{}
			got := answer(t, f.ok(f.msg("all", tc.entity, map[string]any{"columns": columns, "where": map[string]any{"items": items}}, tc.params...)))
			scanned := f.c.cost.Snapshot().Get(cost.RowsScanned)
			// The OR with a condition no row meets reads the table and answers the rows of the where alone.
			never := map[string]any{"pred": map[string]any{"column": "id", "op": "is_null", "conn": "or"}}
			want := answer(t, f.ok(f.msg("all", tc.entity, map[string]any{"columns": columns, "where": map[string]any{"items": []any{map[string]any{"group": map[string]any{"items": items}}, never}}}, tc.params...)))
			if got != want {
				t.Fatalf("the where answers %s, and read over the table %s", got, want)
			}
			if tc.keyed && scanned > 1 || !tc.keyed && scanned != int64(rows) {
				t.Fatalf("the where scanned %d rows of %d, keyed %v", scanned, rows, tc.keyed)
			}
		})
	}
}
