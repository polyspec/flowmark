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
	// milestoneHeader and milestoneRow are a three-column table without detail sections.
	milestoneHeader = "| id | parent | title |"
	milestoneDelim  = "| --- | --- | --- |"
	milestoneRow    = "| M1 |  | Foundation |"
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
// not checked; kept says whether one or more rows are returned along with the diagnostics;
// alone says that the first diagnostic is the only one; readOnly says that the
// accepted rows are only read, since the writer refuses their shared key as
// DUPLICATE_KEY, which the validator reports and which leaves the table unavailable.
type docCase struct {
	name     string
	table    string // "" is items
	text     string
	rule     string
	line     int
	col      int
	kept     bool
	alone    bool
	readOnly bool
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
		{name: "10#document-order: an H1 with the table name capitalized", rule: RuleNoncanonical, line: 1, col: 3, kept: true, text: raw("# Items", "- [ ] T1 Plan done:false")},
		noncanonicalDoc("10#document-order: an H1 with the table name in capitals", 1, raw("# ITEMS", "- [ ] T1 Plan done:false")),
		{name: "10#document-order: an H1 in another case over no rows", rule: RuleNoncanonical, line: 1, col: 3, text: raw("# Items")},
		noncanonicalDoc("10#document-order: a blank line between the H1 and the rows", 2, raw("# items", "", "- [ ] T1 Plan done:false")),
		noncanonicalDoc("10#document-order: two blank lines between the H1 and the rows", 2, raw("# items", "", "", "- [ ] T1 Plan done:false")),
		noncanonicalDoc("10#document-order: a line of spaces and tabs between the H1 and the rows", 2, raw("# items", " \t ", "- [ ] T1 Plan done:false")),
		noncanonicalDoc("10#document-order: a blank line between the H1 and a table", 2, raw("# milestones", "", "| id | parent | title |", "| --- | --- | --- |", row("M1", "", "Foundation"))),

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
		docAccept("10#readable-values: a foreign key whose key is not of its column type keeps its bytes on an unchanged rewrite",
			itemsOf("- [ ] T1 Plan owner:["+strings.Repeat("P", 40)+"](people.flowmark.md) done:false")),
		docAccept("10#foreign-keys-as-links: a dangling link keeps its written target", itemsOf("- [ ] T1 Plan owner:[P9](people.flowmark.md#people-row-p9) done:false")),
		docAccept("10#list-rows: the weight field of an i16", itemsOf("- [ ] T1 Plan done:false weight:-3")),
		docAccept("10#readable-values: a temporal field written now is kept as written, not of its type", itemsOf("- [ ] T1 Plan done:false seen_on:now")),
		docAccept("10#readable-values: a text field written now is text", itemsOf("- [ ] T1 Plan done:false note:now")),
		docAccept("10#readable-values: a temporal cell written now is kept as written, and a text cell now is text", peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "now", "true", "now", "", "", "", "", "", "", "", ""),
			"", "## people row P1", "", "### bio", "", "Notes.")),
		docAccept("10#list-rows: a list row without a state column", raw("# notes", "- N1 Plan", "  - N2 Child")),
		docReject("10#list-rows: a state glyph in a list without a state column", RuleBlock, 2, raw("# notes", "- [ ] N1 Plan")),
		noncanonicalDoc("10#detail-headings-and-anchors: detail sections out of storage order", 5, itemsOf(
			"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", "", "## items row T2", "", "B.", "", "## items row T1", "", "A.")),
		docAccept("10#single-line-places: U+1680 at the start of a title", itemsOf("- [ ] T1  Plan done:false")),
		docAccept("10#single-line-places: a prepended concatenation mark at the start of a title", itemsOf("- [ ] T1 ؀Plan done:false")),
		docAccept("10#detail-headings-and-anchors: an open fence takes in the next detail heading as its body", itemsOf(
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
		docAccept("10#list-rows: an undeclared token after a field is title text", itemsOf("- [ ] T1 Plan nope:1 done:false")),
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
		// The run after the list marker, measured in columns, a tab advancing to the next multiple of four.
		{name: "10#list-rows: a tab after the list marker is a run of three columns", rule: RuleNoncanonical, line: 2, col: 2, kept: true, text: itemsOf("-\t[ ] T1 Plan done:false")},
		{name: "10#list-rows: a space and a tab after the list marker are a run of three columns", rule: RuleNoncanonical, line: 2, col: 2, kept: true, text: itemsOf("- \t[ ] T1 Plan done:false")},
		{name: "10#list-rows: a tab after the list marker of a child row is a run of one column", rule: RuleNoncanonical, line: 3, col: 4, kept: true, text: itemsOf("- [ ] T1 Plan done:false", "  -\t[ ] T2 Child done:false")},
		{name: "10#list-rows: two tabs after the list marker are a run of seven columns", rule: RuleBlock, line: 2, col: 2, text: itemsOf("-\t\t[ ] T1 Plan done:false")},
		{name: "10#list-rows: two spaces after the list marker", rule: RuleNoncanonical, line: 2, col: 2, kept: true, text: itemsOf("-  [ ] T1 Plan done:false")},
		{name: "10#list-rows: four spaces after the list marker", rule: RuleNoncanonical, line: 2, col: 2, kept: true, text: itemsOf("-    [ ] T1 Plan done:false")},
		noncanonicalDoc("10#list-rows: three spaces after the marker of a child row", 3, itemsOf("- [ ] T1 Plan done:false", "  -   [ ] T2 Child done:false")),
		noncanonicalDoc("10#list-rows: three spaces after the marker of a row with a sibling", 2, itemsOf("-   [ ] T1 Plan done:false", "- [ ] T2 Other done:false")),
		{name: "10#list-rows: five spaces after the list marker open a code block", rule: RuleBlock, line: 2, col: 2, text: itemsOf("-     [ ] T1 Plan done:false")},
		{name: "10#list-rows: five spaces after the marker of a child row", rule: RuleBlock, line: 3, col: 4, text: itemsOf("- [ ] T1 Plan done:false", "  -     [ ] T2 Child done:false")},
		noncanonicalDoc("10#list-rows: a child at the content indent of a row with three spaces after its marker", 2, itemsOf("-   [ ] T1 Plan done:false", "    - [ ] T2 Child done:false")),
		noncanonicalDoc("10#list-rows: a child at the content indent of a row with four spaces after its marker", 2, itemsOf("-    [ ] T1 Plan done:false", "      - [ ] T2 Child done:false")),
		noncanonicalDoc("10#list-rows: a child at the content indent of a row with two spaces after its marker", 2, itemsOf("-  [ ] T1 Plan done:false", "   - [ ] T2 Child done:false")),
		noncanonicalDoc("10#list-rows: a tab that ends the title before a field", 2, itemsOf("- [ ] T1 Plan\tdone:false")),
		noncanonicalDoc("10#list-rows: a tab that ends the title before a field and a second field", 2, itemsOf("- [ ] T1 Plan\tdone:false note:\"x\"")),
		noncanonicalDoc("10#list-rows: an unescaped title token that begins with a column", 2, itemsOf("- [ ] T1 Set owner: review done:false")),
		noncanonicalDoc("10#list-rows: an unescaped title token with a value that begins with a column", 2, itemsOf("- [ ] T1 Set owner:x review done:false")),
		noncanonicalDoc("10#list-rows: an unescaped title token after a tab that begins with a column", 2, itemsOf("- [ ] T1 Set\towner:x review done:false")),
		docReject("10#list-rows: an unescaped title token that ends the title", RuleBlock, 2, itemsOf(`- [ ] T1 Plan owner:a" done:false`)),
		docReject("10#list-rows: a title that ends with a column and no value", RuleBlock, 2, itemsOf("- [ ] T1 fix owner:")),
		docReject("10#list-rows: a title that ends with a column and no value before a field", RuleBlock, 2, itemsOf("- [ ] T1 fix owner: done:false")),
		docAccept("10#list-rows: two spaces before an escaped title token", itemsOf(`- [ ] T1 a  owner\:x done:false`)),
		docAccept("10#list-rows: a tab inside a title before an escaped title token", itemsOf("- [ ] T1 Set\towner\\:x done:false")),
		docAccept("10#list-rows: a tab inside a title before an escaped column token that holds a column", itemsOf("- [ ] T1 Plan\tnote\\:x done:false")),
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
		docAccept("10#detail-headings-and-anchors: a body and a section", itemsOf("- [x] T1 Plan done:true", "", "## items row T1", "", "Body text.", "", "### evidence", "", "Proof.")),
		docAccept("10#detail-headings-and-anchors: a present empty section alone", itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### cause")),
		docAccept("10#detail-headings-and-anchors: a level-4 heading inside a body", itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "Body.", "", "#### deep", "", "Deeper.")),
		docReject("10#detail-headings-and-anchors: a second detail section of a row", RuleBlock, 8, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "A.", "", "## items row T1", "", "B.")),
		docReject("10#detail-headings-and-anchors: a detail section that names no row", RuleBlock, 4, itemsOf("- [ ] T1 Plan done:false", "", "## items row T9", "", "A.")),
		docReject("10#document-order: a detail section whose key two rows share, only one of them with a section", RuleBlock, 5, itemsOf("- [ ] T1 Plan done:false", "- [ ] T1 Other done:false", "", "## items row T1", "", "A.")),
		{name: "10#document-order: keys equal without regard to case match one row each", text: itemsOf("- [ ] T1 Plan done:false", "- [ ] t1 Other done:false", "", "## items row T1", "", "A."), readOnly: true},
		docReject("10#document-order: content after the list", RuleBlock, 4, itemsOf("- [ ] T1 Plan done:false", "", "Stray.")),
		docReject("10#detail-headings-and-anchors: a level-2 heading that is no detail section", RuleBlock, 4, itemsOf("- [ ] T1 Plan done:false", "", "## Notes", "", "x")),
		docReject("10#sections-and-their-values: an undeclared section", RuleSection, 6, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### nope", "", "x")),
		docReject("10#sections-and-their-values: a section written twice", RuleSection, 10, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### evidence", "", "a", "", "### evidence", "", "b")),
		docReject("10#sections-and-their-values: a section written twice with equal values", RuleSection, 10, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### evidence", "", "a", "", "### evidence", "", "a")),
		noncanonicalDoc("10#detail-headings-and-anchors: a detail heading with a wrong case of row", 4, itemsOf("- [ ] T1 Plan done:false", "", "## items Row T1", "", "A.")),
		noncanonicalDoc("10#detail-headings-and-anchors: a detail heading with a wrong case of the table name", 4, itemsOf("- [ ] T1 Plan done:false", "", "## Items row T1", "", "A.")),
		noncanonicalDoc("10#detail-headings-and-anchors: a detail heading with the table name and row in capitals", 4, itemsOf("- [ ] T1 Plan done:false", "", "## ITEMS ROW T1", "", "A.")),
		docReject("10#detail-headings-and-anchors: a detail heading whose table name lowercases with U+0307 is another table", RuleBlock, 4, itemsOf("- [ ] T1 Plan done:false", "", "## İtems row T1", "", "A.")),
		docReject("10#detail-headings-and-anchors: a detail heading naming another table", RuleBlock, 4, itemsOf("- [ ] T1 Plan done:false", "", "## people row P1", "", "A.")),
		docReject("10#detail-headings-and-anchors: a detail heading with a lowercase key", RuleBlock, 4, itemsOf("- [ ] T1 Plan done:false", "", "## items row t1", "", "A.")),
		noncanonicalDoc("10#detail-headings-and-anchors: a detail heading with extra spaces", 4, itemsOf("- [ ] T1 Plan done:false", "", "## items  row T1", "", "A.")),
		noncanonicalDoc("10#detail-headings-and-anchors: a detail heading with a tab after its hashes", 4, itemsOf("- [ ] T1 Plan done:false", "", "##\titems row T1", "", "A.")),
		noncanonicalDoc("10#sections-and-their-values: a section heading with a tab after its hashes", 6, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "###\tevidence", "", "a")),
		noncanonicalDoc("10#detail-headings-and-anchors: a detail section without content", 4, itemsOf("- [ ] T1 Plan done:false", "", "## items row T1")),
		noncanonicalDoc("10#detail-headings-and-anchors: a detail section directly after the list", 3, itemsOf("- [ ] T1 Plan done:false", "## items row T1", "", "A.")),

		// The blank line between the rows and the element after them.
		noncanonicalDoc("10#document-order: a line of spaces between the list and the first detail section", 3, itemsOf("- [ ] T1 Plan done:false", "  ", "## items row T1", "", "A.")),
		noncanonicalDoc("10#document-order: a line of a tab between the table and the first detail section", 5, peopleOf(peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""), "\t", "## people row P1", "", "### bio", "", "Notes.")),
		noncanonicalDoc("10#document-order: two blank lines between the list and the first detail section", 3, itemsOf("- [ ] T1 Plan done:false", "", "", "## items row T1", "", "A.")),
		noncanonicalDoc("10#document-order: a detail section directly after a table", 5, peopleOf(peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""), "## people row P1", "", "### bio", "", "Notes.")),
		noncanonicalDoc("10#document-order: a line of spaces after the last row of the file", 3, itemsOf("- [ ] T1 Plan done:false", " ")),
		noncanonicalDoc("10#document-order: a line of spaces after the last row of a block", 8, checkedOf("- [ ] C1 Plan done:false", " ")),
		docReject("10#document-order: a line of a form feed between the list and the first detail section", RuleBlock, 3, itemsOf("- [ ] T1 Plan done:false", "\f", "## items row T1", "", "A.")),

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
		noncanonicalDoc("10#table-rows: a delimiter row with alignment colons", 3, milestonesOf(milestoneHeader, "| :--- | --- | ---: |", milestoneRow)),
		noncanonicalDoc("10#table-rows: a delimiter row without outer pipes", 3, milestonesOf(milestoneHeader, "--- | --- | ---", milestoneRow)),
		noncanonicalDoc("10#table-rows: a delimiter row with trailing spaces and tabs", 3, milestonesOf(milestoneHeader, "| --- | --- | --- |  \t", milestoneRow)),
		noncanonicalDoc("10#table-rows: a delimiter row with vertical tabs and form feeds", 3, milestonesOf(milestoneHeader, "|\v---\f|\v---|\f---\v|", milestoneRow)),
		noncanonicalDoc("10#table-rows: a delimiter row indented by three spaces", 3, milestonesOf(milestoneHeader, "   | --- | --- | --- |", milestoneRow)),
		noncanonicalDoc("10#table-rows: a delimiter row with a leading pipe and no trailing pipe", 3, milestonesOf(milestoneHeader, "| --- | --- | ---", milestoneRow)),
		docReject("10#table-rows: a delimiter row indented by four spaces", RuleBlock, 3, milestonesOf(milestoneHeader, "    | --- | --- | --- |", milestoneRow)),
		docReject("10#table-rows: a delimiter cell with two colons before the hyphens", RuleBlock, 3, milestonesOf(milestoneHeader, "| ::- | --- | --- |", milestoneRow)),
		docReject("10#table-rows: a delimiter row of a cell count other than the header's", RuleBlock, 3, milestonesOf(milestoneHeader, "| --- | --- |", milestoneRow)),
		docReject("10#table-rows: a setext-shaped line without outer pipes", RuleBlock, 3, milestonesOf(milestoneHeader, "---  \t", milestoneRow)),
		docReject("10#table-rows: a list-item-shaped line without outer pipes", RuleBlock, 3, milestonesOf(milestoneHeader, "- --- | --- | ---", milestoneRow)),
		docReject("10#table-rows: a delimiter row indented by a space and a tab", RuleBlock, 3, milestonesOf(milestoneHeader, " \t"+milestoneDelim, milestoneRow)),
		docReject("10#table-rows: a delimiter row without outer pipes indented by a space and a tab", RuleBlock, 3, milestonesOf(milestoneHeader, " \t--- | --- | ---", milestoneRow)),

		// The header row as GFM reads it, and a row without its last pipe.
		noncanonicalDoc("10#table-rows: a header row without outer pipes", 2, milestonesOf("id | parent | title", milestoneDelim, milestoneRow)),
		noncanonicalDoc("10#table-rows: a header row without a leading pipe", 2, milestonesOf("id | parent | title |", milestoneDelim, milestoneRow)),
		noncanonicalDoc("10#table-rows: a header row without a trailing pipe", 2, milestonesOf("| id | parent | title", milestoneDelim, milestoneRow)),
		noncanonicalDoc("10#table-rows: a header row indented by one space", 2, milestonesOf(" "+milestoneHeader, milestoneDelim, milestoneRow)),
		noncanonicalDoc("10#table-rows: a header row indented by three spaces", 2, milestonesOf("   "+milestoneHeader, milestoneDelim, milestoneRow)),
		noncanonicalDoc("10#table-rows: spaces and a tab after the header's last pipe", 2, milestonesOf(milestoneHeader+" \t ", milestoneDelim, milestoneRow)),
		noncanonicalDoc("10#table-rows: a vertical tab after the header's last pipe", 2, milestonesOf(milestoneHeader+"\v", milestoneDelim, milestoneRow)),
		noncanonicalDoc("10#table-rows: a form feed after the header's last pipe", 2, milestonesOf(milestoneHeader+"\f", milestoneDelim, milestoneRow)),
		noncanonicalDoc("10#table-rows: a form feed after a header without a trailing pipe", 2, milestonesOf("| id | parent | title\f", milestoneDelim, milestoneRow)),
		docReject("10#table-rows: a header row indented by four spaces", RuleBlock, 2, milestonesOf("    "+milestoneHeader, milestoneDelim, milestoneRow)),
		docReject("10#table-rows: a header row indented by a space and a tab", RuleBlock, 2, milestonesOf(" \t"+milestoneHeader, milestoneDelim, milestoneRow)),
		docReject("10#table-rows: a header row without a pipe", RuleBlock, 2, milestonesOf("id parent title", milestoneDelim, milestoneRow)),
		noncanonicalDoc("10#table-rows: a row without its trailing pipe", 4, milestonesOf(milestoneHeader, milestoneDelim, "| M1 |  | Foundation")),
		noncanonicalDoc("10#table-rows: a row without its trailing pipe ending in an escaped pipe", 4, milestonesOf(milestoneHeader, milestoneDelim, `| M1 |  | Found\|`)),
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
		docReject("10#detail-headings-and-anchors: a non-null section missing from a row", RuleBlock, 4, peopleOf(
			peopleHeader, peopleDelim,
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""))),
		noncanonicalDoc("10#table-rows: a cell with extra spaces", 4, peopleOf(
			peopleHeader, peopleDelim,
			strings.Replace(row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", ""), "| Ada |", "|  Ada  |", 1),
			"", "## people row P1", "", "### bio", "", "Notes.")),
		{name: "10#table-rows: a table with a header and no rows", rule: RuleNoncanonical, line: 2, text: peopleOf(peopleHeader, peopleDelim)},
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
		{name: "10#document-order: a parent cycle", table: "milestones", rule: RuleParentCycle, text: milestonesOf(
			"| id | parent | title |", "| --- | --- | --- |",
			row("M1", "[M2](milestones.flowmark.md)", "A"),
			row("M2", "[M1](milestones.flowmark.md)", "B"))},
		{name: "20#order: the order column is not a header", table: "milestones", rule: RuleBlock, line: 2,
			text: milestonesOf("| id | parent | title | position |", "| --- | --- | --- | --- |", row("M1", "", "A", "1"))},

		// Block sections: a child block in a row's detail section, its list or table, and its rows' detail sections.
		docAccept("10#document-order: a table block under a row", itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |")),
		docAccept("10#detail-headings-and-anchors: a block row with a section, and a row without a detail section", itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"| 2 | doing | done | 2026-10-01 09:00:00.000000 |",
			"", "#### item_history row 1", "", "##### evidence", "", "Checked.")),
		docAccept("10#detail-headings-and-anchors: a block row with a present empty section", itemsOf(
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
		docReject("10#detail-headings-and-anchors: two block rows with one key and a detail section each collide", RuleBlock, 25, itemsOf(
			"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"", "#### item_history row 1", "", "##### evidence", "", "Checked.",
			"", "## items row T2", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | doing | done | 2026-10-01 09:00:00.000000 |",
			"", "#### item_history row 1", "", "##### evidence", "", "Other.")),
		docReject("10#detail-headings-and-anchors: a block row's detail names no row of its block", RuleBlock, 11, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"", "#### item_history row 9", "", "x")),
		docReject("10#detail-headings-and-anchors: a block row's detail at the level of the row above", RuleBlock, 11, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"", "##### item_history row 1", "", "x")),
		docReject("10#document-order: content after a block's table", RuleBlock, 10, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"Stray.")),
		docReject("10#detail-headings-and-anchors: a block named at the level of its row's detail heading", RuleBlock, 8, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "Body.", "", "## item_history", "| id | from_state | to_state | at |")),
		docReject("10#sections-and-their-values: a block section written twice", RuleSection, 11, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"", "### item_history", "| id | from_state | to_state | at |", "| --- | --- | --- | --- |")),
		// A parent cell names a row of its own block; a key two blocks of the file share is assigned by its block.
		{name: "10#table-rows: a parent cell in one block names a key another block shares", readOnly: true, text: itemsOf(
			"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", "",
			"## items row T1", "", "### item_steps", "| id | parent |", "| --- | --- |",
			"| S1 |  |", "| S2 | [S1](items.flowmark.md) |", "",
			"## items row T2", "", "### item_steps", "| id | parent |", "| --- | --- |", "| S1 |  |"), kept: true},
		{name: "10#document-order: a parent cell naming a row of another block", rule: RuleBlock, line: 17, alone: true, text: itemsOf(
			"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", "",
			"## items row T1", "", "### item_steps", "| id | parent |", "| --- | --- |", "| S1 |  |", "",
			"## items row T2", "", "### item_steps", "| id | parent |", "| --- | --- |", "| S2 | [S1](items.flowmark.md) |")},
		{name: "10#table-rows: a parent cell naming a key two rows of its block share", rule: RuleBlock, line: 11, alone: true, text: itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_steps",
			"| id | parent |", "| --- | --- |", "| S1 |  |", "| S1 |  |", "| S2 | [S1](items.flowmark.md) |")},
		{name: "10#document-order: a detail key that two blocks of the file share, with a section in one", readOnly: true, text: itemsOf(
			"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", "",
			"## items row T1", "", "### item_notes", "- N1 Spec the codec", "",
			"#### item_notes row N1", "", "Body of N1.", "",
			"## items row T2", "", "### item_notes", "- N1 Other"), kept: true},

		// A blank line after a block heading is noncanonical in any count, and a line of spaces or tabs is one.
		noncanonicalDoc("10#document-order: two blank lines between a block heading and a table", 7, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history", "", "",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |")),
		noncanonicalDoc("10#document-order: a line of spaces between a block heading and a table", 7, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_history", "  ",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |")),
		noncanonicalDoc("10#document-order: two blank lines between a block heading and a list", 7, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_notes", "", "",
			"- N1 Spec the codec")),
		noncanonicalDoc("10#document-order: a line of a tab between a block heading and a list", 7, itemsOf(
			"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_notes", "\t",
			"- N1 Spec the codec")),

		// The block of a state machine (item_checks): its glyphs, fields, order, nesting and history block.
		docAccept("10#list-rows: the glyph of each state in a block", checkedOf("- [ ] C1 Open done:false", "- [~] C2 Doing done:false", "- [x] C3 Done done:true")),
		docReject("10#list-rows: an unknown glyph in a block", RuleBlock, 7, checkedOf("- [?] C1 Open done:false")),
		docReject("10#list-rows: a missing glyph in a block", RuleBlock, 7, checkedOf("- C1 Open done:false")),
		docAccept("10#list-rows: a child and a grandchild indented in a block", checkedOf("- [x] C1 Plan done:true", "  - [~] C2 Child done:false", "    - [ ] C3 Grandchild done:false")),
		docReject("10#list-rows: a row indented under no row of its block", RuleBlock, 7, checkedOf("  - [ ] C1 Child done:false")),
		docReject("10#list-rows: a child row indented under a parent in another block", RuleBlock, 13, itemsOf(
			"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", "", "## items row T1", "", "### item_checks",
			"- [x] C1 Plan done:true", "", "## items row T2", "", "### item_checks", "  - [ ] C2 Child done:false")),
		docAccept("10#list-rows: fields after the title in a block", checkedOf("- [ ] C1 Plan done:false weight:2")),
		noncanonicalDoc("10#list-rows: a declared column name before the title in a block", 7, checkedOf("- [ ] C1 weight:2 Plan done:false")),
		docAccept("10#list-rows: an escaped title token in a block", checkedOf(`- [ ] C1 Set weight\: review done:false`)),
		docAccept("20#order: siblings of a block in their line order", checkedOf("- [ ] C2 Two done:false", "- [ ] C1 One done:false")),
		docReject("20#order: the order column written as a field of a block row", RuleBlock, 7, checkedOf("- [ ] C1 Plan done:false position:1")),
		docReject("10#list-rows: a placement foreign key written as a field of a block row", RuleBlock, 7, checkedOf("- [ ] C1 Plan item:T1 done:false")),
		docReject("10#list-rows: a non-null field of a block row missing", RuleBlock, 7, checkedOf("- [ ] C1 Plan")),
		docAccept("10#detail-headings-and-anchors: a present empty section of a block row", checkedOf("- [x] C1 Plan done:true", "", "#### item_checks row C1", "", "##### evidence")),
		docAccept("10#document-order: a history block under a block row's detail section", checkedOf(
			"- [~] C1 Plan done:false", "", "#### item_checks row C1", "", "##### item_check_history",
			"| id | from_state | to_state | at |", "| --- | --- | --- | --- |",
			"| 1 | open | doing | 2026-10-01 08:00:00.000000 |")),
		docReject("10#document-order: a history block outside its block row's detail section", RuleBlock, 9, checkedOf(
			"- [~] C1 Plan done:false", "", "##### item_check_history", "| id | from_state | to_state | at |", "| --- | --- | --- | --- |")),
		docReject("10#detail-headings-and-anchors: a block row's detail names no row of its block", RuleBlock, 9, checkedOf(
			"- [x] C1 Plan done:true", "", "#### item_checks row C9", "", "x")),
	}
}

// checkedOf is the items file whose row T1 holds an item_checks block of the given
// lines: the block heading is line 6, so its first row is line 7.
func checkedOf(rows ...string) string {
	return itemsOf(append([]string{"- [ ] T1 Plan done:false", "", "## items row T1", "", "### item_checks"}, rows...)...)
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
				if c.readOnly {
					return
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
			if c.alone && len(diags) != 1 {
				t.Fatalf("%d diagnostics; want %s alone: %+v", len(diags), c.rule, diags)
			}
			if (len(rows) > 0) != c.kept {
				t.Fatalf("rows returned = %d; want kept %v", len(rows), c.kept)
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

// TestListTitleDecoding requires that the title of a list row reads each escaped
// title token `<column>\:` as `<column>:`, and keeps the white space between the
// title tokens byte for byte, a tab included.
func TestListTitleDecoding(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	cases := []struct {
		name, line, title string
	}{
		{"10#list-rows: an escaped title token decodes", "- [ ] T1 Set owner\\:x done:false", "Set owner:x"},
		{"10#list-rows: two spaces before an escaped title token are kept", "- [ ] T1 a  owner\\:x done:false", "a  owner:x"},
		{"10#list-rows: a tab inside a title before an escaped title token decodes", "- [ ] T1 Set\towner\\:x done:false", "Set\towner:x"},
		{"10#list-rows: a tab inside a title before an escaped column token decodes", "- [ ] T1 Plan\tnote\\:x done:false", "Plan\tnote:x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, diags, err := ParseDocument(cat, "items", "items.flowmark.md", []byte(itemsOf(c.line)), lk)
			if err != nil || len(diags) > 0 || len(rows) != 1 {
				t.Fatalf("read: %v %+v rows %d", err, diags, len(rows))
			}
			if rows[0].Title != c.title {
				t.Fatalf("title %q; want %q", rows[0].Title, c.title)
			}
		})
	}
}

// TestDetailTableNameCase requires the table name and `row` of a detail heading to
// compare as the viewers lowercase them: U+017F is not s, so a heading naming it is
// another table; U+212A lowercases to k, and a capital K is the table in lower case.
func TestDetailTableNameCase(t *testing.T) {
	cat, diags := catalog.Load("testdata/casefold")
	if len(diags) > 0 || cat == nil {
		t.Fatalf("casefold fixture catalog: %+v", diags)
	}
	lk := documentLinker()
	cases := []struct {
		name, table, heading, rule string
	}{
		{"10#detail-headings-and-anchors: a table name with a long s is another table", "stems", "ſtems row S1", RuleBlock},
		{"10#detail-headings-and-anchors: a table name with a Kelvin sign is noncanonical", "ktems", "Ktems row K1", RuleNoncanonical},
		{"10#detail-headings-and-anchors: a table name in capitals is noncanonical", "ktems", "Ktems row K1", RuleNoncanonical},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key := map[string]string{"stems": "S1", "ktems": "K1"}[c.table]
			text := raw("# "+c.table, "| id | title |", "| --- | --- |", "| "+key+" | Plan |", "", "## "+c.heading, "", "A.")
			_, diags, err := ParseDocument(cat, c.table, c.table+".flowmark.md", []byte(text), lk)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) == 0 || diags[0].Rule != c.rule {
				t.Fatalf("diagnostics %+v; want first rule %s", diags, c.rule)
			}
		})
	}
}

// TestH1LowercasedAsViewers requires that an H1 is compared with the table name as
// the viewers lowercase it: U+0130 lowercases to i and U+0307, so `# İtems` names
// no table of items.flowmark.md, and the file has no H1 and a stray heading.
func TestH1LowercasedAsViewers(t *testing.T) {
	cat := loadDocumentFixture(t)
	text := raw("# İtems", "- [ ] T1 Plan done:false")
	_, diags, err := ParseDocument(cat, "items", "items.flowmark.md", []byte(text), documentLinker())
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 || diags[0].Rule != RuleNoncanonical || diags[0].Line != 1 || diags[0].Column != 1 || !hasRule(diags, RuleBlock) {
		t.Fatalf("diagnostics %+v; want the missing H1 at 1:1 and format.block", diags)
	}
}

// TestSetextDetailHeading requires that a setext detail heading is located at the
// first character of its text, the first line of its text, and that the link
// reference definitions CommonMark 0.31.2 reads at the start of its paragraph are
// not part of its text (§4.7): they stand before it, in the content they end.
func TestSetextDetailHeading(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	two := []string{"- [ ] T1 Plan done:false", "- [ ] T2 Other done:false", ""}
	cases := []struct {
		name       string
		text       string
		rule       string
		line, col  int
		bodies     []string
		noBlockErr bool
	}{
		{"10#diagnostics: a setext detail heading is located at the first character of its text",
			itemsOf("- [ ] T1 Plan done:false", "", "items row T1", "------------", "", "A."), RuleNoncanonical, 4, 1, []string{"A."}, true},
		{"10#diagnostics: an indented setext detail heading is located at the first character of its text",
			itemsOf("- [ ] T1 Plan done:false", "", "  items row T1", "------------", "", "A."), RuleNoncanonical, 4, 3, []string{"A."}, true},
		{"10#detail-headings-and-anchors: a definition before a setext detail heading is not its text",
			itemsOf(append(two, "## items row T1", "", "[r]: /u", "items row T2", "------------", "", "B.")...), RuleNoncanonical, 8, 1, []string{"[r]: /u", "B."}, true},
		{"10#detail-headings-and-anchors: definitions over several lines before a setext detail heading are not its text",
			itemsOf(append(two, "## items row T1", "", "[r]:", "/u", "'t'", "[s]: /v", "items row T2", "------------", "", "B.")...), RuleNoncanonical, 11, 1, []string{"[r]:\n/u\n't'\n[s]: /v", "B."}, true},
		{"10#detail-headings-and-anchors: a definition right after the list is content before the setext detail heading",
			itemsOf("- [ ] T1 Plan done:false", "", "[r]: /u", "items row T1", "------------", "", "A."), RuleBlock, 4, 1, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, diags, err := ParseDocument(cat, "items", "items.flowmark.md", []byte(c.text), lk)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) == 0 || diags[0].Rule != c.rule || diags[0].Line != c.line || diags[0].Column != c.col {
				t.Fatalf("diagnostics %+v; want first %s at %d:%d", diags, c.rule, c.line, c.col)
			}
			if c.noBlockErr && hasRule(diags, RuleBlock) {
				t.Fatalf("diagnostics %+v; want no format.block", diags)
			}
			if c.bodies == nil {
				return
			}
			if len(rows) != len(c.bodies) {
				t.Fatalf("%d rows; want %d", len(rows), len(c.bodies))
			}
			for k, b := range c.bodies {
				if b += "\n"; rows[k].Body != b {
					t.Fatalf("row %s body %q; want %q", rows[k].Key, rows[k].Body, b)
				}
			}
			checkDocumentP2(t, cat, "items", rows, lk)
		})
	}
}

// TestDocumentSectionHeading requires that the faults of a section or block heading
// of a detail section are located at the first character of the heading text, at
// any level: a section of a block row's detail section is a level-5 heading.
func TestDocumentSectionHeading(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	cases := []struct {
		name, text, rule, message string
		line, col                 int
	}{
		{"10#diagnostics: an undeclared section heading is located at its text", itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "###  nope", "", "x"), RuleSection, "is not a section", 6, 6},
		{"10#diagnostics: a noncanonical section heading is located at its text", itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "###  evidence", "", "x"), RuleNoncanonical, "section heading", 6, 6},
		{"10#diagnostics: a section written twice is located at its text", itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "### evidence", "", "a", "", "###  evidence", "", "b"), RuleSection, "written twice", 10, 6},
		{"10#diagnostics: a noncanonical block heading is located at its text", itemsOf("- [ ] T1 Plan done:false", "", "## items row T1", "", "###  item_checks", "- [x] C1 Plan done:true"), RuleNoncanonical, "block heading", 6, 6},
		{"10#diagnostics: a section heading of a block row's detail section is located at its text", checkedOf("- [x] C1 Plan done:true", "", "#### item_checks row C1", "", "##### nope", "", "x"), RuleSection, "is not a section", 11, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags, err := ParseDocument(cat, "items", "items.flowmark.md", []byte(c.text), lk)
			if err != nil {
				t.Fatal(err)
			}
			if !hasDiagAt(diags, c.rule, c.message, c.line, c.col) {
				t.Fatalf("diagnostics %+v; want %s %q at %d:%d", diags, c.rule, c.message, c.line, c.col)
			}
		})
	}
}

// hasDiagAt reports whether diags hold a diagnostic of rule whose message contains
// message, at line and column.
func hasDiagAt(diags []Diagnostic, rule, message string, line, col int) bool {
	for _, d := range diags {
		if d.Rule == rule && d.Line == line && d.Column == col && strings.Contains(d.Message, message) {
			return true
		}
	}
	return false
}

// TestTableRowWithoutTrailingPipe requires that a row without its last pipe keeps
// its last cell whole, a final `\|` being a pipe of that cell, and that canon
// writes the row with its last pipe.
func TestTableRowWithoutTrailingPipe(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	cases := []struct{ name, in, want string }{
		{"10#table-rows: a row without its trailing pipe keeps its last cell", "| M1 |  | Foundation", milestoneRow},
		{"10#table-rows: a row without its trailing pipe keeps a final escaped pipe", `| M1 |  | Found\|`, `| M1 |  | Found\| |`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, _, err := ParseDocument(cat, "milestones", "milestones.flowmark.md", []byte(milestonesOf(milestoneHeader, milestoneDelim, c.in)), lk)
			if err != nil || len(rows) != 1 {
				t.Fatalf("read: %v, %d rows", err, len(rows))
			}
			out := checkDocumentP2(t, cat, "milestones", rows, lk)
			if want := milestonesOf(milestoneHeader, milestoneDelim, c.want); string(out) != want {
				t.Fatalf("canon writes %q; want %q", out, want)
			}
		})
	}
}

// TestOneColumnHeader requires that the header of a one-column table, which GFM
// reads with a pipe at either end or none, is noncanonical with its row kept.
func TestOneColumnHeader(t *testing.T) {
	cat, diags := catalog.Load("testdata/onecol")
	if len(diags) > 0 || cat == nil {
		t.Fatalf("onecol fixture catalog: %+v", diags)
	}
	lk := documentLinker()
	for _, header := range []string{"id |", "| id", "id"} {
		t.Run("10#table-rows: a one-column header "+header, func(t *testing.T) {
			text := raw("# tags", header, "| --- |", "| A |")
			rows, diags, err := ParseDocument(cat, "tags", "tags.flowmark.md", []byte(text), lk)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) == 0 || diags[0].Rule != RuleNoncanonical || diags[0].Line != 2 || diags[0].Column != 1 || hasRule(diags, RuleBlock) {
				t.Fatalf("diagnostics %+v; want format.noncanonical at 2:1 alone in rule", diags)
			}
			if len(rows) != 1 || rows[0].Key != "A" {
				t.Fatalf("rows %d; want the row A", len(rows))
			}
			if out := checkDocumentP2(t, cat, "tags", rows, lk); string(out) != raw("# tags", "| id |", "| --- |", "| A |") {
				t.Fatalf("canon writes %q", out)
			}
		})
	}
}

// TestListChildAtContentIndent requires that a row whose marker stands where a wider
// row's content begins, at an indentation to which the two-space grammar gives no
// place, is that row's child as CommonMark reads it, and is noncanonical; canon
// writes it two spaces deeper than its parent.
func TestListChildAtContentIndent(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	cases := []struct {
		name    string
		lines   []string
		parents map[string]string
	}{
		{"10#list-rows: a child at the content indent of a three-space run", []string{"-   [ ] T1 Plan done:false", "    - [ ] T2 Child done:false"},
			map[string]string{"T1": "", "T2": "T1"}},
		{"10#list-rows: a child at the content indent of a four-space run", []string{"-    [ ] T1 Plan done:false", "      - [ ] T2 Child done:false"},
			map[string]string{"T1": "", "T2": "T1"}},
		{"10#list-rows: a child at the content indent of a two-space run", []string{"-  [ ] T1 Plan done:false", "   - [ ] T2 Child done:false"},
			map[string]string{"T1": "", "T2": "T1"}},
		{"10#list-rows: two children at the content indent and a sibling after them", []string{"-   [ ] T1 Plan done:false", "    - [ ] T2 Child done:false", "    - [ ] T3 Child done:false", "- [ ] T4 Other done:false"},
			map[string]string{"T1": "", "T2": "T1", "T3": "T1", "T4": ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, diags, err := ParseDocument(cat, "items", "items.flowmark.md", []byte(itemsOf(c.lines...)), lk)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) == 0 || diags[0].Rule != RuleNoncanonical || hasRule(diags, RuleBlock) {
				t.Fatalf("diagnostics %+v; want format.noncanonical alone in rule", diags)
			}
			if len(rows) != len(c.parents) {
				t.Fatalf("%d rows; want %d", len(rows), len(c.parents))
			}
			for _, r := range rows {
				got := ""
				if v := r.Scalars["parent"]; v != nil {
					got = *v
				}
				if got != c.parents[r.Key] {
					t.Fatalf("row %s has parent %q; want %q", r.Key, got, c.parents[r.Key])
				}
			}
			checkDocumentP2(t, cat, "items", rows, lk)
		})
	}
}

// TestListChildUnderWideMarkerRun requires that a row indented two spaces under a
// row with two to four columns after its marker, a tab measured to the next multiple
// of four, is format.block at the child row:
// CommonMark reads it as a sibling, the grammar as the child.
func TestListChildUnderWideMarkerRun(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	for name, run := range map[string]string{"two spaces": "  ", "three spaces": "   ", "four spaces": "    ", "a tab": "\t"} {
		t.Run("10#list-rows: a child two spaces under a run of "+name, func(t *testing.T) {
			text := itemsOf("-"+run+"[ ] T1 Plan done:false", "  - [ ] T2 Child done:false")
			rows, diags, err := ParseDocument(cat, "items", "items.flowmark.md", []byte(text), lk)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range diags {
				if d.Rule == RuleBlock && d.Line == 3 && d.Column == 1 {
					found = true
				}
			}
			if len(rows) > 0 || !found {
				t.Fatalf("rows %d, diagnostics %+v; want format.block at 3:1 and no rows", len(rows), diags)
			}
		})
	}
}

// checkDocumentP2 requires that the rows write, that the written bytes parse
// back without a diagnostic, and that the rows write the same bytes again.
func checkDocumentP2(t *testing.T, cat *catalog.Catalog, table string, rows []*Row, lk Linker) []byte {
	t.Helper()
	out, rf, err := RenderDocument(cat, table, rows, lk)
	if err != nil || len(rf) > 0 {
		t.Fatalf("the rows are refused on write: %v %+v", err, rf)
	}
	back, diags, err := ParseDocument(cat, table, table+".flowmark.md", out, lk)
	if err != nil || len(diags) > 0 || back == nil {
		t.Fatalf("written file does not parse cleanly: %v %+v\n%s", err, diags, out)
	}
	again, rf, err := RenderDocument(cat, table, back, lk)
	if err != nil || len(rf) > 0 || !bytes.Equal(again, out) {
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
			out, rf, err := RenderDocument(cat, table, rows, lk)
			if err != nil || len(rf) > 0 || !bytes.Equal(out, data) {
				t.Fatalf("P1 failed: %v %+v\n got: %q\nwant: %q", err, rf, out, data)
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
	alone bool     // the refusal is the only diagnostic
	at    []locus  // the refusals in order, when their location is checked
	msgs  []string // a text that each refusal's message holds, in order, when checked
}

// locus is the location of a refusal of a write: the row by key, its index among the
// rows given (-1 unless its key is shared byte for byte), and the column it names.
type locus struct {
	key    string
	index  int
	column string
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
		{name: "10#diagnostics: a key shared byte for byte is refused at each later row, with its index", rows: []*Row{
			item("T1", "First", "waiting", "1", done), item("T1", "Second", "waiting", "2", done)},
			rule: RuleDuplicateKey, alone: true, at: []locus{{key: "T1", index: 1, column: "id"}}},
		{name: "10#diagnostics: a key that is equal only without regard to case refuses the greater key", rows: []*Row{
			item("T1", "First", "waiting", "1", done), item("t1", "Second", "waiting", "2", done)},
			rule: RuleDuplicateKey, alone: true, at: []locus{{key: "t1", index: -1, column: "id"}}},
		{name: "10#diagnostics: a case-insensitive pair refuses the same row in either request order", rows: []*Row{
			item("t1", "Second", "waiting", "2", done), item("T1", "First", "waiting", "1", done)},
			rule: RuleDuplicateKey, alone: true, at: []locus{{key: "t1", index: -1, column: "id"}}},
		{name: "10#diagnostics: a sibling order shared by two rows faults each row at order", rows: []*Row{
			item("T1", "A", "waiting", "1", done), item("T2", "B", "waiting", "1", done)},
			rule: RuleConstraint, alone: false, at: []locus{{key: "T1", index: -1, column: "position"}, {key: "T2", index: -1, column: "position"}}},
		{name: "10#diagnostics: a parent cycle faults each row on it at parent", rows: []*Row{
			withParent(item("T1", "A", "waiting", "1", done), "T2"), withParent(item("T2", "B", "waiting", "1", done), "T1")},
			rule: RuleConstraint, at: []locus{{key: "T1", index: -1, column: "parent"}, {key: "T2", index: -1, column: "parent"}}},
		{name: "10#diagnostics: a parent that names no row faults its row alone", rows: []*Row{
			withParent(item("T1", "A", "waiting", "1", done), "T9")},
			rule: RuleConstraint, alone: true, at: []locus{{key: "T1", index: -1, column: "parent"}}},
		{name: "10#diagnostics: a row under a parent that names no row has no cycle fault", rows: []*Row{
			withParent(item("T1", "A", "waiting", "1", done), "T9"), withParent(item("T2", "B", "waiting", "1", done), "T1")},
			rule: RuleConstraint, alone: true, at: []locus{{key: "T1", index: -1, column: "parent"}}},
		{name: "10#diagnostics: orders 1 and 3 fault only the row whose order is outside 1..2", rows: []*Row{
			item("T1", "A", "waiting", "1", done), item("T2", "B", "waiting", "3", done)},
			rule: RuleConstraint, alone: true, at: []locus{{key: "T2", index: -1, column: "position"}}},
		{name: "10#readable-values: a write of a foreign key whose key is not of its column type is CONSTRAINT alone", rows: []*Row{
			item("T1", "Plan", "waiting", "1", map[string]string{"done": "false", "owner": strings.Repeat("P", 40)})},
			rule: RuleConstraint, alone: true, at: []locus{{key: "T1", index: -1, column: "owner"}},
			msgs: []string{"owner is not a varchar(32) value"}},
		{name: "10#diagnostics: orders 1, 2, 3 are accepted", rows: []*Row{
			item("T1", "A", "waiting", "1", done), item("T2", "B", "waiting", "2", done), item("T3", "C", "waiting", "3", done)},
			out: "# items\n- [ ] T1 A done:false\n- [ ] T2 B done:false\n- [ ] T3 C done:false\n"},
		{name: "10#diagnostics: an order not of its type is the type fault alone and counts toward m", rows: []*Row{
			item("T1", "A", "waiting", "1", done), item("T2", "B", "waiting", "x", done), item("T3", "C", "waiting", "2", done)},
			rule: RuleConstraint, at: []locus{{key: "T2", index: -1, column: "position"}},
			msgs: []string{"position is not a i32 value"}},
		{name: "10#diagnostics: an i32 overflow order is the type fault alone", rows: []*Row{
			item("T1", "A", "waiting", "1", done), item("T2", "B", "waiting", "3000000000", done)},
			rule: RuleConstraint, at: []locus{{key: "T2", index: -1, column: "position"}},
			msgs: []string{"position is not a i32 value"}},
		{name: "10#diagnostics: an order not of its type leaves a shared value of its siblings faulted", rows: []*Row{
			item("T1", "A", "waiting", "1", done), item("T2", "B", "waiting", "1", done), item("T3", "C", "waiting", "x", done)},
			rule: RuleConstraint, at: []locus{{key: "T1", index: -1, column: "position"}, {key: "T2", index: -1, column: "position"}, {key: "T3", index: -1, column: "position"}},
			msgs: []string{"shared with a sibling", "shared with a sibling", "position is not a i32 value"}},
		{name: "10#diagnostics: an order not of its type counts toward m, so 4 is outside 1..3", rows: []*Row{
			item("T1", "A", "waiting", "1", done), item("T2", "B", "waiting", "4", done), item("T3", "C", "waiting", "x", done)},
			rule: RuleConstraint, at: []locus{{key: "T2", index: -1, column: "position"}, {key: "T3", index: -1, column: "position"}},
			msgs: []string{"outside 1..3", "position is not a i32 value"}},
		{name: "10#diagnostics: a cycle of T1 and T2 with T3 under T1 faults T1 and T2 and not T3", rows: []*Row{
			withParent(item("T1", "A", "waiting", "1", done), "T2"), withParent(item("T2", "B", "waiting", "1", done), "T1"),
			withParent(item("T3", "C", "waiting", "2", done), "T1")},
			rule: RuleConstraint, at: []locus{{key: "T1", index: -1, column: "parent"}, {key: "T2", index: -1, column: "parent"}}},
		{name: "10#diagnostics: a shared key whose rows also fault in order carries index 0 and 1 on every fault", rows: []*Row{
			item("T1", "First", "waiting", "x", done), item("T1", "Second", "waiting", "x", done)},
			rule: RuleDuplicateKey, at: []locus{{key: "T1", index: 1, column: "id"}, {key: "T1", index: 0, column: "position"}, {key: "T1", index: 1, column: "position"}}},
		{name: "10#diagnostics: a key that two blocks of one table share is DUPLICATE_KEY", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_checks", checkOf("C1", "A")),
			withBlock(item("T2", "Other", "waiting", "2", done), "item_checks", checkOf("C1", "B"))},
			rule: RuleDuplicateKey},
		{name: "10#diagnostics: keys equal without regard to case in two blocks of one table are DUPLICATE_KEY", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_checks", checkOf("C1", "A")),
			withBlock(item("T2", "Other", "waiting", "2", done), "item_checks", checkOf("c1", "B"))},
			rule: RuleDuplicateKey},
		{name: "10#diagnostics: keys equal without regard to case, each with a detail section, are DUPLICATE_KEY", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_checks", withBody(checkOf("C1", "A"), "Body A.\n")),
			withBlock(item("T2", "Other", "waiting", "2", done), "item_checks", withBody(checkOf("c1", "B"), "Body B.\n"))},
			rule: RuleDuplicateKey},
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
		{name: "10#list-rows: a title token after a tab is escaped", rows: []*Row{
			item("T1", "Plan\tnote:x", "waiting", "1", done)},
			out: "# items\n- [ ] T1 Plan\tnote\\:x done:false\n"},
		{name: "10#list-rows: a title token after two spaces is escaped", rows: []*Row{
			item("T1", "a  owner:x", "waiting", "1", done)},
			out: "# items\n- [ ] T1 a  owner\\:x done:false\n"},
		{name: "10#list-rows: a title token written with a tab and the escape already", rows: []*Row{
			item("T1", "Set\towner\\:x", "waiting", "1", done)},
			rule: RuleConstraint},
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
		{name: "10#readable-values: a temporal field assigned now is not of its type", rows: []*Row{
			item("T1", "Plan", "waiting", "1", map[string]string{"done": "false", "seen_on": "now"})}, rule: RuleConstraint},
		{name: "10#readable-values: a text field assigned now is text", rows: []*Row{
			item("T1", "Plan", "waiting", "1", withNote("now"))},
			out: "# items\n- [ ] T1 Plan done:false note:now\n"},
		{name: "10#readable-values: a temporal cell assigned now is not of its type", table: "people", rows: []*Row{
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
		{name: "20#order: siblings with the same value", rows: []*Row{
			item("T1", "Plan", "waiting", "1", done), item("T2", "Other", "waiting", "1", done)}, rule: RuleConstraint},
		{name: "20#order: a row without an order value", rows: []*Row{
			{Table: "items", Key: "T1", Title: "Plan", Scalars: map[string]*string{"state": sp("waiting"), "done": sp("false")},
				Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}},
			item("T2", "Other", "waiting", "1", done)}, rule: RuleConstraint},
		{name: "20#order: siblings with a gap", rows: []*Row{
			item("T1", "Plan", "waiting", "1", done), item("T2", "Other", "waiting", "3", done)}, rule: RuleConstraint},
		{name: "20#hierarchy: a parent that names no row", rows: []*Row{
			withParent(item("T1", "Plan", "waiting", "1", done), "T9")}, rule: RuleConstraint, alone: true},
		{name: "20#hierarchy: a parent cycle", rows: []*Row{
			withParent(item("T1", "Plan", "waiting", "1", done), "T2"),
			withParent(item("T2", "Other", "waiting", "1", done), "T1")}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a body without its final line feed", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "Text.")}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a body with a level-2 heading", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "## x\n")}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a body with a level-3 heading", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "### x\n")}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a body that begins with a blank line", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "\nText.\n")}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a body with a CR", rows: []*Row{
			withBody(item("T1", "Plan", "waiting", "1", done), "A\rB\n")}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a body of a table without a body column", table: "people", rows: []*Row{
			withBody(peopleRow("P1", "Ada", map[string]string{}), "Text.\n")}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a non-null section missing", table: "people", rows: []*Row{
			withoutSection(peopleRow("P1", "Ada", map[string]string{}), "bio")}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: an undeclared section", rows: []*Row{
			withSection(item("T1", "Plan", "waiting", "1", done), "nope", "x\n")}, rule: RuleConstraint},
		{name: "10#single-line-places: a title with an invisible character at its start", rows: []*Row{
			item("T1", "​Plan", "waiting", "1", done)}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a body whose open fence precedes a section", rows: []*Row{
			withSection(withBody(item("T1", "Plan", "waiting", "1", done), "```\ncode\n"), "cause", "x")}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a present empty section", rows: []*Row{
			withSection(item("T1", "Plan", "waiting", "1", done), "cause", "")},
			out: "# items\n- [ ] T1 Plan done:false\n\n## items row T1\n\n### cause\n"},
		{name: "20#hierarchy: a parent that names no row of its block", table: "people", rows: []*Row{
			peopleRow("P1", "Ada", map[string]string{"parent": "P9"})}, rule: RuleConstraint, alone: true},
		{name: "10#list-rows: a list without a state column writes no glyph", table: "notes", rows: []*Row{
			notePlan("N1", "", "Plan"), notePlan("N2", "N1", "Child")},
			out: "# notes\n- N1 Plan\n  - N2 Child\n"},
		{name: "10#document-order: the empty table is the H1 and one LF", table: "milestones", rows: nil,
			out: "# milestones\n"},
		{name: "10#document-order: a milestones table in its storage order", table: "milestones", rows: []*Row{
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
		{name: "10#detail-headings-and-anchors: a block row's section and its detail heading", rows: []*Row{
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
		{name: "20#hierarchy: a block row whose parent names no row of its block", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_notes",
				noteRow("N1", "N9", "A", "1", nil))}, rule: RuleConstraint, alone: true},
		{name: "20#hierarchy: a block row whose parent names a row of another block", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_steps", stepOf("S1", "")),
			withBlock(item("T2", "Other", "waiting", "2", done), "item_steps", stepOf("S2", "S1"))}, rule: RuleConstraint, alone: true},
		{name: "10#detail-headings-and-anchors: a block that the table does not declare", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_links",
				historyRow("1", "waiting", "doing", "2026-10-01 08:00:00.000000"))}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a block row of another table", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history", noteRow("N1", "", "A", "1", nil))}, rule: RuleConstraint},
		{name: "10#table-rows: a placement foreign key assigned to a block row", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history",
				withScalar(historyRow("1", "waiting", "doing", "2026-10-01 08:00:00.000000"), "item", "T2"))}, rule: RuleConstraint},
		{name: "10#detail-headings-and-anchors: a block row missing a non-null cell", rows: []*Row{
			withBlock(item("T1", "Plan", "waiting", "1", done), "item_history",
				withoutScalar(historyRow("1", "waiting", "doing", "2026-10-01 08:00:00.000000"), "to_state"))}, rule: RuleConstraint},
	}
}

// stepOf is a row of the item_steps block with its parent, empty for a root row.
func stepOf(key, parent string) *Row {
	sc := map[string]*string{}
	if parent != "" {
		sc["parent"] = sp(parent)
	}
	return &Row{Table: "item_steps", Key: key, Scalars: sc, Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
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

// checkOf is an item_checks row, a block of an items row, with its state, title, order and done flag.
func checkOf(key, title string) *Row {
	sc := map[string]*string{"state": sp("open"), "position": sp("1"), "done": sp("false")}
	return &Row{Table: "item_checks", Key: key, Title: title, Scalars: sc, Sections: map[string]*string{}, Raw: map[string]string{}, Dangling: map[string]DanglingLink{}}
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
				if c.alone && len(diags) != 1 {
					t.Fatalf("%d diagnostics; want %s alone: %+v", len(diags), c.rule, diags)
				}
				if c.at != nil {
					if len(diags) != len(c.at) {
						t.Fatalf("%d diagnostics %+v; want the %d locations %+v", len(diags), diags, len(c.at), c.at)
					}
					for k, want := range c.at {
						d := diags[k]
						if d.Table != table || d.Key != want.key || d.Index != want.index || d.Column != want.column {
							t.Fatalf("diagnostic %d is %s %s (index %d) column %q; want %s (index %d) column %q: %+v",
								k, d.Table, d.Key, d.Index, d.Column, want.key, want.index, want.column, d)
						}
					}
				}
				if c.msgs != nil {
					if len(diags) != len(c.msgs) {
						t.Fatalf("%d diagnostics %+v; want the %d messages %q", len(diags), diags, len(c.msgs), c.msgs)
					}
					for k, want := range c.msgs {
						if !strings.Contains(diags[k].Message, want) {
							t.Fatalf("diagnostic %d says %q; want it to hold %q", k, diags[k].Message, want)
						}
					}
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
