package store

import (
	"strconv"
	"strings"
	"testing"
)

// dupFault is a refusal expected of a write: the row by table, key and index, the
// column, the rule, and a text its message holds, which names the row it names.
type dupFault struct {
	table, key string
	index      int
	column     string
	rule       string
	names      string
}

// expectFaults requires the refusals rf to be exactly the faults want, in order.
func expectFaults(t *testing.T, out []byte, rf []Refusal, want []dupFault) {
	t.Helper()
	if out != nil || len(rf) != len(want) {
		t.Fatalf("written %q with refusals %+v; want %+v", out, rf, want)
	}
	for k, w := range want {
		g := rf[k]
		if g.Table != w.table || g.Key != w.key || g.Index != w.index || g.Column != w.column || g.Rule != w.rule || !strings.Contains(g.Message, w.names) {
			t.Fatalf("refusal %d is %+v; want %+v", k, g, w)
		}
	}
}

// TestDuplicateFaultOnce requires that a row given to a write has at most one
// duplicate fault, at its key column, naming the first row given before it with its
// key byte for byte. Keys equal only without regard to case are written: the codec
// cannot tell a row the write introduces from one a human edit left, and the
// executor's key checks own that rule (spec 10 Diagnostics, "Write faults").
func TestDuplicateFaultOnce(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	done := map[string]string{"done": "false"}
	items := func(keys ...string) []*Row {
		var rows []*Row
		for i, k := range keys {
			rows = append(rows, item(k, "Row "+k, "waiting", strconv.Itoa(i+1), done))
		}
		return rows
	}
	const dup = RuleDuplicateKey
	cases := []struct {
		name string
		keys []string
		want []dupFault
	}{
		{"10#diagnostics: a byte-equal pair is one fault at the later row", []string{"T1", "T1"}, []dupFault{
			{"items", "T1", 1, "id", dup, "index 0"}}},
		{"10#diagnostics: a byte-equal twin beside a case-only twin is the one fault", []string{"T1", "T1", "t1"}, []dupFault{
			{"items", "T1", 1, "id", dup, "index 0"}}},
		{"10#diagnostics: a case-only twin before a byte-equal pair faults the later of the pair", []string{"t1", "t1", "T1"}, []dupFault{
			{"items", "t1", 1, "id", dup, "index 0"}}},
		{"10#diagnostics: a byte-equal pair after a case-only twin names the first of the pair", []string{"t1", "T1", "T1"}, []dupFault{
			{"items", "T1", 2, "id", dup, "index 1"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, rf, err := RenderDocument(cat, "items", items(c.keys...), lk)
			if err != nil {
				t.Fatal(err)
			}
			expectFaults(t, out, rf, c.want)
		})
	}
	t.Run("10#diagnostics: keys equal only without regard to case are written", func(t *testing.T) {
		out, rf, err := RenderDocument(cat, "items", items("AB", "Ab", "ab"), lk)
		if err != nil || len(rf) > 0 || out == nil {
			t.Fatalf("written %q with refusals %+v, %v", out, rf, err)
		}
	})
}

// TestDuplicateFaultOnceInFilesBlock requires the same of the rows of a block of a
// files row: a byte-equal pair gives one fault, and a case-only twin none.
func TestDuplicateFaultOnceInFilesBlock(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	steps := []*Row{
		stepRow("S1", "One", "1", "true", "", nil),
		stepRow("S1", "Two", "2", "true", "", nil),
		stepRow("s1", "Three", "3", "true", "", nil),
	}
	out, rf := renderFileOf(t, cat, "tasks", taskWithBlocks(map[string][]*Row{"task_steps": steps}), lk)
	t.Run("10#diagnostics: a block of a files row faults the later of a byte-equal pair once", func(t *testing.T) {
		expectFaults(t, out, rf, []dupFault{
			{"task_steps", "S1", 1, "id", RuleDuplicateKey, "index 0"}})
	})
}

// TestUndeclaredBlockNamed requires that a files row's block that its table does not
// declare is refused at the name of the block, as a document row's is.
func TestUndeclaredBlockNamed(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	steps := []*Row{stepRow("S1", "Draft", "1", "true", "", nil)}
	out, rf := renderFileOf(t, cat, "tasks", taskWithBlocks(map[string][]*Row{"item_history": steps}), lk)
	t.Run("10#diagnostics: a block a files row does not declare is refused at its name", func(t *testing.T) {
		expectFaults(t, out, rf, []dupFault{{"tasks", "T1", -1, "item_history", RuleConstraint, "item_history"}})
	})
}
