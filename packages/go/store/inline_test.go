package store

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// shown writes a reading with each line break as ⏎, so a test tells a break from a
// line feed of the text.
func shown(r reading) string { return strings.Join(r, "⏎") }

// TestInlineText pins the rendered text of CommonMark 0.31.2 that the inline reader
// keeps for the slugs, and the attributes of the raw HTML open tags it reads: link
// targets, emphasis delimiters that match, raw HTML and images are not text; code
// spans keep their normalized content and autolinks their text; escapes and entities
// are their characters; and a bracket or a delimiter that forms nothing is text.
func TestInlineText(t *testing.T) {
	cases := []struct {
		name, in, text string
		tags           string // the attributes of the open tags, as fmt prints them
	}{
		{"plain text", "plain text", "plain text", "[]"},
		{"link keeps its text and drops its target", "a [link](/uri \"title\") b", "a link b", "[]"},
		{"two links", "a [link](/u) [two](/v)", "a link two", "[]"},
		{"link in a link is not a link", "[a [b](c) d](e)", "[a b d](e)", "[]"},
		{"image keeps no text", "![alt](/img.png) x", " x", "[]"},
		{"code span keeps its content and hides markup", "`code` and `<b>`", "code and <b>", "[]"},
		{"10#detail-headings-and-anchors: a code span strips one space at each end", "` a `", "a", "[]"},
		{"10#detail-headings-and-anchors: a code span strips only one space at each end", "`  a  `", " a ", "[]"},
		{"10#detail-headings-and-anchors: a code span of spaces is not stripped", "`  `", "  ", "[]"},
		{"10#detail-headings-and-anchors: a code span with a space at one end only is not stripped", "` a`", " a", "[]"},
		{"10#detail-headings-and-anchors: a line ending in a code span is a space", "`a\nb`", "a b", "[]"},
		{"10#detail-headings-and-anchors: a code span holding a backtick", "`` a`b ``", "a`b", "[]"},
		{"unmatched backtick is text", "a ` b", "a ` b", "[]"},
		{"10#detail-headings-and-anchors: an escaped bracket is the bracket and no link opener", `\[not a link](x)`, "[not a link](x)", "[]"},
		{"10#detail-headings-and-anchors: an escaped underscore is the underscore", `T\_1`, "T_1", "[]"},
		{"10#detail-headings-and-anchors: a backslash before a letter is text", `a\b`, `a\b`, "[]"},
		{"shortcut reference keeps its text", "[a]", "[a]", "[]"},
		{"full reference without definitions is literal text", "[a][b]", "[a][b]", "[]"},
		{"bracket with a space before the target is no link", "[a] (b)", "[a] (b)", "[]"},
		{"link with a target that has a space and no title is no link", "[a](b c)", "[a](b c)", "[]"},
		{"link with an angle-bracketed target", "[a](<b c>)", "a", "[]"},
		{"link with an empty target", "[a]()", "a", "[]"},
		{"link with an unclosed title is no link", "[a](b \"unclosed)", "[a](b \"unclosed)", "[]"},
		{"10#detail-headings-and-anchors: an unmatched closing bracket is text", "a]b", "a]b", "[]"},
		{"10#detail-headings-and-anchors: an image opener that forms nothing is text", "![a b", "![a b", "[]"},
		{"10#detail-headings-and-anchors: emphasis and strong delimiters are removed", "*emph* _s_ **strong** __b__", "emph s strong b", "[]"},
		{"10#detail-headings-and-anchors: an intraword underscore opens no emphasis", "snake_case_word", "snake_case_word", "[]"},
		{"10#detail-headings-and-anchors: an intraword asterisk opens emphasis", "a*b*c", "abc", "[]"},
		{"10#detail-headings-and-anchors: an unmatched delimiter is text", "_a *b", "_a *b", "[]"},
		{"10#detail-headings-and-anchors: a delimiter followed by white space opens nothing", "_ a_", "_ a_", "[]"},
		{"10#detail-headings-and-anchors: unequal runs leave their rest as text", "***a*", "**a", "[]"},
		{"10#detail-headings-and-anchors: the rule of three", "*a**b*", "a**b", "[]"},
		{"10#detail-headings-and-anchors: emphasis across a link boundary does not match", "*[a*](b)", "*a*", "[]"},
		{"10#detail-headings-and-anchors: emphasis inside a link matches", "[*a*](b)", "a", "[]"},
		{"10#detail-headings-and-anchors: different delimiters do not match", "*a_", "*a_", "[]"},
		{"10#detail-headings-and-anchors: a closer removes the delimiters it encloses", "_a *b_ c*", "a *b c*", "[]"},
		{"10#detail-headings-and-anchors: an escaped delimiter is text", `\_a_`, "_a_", "[]"},
		{"10#detail-headings-and-anchors: a delimiter in a code span is code", "`_a_`", "_a_", "[]"},
		{"raw HTML tags are not text", "<b>bold</b>", "bold", "[[]]"},
		{"raw HTML open tag attribute", `<a id="x">y</a>`, "y", `[[{id x}]]`},
		{"attribute values are decoded", "<span name='a&amp;b' data-x=1>z</span>", "z", "[[{name a&b} {data-x 1}]]"},
		{"named entity", "&eacute;t&eacute;", "été", "[]"},
		{"numeric entity", "x&#x41;y", "xAy", "[]"},
		{"10#detail-headings-and-anchors: an entity is no delimiter", "&#95;a&#95;", "_a_", "[]"},
		{"unknown named reference is text", "&notanentity; x", "&notanentity; x", "[]"},
		{"autolink keeps its text", "<http://a.b/c>", "http://a.b/c", "[]"},
		{"email autolink keeps its text", "<me@ex.com>", "me@ex.com", "[]"},
		{"angle bracket that opens no tag is text", "<not a tag", "<not a tag", "[]"},
		{"comment is not text", "<!-- c --> z", " z", "[]"},
		{"comment in a code span is code", "`<!-- c -->`", "<!-- c -->", "[]"},
		{"processing instruction is not text", "<?pi?>z", "z", "[]"},
		{"CDATA section is not text", "<![CDATA[x]]>z", "z", "[]"},
		{"declaration is not text", "<!DOCTYPE html>z", "z", "[]"},
		{"10#detail-headings-and-anchors: a line ending inside a setext heading is a break", "a\nb", "a⏎b", "[]"},
		{"10#detail-headings-and-anchors: spaces before a line ending are no text", "a  \nb", "a⏎b", "[]"},
		{"10#detail-headings-and-anchors: a backslash before a line ending is a hard break", "a\\\nb", "a⏎b", "[]"},
		{"10#detail-headings-and-anchors: a backslash at the end is text", "a\\", "a\\", "[]"},
		{"10#detail-headings-and-anchors: a line feed character reference is text, not a break", "a&#10;b", "a\nb", "[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := inlineText(c.in, linkRefs{})
			if got := shown(res.text); got != c.text {
				t.Fatalf("inlineText(%q) text = %q; want %q", c.in, got, c.text)
			}
			if got := fmt.Sprint(res.tags); got != c.tags {
				t.Fatalf("inlineText(%q) tags = %s; want %s", c.in, got, c.tags)
			}
		})
	}
}

// TestInlineAlt pins the alt text that the reader keeps for an image, the plain-text
// rendering of its description, which the slugs read beside the text without it: an
// image keeps no text but its alt text, nested links and images included, its
// emphasis delimiters removed.
func TestInlineAlt(t *testing.T) {
	cases := []struct {
		name, in, text, alt string
	}{
		{"image keeps its alt text in alt only", "![alt](/img.png) x", " x", "alt x"},
		{"10#detail-headings-and-anchors: alt text drops its emphasis delimiters", "![a *b* _T1_](x)", "", "a b T1"},
		{"link in alt text keeps its text", "![a [b](c)](d)", "", "a b"},
		{"image in a link keeps the alt text", "[![a](b)](c)", "", "a"},
		{"image nested in an image keeps both alt texts", "![x ![y](z)](w)", "", "x y"},
		{"unclosed image bracket is text in both readings", "![a b", "![a b", "![a b"},
		{"10#detail-headings-and-anchors: a code span in alt text keeps its content", "![` a `](x)", "", "a"},
		{"10#detail-headings-and-anchors: a line break in alt text is a break", "![a\nb](x)", "", "a⏎b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := inlineText(c.in, linkRefs{})
			if shown(res.text) != c.text || shown(res.alt) != c.alt {
				t.Fatalf("inlineText(%q) = text %q, alt %q; want %q, %q", c.in, shown(res.text), shown(res.alt), c.text, c.alt)
			}
		})
	}
}

// testRefs returns the definitions of each viewer for a file that defines the footnote
// labels notes and no link reference: GitHub's and GitLab's footnote definitions, and
// none for VS Code.
func testRefs(notes ...string) viewerRefs {
	var defs []*noteDef
	for _, n := range notes {
		defs = append(defs, &noteDef{label: n})
	}
	return viewerRefs{
		gh: linkRefs{notes: newNoteRefs(defs, true)},
		gl: linkRefs{notes: newNoteRefs(defs, false)},
	}
}

// TestViewerReadings pins the readings of VS Code and GitLab: each viewer's math, and
// GitLab's footnote references and wikilinks, accepted and refused.
func TestViewerReadings(t *testing.T) {
	notes := testRefs("n")
	cases := []struct {
		name, in, vs, gl string
	}{
		{"10#detail-headings-and-anchors: a math span", "a $b$ c", "a  c", "a b c"},
		{"10#detail-headings-and-anchors: GitLab keeps the source of math, a tag included", "$T<b>1$", "", "T<b>1"},
		{"10#detail-headings-and-anchors: GitLab normalizes one-dollar math as a code span", "$ b $", "", "$ b $"},
		{"10#detail-headings-and-anchors: GitLab opens no one-dollar math before a space", "a $ b$ c", "a  c", "a $ b$ c"},
		{"10#detail-headings-and-anchors: GitLab closes no math before a digit", "$b$1", "$b$1", "$b$1"},
		{"10#detail-headings-and-anchors: GitLab takes two-dollar math as it stands", "$$ b $$", "", " b "},
		{"10#detail-headings-and-anchors: three dollars open nothing", "$$$b$$$", "$$$b$$$", "$$$b$$$"},
		{"10#detail-headings-and-anchors: code math", "$`b  c`$", "", "b  c"},
		{"10#detail-headings-and-anchors: empty code math opens nothing in GitLab", "$``$", "", "$``$"},
		{"10#detail-headings-and-anchors: an escaped dollar opens nothing", `\$b$`, "$b$", "$b$"},
		{"10#detail-headings-and-anchors: a dollar after a backslash closes nothing in GitLab", `$b\$c$`, "", `b\$c`},
		{"10#detail-headings-and-anchors: VS Code opens no math after a word character", "a$b$", "a$b$", "ab"},
		{"10#detail-headings-and-anchors: VS Code closes no math before a word character", "$b$c", "$b$c", "bc"},
		{"10#detail-headings-and-anchors: VS Code's guard after a long tag", "<span>$b$", "$b$", "b"},
		{"10#detail-headings-and-anchors: a code span ends VS Code's guard", "<span>`x`$b$", "x", "xb"},
		{"10#detail-headings-and-anchors: a link ends VS Code's guard at its bracket", "<span>[$b$](/u)", "", "b"},
		{"10#detail-headings-and-anchors: a defined footnote reference is no text in GitLab", "a[^n]", "a[^n]", "a"},
		{"10#detail-headings-and-anchors: an undefined footnote reference is restored", "a[^m<i>x] *b*", "a[^mx] b", "a[^m<i>x] b"},
		{"10#detail-headings-and-anchors: a footnote reference stops emphasis inside it", "*a[^m*]", "a[^m]", "*a[^m*]"},
		{"10#detail-headings-and-anchors: a code span in brackets is no footnote reference", "[^`n`]", "[^n]", "[^n]"},
		{"10#detail-headings-and-anchors: a nested bracket is no footnote reference", "[^n[x]]", "[^n[x]]", "[^n[x]]"},
		{"10#detail-headings-and-anchors: '![^' opens no image in GitLab", "![^m]", "![^m]", "![^m]"},
		{"10#detail-headings-and-anchors: a wikilink renders its title", "a [[T1|x]] b", "a [[T1|x]] b", "a T1 b"},
		{"10#detail-headings-and-anchors: a wikilink without a pipe renders its target", "[[T1]]", "[[T1]]", "T1"},
		{"10#detail-headings-and-anchors: a wikilink's title is trimmed", "[[ a b |c]]", "[[ a b |c]]", "a b"},
		{"10#detail-headings-and-anchors: a wikilink's title holds no markup", "[[*a* `b` $c$|d]]", "[[a b |d]]", "*a* `b` $c$"},
		{"10#detail-headings-and-anchors: a wikilink's title decodes references, then escapes", `[[a&amp;b\*c&#92;*|d]]`, "[[a&b*c\\*|d]]", "a&b*c*"},
		{"10#detail-headings-and-anchors: an escaped pipe stays in a wikilink's title", `[[a\|b|c]]`, "[[a|b|c]]", "a|b"},
		{"10#detail-headings-and-anchors: a third component opens no wikilink", "[[a|b|c]]", "[[a|b|c]]", "[[a|b|c]]"},
		{"10#detail-headings-and-anchors: an open bracket keeps a wikilink from opening", "[x [[a|b]]]", "[x [[a|b]]]", "[x [[a|b]]]"},
		{"10#detail-headings-and-anchors: an image's bracket keeps a wikilink from opening", "![[a|b]]", "![[a|b]]", "![[a|b]]"},
		{"10#detail-headings-and-anchors: an unclosed wikilink opens nothing", "[[a|b] c", "[[a|b] c", "[[a|b] c"},
		{"10#detail-headings-and-anchors: a wikilink's line ending is in its title", "[[a\nb|c]]", "[[a⏎b|c]]", "a\nb"},
		{"10#detail-headings-and-anchors: emphasis encloses a wikilink", "*[[a*|b]]*", "[[a|b]]*", "a*"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs, gl := shown(vsText(c.in, notes.vs)), shown(glText(c.in, notes.gl))
			if vs != c.vs || gl != c.gl {
				t.Fatalf("%q reads vs %q, gl %q; want %q, %q", c.in, vs, gl, c.vs, c.gl)
			}
		})
	}
}

// TestGitHubFootnotes pins GitHub's reading of footnote references: a defined one renders
// the number of its definition by first reference, an undefined one its source as text,
// and a link wins over a reference; accepted and refused.
func TestGitHubFootnotes(t *testing.T) {
	cases := []struct {
		name, in, text string
	}{
		{"10#detail-headings-and-anchors: references render numbers by first reference", "a[^b] c[^a] d[^b]", "a1 c2 d1"},
		{"10#detail-headings-and-anchors: a label folds as a link label", "[^A] [^ a ] [^ss] [^ß]", "1 1 2 2"},
		{"10#detail-headings-and-anchors: an undefined reference is its source", "a[^x<i>*y*] b", "a[^x<i>*y*] b"},
		{"10#detail-headings-and-anchors: an empty label is no reference", "[^]", "[^]"},
		{"10#detail-headings-and-anchors: an inline link wins", "[^a](u) [^a][]", "^a 1[]"},
		{"10#detail-headings-and-anchors: a reference inside a link's text", "[x[^a]](/u)", "x1"},
		{"10#detail-headings-and-anchors: a reference in brackets", "[[^a]] [b [^a]] [^a [c]]", "[1] [b 1] [^a [c]]"},
		{"10#detail-headings-and-anchors: '![^' is '!' and a reference", "![^a]", "!1"},
		{"10#detail-headings-and-anchors: a reference ends emphasis inside it", "*x[^a*b]", "*x1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			refs := testRefs("a", "b", "ß", "a*b")
			if got := shown(inlineText(c.in, refs.gh).text); got != c.text {
				t.Fatalf("inlineText(%q) = %q; want %q", c.in, got, c.text)
			}
		})
	}
}

// TestInlineBreaks requires a reading to hold its line breaks apart from its text, so
// S_GH joins its lines with nothing and S_GL with a space.
func TestInlineBreaks(t *testing.T) {
	res := inlineText("items row\nT1", linkRefs{})
	if want := (reading{"items row", "T1"}); !reflect.DeepEqual(res.text, want) {
		t.Fatalf("text = %q; want %q", res.text, want)
	}
}
