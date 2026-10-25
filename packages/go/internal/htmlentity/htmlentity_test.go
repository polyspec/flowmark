package htmlentity

import (
	"github.com/polyspec/flowmark/packages/go/internal/tabletest"
	"os"
	"testing"
)

// TestTableIsFresh requires table.go to be the rendering of the data file, so a
// table edited by hand, or left behind by a changed data file, fails here.
func TestTableIsFresh(t *testing.T) {
	data, err := os.ReadFile("testdata/entities-" + Version + ".json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := Render(data)
	if err != nil {
		t.Fatal(err)
	}
	tabletest.Fresh(t, want, "the data file")
}

// TestReference pins the references that CommonMark's entity pattern reads, the
// strict name decoding, and the replacement of invalid numeric references.
func TestReference(t *testing.T) {
	cases := []struct {
		name string
		in   string
		text string
		n    int
	}{
		{"named with semicolon", "&amp;x", "&", 5},
		{"named letter with accent", "&eacute;", "é", 8},
		{"named with two code points", "&acE;", "∾̳", 5},
		{"name is case sensitive, Aelig is an entry", "&AElig;", "Æ", 7},
		{"name in lower case that is no entry", "&AELIG;", "", 0},
		{"named without semicolon", "&amp", "", 0},
		{"unknown name", "&notanentity;", "", 0},
		{"name of one letter", "&a;", "", 0},
		{"name that begins with a digit", "&1a;", "", 0},
		{"name of thirty-three characters", "&" + "a" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa;", "", 0},
		{"decimal", "&#65;", "A", 5},
		{"hexadecimal with capital X", "&#X41;", "A", 6},
		{"hexadecimal", "&#x41;", "A", 6},
		{"hexadecimal without semicolon", "&#x41", "", 0},
		{"decimal of seven digits at the last code point", "&#1114111;", "\U0010FFFF", 10},
		{"decimal above the last code point", "&#1114112;", "�", 10},
		{"decimal of eight digits", "&#12345678;", "", 0},
		{"hexadecimal of six digits", "&#x10FFFF;", "\U0010FFFF", 10},
		{"hexadecimal of seven digits", "&#x1234567;", "", 0},
		{"zero is replaced", "&#0;", "�", 4},
		{"C1 control is its code point, not windows-1252", "&#x80;", "\u0080", 6},
		{"C1 control 0x83 is its code point", "&#131;", "\u0083", 6},
		{"C1 control 0x81 is its code point", "&#x81;", "\u0081", 6},
		{"surrogate is replaced", "&#xD800;", "�", 8},
		{"code point above U+10FFFF is replaced", "&#x110000;", "�", 10},
		{"empty numeric", "&#;", "", 0},
		{"ampersand alone", "& ", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, n := Reference(c.in)
			if text != c.text || n != c.n {
				t.Fatalf("Reference(%q) = %q, %d; want %q, %d", c.in, text, n, c.text, c.n)
			}
		})
	}
}

// TestExpand pins the decoding of an attribute value: its references are replaced,
// the rest stays.
func TestExpand(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"plain", "plain"},
		{"a&amp;b", "a&b"},
		{"&eacute;t&eacute;", "été"},
		{"&nope; &#x41;", "&nope; A"},
		{"&amp", "&amp"},
		{"&#x80;&#x83;&#x81;", "€ƒ\u0081"},
		{"&#0;&#xD800;", "��"},
	}
	for _, c := range cases {
		if got := Expand(c[0]); got != c[1] {
			t.Fatalf("Expand(%q) = %q; want %q", c[0], got, c[1])
		}
	}
}
