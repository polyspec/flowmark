package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

const documentRoot = "testdata/document"

func loadDocumentFixture(t testing.TB) *catalog.Catalog {
	t.Helper()
	cat, diags := catalog.Load(documentRoot)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("document fixture catalog: %+v", diags)
	}
	return cat
}

// documentLinker holds the rows of the people table, each with a detail section,
// so that every link to them carries its anchor.
func documentLinker() stubLinker {
	return stubLinker{
		"people/P1": {file: "people.flowmark.md", anchor: "people-row-p1"},
		"people/P2": {file: "people.flowmark.md", anchor: "people-row-p2"},
		"people/P3": {file: "people.flowmark.md", anchor: "people-row-p3"},
	}
}

func readDocumentGolden(t testing.TB, table string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(documentRoot, table+".flowmark.md"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// itemsOf is the document file of items with the lines after its H1.
func itemsOf(lines ...string) string { return raw(append([]string{"# items"}, lines...)...) }

// peopleOf is the document file of people with the lines after its H1.
func peopleOf(lines ...string) string { return raw(append([]string{"# people"}, lines...)...) }

// milestonesOf is the document file of milestones with the lines after its H1.
func milestonesOf(lines ...string) string { return raw(append([]string{"# milestones"}, lines...)...) }

// row joins the cells of a table row, as the writer spells it.
func row(cells ...string) string { return "| " + strings.Join(cells, " | ") + " |" }

const (
	peopleHeader = "| id | parent | name | role | active | born | seen | clock | ref | blob | weight | rank | big | score |"
	peopleDelim  = "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
)

// tableOfH1 is the table that a document names in its H1 line, items when the
// document has none: the rule cases that name no table are the items cases.
func tableOfH1(text string) string {
	for _, table := range []string{"people", "milestones", "notes"} {
		if strings.HasPrefix(text, "# "+table+"\n") {
			return table
		}
	}
	return "items"
}

// docCase is one input of the document grammar. rule is the rule of the first
// diagnostic, empty when the input is accepted; line and col locate it, 0 when
// not checked; kept says whether rows are returned along with the diagnostics.
type docCase struct {
	name  string
	table string // "" is items
	text  string
	rule  string
	line  int
	col   int
	kept  bool
}

func docAccept(name, text string) docCase { return docCase{name: name, text: text} }

func docReject(name, rule string, line int, text string) docCase {
	return docCase{name: name, rule: rule, line: line, text: text}
}

func noncanonicalDoc(name string, line int, text string) docCase {
	return docCase{name: name, rule: RuleNoncanonical, line: line, text: text, kept: true}
}

func documentParseCases() []docCase {
	return []docCase{
		// The document file: the H1, the list or table directly under it, one LF at the end.
		docAccept("10#document-order: a list of rows under the H1", itemsOf("- [ ] T1 Plan done:false")),
		docAccept("10#document-order: an empty list is the H1 and one LF", itemsOf()),
		docAccept("10#document-order: an empty table is the H1 and one LF", milestonesOf()),
		docAccept("10#document-order: a table of rows under the H1", milestonesOf(
			"| id | parent | title |", "| --- | --- | --- |", row("M1", "", "Foundation"))),
		noncanonicalDoc("20#document-model: a document file without a final line feed", 2, "# items\n- [ ] T1 Plan done:false"),

		// The encoding and the line structure.
		docReject("10#encoding-and-line-structure: invalid UTF-8", RuleEncoding, 2, "# items\n- [ ] T1 P\xff done:false\n"),
		docReject("10#encoding-and-line-structure: U+0000 in a row", RuleEncoding, 2, "# items\n- [ ] T1 P\x00 done:false\n"),
		{name: "10#encoding-and-line-structure: byte order mark", text: "\xef\xbb\xbf" + itemsOf("- [ ] T1 Plan done:false"), rule: RuleNoncanonical, line: 1, kept: true},
		noncanonicalDoc("10#encoding-and-line-structure: CRLF line endings", 1, strings.ReplaceAll(itemsOf("- [ ] T1 Plan done:false"), "\n", "\r\n")),
		noncanonicalDoc("10#document-order: H1 with extra spaces", 1, raw("#  items", "- [ ] T1 Plan done:false")),
		noncanonicalDoc("10#document-order: no H1 line", 1, raw("- [ ] T1 Plan done:false")),
		noncanonicalDoc("10#document-order: a blank line between the H1 and the rows", 2, raw("# items", "", "- [ ] T1 Plan done:false")),

		// List rows: the glyph, the key, the title and the fields.
		docAccept("10#list-rows: the glyph of each state", itemsOf("- [x] T1 Plan done:true", "- [~] T2 Plan done:false", "- [ ] T3 Plan done:false")),
		docAccept("10#list-rows: a child row indented two spaces", itemsOf("- [ ] T1 Plan done:false", "  - [ ] T2 Child done:false")),
		docAccept("10#list-rows: a child of a child", itemsOf("- [ ] T1 Plan done:false", "  - [ ] T2 Child done:false", "    - [ ] T3 Grandchild done:false")),
		docAccept("10#list-rows: a quoted field holding a space", itemsOf(`- [ ] T1 Plan done:false note:"a b"`)),
		docAccept("10#list-rows: escaped quote and backslash in a field", itemsOf(`- [ ] T1 Plan done:false note:"a \"b\" \\ c"`)),
		docAccept("10#list-rows: the empty text is a pair of quotes", itemsOf(`- [ ] T1 Plan done:false note:""`)),
		docAccept("10#list-rows: a title ending in a quoted word", itemsOf(`- [ ] T1 Say "hi" done:false`)),
		docAccept("10#list-rows: an escaped title token", itemsOf(`- [ ] T1 Set owner\: review done:false`)),
		docAccept("10#list-rows: a title token ending in an escaped colon is not a field", itemsOf(`- [ ] T1 Plan owner\: done:false`)),
		docAccept("10#list-rows: a foreign key link", itemsOf("- [ ] T1 Plan owner:[P1](people.flowmark.md#people-row-p1) done:false")),
		docAccept("10#foreign-keys-as-links: a dangling link keeps its written target", itemsOf("- [ ] T1 Plan owner:[P9](people.flowmark.md#people-row-p9) done:false")),
		docAccept("10#list-rows: the weight field of an i16", itemsOf("- [ ] T1 Plan done:false weight:-3")),
		docAccept("20#readable-values: a temporal field written now is kept as written, not of its type", itemsOf("- [ ] T1 Plan done:false seen_on:now")),
		docAccept("10#readable-values: a text field written now is text", itemsOf("- [ ] T1 Plan done:false note:now")),
		docAccept("20#readable-values: a temporal cell written now is kept as written, and a text cell now is text", peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "now", "true", "now", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.")),
		docAccept("10#list-rows: a list row without a state column", raw("# notes", "- N1 Plan", "  - N2 Child")),
		docReject("10#list-rows: a state glyph in a list without a state column", RuleBlock, 2, raw("# notes", "- [ ] N1 Plan")),
		noncanonicalDoc("10#detail-sections: detail sections out of storage order", 5, itemsOf(
			"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", "", "## items row T2", "", "B.", "", "## items row T1", "", "A.")),
		docAccept("10#single-line-places: U+1680 at the start of a title", itemsOf("- [ ] T1  Plan done:false")),
		docAccept("10#single-line-places: a prepended concatenation mark at the start of a title", itemsOf("- [ ] T1 ؀Plan done:false")),
		docAccept("10#detail-sections: an open fence takes in the next detail heading as its body", itemsOf(
			"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", "", "## items row T1", "", "```", "", "## items row T2", "", "x")),
		docReject("10#single-line-places: a table cell that begins with an invisible character", RuleBlock, 4, peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "​Ada", "lead", "true", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.")),
		docReject("10#list-rows: a row without a glyph", RuleBlock, 2, itemsOf("- T1 Plan done:false")),
		docReject("10#list-rows: an unknown glyph", RuleBlock, 2, itemsOf("- [?] T1 Plan done:false")),
		docReject("10#list-rows: a glyph of two characters", RuleBlock, 2, itemsOf("- [xx] T1 Plan done:false")),
		docReject("10#list-rows: an odd indentation", RuleBlock, 2, itemsOf("   - [ ] T1 Plan done:false")),
		docReject("10#list-rows: a row two levels under the row above", RuleBlock, 3, itemsOf("- [ ] T1 Plan done:false", "    - [ ] T2 Deep done:false")),
		docReject("10#list-rows: an undeclared field", RuleBlock, 2, itemsOf("- [ ] T1 Plan done:false nope:1")),
		docReject("10#list-rows: a field out of column order", RuleBlock, 2, itemsOf(`- [ ] T1 Plan note:"x" done:false`)),
		docReject("10#list-rows: a duplicate field", RuleBlock, 2, itemsOf("- [ ] T1 Plan done:false done:true")),
		docReject("10#list-rows: a non-null field missing", RuleBlock, 2, itemsOf("- [ ] T1 Plan")),
		docReject("10#list-rows: a bare value with an invisible character", RuleBlock, 2, itemsOf("- [ ] T1 Plan done:false note:a b")),
		docReject("10#list-rows: a bare value that is empty", RuleBlock, 2, itemsOf("- [ ] T1 Plan done:false note:")),
		docReject("10#list-rows: a quoted value with a bad escape", RuleBlock, 2, itemsOf(`- [ ] T1 Plan done:false note:"a\x"`)),
		docReject("10#list-rows: a quoted value that begins with an invisible character", RuleBlock, 2, itemsOf(`- [ ] T1 Plan done:false note:" a"`)),
		docReject("10#foreign-keys-as-links: a foreign key written as plain text", RuleBlock, 2, itemsOf("- [ ] T1 Plan owner:P1 done:false")),
		docReject("10#list-rows: an empty title", RuleBlock, 2, itemsOf("- [ ] T1 done:false")),
		docReject("10#list-rows: a key with a dash before an underscore", RuleBlock, 2, itemsOf("- [ ] T-_1 Plan done:false")),
		docReject("10#list-rows: a tab that ends the title before a field", RuleBlock, 2, itemsOf("- [ ] T1 Plan\tdone:false")),
		{name: "10#list-rows: an unescaped title token that begins with a column", rule: RuleBlock, line: 2, col: 10, text: itemsOf("- [ ] T1 Set owner: review done:false")},
		docReject("10#list-rows: a title that begins with an invisible character", RuleBlock, 2, itemsOf("- [ ] T1 ​Plan done:false")),
		docReject("10#single-line-places: a title with a comment opener", RuleBlock, 2, itemsOf("- [ ] T1 a <!-- b done:false")),
		docReject("10#list-rows: a parent field in a list", RuleBlock, 2, itemsOf("- [ ] T1 Plan done:false parent:T1")),
		docReject("10#list-rows: the order column written as a field", RuleBlock, 2, itemsOf("- [ ] T1 Plan done:false position:1")),
		docReject("10#list-rows: the state written as a field", RuleBlock, 2, itemsOf("- [ ] T1 Plan done:false state:waiting")),
		noncanonicalDoc("10#list-rows: two spaces between the key and the title", 2, itemsOf("- [ ] T1  Plan done:false")),
		noncanonicalDoc("10#list-rows: two spaces between fields", 2, itemsOf(`- [ ] T1 Plan done:false  note:"x"`)),
		noncanonicalDoc("10#list-rows: a tab between fields", 2, itemsOf("- [ ] T1 Plan done:false\tnote:\"x\"")),
		noncanonicalDoc("10#list-rows: a space at the end of the line", 2, itemsOf("- [ ] T1 Plan done:false ")),
		noncanonicalDoc("10#list-rows: a quoted bare value", 2, itemsOf(`- [ ] T1 Plan done:"false"`)),
		noncanonicalDoc("10#readable-values: an integer with a leading zero", 2, itemsOf("- [ ] T1 Plan done:false weight:01")),
		docAccept("10#list-rows: siblings in the line order of their order column", itemsOf("- [ ] T2 Plan done:false", "- [ ] T1 Plan done:false")),

		// Detail sections.
		docAccept("10#detail-sections: a body and a section", itemsOf("- [x] T1 Plan done:true", "", "## items row T1", "", "Body text.", "", "### evidence", "", "Proof.")),
		docAccept("10#detail-sections: a present empty section alone", itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### cause")),
		docAccept("10#detail-sections: a level-4 heading inside a body", itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "Body.", "", "#### deep", "", "Deeper.")),
		docReject("10#detail-sections: a second detail section of a row", RuleBlock, 8, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "A.", "", "## items row T1", "", "B.")),
		docReject("10#detail-sections: a detail section that names no row", RuleBlock, 4, itemsOf("- [ ] T1 Plan done:false", "", "## items row T9", "", "A.")),
		docReject("10#document-order: content after the list", RuleBlock, 4, itemsOf("- [ ] T1 Plan done:false", "", "Stray.")),
		docReject("10#detail-sections: a level-2 heading that is no detail section", RuleBlock, 4, itemsOf("- [ ] T1 Plan done:false", "", "## Notes", "", "x")),
		docReject("10#sections-and-their-values: an undeclared section", RuleSection, 6, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### nope", "", "x")),
		docReject("10#sections-and-their-values: a section written twice", RuleSection, 10, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### evidence", "", "a", "", "### evidence", "", "b")),
		noncanonicalDoc("10#detail-sections: a detail heading with a wrong case of row", 4, itemsOf("- [ ] T1 Plan done:false", "", "## items Row T1", "", "A.")),
		noncanonicalDoc("10#detail-sections: a detail heading with extra spaces", 4, itemsOf("- [ ] T1 Plan done:false", "", "## items  row T1", "", "A.")),
		noncanonicalDoc("10#detail-sections: a detail heading with a tab after its hashes", 4, itemsOf("- [ ] T1 Plan done:false", "", "##\titems row T1", "", "A.")),
		noncanonicalDoc("10#sections-and-their-values: a section heading with a tab after its hashes", 6, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "###\tevidence", "", "a")),
		noncanonicalDoc("10#detail-sections: a detail section without content", 4, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1")),
		noncanonicalDoc("10#detail-sections: a detail section directly after the list", 3, itemsOf("- [ ] T1 Plan done:false", "## items row T1", "", "A.")),

		// The people table: cells, the pipe escape, the parent link and the non-null sections.
		{name: "10#table-rows: rows with a parent link, a quoted title and the pipe escape", table: "people", text: peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""),
			row("P2", "[P1](#people-row-p1)", `Max \| Ops`, `""`, "false", "", "", "", "", "", "", "", "", ""),
			row("P3", "[P2](#people-row-p2)", `"\"Q\""`, "", "true", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.",
			"", "## people row P2", "", "### bio", "", "Ops.",
			"", "## people row P3", "", "### bio", "", "Third.")},
		docReject("10#table-rows: a row with one cell too few", RuleBlock, 4, peopleOf(peopleHeader, peopleDelim,
			row("P1", "", "Ada"), "", "## people row P1", "", "### bio", "", "Notes.")),
		{name: "10#table-rows: a header that names other columns", table: "people", rule: RuleBlock, line: 2,
			text: peopleOf("| id | name |", peopleDelim, row("P1", "Ada"))},
		{name: "10#table-rows: a delimiter row that is not one", table: "people", rule: RuleBlock, line: 3,
			text: peopleOf(peopleHeader, "| ? |", row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""), "", "## people row P1", "", "### bio", "", "Notes.")},
		docReject("10#foreign-keys-as-links: a parent cell written as plain text", RuleBlock, 5, peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""),
			row("P2", "P1", "Max", "", "true", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.",
			"", "## people row P2", "", "### bio", "", "Ops.")),
		docReject("10#table-rows: a parent that names no row", RuleBlock, 5, peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""),
			row("P2", "[P9](#people-row-p9)", "Max", "", "true", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.",
			"", "## people row P2", "", "### bio", "", "Ops.")),
		{name: "10#table-rows: a parent that names a key two rows share", table: "people", rule: RuleBlock, text: peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""),
			row("P1", "", "Ada again", "", "true", "", "", "", "", "", "", "", "", ""),
			row("P2", "[P1](#people-row-p1)", "Max", "", "true", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.",
			"", "## people row P2", "", "### bio", "", "Ops.")},
		docReject("10#table-rows: a non-null cell that is NULL", RuleBlock, 4, peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.")),
		docReject("10#table-rows: an empty title cell", RuleBlock, 4, peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "", "lead", "true", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.")),
		docReject("10#table-rows: a quoted cell with a bad escape", RuleBlock, 4, peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", `"\q"`, "lead", "true", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.")),
		docReject("10#detail-sections: a non-null section missing from a row", RuleBlock, 4, peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""))),
		noncanonicalDoc("10#table-rows: a cell with extra spaces", 4, peopleOf(
			peopleHeader, peopleDelim,
			strings.Replace(row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""), "| Ada |", "|  Ada  |", 1),
			"", "## people row P1", "", "### bio", "", "Notes.")),
		noncanonicalDoc("10#table-rows: a table with a header and no rows", 2, peopleOf(peopleHeader, peopleDelim)),
		noncanonicalDoc("10#table-rows: rows out of storage order", 5, peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""),
			row("P3", "[P2](#people-row-p2)", "Third", "", "true", "", "", "", "", "", "", "", "", ""),
			row("P2", "[P1](#people-row-p1)", "Max", "", "true", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.",
			"", "## people row P2", "", "### bio", "", "Ops.",
			"", "## people row P3", "", "### bio", "", "Third.")),

		// The sections of a row: an undeclared, a repeated and a child-block heading.
		{name: "10#sections-and-their-values: an undeclared section of a people row", table: "people", rule: RuleSection, line: 12, text: peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.", "", "### nope", "", "x")},

		// The milestones table: the order column is logical, the parent is a link to a row of the file.
		{name: "10#table-rows: a parent that names no row of a milestones table", table: "milestones", rule: RuleBlock, line: 4,
			text: milestonesOf("| id | parent | title |", "| --- | --- | --- |", row("M2", "[M9](milestones.flowmark.md)", "Parser"))},
		{name: "10#storage-order: a parent cycle", table: "milestones", rule: RuleParentCycle, text: milestonesOf(
			"| id | parent | title |", "| --- | --- | --- |",
			row("M1", "[M2](milestones.flowmark.md)", "A"),
			row("M2", "[M1](milestones.flowmark.md)", "B"))},
		{name: "10#order: the order column is not a header", table: "milestones", rule: RuleBlock, line: 2,
			text: milestonesOf("| id | parent | title | position |", "| --- | --- | --- | --- |", row("M1", "", "A", "1"))},

		// Block sections: a child block in a row's detail section, its list or table, and its rows' detail sections.
		docAccept("10#document-order: a table block under a row", itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |")),
		docAccept("10#detail-sections: a block row with a section, and a row without a detail section", itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"| 2 | doing | done | 2026-10-01 09:00:00.000000 |",
			"", "#### item_history row 1", "", "##### evidence", "", "Checked.")),
		docAccept("10#detail-sections: a block row with a present empty section", itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"", "#### item_history row 1", "", "##### evidence")),
		docAccept("10#document-order: a list block of nested rows, a NULL field and a body", itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_notes",
			"- N1 Spec the codec tag:urgent", `  - N2 Child note tag:""`, "- N3 Plain",
			"", "#### item_notes row N1", "", "Body of N1.")),
		docAccept("10#sections-and-their-values: a markdown section before a child block", itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### evidence", "", "Proof.",
			"", "### item_history", "| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |")),
		docAccept("10#document-order: a placement foreign key is implied by nesting, not a field", itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_notes",
			"- N1 Spec the codec")),
		noncanonicalDoc("10#document-order: an empty block heading", 6, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history")),
		noncanonicalDoc("10#document-order: a blank line between a block heading and its rows", 7, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history", "",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |")),
		docReject("10#list-rows: a placement foreign key written as a field of a block row", RuleBlock, 7, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_notes",
			"- N1 Spec item:T1 tag:urgent")),
		docReject("10#table-rows: a placement foreign key written as a cell of a block table", RuleBlock, 7, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | item | from_state | to_state | at |", "| --- | --- | --- | --- | --- |",
			"| 1 | T1 | waiting | doing | 2026-10-01 08:00:00.000000 |")),
		docReject("10#table-rows: a non-null cell of a block row that is NULL", RuleBlock, 9, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting |  | 2026-10-01 08:00:00.000000 |")),
		docReject("10#detail-sections: two block rows with one key and a detail section each collide", RuleBlock, 25, itemsOf(
			"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"", "#### item_history row 1", "", "##### evidence", "", "Checked.",
			"", "## items row T2", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | doing | done | 2026-10-01 09:00:00.000000 |",
			"", "#### item_history row 1", "", "##### evidence", "", "Other.")),
		docReject("10#detail-sections: a block row's detail names no row of its block", RuleBlock, 11, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"", "#### item_history row 9", "", "x")),
		docReject("10#detail-sections: a block row's detail at the level of the row above", RuleBlock, 11, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"", "##### item_history row 1", "", "x")),
		docReject("10#document-order: content after a block's table", RuleBlock, 10, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"Stray.")),
		docReject("10#detail-sections: a block named at the level of its row's detail heading", RuleBlock, 8, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "Body.", "", "## item_history", "| id | from_state | to_state | at |")),
		docReject("10#sections-and-their-values: a block section written twice", RuleSection, 11, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"", "### item_history", "| id | from_state | to_state | at |", "| --- | --- | --- | --- |")),
	}
}

// TestParseDocumentRules reads each case: an accepted input has no diagnostic,
// returns its rows and emits as itself (P1); a rejected input's first diagnostic
// has the rule, line and column of its case; a noncanonical input emits its
// canonical form, which parses back to the same bytes (P2).
func TestParseDocumentRules(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	for _, c := range documentParseCases() {
		t.Run(c.name, func(t *testing.T) {
			table := c.table
			if table == "" {
				table = tableOfH1(c.text)
			}
			rel := table + ".flowmark.md"
			rows, diags, err := ParseDocument(cat, table, rel, []byte(c.text), lk)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.rule == "" {
				if len(diags) > 0 {
					t.Fatalf("accepted input has diagnostics: %+v", diags)
				}
				if rows == nil {
					t.Fatal("accepted input returned no rows")
				}
				out := checkDocumentP2(t, cat, table, rows, lk)
				if !bytes.Equal(out, []byte(c.text)) {
					t.Fatalf("P1: emit(parse(s)) != s\n got: %q\nwant: %q", out, c.text)
				}
				return
			}
			if len(diags) == 0 {
				t.Fatalf("no diagnostic; want %s", c.rule)
			}
			if diags[0].Rule != c.rule {
				t.Fatalf("first diagnostic %+v; want rule %s", diags[0], c.rule)
			}
			if diags[0].File != rel || diags[0].Line < 1 || diags[0].Column < 1 {
				t.Fatalf("diagnostic not located in %s: %+v", rel, diags[0])
			}
			if c.line != 0 && diags[0].Line != c.line {
				t.Fatalf("first diagnostic at line %d; want %d: %+v", diags[0].Line, c.line, diags[0])
			}
			if c.col != 0 && diags[0].Column != c.col {
				t.Fatalf("first diagnostic at column %d; want %d: %+v", diags[0].Column, c.col, diags[0])
			}
			if (rows != nil) != c.kept {
				t.Fatalf("rows returned = %v; want %v", rows != nil, c.kept)
			}
			if rows != nil {
				out := checkDocumentP2(t, cat, table, rows, lk)
				if bytes.Equal(out, []byte(c.text)) {
					t.Fatalf("noncanonical input emits itself: %q", out)
				}
			}
		})
	}
}

// checkDocumentP2 requires that the rows write, that the written bytes parse
// back without a diagnostic, and that the rows write the same bytes again.
func checkDocumentP2(t *testing.T, cat *catalog.Catalog, table string, rows []*Row, lk Linker) []byte {
	t.Helper()
	out, diags, err := RenderDocument(cat, table, rows, lk)
	if err != nil || len(diags) > 0 {
		t.Fatalf("the rows are refused on write: %v %+v", err, diags)
	}
	back, diags, err := ParseDocument(cat, table, table+".flowmark.md", out, lk)
	if err != nil || len(diags) > 0 || back == nil {
		t.Fatalf("written file does not parse cleanly: %v %+v\n%s", err, diags, out)
	}
	again, diags, err := RenderDocument(cat, table, back, lk)
	if err != nil || len(diags) > 0 || !bytes.Equal(again, out) {
		t.Fatalf("P2: emit(parse(emit(d))) != emit(d)\n got: %q\nwant: %q", again, out)
	}
	return out
}

// TestDocumentGoldens requires that each golden document parses without a
// diagnostic and writes back to its own bytes (P1), and that the rows it reads
// are written in the same form by a second round trip (P2).
func TestDocumentGoldens(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	for _, table := range []string{"items", "people", "milestones"} {
		t.Run(table, func(t *testing.T) {
			data := readDocumentGolden(t, table)
			rows, diags, err := ParseDocument(cat, table, table+".flowmark.md", data, lk)
			if err != nil || len(diags) > 0 || rows == nil {
				t.Fatalf("golden has diagnostics: %v %+v", err, diags)
			}
			out, diags, err := RenderDocument(cat, table, rows, lk)
			if err != nil || len(diags) > 0 || !bytes.Equal(out, data) {
				t.Fatalf("P1 failed: %v %+v\n got: %q\nwant: %q", err, diags, out, data)
			}
			checkDocumentP2(t, cat, table, rows, lk)
		})
	}
}

// TestBlockTableIsNoDocument requires that a block table is not read or written as
// a document file: its rows live in the detail sections of its placement parent.
func TestBlockTableIsNoDocument(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	if _, _, err := ParseDocument(cat, "item_history", "item_history.flowmark.md", nil, lk); err == nil {
		t.Fatal("a block table read as a document table: want an error")
	}
	if _, _, err := RenderDocument(cat, "item_notes", nil, lk); err == nil {
		t.Fatal("a block table written as a document table: want an error")
	}
}

// writeCase is one set of rows the writer takes. rule is the rule of the first
// diagnostic, empty when the rows are accepted; out is the bytes written, when checked.
type writeCase struct {
	name  string
	table string // "" is items
	rows  []*Row
	rule  string
	out   string
}

// item is an items row with its state, its done flag and its position.
func item(key, title, state, position string, extra map[string]string) *Row {
	sc := map[string]*string{"state": sp(state), "position": sp(position)}
	for _, k := range []string{"done", "owner", "note", "weight", "seen_on"} {
		if v, ok := extra[k]; ok {
			sc[k] = sp(v)
		}
	}
	return &Row{Table: "items", Key: key, Title: title, Scalars: sc, Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
}

func writeCases() []writeCase {
	done := map[string]string{"done": "false"}
	doneTrue := map[string]string{"done": "true"}
	withNote := func(note string) map[string]string { return map[string]string{"done": "false", "note": note} }
	return []writeCase{
		{name: "10#list-rows: a waiting row", rows: []*Row{item("T1", "Plan", "waiting", "1", done)},
			out: "# items\n- [ ] T1 Plan done:false\n"},
		{name: "10#list-rows: a done row has the done glyph", rows: []*Row{item("T1", "Plan", "done", "1", doneTrue)},
			out: "# items\n- [x] T1 Plan done:true\n"},
		{name: "10#list-rows: a child row two spaces deeper", rows: []*Row{
			withParent(item("T1", "Plan", "waiting", "1", done), ""),
			withParent(item("T2", "Child", "waiting", "1", done), "T1"),
		}, out: "# items\n- [ ] T1 Plan done:false\n  - [ ] T2 Child done:false\n"},
		{name: "10#list-rows: the rows written in sibling order", rows: []*Row{
			item("T2", "Second", "waiting", "2", done),
			item("T1", "First", "waiting", "1", done),
		}, out: "# items\n- [ ] T1 First done:false\n- [ ] T2 Second done:false\n"},
		{name: "10#list-rows: a title token that begins with a column is escaped", rows: []*Row{
			item("T1", "Set owner: review", "waiting", "1", done)},
			out: "# items\n- [ ] T1 Set owner\\: review done:false\n"},
		{name: "10#list-rows: a field with a space is quoted", rows: []*Row{
			item("T1", "Plan", "waiting", "1", withNote("a b"))},
			out: "# items\n- [ ] T1 Plan done:false note:\"a b\"\n"},
		{name: "10#list-rows: an empty field is the pair of quotes", rows: []*Row{
			item("T1", "Plan", "waiting", "1", withNote(""))},
			out: "# items\n- [ ] T1 Plan done:false note:\"\"\n"},
		{name: "10#list-rows: a quote and a backslash are escaped in a field", rows: []*Row{
			item("T1", "Plan", "waiting", "1", withNote(`say "hi" \ bye`))},
			out: "# items\n- [ ] T1 Plan done:false note:\"say \\\"hi\\\" \\\\ bye\"\n"},
		{name: "10#list-rows: a title token that begins with a column is escaped", rows: []*Row{
			item("T1", "Ends with owner:", "waiting", "1", done)},
			out: "# items\n- [ ] T1 Ends with owner\\: done:false\n"},
		{name: "10#list-rows: a title token written with the escape already", rows: []*Row{
			item("T1", `Set owner\: review`, "waiting", "1", done)},
			rule: RuleConstraint},
		{name: "10#list-rows: a title with a leading space", rows: []*Row{
			item("T1", " Plan", "waiting", "1", done)}, rule: RuleConstraint},
		{name: "10#list-rows: a title with a line feed", rows: []*Row{
			item("T1", "Pl\nan", "waiting", "1", done)}, rule: RuleConstraint},
		{name: "10#list-rows: a state without a glyph", rows: []*Row{
			item("T1", "Plan", "nope", "1", done)}, rule: RuleConstraint},
		{name: "10#list-rows: a non-null field missing", rows: []*Row{
			item("T1", "Plan", "waiting", "1", map[string]string{})}, rule: RuleConstraint},
		{name: "10#foreign-keys-as-links: a foreign key that names no row", rows: []*Row{
			item("T1", "Plan", "waiting", "1", map[string]string{"done": "false", "owner": "P9"})},
			rule: RuleForeignKey},
		{name: "10#list-rows: a field with a trailing space", rows: []*Row{
			item("T1", "Plan", "waiting", "1", withNote("a "))}, rule: RuleConstraint},
		{name: "10#list-rows: a field with a line feed", rows: []*Row{
			item("T1", "Plan", "waiting", "1", withNote("a\nb"))}, rule: RuleConstraint},
		{name: "20#readable-values: a temporal field assigned now is not of its type", rows: []*Row{
			item("T1", "Plan", "waiting", "1", map[string]string{"done": "false", "seen_on": "now"})}, rule: RuleConstraint},
		{name: "10#readable-values: a text field assigned now is text", rows: []*Row{
			item("T1", "Plan", "waiting", "1", withNote("now"))},
			out: "# items\n- [ ] T1 Plan done:false note:now\n"},
		{name: "20#readable-values: a temporal cell assigned now is not of its type", table: "people", rows: []*Row{
			peopleRow("P1", "Ada", map[string]string{"born": "now"})}, rule: RuleConstraint},
		{name: "10#readable-values: a text cell assigned now is text", table: "people", rows: []*Row{
			peopleRow("P1", "Ada", map[string]string{"role": "now"})},
			out: "# people\n" + peopleHeader + "\n" + peopleDelim + "\n" +
				row("P1", "", "Ada", "now", "true", "", "", "", "", "", "", "", "", "") + "\n" +
				"\n## people row P1\n\n### bio\n\nNotes.\n"},
		{name: "10#list-rows: a key with a dash before an underscore", rows: []*Row{
			item("T-_1", "Plan", "waiting", "1", done)}, rule: RuleConstraint},
		{name: "10#list-rows: two rows with keys equal without regard to case", rows: []*Row{
			item("T1", "Plan", "waiting", "1", done), item("t1", "Other", "waiting", "2", done)},
			rule: RuleDuplicateKey},
		{name: "10#order: siblings with the same value", rows: []*Row{
			item("T1", "Plan", "waiting", "1", done), item("T2", "Other", "waiting", "1", done)}, rule: RuleConstraint},
		{name: "10#order: a row without an order value", rows: []*Row{
			{Table: "items", Key: "T1", Title: "Plan", Scalars: map[string]*string{"state": sp("waiting"), "done": sp("false")},
				Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}},
			item("T2", "Other", "waiting", "1", done)}, rule: RuleConstraint},
		{name: "10#order: siblings with a gap", rows: []*Row{
			item("T1", "Plan", "waiting", "1", done), item("T2", "Other", "waiting", "3", done)}, rule: RuleConstraint},
		{name: "10#hierarchy: a parent that names no row", rows: []*Row{
			withParent(item("T1", "Plan", "waiting", "1", done), "T9")}, rule: RuleConstraint},
		{name: "10#hierarchy: a parent cycle", rows: []*Row{
			withParent(item("T1", "Plan", "waiting", "1", done), "T2"),
			withParent(item("T2", "Other", "waiting", "1", done), "T1")}, rule: RuleConstraint},
		{name: "10#detail-sections: a body without its final line feed", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "Text.")}, rule: RuleConstraint},
		{name: "10#detail-sections: a body with a level-2 heading", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "## x\n")}, rule: RuleConstraint},
		{name: "10#detail-sections: a body with a level-3 heading", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "### x\n")}, rule: RuleConstraint},
		{name: "10#detail-sections: a body that begins with a blank line", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "\nText.\n")}, rule: RuleConstraint},
		{name: "10#detail-sections: a body with a CR", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "A\rB\n")}, rule: RuleConstraint},
		{name: "10#detail-sections: a body of a table without a body column", table: "people", rows: []*Row{
			withBody(peopleRow("P1", "Ada", map[string]string{}), "Text.\n")}, rule: RuleConstraint},
		{name: "10#detail-sections: a non-null section missing", table: "people", rows: []*Row{
			withoutSection(peopleRow("P1", "Ada", map[string]string{}), "bio")}, rule: RuleConstraint},
		{name: "10#detail-sections: an undeclared section", rows: []*Row{
			withSection(item("T1", "Plan", "waiting", "1", done), "nope", "x\n")}, rule: RuleConstraint},
		{name: "10#single-line-places: a title with an invisible character at its start", rows: []*Row{
			item("T1", "​Plan", "waiting", "1", done)}, rule: RuleConstraint},
		{name: "10#detail-sections: a body whose open fence precedes a section", rows: []*Row{
			withSection(withBody(item("T1", "Plan", "waiting", "1", done), "```\ncode\n"), "cause", "x")}, rule: RuleConstraint},
		{name: "10#detail-sections: a present empty section", rows: []*Row{
			withSection(item("T1", "Plan", "waiting", "1", done), "cause", "")},
			out: "# items\n- [ ] T1 Plan done:false\n\n## items row T1\n\n### cause\n"},
		{name: "10#hierarchy: a parent that names no row of its file", table: "people", rows: []*Row{
			peopleRow("P1", "Ada", map[string]string{"parent": "P9"})}, rule: RuleConstraint},
		{name: "10#list-rows: a list without a state column writes no glyph", table: "notes", rows: []*Row{
			notePlan("N1", "", "Plan"), notePlan("N2", "N1", "Child")},
			out: "# notes\n- N1 Plan\n  - N2 Child\n"},
		{name: "10#storage-order: the empty table is the H1 and one LF", table: "milestones", rows: nil,
			out: "# milestones\n"},
		{name: "10#storage-order: a milestones table in its storage order", table: "milestones", rows: []*Row{
			milestone("M3", "", "Release", "2"),
			milestone("M2", "M1", "Parser", "1"),
			milestone("M1", "", "Foundation", "1"),
		}, out: "# milestones\n| id | parent | title |\n| --- | --- | --- |\n| M1 |  | Foundation |\n| M2 | [M1](milestones.flowmark.md) | Parser |\n| M3 |  | Release |\n"},
		{name: "10#table-rows: a cell with a pipe, a quote and a NULL", table: "people", rows: []*Row{
			peopleRow("P1", "Max | Ops", map[string]string{"role": ""})},
			out: "# people\n" + peopleHeader + "\n" + peopleDelim + "\n" +
				row("P1", "", `Max \| Ops`, `""`, "true", "", "", "", "", "", "", "", "", "") + "\n" +
				"\n## people row P1\n\n### bio\n\nNotes.\n"},

		// Block sections: a block is written in its row's detail section, only when it holds rows.
		{name: "10#document-order: a table block under a row", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history", historyRow("1", "waiting", "doing", "2026-10-01 08:00:00.000000"))},
			out: "# items\n- [ ] T1 Plan done:false\n\n## items row T1\n\n### item_history\n" +
				"| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |\n"},
		{name: "10#detail-sections: a block row's section and its detail heading", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history",
				withSection(historyRow("1", "waiting", "doing", "2026-10-01 08:00:00.000000"), "evidence", "Checked.\n"))},
			out: "# items\n- [ ] T1 Plan done:false\n\n## items row T1\n\n### item_history\n" +
				"| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |\n\n" +
				"#### item_history row 1\n\n##### evidence\n\nChecked.\n"},
		{name: "10#document-order: an empty block is not written", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history")},
			out: "# items\n- [ ] T1 Plan done:false\n"},
		{name: "10#document-order: a block of two rows is written in storage order", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history",
				historyRow("2", "doing", "done", "2026-10-01 09:00:00.000000"),
				historyRow("1", "waiting", "doing", "2026-10-01 08:00:00.000000"))},
			out: "# items\n- [ ] T1 Plan done:false\n\n## items row T1\n\n### item_history\n" +
				"| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n" +
				"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |\n" +
				"| 2 | doing | done | 2026-10-01 09:00:00.000000 |\n"},
		{name: "10#document-order: a list block of nested rows", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_notes",
				noteRow("N1", "", "Spec the codec", "1", map[string]string{"tag": "urgent"}),
				noteRow("N2", "N1", "Child note", "1", map[string]string{"tag": ""}))},
			out: "# items\n- [ ] T1 Plan done:false\n\n## items row T1\n\n### item_notes\n" +
				"- N1 Spec the codec tag:urgent\n  - N2 Child note tag:\"\"\n"},
		{name: "10#list-rows: a block of list rows whose order has a gap", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_notes",
				noteRow("N1", "", "A", "1", nil), noteRow("N2", "", "B", "3", nil))}, rule: RuleConstraint},
		{name: "10#list-rows: two block rows with keys equal without regard to case", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_notes",
				noteRow("N1", "", "A", "1", nil), noteRow("n1", "", "B", "2", nil))}, rule: RuleDuplicateKey},
		{name: "10#hierarchy: a block row whose parent names no row of its block", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_notes",
				noteRow("N1", "N9", "A", "1", nil))}, rule: RuleConstraint},
		{name: "10#detail-sections: a block that the table does not declare", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_links",
				historyRow("1", "waiting", "doing", "2026-10-01 08:00:00.000000"))}, rule: RuleConstraint},
		{name: "10#detail-sections: a block row of another table", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history", noteRow("N1", "", "A", "1", nil))}, rule: RuleConstraint},
		{name: "10#hierarchy: a placement foreign key assigned to a block row", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history",
				withScalar(historyRow("1", "waiting", "doing", "2026-10-01 08:00:00.000000"), "item", "T2"))}, rule: RuleConstraint},
		{name: "10#detail-sections: a block row missing a non-null cell", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history",
				withoutScalar(historyRow("1", "waiting", "doing", "2026-10-01 08:00:00.000000"), "to_state"))}, rule: RuleConstraint},
	}
}

// withBlock sets the rows of the child block table of row r.
func withBlock(r *Row, table string, rows ...*Row) *Row {
	if r.Blocks == nil {
		r.Blocks = map[string][]*Row{}
	}
	r.Blocks[table] = rows
	return r
}

// withScalar sets the scalar column name of row r.
func withScalar(r *Row, name, value string) *Row {
	r.Scalars[name] = sp(value)
	return r
}

// withoutScalar removes the scalar column name from row r.
func withoutScalar(r *Row, name string) *Row {
	delete(r.Scalars, name)
	return r
}

// historyRow is a row of the item_history block of an items row; its evidence is NULL.
func historyRow(id, from, to, at string) *Row {
	sc := map[string]*string{"from_state": sp(from), "to_state": sp(to), "at": sp(at)}
	return &Row{Table: "item_history", Key: id, Scalars: sc, Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
}

// noteRow is a row of the item_notes block of an items row with its parent, its order and the scalars given.
func noteRow(key, parent, title, position string, scalars map[string]string) *Row {
	sc := map[string]*string{"position": sp(position)}
	if parent != "" {
		sc["parent"] = sp(parent)
	}
	for k, v := range scalars {
		sc[k] = sp(v)
	}
	return &Row{Table: "item_notes", Key: key, Title: title, Scalars: sc, Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
}

// withParent sets the parent of row r to the key parent, empty for a root row.
func withParent(r *Row, parent string) *Row {
	if parent != "" {
		r.Scalars["parent"] = sp(parent)
	}
	return r
}

// withBody sets the body of row r.
func withBody(r *Row, body string) *Row {
	r.Body = body
	return r
}

// withoutSection removes the markdown section name from row r.
func withoutSection(r *Row, name string) *Row {
	delete(r.Sections, name)
	return r
}

// withSection sets the markdown section name of row r.
func withSection(r *Row, name, value string) *Row {
	r.Sections[name] = sp(value)
	return r
}

// notePlan is a row of the notes table, a list without a state column, with its parent.
func notePlan(key, parent, title string) *Row {
	sc := map[string]*string{"position": sp("1")}
	if parent != "" {
		sc["parent"] = sp(parent)
	}
	return &Row{Table: "notes", Key: key, Title: title, Scalars: sc, Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
}

// milestone is a row of the milestones table with its parent and its position.
func milestone(key, parent, title, position string) *Row {
	sc := map[string]*string{"position": sp(position)}
	if parent != "" {
		sc["parent"] = sp(parent)
	}
	return &Row{Table: "milestones", Key: key, Title: title, Scalars: sc, Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
}

// peopleRow is a row of the people table with a non-null bio and the scalars given.
func peopleRow(key, name string, scalars map[string]string) *Row {
	sc := map[string]*string{"active": sp("true")}
	for k, v := range scalars {
		sc[k] = sp(v)
	}
	return &Row{Table: "people", Key: key, Title: name, Scalars: sc,
		Sections: map[string]*string{"bio": sp("Notes.\n")}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
}

// TestRenderDocumentRules requires that each row set writes its bytes, or is
// refused with the rule that names it, and that a refusal writes nothing.
func TestRenderDocumentRules(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	for _, c := range writeCases() {
		t.Run(c.name, func(t *testing.T) {
			table := c.table
			if table == "" {
				table = "items"
			}
			out, diags, err := RenderDocument(cat, table, c.rows, lk)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.rule != "" {
				if len(diags) == 0 || diags[0].Rule != c.rule {
					t.Fatalf("diagnostics %+v; want rule %s", diags, c.rule)
				}
				if out != nil {
					t.Fatalf("a refused row set writes bytes: %q", out)
				}
				return
			}
			if len(diags) > 0 {
				t.Fatalf("accepted rows have diagnostics: %+v", diags)
			}
			if c.out != "" && string(out) != c.out {
				t.Fatalf("written %q; want %q", out, c.out)
			}
		})
	}
}
