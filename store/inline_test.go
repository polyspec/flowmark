package store

import (
	"fmt"
	"testing"
)

// TestInlineText pins the text that the inline reader keeps for the folds, and the
// attributes of the raw HTML open tags it reads: link targets, emphasis markup,
// raw HTML and images are not text, code spans and autolinks keep their content,
// entities are decoded, and a bracket that forms no link is text.
func TestInlineText(t *testing.T) {
	cases := []struct {
		name, in, text string
		tags           string // the attributes of the open tags, as fmt prints them
	}{
		{"plain text", "plain text", "plain text", "[]"},
		{"link keeps its text and drops its target", "a [link](/uri \"title\") b", "a link b", "[]"},
		{"two links", "a [link](/u) [two](/v)", "a link two", "[]"},
		{"link in a link is not a link", "[a [b](c) d](e)", "a b d(e)", "[]"},
		{"image keeps no text", "![alt](/img.png) x", " x", "[]"},
		{"code span keeps its content and hides markup", "`code` and `<b>`", "code and <b>", "[]"},
		{"unmatched backtick is text", "a ` b", "a ` b", "[]"},
		{"escaped bracket is no link opener", `\[not a link](x)`, "not a link(x)", "[]"},
		{"shortcut reference keeps its text", "[a]", "a", "[]"},
		{"full reference without definitions keeps both labels", "[a][b]", "ab", "[]"},
		{"bracket with a space before the target is no link", "[a] (b)", "a (b)", "[]"},
		{"link with a target that has a space and no title is no link", "[a](b c)", "a(b c)", "[]"},
		{"link with an angle-bracketed target", "[a](<b c>)", "a", "[]"},
		{"link with an empty target", "[a]()", "a", "[]"},
		{"link with an unclosed title is no link", "[a](b \"unclosed)", "a(b \"unclosed)", "[]"},
		{"emphasis markup is kept as characters that fold away", "*emph* _s_", "*emph* _s_", "[]"},
		{"raw HTML tags are not text", "<b>bold</b>", "bold", "[[]]"},
		{"raw HTML open tag attribute", `<a id="x">y</a>`, "y", `[[{id x}]]`},
		{"attribute values are decoded", "<span name='a&amp;b' data-x=1>z</span>", "z", "[[{name a&b} {data-x 1}]]"},
		{"named entity", "&eacute;t&eacute;", "été", "[]"},
		{"numeric entity", "x&#x41;y", "xAy", "[]"},
		{"unknown named reference is text", "&notanentity; x", "&notanentity; x", "[]"},
		{"autolink keeps its text", "<http://a.b/c>", "http://a.b/c", "[]"},
		{"email autolink keeps its text", "<me@ex.com>", "me@ex.com", "[]"},
		{"angle bracket that opens no tag is text", "<not a tag", "<not a tag", "[]"},
		{"comment is not text", "<!-- c --> z", " z", "[]"},
		{"comment in a code span is code", "`<!-- c -->`", "<!-- c -->", "[]"},
		{"processing instruction is not text", "<?pi?>z", "z", "[]"},
		{"CDATA section is not text", "<![CDATA[x]]>z", "z", "[]"},
		{"declaration is not text", "<!DOCTYPE html>z", "z", "[]"},
		{"line feed inside a setext heading", "a\nb", "a\nb", "[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := inlineText(c.in, linkRefs{})
			if res.text != c.text {
				t.Fatalf("inlineText(%q) text = %q; want %q", c.in, res.text, c.text)
			}
			if got := fmt.Sprint(res.tags); got != c.tags {
				t.Fatalf("inlineText(%q) tags = %s; want %s", c.in, got, c.tags)
			}
		})
	}
}

// TestInlineAlt pins the alt text that the reader keeps for an image, which the F_G
// fold reads beside the text without it: an image keeps no text but its alt text,
// nested links and emphasis included, and an image in a link keeps its alt text too.
func TestInlineAlt(t *testing.T) {
	cases := []struct {
		name, in, text, alt string
	}{
		{"image keeps its alt text in alt only", "![alt](/img.png) x", " x", "alt x"},
		{"alt text keeps its emphasis markers as characters", "![a *b*](x)", "", "a *b*"},
		{"link in alt text keeps its text", "![a [b](c)](d)", "", "a b"},
		{"image in a link keeps the alt text", "[![a](b)](c)", "", "a"},
		{"image nested in an image keeps both alt texts", "![x ![y](z)](w)", "", "x y"},
		{"unclosed image bracket is text in both readings", "![a b", "a b", "a b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := inlineText(c.in, linkRefs{})
			if res.text != c.text || res.alt != c.alt {
				t.Fatalf("inlineText(%q) = text %q, alt %q; want %q, %q", c.in, res.text, res.alt, c.text, c.alt)
			}
		})
	}
}
