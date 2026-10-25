package store

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf16"
)

// TestUnicodeVersion requires the unicode package of Go to be Unicode 17.0.0, the
// version of the lowercase mapping of every slug, of the properties that S_GH keeps
// (the version GitHub's ids follow, testdata/slug) and of the categories of S_GL (the
// version of finl_unicode 1.4.0, which comrak 0.56.0 reads them from), so a Go
// release that changes the version fails here instead of changing the slugs quietly.
func TestUnicodeVersion(t *testing.T) {
	if unicode.Version != "17.0.0" {
		t.Fatalf("unicode.Version = %q; spec 10 needs 17.0.0", unicode.Version)
	}
}

// TestLowerFull pins the full default lowercase mapping: the simple mapping of each
// character, except U+0130, whose lowercase is two characters.
func TestLowerFull(t *testing.T) {
	cases := [][2]string{
		{"Items Row T1", "items row t1"},
		{"\U00000130", "i\U00000307"},
		{"\U00000130TEMS", "i\U00000307tems"},
		{"\U0000212A", "k"},
		{"\U0000017F", "\U0000017F"},
		{"\U00001E9E", "\U000000DF"},
		{"\U00000391\U000003A3", "\U000003B1\U000003C3"},
		{"\U0000216B", "\U0000217B"},
		{"\U000024B6", "\U000024D0"},
		{"", ""},
	}
	for _, c := range cases {
		if got := lowerFull(c[0]); got != c[1] {
			t.Fatalf("lowerFull(%q) = %q; want %q", c[0], got, c[1])
		}
	}
}

// TestSlugs pins S_GH, S_VS and S_GL of spec 10 (Detail headings and anchors) on the
// cases that tell them apart.
func TestSlugs(t *testing.T) {
	cases := []struct {
		name, text, gh, vs, gl string
	}{
		{"10#detail-headings-and-anchors: ASCII letters, digits and spaces", "Items row T1", "items-row-t1", "items-row-t1", "items-row-t1"},
		{"10#detail-headings-and-anchors: hyphen and underscore are kept", "a-b_c d", "a-b_c-d", "a-b_c-d", "a-b_c-d"},
		{"10#detail-headings-and-anchors: no normalization, no mark removal", "caf\U000000E9", "caf\U000000E9", "caf\U000000E9", "caf\U000000E9"},
		{"10#detail-headings-and-anchors: a combining mark is kept", "e\U00000301", "e\U00000301", "e\U00000301", "e\U00000301"},
		{"10#detail-headings-and-anchors: a superscript digit is No: removed by S_GH, kept by S_GL", "x\U000000B2", "x", "x", "x\U000000B2"},
		{"10#detail-headings-and-anchors: a roman numeral is Alphabetic and a number", "\U0000216B", "\U0000217B", "\U0000217B", "\U0000217B"},
		{"10#detail-headings-and-anchors: a circled letter is Alphabetic but a symbol", "\U000024B6", "\U000024D0", "\U000024D0", ""},
		{"10#detail-headings-and-anchors: punctuation and symbols are removed", "a!b.c(d)e+f", "abcdef", "abcdef", "abcdef"},
		{"10#detail-headings-and-anchors: connector punctuation other than the underscore", "a\U0000203Fb", "a\U0000203Fb", "a\U0000203Fb", "a\U0000203Fb"},
		{"10#detail-headings-and-anchors: a dash other than the hyphen is removed", "a\U00002013b", "ab", "ab", "ab"},
		{"10#detail-headings-and-anchors: hyphen runs and edge hyphens are kept", "-a--b-", "-a--b-", "-a--b-", "-a--b-"},
		{"10#detail-headings-and-anchors: spaces are never collapsed", "a  b ", "a--b-", "a--b", "a--b-"},
		{"10#detail-headings-and-anchors: S_VS trims JavaScript white space first", "\U000000A0\t a b\U00003000\U0000FEFF", "-a-b", "a-b", "-a-b"},
		{"10#detail-headings-and-anchors: a no-break space is removed", "a\U000000A0b", "ab", "ab", "ab"},
		{"10#detail-headings-and-anchors: an ideograph is kept", "\U000065E5\U0000672C", "\U000065E5\U0000672C", "\U000065E5\U0000672C", "\U000065E5\U0000672C"},
		{"10#detail-headings-and-anchors: the Kelvin sign lowercases to k", "\U0000212A", "k", "k", "k"},
		{"10#detail-headings-and-anchors: a dotted capital I lowercases to i and a combining dot", "\U00000130", "i\U00000307", "i\U00000307", "i\U00000307"},
		{"10#detail-headings-and-anchors: a letter assigned in Unicode 14.0 is kept by S_GH, removed by S_VS", "\U000107A5", "\U000107A5", "", "\U000107A5"},
		{"10#detail-headings-and-anchors: a digit assigned in Unicode 16.0 is kept by S_GH, removed by S_VS", "\U0001CCF0", "\U0001CCF0", "", "\U0001CCF0"},
		{"10#detail-headings-and-anchors: a modifier letter assigned in Unicode 14.0 is kept by S_GH, removed by S_VS", "\U0000A7F2", "\U0000A7F2", "", "\U0000A7F2"},
		{"10#detail-headings-and-anchors: a format character is removed", "a\U0000200Bb", "ab", "ab", "ab"},
		{"10#detail-headings-and-anchors: the zero width joiner is Join_Control, kept by S_GH", "a\U0000200Db", "a\U0000200Db", "ab", "ab"},
		{"10#detail-headings-and-anchors: a final sigma is ς under S_VS and S_GL", "ΑΣ ΣΑ", "ασ-σα", "ας-σα", "ας-σα"},
		{"10#detail-headings-and-anchors: a sigma before a case-ignorable mark and a letter is not final", "ΑΣ́Α", "ασ́α", "ασ́α", "ασ́α"},
		{"10#detail-headings-and-anchors: a sigma after a case-ignorable period is final", "Α.Σ", "ασ", "ας", "ας"},
		{"10#detail-headings-and-anchors: a control character is removed", "a\nb", "ab", "ab", "ab"},
		{"10#detail-headings-and-anchors: a private use character is removed", "a\U0000E000b", "ab", "ab", "ab"},
		{"10#detail-headings-and-anchors: the empty text", "", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := slugGH(c.text); got != c.gh {
				t.Fatalf("slugGH(%q) = %q; want %q", c.text, got, c.gh)
			}
			if got := slugVS(c.text); got != c.vs {
				t.Fatalf("slugVS(%q) = %q; want %q", c.text, got, c.vs)
			}
			if got := slugGL(c.text); got != c.gl {
				t.Fatalf("slugGL(%q) = %q; want %q", c.text, got, c.gl)
			}
		})
	}
}

// TestTakesAnchor pins the numbering rule: a slug takes the anchor it equals, and the
// anchor that is the slug, a hyphen and a decimal number of one or more written
// without a leading zero.
func TestTakesAnchor(t *testing.T) {
	cases := []struct {
		name, slug, anchor string
		want               bool
	}{
		{"10#detail-headings-and-anchors: equal", "items-row-t1", "items-row-t1", true},
		{"10#detail-headings-and-anchors: numbered once", "items-row-t", "items-row-t-1", true},
		{"10#detail-headings-and-anchors: numbered with two digits", "items-row-t", "items-row-t-10", true},
		{"10#detail-headings-and-anchors: a leading zero is no numbering", "items-row-t", "items-row-t-01", false},
		{"10#detail-headings-and-anchors: zero is no numbering", "items-row-t", "items-row-t-0", false},
		{"10#detail-headings-and-anchors: a number without its hyphen is no numbering", "items-row-t", "items-row-t1", false},
		{"10#detail-headings-and-anchors: a hyphen without a number is no numbering", "items-row-t", "items-row-t-", false},
		{"10#detail-headings-and-anchors: a number and a letter are no numbering", "items-row-t", "items-row-t-1a", false},
		{"10#detail-headings-and-anchors: the anchor of a longer slug", "items-row-t1", "items-row-t", false},
		{"10#detail-headings-and-anchors: a non-ASCII digit is no numbering", "items-row-t", "items-row-t-\U00000661", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := takesAnchor(c.slug, c.anchor); got != c.want {
				t.Fatalf("takesAnchor(%q, %q) = %v; want %v", c.slug, c.anchor, got, c.want)
			}
		})
	}
}

// slugifyFile is VS Code's slugify.ts, as scripts/record-slugs.mts ran it to record the
// S_VS vectors, with VS Code's MIT license beside it in vscode-LICENSE.txt: its
// githubSlugReplaceRegex is the expression of the characters its slug removes.
const slugifyFile = "testdata/slug/vscode-slugify.ts"

// TestVSRemovalIsVSCodeRegex requires the characters that S_VS removes, by the Unicode
// 13.0.0 table, to be the characters that githubSlugReplaceRegex of VS Code's
// slugify.ts matches, over every code point but the surrogates, which a Go string
// cannot hold.
func TestVSRemovalIsVSCodeRegex(t *testing.T) {
	src, err := os.ReadFile(slugifyFile)
	if err != nil {
		t.Fatal(err)
	}
	set, err := slugifySet(string(src))
	if err != nil {
		t.Fatalf("%s: %v", slugifyFile, err)
	}
	var diff []string
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if utf16.IsSurrogate(r) {
			continue
		}
		if vsRemoved(r) != set[r] {
			diff = append(diff, fmt.Sprintf("%U removed=%v regex=%v", r, vsRemoved(r), set[r]))
		}
	}
	if len(diff) > 0 {
		t.Fatalf("%d code points differ, first %s", len(diff), strings.Join(diff[:min(len(diff), 10)], "; "))
	}
}

// slugifySet returns the code points that githubSlugReplaceRegex of slugify.ts
// matches. The expression is an alternation of a character class of UTF-16 code
// units, or of a high surrogate, written alone or as a class, followed by a class of
// low surrogates.
func slugifySet(src string) (map[rune]bool, error) {
	const prefix = "const githubSlugReplaceRegex = /"
	start := strings.Index(src, prefix)
	if start < 0 {
		return nil, fmt.Errorf("no %q", prefix)
	}
	body := src[start+len(prefix):]
	end := strings.Index(body, "/g;\n")
	if end < 0 {
		return nil, fmt.Errorf("no end of the expression")
	}
	set := map[rune]bool{}
	for _, alt := range splitAlternatives(body[:end]) {
		var atoms [][]uint16
		for p := 0; p < len(alt); {
			units, n, err := regexAtom(alt[p:])
			if err != nil {
				return nil, fmt.Errorf("%q: %w", alt, err)
			}
			atoms = append(atoms, units)
			p += n
		}
		switch len(atoms) {
		case 1:
			for _, u := range atoms[0] {
				set[rune(u)] = true
			}
		case 2:
			for _, hi := range atoms[0] {
				for _, lo := range atoms[1] {
					r := utf16.DecodeRune(rune(hi), rune(lo))
					if r == unicode.ReplacementChar {
						return nil, fmt.Errorf("%q: %04X %04X is no surrogate pair", alt, hi, lo)
					}
					set[r] = true
				}
			}
		default:
			return nil, fmt.Errorf("%q: %d atoms", alt, len(atoms))
		}
	}
	return set, nil
}

// splitAlternatives splits an expression at each '|' outside a class.
func splitAlternatives(s string) []string {
	var out []string
	depth, last := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '[':
			depth++
		case ']':
			depth--
		case '|':
			if depth == 0 {
				out = append(out, s[last:i])
				last = i + 1
			}
		}
	}
	return append(out, s[last:])
}

// regexAtom reads a class or a single escaped code unit at the start of s, and returns
// its code units and its length.
func regexAtom(s string) ([]uint16, int, error) {
	if s[0] != '[' {
		u, n, err := regexUnit(s)
		return []uint16{u}, n, err
	}
	var units []uint16
	p := 1
	for p < len(s) && s[p] != ']' {
		lo, n, err := regexUnit(s[p:])
		if err != nil {
			return nil, 0, err
		}
		p += n
		hi := lo
		if p+1 < len(s) && s[p] == '-' && s[p+1] != ']' {
			hi, n, err = regexUnit(s[p+1:])
			if err != nil {
				return nil, 0, err
			}
			p += 1 + n
		}
		if hi < lo {
			return nil, 0, fmt.Errorf("reversed range at %q", s[:p])
		}
		for u := int(lo); u <= int(hi); u++ {
			units = append(units, uint16(u))
		}
	}
	if p >= len(s) {
		return nil, 0, fmt.Errorf("unclosed class")
	}
	return units, p + 1, nil
}

// regexUnit reads one code unit: \0, \xHH, \uHHHH, an escaped ASCII punctuation
// character, or a printable ASCII character written as itself.
func regexUnit(s string) (uint16, int, error) {
	if s[0] != '\\' {
		if s[0] < 0x20 || s[0] > 0x7e {
			return 0, 0, fmt.Errorf("unexpected byte %q", s[0])
		}
		return uint16(s[0]), 1, nil
	}
	if len(s) < 2 {
		return 0, 0, fmt.Errorf("lone backslash")
	}
	hex := func(n int) (uint16, int, error) {
		if len(s) < 2+n {
			return 0, 0, fmt.Errorf("short escape %q", s)
		}
		v, err := strconv.ParseUint(s[2:2+n], 16, 16)
		return uint16(v), 2 + n, err
	}
	switch s[1] {
	case '0':
		return 0, 2, nil
	case 'x':
		return hex(2)
	case 'u':
		return hex(4)
	}
	if !isASCIIPunct(s[1]) {
		return 0, 0, fmt.Errorf("unknown escape %q", s[:2])
	}
	return uint16(s[1]), 2, nil
}
