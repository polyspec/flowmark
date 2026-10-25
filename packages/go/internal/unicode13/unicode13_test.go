package unicode13

import (
	"github.com/polyspec/flowmark/packages/go/internal/tabletest"
	"slices"
	"strings"
	"testing"
)

// licenseFile is the Unicode License v3 beside the data files.
const licenseFile = "testdata/UNICODE-LICENSE.txt"

// TestUnicodeLicense requires the Unicode License v3, under which the data files are
// distributed, to stand beside them in full: its copyright and permission notice must
// appear with every copy.
func TestUnicodeLicense(t *testing.T) { tabletest.License(t, licenseFile) }

// TestTableIsFresh requires table.go to be the rendering of the data files, so a table
// edited by hand, or left behind by a changed data file, fails here.
func TestTableIsFresh(t *testing.T) {
	want, err := Table()
	if err != nil {
		t.Fatal(err)
	}
	tabletest.Fresh(t, want, "the data files")
}

// TestCategory pins code points whose category in Unicode 13.0.0 VS Code's slug
// reads, those assigned after 13.0.0 among them.
func TestCategory(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want string
	}{
		{"NULL", 0x0000, "Cc"},
		{"SPACE", 0x0020, "Zs"},
		{"HYPHEN-MINUS", 0x002D, "Pd"},
		{"DIGIT ZERO", 0x0030, "Nd"},
		{"LATIN CAPITAL LETTER A", 0x0041, "Lu"},
		{"LOW LINE", 0x005F, "Pc"},
		{"LATIN SMALL LETTER A", 0x0061, "Ll"},
		{"NO-BREAK SPACE", 0x00A0, "Zs"},
		{"SUPERSCRIPT TWO", 0x00B2, "No"},
		{"COMBINING ACUTE ACCENT", 0x0301, "Mn"},
		{"reserved 0378", 0x0378, "Cn"},
		{"ZERO WIDTH JOINER", 0x200D, "Cf"},
		{"EN DASH", 0x2013, "Pd"},
		{"LINE SEPARATOR", 0x2028, "Zl"},
		{"PARAGRAPH SEPARATOR", 0x2029, "Zp"},
		{"ROMAN NUMERAL TWELVE", 0x216B, "Nl"},
		{"CIRCLED LATIN CAPITAL LETTER A", 0x24B6, "So"},
		{"CJK UNIFIED IDEOGRAPH-65E5", 0x65E5, "Lo"},
		{"MODIFIER LETTER CAPITAL C, assigned in 14.0", 0xA7F2, "Cn"},
		{"a high surrogate", 0xD800, "Cs"},
		{"a private use character", 0xE000, "Co"},
		{"a noncharacter", 0xFFFF, "Cn"},
		{"MODIFIER LETTER SMALL Q, assigned in 14.0", 0x107A5, "Cn"},
		{"OUTLINED DIGIT ZERO, assigned in 16.0", 0x1CCF0, "Cn"},
		{"CJK UNIFIED IDEOGRAPH-3134A, the last of 13.0", 0x3134A, "Lo"},
		{"the code point after it", 0x3134B, "Cn"},
		{"the last private use code point", 0x10FFFD, "Co"},
		{"the last code point", 0x10FFFF, "Cn"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Category(c.r); got != c.want {
				t.Fatalf("Category(%U) = %s; want %s", c.r, got, c.want)
			}
		})
	}
}

// TestAlphabetic pins the Alphabetic code points that are not letters, which
// VS Code's slug keeps, and characters that are not Alphabetic.
func TestAlphabetic(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want bool
	}{
		{"LATIN CAPITAL LETTER A", 0x0041, true},
		{"DIGIT ZERO", 0x0030, false},
		{"LOW LINE", 0x005F, false},
		{"COMBINING GREEK YPOGEGRAMMENI is Other_Alphabetic", 0x0345, true},
		{"COMBINING ACUTE ACCENT", 0x0301, false},
		{"ROMAN NUMERAL TWELVE is Nl", 0x216B, true},
		{"CIRCLED LATIN CAPITAL LETTER A is Other_Alphabetic", 0x24B6, true},
		{"CIRCLED LATIN SMALL LETTER Z is Other_Alphabetic", 0x24E9, true},
		{"CIRCLED DIGIT ONE", 0x2460, false},
		{"MODIFIER LETTER CAPITAL C, assigned in 14.0", 0xA7F2, false},
		{"CJK UNIFIED IDEOGRAPH-3134A", 0x3134A, true},
		{"the last code point", 0x10FFFF, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Alphabetic(c.r); got != c.want {
				t.Fatalf("Alphabetic(%U) = %v; want %v", c.r, got, c.want)
			}
		})
	}
}

// fullCover is a category file of UnicodeVersion whose ranges cover every code point.
const fullCover = "# DerivedGeneralCategory-13.0.0.txt\n0000..0040 ; Cc\n0041..0042 ; Lu # two\n0043..10FFFF ; Cn\n"

// TestParseCategories: a file of UnicodeVersion with its header whose ranges cover
// every code point once is read to its ranges, adjacent ranges of one category joined;
// any other file is refused.
func TestParseCategories(t *testing.T) {
	const header = "# DerivedGeneralCategory-13.0.0.txt\n"
	cases := []struct {
		name string
		text string
		want []CategoryRange // nil when refused
	}{
		{"ranges that cover every code point", fullCover, []CategoryRange{{0, 0x40, "Cc"}, {0x41, 0x42, "Lu"}, {0x43, 0x10FFFF, "Cn"}}},
		{"ranges out of order", header + "0041..10FFFF ; Cn\n0000..0040 ; Cc\n", []CategoryRange{{0, 0x40, "Cc"}, {0x41, 0x10FFFF, "Cn"}}},
		{"adjacent ranges of one category are joined", header + "0000..0040 ; Cn\n0041 ; Cn\n0042..10FFFF ; Lo\n", []CategoryRange{{0, 0x41, "Cn"}, {0x42, 0x10FFFF, "Lo"}}},
		{"another version", strings.Replace(fullCover, "13.0.0", "14.0.0", 1), nil},
		{"no header", strings.TrimPrefix(fullCover, header), nil},
		{"an empty file", "", nil},
		{"a gap", header + "0000..0040 ; Cc\n0042..10FFFF ; Cn\n", nil},
		{"a gap at the start", header + "0001..10FFFF ; Cn\n", nil},
		{"a gap at the end", header + "0000..10FFFE ; Cn\n", nil},
		{"an overlap", header + "0000..0041 ; Cc\n0041..10FFFF ; Cn\n", nil},
		{"an unknown category", header + "0000..10FFFF ; Xx\n", nil},
		{"a reversed range", header + "0041..0040 ; Cc\n", nil},
		{"a line without a semicolon", header + "0000..10FFFF Cn\n", nil},
		{"a code point past U+10FFFF", header + "0000..110000 ; Cn\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseCategories(strings.NewReader(c.text))
			if c.want == nil {
				if err == nil {
					t.Fatalf("ParseCategories accepted %q: %v", c.text, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCategories refused: %v", err)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("ParseCategories = %v; want %v", got, c.want)
			}
		})
	}
}

// TestParseAlphabetic: a file of UnicodeVersion with its header is read to its ranges
// of Alphabetic, adjacent ranges joined; any other file, or a range that is malformed
// or overlaps another, is refused.
func TestParseAlphabetic(t *testing.T) {
	const header = "# DerivedCoreProperties-13.0.0.txt\n"
	cases := []struct {
		name string
		text string
		want []Range // nil when refused
	}{
		{"one range of the property", header + "0041..005A ; Alphabetic # L&\n", []Range{{0x41, 0x5A}}},
		{"a line of another property is skipped", header + "0041 ; Math\n0042 ; Alphabetic\n", []Range{{0x42, 0x42}}},
		{"adjacent ranges are joined", header + "0041 ; Alphabetic\n0042..0043 ; Alphabetic\n0045 ; Alphabetic\n", []Range{{0x41, 0x43}, {0x45, 0x45}}},
		{"ranges out of order", header + "0045 ; Alphabetic\n0041 ; Alphabetic\n", []Range{{0x41, 0x41}, {0x45, 0x45}}},
		{"another version", "# DerivedCoreProperties-17.0.0.txt\n0041 ; Alphabetic\n", nil},
		{"no header", "0041 ; Alphabetic\n", nil},
		{"an empty file", "", nil},
		{"no range of the property", header + "0041 ; Math\n", nil},
		{"overlapping ranges", header + "0041..0045 ; Alphabetic\n0043 ; Alphabetic\n", nil},
		{"a reversed range", header + "0045..0041 ; Alphabetic\n", nil},
		{"a code point that is not hexadecimal", header + "00G1 ; Alphabetic\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseAlphabetic(strings.NewReader(c.text))
			if c.want == nil {
				if err == nil {
					t.Fatalf("ParseAlphabetic accepted %q: %v", c.text, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAlphabetic refused: %v", err)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("ParseAlphabetic = %v; want %v", got, c.want)
			}
		})
	}
}
