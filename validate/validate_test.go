package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

// catalogRoot is the corpus root whose dbspec/ set is the test catalog; the
// cases are corpora that hold data only.
const catalogRoot = "testdata/catalog"

func loadCatalog(t testing.TB) *catalog.Catalog {
	t.Helper()
	cat, diags := catalog.Load(catalogRoot)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("test catalog: %+v", diags)
	}
	return cat
}

// wanted is one expected diagnostic: its file and rule, its line and column when
// they are compared (0 when not), and a text its message holds ("" when not
// compared).
type wanted struct {
	file      string
	line, col int
	rule      string
	says      string
}

// parseWant reads a want entry "file[:line[:col]] rule[ ~ text]".
func parseWant(t *testing.T, w string) wanted {
	t.Helper()
	spec, says, _ := strings.Cut(w, " ~ ")
	loc, rule, ok := strings.Cut(spec, " ")
	if !ok {
		t.Fatalf("bad want %q", w)
	}
	parts := strings.Split(loc, ":")
	out := wanted{file: parts[0], rule: rule, says: says}
	nums := []*int{&out.line, &out.col}
	if len(parts) > 3 {
		t.Fatalf("bad want %q", w)
	}
	for i, p := range parts[1:] {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("bad want %q: %v", w, err)
		}
		*nums[i] = n
	}
	return out
}

func (w wanted) matches(d catalog.Diagnostic) bool {
	return d.File == w.file && d.Rule == w.rule &&
		(w.line == 0 || d.Line == w.line) && (w.col == 0 || d.Column == w.col) &&
		strings.Contains(d.Message, w.says)
}

// check requires that the diagnostics are exactly want: each want entry matches
// one diagnostic, and no diagnostic is left over.
func check(t *testing.T, diags []catalog.Diagnostic, want []string) {
	t.Helper()
	used := make([]bool, len(diags))
	var missing []string
	for _, ws := range want {
		w := parseWant(t, ws)
		found := false
		for i, d := range diags {
			if !used[i] && w.matches(d) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			missing = append(missing, ws)
		}
	}
	var extra []string
	for i, d := range diags {
		if !used[i] {
			extra = append(extra, fmt.Sprintf("%s:%d:%d %s: %s", d.File, d.Line, d.Column, d.Rule, d.Message))
		}
	}
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("diagnostics differ\nmissing: %q\nextra:   %q", missing, extra)
	}
}

// TestCorpusRules holds the accepting and rejecting cases of each rule whose
// corpus is under testdata/cases; want names the diagnostics it must report.
func TestCorpusRules(t *testing.T) {
	cat := loadCatalog(t)
	cases := []struct {
		name, dir string
		want      []string
	}{
		{name: "50#diagnostics: a clean corpus reports nothing", dir: "clean"},
		{name: "50#parse-and-type: a value not of its column type is type", dir: "type-reject",
			want: []string{"entries/E1.flowmark.md:5:3 type"}},
		{name: "50#keys-and-references: a dangling foreign key of a files row", dir: "dangling-reject",
			want: []string{"entries/E1.flowmark.md:2:3 reference.dangling"}},
		{name: "50#keys-and-references: a parent that names no row", dir: "dangling-parent-reject",
			want: []string{"entries/E1.flowmark.md:2:3 reference.dangling"}},
		{name: "50#keys-and-references: a dangling foreign key of a document row", dir: "dangling-document-reject",
			want: []string{"items.flowmark.md:2 reference.dangling"}},
		{name: "50#diagnostics: a dangling foreign key into an unavailable table is not reported", dir: "dangling-into-blocked",
			want: []string{"plans/_bad.flowmark.md:1 format.block"}},
		{name: "50#diagnostics: a dangling foreign key from a readable row of an unavailable table is reported", dir: "dangling-blocked-source",
			want: []string{
				"entries/_bad.flowmark.md:1 format.block",
				"entries/E1.flowmark.md:2 reference.dangling",
			}},
		{name: "50#key-rules: keys equal without regard to case in a document table", dir: "key-dup-document-reject",
			want: []string{"items.flowmark.md:3 key.duplicate ~ the key t1 equals the key T1 of items.flowmark.md without regard to case"}},
		{name: "50#key-rules: keys equal byte for byte in a document table", dir: "key-dup-byte-reject",
			want: []string{"items.flowmark.md:3 key.duplicate ~ the key T1 equals the key T1 of items.flowmark.md byte for byte"}},
		{name: "20#hierarchy: a cycle through two files is the validator's parent.cycle at each", dir: "parent-cycle-reject",
			want: []string{
				"entries/A.flowmark.md:2:3 parent.cycle",
				"entries/B.flowmark.md:2:3 parent.cycle",
			}},
		{name: "50#hierarchy-and-order: a gap between ranks is no fault", dir: "order-gap"},
		{name: "50#hierarchy-and-order: a value shared with a sibling", dir: "order-shared-reject",
			want: []string{
				"entries/E1.flowmark.md:3:3 order",
				"entries/E2.flowmark.md:3:3 order",
			}},
		{name: "50#diagnostics: no order in an unavailable table", dir: "order-blocked",
			want: []string{"entries/_bad.flowmark.md:1 format.block"}},
		{name: "20#corpus-documents-and-the-walk: a document outside every place", dir: "walk-root-doc",
			want: []string{"notes.flowmark.md:1 corpus.walk"}},
		{name: "20#corpus-documents-and-the-walk: a case variant of a document table", dir: "walk-case-doc",
			want: []string{"items.FLOWMARK.md:1 corpus.walk"}},
		{name: "20#corpus-documents-and-the-walk: a case variant of a files table folder", dir: "walk-case-folder",
			want: []string{"Plans:1 corpus.walk"}},
		{name: "10#diagnostics: a format diagnostic passes through", dir: "noncanonical",
			want: []string{"plans/P1.flowmark.md format.noncanonical"}},
		{name: "50#a-state-changed-outside-the-engine: a non-initial state without history", dir: "history-gap-no-history",
			want: []string{"items.flowmark.md:2 history.gap"}},
		{name: "50#a-state-changed-outside-the-engine: a state that differs from the last to", dir: "history-gap-last-to",
			want: []string{"items.flowmark.md:2 history.gap"}},
		{name: "50#a-state-changed-outside-the-engine: a first from that is not initial", dir: "history-chain-first",
			want: []string{"items.flowmark.md:9 history.chain"}},
		{name: "50#a-state-changed-outside-the-engine: a from that differs from the previous to and a pair not declared are two faults", dir: "history-chain-prev",
			want: []string{
				"items.flowmark.md:10 history.chain ~ differs from the to doing",
				"items.flowmark.md:10 state.transition ~ from waiting to done",
			}},
		{name: "50#a-state-changed-outside-the-engine: a pair that is not a declared transition is state.transition", dir: "history-chain-pair",
			want: []string{"items.flowmark.md:9 state.transition ~ from waiting to done, which is not a declared transition"}},
		{name: "50#diagnostics: a document with an unreadable row gives no rows to the history rules", dir: "history-under-blocked",
			want: []string{"items.flowmark.md format.block"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags, err := Corpus(cat, filepath.Join("testdata", "cases", tc.dir))
			if err != nil {
				t.Fatal(err)
			}
			check(t, diags, tc.want)
		})
	}
}

// planOne is a plans row that the entries of the cases name.
const planOne = "# Plan one\n\nPlan intro.\n"

// entry returns an entries row: its title, its parent ("" for none) and its
// position, then the lines that follow its field list.
func entry(title, parent, position string, after ...string) string {
	var b strings.Builder
	b.WriteString("# " + title + "\n")
	if parent != "" {
		b.WriteString("- parent: [" + parent + "](" + parent + ".flowmark.md)\n")
	}
	b.WriteString("- plan: [P1](../plans/P1.flowmark.md)\n- position: " + position + "\n- kind: alpha\n- done: true\n")
	for _, line := range after {
		b.WriteString(line + "\n")
	}
	return b.String()
}

// steps returns the entry_steps block of an entries row, one line per step:
// "key parent state", "-" being no parent.
func steps(rows ...string) []string {
	out := []string{"", "## entry_steps", "| id | parent | state |", "| --- | --- | --- |"}
	for _, r := range rows {
		f := strings.Fields(r)
		parent := ""
		if f[1] != "-" {
			parent = "[" + f[1] + "](" + f[3] + ")"
		}
		out = append(out, "| "+f[0]+" | "+parent+" | "+f[2]+" |")
	}
	return out
}

// tickets returns the tickets document, one line per row: "key parent state",
// "-" being no parent.
func tickets(rows ...string) string {
	var b strings.Builder
	b.WriteString("# tickets\n| id | parent | state | title |\n| --- | --- | --- | --- |\n")
	for _, r := range rows {
		f := strings.Fields(r)
		parent := ""
		if f[1] != "-" {
			parent = "[" + f[1] + "](tickets.flowmark.md)"
		}
		b.WriteString("| " + f[0] + " | " + parent + " | " + f[2] + " | Ticket " + f[0] + " |\n")
	}
	return b.String()
}

// doingItems returns the items document with the doing rows keys, each with the
// history row that brought it from waiting to doing.
func doingItems(keys ...string) string {
	var b strings.Builder
	b.WriteString("# items\n")
	for _, k := range keys {
		b.WriteString("- [~] " + k + " Item " + k + "\n")
	}
	for i, k := range keys {
		b.WriteString("\n## items row " + k + "\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n")
		b.WriteString("| " + strconv.Itoa(i+1) + " | waiting | doing | 2026-10-01 08:00:00.000000 |\n")
	}
	return b.String()
}

// card returns a cards row in the state open at position, then its other fields.
func card(title, position string, fields ...string) string {
	return "# " + title + "\n- state: open\n- position: " + position + "\n" + strings.Join(append(fields, ""), "\n")
}

// measure returns a measures row with its title, then its field items.
func measure(title string, fields ...string) string {
	return "# " + title + "\n" + strings.Join(append(fields, ""), "\n")
}

// tasks returns the tasks document with the one row key in the state of glyph,
// whose task_history block holds the table rows, then any further lines of the
// row's scope.
func tasks(key, glyph string, lines ...string) string {
	var b strings.Builder
	b.WriteString("# tasks\n- [" + glyph + "] " + key + " Task " + key + "\n\n## tasks row " + key + "\n\n")
	b.WriteString("### task_history\n| id | from_state | to_state | at | owner |\n| --- | --- | --- | --- | --- |\n")
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	return b.String()
}

// longKey is a key of 40 characters, longer than the varchar(16) of a parent.
const longKey = "ABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJKLMN"

// TestValidatorRules holds the accepting and rejecting cases of the validator
// rules whose corpus is written at run time.
func TestValidatorRules(t *testing.T) {
	cat := loadCatalog(t)
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		// state.limit: the rows past the limit, in corpus order, at their state.
		{name: "50#the-state-machine: a table at its limit is accepted",
			files: map[string]string{"tickets.flowmark.md": tickets("K1 - doing", "K2 - open")}},
		{name: "50#the-state-machine: two rows over the limit in a document are each reported at its state cell",
			files: map[string]string{"tickets.flowmark.md": tickets("K1 - doing", "K2 - doing", "K3 - doing")},
			want: []string{
				"tickets.flowmark.md:5:11 state.limit ~ limit 1 of the state doing, which 3 rows",
				"tickets.flowmark.md:6:11 state.limit ~ limit 1 of the state doing, which 3 rows",
			}},
		{name: "50#the-state-machine: a list row over the limit is reported at its glyph",
			files: map[string]string{"items.flowmark.md": doingItems("T1", "T2", "T3")},
			want: []string{
				"items.flowmark.md:3:4 state.limit",
				"items.flowmark.md:4:4 state.limit",
			}},
		{name: "50#the-state-machine: an i64-keyed files table is in corpus order, so 9 follows 10",
			files: map[string]string{
				"jobs/9.flowmark.md":  "# Nine\n- state: doing\n",
				"jobs/10.flowmark.md": "# Ten\n- state: doing\n",
			},
			want: []string{"jobs/9.flowmark.md:2:3 state.limit"}},
		{name: "50#the-state-machine: a files table with an order column is in corpus order, not in sibling order",
			files: map[string]string{
				"cards/A.flowmark.md": "# A\n- state: doing\n- position: 2\n",
				"cards/B.flowmark.md": "# B\n- state: doing\n- position: 1\n",
			},
			want: []string{"cards/B.flowmark.md:2:3 state.limit"}},
		{name: "50#the-state-machine: a document out of storage order is in corpus order, so the later line is past the limit",
			files: map[string]string{"tickets.flowmark.md": tickets("K2 - doing", "K1 - doing")},
			want: []string{
				"tickets.flowmark.md:5:11 state.limit ~ K1 is past",
				"tickets.flowmark.md format.noncanonical",
			}},
		{name: "50#the-state-machine: a block-table machine across two parent files",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("First", "", "2", steps("S1 - doing")...),
				"entries/E2.flowmark.md": entry("Second", "", "1", steps("S2 - doing")...),
			},
			want: []string{"entries/E2.flowmark.md:10 state.limit"}},
		{name: "50#diagnostics: no state.limit in an unavailable table",
			files: map[string]string{
				"plans/P1.flowmark.md":     planOne,
				"entries/E1.flowmark.md":   entry("First", "", "2", steps("S1 - doing")...),
				"entries/E2.flowmark.md":   entry("Second", "", "1", steps("S2 - doing")...),
				"entries/_bad.flowmark.md": "# Bad key\n",
			},
			want: []string{"entries/_bad.flowmark.md:1 format.block"}},
		{name: "50#diagnostics: a table unavailable only through byte-equal keys has state.limit",
			files: map[string]string{"tickets.flowmark.md": tickets("K1 - doing", "K1 - doing")},
			want: []string{
				"tickets.flowmark.md:5:3 key.duplicate",
				"tickets.flowmark.md:5:11 state.limit",
			}},

		// order: a rank orders its siblings; only a shared rank is an order fault, and
		// a rank not of its type is type alone.
		{name: "50#hierarchy-and-order: orders 1, 2, 3 are accepted",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("One", "", "1"),
				"entries/E2.flowmark.md": entry("Two", "", "2"),
				"entries/E3.flowmark.md": entry("Three", "", "3"),
			}},
		{name: "50#hierarchy-and-order: orders 1, 2, x are type alone",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("One", "", "1"),
				"entries/E2.flowmark.md": entry("Two", "", "2"),
				"entries/E3.flowmark.md": entry("Three", "", "x"),
			},
			want: []string{"entries/E3.flowmark.md:3:3 type"}},
		{name: "50#hierarchy-and-order: orders 1, 1, x are order at both 1s and type at x",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("One", "", "1"),
				"entries/E2.flowmark.md": entry("Two", "", "1"),
				"entries/E3.flowmark.md": entry("Three", "", "x"),
			},
			want: []string{
				"entries/E1.flowmark.md:3:3 order",
				"entries/E2.flowmark.md:3:3 order",
				"entries/E3.flowmark.md:3:3 type",
			}},
		{name: "50#hierarchy-and-order: ranks -5, 4000, x are type alone at x",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("One", "", "-5"),
				"entries/E2.flowmark.md": entry("Two", "", "4000"),
				"entries/E3.flowmark.md": entry("Three", "", "x"),
			},
			want: []string{"entries/E3.flowmark.md:3:3 type"}},
		{name: "50#hierarchy-and-order: ranks 1000, 1000 are order at both, naming the rank",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("One", "", "1000"),
				"entries/E2.flowmark.md": entry("Two", "", "1000"),
			},
			want: []string{
				"entries/E1.flowmark.md:3:3 order ~ rank 1000",
				"entries/E2.flowmark.md:3:3 order ~ rank 1000",
			}},
		{name: "50#hierarchy-and-order: an i32 overflow is type alone",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("One", "", "1"),
				"entries/E2.flowmark.md": entry("Two", "", "2"),
				"entries/E3.flowmark.md": entry("Three", "", "3000000000"),
			},
			want: []string{"entries/E3.flowmark.md:3:3 type"}},

		// order: a parent not of its type belongs to no known sibling group.
		{name: "50#hierarchy-and-order: a row with an unreadable parent is type alone",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("One", "", "1"),
				"entries/E2.flowmark.md": entry("Two", "", "2"),
				"entries/E3.flowmark.md": entry("Three", longKey, "1"),
			},
			want: []string{"entries/E3.flowmark.md:2:3 type"}},
		{name: "50#hierarchy-and-order: a rank an unreadable parent's row shares is no order fault",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("One", "", "1"),
				"entries/E2.flowmark.md": entry("Two", "", "3"),
				"entries/E3.flowmark.md": entry("Three", longKey, "1"),
			},
			want: []string{"entries/E3.flowmark.md:2:3 type"}},
		{name: "50#hierarchy-and-order: a value shared within the group stands whatever an unreadable parent joins",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("One", "", "1"),
				"entries/E2.flowmark.md": entry("Two", "", "1"),
				"entries/E3.flowmark.md": entry("Three", longKey, "2"),
			},
			want: []string{
				"entries/E1.flowmark.md:3:3 order",
				"entries/E2.flowmark.md:3:3 order",
				"entries/E3.flowmark.md:2:3 type",
			}},

		// parent.cycle across files, in an unavailable table too.
		{name: "50#diagnostics: a cross-file cycle in an unavailable table is reported among the rows read",
			files: map[string]string{
				"plans/P1.flowmark.md":     planOne,
				"entries/A.flowmark.md":    entry("A", "B", "1"),
				"entries/B.flowmark.md":    entry("B", "A", "1"),
				"entries/_bad.flowmark.md": "# Bad key\n",
			},
			want: []string{
				"entries/A.flowmark.md:2:3 parent.cycle",
				"entries/B.flowmark.md:2:3 parent.cycle",
				"entries/_bad.flowmark.md:1 format.block",
			}},
		{name: "50#diagnostics: a cycle through an unreadable file has its diagnostic alone",
			files: map[string]string{
				"plans/P1.flowmark.md":  planOne,
				"entries/A.flowmark.md": entry("A", "B", "1"),
				"entries/B.flowmark.md": entry("B", "C", "1"),
				"entries/C.flowmark.md": "# C\n- parent: [A](A.flowmark.md)\n- plan: [P1](../plans/P1.flowmark.md)\n- position: 1\n- kind: alpha\n",
			},
			want: []string{"entries/C.flowmark.md format.block"}},
		{name: "20#hierarchy: a cycle in a document table is the read parent.cycle alone",
			files: map[string]string{"tickets.flowmark.md": tickets("K1 K2 open", "K2 K1 open")},
			want:  []string{"tickets.flowmark.md parent.cycle"}},
		{name: "20#hierarchy: a cycle in one block is the read parent.cycle alone",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("First", "", "1", steps("S1 S2 open E1.flowmark.md", "S2 S1 open E1.flowmark.md")...),
			},
			want: []string{"entries/E1.flowmark.md parent.cycle"}},

		// key.duplicate over every block of every file, in corpus order.
		{name: "50#keys-and-references: byte-equal keys in two blocks of one file",
			files: map[string]string{"items.flowmark.md": "# items\n- [x] T1 One\n- [x] T2 Two\n" +
				"\n## items row T1\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |\n| 2 | doing | done | 2026-10-01 09:00:00.000000 |\n" +
				"\n## items row T2\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 2 | waiting | doing | 2026-10-01 08:00:00.000000 |\n| 3 | doing | done | 2026-10-01 09:00:00.000000 |\n"},
			want: []string{"items.flowmark.md:18:3 key.duplicate ~ items.flowmark.md"}},
		{name: "50#keys-and-references: keys equal without regard to case in blocks of two files are reported at the later path",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("First", "", "2", steps("s1 - open")...),
				"entries/E2.flowmark.md": entry("Second", "", "1", steps("S1 - open")...),
			},
			want: []string{"entries/E2.flowmark.md:10:3 key.duplicate ~ of entries/E1.flowmark.md"}},
		{name: "50#keys-and-references: the duplicate in a document is the later line, whatever the storage order",
			files: map[string]string{"tickets.flowmark.md": tickets("k1 - open", "K1 - open")},
			want: []string{
				"tickets.flowmark.md:5:3 key.duplicate ~ the key K1 equals the key k1",
				"tickets.flowmark.md format.noncanonical",
			}},
		{name: "50#parse-and-type: a foreign key whose key is not of its column type is type alone",
			files: map[string]string{
				"plans/P1.flowmark.md": planOne,
				"items.flowmark.md":    "# items\n- [ ] T1 First item plan:[" + longKey + "](plans/" + longKey + ".flowmark.md)\n",
			},
			want: []string{"items.flowmark.md:2 type"}},
		// key.duplicate of a unique key: byte for byte, in corpus order, NULLs and
		// values not of their type taking no part.
		{name: "50#keys-and-references: distinct unique values are accepted",
			files: map[string]string{
				"cards/A.flowmark.md": card("A", "1", "- code: x"),
				"cards/B.flowmark.md": card("B", "2", "- code: y"),
			}},
		{name: "50#keys-and-references: a duplicate unique value is reported at the later path, naming the first",
			files: map[string]string{
				"cards/A.flowmark.md": card("A", "1", "- code: x"),
				"cards/B.flowmark.md": card("B", "2", "- code: x"),
			},
			want: []string{"cards/B.flowmark.md:4:3 key.duplicate ~ (code) of B equals that of A in cards/A.flowmark.md"}},
		{name: "50#keys-and-references: unique values that differ in case only are distinct",
			files: map[string]string{
				"cards/A.flowmark.md": card("A", "1", "- code: x"),
				"cards/B.flowmark.md": card("B", "2", "- code: X"),
			}},
		{name: "50#keys-and-references: two NULL unique values are accepted",
			files: map[string]string{
				"cards/A.flowmark.md": card("A", "1"),
				"cards/B.flowmark.md": card("B", "2"),
			}},
		{name: "50#keys-and-references: unique values not of their type are type alone",
			files: map[string]string{
				"cards/A.flowmark.md": card("A", "1", "- code: 123456789"),
				"cards/B.flowmark.md": card("B", "2", "- code: 123456789"),
			},
			want: []string{
				"cards/A.flowmark.md:4:3 type",
				"cards/B.flowmark.md:4:3 type",
			}},
		{name: "50#keys-and-references: a composite unique key is reported at its first column",
			files: map[string]string{
				"cards/A.flowmark.md": card("A", "1", "- lane: l1", "- slot: 1"),
				"cards/B.flowmark.md": card("B", "2", "- lane: l1", "- slot: 2"),
				"cards/C.flowmark.md": card("C", "3", "- lane: l1", "- slot: 1"),
			},
			want: []string{"cards/C.flowmark.md:4:3 key.duplicate ~ (lane, slot) of C equals that of A"}},
		{name: "50#diagnostics: no unique key.duplicate in an unavailable table",
			files: map[string]string{
				"cards/A.flowmark.md":    card("A", "1", "- code: x"),
				"cards/B.flowmark.md":    card("B", "2", "- code: x"),
				"cards/_bad.flowmark.md": "# Bad key\n",
			},
			want: []string{"cards/_bad.flowmark.md:1 format.block"}},
		{name: "10#foreign-keys-as-links: a link to a key that rows of two files share has no derived target",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("First", "", "1", steps("S1 - open")...),
				"entries/E2.flowmark.md": entry("Second", "", "2", steps("S1 - open")...),
				"jobs/1.flowmark.md":     "# One\n- state: open\n- step: [S1](../entries/E2.flowmark.md)\n",
			},
			want: []string{"entries/E2.flowmark.md:10:3 key.duplicate"}},
		{name: "10#foreign-keys-as-links: a link to a key one row holds has its derived target",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("First", "", "1", steps("S1 - open")...),
				"jobs/1.flowmark.md":     "# One\n- state: open\n- step: [S1](../entries/E1.flowmark.md)\n",
			}},
		{name: "50#keys-and-references: distinct keys across blocks are accepted",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("First", "", "2", steps("S1 - open")...),
				"entries/E2.flowmark.md": entry("Second", "", "1", steps("S2 - open")...),
			}},
		{name: "50#keys-and-references: a unique value and a NULL are distinct",
			files: map[string]string{
				"cards/A.flowmark.md": card("A", "1", "- code: ab"),
				"cards/B.flowmark.md": card("B", "2"),
			}},
		{name: "50#keys-and-references: unique values ab and AB are distinct, as the binary collation compares them",
			files: map[string]string{
				"cards/A.flowmark.md": card("A", "1", "- code: ab"),
				"cards/B.flowmark.md": card("B", "2", "- code: AB"),
			}},
		{name: "50#keys-and-references: unique values ab and ab are a duplicate",
			files: map[string]string{
				"cards/A.flowmark.md": card("A", "1", "- code: ab"),
				"cards/B.flowmark.md": card("B", "2", "- code: ab"),
			},
			want: []string{"cards/B.flowmark.md:4:3 key.duplicate ~ (code) of B equals that of A"}},

		// key.duplicate of the primary key: byte for byte, else without regard to case.
		{name: "50#key-rules: a byte-equal key is a duplicate byte for byte",
			files: map[string]string{"tickets.flowmark.md": tickets("T1 - open", "T1 - open")},
			want:  []string{"tickets.flowmark.md:5:3 key.duplicate ~ the key T1 equals the key T1 of tickets.flowmark.md byte for byte"}},
		{name: "50#key-rules: a key equal in case only is a duplicate without regard to case",
			files: map[string]string{"tickets.flowmark.md": tickets("T1 - open", "t1 - open")},
			want:  []string{"tickets.flowmark.md:5:3 key.duplicate ~ the key t1 equals the key T1 of tickets.flowmark.md without regard to case"}},
		{name: "50#key-rules: each later key names the first row it equals, byte for byte before without regard to case",
			files: map[string]string{"tickets.flowmark.md": tickets("t1 - open", "T1 - open", "T1 - open")},
			want: []string{
				"tickets.flowmark.md:5:3 key.duplicate ~ the key T1 equals the key t1 of tickets.flowmark.md without regard to case",
				"tickets.flowmark.md:6:3 key.duplicate ~ the key T1 equals the key T1 of tickets.flowmark.md byte for byte",
				"tickets.flowmark.md format.noncanonical",
			}},

		// check: false over the row's logical values; unknown passes.
		{name: "50#checks: a row whose checks hold is accepted",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1", "- low: 9", "- high: 10", "- price: 10.00", "- ratio: 0.5",
				"- starts: 2026-10-01 23:59:59", "- ends: 2026-10-02 00:00:00", "- label: B", "- code: b")}},
		{name: "50#checks: a row of NULLs leaves every check unknown, so it passes",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1")}},
		{name: "50#checks: integers compare by value",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1", "- low: 10", "- high: 9")},
			want:  []string{"measures/M1.flowmark.md:1:1 check ~ the check ck_measures_range is false for M1"}},
		{name: "50#checks: decimals compare by value",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1", "- price: 1.99")},
			want:  []string{"measures/M1.flowmark.md:1:1 check ~ ck_measures_price"}},
		{name: "50#checks: f64 values compare by value",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1", "- ratio: 10")},
			want:  []string{"measures/M1.flowmark.md:1:1 check ~ ck_measures_ratio"}},
		{name: "50#checks: datetimes compare chronologically",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1", "- starts: 2026-10-02 00:00:00", "- ends: 2026-10-01 23:59:59")},
			want:  []string{"measures/M1.flowmark.md:1:1 check ~ ck_measures_span"}},
		{name: "50#checks: text compares by code point",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1", "- code: B")},
			want:  []string{"measures/M1.flowmark.md:1:1 check ~ ck_measures_code"}},
		{name: "50#checks: two false checks are two reports",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1", "- low: 10", "- high: 9", "- label: c")},
			want: []string{
				"measures/M1.flowmark.md:1:1 check ~ ck_measures_range",
				"measures/M1.flowmark.md:1:1 check ~ ck_measures_label",
			}},
		{name: "50#checks: a check false whatever a raw value is stands beside its type",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1", "- low: x", "- high: -1")},
			want: []string{
				"measures/M1.flowmark.md:2:3 type",
				"measures/M1.flowmark.md:1:1 check ~ ck_measures_positive",
			}},
		{name: "50#checks: a check that a raw value decides is type alone",
			files: map[string]string{"measures/M1.flowmark.md": measure("M1", "- low: x", "- high: 1", "- label: toolong")},
			want: []string{
				"measures/M1.flowmark.md:2:3 type",
				"measures/M1.flowmark.md:4:3 type",
			}},
		{name: "50#checks: a check is reported in an unavailable table",
			files: map[string]string{
				"measures/M1.flowmark.md":   measure("M1", "- low: 10", "- high: 9"),
				"measures/_bad.flowmark.md": "# Bad key\n",
			},
			want: []string{
				"measures/M1.flowmark.md:1:1 check ~ ck_measures_range",
				"measures/_bad.flowmark.md:1 format.block",
			}},
		{name: "50#checks: a check reads the placement foreign key as the key of the parent row",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"entries/E1.flowmark.md": entry("First", "", "1", steps("S1 - open")...),
				"entries/E9.flowmark.md": entry("Ninth", "", "2", steps("S9 - open")...),
			},
			want: []string{"entries/E9.flowmark.md:10 check ~ the check ck_steps_entry is false for S9"}},

		// history.chain, state.transition and state.required over the history rows.
		{name: "50#a-state-changed-outside-the-engine: a history whose pairs record their required columns is accepted",
			files: map[string]string{"tasks.flowmark.md": tasks("T1", "x", "| 1 | waiting | doing | 2026-10-01 08:00:00.000000 | ann |",
				"| 2 | doing | done | 2026-10-01 09:00:00.000000 |  |", "", "#### task_history row 2", "", "##### evidence", "", "Proof.")}},
		{name: "50#a-state-changed-outside-the-engine: a NULL required cell, which holds no element, is state.required at the row",
			files: map[string]string{"tasks.flowmark.md": tasks("T1", "~", "| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |  |")},
			want:  []string{"tasks.flowmark.md:9:1 state.required ~ requires owner, and owner holds no value"}},
		{name: "50#a-state-changed-outside-the-engine: an empty required cell is state.required at the cell",
			files: map[string]string{"tasks.flowmark.md": tasks("T1", "~", `| 1 | waiting | doing | 2026-10-01 08:00:00.000000 | "" |`)},
			want:  []string{"tasks.flowmark.md:9:54 state.required ~ owner"}},
		{name: "50#a-state-changed-outside-the-engine: an absent required section of the terminal is state.required at the row",
			files: map[string]string{"tasks.flowmark.md": tasks("T1", "x", "| 1 | waiting | doing | 2026-10-01 08:00:00.000000 | ann |",
				"| 2 | doing | done | 2026-10-01 09:00:00.000000 |  |")},
			want: []string{"tasks.flowmark.md:10:1 state.required ~ requires evidence"}},
		{name: "50#a-state-changed-outside-the-engine: a present empty required section is state.required at the section",
			files: map[string]string{"tasks.flowmark.md": tasks("T1", "x", "| 1 | waiting | doing | 2026-10-01 08:00:00.000000 | ann |",
				"| 2 | doing | done | 2026-10-01 09:00:00.000000 |  |", "", "#### task_history row 2", "", "##### evidence")},
			want: []string{"tasks.flowmark.md:14:7 state.required ~ requires evidence"}},
		{name: "50#a-state-changed-outside-the-engine: a required value not of its type is type alone",
			files: map[string]string{"tasks.flowmark.md": tasks("T1", "~", "| 1 | waiting | doing | 2026-10-01 08:00:00.000000 | abcdefghijklmnopqrs |")},
			want:  []string{"tasks.flowmark.md:9 type"}},
		{name: "50#a-state-changed-outside-the-engine: an undeclared pair is state.transition, with no state.required",
			files: map[string]string{"tasks.flowmark.md": tasks("T1", "x", "| 1 | waiting | done | 2026-10-01 08:00:00.000000 |  |")},
			want:  []string{"tasks.flowmark.md:9 state.transition ~ from waiting to done"}},
		{name: "50#a-state-changed-outside-the-engine: a to not of its type is type alone",
			files: map[string]string{"items.flowmark.md": "# items\n- [x] T1 First item\n\n## items row T1\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | waiting | abcdefghijklmnopqrs | 2026-10-01 08:00:00.000000 |\n| 2 | doing | done | 2026-10-01 09:00:00.000000 |\n"},
			want: []string{"items.flowmark.md:9 type"}},
		{name: "50#a-state-changed-outside-the-engine: a to not of its type is type alone, and its own from is still judged",
			files: map[string]string{"items.flowmark.md": "# items\n- [x] T1 First item\n\n## items row T1\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | doing | abcdefghijklmnopqrs | 2026-10-01 08:00:00.000000 |\n"},
			want: []string{
				"items.flowmark.md:9 history.chain ~ from doing",
				"items.flowmark.md:9 type",
			}},
		{name: "50#a-state-changed-outside-the-engine: a later row whose to is not of its type still has its from judged against the previous to",
			files: map[string]string{"items.flowmark.md": "# items\n- [x] T1 First item\n\n## items row T1\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |\n| 2 | waiting | abcdefghijklmnopqrs | 2026-10-01 09:00:00.000000 |\n"},
			want: []string{
				"items.flowmark.md:10 history.chain ~ from waiting, which differs from the to doing",
				"items.flowmark.md:10 type",
			}},
		{name: "50#a-state-changed-outside-the-engine: a last to not of its type leaves no gap to judge",
			files: map[string]string{"items.flowmark.md": "# items\n- [x] T1 First item\n\n## items row T1\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | waiting | abcdefghijklmnopqrs | 2026-10-01 08:00:00.000000 |\n"},
			want: []string{"items.flowmark.md:9 type"}},
		{name: "50#a-state-changed-outside-the-engine: a from not of its type is type alone",
			files: map[string]string{"items.flowmark.md": "# items\n- [x] T1 First item\n\n## items row T1\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | abcdefghijklmnopqrs | done | 2026-10-01 08:00:00.000000 |\n"},
			want: []string{"items.flowmark.md:9 type"}},
		{name: "50#a-state-changed-outside-the-engine: a state not of its type is type alone, and its history rows are still judged",
			files: map[string]string{"jobs/1.flowmark.md": "# One\n- state: abcdefghijklmnopqrs\n\n## job_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | open | open | 2026-10-01 08:00:00.000000 |\n"},
			want: []string{
				"jobs/1.flowmark.md:2:3 type",
				"jobs/1.flowmark.md:7 state.transition ~ from open to open",
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, tc.files)
			diags, err := Corpus(cat, root)
			if err != nil {
				t.Fatal(err)
			}
			check(t, diags, tc.want)
		})
	}
}

// caseInsensitive reports whether the file system of dir folds case: then two
// names that differ only in case are one file.
func caseInsensitive(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(probe)
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

// writeTree writes files under root, creating their folders.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// caseSensitiveEnv names the folder, on a case-sensitive file system, where a test
// writes a corpus whose names differ only in case. It is read only when the test's
// temporary folder folds case; it must then be set and absolute.
const caseSensitiveEnv = "FLOWMARK_CASE_SENSITIVE_DIR"

// caseSensitiveRoot returns an empty folder on a case-sensitive file system: the
// test's temporary folder when its file system keeps case, and otherwise a new
// folder under the one caseSensitiveEnv declares. It fails the test when neither
// keeps case.
func caseSensitiveRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if !caseInsensitive(t, root) {
		return root
	}
	dir := os.Getenv(caseSensitiveEnv)
	if dir == "" {
		t.Fatalf("the file system of %s folds case, so two names that differ only in case cannot be written; set %s to an absolute folder on a case-sensitive file system", root, caseSensitiveEnv)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("%s=%s is not an absolute path", caseSensitiveEnv, dir)
	}
	root, err := os.MkdirTemp(dir, "flowmark-case-")
	if err != nil {
		t.Fatalf("%s=%s: %v", caseSensitiveEnv, dir, err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	if caseInsensitive(t, root) {
		t.Fatalf("%s=%s folds case", caseSensitiveEnv, dir)
	}
	return root
}

// TestFilesCaseDuplicate covers the case-only collision of two files rows. Two
// names that differ only in case cannot coexist on a case-folding file system, so
// the corpus is written at run time on a case-sensitive one (caseSensitiveRoot).
func TestFilesCaseDuplicate(t *testing.T) {
	cat := loadCatalog(t)
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "50#key-rules: keys of two files equal without regard to case",
			files: map[string]string{"plans/P1.flowmark.md": planOne, "plans/p1.flowmark.md": planOne},
			want:  []string{"plans/p1.flowmark.md:1:1 key.duplicate ~ of plans/P1.flowmark.md"},
		},
		{
			name: "50#diagnostics: an unavailable table is not checked for key.duplicate",
			files: map[string]string{
				"plans/P1.flowmark.md":   planOne,
				"plans/p1.flowmark.md":   planOne,
				"plans/_bad.flowmark.md": "# Bad key\n",
			},
			want: []string{"plans/_bad.flowmark.md:1 format.block"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := caseSensitiveRoot(t)
			writeTree(t, root, tc.files)
			diags, err := Corpus(cat, root)
			if err != nil {
				t.Fatal(err)
			}
			check(t, diags, tc.want)
		})
	}
}

// TestCheck pins the comparison helper: a want with a line and a column must
// match both, and a want with a text must match the message.
func TestCheck(t *testing.T) {
	d := catalog.Diagnostic{File: "a.flowmark.md", Line: 2, Column: 3, Rule: "order", Message: "the order of A"}
	for _, w := range []string{"a.flowmark.md order", "a.flowmark.md:2 order", "a.flowmark.md:2:3 order", "a.flowmark.md:2:3 order ~ of A"} {
		if !parseWant(t, w).matches(d) {
			t.Errorf("%q does not match %+v", w, d)
		}
	}
	for _, w := range []string{"a.flowmark.md:3 order", "a.flowmark.md:2:1 order", "a.flowmark.md:2:3 order ~ of B", "b.flowmark.md order"} {
		if parseWant(t, w).matches(d) {
			t.Errorf("%q matches %+v", w, d)
		}
	}
}
