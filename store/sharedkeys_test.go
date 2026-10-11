package store

import (
	"bytes"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

// sharedKeysRoot is the fixture of keys that rows of two blocks share byte for byte:
// the document list items, whose rows hold the block tables steps (with a body and
// a parent) and refs (with a foreign key step to steps); and the files table tasks,
// whose rows hold the block list task_steps and the block table task_refs (with a
// foreign key step to task_steps).
const sharedKeysRoot = "testdata/sharedkeys"

func loadSharedKeys(t testing.TB) *catalog.Catalog {
	t.Helper()
	cat, diags := catalog.Load(sharedKeysRoot)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("sharedkeys fixture catalog: %+v", diags)
	}
	return cat
}

// sharedLinker is a stubLinker that returns ErrKeyShared for the keys, by
// "<table>/<key>", that rows of several files of the corpus share byte for byte.
type sharedLinker struct {
	stubLinker
	shared map[string]bool
}

func (s sharedLinker) Target(from, table, key string) (string, error) {
	if s.shared[table+"/"+key] {
		return "", ErrKeyShared
	}
	return s.stubLinker.Target(from, table, key)
}

// sharedDoc is the items file of the rows T1 and T2, each with a detail section
// holding the lines given for it, in the order of the keys given.
func sharedDoc(order []string, details map[string][]string) string {
	lines := []string{"# items", "- T1 One", "- T2 Two"}
	for _, key := range order {
		lines = append(lines, "", "## items row "+key)
		lines = append(lines, details[key]...)
	}
	return raw(lines...)
}

// refsBlock is the refs block of an items row with the rows given.
func refsBlock(rows ...string) []string {
	return append([]string{"", "### refs", "| id | step |", "| --- | --- |"}, rows...)
}

// stepsBlock is the steps block of an items row with the rows given.
func stepsBlock(rows ...string) []string {
	return append([]string{"", "### steps", "| id | parent |", "| --- | --- |"}, rows...)
}

// stepDetail is the detail section of the steps row key with a one-line body.
func stepDetail(key string) []string {
	return []string{"", "#### steps row " + key, "", "Body."}
}

// join concatenates line groups.
func join(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// TestSharedKeyLinks requires that a link to a key that rows of two blocks of the
// file share byte for byte is read as spec 10 states: a parent link derives its
// target from the row of its own block, and any other link has no derived target,
// so it is read without a target check, whatever the order of the rows.
func TestSharedKeyLinks(t *testing.T) {
	cat := loadSharedKeys(t)
	lk := stubLinker{}
	const anchor, file = "#steps-row-s1", "items.flowmark.md"
	both := []string{"T1", "T2"}
	// refsCase is the file whose block link holds the refs row R1 linking S1 with
	// target, and whose block detail holds the detail section of its S1.
	refsCase := func(link, detail, target string, order []string) string {
		blocks := map[string][]string{}
		for _, key := range both {
			var g [][]string
			if key == link {
				g = append(g, refsBlock("| R1 | [S1]("+target+") |"))
			}
			g = append(g, stepsBlock("| S1 |  |"))
			if key == detail {
				g = append(g, stepDetail("S1"))
			}
			blocks[key] = join(g...)
		}
		return sharedDoc(order, blocks)
	}
	cases := []struct {
		name string
		text string
		line int    // the line of the one noncanonical diagnostic, 0 when the file has none
		col  int    // the column of that diagnostic, 0 when not checked
		msg  string // a text its message holds, "" when not checked
	}{
		{"10#foreign-keys-as-links: a parent link in the second block takes its target from its own block", sharedDoc(both, map[string][]string{
			"T1": stepsBlock("| S1 |  |"),
			"T2": join(stepsBlock("| S1 |  |", "| S2 | [S1]("+anchor+") |"), stepDetail("S1")),
		}), 0, 0, ""},
		{"10#foreign-keys-as-links: a parent link in the first block takes its target from its own block, the mirror", sharedDoc(both, map[string][]string{
			"T1": join(stepsBlock("| S1 |  |", "| S2 | [S1]("+anchor+") |"), stepDetail("S1")),
			"T2": stepsBlock("| S1 |  |"),
		}), 0, 0, ""},
		{"10#foreign-keys-as-links: a parent link whose key another block shares is checked against the row of its own block", sharedDoc(both, map[string][]string{
			"T1": stepsBlock("| S1 |  |"),
			"T2": join(stepsBlock("| S1 |  |", "| S2 | [S1]("+file+") |"), stepDetail("S1")),
		}), 18, 8, "the link target is " + anchor},
		{"10#foreign-keys-as-links: a shared-key link in the first block, the detail in the first block", refsCase("T1", "T1", anchor, both), 0, 0, ""},
		{"10#foreign-keys-as-links: a shared-key link in the first block, the detail in the second block", refsCase("T1", "T2", file, both), 0, 0, ""},
		{"10#foreign-keys-as-links: a shared-key link in the second block, the detail in the first block", refsCase("T2", "T1", file, both), 0, 0, ""},
		{"10#foreign-keys-as-links: a shared-key link in the second block, the detail in the second block", refsCase("T2", "T2", anchor, both), 0, 0, ""},
		{"10#foreign-keys-as-links: a shared-key link with a target matching neither row", refsCase("T1", "T1", "elsewhere.flowmark.md", both), 0, 0, ""},
		{"10#foreign-keys-as-links: a shared-key link with the rows in another order has only the row-order fault", refsCase("T1", "T1", anchor, []string{"T2", "T1"}), 5, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, diags, err := ParseDocument(cat, "items", "items.flowmark.md", []byte(c.text), lk)
			if err != nil {
				t.Fatal(err)
			}
			if c.line == 0 {
				if len(diags) > 0 || rows == nil {
					t.Fatalf("diagnostics %+v; want none\n%s", diags, c.text)
				}
				return
			}
			if len(diags) != 1 || diags[0].Rule != RuleNoncanonical || diags[0].Line != c.line ||
				(c.col != 0 && diags[0].Column != c.col) || !strings.Contains(diags[0].Message, c.msg) {
				t.Fatalf("diagnostics %+v; want one %s at line %d column %d holding %q\n%s", diags, RuleNoncanonical, c.line, c.col, c.msg, c.text)
			}
		})
	}
}

// sharedStep is a steps row with no parent and no body.
func sharedStep(key string) *Row { return newRow("steps", key, "") }

// sharedItem is an items row with its position and the blocks given.
func sharedItem(key, position string, blocks map[string][]*Row) *Row {
	r := newRow("items", key, "Item "+key)
	r.Scalars["position"] = sp(position)
	r.Blocks = blocks
	return r
}

// TestSharedKeyLinkWrite requires that a write of a key two blocks share byte for
// byte is refused at the second row, and that a link to that key, which has no
// derived target and no target kept from a read, is refused at its column.
func TestSharedKeyLinkWrite(t *testing.T) {
	cat := loadSharedKeys(t)
	ref := newRow("refs", "R1", "")
	ref.Scalars["step"] = sp("S1")
	rows := []*Row{
		sharedItem("T1", "1", map[string][]*Row{"refs": {ref}, "steps": {sharedStep("S1")}}),
		sharedItem("T2", "2", map[string][]*Row{"steps": {sharedStep("S1")}}),
	}
	out, rf, err := RenderDocument(cat, "items", docPath("items"), rows, stubLinker{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Refusal{
		{Table: "steps", Key: "S1", Index: 1, Column: "id", Rule: RuleDuplicateKey},
		{Table: "refs", Key: "R1", Index: -1, Column: "step", Rule: RuleConstraint},
	}
	t.Run("10#foreign-keys-as-links: a link to a key two blocks share is refused at its column", func(t *testing.T) {
		if out != nil || len(rf) != len(want) {
			t.Fatalf("written %q with refusals %+v; want %+v", out, rf, want)
		}
		for k, w := range want {
			g := rf[k]
			if g.Table != w.Table || g.Key != w.Key || g.Index != w.Index || g.Column != w.Column || g.Rule != w.Rule {
				t.Fatalf("refusal %d is %+v; want %+v", k, g, w)
			}
		}
	})
}

// TestSharedKeyLinksAcrossFiles requires that a link from a block of a files row to
// a key that rows of the table in several files share byte for byte has no derived
// target although the file holds one of those rows: it is read without a target
// check and written as read, and a write that keeps no target is refused.
func TestSharedKeyLinksAcrossFiles(t *testing.T) {
	cat := loadSharedKeys(t)
	lk := sharedLinker{
		stubLinker: stubLinker{"tasks/T1": {file: "tasks/T1.flowmark.md"}, "tasks/T2": {file: "tasks/T2.flowmark.md"}},
		shared:     map[string]bool{"task_steps/S1": true},
	}
	file := func(target string) string {
		return raw("# Title", "", "## task_refs", "| id | step |", "| --- | --- |", "| R1 | [S1]("+target+") |", "", "## task_steps", "- S1 Draft")
	}
	for _, c := range []struct{ name, target string }{
		{"10#foreign-keys-as-links: a link to a key shared across files, written with another file's target", "T2.flowmark.md"},
		{"10#foreign-keys-as-links: a link to a key shared across files, written with its own file's target", "T1.flowmark.md"},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := file(c.target)
			row, diags := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", []byte(text), lk)
			if len(diags) > 0 || row == nil {
				t.Fatalf("diagnostics %+v; want none", diags)
			}
			out, rf := renderFileOf(t, cat, "tasks", row, lk)
			if len(rf) > 0 || !bytes.Equal(out, []byte(text)) {
				t.Fatalf("written %q with %+v; want the file as read %q", out, rf, text)
			}
		})
	}
	t.Run("10#foreign-keys-as-links: a write of a link to a key shared across files with no target kept is refused", func(t *testing.T) {
		ref := newRow("task_refs", "R1", "")
		ref.Scalars["step"] = sp("S1")
		task := newRow("tasks", "T1", "Title")
		task.Blocks = map[string][]*Row{"task_refs": {ref}, "task_steps": {newRow("task_steps", "S1", "Draft")}}
		out, rf := renderFileOf(t, cat, "tasks", task, lk)
		if out != nil || len(rf) != 1 || rf[0].Table != "task_refs" || rf[0].Key != "R1" || rf[0].Column != "step" || rf[0].Rule != RuleConstraint {
			t.Fatalf("written %q with %+v; want one CONSTRAINT at task_refs R1 step", out, rf)
		}
	})
}
