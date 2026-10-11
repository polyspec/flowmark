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
// anchors), in the order of the rule's sentences: the numbering rule, the slugs, the
// heading forms and the HTML id and name.
func clashCases() []clashCase {
	detail := "## items row T"
	d1 := "## items row T1"
	return []clashCase{
		{"keys T1 and T10 never collide", []string{"## items row T1", "", "## items row T10"}, nil},
		{"payload items row T beside detail items row T-1 is refused", []string{"## items row T-1", "", "# items row T"}, []int{3}},
		{"payload Notes beside detail items row T-1 is accepted", []string{"## items row T-1", "", "# Notes"}, nil},
		{"10#detail-headings-and-anchors: a payload differing from the detail by a final number is accepted", []string{detail, "", "# items row T2"}, nil},
		{"10#detail-headings-and-anchors: a number with a leading zero is no numbering", []string{"## items row T-01", "", "# items row T"}, nil},
		{"10#detail-headings-and-anchors: zero is no numbering", []string{"## items row T-0", "", "# items row T"}, nil},
		{"10#detail-headings-and-anchors: a number of two digits is a numbering", []string{"## items row T-12", "", "# items row T"}, []int{3}},
		{"payload differing by a number that is not final is accepted", []string{detail, "", "# items 2 row T"}, nil},
		{"10#detail-headings-and-anchors: a superscript two at the end is no numbering", []string{d1, "", "# items row T²"}, nil},
		{"10#detail-headings-and-anchors: a superscript two is removed by S_GH", []string{d1, "", "# items row T²1"}, []int{3}},
		{"10#detail-headings-and-anchors: a roman numeral is kept, as a letter, by every slug", []string{d1, "", "# items row T1Ⅻ"}, nil},
		{"10#detail-headings-and-anchors: a roman numeral is no ASCII letter", []string{"## items row xii", "", "# items row Ⅻ"}, nil},
		{"10#detail-headings-and-anchors: an accented letter is kept", []string{"## items row cafe", "", "# Items row café"}, nil},
		{"10#detail-headings-and-anchors: a combining mark is kept", []string{d1, "", "# items row T\U000003011"}, nil},
		{"10#detail-headings-and-anchors: a circled letter is kept by S_GH as Alphabetic and an ideograph by S_GL", []string{d1, "", "# items row T1Ⓐ日本"}, nil},
		{"10#detail-headings-and-anchors: a circled letter is removed by S_GL", []string{d1, "", "# items row TⒶ1"}, []int{3}},
		{"10#detail-headings-and-anchors: a dotted capital I lowercases to i and a combining dot", []string{d1, "", "# İtems row T1"}, nil},
		{"10#detail-headings-and-anchors: the Kelvin sign lowercases to k", []string{"## items row k1", "", "# items row \U0000212A1"}, []int{3}},
		{"10#detail-headings-and-anchors: spaces are hyphens in every slug", []string{d1, "", "# itemsrowT1"}, nil},
		{"10#detail-headings-and-anchors: an underscore is kept", []string{d1, "", "# items_row_T1"}, nil},
		{"10#detail-headings-and-anchors: two spaces are two hyphens", []string{d1, "", "# items row  T1"}, nil},
		{"10#detail-headings-and-anchors: a removed character leaves its space", []string{d1, "", "# items row T1 !"}, nil},
		{"10#detail-headings-and-anchors: a hyphen is kept", []string{d1, "", "# items-row-T1"}, []int{3}},
		{"10#detail-headings-and-anchors: case is lowered", []string{d1, "", "# Items Row T1"}, []int{3}},
		{"10#detail-headings-and-anchors: a letter that VS Code's expression removes is removed by S_VS", []string{d1, "", "# items\U000107a5 row T1"}, []int{3}},
		{"10#detail-headings-and-anchors: a digit that VS Code's expression removes is removed by S_VS", []string{d1, "", "# items row\U0001ccf0 T1"}, []int{3}},
		{"10#detail-headings-and-anchors: a modifier letter that VS Code's expression removes is removed by S_VS", []string{d1, "", "# items\U0000A7F2 row T1"}, []int{3}},
		{"10#detail-headings-and-anchors: a zero width joiner is removed by S_VS", []string{d1, "", "# items‍ row T1"}, []int{3}},
		{"10#detail-headings-and-anchors: a zero width space is removed by S_GH", []string{d1, "", "# items row​ T1"}, []int{3}},
		{"10#detail-headings-and-anchors: an entity decodes to its character", []string{d1, "", "# items row &#84;1"}, []int{3}},
		{"10#detail-headings-and-anchors: an entity decodes to an accented letter", []string{"## items row cafe", "", "# items row caf&eacute;"}, nil},
		{"unknown entity is text", []string{detail, "", "# items row &nope;T"}, nil},
		{"10#detail-headings-and-anchors: link text counts, link target does not", []string{d1, "", "# Items row [T1](items)"}, []int{3}},
		{"a detail heading of another level is a payload heading", []string{detail, "", "### items row T"}, []int{3}},
		{"emphasis markup is not text", []string{detail, "", "## *Items* row **T**"}, []int{3}},
		{"10#detail-headings-and-anchors: underscore emphasis is not text", []string{d1, "", "# items row _T1_"}, []int{3}},
		{"10#detail-headings-and-anchors: an underscore that opens no emphasis is text", []string{d1, "", "# items row _T1"}, nil},
		{"10#detail-headings-and-anchors: intraword underscores open no emphasis", []string{"## items row T_1_", "", "# items row T_1_"}, []int{3}},
		{"10#detail-headings-and-anchors: an escaped underscore is the underscore", []string{"## items row T_1", "", "# items row T\\_1"}, []int{3}},
		{"10#detail-headings-and-anchors: an escaped underscore is not removed", []string{d1, "", "# items row T\\_1"}, nil},
		{"code span keeps its content", []string{detail, "", "## `items row` T"}, []int{3}},
		{"10#detail-headings-and-anchors: a code span strips one space at each end", []string{d1, "", "## items row ` T1 `"}, []int{3}},
		{"10#detail-headings-and-anchors: a code span of one space keeps it", []string{d1, "", "## items row` `T1"}, []int{3}},
		{"10#detail-headings-and-anchors: an image's alt text drops its emphasis", []string{d1, "", "# items row ![_T1_]()"}, []int{3}},
		{"raw HTML tag is not text", []string{detail, "", "## <b>Items</b> row T"}, []int{3}},
		{"10#detail-headings-and-anchors: heading in a list item takes the anchor", []string{d1, "", "- ## items row T1"}, []int{3}},
		{"heading in a block quote takes the anchor", []string{d1, "", "> ## items row T1"}, []int{3}},
		{"block-quote heading alone is no detail and collides with nothing", []string{"> ## items row T1"}, nil},
		{"10#detail-headings-and-anchors: setext heading takes the anchor", []string{d1, "", "items-row T1", "---"}, []int{3}},
		{"10#detail-headings-and-anchors: setext heading in a block quote takes the anchor", []string{d1, "", "> Items row T1", "> ---"}, []int{3}},
		{"10#detail-headings-and-anchors: a line break is a space under S_GL", []string{d1, "", "items row", "T1", "==="}, []int{3}},
		{"10#detail-headings-and-anchors: a line break is nothing under S_GH", []string{d1, "", "items row-", "T1", "==="}, []int{3}},
		{"10#detail-headings-and-anchors: a hard line break is a break", []string{d1, "", "items row-\\", "T1", "==="}, []int{3}},
		{"10#detail-headings-and-anchors: spaces before a line break are no text", []string{d1, "", "items row-  ", "T1", "==="}, []int{3}},
		{"10#detail-headings-and-anchors: S_VS trims the text first", []string{d1, "", "## &#32;items row T1&#32;"}, []int{3}},
		{"10#detail-headings-and-anchors: S_VS trims a code span's spaces", []string{d1, "", "## `  items row T1  `"}, []int{3}},
		{"detail headings differing only in case are no anchor collision", []string{detail, "", "## items row t"}, nil},
		{"heading inside a fence is no heading", []string{detail, "", "```", "## items row T", "```"}, nil},
		{"heading inside an HTML block is no heading", []string{detail, "", "<div>", "## items row T", "</div>"}, nil},
		{"an HTML id in a paragraph", []string{detail, "", `<a id="items-row-t"></a>`}, []int{3}},
		{"an HTML name in a paragraph", []string{detail, "", "<a name='items-row-t'>x</a>"}, []int{3}},
		{"10#detail-headings-and-anchors: an HTML id compares once lowercased", []string{detail, "", "<span id=ITEMS-ROW-T>x</span>"}, []int{3}},
		{"10#detail-headings-and-anchors: an HTML id is not case-folded, so a long s is no s", []string{"## items row S1", "", `<a id="items-row-ſ1"></a>`}, nil},
		{"10#detail-headings-and-anchors: an HTML id of the user-content- form takes the anchor", []string{d1, "", `<a id="user-content-items-row-t1"></a>`}, []int{3}},
		{"10#detail-headings-and-anchors: an HTML name of the user-content- form compares once lowercased", []string{d1, "", `<a name="User-Content-Items-Row-T1"></a>`}, []int{3}},
		{"10#detail-headings-and-anchors: an HTML id with another prefix is no anchor", []string{d1, "", `<a id="x-items-row-t1"></a>`}, nil},
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
		{"10#detail-headings-and-anchors: an image's alt text counts under the slugs", []string{"## items row T1", "", "## ![items row T1](x.png)"}, []int{3}},
		{"10#detail-headings-and-anchors: an image's alt text counts under the slugs beside other text", []string{"## items row T1", "", "## ![items row](x.png) T1"}, []int{3}},
		{"10#detail-headings-and-anchors: an image is no text under S_GH", []string{"## items row T1", "", "# items row ![Notes](x.png)T1"}, []int{3}},
		{"10#detail-headings-and-anchors: an image is its alt text under S_VS and S_GL, no text under S_GH, so a space before it stays", []string{"## items row T1", "", "## ![Notes](x.png) items row T1"}, nil},
		{"10#detail-headings-and-anchors: an image's destination is no text", []string{"## items row T1", "", "## ![Notes](items-row-t1.png)"}, nil},
		{"10#detail-headings-and-anchors: a brace id is literal text under CommonMark", []string{"## items row T1", "", "## Notes {#items-row-t1}"}, nil},
		{"10#detail-headings-and-anchors: a full reference to a defined label counts by its link text", []string{"## items row T1", "", "## [items row T1][r]", "", "[r]: /u"}, []int{3}},
		{"10#detail-headings-and-anchors: a full reference without its definition is literal text", []string{"## items row T1", "", "## [items row T1][r]"}, nil},
		{"10#detail-headings-and-anchors: a definition with a junk tail is no definition, so its reference is literal text", []string{"## items row T1", "", "## [items row T1][r]", "", "[r]: /u junk"}, nil},
		{"10#detail-headings-and-anchors: a definition with a junk tail after its title is no definition", []string{"## items row T1", "", "## [items row T1][r]", "", `[r]: /u "t" junk`}, nil},
		{"10#detail-headings-and-anchors: a definition whose destination is on the next line defines its label", []string{"## items row T1", "", "## [items row T1][r]", "", "[r]:", "/u"}, []int{3}},
		{"10#detail-headings-and-anchors: a setext heading keeps a definition line that is not at its start", []string{"## items row T-x", "", "items row", "[t]: x", "---"}, []int{3}},
		{"10#detail-headings-and-anchors: an HTML id after a definition-like line of a paragraph is read", []string{"## items row T1", "", "text", "[r]: /u '<a id=items-row-t1>'"}, []int{4}},
		{"10#detail-headings-and-anchors: a definition in a block quote defines the label", []string{"## items row T1", "", "## [items row T1][r]", "", "> [r]: /u"}, []int{3}},
		{"10#detail-headings-and-anchors: a definition in a code block defines nothing", []string{"## items row T1", "", "## [items row T1][r]", "", "```", "[r]: /u", "```"}, nil},
		{"10#detail-headings-and-anchors: a label matches after case and white space are folded", []string{"## items row T1", "", "## [Items row T1][ R ]", "", "[r]: /u"}, []int{3}},
		{"10#detail-headings-and-anchors: a detail heading whose table name differs in case is a detail heading", []string{"## Items row T1", "", "## items row T1"}, nil},
		{"10#detail-headings-and-anchors: a no-break space between words is no separator of a heading, and every slug removes it", []string{"## items row T1", "", "## items row T1"}, nil},
		{"10#detail-headings-and-anchors: a next-line character between words is no separator of a heading, and every slug removes it", []string{"## items row T1", "", "## items\u0085row T1"}, nil},
		{"10#detail-headings-and-anchors: a no-break space beside a space is removed by every slug", []string{"## items row T1", "", "## items \U000000A0row T1"}, []int{3}},
		{"10#detail-headings-and-anchors: GitLab reads a math span's literal source, a tag in it included", []string{"## items row Tb1", "", "# items row $T<b>1$"}, []int{3}},
		{"10#detail-headings-and-anchors: GitLab reads code math's literal source", []string{"## items row T1", "", "# items row $`T1`$ x"}, nil},
		{"10#detail-headings-and-anchors: a math span in an image's alt text is its source under S_GL and nothing under S_VS", []string{"## items row T1", "", "# items row ![$&#84;1$](/i.png)"}, nil},
		{"10#detail-headings-and-anchors: VS Code reads a math span as no text", []string{"## items row T1", "", "# items row T1 $x$"}, []int{3}},
		{"10#detail-headings-and-anchors: VS Code reads display math as no text", []string{"## items row T1", "", "# items row T1 $$x$$"}, []int{3}},
		{"10#detail-headings-and-anchors: a '$' after a word character opens no math in VS Code", []string{"## items row T1", "", "# items row T1x$y$"}, nil},
		{"10#detail-headings-and-anchors: VS Code opens no math right after an inline HTML tag its guard matches", []string{"## items row T1", "", "# items row T1<span>$x$"}, nil},
		{"10#detail-headings-and-anchors: a delimiter run ends VS Code's guard", []string{"## items row T1", "", "# items row T1<span>*$x$"}, []int{3}},
		{"10#detail-headings-and-anchors: a short tag is no guard of VS Code", []string{"## items row T1", "", "# items row T1<b>$x$"}, []int{3}},
		{"10#detail-headings-and-anchors: GitLab reads a defined footnote reference as no text", []string{"## items row T-1", "", "# items row T[^1]", "", "[^1]: n"}, []int{3}},
		{"10#detail-headings-and-anchors: GitLab restores an undefined footnote reference with its raw HTML as text", []string{"## items row Tabc", "", "# items row T[^a<b>c]"}, []int{3}},
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
		{"10#detail-headings-and-anchors: a read locates a heading at the first character of its text", "#### people row P1\n\nx\n", 10, 6},
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
		{name: "files read: title takes the anchor of a child detail", run: readFile(strings.Replace(stepsFile, "# Title", "# task_steps row S1", 1)), rule: RuleBlock, line: 1},
		{name: "files read: canonical file with a child detail is accepted", run: readFile(stepsFile), clean: true},
		{name: "files write: title takes the anchor of a child detail", run: writeFile(steps("task_steps row S1")), rule: RuleConstraint, column: "title"},
		{name: "files write: canonical row is accepted", run: writeFile(steps("Title")), clean: true},
		{name: "document read: a heading in a detail section takes the anchor", run: readDoc("# people\n" +
			peopleHeader + "\n" + peopleDelim + "\n" + row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", "") + "\n" +
			"\n## people row P1\n\n### bio\n\n#### people row P1\n\nx\n"), rule: RuleBlock, line: 10},
		{name: "document read: a canonical detail section is accepted", run: readDoc("# people\n" +
			peopleHeader + "\n" + peopleDelim + "\n" + row("P1", "", "Ada", "lead", "true", "", "", "", "", "", "", "", "", "") + "\n" +
			"\n## people row P1\n\n### bio\n\nNotes.\n"), clean: true},
		{name: "document write: a heading in a detail section takes the anchor", run: writeDoc([]*Row{personBio("#### people row P1\n\nx\n")}), rule: RuleConstraint, column: "bio"},
		{name: "document write: a duplicate key is DUPLICATE_KEY before the collision", run: writeDoc([]*Row{
			peopleRow("P1", "Ada", nil), peopleRow("P1", "Ben", nil)}), rule: RuleDuplicateKey},
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
