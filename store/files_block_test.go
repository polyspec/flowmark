package store

import (
	"bytes"
	"testing"
)

// blockParseCases are the files rows with child blocks: the block sections at level
// two of the row, their rows' detail sections, and a block under a block row, which
// the catalog places at level six at most.
func blockParseCases() []parseCase {
	return []parseCase{
		accept("10#document-order: a list block under a files row", withBase("## task_steps", "- S1 Draft done:true", "", "### task_steps row S1", "", "Step body.")),
		accept("10#document-order: a table block under a block row, at level six", withBase(
			"## task_steps", "- S1 Draft done:true", "", "### task_steps row S1", "",
			"#### step_refs", "| id | label |", "| --- | --- |", "| 1 | spec |",
			"", "##### step_refs row 1", "", "###### note", "", "Ref note.")),
		accept("10#sections-and-their-values: a markdown section before a child block", withBase(
			"## cause", "", "Cause text.", "", "## task_steps", "- S1 Draft done:true")),
		noncanonical("10#document-order: an empty block heading of a files row", 5, withBase("## task_steps")),
		noncanonical("10#detail-sections: a block row's detail section without content", 8, withBase(
			"## task_steps", "- S1 Draft done:true", "", "### task_steps row S1")),
		reject("10#list-rows: a placement foreign key written as a field of a block row", RuleBlock, 6, withBase(
			"## task_steps", "- S1 Draft task:T1 done:true")),
		reject("10#list-rows: a non-null field of a block row missing", RuleBlock, 6, withBase(
			"## task_steps", "- S1 Draft")),
		reject("10#detail-sections: a block row's detail names no row", RuleBlock, 8, withBase(
			"## task_steps", "- S1 Draft done:true", "", "### task_steps row S9", "", "x")),
		reject("10#sections-and-their-values: a block section written twice", RuleSection, 8, withBase(
			"## task_steps", "- S1 Draft done:true", "", "## task_steps", "- S2 Two done:true")),
		reject("10#table-rows: a placement foreign key written as a cell of a block table", RuleBlock, 11, withBase(
			"## task_steps", "- S1 Draft done:true", "", "### task_steps row S1", "",
			"#### step_refs", "| id | step | label |", "| --- | --- | --- |", "| 1 | S1 | spec |")),
	}
}

// stepRow is a task_steps row of the tasks fixture with its position, done flag,
// body and child rows; a nil blocks argument has no child block.
func stepRow(key, title, position, done, body string, blocks map[string][]*Row) *Row {
	sc := map[string]*string{"position": sp(position), "done": sp(done)}
	return &Row{Table: "task_steps", Key: key, Title: title, Body: body, Scalars: sc,
		Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}, Blocks: blocks}
}

// taskWithBlocks is a tasks row whose child blocks are the given rows.
func taskWithBlocks(blocks map[string][]*Row) *Row {
	return &Row{Table: "tasks", Key: "T1", Title: "Title",
		Scalars:  map[string]*string{"position": sp("1"), "done": sp("true")},
		Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}, Blocks: blocks}
}

// TestRenderFileBlocks requires that a files row writes its child blocks after its
// sections, in the form the parse reads back, and that a block the row does not
// declare, a placement foreign key given as a scalar or a row of another table is
// refused, while an empty block is not written.
func TestRenderFileBlocks(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	ref := &Row{Table: "step_refs", Key: "1", Scalars: map[string]*string{"label": sp("spec")},
		Sections: map[string]*string{"note": sp("Ref note.\n")}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
	steps := []*Row{stepRow("S1", "Draft", "1", "true", "Step body.\n", map[string][]*Row{"step_refs": {ref}})}
	want := "# Title\n- position: 1\n- done: true\n\n" +
		"## task_steps\n- S1 Draft done:true\n\n" +
		"### task_steps row S1\n\nStep body.\n\n" +
		"#### step_refs\n| id | label |\n| --- | --- |\n| 1 | spec |\n\n" +
		"##### step_refs row 1\n\n###### note\n\nRef note.\n"

	out, diags := RenderFile(cat, "tasks", taskWithBlocks(map[string][]*Row{"task_steps": steps}), lk)
	if len(diags) > 0 || string(out) != want {
		t.Fatalf("written %q with %+v; want %q", out, diags, want)
	}
	back, pd := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", out, lk)
	if len(pd) > 0 || back == nil {
		t.Fatalf("written blocks do not parse: %+v", pd)
	}
	if got := back.Blocks["task_steps"]; len(got) != 1 || len(got[0].Blocks["step_refs"]) != 1 {
		t.Fatalf("read blocks %+v; want the step and its reference", back.Blocks)
	}
	if again, _ := RenderFile(cat, "tasks", back, lk); !bytes.Equal(again, out) {
		t.Fatalf("P2: %q != %q", again, out)
	}

	out, diags = RenderFile(cat, "tasks", taskWithBlocks(map[string][]*Row{"task_steps": nil}), lk)
	if len(diags) > 0 || string(out) != "# Title\n- position: 1\n- done: true\n" {
		t.Fatalf("an empty block: written %q with %+v", out, diags)
	}

	refused := []struct {
		name string
		row  *Row
	}{
		{"a block the files table does not declare", taskWithBlocks(map[string][]*Row{"item_history": steps})},
		{"a placement foreign key assigned to a block row", taskWithBlocks(map[string][]*Row{"task_steps": {
			withScalar(stepRow("S1", "Draft", "1", "true", "", nil), "task", "T1")}})},
		{"a row of another table in a block", taskWithBlocks(map[string][]*Row{"task_steps": {
			stepRow("S1", "Draft", "1", "true", "", map[string][]*Row{"step_refs": {withRowTable(ref, "tasks")}})}})},
	}
	for _, c := range refused {
		out, diags := RenderFile(cat, "tasks", c.row, lk)
		if out != nil || len(diags) == 0 || diags[0].Rule != RuleConstraint {
			t.Fatalf("%s: written %q with %+v; want CONSTRAINT", c.name, out, diags)
		}
	}
}

// withRowTable sets the table of row r.
func withRowTable(r *Row, table string) *Row {
	r.Table = table
	return r
}
