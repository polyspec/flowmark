package irexec

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// none is the projection of the keys only.
var none = map[string]any{"mode": "none"}

// ids runs an all read of entity with the where w and returns the ids it answers.
func (f *fixture) ids(entity string, w map[string]any, params ...any) []string {
	f.t.Helper()
	body := map[string]any{"columns": none}
	if w != nil {
		body["where"] = w
	}
	res := f.ok(f.msg("all", entity, body, params...))
	var out []string
	for _, r := range res.Answer.Rows {
		v, _ := r.Get("id")
		switch x := v.(type) {
		case string:
			out = append(out, x)
		case json.Number:
			out = append(out, x.String())
		}
	}
	return out
}

// TestReadKinds pins every read kind of the protocol with an accepting case: one,
// all, count, group_count, sum, avg and paginate (orm protocol 1 and spec 60
// Surfaces).
func TestReadKinds(t *testing.T) {
	f := baseFixture(t)
	f.sub(t, "60#surfaces: all answers the matching rows in storage order", func(f *fixture) {
		f.want(f.msg("all", "items", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"state"}}}),
			`{"rows":[{"id":"T1","parent":null,"owner":"P1","state":"done"},{"id":"T2","parent":"T1","owner":null,"state":"doing"},{"id":"T3","parent":null,"owner":null,"state":"waiting"},{"id":"T4","parent":null,"owner":null,"state":"dropped"}]}`)
	})
	f.sub(t, "60#surfaces: all answers every default column in field order", func(f *fixture) {
		f.want(f.msg("all", "items", map[string]any{"where": where(pred("id", "eq", p(0)))}, "T1"),
			`{"rows":[{"id":"T1","parent":null,"state":"done","title":"Ship the parser","body":"Parser body.","position":1,"owner":"P1","due":null,"evidence":"Golden tests pass.","cause":null}]}`)
	})
	f.sub(t, "60#surfaces: one answers the first row by the order", func(f *fixture) {
		f.want(f.msg("one", "items", map[string]any{"columns": none, "order": []any{map[string]any{"column": "title", "desc": true}}}),
			`{"rows":[{"id":"T2","parent":"T1","owner":null}]}`)
	})
	f.sub(t, "60#surfaces: one answers no row when none matches", func(f *fixture) {
		f.want(f.msg("one", "items", map[string]any{"columns": none, "where": where(pred("id", "eq", p(0)))}, "T9"), `{"rows":[]}`)
	})
	f.sub(t, "60#surfaces: count counts the matching rows", func(f *fixture) {
		f.want(f.msg("count", "items", map[string]any{"where": where(pred("state", "in", map[string]any{"ps": []int{0, 1}}))}, "doing", "done"), `{"count":2}`)
	})
	f.sub(t, "60#surfaces: count with group_by counts the groups", func(f *fixture) {
		f.want(f.msg("count", "items", map[string]any{"group_by": []string{"owner"}}), `{"count":2}`)
	})
	f.sub(t, "60#surfaces: group_count groups NULLs together", func(f *fixture) {
		f.want(f.msg("group_count", "items", map[string]any{"group_by": []string{"owner"}}),
			`{"rows":[{"owner":"P1","row_count":1},{"owner":null,"row_count":3}]}`)
	})
	f.sub(t, "60#surfaces: sum adds the values in the column type, skipping soft-deleted rows", func(f *fixture) {
		f.want(f.msg("sum", "people", map[string]any{"agg": "score"}), `{"value":"1.50"}`)
	})
	f.sub(t, "60#surfaces: sum of no value is 0", func(f *fixture) {
		f.want(f.msg("sum", "people", map[string]any{"agg": "score", "where": where(pred("id", "eq", p(0)))}, "P9"), `{"value":"0.00"}`)
	})
	f.sub(t, "60#surfaces: avg skips NULLs and is an f64", func(f *fixture) {
		f.want(f.msg("avg", "people", map[string]any{"agg": "age"}), `{"value":36}`)
	})
	f.sub(t, "60#surfaces: avg of no value is NULL", func(f *fixture) {
		f.want(f.msg("avg", "people", map[string]any{"agg": "age", "where": where(pred("id", "eq", p(0)))}, "P2"), `{"value":null}`)
	})
	f.sub(t, "60#surfaces: paginate answers the page and the count of every matching row", func(f *fixture) {
		f.want(f.msg("paginate", "items", map[string]any{"columns": none, "limit": map[string]any{"offset": 1, "count": 2}}),
			`{"rows":[{"id":"T2","parent":"T1","owner":null},{"id":"T3","parent":null,"owner":null}],"count":4}`)
	})
	f.sub(t, "60#surfaces: a lock update is accepted as a no-op", func(f *fixture) {
		f.want(f.msg("one", "tags", map[string]any{"lock": "update"}), `{"rows":[{"id":1,"label":"go","kind":"topic"}]}`)
	})
}

// TestReadRejects pins the refusals of the read kinds.
func TestReadRejects(t *testing.T) {
	f := baseFixture(t)
	cases := []struct {
		name string
		msg  []byte
		code string
	}{
		{"60#surfaces: sum of a text column is not allowed", f.msg("sum", "people", map[string]any{"agg": "name"}), "OPERATOR_NOT_ALLOWED"},
		{"60#surfaces: avg of a bool column is not allowed", f.msg("avg", "people", map[string]any{"agg": "active"}), "OPERATOR_NOT_ALLOWED"},
		{"60#surfaces: avg of an unknown column", f.msg("avg", "people", map[string]any{"agg": "nope"}), "COLUMN_UNKNOWN"},
		{"60#surfaces: group_count needs group_by", f.msg("group_count", "items", nil), "IR_INVALID"},
		{"60#surfaces: paginate with a limit of no row is invalid", f.msg("paginate", "items", map[string]any{"limit": map[string]any{"offset": 0, "count": 0}}), "IR_INVALID"},
		{"60#surfaces: lock update_nowait is unsupported", f.msg("one", "items", map[string]any{"lock": "update_nowait"}), "CAPABILITY_UNSUPPORTED"},
		{"60#surfaces: lock share_nowait is unsupported", f.msg("all", "items", map[string]any{"lock": "share_nowait"}), "CAPABILITY_UNSUPPORTED"},
		{"60#surfaces: force_index is unsupported", f.msg("all", "items", map[string]any{"force_index": "ix_items_owner"}), "CAPABILITY_UNSUPPORTED"},
		{"60#surfaces: force_index of an undeclared index is unknown", f.msg("all", "items", map[string]any{"force_index": "ix_nope"}), "INDEX_UNKNOWN"},
		{"60#surfaces: an unknown entity", f.msg("all", "nope", nil), "ENTITY_UNKNOWN"},
		{"60#surfaces: an unknown column", f.msg("all", "items", map[string]any{"where": where(pred("nope", "eq", p(0)))}, "x"), "COLUMN_UNKNOWN"},
		{"60#surfaces: group_by is invalid on all", f.msg("all", "items", map[string]any{"group_by": []string{"owner"}}), "IR_INVALID"},
		{"60#surfaces: group_by is invalid on one", f.msg("one", "items", map[string]any{"group_by": []string{"owner"}}), "IR_INVALID"},
		{"60#surfaces: group_by is invalid on sum", f.msg("sum", "people", map[string]any{"agg": "score", "group_by": []string{"active"}}), "IR_INVALID"},
		{"60#surfaces: group_by is invalid in a subquery", f.msg("all", "people", map[string]any{"where": where(pred("id", "in", map[string]any{"sub": map[string]any{"query": map[string]any{"entity": "items", "group_by": []string{"owner"}}, "column": "owner"}}))}), "IR_INVALID"},
		{"60#surfaces: a group order by a column outside the group is invalid", f.msg("group_count", "items", map[string]any{"group_by": []string{"owner"}, "order": []any{map[string]any{"column": "state"}}}), "IR_INVALID"},
	}
	for _, c := range cases {
		f.sub(t, c.name, func(f *fixture) { f.fails(c.msg, c.code) })
	}
}

// TestWhere pins every predicate form of the IR Where (orm protocol 1.3) with an
// accepting case, under SQL's three-valued logic.
func TestWhere(t *testing.T) {
	f := baseFixture(t)
	type pc struct {
		name   string
		entity string
		w      map[string]any
		params []any
		want   []string
	}
	ref := func(path, col string) map[string]any {
		return map[string]any{"ref": map[string]any{"path": path, "column": col}}
	}
	cases := []pc{
		{"60#surfaces: eq", "items", where(pred("state", "eq", p(0))), []any{"doing"}, []string{"T2"}},
		{"60#surfaces: not_eq", "items", where(pred("state", "not_eq", p(0))), []any{"doing"}, []string{"T1", "T3", "T4"}},
		{"60#surfaces: gt compares text by code point", "items", where(pred("title", "gt", p(0))), []any{"S"}, []string{"T1", "T2"}},
		{"60#surfaces: gte on a date", "items", where(pred("due", "gte", p(0))), []any{"2026-10-20"}, []string{"T2"}},
		{"60#surfaces: lt with NULLs unknown", "items", where(pred("due", "lt", p(0))), []any{"2026-10-21"}, []string{"T2"}},
		{"60#surfaces: lte", "items", where(pred("due", "lte", p(0))), []any{"2026-10-19"}, nil},
		{"60#surfaces: in", "items", where(pred("state", "in", map[string]any{"ps": []int{0, 1}})), []any{"waiting", "dropped"}, []string{"T3", "T4"}},
		{"60#surfaces: not_in", "items", where(pred("state", "not_in", map[string]any{"ps": []int{0, 1}})), []any{"waiting", "dropped"}, []string{"T1", "T2"}},
		{"60#surfaces: not_in with a NULL is never true", "items", where(pred("owner", "not_in", map[string]any{"ps": []int{0, 1}})), []any{"P2", nil}, nil},
		{"60#surfaces: between", "items", where(pred("due", "between", map[string]any{"ps": []int{0, 1}})), []any{"2026-10-01", "2026-10-31"}, []string{"T2"}},
		{"60#surfaces: is_null", "items", where(pred("owner", "is_null", nil)), nil, []string{"T2", "T3", "T4"}},
		{"60#surfaces: is_not_null", "items", where(pred("owner", "is_not_null", nil)), nil, []string{"T1"}},
		{"60#surfaces: not_eq_col", "items", where(pred("id", "not_eq_col", ref("", "parent"))), nil, []string{"T2"}},
		{"60#surfaces: eq_col with a NULL is unknown", "items", where(pred("id", "eq_col", ref("", "parent"))), nil, nil},
		{"60#surfaces: tuple_in", "items", where(map[string]any{"pred": map[string]any{"op": "tuple_in", "cols": []string{"state", "title"}, "ps": []int{0, 1, 2, 3}}}), []any{"doing", "Write the tests", "done", "nope"}, []string{"T2"}},
		{"60#surfaces: tuple_not_in", "items", where(map[string]any{"pred": map[string]any{"op": "tuple_not_in", "cols": []string{"state", "title"}, "ps": []int{0, 1}}}), []any{"doing", "Write the tests"}, []string{"T1", "T3", "T4"}},
		{"60#surfaces: year of a date", "items", where(pred("due", "eq", map[string]any{"p": 0, "fn": map[string]any{"name": "year"}})), []any{2026}, []string{"T2"}},
		{"60#surfaces: month in a list", "items", where(pred("due", "in", map[string]any{"ps": []int{0, 1}, "fn": map[string]any{"name": "month"}})), []any{9, 10}, []string{"T2"}},
		{"60#surfaces: day_of_week counts from Sunday as 1", "items", where(pred("due", "eq", map[string]any{"p": 0, "fn": map[string]any{"name": "day_of_week"}})), []any{3}, []string{"T2"}},
		{"60#surfaces: date of a datetime", "people", where(pred("joined", "eq", map[string]any{"p": 0, "fn": map[string]any{"name": "date"}})), []any{"2026-10-01"}, []string{"P1"}},
		{"60#surfaces: today", "items", where(pred("due", "gt", map[string]any{"value": map[string]any{"name": "today"}})), nil, []string{"T2"}},
		{"60#surfaces: days_later against a date", "items", where(pred("due", "lt", map[string]any{"value": map[string]any{"name": "days_later", "ps": []int{0}}})), []any{10}, []string{"T2"}},
		{"60#surfaces: days_ago against a datetime", "people", where(pred("joined", "gt", map[string]any{"value": map[string]any{"name": "days_ago", "ps": []int{0}}})), []any{10}, []string{"P2"}},
		{"60#surfaces: now", "people", where(pred("joined", "lt", map[string]any{"value": map[string]any{"name": "now"}})), nil, []string{"P1", "P2"}},
		{"60#surfaces: months_ago", "people", where(pred("joined", "gte", map[string]any{"value": map[string]any{"name": "months_ago", "ps": []int{0}}})), []any{0}, nil},
		{"60#surfaces: AND binds tighter than OR", "items", where(pred("state", "eq", p(0)), map[string]any{"pred": map[string]any{"conn": "or", "column": "state", "op": "eq", "p": 1}}, map[string]any{"pred": map[string]any{"conn": "and", "column": "owner", "op": "eq", "p": 2}}), []any{"waiting", "doing", "P1"}, []string{"T3"}},
		{"60#surfaces: a nested group negated", "items", where(map[string]any{"group": map[string]any{"not": true, "items": []any{pred("state", "eq", p(0))}}}), []any{"done"}, []string{"T2", "T3", "T4"}},
		{"60#surfaces: in a subquery", "items", where(pred("owner", "in", map[string]any{"sub": map[string]any{"query": map[string]any{"entity": "people", "where": where(pred("active", "eq", p(0)))}, "column": "id"}})), []any{true}, []string{"T1"}},
		{"60#surfaces: not_in a subquery", "people", where(pred("id", "not_in", map[string]any{"sub": map[string]any{"query": map[string]any{"entity": "items", "where": where(pred("owner", "is_not_null", nil))}, "column": "owner"}})), nil, []string{"P2"}},
		{"60#surfaces: a subquery refers to its owner by ^", "people", where(pred("id", "in", map[string]any{"sub": map[string]any{"query": map[string]any{"entity": "items", "where": where(pred("owner", "eq_col", ref("^", "id")))}, "column": "owner"}})), nil, []string{"P1"}},
		{"60#surfaces: a bool", "people", where(pred("active", "eq", p(0))), []any{false}, []string{"P2"}},
		{"60#surfaces: a decimal as a string", "people", where(pred("score", "gt", p(0))), []any{"1.4"}, []string{"P1"}},
		{"60#surfaces: an f64", "notes", where(pred("weight", "lt", p(0))), []any{0.75}, []string{"N1"}},
		{"60#surfaces: an identity key", "tags", where(pred("id", "gte", p(0))), []any{2}, []string{"2"}},
	}
	for _, c := range cases {
		f.sub(t, c.name, func(f *fixture) {
			got := f.ids(c.entity, c.w, c.params...)
			if !slices.Equal(got, c.want) {
				f.t.Fatalf("ids %v; want %v", got, c.want)
			}
		})
	}
	rejects := []struct {
		name string
		msg  []byte
		code string
	}{
		{"60#surfaces: an operator the column type does not take", f.msg("all", "people", map[string]any{"where": where(pred("active", "gt", p(0)))}, true), "OPERATOR_NOT_ALLOWED"},
		{"60#surfaces: a parameter not of the compared type", f.msg("all", "items", map[string]any{"where": where(pred("state", "eq", p(0)))}, 5), "CODEC_ENCODE"},
		{"60#surfaces: a group may not start with or", f.msg("all", "items", map[string]any{"where": where(map[string]any{"pred": map[string]any{"conn": "or", "column": "id", "op": "eq", "p": 0}})}, "T1"), "OR_AT_GROUP_START"},
		{"60#surfaces: an empty in", f.msg("all", "items", map[string]any{"where": where(pred("id", "in", map[string]any{"ps": []int{}}))}), "EMPTY_IN"},
		{"60#surfaces: an unknown function", f.msg("all", "items", map[string]any{"where": where(pred("due", "eq", map[string]any{"p": 0, "fn": map[string]any{"name": "week"}}))}, 1), "FUNCTION_UNKNOWN"},
		{"60#surfaces: a column function on a text column", f.msg("all", "items", map[string]any{"where": where(pred("title", "eq", map[string]any{"p": 0, "fn": map[string]any{"name": "year"}}))}, 1), "OPERATOR_NOT_ALLOWED"},
		{"60#surfaces: a reference to a join the statement lacks", f.msg("all", "items", map[string]any{"where": where(pred("id", "eq_col", ref("nope", "id")))}), "ENTITY_NOT_JOINED"},
		{"60#surfaces: two columns of different kinds do not meet", f.msg("all", "items", map[string]any{"where": where(pred("id", "eq_col", ref("", "position")))}), "OPERATOR_NOT_ALLOWED"},
		{"60#surfaces: a column of a different kind does not meet an in subquery column", f.msg("all", "items", map[string]any{"where": where(pred("id", "in", map[string]any{"sub": map[string]any{"query": map[string]any{"entity": "people"}, "column": "age"}}))}), "OPERATOR_NOT_ALLOWED"},
		{"60#surfaces: a column comparison of a bool by order", f.msg("all", "people", map[string]any{"where": where(pred("active", "gt_col", ref("", "active")))}), "OPERATOR_NOT_ALLOWED"},
		{"60#surfaces: ^ outside a subquery", f.msg("all", "items", map[string]any{"where": where(pred("id", "eq_col", ref("^", "id")))}), "IR_INVALID"},
		{"60#surfaces: a scalar column subquery without a limit", f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "sub": map[string]any{"any_item": map[string]any{"query": map[string]any{"entity": "items", "order": []any{map[string]any{"column": "id"}}}, "column": "id"}}}}), "IR_INVALID"},
		{"60#surfaces: a scalar column subquery whose order lacks the primary key", f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "sub": map[string]any{"any_item": map[string]any{"query": map[string]any{"entity": "items", "order": []any{map[string]any{"column": "title"}}, "limit": map[string]any{"offset": 0, "count": 1}}, "column": "id"}}}}), "IR_INVALID"},
	}
	for _, c := range rejects {
		f.sub(t, c.name, func(f *fixture) { f.fails(c.msg, c.code) })
	}
}

// TestContains pins contains (the Lk operator) and contains_binary (the Lb
// operator) on a corpus (spec 60 Surfaces): a substring match, contains comparing
// the ASCII letters without regard to case and every other character by code point,
// contains_binary every character by code point; a NULL column or parameter is
// unknown, and %, _ and \ in the parameter are literal.
func TestContains(t *testing.T) {
	docs := baseDocs()
	docs["people.flowmark.md"] = lines(
		"# people",
		"| id | name | email | age | active | score | joined | deleted_at | updated_at |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
		"| P1 | One | Apple |  | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P2 | Two | apple |  | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P3 | Three | Évian |  | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P4 | Four | évian |  | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P5 | Five | x_y |  | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P6 | Six | 50% |  | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P7 | Seven |  |  | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
	)
	f := catalogFixture(t, docs)
	not := func(item map[string]any) map[string]any {
		return where(map[string]any{"group": map[string]any{"not": true, "items": []any{item}}})
	}
	cases := []struct {
		name  string
		w     map[string]any
		param any
		want  []string
	}{
		{"60#surfaces: contains folds the ASCII letters", where(pred("email", "contains", p(0))), "PPL", []string{"P1", "P2"}},
		{"60#surfaces: contains compares a non-ASCII letter by code point", where(pred("email", "contains", p(0))), "ÉV", []string{"P3"}},
		{"60#surfaces: contains takes _ literally", where(pred("email", "contains", p(0))), "_", []string{"P5"}},
		{"60#surfaces: contains takes % literally", where(pred("email", "contains", p(0))), "%", []string{"P6"}},
		{"60#surfaces: contains of the empty text matches every non-NULL value", where(pred("email", "contains", p(0))), "", []string{"P1", "P2", "P3", "P4", "P5", "P6"}},
		{"60#surfaces: contains on a NULL value is unknown, also negated", not(pred("email", "contains", p(0))), "x", []string{"P1", "P2", "P3", "P4", "P6"}},
		{"60#surfaces: contains of a NULL parameter is unknown", where(pred("email", "contains", p(0))), nil, nil},
		{"60#surfaces: contains of a NULL parameter is unknown negated", not(pred("email", "contains", p(0))), nil, nil},
		{"60#surfaces: contains_binary compares by code point", where(pred("email", "contains_binary", p(0))), "pp", []string{"P1", "P2"}},
		{"60#surfaces: contains_binary does not fold case", where(pred("email", "contains_binary", p(0))), "PP", nil},
		{"60#surfaces: contains_binary of a non-ASCII letter", where(pred("email", "contains_binary", p(0))), "É", []string{"P3"}},
		{"60#surfaces: contains_binary takes _ literally", where(pred("email", "contains_binary", p(0))), "_", []string{"P5"}},
		{"60#surfaces: contains_binary on a NULL value is unknown, also negated", not(pred("email", "contains_binary", p(0))), "x", []string{"P1", "P2", "P3", "P4", "P6"}},
		{"60#surfaces: contains_binary of a NULL parameter is unknown", where(pred("email", "contains_binary", p(0))), nil, nil},
	}
	for _, c := range cases {
		f.sub(t, c.name, func(f *fixture) {
			if got := f.ids("people", c.w, c.param); !slices.Equal(got, c.want) {
				f.t.Fatalf("ids %v; want %v", got, c.want)
			}
		})
	}
	f.sub(t, "60#surfaces: contains on an integer column is not allowed", func(f *fixture) {
		f.fails(f.msg("all", "tags", map[string]any{"where": where(pred("id", "contains", p(0)))}, "1"), "OPERATOR_NOT_ALLOWED")
	})
	f.sub(t, "60#surfaces: contains_binary on an integer column is not allowed", func(f *fixture) {
		f.fails(f.msg("all", "tags", map[string]any{"where": where(pred("id", "contains_binary", p(0)))}, "1"), "OPERATOR_NOT_ALLOWED")
	})
}

// TestJoins pins inner and left joins, their on and where groups, and a joined
// placement (orm protocol 1.2).
func TestJoins(t *testing.T) {
	f := baseFixture(t)
	join := func(kind string, q map[string]any) map[string]any {
		q["entity"] = "items"
		q["columns"] = map[string]any{"mode": "none", "add": []string{"state"}}
		return map[string]any{"rel": "it", "kind": kind, "left": "item", "right": "id", "query": q}
	}
	f.sub(t, "60#surfaces: an inner join keeps the rows with a child", func(f *fixture) {
		f.want(f.msg("all", "notes", map[string]any{"columns": none, "joins": []any{join("inner", map[string]any{})}}),
			`{"rows":[{"id":"N1","parent":null,"item":"T1","it":{"id":"T1","parent":null,"owner":"P1","state":"done"}}]}`)
	})
	f.sub(t, "60#surfaces: a left join keeps every row, its child NULL", func(f *fixture) {
		f.want(f.msg("all", "notes", map[string]any{"columns": none, "joins": []any{join("left", map[string]any{})}}),
			`{"rows":[{"id":"N1","parent":null,"item":"T1","it":{"id":"T1","parent":null,"owner":"P1","state":"done"}},{"id":"N2","parent":"N1","item":null,"it":null},{"id":"N3","parent":null,"item":null,"it":null}]}`)
	})
	f.sub(t, "60#surfaces: an on group restricts the child of a left join", func(f *fixture) {
		f.want(f.msg("all", "notes", map[string]any{"columns": none, "joins": []any{join("left", map[string]any{"on": where(pred("state", "eq", p(0)))})}}, "doing"),
			`{"rows":[{"id":"N1","parent":null,"item":"T1","it":null},{"id":"N2","parent":"N1","item":null,"it":null},{"id":"N3","parent":null,"item":null,"it":null}]}`)
	})
	f.sub(t, "60#surfaces: a join's where is appended with AND", func(f *fixture) {
		f.want(f.msg("all", "notes", map[string]any{"columns": none, "joins": []any{join("left", map[string]any{"where": where(pred("state", "eq", p(0)))})}}, "done"),
			`{"rows":[{"id":"N1","parent":null,"item":"T1","it":{"id":"T1","parent":null,"owner":"P1","state":"done"}}]}`)
	})
	f.sub(t, "60#surfaces: a joined item places a join's where", func(f *fixture) {
		res := f.ok(f.msg("all", "notes", map[string]any{"columns": none,
			"where": where(pred("id", "eq", p(0)), map[string]any{"joined": map[string]any{"conn": "or", "join": "it"}}),
			"joins": []any{join("left", map[string]any{"where": where(pred("state", "eq", p(1)))})}}, "N3", "done"))
		var ids []string
		for _, r := range res.Answer.Rows {
			v, _ := r.Get("id")
			ids = append(ids, v.(string))
		}
		if !slices.Equal(ids, []string{"N1", "N3"}) {
			f.t.Fatalf("ids %v", ids)
		}
	})
	f.sub(t, "60#surfaces: a join kind other than inner or left is invalid", func(f *fixture) {
		f.fails(f.msg("all", "notes", map[string]any{"joins": []any{join("outer", map[string]any{})}}), "IR_INVALID")
	})
}

// TestJoinSoftDeleted pins a join to a soft-deleted row (spec 60 Surfaces,
// soft_delete): the join condition excludes it, so a left join binds NULL and an
// inner join drops the parent row, from all and from count alike.
func TestJoinSoftDeleted(t *testing.T) {
	docs := baseDocs()
	docs["items.flowmark.md"] = strings.Replace(docs["items.flowmark.md"], "- [ ] T3 Review the design", "- [ ] T3 Review the design owner:[P3](people.flowmark.md)", 1)
	f := catalogFixture(t, docs)
	join := func(kind string) map[string]any {
		return map[string]any{"rel": "who", "kind": kind, "left": "owner", "right": "id", "query": map[string]any{"entity": "people", "columns": map[string]any{"mode": "none", "add": []string{"name"}}}}
	}
	body := func(kind string) map[string]any {
		return map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"parent"}}, "where": where(pred("id", "in", map[string]any{"ps": []int{0, 1}})), "joins": []any{join(kind)}}
	}
	f.sub(t, "60#surfaces: a left join to a soft-deleted row binds NULL", func(f *fixture) {
		f.want(f.msg("all", "items", body("left"), "T1", "T3"),
			`{"rows":[{"id":"T1","owner":"P1","who":{"id":"P1","name":"Ada"}},{"id":"T3","owner":"P3","who":null}]}`)
	})
	f.sub(t, "60#surfaces: an inner join to a soft-deleted row drops the parent", func(f *fixture) {
		f.want(f.msg("all", "items", body("inner"), "T1", "T3"),
			`{"rows":[{"id":"T1","owner":"P1","who":{"id":"P1","name":"Ada"}}]}`)
	})
	f.sub(t, "60#surfaces: count with an inner join to a soft-deleted row drops the parent", func(f *fixture) {
		b := body("inner")
		delete(b, "columns")
		f.want(f.msg("count", "items", b, "T1", "T3"), `{"count":1}`)
	})
}

// TestRelations pins the assembled relations (orm protocol 1.2 and 2.1, spec 60
// Surfaces): one and many, key_by, flatten, limit_per_parent and if_parent.
func TestRelations(t *testing.T) {
	f := baseFixture(t)
	hist := func(extra map[string]any) map[string]any {
		q := map[string]any{"entity": "item_history", "columns": map[string]any{"mode": "none", "add": []string{"to_state"}}}
		for k, v := range extra {
			q[k] = v
		}
		return map[string]any{"rel": "history", "kind": "many", "keys": []any{map[string]any{"left": "id", "right": "item"}}, "query": q}
	}
	items := func(rels ...any) map[string]any {
		return map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"parent", "owner"}}, "relations": rels}
	}
	f.sub(t, "60#surfaces: a many relation attaches the child rows in child order", func(f *fixture) {
		f.want(f.msg("all", "items", items(hist(nil))),
			`{"rows":[{"id":"T1","history":[{"id":1,"item":"T1","to_state":"doing"},{"id":2,"item":"T1","to_state":"done"}]},{"id":"T2","history":[{"id":3,"item":"T2","to_state":"doing"}]},{"id":"T3","history":[]},{"id":"T4","history":[{"id":4,"item":"T4","to_state":"dropped"}]}]}`)
	})
	f.sub(t, "60#surfaces: key_by keys the child rows", func(f *fixture) {
		f.want(f.msg("all", "items", map[string]any{"columns": none, "where": where(pred("id", "eq", p(0))), "relations": []any{hist(map[string]any{"key_by": "to_state"})}}, "T1"),
			`{"rows":[{"id":"T1","parent":null,"owner":"P1","history":{"doing":{"id":1,"item":"T1","to_state":"doing"},"done":{"id":2,"item":"T1","to_state":"done"}}}]}`)
	})
	f.sub(t, "60#surfaces: key_by names a non-null column", func(f *fixture) {
		f.fails(f.msg("all", "items", map[string]any{"columns": none, "relations": []any{hist(map[string]any{"key_by": "evidence"})}}), "IR_INVALID")
	})
	f.sub(t, "60#surfaces: limit_per_parent keeps the first rows by the primary key", func(f *fixture) {
		f.want(f.msg("all", "items", map[string]any{"columns": none, "where": where(pred("id", "eq", p(0))), "relations": []any{hist(map[string]any{"limit_per_parent": 1})}}, "T1"),
			`{"rows":[{"id":"T1","parent":null,"owner":"P1","history":[{"id":1,"item":"T1","to_state":"doing"}]}]}`)
	})
	f.sub(t, "60#surfaces: a one relation with an order attaches the first by it", func(f *fixture) {
		rel := hist(map[string]any{"order": []any{map[string]any{"column": "id", "desc": true}}})
		rel["kind"] = "one"
		f.want(f.msg("all", "items", map[string]any{"columns": none, "where": where(pred("id", "eq", p(0))), "relations": []any{rel}}, "T1"),
			`{"rows":[{"id":"T1","parent":null,"owner":"P1","history":{"id":2,"item":"T1","to_state":"done"}}]}`)
	})
	f.sub(t, "60#surfaces: if_parent loads the relation only for the matching parents", func(f *fixture) {
		rel := hist(map[string]any{"if_parent": map[string]any{"column": "state", "p": 0}})
		f.want(f.msg("all", "items", map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"parent", "owner"}}, "relations": []any{rel}}, "doing"),
			`{"rows":[{"id":"T1","state":"done","history":[]},{"id":"T2","state":"doing","history":[{"id":3,"item":"T2","to_state":"doing"}]},{"id":"T3","state":"waiting","history":[]},{"id":"T4","state":"dropped","history":[]}]}`)
	})
	f.sub(t, "60#surfaces: a one relation without a child attaches NULL", func(f *fixture) {
		rel := map[string]any{"rel": "who", "kind": "one", "keys": []any{map[string]any{"left": "owner", "right": "id"}}, "query": map[string]any{"entity": "people", "columns": map[string]any{"mode": "none", "add": []string{"name"}}}}
		f.want(f.msg("all", "items", map[string]any{"columns": none, "where": where(pred("id", "in", map[string]any{"ps": []int{0, 1}})), "relations": []any{rel}}, "T1", "T2"),
			`{"rows":[{"id":"T1","parent":null,"owner":"P1","who":{"id":"P1","name":"Ada"}},{"id":"T2","parent":"T1","owner":null,"who":null}]}`)
	})
	f.sub(t, "60#surfaces: a relation from a join's rows", func(f *fixture) {
		j := map[string]any{"rel": "it", "kind": "inner", "left": "item", "right": "id", "query": map[string]any{"entity": "items", "columns": map[string]any{"mode": "none", "remove": []string{"parent", "owner"}}, "relations": []any{hist(nil)}}}
		f.want(f.msg("all", "notes", map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"parent"}}, "joins": []any{j}}),
			`{"rows":[{"id":"N1","item":"T1","it":{"id":"T1","history":[{"id":1,"item":"T1","to_state":"doing"},{"id":2,"item":"T1","to_state":"done"}]}}]}`)
	})
	f.sub(t, "60#surfaces: a flattened one relation is answered as a one relation, the parent's outputs kept", func(f *fixture) {
		rel := map[string]any{"rel": "who", "kind": "one", "keys": []any{map[string]any{"left": "owner", "right": "id"}}, "query": map[string]any{"entity": "people", "flatten": true, "columns": map[string]any{"mode": "none", "add": []string{"name"}}}}
		f.want(f.msg("all", "items", map[string]any{"columns": none, "where": where(pred("id", "in", map[string]any{"ps": []int{0, 1}})), "relations": []any{rel}}, "T1", "T2"),
			`{"rows":[{"id":"T1","parent":null,"owner":"P1","who":{"id":"P1","name":"Ada"}},{"id":"T2","parent":"T1","owner":null,"who":null}]}`)
	})
	f.sub(t, "60#surfaces: a relation needs keys", func(f *fixture) {
		rel := hist(nil)
		rel["keys"] = []any{}
		f.fails(f.msg("all", "items", items(rel)), "IR_INVALID")
	})
	f.sub(t, "60#surfaces: a relation child takes no limit", func(f *fixture) {
		f.fails(f.msg("all", "items", items(hist(map[string]any{"limit": map[string]any{"offset": 0, "count": 1}}))), "LIMIT_IN_RELATION")
	})
	f.sub(t, "60#surfaces: two relations of one name conflict", func(f *fixture) {
		f.fails(f.msg("all", "items", items(hist(nil), hist(nil))), "COLUMN_ALIAS_CONFLICT")
	})
}

// TestColumns pins the projection: the modes, add and remove, column functions and
// scalar subqueries (orm protocol 1.1).
func TestColumns(t *testing.T) {
	f := baseFixture(t)
	f.sub(t, "60#surfaces: remove keeps the primary key", func(f *fixture) {
		f.want(f.msg("all", "tags", map[string]any{"columns": map[string]any{"remove": []string{"id", "kind"}}}),
			`{"rows":[{"id":1,"label":"go"},{"id":2,"label":"ops"}]}`)
	})
	f.sub(t, "60#surfaces: a column function output", func(f *fixture) {
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "fn": map[string]any{"joined_year": map[string]any{"column": "joined", "fn": map[string]any{"name": "year"}}}}}),
			`{"rows":[{"id":"P1","joined_year":2026},{"id":"P2","joined_year":2026}]}`)
	})
	f.sub(t, "60#surfaces: scalar subqueries count, sum and avg", func(f *fixture) {
		sub := func(agg, col string) map[string]any {
			m := map[string]any{"query": map[string]any{"entity": "items", "where": where(pred("owner", "eq_col", map[string]any{"ref": map[string]any{"path": "^", "column": "id"}}))}, "agg": agg}
			if col != "" {
				m["column"] = col
			}
			return m
		}
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "sub": map[string]any{"n": sub("count", ""), "s": sub("sum", "position"), "a": sub("avg", "position")}}}),
			`{"rows":[{"id":"P1","a":1,"n":1,"s":1},{"id":"P2","a":null,"n":0,"s":0}]}`)
	})
	f.sub(t, "60#surfaces: a scalar column subquery is the column of the first row of its order, within its limit", func(f *fixture) {
		top := func(offset int) map[string]any {
			return map[string]any{"column": "id", "query": map[string]any{"entity": "items", "order": []any{map[string]any{"column": "id", "desc": true}}, "limit": map[string]any{"offset": offset, "count": 1}}}
		}
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "sub": map[string]any{"top": top(0), "second": top(1)}}}),
			`{"rows":[{"id":"P1","second":"T3","top":"T4"},{"id":"P2","second":"T3","top":"T4"}]}`)
	})
	f.sub(t, "60#surfaces: a scalar column subquery of no row is NULL", func(f *fixture) {
		first := map[string]any{"column": "id", "query": map[string]any{"entity": "items", "where": where(pred("owner", "eq_col", map[string]any{"ref": map[string]any{"path": "^", "column": "id"}})),
			"order": []any{map[string]any{"column": "id"}}, "limit": map[string]any{"offset": 0, "count": 1}}}
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "sub": map[string]any{"first": first}}}),
			`{"rows":[{"id":"P1","first":"T1"},{"id":"P2","first":null}]}`)
	})
	f.sub(t, "60#surfaces: a datetime answers in the wire form", func(f *fixture) {
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"joined", "active", "score"}}, "where": where(pred("id", "eq", p(0)))}, "P1"),
			`{"rows":[{"id":"P1","joined":"2026-10-01 09:00:00.000000","active":true,"score":"1.50"}]}`)
	})
	f.sub(t, "60#surfaces: an output named as a column conflicts", func(f *fixture) {
		f.fails(f.msg("all", "people", map[string]any{"columns": map[string]any{"fn": map[string]any{"name": map[string]any{"column": "joined", "fn": map[string]any{"name": "year"}}}}}), "COLUMN_ALIAS_CONFLICT")
	})
}

// TestOrderAndLimit pins the IR order and limit (spec 20 Order, spec 60 Surfaces).
func TestOrderAndLimit(t *testing.T) {
	f := baseFixture(t)
	order := func(os ...map[string]any) map[string]any {
		return map[string]any{"columns": none, "order": os}
	}
	f.sub(t, "20#order: a read without an order answers storage order", func(f *fixture) {
		if got := f.ids("notes", nil); !slices.Equal(got, []string{"N1", "N2", "N3"}) {
			f.t.Fatalf("ids %v", got)
		}
	})
	f.sub(t, "20#order: an order on the order column sorts by its value, ties in storage order", func(f *fixture) {
		res := f.ok(f.msg("all", "notes", order(map[string]any{"column": "position"})))
		var ids []string
		for _, r := range res.Answer.Rows {
			v, _ := r.Get("id")
			ids = append(ids, v.(string))
		}
		if !slices.Equal(ids, []string{"N1", "N2", "N3"}) {
			f.t.Fatalf("ids %v", ids)
		}
	})
	f.sub(t, "60#surfaces: desc and a second item", func(f *fixture) {
		f.want(f.msg("all", "items", order(map[string]any{"column": "position", "desc": true}, map[string]any{"column": "id", "desc": true})),
			`{"rows":[{"id":"T4","parent":null,"owner":null},{"id":"T3","parent":null,"owner":null},{"id":"T2","parent":"T1","owner":null},{"id":"T1","parent":null,"owner":"P1"}]}`)
	})
	f.sub(t, "60#surfaces: a column function order", func(f *fixture) {
		f.want(f.msg("all", "people", order(map[string]any{"column": "joined", "fn": map[string]any{"name": "day_of_week"}, "desc": true})),
			`{"rows":[{"id":"P2"},{"id":"P1"}]}`)
	})
	f.sub(t, "60#surfaces: random answers every row", func(f *fixture) {
		res := f.ok(f.msg("all", "items", order(map[string]any{"random": true})))
		if len(res.Answer.Rows) != 4 {
			f.t.Fatalf("%d rows", len(res.Answer.Rows))
		}
	})
	f.sub(t, "60#surfaces: a limit past the rows answers none", func(f *fixture) {
		f.want(f.msg("all", "tags", map[string]any{"limit": map[string]any{"offset": 5, "count": 2}}), `{"rows":[]}`)
	})
	f.sub(t, "60#surfaces: an order item with both a column and random is invalid", func(f *fixture) {
		f.fails(f.msg("all", "items", order(map[string]any{"column": "id", "random": true})), "IR_INVALID")
	})
}

// TestOrderNulls pins where an IR order puts NULL (spec 60 Surfaces, ordering):
// before every value ascending and after every value descending, two NULLs tied in
// storage order — for a root order, the groups of group_count and the ranking of
// limit_per_parent.
func TestOrderNulls(t *testing.T) {
	docs := baseDocs()
	docs["people.flowmark.md"] = lines(
		"# people",
		"| id | name | email | age | active | score | joined | deleted_at | updated_at |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
		"| P1 | One |  |  | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P2 | Two |  | 5 | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P3 | Three |  | 3 | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P4 | Four |  |  | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
		"| P5 | Five |  | 1 | true | 0.00 | 2026-10-01 09:00:00 |  |  |",
	)
	f := catalogFixture(t, docs)
	byAge := func(desc bool) map[string]any {
		return map[string]any{"columns": none, "order": []any{map[string]any{"column": "age", "desc": desc}}}
	}
	ids := func(f *fixture, m []byte) []string {
		var out []string
		for _, r := range f.ok(m).Answer.Rows {
			v, _ := r.Get("id")
			out = append(out, v.(string))
		}
		return out
	}
	f.sub(t, "60#surfaces: NULL sorts before every value ascending, NULLs in storage order", func(f *fixture) {
		if got := ids(f, f.msg("all", "people", byAge(false))); !slices.Equal(got, []string{"P1", "P4", "P5", "P3", "P2"}) {
			f.t.Fatalf("ids %v", got)
		}
	})
	f.sub(t, "60#surfaces: NULL sorts after every value descending, NULLs in storage order", func(f *fixture) {
		if got := ids(f, f.msg("all", "people", byAge(true))); !slices.Equal(got, []string{"P2", "P3", "P5", "P1", "P4"}) {
			f.t.Fatalf("ids %v", got)
		}
	})
	f.sub(t, "60#surfaces: group_count ordered ascending puts the NULL group first", func(f *fixture) {
		f.want(f.msg("group_count", "items", map[string]any{"group_by": []string{"owner"}, "order": []any{map[string]any{"column": "owner"}}}),
			`{"rows":[{"owner":null,"row_count":3},{"owner":"P1","row_count":1}]}`)
	})
	f.sub(t, "60#surfaces: group_count ordered descending puts the NULL group last", func(f *fixture) {
		f.want(f.msg("group_count", "items", map[string]any{"group_by": []string{"owner"}, "order": []any{map[string]any{"column": "owner", "desc": true}}}),
			`{"rows":[{"owner":"P1","row_count":1},{"owner":null,"row_count":3}]}`)
	})
	hist := func(desc bool) []byte {
		rel := map[string]any{"rel": "history", "kind": "many", "keys": []any{map[string]any{"left": "id", "right": "item"}},
			"query": map[string]any{"entity": "item_history", "columns": none, "limit_per_parent": 1, "order": []any{map[string]any{"column": "evidence", "desc": desc}}}}
		return f.msg("all", "items", map[string]any{"columns": none, "where": where(pred("id", "eq", p(0))), "relations": []any{rel}}, "T1")
	}
	f.sub(t, "60#surfaces: limit_per_parent ranked ascending keeps a NULL row first", func(f *fixture) {
		f.want(hist(false), `{"rows":[{"id":"T1","parent":null,"owner":"P1","history":[{"id":1,"item":"T1"}]}]}`)
	})
	f.sub(t, "60#surfaces: limit_per_parent ranked descending keeps a value before NULL", func(f *fixture) {
		f.want(hist(true), `{"rows":[{"id":"T1","parent":null,"owner":"P1","history":[{"id":2,"item":"T1"}]}]}`)
	})
}

// TestClock pins the request clock (spec 40 The clock, spec 60 Surfaces): one value
// per request, truncated to p digits when written into datetime(p) and kept at six
// digits when compared with a column, now and the relative time functions alike.
func TestClock(t *testing.T) {
	const half = "2026-10-11 10:00:00.500000"
	const late = "2026-10-11 10:00:00.999999"
	docs := baseDocs()
	docs["people.flowmark.md"] = lines(
		"# people",
		"| id | name | email | age | active | score | joined | deleted_at | updated_at |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
		"| P1 | Ada |  |  | true | 0.00 | 2026-10-11 10:00:00 |  |  |",
	)
	f := catalogFixture(t, docs)
	at := func(f *fixture, op string, value map[string]any, params ...any) []string {
		res := f.ok(f.msgAt(half, "all", "people", map[string]any{"columns": none, "where": where(pred("joined", op, map[string]any{"value": value}))}, params...))
		var out []string
		for _, r := range res.Answer.Rows {
			v, _ := r.Get("id")
			out = append(out, v.(string))
		}
		return out
	}
	f.sub(t, "40#the-clock: a datetime(0) column equals no clock with a fraction", func(f *fixture) {
		if got := at(f, "eq", map[string]any{"name": "now"}); got != nil {
			f.t.Fatalf("ids %v", got)
		}
	})
	f.sub(t, "40#the-clock: a datetime(0) column is less than the clock of its second", func(f *fixture) {
		if got := at(f, "lt", map[string]any{"name": "now"}); !slices.Equal(got, []string{"P1"}) {
			f.t.Fatalf("ids %v", got)
		}
	})
	f.sub(t, "40#the-clock: a relative time function keeps six digits", func(f *fixture) {
		if got := at(f, "lt", map[string]any{"name": "seconds_ago", "ps": []int{0}}, 0.25); !slices.Equal(got, []string{"P1"}) {
			f.t.Fatalf("ids %v", got)
		}
	})
	t.Run("40#the-clock: written into datetime(0) and datetime(3) the clock is truncated, never rounded", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.ok(f.msgAt(late, "insert", "people", map[string]any{"set": sets("name")}, "Lin")))
		f.commit(f.ok(f.msgAt(late, "insert", "stamps", map[string]any{"set": sets("label")}, "one")))
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"joined"}}, "where": where(pred("id", "eq", p(0)))}, "P4"),
			`{"rows":[{"id":"P4","joined":"2026-10-11 10:00:00.000000"}]}`)
		f.want(f.msg("all", "stamps", map[string]any{"columns": map[string]any{"mode": "all"}}),
			`{"rows":[{"id":1,"label":"one","at":"2026-10-11 10:00:00.999000","updated_at":null,"opens":null,"closes":null}]}`)
		f.commit(f.ok(f.msgAt(half, "update", "stamps", map[string]any{"set": sets("label"), "where": where(pred("id", "eq", p(1)))}, "two", 1)))
		f.want(f.msg("all", "stamps", map[string]any{"columns": map[string]any{"mode": "all"}}),
			`{"rows":[{"id":1,"label":"two","at":"2026-10-11 10:00:00.999000","updated_at":"2026-10-11 10:00:00.500000","opens":null,"closes":null}]}`)
	})
}

// TestRelativeTimeRange pins a relative time function whose result lies outside
// 0001-01-01 to 9999-12-31 23:59:59.999999, or whose interval cannot be represented
// (spec 60 Surfaces): it is NULL, so a comparison with it is unknown and neither it
// nor its negation matches a row.
func TestRelativeTimeRange(t *testing.T) {
	f := baseFixture(t)
	cases := []struct {
		name, fn string
		param    any
	}{
		{"60#surfaces: seconds_ago past every datetime is NULL", "seconds_ago", 1e300},
		{"60#surfaces: days_later past 9999-12-31 is NULL", "days_later", 4000000},
		{"60#surfaces: months_ago before 0001-01-01 is NULL", "months_ago", 200000},
		{"60#surfaces: hours_later past 9999-12-31 is NULL", "hours_later", 2000000000},
	}
	for _, c := range cases {
		cmp := pred("joined", "lt", map[string]any{"value": map[string]any{"name": c.fn, "ps": []int{0}}})
		f.sub(t, c.name, func(f *fixture) {
			if got := f.ids("people", where(cmp), c.param); got != nil {
				f.t.Fatalf("ids %v", got)
			}
			if got := f.ids("people", where(map[string]any{"group": map[string]any{"not": true, "items": []any{cmp}}}), c.param); got != nil {
				f.t.Fatalf("negated: ids %v", got)
			}
		})
	}
	f.sub(t, "60#surfaces: a relative time function inside the range is a value", func(f *fixture) {
		if got := f.ids("people", where(pred("joined", "lt", map[string]any{"value": map[string]any{"name": "days_later", "ps": []int{0}}})), 2000000); !slices.Equal(got, []string{"P1", "P2"}) {
			f.t.Fatalf("ids %v", got)
		}
	})
}

// TestOutputInvariant pins that a stored datetime the answer cannot write in its
// wire form is INTERNAL, never answered as its stored text: the corpus holds only
// readable values, so such a value is a defect of the executor, and an error is
// never hidden (AGENTS).
func TestOutputInvariant(t *testing.T) {
	f := baseFixture(t).on(t)
	bad := "2026-13-45 99:00:00"
	for _, x := range f.c.recs["people"] {
		if x.row.Key == "P1" {
			x.row.Scalars["joined"] = &bad
		}
	}
	f.fails(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"joined"}}, "where": where(pred("id", "eq", p(0)))}, "P1"), "INTERNAL")
}

// TestUnavailableTable pins spec 10's unavailable table: a request that needs any
// row of a table holding an unreadable file, or two rows with byte-equal keys, fails
// as CONSTRAINT naming the file and the diagnostic, and a request that needs none
// of its rows is answered.
func TestUnavailableTable(t *testing.T) {
	broken := baseDocs()
	broken["notes/N9.flowmark.md"] = "no heading\n"
	f := catalogFixture(t, broken)
	f.sub(t, "10#diagnostics: a read of an unavailable table fails naming the file", func(f *fixture) {
		err := f.failure(f.msg("all", "notes", nil), "CONSTRAINT")
		if err.Diagnostic == nil || err.Diagnostic.File != "notes/N9.flowmark.md" || !strings.HasPrefix(err.Diagnostic.Rule, "format.") {
			f.t.Fatalf("diagnostic %+v", err.Diagnostic)
		}
	})
	f.sub(t, "10#diagnostics: a point lookup by key in an unavailable table fails", func(f *fixture) {
		f.fails(f.msg("one", "notes", map[string]any{"where": where(pred("id", "eq", p(0)))}, "N1"), "CONSTRAINT")
	})
	f.sub(t, "10#diagnostics: a block table of an unavailable table is unavailable", func(f *fixture) {
		f.fails(f.msg("count", "steps", nil), "CONSTRAINT")
	})
	f.sub(t, "10#diagnostics: a relation into an unavailable table fails", func(f *fixture) {
		rel := map[string]any{"rel": "ns", "kind": "many", "keys": []any{map[string]any{"left": "id", "right": "item"}}, "query": map[string]any{"entity": "notes"}}
		f.fails(f.msg("all", "items", map[string]any{"relations": []any{rel}}), "CONSTRAINT")
	})
	f.sub(t, "10#diagnostics: a subquery over an unavailable table fails", func(f *fixture) {
		f.fails(f.msg("all", "items", map[string]any{"where": where(pred("id", "in", map[string]any{"sub": map[string]any{"query": map[string]any{"entity": "notes"}, "column": "item"}}))}), "CONSTRAINT")
	})
	f.sub(t, "10#diagnostics: a request that needs no row of it is answered", func(f *fixture) {
		f.want(f.msg("count", "items", nil), `{"count":4}`)
	})
	dup := baseDocs()
	dup["tags.flowmark.md"] = lines("# tags", "| id | label | kind |", "| --- | --- | --- |", "| 1 | go | topic |", "| 1 | ops | area |")
	g := catalogFixture(t, dup)
	g.sub(t, "10#diagnostics: two rows with byte-equal keys name the second row and key.duplicate", func(f *fixture) {
		err := f.failure(f.msg("all", "tags", nil), "CONSTRAINT")
		if d := err.Diagnostic; d == nil || d.Rule != "key.duplicate" || d.File != "tags.flowmark.md" || d.Line != 5 {
			f.t.Fatalf("diagnostic %+v", err.Diagnostic)
		}
	})
}

// TestTypeFaultValue pins spec 10 Readable values: a request that evaluates a value
// not of its column type fails as CONSTRAINT naming its type diagnostic, and one
// that does not evaluate it is answered.
func TestTypeFaultValue(t *testing.T) {
	docs := baseDocs()
	docs["people.flowmark.md"] = strings.Replace(docs["people.flowmark.md"], "| P1 | Ada | ada@x.io | 36 |", "| P1 | Ada | ada@x.io | old |", 1)
	f := catalogFixture(t, docs)
	f.sub(t, "10#readable-values: returning the value fails naming the type diagnostic", func(f *fixture) {
		err := f.failure(f.msg("all", "people", nil), "CONSTRAINT")
		if d := err.Diagnostic; d == nil || d.Rule != "type" || d.File != "people.flowmark.md" || d.Line != 4 {
			f.t.Fatalf("diagnostic %+v", err.Diagnostic)
		}
	})
	f.sub(t, "10#readable-values: an aggregate of the value fails", func(f *fixture) {
		f.fails(f.msg("avg", "people", map[string]any{"agg": "age"}), "CONSTRAINT")
	})
	f.sub(t, "10#readable-values: a predicate on the value fails", func(f *fixture) {
		f.fails(f.msg("count", "people", map[string]any{"where": where(pred("age", "is_null", nil))}), "CONSTRAINT")
	})
	f.sub(t, "10#readable-values: a request that does not evaluate it is answered", func(f *fixture) {
		f.want(f.msg("all", "people", map[string]any{"columns": none}), `{"rows":[{"id":"P1"},{"id":"P2"}]}`)
	})
}
