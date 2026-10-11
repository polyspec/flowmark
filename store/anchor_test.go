package store

import (
	"reflect"
	"strings"
	"testing"
)

// anchorTables are the detail levels of the tables a case file may hold: a detail
// heading of items is at level two, one of a block table of a files row at level three.
var anchorTables = map[string]int{"items": 2, "task_steps": 3, "people": 2}

// clashCase is one file of the collision rule: lines is the file, want the lines
// where a heading or an HTML tag collides with a detail anchor.
type clashCase struct {
	name  string
	lines []string
	want  []int
}

// clashCases are the files of the detail-anchor rule of spec 10 (Detail headings and
// anchors), in the order of the rule's sentences: the digit-run removal, the fold,
// the heading forms and the HTML id and name.
func clashCases() []clashCase {
	detail := "## items row T"
	return []clashCase{
		{"keys T1 and T10 never collide", []string{"## items row T1", "", "## items row T10"}, nil},
		{"payload items row T beside detail items row T-1 is refused", []string{"## items row T-1", "", "# items row T"}, []int{3}},
		{"payload Notes beside detail items row T-1 is accepted", []string{"## items row T-1", "", "# Notes"}, nil},
		{"payload differing from the detail by a final number is refused", []string{detail, "", "# items row T2"}, []int{3}},
		{"payload differing by a number that is not final is accepted", []string{detail, "", "# items 2 row T"}, nil},
		{"superscript two is removed by F_G", []string{"## items row T1", "", "# items row T²"}, []int{3}},
		{"roman numeral is removed by F_G", []string{"## items row xii", "", "# items row Ⅻ"}, nil},
		{"café folds to cafe", []string{"## items row cafe", "", "# Items row café"}, []int{3}},
		{"entity decodes to a letter", []string{"## items row cafe", "", "# items row caf&eacute;"}, []int{3}},
		{"unknown entity is text", []string{detail, "", "# items row &nope;T"}, nil},
		{"link text counts, link target does not", []string{"## items row T1", "", "# Items row [T](items)"}, []int{3}},
		{"a detail heading of another level is a payload heading", []string{detail, "", "### items row T"}, []int{3}},
		{"emphasis markup is not text", []string{detail, "", "## *Items* row **T**"}, []int{3}},
		{"code span keeps its content", []string{detail, "", "## `items row` T"}, []int{3}},
		{"raw HTML tag is not text", []string{detail, "", "## <b>Items</b> row T"}, []int{3}},
		{"heading in a list item takes the anchor", []string{"## items row T1", "", "- ## items row T"}, []int{3}},
		{"heading in a block quote takes the anchor", []string{detail, "", "> ## items row T1"}, []int{3}},
		{"block-quote heading alone is no detail and collides with nothing", []string{"> ## items row T1"}, nil},
		{"setext heading takes the anchor", []string{"## items row T1", "", "items-row T", "---"}, []int{3}},
		{"setext heading in a block quote takes the anchor", []string{"## items row T1", "", "> Items row T", "> ---"}, []int{3}},
		{"detail headings differing only in case are no anchor collision", []string{detail, "", "## items row t"}, nil},
		{"heading inside a fence is no heading", []string{detail, "", "```", "## items row T", "```"}, nil},
		{"heading inside an HTML block is no heading", []string{detail, "", "<div>", "## items row T", "</div>"}, nil},
		{"an HTML id in a paragraph", []string{detail, "", `<a id="items-row-t"></a>`}, []int{3}},
		{"an HTML name in a paragraph", []string{detail, "", "<a name='items-row-t'>x</a>"}, []int{3}},
		{"an HTML id compares without regard to case", []string{detail, "", "<span id=ITEMS-ROW-T>x</span>"}, []int{3}},
		{"an HTML id in an HTML block", []string{detail, "", `<div id="items-row-t">`, "x", "</div>"}, []int{3}},
		{"a data attribute is no id", []string{detail, "", "<div data-id=items-row-t>x</div>"}, nil},
		{"an id in a code span is no HTML", []string{detail, "", "`<a id=items-row-t>`"}, nil},
		{"an id in a fenced code block is no HTML", []string{detail, "", "```", `<a id="items-row-t"></a>`, "```"}, nil},
		{"an id in an indented code block is no HTML", []string{detail, "", `    <a id="items-row-t"></a>`}, nil},
		{"10#detail-headings-and-anchors: an id in a list item's indented code block is no HTML", []string{"## items row T1", "", "- x", "", "      <a id=\"items-row-t1\"></a>"}, nil},
		{"10#detail-headings-and-anchors: an escaped tag is text, not HTML", []string{"## items row T1", "", "\\<a id=\"items-row-t1\">"}, nil},
		{"10#detail-headings-and-anchors: an id in an unclosed fence is no HTML", []string{"## items row T1", "", "a", "", "```", "<a id=items-row-t1>"}, nil},
		{"an id in a comment is no HTML", []string{detail, "", "<!-- <a id=items-row-t> -->"}, nil},
		{"an HTML id in a heading", []string{detail, "", "## Title <a id=items-row-t></a>"}, []int{3}},
		{"a detail with no collision", []string{"## items row T1", "", "Notes.", "", "## items row T2"}, nil},
		{"10#detail-headings-and-anchors: an image's alt text counts under F_G", []string{"## items row T1", "", "## ![items row T1](x.png)"}, []int{3}},
		{"10#detail-headings-and-anchors: an image's alt text counts under F_G beside other text", []string{"## items row T1", "", "## ![items row](x.png) T1"}, []int{3}},
		{"10#detail-headings-and-anchors: an image counts under F_G by no text", []string{"## items row T1", "", "## ![Notes](x.png) items row T1"}, []int{3}},
		{"10#detail-headings-and-anchors: an image's destination is no text", []string{"## items row T1", "", "## ![Notes](items-row-t1.png)"}, nil},
		{"10#detail-headings-and-anchors: a brace id is literal text under CommonMark", []string{"## items row T1", "", "## Notes {#items-row-t1}"}, nil},
		{"10#detail-headings-and-anchors: a full reference to a defined label counts by its link text", []string{"## items row T1", "", "## [items row T1][r]", "", "[r]: /u"}, []int{3}},
		{"10#detail-headings-and-anchors: a full reference without its definition is literal text", []string{"## items row T1", "", "## [items row T1][r]"}, nil},
		{"10#detail-headings-and-anchors: a definition with a junk tail is no definition, so its heading takes the anchor", []string{"## items row T1", "", "## [items row][T1]", "", "[T1]: /u junk"}, []int{3}},
		{"10#detail-headings-and-anchors: a definition with a junk tail after its title is no definition", []string{"## items row T1", "", "## [items row T1][r]", "", `[r]: /u "t" junk`}, nil},
		{"10#detail-headings-and-anchors: a definition whose destination is on the next line defines its label", []string{"## items row T1", "", "## [items row T1][r]", "", "[r]:", "/u"}, []int{3}},
		{"10#detail-headings-and-anchors: a setext heading keeps a definition line that is not at its start", []string{"## items row Tx", "", "items row", "[t]: x", "---"}, []int{3}},
		{"10#detail-headings-and-anchors: an HTML id after a definition-like line of a paragraph is read", []string{"## items row T1", "", "text", "[r]: /u '<a id=items-row-t1>'"}, []int{4}},
		{"10#detail-headings-and-anchors: a definition in a block quote defines the label", []string{"## items row T1", "", "## [items row T1][r]", "", "> [r]: /u"}, []int{3}},
		{"10#detail-headings-and-anchors: a definition in a code block defines nothing", []string{"## items row T1", "", "## [items row T1][r]", "", "```", "[r]: /u", "```"}, nil},
		{"10#detail-headings-and-anchors: a label matches after case and white space are folded", []string{"## items row T1", "", "## [Items row T1][ R ]", "", "[r]: /u"}, []int{3}},
		{"10#detail-headings-and-anchors: a detail heading whose table name differs in case is a detail heading", []string{"## Items row T1", "", "## items row T1"}, nil},
		{"10#detail-headings-and-anchors: a no-break space between words is no separator of a heading", []string{"## items row T1", "", "## items row T1"}, []int{3}},
		{"10#detail-headings-and-anchors: a next-line character between words is no separator of a heading", []string{"## items row T1", "", "## items\u0085row T1"}, []int{3}},
	}
}

// TestAnchorClashes runs the collision rule on the files of clashCases.
func TestAnchorClashes(t *testing.T) {
	for _, c := range clashCases() {
		t.Run(c.name, func(t *testing.T) {
			var got []int
			for _, cl := range anchorClashes(splitLines(joinLines(c.lines)), anchorTables) {
				got = append(got, cl.line)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("clash lines = %v; want %v", got, c.want)
			}
		})
	}
}

// TestAnchorClashNamesOther requires a clash to name the detail heading whose anchor
// it takes, so the diagnostic of a write or a read names the other heading.
func TestAnchorClashNamesOther(t *testing.T) {
	lines := splitLines("## items row T-1\n\n# items row T\n")
	clashes := anchorClashes(lines, anchorTables)
	if len(clashes) != 1 {
		t.Fatalf("clashes = %+v; want one", clashes)
	}
	if clashes[0].other != "items row T-1" || clashes[0].text != "items row T" {
		t.Fatalf("clash = %+v; want the payload heading naming items row T-1", clashes[0])
	}
}

// TestDetailHeadingSpellings requires the spellings of a detail heading of the people table
// to be read as spec 10 states: the spaces, tabs and line endings separate its words, and
// a setext spelling, a tab or a case of the table name is noncanonical, while another white
// space character between the words makes the heading a block that is no detail heading.
func TestDetailHeadingSpellings(t *testing.T) {
	docs := loadDocumentFixture(t)
	lk := fixtureLinker()
	doc := func(heading string) string {
		return "# people\n" + peopleHeader + "\n" + peopleDelim + "\n" +
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", "") + "\n\n" + heading + "\n\nNotes.\n"
	}
	cases := []struct {
		name, heading string
		rule          string
	}{
		{"10#detail-headings-and-anchors: a single-line setext detail heading is noncanonical", "people row P1\n---", RuleNoncanonical},
		{"10#detail-headings-and-anchors: a two-line setext detail heading is noncanonical", "people\nrow P1\n---", RuleNoncanonical},
		{"10#detail-headings-and-anchors: a tab between the words of a detail heading is noncanonical", "## people\trow P1", RuleNoncanonical},
		{"10#detail-headings-and-anchors: a run of spaces before the text of a detail heading is noncanonical", "##  people row P1", RuleNoncanonical},
		{"10#detail-headings-and-anchors: a run of spaces between the words of a detail heading is noncanonical", "## people  row P1", RuleNoncanonical},
		{"10#detail-headings-and-anchors: a space after the text of a detail heading is noncanonical", "## people row P1  ", RuleNoncanonical},
		{"10#detail-headings-and-anchors: a case of the table name is noncanonical", "## People row P1", RuleNoncanonical},
		{"10#detail-headings-and-anchors: a no-break space between words of a detail heading is a block", "## people row P1", RuleBlock},
		{"10#detail-headings-and-anchors: a next-line character between words of a detail heading is a block", "## people\u0085row P1", RuleBlock},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags, err := ParseDocument(docs, "people", "people.flowmark.md", []byte(doc(c.heading)), lk)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) == 0 || diags[0].Rule != c.rule {
				t.Fatalf("diagnostics %+v; want %s first", diags, c.rule)
			}
		})
	}
}

// TestAnchorLocations requires a read to locate a collision at the element that collides:
// a heading at the first character of its text, and an HTML id or name at the '<' of its
// tag, each with its line and its column in code points.
func TestAnchorLocations(t *testing.T) {
	docs := loadDocumentFixture(t)
	lk := fixtureLinker()
	// The detail section of P1 opens at line 6 and its bio section at line 8, so the body
	// begins at line 10.
	doc := func(body string) string {
		return "# people\n" + peopleHeader + "\n" + peopleDelim + "\n" +
			row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", "") +
			"\n\n## people row P1\n\n### bio\n\n" + body
	}
	cases := []struct {
		name, body string
		line, col  int
	}{
		{"10#detail-headings-and-anchors: a read locates a heading at the first character of its text", "#### people row P\n\nx\n", 10, 6},
		{"10#detail-headings-and-anchors: a read locates an HTML id of a paragraph at its tag", "x\ny\n  <span id=\"people-row-p1\">z</span>\n", 12, 3},
		{"10#detail-headings-and-anchors: a read locates an HTML id of an HTML block at its tag", "<div>\nx\n  <div id=\"people-row-p1\">\n", 12, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags, err := ParseDocument(docs, "people", "people.flowmark.md", []byte(doc(c.body)), lk)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) == 0 || diags[0].Rule != RuleBlock {
				t.Fatalf("diagnostics %+v; want %s first", diags, RuleBlock)
			}
			if diags[0].Line != c.line || diags[0].Column != c.col {
				t.Fatalf("located at %d:%d; want %d:%d: %+v", diags[0].Line, diags[0].Column, c.line, c.col, diags[0])
			}
		})
	}
}

// located is a fault of either codec: a read's rule and line, or a write's rule and
// the column of the row it refuses.
type located struct {
	rule   string
	line   int
	column string
}

func readLocated(diags []Diagnostic) []located {
	out := make([]located, 0, len(diags))
	for _, d := range diags {
		out = append(out, located{rule: d.Rule, line: d.Line})
	}
	return out
}

func writeLocated(rf []Refusal) []located {
	out := make([]located, 0, len(rf))
	for _, r := range rf {
		out = append(out, located{rule: r.Rule, column: r.Column})
	}
	return out
}

// anchorCodecCase is one codec input of the collision rule: the read of a file or
// document through a parser, or the write of a row set through a renderer, with the
// rule its first fault carries, and the line of a read's fault or the column of a
// write's.
type anchorCodecCase struct {
	name   string
	run    func(t *testing.T) []located
	rule   string
	line   int
	column string
	clean  bool // the input is accepted
}

// TestAnchorCodecs requires both codecs to refuse a collision: a read as format.block
// and a write as CONSTRAINT, in a files row file and in a document file, and a
// write of a duplicate key as DUPLICATE_KEY, since that check comes first.
func TestAnchorCodecs(t *testing.T) {
	cat := loadFixture(t)
	docs := loadDocumentFixture(t)
	lk := fixtureLinker()
	readFile := func(text string) func(t *testing.T) []located {
		return func(t *testing.T) []located {
			_, diags := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", []byte(text), lk)
			return readLocated(diags)
		}
	}
	writeFile := func(row *Row) func(t *testing.T) []located {
		return func(t *testing.T) []located {
			_, rf := renderFileOf(t, cat, "tasks", row, lk)
			return writeLocated(rf)
		}
	}
	readDoc := func(text string) func(t *testing.T) []located {
		return func(t *testing.T) []located {
			_, diags, err := ParseDocument(docs, "people", "people.flowmark.md", []byte(text), lk)
			if err != nil {
				t.Fatal(err)
			}
			return readLocated(diags)
		}
	}
	writeDoc := func(rows []*Row) func(t *testing.T) []located {
		return func(t *testing.T) []located {
			_, rf, err := RenderDocument(docs, "people", rows, lk)
			if err != nil {
				t.Fatal(err)
			}
			return writeLocated(rf)
		}
	}
	steps := func(title string) *Row {
		r := taskWithBlocks(map[string][]*Row{"task_steps": {stepRow("S1", "Draft", "1", "true", "Step body.\n", nil)}})
		r.Title = title
		return r
	}
	personBio := func(bio string) *Row {
		r := peopleRow("P1", "Ada", nil)
		r.Sections["bio"] = sp(bio)
		return r
	}
	const stepsFile = "# Title\n- position: 1\n- done: true\n\n## task_steps\n- S1 Draft done:true\n\n### task_steps row S1\n\nStep body.\n"
	cases := []anchorCodecCase{
		{name: "files read: title takes the anchor of a child detail", run: readFile(strings.Replace(stepsFile, "# Title", "# task steps row S1", 1)), rule: RuleBlock, line: 1},
		{name: "files read: canonical file with a child detail is accepted", run: readFile(stepsFile), clean: true},
		{name: "files write: title takes the anchor of a child detail", run: writeFile(steps("task steps row S1")), rule: RuleConstraint, column: "title"},
		{name: "files write: canonical row is accepted", run: writeFile(steps("Title")), clean: true},
		{name: "document read: a heading in a detail section takes the anchor", run: readDoc("# people\n" +
			peopleHeader + "\n" + peopleDelim + "\n" + row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", "") + "\n" +
			"\n## people row P1\n\n### bio\n\n#### people row P\n\nx\n"), rule: RuleBlock, line: 10},
		{name: "document read: a canonical detail section is accepted", run: readDoc("# people\n" +
			peopleHeader + "\n" + peopleDelim + "\n" + row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", "") + "\n" +
			"\n## people row P1\n\n### bio\n\nNotes.\n"), clean: true},
		{name: "document write: a heading in a detail section takes the anchor", run: writeDoc([]*Row{personBio("#### people row P\n\nx\n")}), rule: RuleConstraint, column: "bio"},
		{name: "document write: a duplicate key is DUPLICATE_KEY before the collision", run: writeDoc([]*Row{
			peopleRow("P1", "Ada", nil), peopleRow("p1", "Ben", nil)}), rule: RuleDuplicateKey},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := c.run(t)
			if c.clean {
				if len(diags) > 0 {
					t.Fatalf("accepted input has diagnostics: %+v", diags)
				}
				return
			}
			if len(diags) == 0 {
				t.Fatalf("no diagnostic; want %s", c.rule)
			}
			if diags[0].rule != c.rule {
				t.Fatalf("diagnostic %+v; want rule %s", diags[0], c.rule)
			}
			if c.line != 0 && diags[0].line != c.line {
				t.Fatalf("first diagnostic at line %d; want %d: %+v", diags[0].line, c.line, diags[0])
			}
			if c.column != "" && diags[0].column != c.column {
				t.Fatalf("first diagnostic at column %q; want %q: %+v", diags[0].column, c.column, diags[0])
			}
		})
	}
}
