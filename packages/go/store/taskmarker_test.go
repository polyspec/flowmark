package store

import (
	"testing"
)

// markerValue is the value of the task marker column of the first row of rows, or "NULL".
func markerValue(rows []*Row, column string) string {
	if len(rows) == 0 || rows[0].Scalars[column] == nil {
		return "NULL"
	}
	return *rows[0].Scalars[column]
}

// TestTaskMarkerRead pins the task marker of a list row (spec 30 Markdown settings, orm DBS-80): a varchar(1) column is
// the one character between the brackets, the empty text `[]`; a bool column is the GFM checkbox `[ ]` for false and
// `[x]` for true, `[X]` reading as true and format.noncanonical; a row without a box is NULL, and anything else between
// the brackets is format.block.
func TestTaskMarkerRead(t *testing.T) {
	cases := []struct {
		name   string
		table  string
		line   string // the one row of the document
		value  string // the value read, "NULL" for none; "" with a rule that refuses
		rule   string // "" when the document is accepted and canonical
		column int    // the column of the diagnostic
	}{
		{"30#markdown-settings: a varchar(1) marker holds its character", "chores", "- [?] C1 Ask", "?", "", 0},
		{"30#markdown-settings: a varchar(1) marker holds a space", "chores", "- [ ] C1 Ask", " ", "", 0},
		{"30#markdown-settings: a varchar(1) marker holds x", "chores", "- [x] C1 Ask", "x", "", 0},
		{"30#markdown-settings: a varchar(1) marker holds a character of several bytes", "chores", "- [✓] C1 Ask", "✓", "", 0},
		{"30#markdown-settings: a varchar(1) marker holds a closing bracket", "chores", "- []] C1 Ask", "]", "", 0},
		{"30#markdown-settings: the empty text is []", "chores", "- [] C1 Ask", "", "", 0},
		{"30#markdown-settings: a row without a box is NULL", "chores", "- C1 Ask", "NULL", "", 0},
		{"30#markdown-settings: a marker of two characters", "chores", "- [ab] C1 Ask", "", RuleBlock, 4},
		{"30#markdown-settings: a marker without its closing bracket", "chores", "- [a C1 Ask", "", RuleBlock, 4},
		{"30#markdown-settings: a marker not followed by a space", "chores", "- [a]C1 Ask", "", RuleBlock, 6},
		{"30#markdown-settings: a bool marker unchecked is false", "todos", "- [ ] D1 Open", "false", "", 0},
		{"30#markdown-settings: a bool marker checked is true", "todos", "- [x] D1 Open", "true", "", 0},
		{"30#markdown-settings: a bool marker with a capital X is true and noncanonical", "todos", "- [X] D1 Open", "true", RuleNoncanonical, 4},
		{"30#markdown-settings: a bool row without a box is NULL", "todos", "- D1 Open", "NULL", "", 0},
		{"30#markdown-settings: a bool marker holds no other character", "todos", "- [?] D1 Open", "", RuleBlock, 4},
		{"30#markdown-settings: a bool marker is never empty", "todos", "- [] D1 Open", "", RuleBlock, 4},
	}
	cat := loadDocumentFixture(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			column := map[string]string{"chores": "mark", "todos": "done"}[c.table]
			text := raw("# "+c.table, c.line)
			rows, diags, err := ParseDocument(cat, c.table, c.table+".flowmark.md", []byte(text), documentLinker())
			if err != nil {
				t.Fatal(err)
			}
			if c.rule == "" {
				if len(diags) > 0 {
					t.Fatalf("accepted input has diagnostics: %+v", diags)
				}
				if got := markerValue(rows, column); got != c.value {
					t.Fatalf("%s = %q, want %q", column, got, c.value)
				}
				wantP1(t, checkDocumentP2(t, cat, c.table, rows, documentLinker()), text)
				return
			}
			if len(diags) == 0 || diags[0].Rule != c.rule || diags[0].Line != 2 || diags[0].Column != c.column {
				t.Fatalf("diagnostics %+v; want %s at 2:%d", diags, c.rule, c.column)
			}
			if c.value != "" {
				if got := markerValue(rows, column); got != c.value {
					t.Fatalf("%s = %q, want %q", column, got, c.value)
				}
			}
		})
	}
}

// TestTaskMarkerRendered pins the writer of a task marker (spec 30 Markdown settings): each value is written as the
// read reads it back, and a value the box cannot show is refused at the column.
func TestTaskMarkerRendered(t *testing.T) {
	row := func(table, key, column string, v *string) *Row {
		return &Row{Table: table, Key: key, Title: "Row " + key, Scalars: map[string]*string{column: v}, Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
	}
	cat := loadDocumentFixture(t)
	accepted := []struct {
		name, table, column string
		value               *string
		want                string
	}{
		{"30#markdown-settings: a varchar(1) value is written in the box", "chores", "mark", sp("!"), "- [!] C1 Row C1"},
		{"30#markdown-settings: the empty text is written []", "chores", "mark", sp(""), "- [] C1 Row C1"},
		{"30#markdown-settings: NULL is written without a box", "chores", "mark", nil, "- C1 Row C1"},
		{"30#markdown-settings: a bool false is the unchecked box", "todos", "done", sp("false"), "- [ ] C1 Row C1"},
		{"30#markdown-settings: a bool true is the checked box", "todos", "done", sp("true"), "- [x] C1 Row C1"},
		{"30#markdown-settings: a bool NULL is written without a box", "todos", "done", nil, "- C1 Row C1"},
	}
	for _, c := range accepted {
		t.Run(c.name, func(t *testing.T) {
			out, refused, err := RenderDocument(cat, c.table, c.table+".flowmark.md", []*Row{row(c.table, "C1", c.column, c.value)}, documentLinker())
			if err != nil || len(refused) > 0 {
				t.Fatalf("render: %v %+v", err, refused)
			}
			if want := raw("# "+c.table, c.want); string(out) != want {
				t.Fatalf("written\n%s\nwant\n%s", out, want)
			}
		})
	}
	refused := []struct {
		name, table, column, value string
	}{
		{"30#markdown-settings: a value longer than the varchar(1)", "chores", "mark", "ab"},
		{"30#markdown-settings: a line break in the box", "chores", "mark", "\n"},
		{"30#markdown-settings: a value that is not a bool", "todos", "done", "maybe"},
	}
	for _, c := range refused {
		t.Run(c.name, func(t *testing.T) {
			_, refused, err := RenderDocument(cat, c.table, c.table+".flowmark.md", []*Row{row(c.table, "C1", c.column, sp(c.value))}, documentLinker())
			if err != nil {
				t.Fatal(err)
			}
			if len(refused) != 1 || refused[0].Column != c.column || refused[0].Rule != RuleConstraint {
				t.Fatalf("refusals %+v; want one CONSTRAINT at %s", refused, c.column)
			}
		})
	}
}
