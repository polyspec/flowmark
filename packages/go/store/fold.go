package store

import (
	"strings"
	"unicode"

	"github.com/polyspec/flowmark/packages/go/internal/unicode13"
)

// The slugs of a heading's rendered text that decide a detail-anchor collision (spec 10,
// Detail headings and anchors), each as its viewer computes it. S_GH is the slug of
// GitHub's renderer and S_VS that of VS Code's slugify.ts, each reproducing the ids that
// the viewer renders, recorded by scripts/record-slugs.mjs into testdata/slug; S_GL is
// comrak 0.56.0's Anchorizer, which GitLab renders headings with through
// gitlab-glfm-markdown 0.0.45. None of them normalizes the text, trims hyphens or
// collapses runs.

// lowerFull lowercases s by the full default lowercase mapping of Unicode without its
// conditional mappings: the simple mapping of each character, except U+0130, whose
// lowercase is i followed by U+0307, the one unconditional mapping of SpecialCasing.txt
// to more than one character. GitHub's ids lowercase so, a final Σ included
// (testdata/slug).
func lowerFull(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "\U00000130", "i\U00000307"))
}

// slugGH is S_GH, the slug of GitHub's heading ids: the text lowercased by lowerFull,
// every character kept that ghWord names, and the space and '-', the others removed,
// and each space then written '-'.
func slugGH(s string) string {
	var b strings.Builder
	for _, r := range lowerFull(s) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || ghWord(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ghWord reports whether GitHub keeps r in an id: r is a word character of Unicode
// Technical Standard #18 (Annex C), Alphabetic, a mark, a decimal number, connector
// punctuation or Join_Control, by the Unicode 17.0.0 data of Go's unicode package
// (TestUnicodeVersion), which GitHub's ids follow over every code point.
func ghWord(r rune) bool {
	return unicode.In(r, unicode.L, unicode.Nl, unicode.Other_Alphabetic, unicode.M, unicode.Nd, unicode.Pc, unicode.Join_Control)
}

// slugVS is S_VS, VS Code's slug (githubSlugifier.fromHeading of its slugify.ts): the
// text without the white space that JavaScript's String.prototype.trim removes at its
// ends, lowercased by JavaScript's toLowerCase (lowerJS), the characters that
// vsRemoved names removed, and each JavaScript white space character (\s) then written
// '-'.
func slugVS(s string) string {
	var b strings.Builder
	for _, r := range lowerJS(strings.TrimFunc(s, jsTrimmed)) {
		switch {
		case vsRemoved(r):
		case jsTrimmed(r):
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// vsRemoved reports whether the expression githubSlugReplaceRegex of VS Code's
// slugify.ts matches r: r is neither the space, nor '-', nor Alphabetic in Unicode
// 13.0.0, and its general category in Unicode 13.0.0 is No, Pd, Ps, Pe, Pi, Pf, Po, Sm,
// Sc, Sk, So, Cc, Cf, Co, Cn, Zs, Zl or Zp. A character assigned after 13.0.0 is Cn
// there, so it is removed. TestVSRemovalIsVSCodeRegex requires this to be the set of
// the expression in the vendored slugify.ts.
func vsRemoved(r rune) bool {
	if r == ' ' || r == '-' || unicode13.Alphabetic(r) {
		return false
	}
	switch unicode13.Category(r) {
	case "No", "Pd", "Ps", "Pe", "Pi", "Pf", "Po", "Sm", "Sc", "Sk", "So", "Cc", "Cf", "Co", "Cn", "Zs", "Zl", "Zp":
		return true
	}
	return false
}

// lowerJS lowercases s as JavaScript's String.prototype.toLowerCase does: lowerFull,
// except that Σ is ς at the end of a word, the Final_Sigma condition of
// SpecialCasing.txt: a cased character precedes it, and none follows it, with
// case-ignorable characters skipped on both sides.
func lowerJS(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if r == 'Σ' && finalSigma(rs, i) {
			b.WriteRune('ς')
			continue
		}
		b.WriteString(lowerFull(string(r)))
	}
	return b.String()
}

// finalSigma reports whether the Σ at i of rs meets the Final_Sigma condition.
func finalSigma(rs []rune, i int) bool {
	before := false
	for j := i - 1; j >= 0; j-- {
		if !caseIgnorable(rs[j]) {
			before = cased(rs[j])
			break
		}
	}
	if !before {
		return false
	}
	for j := i + 1; j < len(rs); j++ {
		if !caseIgnorable(rs[j]) {
			return !cased(rs[j])
		}
	}
	return true
}

// cased reports whether r has the Cased property: Lowercase, Uppercase or Lt.
func cased(r rune) bool {
	return unicode.In(r, unicode.Ll, unicode.Lu, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

// caseIgnorable reports whether r has the Case_Ignorable property: its category is Mn,
// Me, Cf, Lm or Sk, or its Word_Break is MidLetter, MidNumLet or Single_Quote.
func caseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '·', '·', '՟', '״', '‘', '’', '․', '‧',
		'︓', '﹒', '﹕', '＇', '．', '：':
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

// jsTrimmed reports whether String.prototype.trim removes r, which is also what \s of a
// JavaScript expression matches: an ECMAScript WhiteSpace (tab, vertical tab, form
// feed, U+FEFF and the category Zs) or LineTerminator (line feed, carriage return,
// U+2028 and U+2029).
func jsTrimmed(r rune) bool {
	switch r {
	case '\t', '\v', '\f', '\U0000FEFF', '\n', '\r', '\U00002028', '\U00002029':
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// slugGL is S_GL, comrak's Anchorizer: the text lowercased by Rust's to_lowercase,
// which is lowerJS, the full mapping with the Final_Sigma condition (GitLab writes a
// final Σ as ς, testdata/slug), every character kept whose general category is a
// letter, a mark, a number or connector punctuation, and the space and '-', the others
// removed, and each space then written '-'. comrak reads the categories from
// finl_unicode 1.4.0, which gitlab-glfm-markdown 0.0.45's Cargo.lock pins, and whose
// tables are those of Unicode 17.0.0, the version of Go's unicode package
// (TestUnicodeVersion).
func slugGL(s string) string {
	var b strings.Builder
	for _, r := range lowerJS(s) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.Pc):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// takesAnchor reports whether a heading of the slug s takes the anchor a: s is a, or a
// is s, '-' and a decimal number of one or more written without a leading zero, the
// number a viewer appends to a repeated slug.
func takesAnchor(s, a string) bool {
	if s == a {
		return true
	}
	n, ok := strings.CutPrefix(a, s+"-")
	if !ok || n == "" || n[0] == '0' {
		return false
	}
	for i := 0; i < len(n); i++ {
		if n[i] < '0' || n[i] > '9' {
			return false
		}
	}
	return true
}
