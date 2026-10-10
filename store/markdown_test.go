package store

import "testing"

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

// TestInterruptsParagraph pins the rule of CommonMark 0.31.2 that an HTML block
// of types 1 to 6 may interrupt a paragraph and a type 7 block may not.
func TestInterruptsParagraph(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"<div>", true}, {"<!-- x", true}, {"<script>", true}, {"<?php", true},
		{"<span>", false}, {"<span class=\"a\">", false}, {"</span>", false}, {"<a> ", false},
		{"    <div>", false},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			if got := opensInterrupt(c.line); got != c.want {
				t.Fatalf("opensInterrupt(%q) = %v; want %v", c.line, got, c.want)
			}
		})
	}
}
