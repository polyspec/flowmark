package irexec

import (
	"strings"
	"testing"
)

// TestNoAssignment pins that the engine assigns no value a database does not assign (spec 20 Keys and Order, plan
// Ruling: the engine holds only what the common databases hold): an insert gives its text key and its order column as it
// gives any non-null column without a default, a read answers the order column as written, and a write renumbers no
// sibling.
func TestNoAssignment(t *testing.T) {
	cases := []struct {
		name   string
		entity string
		set    []any
		params []any
		code   string // "" when the insert is accepted
		answer string
	}{
		{"20#keys: an insert without its text key is refused", "people", sets("name"), []any{"Eve"}, "IR_INVALID", ""},
		{"20#keys: an insert with its text key is accepted", "people", sets("id", "name"), []any{"P9", "Eve"}, "", `{"affected":1,"keys":["P9"]}`},
		{"20#keys: a list insert without its text key is refused", "items", sets("title", "body", "position"), []any{"Fifth", "", 5}, "IR_INVALID", ""},
		{"20#order: a list insert without its order column is refused", "items", sets("id", "title", "body"), []any{"T5", "Fifth", ""}, "IR_INVALID", ""},
		{"20#order: a list insert with any integer of its order column is accepted", "items", sets("id", "title", "body", "position"), []any{"T5", "Fifth", "", 50}, "", `{"affected":1,"keys":["T5"]}`},
		{"20#order: a files insert without its order column is refused", "notes", sets("id", "title", "body"), []any{"N4", "Fourth", ""}, "IR_INVALID", ""},
		{"20#order: a files insert with any integer of its order column is accepted", "notes", sets("id", "title", "body", "position"), []any{"N4", "Fourth", "", 70}, "", `{"affected":1,"keys":["N4"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh(t)
			m := f.msg("insert", tc.entity, map[string]any{"set": tc.set}, tc.params...)
			if tc.code != "" {
				if e := f.failure(m, tc.code); !strings.Contains(e.Msg, "is not set") {
					t.Fatalf("refusal %q; want a required column that is not set", e.Msg)
				}
				return
			}
			f.want(m, tc.answer)
		})
	}
	t.Run("20#order: a read answers the order column as written", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "position")}, "N4", "Fourth", "", 70), `{"affected":1,"keys":["N4"]}`))
		if got := f.file("notes/N4.flowmark.md"); got != lines("# Fourth", "- position: 70") {
			t.Fatalf("N4:\n%s", got)
		}
		f.want(notePositions(f),
			`{"rows":[{"id":"N1","parent":null,"position":1},{"id":"N2","parent":"N1","position":1},{"id":"N3","parent":null,"position":2},{"id":"N4","parent":null,"position":70}]}`)
	})
	t.Run("20#order: an update of the order column renumbers no sibling", func(t *testing.T) {
		f := fresh(t)
		res := f.ok(f.msg("update", "notes", map[string]any{"set": sets("position"), "where": where(pred("id", "eq", p(1)))}, 1, "N3"))
		wantTargets(t, res, "notes/N3.flowmark.md")
		f.commit(res)
		f.want(notePositions(f),
			`{"rows":[{"id":"N1","parent":null,"position":1},{"id":"N2","parent":"N1","position":1},{"id":"N3","parent":null,"position":1}]}`)
	})
}
