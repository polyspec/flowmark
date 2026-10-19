package unicodedcp

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
)

const (
	dataFile    = "testdata/DerivedCoreProperties-17.0.0.txt"
	licenseFile = "testdata/UNICODE-LICENSE.txt"
)

// TestUnicodeLicense requires the Unicode License v3, under which the data file is
// distributed, to stand beside it in full: its copyright and permission notice must
// appear with every copy.
func TestUnicodeLicense(t *testing.T) {
	src, err := os.ReadFile(licenseFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.HasPrefix(text, "UNICODE LICENSE V3\n") {
		t.Fatalf("%s does not begin with the license title", licenseFile)
	}
	for _, want := range []string{"COPYRIGHT AND PERMISSION NOTICE", "Permission is hereby granted, free of charge,"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%s lacks %q", licenseFile, want)
		}
	}
}

// TestTableIsFresh requires table.go to be the rendering of the data file, so a
// table edited by hand, or left behind by a changed data file, fails here.
func TestTableIsFresh(t *testing.T) {
	f, err := os.Open(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	ranges, err := Parse(f)
	if err != nil {
		t.Fatalf("%s: %v", dataFile, err)
	}
	want, err := Render(ranges)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("table.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("table.go is not the rendering of " + dataFile + ": run go generate in internal/unicodedcp")
	}
}

// TestDefaultIgnorable pins the code points named in the data file's header and
// in the single-line rule of spec 10: the members of the property, and the
// White_Space, Cc and Prepended_Concatenation_Mark characters that are not.
func TestDefaultIgnorable(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want bool
	}{
		{"LATIN CAPITAL LETTER A", 0x0041, false},
		{"SPACE is White_Space, not Default_Ignorable", 0x0020, false},
		{"NO-BREAK SPACE is White_Space, not Default_Ignorable", 0x00A0, false},
		{"IDEOGRAPHIC SPACE is White_Space, not Default_Ignorable", 0x3000, false},
		{"OGHAM SPACE MARK is White_Space, not Default_Ignorable", 0x1680, false},
		{"ARABIC NUMBER SIGN is a Prepended_Concatenation_Mark", 0x0600, false},
		{"ARABIC END OF AYAH is a Prepended_Concatenation_Mark", 0x06DD, false},
		{"FILE SEPARATOR is Cc, not Default_Ignorable", 0x001C, false},
		{"INTERLINEAR ANNOTATION ANCHOR is excluded from the property", 0xFFF9, false},
		{"SOFT HYPHEN", 0x00AD, true},
		{"COMBINING GRAPHEME JOINER", 0x034F, true},
		{"ARABIC LETTER MARK", 0x061C, true},
		{"HANGUL CHOSEONG FILLER", 0x115F, true},
		{"HANGUL JUNGSEONG FILLER", 0x1160, true},
		{"HANGUL JUNGSEONG A is not in the range", 0x1161, false},
		{"ZERO WIDTH SPACE", 0x200B, true},
		{"RIGHT-TO-LEFT MARK", 0x200F, true},
		{"the character after the marks is not", 0x2010, false},
		{"reserved code point 2065", 0x2065, true},
		{"HANGUL FILLER", 0x3164, true},
		{"VARIATION SELECTOR-16", 0xFE0F, true},
		{"VARIATION SELECTOR-17 is in the supplement range", 0xE0100, true},
		{"the end of the supplement range", 0xE01EF, true},
		{"ZERO WIDTH NO-BREAK SPACE", 0xFEFF, true},
		{"HALFWIDTH HANGUL FILLER", 0xFFA0, true},
		{"LANGUAGE TAG", 0xE0001, true},
		{"reserved code point E0FFF", 0xE0FFF, true},
		{"the last code point", 0x10FFFF, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DefaultIgnorable(c.r); got != c.want {
				t.Fatalf("DefaultIgnorable(%U) = %v; want %v", c.r, got, c.want)
			}
		})
	}
}

// TestParse: a file of UnicodeVersion with its header is read to its ranges of
// the property; any other file, or a range that is malformed, is refused.
func TestParse(t *testing.T) {
	const header = "# DerivedCoreProperties-17.0.0.txt\n"
	cases := []struct {
		name string
		text string
		want []Range // nil when refused
	}{
		{"one range of the property", header + "00AD          ; Default_Ignorable_Code_Point # Cf       SOFT HYPHEN\n", []Range{{0xAD, 0xAD}}},
		{"a range and a line of another property", header + "0041..0042 ; Alphabetic # L&\n0100..0101 ; Default_Ignorable_Code_Point # Cf\n", []Range{{0x100, 0x101}}},
		{"ranges in order", header + "0010..0012 ; Default_Ignorable_Code_Point\n0014 ; Default_Ignorable_Code_Point\n", []Range{{0x10, 0x12}, {0x14, 0x14}}},
		{"another version", "# DerivedCoreProperties-16.0.0.txt\n0041 ; Default_Ignorable_Code_Point\n", nil},
		{"no header", "0041 ; Default_Ignorable_Code_Point\n", nil},
		{"no range of the property", header + "0041 ; Alphabetic\n", nil},
		{"an empty file", "", nil},
		{"a reversed range", header + "0012..0010 ; Default_Ignorable_Code_Point\n", nil},
		{"overlapping ranges", header + "0010..0020 ; Default_Ignorable_Code_Point\n0018 ; Default_Ignorable_Code_Point\n", nil},
		{"out of order ranges", header + "0030 ; Default_Ignorable_Code_Point\n0010 ; Default_Ignorable_Code_Point\n", nil},
		{"a code point past U+10FFFF", header + "110000 ; Default_Ignorable_Code_Point\n", nil},
		{"a code point that is not hexadecimal", header + "00G1 ; Default_Ignorable_Code_Point\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(c.text))
			if c.want == nil {
				if err == nil {
					t.Fatalf("Parse accepted %q: %v", c.text, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse refused: %v", err)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("Parse = %v; want %v", got, c.want)
			}
		})
	}
}
