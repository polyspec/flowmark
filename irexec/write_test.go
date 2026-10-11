package irexec

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/store"
)

// fresh returns a new base fixture for a case that commits, reporting to t.
func fresh(t *testing.T) *fixture { return baseFixture(t).on(t) }

// sets is the set of a request, each column assigned its parameter in order.
func sets(cols ...string) []any {
	out := make([]any, len(cols))
	for i, c := range cols {
		out[i] = set(c, i)
	}
	return out
}

// wantFaults requires the faults of err to be exactly want, in order: the table,
// key, index, column and rule of each, and a text its message holds.
func wantFaults(t testing.TB, err *Error, want []dupFault) {
	t.Helper()
	if len(err.Faults) != len(want) {
		t.Fatalf("faults %+v; want %+v", err.Faults, want)
	}
	for i, w := range want {
		g := err.Faults[i]
		if g.Table != w.table || g.Key != w.key || g.Index != w.index || g.Column != w.column || g.Rule != w.rule || !strings.Contains(g.Message, w.names) {
			t.Fatalf("fault %d is %+v; want %+v", i, g, w)
		}
	}
}

// dupFault is an expected fault of a refused write.
type dupFault struct {
	table, key string
	index      int
	column     string
	rule       string
	names      string
}

// TestInsert pins insert (spec 60 Surfaces, spec 20 Keys and Order): executor-
// assigned keys and order, defaults written explicitly, and the answer's keys.
func TestInsert(t *testing.T) {
	t.Run("60#surfaces: a key_prefix key, the default state and an appended order", func(t *testing.T) {
		f := fresh(t)
		res := f.want(f.msg("insert", "items", map[string]any{"set": sets("title", "body")}, "New item", ""), `{"affected":1,"keys":["T5"]}`)
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].Path != "items.flowmark.md" {
			t.Fatalf("targets %+v", res.Post.Targets)
		}
		f.commit(res)
		if !strings.Contains(f.file("items.flowmark.md"), "- [-] T4 Drop the old path\n- [ ] T5 New item\n") {
			t.Fatalf("items:\n%s", f.file("items.flowmark.md"))
		}
		f.want(f.msg("one", "items", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"position", "state"}}, "where": where(pred("id", "eq", p(0)))}, "T5"),
			`{"rows":[{"id":"T5","parent":null,"owner":null,"position":4,"state":"waiting"}]}`)
	})
	t.Run("60#surfaces: a multi-row insert answers each key in insert order", func(t *testing.T) {
		f := fresh(t)
		res := f.want(f.msg("insert", "items", map[string]any{"set": sets("title", "body"), "rows": [][]int{{2, 3}}}, "A", "", "B", "Body of B."), `{"affected":2,"keys":["T5","T6"]}`)
		f.commit(res)
		if !strings.Contains(f.file("items.flowmark.md"), "## items row T6\n\nBody of B.\n") {
			t.Fatalf("items:\n%s", f.file("items.flowmark.md"))
		}
	})
	t.Run("20#order: an insert with k places the row at k and the siblings follow", func(t *testing.T) {
		f := fresh(t)
		res := f.want(f.msg("insert", "items", map[string]any{"set": sets("title", "body", "parent", "position")}, "First child", "", "T1", 1), `{"affected":1,"keys":["T5"]}`)
		f.commit(res)
		if !strings.Contains(f.file("items.flowmark.md"), "- [x] T1 Ship the parser owner:[P1](people.flowmark.md)\n  - [ ] T5 First child\n  - [~] T2 Write the tests") {
			t.Fatalf("items:\n%s", f.file("items.flowmark.md"))
		}
	})
	t.Run("60#surfaces: defaults are written explicitly and now is the request clock", func(t *testing.T) {
		f := fresh(t)
		res := f.want(f.msg("insert", "people", map[string]any{"set": sets("name")}, "Lin"), `{"affected":1,"keys":["P4"]}`)
		f.commit(res)
		if !strings.Contains(f.file("people.flowmark.md"), "| P4 | Lin |  |  | true | 0.00 | 2026-10-11 09:00:00 |  |  |\n") {
			t.Fatalf("people:\n%s", f.file("people.flowmark.md"))
		}
	})
	t.Run("60#surfaces: an identity key is max+1", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(f.msg("insert", "tags", map[string]any{"set": sets("label", "kind")}, "web", "area"), `{"affected":1,"keys":[3]}`))
	})
	t.Run("20#storage: an insert into a files table creates its file", func(t *testing.T) {
		f := fresh(t)
		res := f.want(f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body")}, "N4", "Fourth", "Text."), `{"affected":1,"keys":["N4"]}`)
		if tg := res.Post.Targets; len(tg) != 1 || tg[0].Path != "notes/N4.flowmark.md" || tg[0].Before != nil {
			t.Fatalf("targets %+v", tg)
		}
		f.commit(res)
		if got := f.file("notes/N4.flowmark.md"); got != lines("# Fourth", "- position: 3", "", "Text.") {
			t.Fatalf("N4:\n%s", got)
		}
	})
	t.Run("20#storage: an insert of a block row writes it in its parent's file", func(t *testing.T) {
		f := fresh(t)
		res := f.want(f.msg("insert", "steps", map[string]any{"set": sets("id", "note", "title", "done")}, "S3", "N1", "Ship", false), `{"affected":1,"keys":["S3"]}`)
		f.commit(res)
		if !strings.Contains(f.file("notes/N1.flowmark.md"), "- S2 Review done:true\n- S3 Ship done:false\n") {
			t.Fatalf("N1:\n%s", f.file("notes/N1.flowmark.md"))
		}
	})
	t.Run("50#keys-and-references: a reference to a soft-deleted row is satisfied", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(f.msg("insert", "items", map[string]any{"set": sets("title", "body", "owner")}, "Kept", "", "P3"), `{"affected":1,"keys":["T5"]}`))
	})
	t.Run("60#surfaces: on_duplicate updates the row an inserted row meets on a unique key", func(t *testing.T) {
		f := fresh(t)
		res := f.want(f.msg("insert", "tags", map[string]any{"set": sets("label", "kind"), "on_duplicate": []any{set("kind", 2)}}, "go", "topic", "area"), `{"affected":1,"keys":[1]}`)
		f.commit(res)
		if !strings.Contains(f.file("tags.flowmark.md"), "| 1 | go | area |") {
			t.Fatalf("tags:\n%s", f.file("tags.flowmark.md"))
		}
	})
	upsert := func(f *fixture, id string) []byte {
		return f.msg("insert", "people", map[string]any{"set": sets("id", "name"), "on_duplicate": []any{set("name", 1)}}, id, "Upserted")
	}
	t.Run("60#surfaces: on_duplicate meets the row whose primary key equals without regard to case, which keeps its key", func(t *testing.T) {
		f := fresh(t)
		res := f.want(upsert(f, "p1"), `{"affected":1,"keys":["P1"]}`)
		f.commit(res)
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"name"}}}),
			`{"rows":[{"id":"P1","name":"Upserted"},{"id":"P2","name":"Max"}]}`)
	})
	t.Run("60#surfaces: on_duplicate meets the byte-equal key first, else the least in byte order", func(t *testing.T) {
		// Two files whose names differ only in case: a case-sensitive root.
		docs := baseDocs()
		docs["notes/NX.flowmark.md"] = lines("# Upper", "- position: 3")
		docs["notes/nx.flowmark.md"] = lines("# Lower", "- position: 4")
		f := newFixtureIn(t, caseSensitiveRoot(t), catalogRoot, docs).on(t)
		up := func(id string) []byte {
			return f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body"), "on_duplicate": []any{set("title", 1)}}, id, "Upserted", "")
		}
		f.want(up("nx"), `{"affected":1,"keys":["nx"]}`)
		res := f.want(up("Nx"), `{"affected":1,"keys":["NX"]}`)
		if tg := res.Post.Targets; len(tg) != 1 || tg[0].Path != "notes/NX.flowmark.md" {
			t.Fatalf("targets %+v", tg)
		}
	})
	t.Run("60#surfaces: an insert without on_duplicate whose key equals without regard to case", func(t *testing.T) {
		f := fresh(t)
		err := f.fails(f.msg("insert", "people", map[string]any{"set": sets("id", "name")}, "p1", "X"), "DUPLICATE_KEY")
		wantFaults(t, err, []dupFault{{"people", "p1", -1, "id", "DUPLICATE_KEY", "the key P1"}})
	})
	dangling := func() map[string]string {
		docs := baseDocs()
		docs["notes/N3.flowmark.md"] = lines("# Second root", "- item: [T9](x.md)", "- position: 2")
		return docs
	}
	t.Run("10#foreign-keys-as-links: an insert that gives a dangling link its row rewrites the link", func(t *testing.T) {
		f := catalogFixture(t, dangling()).on(t)
		res := f.want(f.msg("insert", "items", map[string]any{"set": sets("id", "title", "body")}, "T9", "Ninth", ""), `{"affected":1,"keys":["T9"]}`)
		var paths []string
		for _, tg := range res.Post.Targets {
			paths = append(paths, tg.Path)
		}
		if strings.Join(paths, " ") != "items.flowmark.md notes/N3.flowmark.md" {
			t.Fatalf("targets %v", paths)
		}
		f.commit(res)
		if got := f.file("notes/N3.flowmark.md"); got != lines("# Second root", "- item: [T9](../items.flowmark.md)", "- position: 2") {
			t.Fatalf("N3:\n%s", got)
		}
	})
	t.Run("10#diagnostics: an insert whose row a dangling link may name needs the linking tables", func(t *testing.T) {
		docs := dangling()
		docs["notes/N9.flowmark.md"] = "no heading\n"
		f := catalogFixture(t, docs).on(t)
		err := f.fails(f.msg("insert", "items", map[string]any{"set": sets("id", "title", "body")}, "T9", "Ninth", ""), "CONSTRAINT")
		if err.Diagnostic == nil || err.Diagnostic.File != "notes/N9.flowmark.md" {
			t.Fatalf("diagnostic %+v", err.Diagnostic)
		}
	})
	t.Run("60#surfaces: on_duplicate inserts a row that meets no key", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(f.msg("insert", "tags", map[string]any{"set": sets("label", "kind"), "on_duplicate": []any{set("kind", 2)}}, "web", "topic", "area"), `{"affected":1,"keys":[3]}`))
	})
}

// TestInsertRejects pins the refusals of insert.
func TestInsertRejects(t *testing.T) {
	f := baseFixture(t)
	cases := []struct {
		name string
		msg  []byte
		code string
		want []dupFault
	}{
		{"50#keys-and-references: a reference that names no row", f.msg("insert", "items", map[string]any{"set": sets("title", "body", "owner")}, "X", "", "P9"), "FOREIGN_KEY",
			[]dupFault{{"items", "T5", -1, "owner", "FOREIGN_KEY", "P9"}}},
		{"50#keys-and-references: a block row's placement key that names no row", f.msg("insert", "steps", map[string]any{"set": sets("id", "note", "title", "done")}, "S3", "N9", "X", false), "FOREIGN_KEY",
			[]dupFault{{"steps", "S3", -1, "note", "FOREIGN_KEY", "N9"}}},
		{"10#readable-values: a value not of its column type", f.msg("insert", "people", map[string]any{"set": sets("name", "age")}, "X", "old"), "CONSTRAINT",
			[]dupFault{{"people", "P4", -1, "age", "CONSTRAINT", "age"}}},
		{"10#readable-values: a text longer than its column", f.msg("insert", "tags", map[string]any{"set": sets("label", "kind")}, "a-label-longer-than-16", "topic"), "CONSTRAINT",
			[]dupFault{{"tags", "3", -1, "label", "CONSTRAINT", "label"}}},
		{"50#keys-and-references: a unique key held by another row", f.msg("insert", "people", map[string]any{"set": sets("name", "email")}, "X", "ada@x.io"), "DUPLICATE_KEY",
			[]dupFault{{"people", "P4", -1, "email", "DUPLICATE_KEY", "P1"}}},
		{"50#keys-and-references: a soft-deleted row still holds its unique key", f.msg("insert", "people", map[string]any{"set": sets("name", "email")}, "X", "gone@x.io"), "DUPLICATE_KEY",
			[]dupFault{{"people", "P4", -1, "email", "DUPLICATE_KEY", "P3"}}},
		{"50#checks: a check made false", f.msg("insert", "people", map[string]any{"set": sets("name", "age")}, "X", -1), "CONSTRAINT",
			[]dupFault{{"people", "P4", -1, "", "CONSTRAINT", "ck_people_age"}}},
		{"50#checks: an in check made false", f.msg("insert", "tags", map[string]any{"set": sets("label", "kind")}, "web", "other"), "CONSTRAINT",
			[]dupFault{{"tags", "3", -1, "", "CONSTRAINT", "ck_tags_kind"}}},
		{"50#the-state-machine: an inserted state that is not initial", f.msg("insert", "items", map[string]any{"set": sets("title", "body", "state")}, "X", "", "doing"), "CONSTRAINT",
			[]dupFault{{"items", "T5", -1, "state", "CONSTRAINT", "initial"}}},
		{"20#order: an order outside 1..n+1", f.msg("insert", "items", map[string]any{"set": sets("title", "body", "position")}, "X", "", 9), "CONSTRAINT",
			[]dupFault{{"items", "T5", -1, "position", "CONSTRAINT", "1..4"}}},
		{"20#hierarchy: a parent cycle cannot be inserted, a parent outside the block can", f.msg("insert", "steps", map[string]any{"set": sets("id", "note", "title", "done")}, "S1", "N2", "X", false), "DUPLICATE_KEY",
			[]dupFault{{"steps", "S1", -1, "id", "DUPLICATE_KEY", "notes/N1.flowmark.md"}}},
		{"10#sections-and-their-values: a markdown value ending with a line feed", f.msg("insert", "items", map[string]any{"set": sets("title", "body")}, "X", "Body\n"), "CONSTRAINT",
			[]dupFault{{"items", "T5", -1, "body", "CONSTRAINT", "blank line"}}},
	}
	for _, c := range cases {
		f.sub(t, c.name, func(f *fixture) {
			err := f.fails(c.msg, c.code)
			wantFaults(f.t, err, c.want)
		})
	}
	uf := catalogFixture(t, map[string]string{"tags.flowmark.md": "no heading\n"})
	codes := []struct {
		name string
		f    *fixture
		msg  []byte
		code string
	}{
		{"60#surfaces: an insert that omits a required column", f, f.msg("insert", "items", map[string]any{"set": sets("title")}, "X"), "IR_INVALID"},
		{"60#surfaces: an insert that sets the identity column", f, f.msg("insert", "tags", map[string]any{"set": sets("id", "label", "kind")}, 9, "web", "area"), "IR_INVALID"},
		{"60#surfaces: rows with on_duplicate", f, f.msg("insert", "tags", map[string]any{"set": sets("label", "kind"), "rows": [][]int{{0, 1}}, "on_duplicate": []any{set("kind", 1)}}, "a", "area"), "IR_INVALID"},
		{"10#diagnostics: an insert into an unavailable table", uf, uf.msg("insert", "tags", map[string]any{"set": sets("label", "kind")}, "web", "area"), "CONSTRAINT"},
	}
	for _, c := range codes {
		c.f.sub(t, c.name, func(f *fixture) { f.fails(c.msg, c.code) })
	}
}

// TestKeyChecks pins the key checks of spec 10 Write faults, which run on the
// request against the corpus before any file renders and report the request's
// indices (T12.1-23).
func TestKeyChecks(t *testing.T) {
	f := baseFixture(t)
	ins := func(ids ...string) []byte {
		params := []any{}
		var rows [][]int
		for i, id := range ids {
			params = append(params, id, "Row "+id, "")
			if i > 0 {
				rows = append(rows, []int{3 * i, 3*i + 1, 3*i + 2})
			}
		}
		body := map[string]any{"set": sets("id", "title", "body")}
		if rows != nil {
			body["rows"] = rows
		}
		return f.msg("insert", "items", body, params...)
	}
	const dup = store.RuleDuplicateKey
	cases := []struct {
		name string
		ids  []string
		want []dupFault
	}{
		{"10#diagnostics: inserting a key the table holds names the table's row", []string{"T1"}, []dupFault{
			{"items", "T1", -1, "id", dup, "the items row in items.flowmark.md"}}},
		{"10#diagnostics: two given rows with the table's key name the table's row, then index 0", []string{"T1", "T1"}, []dupFault{
			{"items", "T1", 0, "id", dup, "the items row in items.flowmark.md"},
			{"items", "T1", 1, "id", dup, "index 0"}}},
		{"10#diagnostics: a byte-equal pair of given rows is one fault at the later row", []string{"T9", "T9"}, []dupFault{
			{"items", "T9", 1, "id", dup, "index 0"}}},
		{"10#diagnostics: a key equal to the table's only without regard to case", []string{"t1"}, []dupFault{
			{"items", "t1", -1, "id", dup, "the key T1 of the items row"}}},
		{"10#diagnostics: of two introduced case-only keys the greater is faulted", []string{"x9", "X9"}, []dupFault{
			{"items", "x9", -1, "id", dup, "the key X9 of the row at index 1"}}},
		{"10#diagnostics: a byte-equal twin and a case-only twin fault each later row once", []string{"T9", "T9", "t9"}, []dupFault{
			{"items", "T9", 1, "id", dup, "index 0"},
			{"items", "t9", -1, "id", dup, "the key T9 of the row at index 0"}}},
	}
	for _, c := range cases {
		f.sub(t, c.name, func(f *fixture) {
			wantFaults(f.t, f.fails(ins(c.ids...), "DUPLICATE_KEY"), c.want)
		})
	}
	f.sub(t, "10#diagnostics: new keys pass the key checks", func(f *fixture) {
		f.want(ins("T7", "T8"), `{"affected":2,"keys":["T7","T8"]}`)
	})
	f.sub(t, "10#diagnostics: a DUPLICATE_KEY from the codec is internal", func(f *fixture) {
		r := &reader{e: f.e, c: f.c, cl: &call{}}
		w := &writer{r: r, c: f.c, next: f.c.clone(), touched: map[string]bool{"tags.flowmark.md": true}}
		doc := w.next.files["tags.flowmark.md"]
		doc.rows = append(doc.rows, cloneRow(doc.rows[0]))
		if _, _, err := w.render(); err == nil || err.Code != "INTERNAL" {
			f.t.Fatalf("render: %v; want INTERNAL", err)
		}
	})
}

// TestFaultOrder pins a refused write with faults of several rules (spec 10
// Diagnostics, "Write faults"): the rows in request order, within a row its value
// faults before its key fault, FOREIGN_KEY only when no row has another fault, a
// fault that needs a rendered file only when no key or value fault stands, and the
// code that of the first fault.
func TestFaultOrder(t *testing.T) {
	f := baseFixture(t)
	const dup, fk, con = "DUPLICATE_KEY", "FOREIGN_KEY", "CONSTRAINT"
	people := func(rows ...[]any) []byte {
		var params []any
		var more [][]int
		for i, r := range rows {
			params = append(params, r...)
			if i > 0 {
				more = append(more, []int{3 * i, 3*i + 1, 3*i + 2})
			}
		}
		body := map[string]any{"set": sets("id", "name", "age")}
		if more != nil {
			body["rows"] = more
		}
		return f.msg("insert", "people", body, params...)
	}
	cases := []struct {
		name string
		msg  []byte
		code string
		want []dupFault
	}{
		{"10#diagnostics: a row's check fault comes before its key fault", people([]any{"P1", "X", -1}), con,
			[]dupFault{{"people", "P1", -1, "", con, "ck_people_age"}, {"people", "P1", -1, "id", dup, "people.flowmark.md"}}},
		{"10#diagnostics: a row's title fault comes before its key fault", people([]any{"P1", "two\nlines", 1}), con,
			[]dupFault{{"people", "P1", -1, "name", con, "line break"}, {"people", "P1", -1, "id", dup, "people.flowmark.md"}}},
		{"10#diagnostics: the rows stand in request order, a key fault first when its row is first", people([]any{"P1", "X", 1}, []any{"P9", "Y", -1}), dup,
			[]dupFault{{"people", "P1", -1, "id", dup, "people.flowmark.md"}, {"people", "P9", -1, "", con, "ck_people_age"}}},
		{"10#diagnostics: a FOREIGN_KEY fault is not reported beside another fault", f.msg("insert", "items", map[string]any{"set": sets("id", "title", "body", "owner"), "rows": [][]int{{4, 5, 6, 7}}}, "T8", "A", "", "P9", "T1", "B", "", nil), dup,
			[]dupFault{{"items", "T1", -1, "id", dup, "items.flowmark.md"}}},
		{"10#diagnostics: a FOREIGN_KEY fault alone is the code", f.msg("insert", "items", map[string]any{"set": sets("id", "title", "body", "owner")}, "T8", "A", "", "P9"), fk,
			[]dupFault{{"items", "T8", -1, "owner", fk, "P9"}}},
		{"10#diagnostics: a files row placed nowhere for its key still has its check judged", f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "weight")}, "N1", "X", "", -1), con,
			[]dupFault{{"notes", "N1", -1, "", con, "ck_notes_weight"}, {"notes", "N1", -1, "id", dup, "notes/N1.flowmark.md"}}},
		{"10#diagnostics: a key fault stops the render, so no anchor fault is reported", f.msg("insert", "items", map[string]any{"set": sets("id", "title", "body")}, "T1", "Items row T1", "A body."), dup,
			[]dupFault{{"items", "T1", -1, "id", dup, "items.flowmark.md"}}},
	}
	for _, c := range cases {
		f.sub(t, c.name, func(f *fixture) { wantFaults(f.t, f.fails(c.msg, c.code), c.want) })
	}
}

// TestUpdate pins update (spec 60 Surfaces, spec 50 The state machine, spec 20
// Order).
func TestUpdate(t *testing.T) {
	byID := func(f *fixture, entity string, sets []any, id any, params ...any) []byte {
		params = append(params, id)
		return f.msg("update", entity, map[string]any{"set": sets, "where": where(pred("id", "eq", p(len(params)-1)))}, params...)
	}
	t.Run("50#the-state-machine: a transition inserts its history row at the request clock", func(t *testing.T) {
		f := fresh(t)
		res := f.want(byID(f, "items", sets("state"), "T3", "doing"), `{"affected":1,"keys":[]}`)
		f.commit(res)
		if !strings.Contains(f.file("items.flowmark.md"), "## items row T3\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n| 5 | waiting | doing | 2026-10-11 09:00:00.123456 |\n") {
			t.Fatalf("items:\n%s", f.file("items.flowmark.md"))
		}
	})
	t.Run("50#the-state-machine: a required column is copied into the history row", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(byID(f, "items", sets("state", "evidence"), "T2", "done", "Fuzzed."), `{"affected":1,"keys":[]}`))
		f.want(f.msg("one", "item_history", map[string]any{"columns": map[string]any{"mode": "all"}, "where": where(pred("id", "eq", p(0)))}, 5),
			`{"rows":[{"id":5,"item":"T2","from_state":"doing","to_state":"done","at":"2026-10-11 09:00:00.123456","evidence":"Fuzzed.","cause":null}]}`)
	})
	t.Run("40#the-commit-protocol: a detail section that appears moves the links to its row", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.ok(byID(f, "notes", sets("item"), "N3", "T3")))
		if !strings.Contains(f.file("notes/N3.flowmark.md"), "- item: [T3](../items.flowmark.md)\n") {
			t.Fatalf("N3:\n%s", f.file("notes/N3.flowmark.md"))
		}
		res := f.ok(byID(f, "items", sets("state"), "T3", "doing"))
		paths := []string{}
		for _, tg := range res.Post.Targets {
			paths = append(paths, tg.Path)
		}
		if strings.Join(paths, " ") != "items.flowmark.md notes/N3.flowmark.md" {
			t.Fatalf("targets %v", paths)
		}
		f.commit(res)
		if !strings.Contains(f.file("notes/N3.flowmark.md"), "- item: [T3](../items.flowmark.md#items-row-t3)\n") {
			t.Fatalf("N3:\n%s", f.file("notes/N3.flowmark.md"))
		}
	})
	t.Run("60#surfaces: plus and minus, NULL staying NULL and minus never below 0", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(f.msg("update", "people", map[string]any{"set": []any{map[string]any{"column": "age", "plus_p": 0}}, "where": where(pred("id", "in", map[string]any{"ps": []int{1, 2}}))}, 1, "P1", "P2"), `{"affected":2,"keys":[]}`))
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"age"}}}), `{"rows":[{"id":"P1","age":37},{"id":"P2","age":null}]}`)
		f.commit(f.ok(f.msg("update", "people", map[string]any{"set": []any{map[string]any{"column": "age", "minus_p": 0}}, "where": where(pred("id", "eq", p(1)))}, 100, "P1")))
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"age"}}}), `{"rows":[{"id":"P1","age":0},{"id":"P2","age":null}]}`)
	})
	t.Run("60#surfaces: updated takes the request clock", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.ok(byID(f, "people", sets("name"), "P2", "Maxine")))
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"updated_at"}}}),
			`{"rows":[{"id":"P1","updated_at":null},{"id":"P2","updated_at":"2026-10-11 09:00:00.123456"}]}`)
	})
	t.Run("60#surfaces: an optimistic update that matches", func(t *testing.T) {
		f := fresh(t)
		f.want(f.msg("update", "people", map[string]any{"set": sets("name"), "where": where(pred("id", "eq", p(1))), "optimistic": map[string]any{"column": "score", "p": 2}}, "Ada L", "P1", "1.50"), `{"affected":1,"keys":[]}`)
	})
	t.Run("20#order: an update to k moves the row and the siblings follow", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.ok(byID(f, "notes", sets("position"), "N3", 1)))
		f.want(f.msg("all", "notes", map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"item"}, "add": []string{"position"}}}),
			`{"rows":[{"id":"N3","parent":null,"position":1},{"id":"N1","parent":null,"position":2},{"id":"N2","parent":"N1","position":1}]}`)
	})
	t.Run("20#order: a parent change appends and the old siblings close the gap", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.ok(byID(f, "items", sets("parent"), "T3", "T1")))
		f.want(f.msg("all", "items", map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"owner"}, "add": []string{"position"}}}),
			`{"rows":[{"id":"T1","parent":null,"position":1},{"id":"T2","parent":"T1","position":1},{"id":"T3","parent":"T1","position":2},{"id":"T4","parent":null,"position":2}]}`)
	})
	t.Run("20#storage: an update of a block row rewrites its parent's file", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.ok(byID(f, "steps", sets("done"), "S1", true)))
		if !strings.Contains(f.file("notes/N1.flowmark.md"), "- S1 Draft done:true\n") {
			t.Fatalf("N1:\n%s", f.file("notes/N1.flowmark.md"))
		}
	})
	t.Run("50#a-state-changed-outside-the-engine: a gapped row's other columns can be updated", func(t *testing.T) {
		docs := baseDocs()
		docs["items.flowmark.md"] = strings.Replace(docs["items.flowmark.md"], "- [ ] T3 Review", "- [~] T3 Review", 1)
		f := catalogFixture(t, docs).on(t)
		res := f.ok(byID(f, "items", sets("title"), "T3", "Reviewed"))
		if tg := res.Post.Targets; len(tg) != 1 || !strings.Contains(string(tg[0].After), "- [~] T3 Reviewed\n") {
			t.Fatalf("targets %+v", tg)
		}
	})

	f := baseFixture(t)
	rejects := []struct {
		name string
		msg  []byte
		code string
		want []dupFault
	}{
		{"50#the-state-machine: a change that no transition declares", byID(f, "items", sets("state"), "T1", "waiting"), "CONSTRAINT",
			[]dupFault{{"items", "T1", -1, "state", "CONSTRAINT", "no declared transition"}}},
		{"50#the-state-machine: a transition without its required column", byID(f, "items", sets("state"), "T2", "done"), "CONSTRAINT",
			[]dupFault{{"items", "T2", -1, "evidence", "CONSTRAINT", "requires evidence"}}},
		{"50#the-state-machine: a required column assigned the empty text", byID(f, "items", sets("state", "evidence"), "T2", "done", ""), "CONSTRAINT",
			[]dupFault{{"items", "T2", -1, "evidence", "CONSTRAINT", "requires evidence"}}},
		{"50#the-state-machine: a terminal entry without its required column", byID(f, "items", sets("state"), "T3", "dropped"), "CONSTRAINT",
			[]dupFault{{"items", "T3", -1, "cause", "CONSTRAINT", "requires cause"}}},
		{"50#the-state-machine: a state outside the state set", byID(f, "items", sets("state"), "T3", "lost"), "CONSTRAINT",
			[]dupFault{{"items", "T3", -1, "state", "CONSTRAINT", "not a state"}}},
		{"50#keys-and-references: an update to a reference that names no row", byID(f, "items", sets("owner"), "T3", "P9"), "FOREIGN_KEY",
			[]dupFault{{"items", "T3", -1, "owner", "FOREIGN_KEY", "P9"}}},
		{"50#hierarchy-and-order: a parent cycle", byID(f, "notes", sets("parent"), "N1", "N2"), "CONSTRAINT",
			[]dupFault{{"notes", "N1", -1, "parent", "CONSTRAINT", "cycle"}}},
		{"50#hierarchy-and-order: a parent outside the row's block", byID(f, "items", sets("parent"), "T3", "T3"), "CONSTRAINT",
			[]dupFault{{"items", "T3", -1, "parent", "CONSTRAINT", "cycle"}}},
		{"20#order: an update to an order outside 1..n+1", byID(f, "notes", sets("position"), "N3", 4), "CONSTRAINT",
			[]dupFault{{"notes", "N3", -1, "position", "CONSTRAINT", "1..2"}}},
		{"50#keys-and-references: an update to a unique key another row holds", byID(f, "people", sets("email"), "P2", "ada@x.io"), "DUPLICATE_KEY",
			[]dupFault{{"people", "P2", -1, "email", "DUPLICATE_KEY", "P1"}}},
		{"50#checks: an update that makes a check false", byID(f, "people", sets("age"), "P2", -5), "CONSTRAINT",
			[]dupFault{{"people", "P2", -1, "", "CONSTRAINT", "ck_people_age"}}},
		{"10#readable-values: a value not of its column type", byID(f, "people", sets("score"), "P2", "1.234"), "CONSTRAINT",
			[]dupFault{{"people", "P2", -1, "score", "CONSTRAINT", "score"}}},
		{"10#single-line-places: a title with a line feed", byID(f, "items", sets("title"), "T3", "two\nlines"), "CONSTRAINT",
			[]dupFault{{"items", "T3", -1, "title", "CONSTRAINT", ""}}},
	}
	for _, c := range rejects {
		f.sub(t, c.name, func(f *fixture) { wantFaults(f.t, f.fails(c.msg, c.code), c.want) })
	}
	limitDocs := baseDocs()
	limitDocs["items.flowmark.md"] = strings.Replace(limitDocs["items.flowmark.md"], "- [ ] T3 Review the design\n", "- [ ] T3 Review the design\n- [ ] T5 Fifth\n", 1)
	lf := catalogFixture(t, limitDocs)
	gapDocs := baseDocs()
	gapDocs["items.flowmark.md"] = strings.Replace(gapDocs["items.flowmark.md"], "- [ ] T3 Review", "- [~] T3 Review", 1)
	gf := catalogFixture(t, gapDocs)
	codes := []struct {
		name string
		f    *fixture
		msg  []byte
		code string
	}{
		{"50#the-state-machine: a transition past a limit", lf, lf.msg("update", "items", map[string]any{"set": sets("state"), "where": where(pred("id", "in", map[string]any{"ps": []int{1, 2}}))}, "doing", "T3", "T5"), "CONSTRAINT"},
		{"50#a-state-changed-outside-the-engine: a transition of a gapped row", gf, byID(gf, "items", sets("state", "evidence"), "T3", "done", "x"), "CONSTRAINT"},
		{"60#surfaces: an optimistic update that matches no row", f, f.msg("update", "people", map[string]any{"set": sets("name"), "where": where(pred("id", "eq", p(1))), "optimistic": map[string]any{"column": "score", "p": 2}}, "Ada L", "P1", "9.99"), "OPTIMISTIC_LOCK"},
		{"20#order: an update that assigns the order matches exactly one row", f, f.msg("update", "notes", map[string]any{"set": sets("position"), "where": where(pred("parent", "is_null", nil))}, 1), "CONSTRAINT"},
		{"60#surfaces: an update without where", f, f.msg("update", "notes", map[string]any{"set": sets("title")}, "x"), "IR_INVALID"},
		{"60#surfaces: an update of the key column has no rule yet", f, byID(f, "notes", sets("id"), "N3", "N7"), "CAPABILITY_UNSUPPORTED"},
		{"60#surfaces: a set to null of a non-null column", f, f.msg("update", "notes", map[string]any{"set": []any{map[string]any{"column": "title", "null": true}}, "where": where(pred("id", "eq", p(0)))}, "N3"), "IR_INVALID"},
	}
	for _, c := range codes {
		c.f.sub(t, c.name, func(f *fixture) { f.fails(c.msg, c.code) })
	}
}

// TestDelete pins delete and restore (spec 60 Surfaces, spec 50 Keys and
// references).
func TestDelete(t *testing.T) {
	byID := func(f *fixture, kind, entity string, id any) []byte {
		return f.msg(kind, entity, map[string]any{"where": where(pred("id", "eq", p(0)))}, id)
	}
	t.Run("60#surfaces: a delete of a document row", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(byID(f, "delete", "tags", 2), `{"affected":1,"keys":[]}`))
		f.want(f.msg("count", "tags", nil), `{"count":1}`)
	})
	t.Run("20#order: a delete closes the gap", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.ok(byID(f, "delete", "items", "T3")))
		f.want(f.msg("all", "items", map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"owner", "parent"}, "add": []string{"position"}}}),
			`{"rows":[{"id":"T1","position":1},{"id":"T2","position":1},{"id":"T4","position":2}]}`)
	})
	t.Run("10#encoding-and-line-structure: a delete of a files row removes its whole file", func(t *testing.T) {
		f := fresh(t)
		res := f.ok(byID(f, "delete", "notes", "N3"))
		if tg := res.Post.Targets; len(tg) != 1 || tg[0].Path != "notes/N3.flowmark.md" || tg[0].After != nil {
			t.Fatalf("targets %+v", tg)
		}
		f.commit(res)
	})
	t.Run("60#surfaces: a soft delete sets the column and keeps references", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(byID(f, "delete", "people", "P1"), `{"affected":1,"keys":[]}`))
		f.want(f.msg("all", "people", map[string]any{"columns": none}), `{"rows":[{"id":"P2"}]}`)
	})
	t.Run("60#surfaces: restore clears the soft delete column", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(byID(f, "restore", "people", "P3"), `{"affected":1,"keys":[]}`))
		f.want(f.msg("all", "people", map[string]any{"columns": none}), `{"rows":[{"id":"P1"},{"id":"P2"},{"id":"P3"}]}`)
	})
	t.Run("60#surfaces: restore of a row that is not deleted changes nothing", func(t *testing.T) {
		f := fresh(t)
		res := f.want(byID(f, "restore", "people", "P1"), `{"affected":0,"keys":[]}`)
		if len(res.Post.Targets) != 0 {
			t.Fatalf("targets %+v", res.Post.Targets)
		}
	})
	f := baseFixture(t)
	f.sub(t, "50#keys-and-references: a delete of a row that nothing references", func(f *fixture) {
		f.want(byID(f, "delete", "notes", "N2"), `{"affected":1,"keys":[]}`)
	})
	rejects := []struct {
		name string
		msg  []byte
		want []dupFault
	}{
		{"50#keys-and-references: a delete of a row that has history rows", byID(f, "delete", "items", "T2"), []dupFault{{"items", "T2", -1, "", "FOREIGN_KEY", "item_history"}}},
		{"50#keys-and-references: a delete of a referenced row", byID(f, "delete", "items", "T1"), []dupFault{{"items", "T1", -1, "", "FOREIGN_KEY", "items T2"}}},
		{"50#keys-and-references: a delete of a row that children name as parent", byID(f, "delete", "notes", "N1"), []dupFault{{"notes", "N1", -1, "", "FOREIGN_KEY", "notes N2"}}},
	}
	for _, c := range rejects {
		f.sub(t, c.name, func(f *fixture) { wantFaults(f.t, f.fails(c.msg, "FOREIGN_KEY"), c.want) })
	}
	steps := baseDocs()
	delete(steps, "notes/N2.flowmark.md")
	sf := catalogFixture(t, steps)
	sf.sub(t, "50#keys-and-references: a delete of a row whose scope holds block rows", func(f *fixture) {
		wantFaults(f.t, f.fails(byID(f, "delete", "notes", "N1"), "FOREIGN_KEY"), []dupFault{{"notes", "N1", -1, "", "FOREIGN_KEY", "steps S1"}})
	})
	t.Run("20#keys: a key changes by an insert under the new key, the references pointed at it and the old row deleted", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.ok(f.msg("update", "notes", map[string]any{"set": sets("item"), "where": where(pred("id", "eq", p(1)))}, "T3", "N3")))
		f.fails(byID(f, "delete", "items", "T3"), "FOREIGN_KEY")
		f.commit(f.want(f.msg("insert", "items", map[string]any{"set": sets("id", "title", "body")}, "T9", "Review the design", ""), `{"affected":1,"keys":["T9"]}`))
		f.commit(f.ok(f.msg("update", "notes", map[string]any{"set": sets("item"), "where": where(pred("id", "eq", p(1)))}, "T9", "N3")))
		f.commit(f.want(byID(f, "delete", "items", "T3"), `{"affected":1,"keys":[]}`))
		if got := f.file("notes/N3.flowmark.md"); !strings.Contains(got, "- item: [T9](../items.flowmark.md)\n") {
			t.Fatalf("N3:\n%s", got)
		}
	})
	noSteps := baseDocs()
	noSteps["notes/N1.flowmark.md"] = lines("# First note", "- item: [T1](../items.flowmark.md#items-row-t1)", "- position: 1", "", "Note body.")
	nf := catalogFixture(t, noSteps)
	for _, order := range [][]any{{"N1", "N2"}, {"N2", "N1"}} {
		nf.sub(t, "50#keys-and-references: one delete of a referenced row and the row referencing it, parameters "+order[0].(string)+" then "+order[1].(string), func(f *fixture) {
			err := f.fails(f.msg("delete", "notes", map[string]any{"where": where(pred("id", "in", map[string]any{"ps": []int{0, 1}}))}, order...), "FOREIGN_KEY")
			wantFaults(f.t, err, []dupFault{{"notes", "N1", -1, "", "FOREIGN_KEY", "notes N2"}})
		})
	}
	t.Run("50#keys-and-references: the referencing row deleted by an earlier request, then the referenced row", func(t *testing.T) {
		f := catalogFixture(t, noSteps).on(t)
		f.commit(f.want(byID(f, "delete", "notes", "N2"), `{"affected":1,"keys":[]}`))
		f.commit(f.want(byID(f, "delete", "notes", "N1"), `{"affected":1,"keys":[]}`))
	})
	broken := baseDocs()
	broken["notes/N9.flowmark.md"] = "no heading\n"
	bf := catalogFixture(t, broken)
	codes := []struct {
		name string
		f    *fixture
		msg  []byte
		code string
	}{
		{"10#diagnostics: a delete whose restrict check needs an unavailable table", bf, byID(bf, "delete", "items", "T3"), "CONSTRAINT"},
		{"60#surfaces: restore of a table without soft_delete", f, byID(f, "restore", "tags", 1), "IR_INVALID"},
		{"60#surfaces: restore by a column that is no key", f, f.msg("restore", "people", map[string]any{"where": where(pred("name", "eq", p(0)))}, "Gone"), "IR_INVALID"},
		{"60#surfaces: restore that assigns the soft delete column", f, f.msg("restore", "people", map[string]any{"set": []any{map[string]any{"column": "deleted_at", "null": true}}, "where": where(pred("id", "eq", p(0)))}, "P3"), "IR_INVALID"},
		{"60#surfaces: a delete without where", f, f.msg("delete", "tags", nil), "IR_INVALID"},
	}
	for _, c := range codes {
		c.f.sub(t, c.name, func(f *fixture) { f.fails(c.msg, c.code) })
	}
}
