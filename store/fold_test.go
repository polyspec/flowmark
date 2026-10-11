package store

import (
	"testing"

	"golang.org/x/text/unicode/norm"
)

// TestUnicodeVersion requires the NFKD tables of the module to be those of Unicode
// 17.0.0, the version spec 10 names, so a bump of golang.org/x/text that changes the
// version fails here instead of changing the folds quietly.
func TestUnicodeVersion(t *testing.T) {
	if norm.Version != "17.0.0" {
		t.Fatalf("norm.Version = %q; spec 10 needs 17.0.0", norm.Version)
	}
}

// TestFolds pins F_G, the fold of a heading's text. The cases name what it keeps: it
// removes the characters that are neither letters nor decimal digits before the NFKD
// form, so a superscript two and a roman numeral vanish under it.
func TestFolds(t *testing.T) {
	cases := []struct {
		name, text, g string
	}{
		{"ASCII letters and digits", "Items row T1", "itemsrowt1"},
		{"hyphen, underscore and space are removed", "a-b_c d", "abcd"},
		{"accent is dropped after decomposition", "café", "cafe"},
		{"capital accent is lowercased and dropped", "CAFÉ", "cafe"},
		{"combining mark alone", "á", "a"},
		{"superscript two is removed", "x²", "x"},
		{"superscript digit after a digit", "1²", "1"},
		{"fullwidth letter and digit decompose", "ｔ１", "t1"},
		{"ligature decomposes", "ﬁle", "file"},
		{"Kelvin sign lowercases to k", "K", "k"},
		{"dotted capital I lowercases to i", "İ", "i"},
		{"sharp s has no decomposition", "ß", ""},
		{"roman numeral is removed", "Ⅻ", ""},
		{"ideographs have no ASCII form", "日本", ""},
		{"empty text", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if g := foldG(c.text); g != c.g {
				t.Fatalf("foldG(%q) = %q; want %q", c.text, g, c.g)
			}
		})
	}
}

// TestStripDigits pins the removal of the final run of decimal digits, which is
// applied to each fold once: a run that is not final stays.
func TestStripDigits(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"itemsrowt", "itemsrowt"},
		{"itemsrowt1", "itemsrowt"},
		{"itemsrowt12", "itemsrowt"},
		{"item1srowt", "item1srowt"},
		{"1", ""},
		{"12a1", "12a"},
	}
	for _, c := range cases {
		if got := stripDigits(c[0]); got != c[1] {
			t.Fatalf("stripDigits(%q) = %q; want %q", c[0], got, c[1])
		}
	}
}
