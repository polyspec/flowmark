package store

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// The fold of a heading's rendered text that decides a detail-anchor collision (spec
// 10, Detail headings and anchors). It ends with this step: the text is decomposed by
// NFKD, and only the ASCII letters and digits of the result are kept. Combining marks
// are not ASCII, so that step also drops them.

// foldG is F_G: the characters that are neither letters nor decimal digits are
// removed first, so a superscript digit or a roman numeral is gone before the
// decomposition, as GitHub, GitLab and VS Code keep them.
func foldG(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return asciiAlnum(norm.NFKD.String(strings.ToLower(b.String())))
}

// asciiAlnum keeps the ASCII letters and digits of s.
func asciiAlnum(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if c := s[i]; isASCIILetter(c) || ('0' <= c && c <= '9') {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// stripDigits removes the final run of decimal digits of s, which the fold of a
// heading loses before its comparison with an anchor, since a viewer numbers a
// repeated anchor by appending -<n>.
func stripDigits(s string) string {
	return strings.TrimRight(s, "0123456789")
}
