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
