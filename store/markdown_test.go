package store

import (
	"fmt"
	"testing"
)

// TestHTMLStart pins the HTML block start conditions of CommonMark 0.31.2 to the
// types that commonmark.js 0.31.2 reads at the start of a line (its
// _htmlBlockType); 0 is a line that starts no HTML block.
func TestHTMLStart(t *testing.T) {
	cases := []struct {
		line string
		want int
	}{
		// Types 1 to 5: an open raw element, a comment, a processing instruction, a declaration, CDATA.
		{"<script>", 1}, {"<script", 1}, {"<script\t>", 1}, {"<pre>", 1}, {"<style x>", 1}, {"<textarea>", 1},
		{"<scripts>", 7}, {"<textarea/>", 7},
		{"<!-- x -->", 2}, {"<!-- x", 2}, {"<?php", 3},
		{"<!DOCTYPE html>", 4}, {"<!doctype>", 4}, {"<!a", 4}, {"<!1", 0},
		{"<![CDATA[x", 5},
		// Type 6: the names of the block elements, opened or closed, followed by a space, a tab,
		// the end of the line, '>' or '/>'; the name is case-insensitive.
		{"<div>", 6}, {"<div class=\"x\">", 6}, {"<div/>", 6}, {"<div>x", 6}, {"<div", 6},
		{"<DIV>", 6}, {"</div>", 6}, {"</div >", 6}, {"<div\t>", 6}, {"<div >", 6},
		{"  <div>", 6}, {"<p>", 6}, {"<h6>", 6}, {"<search>", 6}, {"<frameset>", 6}, {"<center>", 6},
		{"<h7>", 7}, {"<divx>", 7}, {"<div-x>", 7}, {"<source>", 7},
		// Type 7: one complete open or closing tag, then white space to the end of the line.
		{"<span>", 7}, {"<span class=\"a\">", 7}, {"<span a=b>", 7}, {"<span a='b c'>", 7},
		{"<span a=\"b\">", 7}, {"<span a = b >", 7}, {"<span a=b/>", 7}, {"<span/>", 7},
		{"</span>", 7}, {"</span >", 7}, {"<a href=\"x\">", 7}, {"<a b c>", 7}, {"<a b=c d>", 7},
		{"<x-y>", 7}, {"<br/>", 7}, {"<br />", 7}, {"<img src=x>", 7}, {"<a> ", 7}, {"<a>\t", 7},
		{"<a> ", 7}, {"   <span>", 7},
		{"<a b=>", 0}, {"<a 1b>", 0}, {"<a:b>", 0}, {"<a_b>", 0}, {"<x.y>", 0}, {"<a>b", 0},
		{"<a>x", 0}, {"<a/ >", 0}, {"<a b>c", 0}, {"hello <span>", 0}, {"<a b=\"c\"d>", 0},
		{"<a b=c\"d>", 0}, {"<a b=c'd'>", 0}, {"    <div>", 0}, {"    <span>", 0},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			if got := htmlStart(c.line); got != c.want {
				t.Fatalf("htmlStart(%q) = %d; want %d", c.line, got, c.want)
			}
		})
	}
}

// TestATXHeading pins the ATX heading start of CommonMark 0.31.2, as commonmark.js
// reads it: one to six '#', then a space, a tab or the end of the line.
func TestATXHeading(t *testing.T) {
	cases := []struct {
		line  string
		level int
		text  string
		tab   bool
		ok    bool
	}{
		{"## x", 2, "x", false, true},
		{"##\tx", 2, "x", true, true},
		{"#\tx", 1, "x", true, true},
		{"##\t", 2, "", true, true},
		{"##", 2, "", false, true},
		{"#", 1, "", false, true},
		{"###### x", 6, "x", false, true},
		{"   ## x", 2, "x", false, true},
		{"    ## x", 0, "", false, false},
		{"##x", 0, "", false, false},
		{"#######", 0, "", false, false},
		{"#######\tx", 0, "", false, false},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			level, text, tab, ok := headingOf(c.line)
			if ok != c.ok || level != c.level || text != c.text || tab != c.tab {
				t.Fatalf("headingOf(%q) = %d, %q, tab %v, %v; want %d, %q, tab %v, %v",
					c.line, level, text, tab, ok, c.level, c.text, c.tab, c.ok)
			}
		})
	}
}

// TestHeadingBoundary pins the section boundary of a run of lines: the first
// level-2 heading, which a tab after its hashes makes and a letter after them does not.
func TestHeadingBoundary(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  int
	}{
		{"tab after the hashes", []string{"x", "##\tNotes", "y"}, 1},
		{"space after the hashes", []string{"x", "## Notes", "y"}, 1},
		{"hashes at the end of the line", []string{"##"}, 0},
		{"letter after the hashes", []string{"x", "##x", "y"}, 3},
		{"seven hashes", []string{"x", "#######", "y"}, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := boundary(c.lines, 0); got != c.want {
				t.Fatalf("boundary(%q) = %d; want %d", c.lines, got, c.want)
			}
		})
	}
}

// TestSetextAfterDefinitions pins the setext headings of CommonMark 0.31.2 (§4.3, §4.7)
// after link reference definitions: the definitions at the start of the paragraph are
// no text of the heading, which begins on the first line they leave, and a paragraph of
// definitions alone makes no heading. The readings are those of commonmark.js, cmark-gfm
// and markdown-it.
func TestSetextAfterDefinitions(t *testing.T) {
	type head struct {
		at, last int
		text     string
	}
	cases := []struct {
		name  string
		lines []string
		want  []head
	}{
		{"10#detail-headings-and-anchors: a definition before the text", []string{"[r]: /u", "people row P1", "---"}, []head{{1, 2, "people row P1"}}},
		{"10#detail-headings-and-anchors: two definitions, one with a title", []string{"[r]: /u", `[s]: /v "t"`, "x", "==="}, []head{{2, 3, "x"}}},
		{"10#detail-headings-and-anchors: a title that does not close is text", []string{"[r]: /u", `"t`, "x", "---"}, []head{{1, 3, "\"t\nx"}}},
		{"10#detail-headings-and-anchors: a definition after the text is text", []string{"x", "[r]: /u", "---"}, []head{{0, 2, "x\n[r]: /u"}}},
		{"10#detail-headings-and-anchors: definitions alone and a dash underline make no heading", []string{"[r]: /u", "---"}, nil},
		{"10#detail-headings-and-anchors: definitions alone and an equals underline make no heading", []string{"[r]: /u", "==="}, nil},
		{"10#detail-headings-and-anchors: a break after definitions alone, then a heading", []string{"[r]: /u", "---", "x", "---"}, []head{{2, 3, "x"}}},
		{"10#detail-headings-and-anchors: a definition in a block quote", []string{"> [r]: /u", "> x", "> ---"}, []head{{1, 2, "x"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []head
			for _, h := range scanHeadings(c.lines, 0).all {
				got = append(got, head{h.at, h.last, h.text})
			}
			if len(got) != len(c.want) {
				t.Fatalf("headings %+v; want %+v", got, c.want)
			}
			for k := range got {
				if got[k] != c.want[k] {
					t.Fatalf("headings %+v; want %+v", got, c.want)
				}
			}
		})
	}
}

// TestInterruptsParagraph pins the rule of CommonMark 0.31.2 that an HTML block
// of types 1 to 6 may interrupt a paragraph and a type 7 block may not, and that a
// footnote definition interrupts one under blockSyntax.notes and a math block under
// blockSyntax.math, and neither under CommonMark.
func TestInterruptsParagraph(t *testing.T) {
	notes, math := blockSyntax{notes: true}, blockSyntax{math: true}
	cases := []struct {
		syn  blockSyntax
		line string
		want bool
	}{
		{blockSyntax{}, "<div>", true}, {blockSyntax{}, "<!-- x", true}, {blockSyntax{}, "<script>", true},
		{blockSyntax{}, "<?php", true}, {blockSyntax{}, "<span>", false}, {blockSyntax{}, "<span class=\"a\">", false},
		{blockSyntax{}, "</span>", false}, {blockSyntax{}, "<a> ", false}, {blockSyntax{}, "    <div>", false},
		{blockSyntax{}, "[^n]: x", false}, {blockSyntax{}, "$$", false},
		{notes, "[^n]: x", true}, {notes, "   [^n]:", true}, {notes, "[^n]:x", true},
		{notes, "    [^n]: x", false}, {notes, "[^]: x", false}, {notes, "[^a b]: x", false}, {notes, "[^n] : x", false},
		{notes, "$$", false},
		{math, "$$", true}, {math, "   $$ x", true}, {math, "$$x$$", true}, {math, "$$a $$ b", true},
		{math, "$$a$$b$$", false}, {math, "    $$", false}, {math, "x $$", false}, {math, "[^n]: x", false},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%+v/%s", c.syn, c.line), func(t *testing.T) {
			if got := c.syn.opensInterrupt(c.line); got != c.want {
				t.Fatalf("opensInterrupt(%q) = %v; want %v", c.line, got, c.want)
			}
		})
	}
}

// TestMathOpens pins math_block's reading of the line that opens it: a block of that
// line alone when one "$$" ends the text after the first, none when two or more follow.
func TestMathOpens(t *testing.T) {
	cases := []struct {
		line          string
		open, oneLine bool
	}{
		{"$$", true, false}, {"$$ x", true, false}, {"$$x$$", true, true}, {"$$$$", true, true},
		{"$$$", true, false}, {"$$a $$ b", true, false}, {"$$a$$b$$", false, false}, {"x$$", false, false},
		{"  $$x$$  ", true, false}, {"$$\t", true, false},
	}
	for _, c := range cases {
		open, oneLine := mathOpens(c.line)
		if open != c.open || oneLine != c.oneLine {
			t.Errorf("mathOpens(%q) = %v, %v; want %v, %v", c.line, open, oneLine, c.open, c.oneLine)
		}
	}
}

// TestTableDelimiterRow pins what a GFM table's delimiter row is (GFM 0.29 §4.10): the cells of a line, each only
// hyphens with an optional colon on either end, separated by pipes, with at most three spaces of indent and a
// pipe optional at either end. It returns the number of cells.
func TestTableDelimiterRow(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		cells int
	}{
		{"a delimiter row of two cells", "-|-", 2},
		{"outer pipes", "|-|-|", 2},
		{"spaces around the cells", " - | - ", 2},
		{"alignment colons", ":-|-:", 2},
		{"centered", "|:-:|--|", 2},
		{"a long hyphen run", "-----|-----", 2},
		{"one cell needs a pipe", "|-|", 1},
		{"one cell without a pipe is no delimiter row", "---", 0},
		{"an empty cell", "-||-", 0},
		{"a letter in a cell", "-|a", 0},
		{"a lone colon", ":|-", 0},
		{"a colon pair without a hyphen", "::|-", 0},
		{"four spaces of indent", "    -|-", 0},
		{"three spaces of indent", "   -|-", 2},
		{"a blank line", "", 0},
		{"an escaped pipe is no separator", `-\|-`, 0},
	}
	t.Run("10#detail-headings-and-anchors: markdown-it takes one cell without a pipe", func(t *testing.T) {
		if got := tableDelimiterCells("---", true); got != 1 {
			t.Fatalf("tableDelimiterCells(%q, true) = %d; want 1", "---", got)
		}
	})
	for _, c := range cases {
		t.Run("10#detail-headings-and-anchors: "+c.name, func(t *testing.T) {
			if got := tableDelimiterCells(c.line, false); got != c.cells {
				t.Fatalf("tableDelimiterCells(%q) = %d; want %d", c.line, got, c.cells)
			}
		})
	}
}

// TestTableRowCells pins the number of cells of a GFM table row (GFM 0.29 §4.10): the cells between the unescaped pipes
// a code span does not protect, a pipe at either end optional.
func TestTableRowCells(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		cells int
	}{
		{"two cells", "a|b", 2},
		{"outer pipes", "|a|b|", 2},
		{"one cell with a pipe", "|a|", 1},
		{"an empty cell", "a||b", 3},
		{"an escaped pipe", `a\|b|c`, 2},
		{"a pipe in a code span splits", "`a|b`|c", 3},
		{"spaces", " a | b ", 2},
		{"no pipe", "a", 1},
	}
	for _, c := range cases {
		t.Run("10#detail-headings-and-anchors: "+c.name, func(t *testing.T) {
			if got := tableRowCells(c.line); got != c.cells {
				t.Fatalf("tableRowCells(%q) = %d; want %d", c.line, got, c.cells)
			}
		})
	}
}

// TestTables pins the headings of a document that holds a GFM table (GFM 0.29 §4.10) under blockSyntax.tables, which
// GitHub, GitLab and VS Code read: a table is a header row and a delimiter row of as many cells, and the lines after
// them are its body rows until a blank line or a block that interrupts a paragraph. A line of the table is no
// paragraph line, so a setext underline after a body row underlines nothing. CommonMark alone reads no table.
func TestTables(t *testing.T) {
	type head struct {
		at, last int
		text     string
	}
	cases := []struct {
		name   string
		lines  []string
		tables bool
		want   []head
	}{
		{"10#detail-headings-and-anchors: a body row is no text for a setext underline", []string{"a|b", "-|-", "items row T1", "---"}, true, nil},
		{"10#detail-headings-and-anchors: CommonMark alone reads that underline", []string{"a|b", "-|-", "items row T1", "---"}, false, []head{{0, 3, "a|b\n-|-\nitems row T1"}}},
		{"10#detail-headings-and-anchors: an ATX heading ends the table", []string{"a|b", "-|-", "c|d", "# items row T1"}, true, []head{{3, 3, "items row T1"}}},
		{"10#detail-headings-and-anchors: a blank line ends the table, and a setext heading after it is read", []string{"a|b", "-|-", "", "items row T1", "---"}, true, []head{{3, 4, "items row T1"}}},
		{"10#detail-headings-and-anchors: a mismatched delimiter row is no table", []string{"a|b|c", "-|-", "items row T1", "---"}, true, []head{{0, 3, "a|b|c\n-|-\nitems row T1"}}},
		{"10#detail-headings-and-anchors: the header row is the last line of the paragraph before it", []string{"x", "a|b", "-|-", "items row T1", "---"}, true, nil},
		{"10#detail-headings-and-anchors: a block quote ends the table", []string{"a|b", "-|-", "> # h1", "# h2"}, true, []head{{2, 2, "h1"}, {3, 3, "h2"}}},
		{"10#detail-headings-and-anchors: a table inside a block quote", []string{"> a|b", "> -|-", "> items row T1", "> ---"}, true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []head
			for _, h := range scanWith(c.lines, 0, blockSyntax{tables: c.tables}).all {
				got = append(got, head{h.at, h.last, h.text})
			}
			if len(got) != len(c.want) {
				t.Fatalf("headings %+v; want %+v", got, c.want)
			}
			for k := range got {
				if got[k] != c.want[k] {
					t.Fatalf("headings %+v; want %+v", got, c.want)
				}
			}
		})
	}
}

// TestMarkdownItTables pins the two readings of VS Code's markdown-it 14 that differ from GitHub's and GitLab's under
// tables: a line that would be an ATX heading is the header row of a table when a delimiter row of as many cells
// follows, and a header row that holds a pipe needs no pipe in its delimiter row.
func TestMarkdownItTables(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		md    bool
		want  []string
	}{
		{"10#detail-headings-and-anchors: a heading line is a header row", []string{"# a|b", "-|-", "x"}, true, nil},
		{"10#detail-headings-and-anchors: GitHub reads that heading", []string{"# a|b", "-|-", "x"}, false, []string{"a|b"}},
		{"10#detail-headings-and-anchors: a heading line without a pipe stays a heading", []string{"# a", "-|-", "x"}, true, []string{"a"}},
		{"10#detail-headings-and-anchors: a heading line with another cell count stays a heading", []string{"# a|b|c", "-|-", "x"}, true, []string{"a|b|c"}},
		{"10#detail-headings-and-anchors: a heading in a block quote is a header row", []string{"> # a|b", "> -|-", "> x"}, true, nil},
		{"10#detail-headings-and-anchors: a heading line two lines before is no header row", []string{"# a|b", "text", "-|-"}, true, []string{"a|b"}},
		{"10#detail-headings-and-anchors: a pipeless delimiter row", []string{"| a |", "---", "x"}, true, nil},
		{"10#detail-headings-and-anchors: GitHub reads that setext heading", []string{"| a |", "---", "x"}, false, []string{"| a |"}},
		{"10#detail-headings-and-anchors: a header row without a pipe is no table", []string{"a", "---", "x"}, true, []string{"a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, h := range scanWith(c.lines, 0, blockSyntax{tables: true, mdTables: c.md}).all {
				got = append(got, h.text)
			}
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Fatalf("headings %q; want %q", got, c.want)
			}
		})
	}
}

// TestTableAtTheStart pins the table whose header row is the first line of the document, which leaves no paragraph
// behind: the scanner holds no unit for it, and the paragraphs after the table are read as paragraphs again.
func TestTableAtTheStart(t *testing.T) {
	lines := []string{"a|b", "-|-", "c|d", "", "items row T1", "---", "", "more text", "==="}
	var got []string
	for _, h := range scanWith(lines, 0, blockSyntax{tables: true}).all {
		got = append(got, h.text)
	}
	if len(got) != 2 || got[0] != "items row T1" || got[1] != "more text" {
		t.Fatalf("headings %q; want the two setext headings after the table", got)
	}
}
