package store

import (
	"strings"
	"testing"

	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

func typ(k dbspec.TypeKind) dbspec.Type { return dbspec.Type{Kind: k} }

func TestReadableValues(t *testing.T) {
	dec62 := dbspec.Type{Kind: dbspec.TypeDecimal, Precision: 6, Scale: 2}
	time3 := dbspec.Type{Kind: dbspec.TypeTime, Precision: 3}
	time0 := dbspec.Type{Kind: dbspec.TypeTime, Precision: 0}
	dt0 := dbspec.Type{Kind: dbspec.TypeDatetime, Precision: 0}
	vc3 := dbspec.Type{Kind: dbspec.TypeVarchar, Length: 3}
	cases := []struct {
		name string
		t    dbspec.Type
		in   string
		want string
		ok   bool
	}{
		{"10#readable-values: i32 accepts a canonical integer", typ(dbspec.TypeI32), "-3", "-3", true},
		{"10#readable-values: i32 canonicalizes leading zeros", typ(dbspec.TypeI32), "007", "7", true},
		{"10#readable-values: i32 refuses a plus sign", typ(dbspec.TypeI32), "+3", "", false},
		{"10#readable-values: i32 refuses a decimal point", typ(dbspec.TypeI32), "3.0", "", false},
		{"10#readable-values: i32 refuses the top of the range plus one", typ(dbspec.TypeI32), "2147483648", "", false},
		{"10#readable-values: i32 accepts the top of the range", typ(dbspec.TypeI32), "2147483647", "2147483647", true},
		{"10#readable-values: i16 refuses out of range", typ(dbspec.TypeI16), "32768", "", false},
		{"10#readable-values: i64 accepts the maximum", typ(dbspec.TypeI64), "9223372036854775807", "9223372036854775807", true},
		{"10#readable-values: i64 refuses past the maximum", typ(dbspec.TypeI64), "9223372036854775808", "", false},
		{"10#readable-values: decimal keeps its scale", dec62, "12.50", "12.50", true},
		{"10#readable-values: decimal pads to its scale", dec62, "12.5", "12.50", true},
		{"10#readable-values: decimal refuses more fraction digits than its scale", dec62, "12.555", "", false},
		{"10#readable-values: decimal refuses more integer digits than its precision", dec62, "1234567.00", "", false},
		{"10#readable-values: f64 writes its shortest form", typ(dbspec.TypeF64), "0.50", "0.5", true},
		{"10#readable-values: f64 writes zero as 0", typ(dbspec.TypeF64), "0.0", "0", true},
		{"10#readable-values: f64 refuses a word", typ(dbspec.TypeF64), "NaN", "", false},
		{"10#readable-values: bool accepts true", typ(dbspec.TypeBool), "true", "true", true},
		{"10#readable-values: bool refuses a capital", typ(dbspec.TypeBool), "True", "", false},
		{"10#readable-values: bool refuses a digit", typ(dbspec.TypeBool), "1", "", false},
		{"10#readable-values: date accepts a date", typ(dbspec.TypeDate), "2026-10-11", "2026-10-11", true},
		{"10#readable-values: date accepts a leap day", typ(dbspec.TypeDate), "2024-02-29", "2024-02-29", true},
		{"10#readable-values: date refuses a day past its month", typ(dbspec.TypeDate), "2026-02-30", "", false},
		{"10#readable-values: date refuses a leap day of a common year", typ(dbspec.TypeDate), "2026-02-29", "", false},
		{"10#readable-values: date refuses an unpadded month", typ(dbspec.TypeDate), "2026-1-1", "", false},
		{"10#readable-values: date refuses now", typ(dbspec.TypeDate), "now", "", false},
		{"10#readable-values: date refuses a quote inside", typ(dbspec.TypeDate), "2026'-10-11", "", false},
		{"10#readable-values: time writes the column's fraction digits", time3, "09:30:00", "09:30:00.000", true},
		{"10#readable-values: time keeps exact fraction digits", time3, "09:30:00.250", "09:30:00.250", true},
		{"10#readable-values: time refuses more fraction digits", time3, "09:30:00.2500", "", false},
		{"10#readable-values: time refuses hour 24", time3, "24:00:00", "", false},
		{"10#readable-values: time without fraction digits", time0, "09:30:00", "09:30:00", true},
		{"10#readable-values: time refuses a fraction for precision zero", time0, "09:30:00.5", "", false},
		{"10#readable-values: datetime accepts the space form", dt0, "2026-10-11 09:30:00", "2026-10-11 09:30:00", true},
		{"10#readable-values: datetime refuses the T form", dt0, "2026-10-11T09:30:00", "", false},
		{"10#readable-values: datetime refuses now", dt0, "now", "", false},
		{"10#readable-values: uuid is lowercase", typ(dbspec.TypeUUID), "0F8FAD5B-D9CB-469F-A165-8C9D2E7F1A2B", "0f8fad5b-d9cb-469f-a165-8c9d2e7f1a2b", true},
		{"10#readable-values: uuid refuses a non-uuid", typ(dbspec.TypeUUID), "not-a-uuid", "", false},
		{"10#readable-values: bytes are padded base64", typ(dbspec.TypeBytes), "AAEC/w==", "AAEC/w==", true},
		{"10#readable-values: bytes pad unpadded base64", typ(dbspec.TypeBytes), "AAEC/w", "AAEC/w==", true},
		{"10#readable-values: bytes accept the empty text", typ(dbspec.TypeBytes), "", "", true},
		{"10#readable-values: bytes refuse what is not base64", typ(dbspec.TypeBytes), "!!", "", false},
		{"10#readable-values: text is written as it is", typ(dbspec.TypeText), "  any <thing> ", "  any <thing> ", true},
		{"10#readable-values: varchar counts runes", vc3, "한글!", "한글!", true},
		{"10#readable-values: varchar refuses more runes than its length", vc3, "한글!!", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := readable(c.t, c.in)
			if ok != c.ok || got != c.want {
				t.Fatalf("readable(%s, %q) = %q, %v; want %q, %v", c.t, c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

// TestSingleLineFaults: every input pins 10#single-line-places (the faults of a
// single-line place, and the inputs that are not faults).
func TestSingleLineFaults(t *testing.T) {
	cases := []struct {
		in   string
		want bool // true when the value breaks a rule
	}{
		{"plain", false},
		{"", false},
		{"a < 3", false},
		{"a <3", false},
		{"inner space ok", false},
		{"a\nb", true},
		{"a\rb", true},
		{" lead", true},
		{"trail ", true},
		{"\ttab", true},
		{"<!-- c", true},
		{"a </b>", true},
		{"a <b>", true},
		{"a <!", true},
		{"a\x00b", true},
	}
	for _, c := range cases {
		if got := singleLineFault(c.in) != ""; got != c.want {
			t.Fatalf("singleLineFault(%q) = %q; breaks = %v, want %v", c.in, singleLineFault(c.in), got, c.want)
		}
	}
}

// TestInvisibleCharacters pins the invisible classes of spec 10 (Single-line
// places) under Unicode 17.0.0: a White_Space character other than U+1680, a
// control, a format character other than a Prepended_Concatenation_Mark, and a
// Default_Ignorable_Code_Point. One member of each class is named, and a
// Prepended_Concatenation_Mark of each of its ranges, so the classes are pinned.
func TestInvisibleCharacters(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want bool
	}{
		{"SPACE is White_Space", 0x0020, true},
		{"TAB is White_Space and Cc", 0x0009, true},
		{"LINE FEED is White_Space and Cc", 0x000A, true},
		{"VERTICAL TAB is White_Space and Cc", 0x000B, true},
		{"NEXT LINE is White_Space and Cc", 0x0085, true},
		{"NO-BREAK SPACE is White_Space", 0x00A0, true},
		{"EN QUAD is White_Space", 0x2000, true},
		{"LINE SEPARATOR is White_Space", 0x2028, true},
		{"IDEOGRAPHIC SPACE is White_Space", 0x3000, true},
		{"OGHAM SPACE MARK is White_Space but text", 0x1680, false},
		{"NUL is Cc", 0x0000, true},
		{"FILE SEPARATOR is Cc and not White_Space", 0x001C, true},
		{"UNIT SEPARATOR is Cc and not White_Space", 0x001F, true},
		{"DELETE is Cc", 0x007F, true},
		{"APPLICATION PROGRAM COMMAND is Cc", 0x009F, true},
		{"SOFT HYPHEN is Cf", 0x00AD, true},
		{"ZERO WIDTH SPACE is Cf", 0x200B, true},
		{"ZERO WIDTH NO-BREAK SPACE is Cf", 0xFEFF, true},
		{"LANGUAGE TAG is Cf", 0xE0001, true},
		{"ARABIC NUMBER SIGN is Cf and a Prepended_Concatenation_Mark", 0x0600, false},
		{"ARABIC END OF AYAH is Cf and a Prepended_Concatenation_Mark", 0x06DD, false},
		{"SYRIAC ABBREVIATION MARK is Cf and a Prepended_Concatenation_Mark", 0x070F, false},
		{"ARABIC DISPUTED END OF AYAH is a Prepended_Concatenation_Mark", 0x0890, false},
		{"ARABIC SMALL LOW WAW is a Prepended_Concatenation_Mark", 0x08E2, false},
		{"KAITHI NUMBER SIGN is a Prepended_Concatenation_Mark", 0x110BD, false},
		{"KAITHI NUMBER SIGN ABOVE is a Prepended_Concatenation_Mark", 0x110CD, false},
		{"COMBINING GRAPHEME JOINER is Default_Ignorable_Code_Point only", 0x034F, true},
		{"HANGUL FILLER is Default_Ignorable_Code_Point only", 0x3164, true},
		{"VARIATION SELECTOR-16 is Default_Ignorable_Code_Point only", 0xFE0F, true},
		{"reserved code point 2065 is Default_Ignorable_Code_Point", 0x2065, true},
		{"LATIN SMALL LETTER A", 0x0061, false},
		{"LATIN SMALL LETTER E WITH ACUTE", 0x00E9, false},
		{"CJK UNIFIED IDEOGRAPH-4E2D", 0x4E2D, false},
		{"HYPHEN", 0x2010, false},
		{"INTERLINEAR ANNOTATION ANCHOR is Cf, though excluded from Default_Ignorable_Code_Point", 0xFFF9, true},
		{"GRINNING FACE", 0x1F600, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := invisible(c.r); got != c.want {
				t.Fatalf("invisible(%U) = %v; want %v", c.r, got, c.want)
			}
		})
	}
}

// TestInvisibleAtEnd: an end is the first or the last character; a character
// inside the value is not an end, and an invalid byte is not a character.
func TestInvisibleAtEnd(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"a", false},
		{" ", true},
		{" a", true},
		{"a ", true},
		{"a b", false},
		{" a", true},
		{"a\t", true},
		{"a b", false},
		{" ", false},
		{" a ", false},
		{"\u06dda\u06dd", false},
		{"ㅤa", true},
		{"a͏", true},
		{"a\u200bb", false},
		{"a\xff", false},
	}
	for _, c := range cases {
		if got := invisibleAtEnd(c.in); got != c.want {
			t.Fatalf("invisibleAtEnd(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

// TestKeyRules: every input pins 20#keys (the key shape and the reserved names).
func TestKeyRules(t *testing.T) {
	long := strings.Repeat("a", 243)
	cases := []struct {
		key string
		ok  bool
	}{
		{"T1", true},
		{"a", true},
		{"7", true},
		{"a_b-c1", true},
		{"a-b", true},
		{"0f8fad5b-d9cb", true},
		{"", false},
		{"_a", false},
		{"-a", false},
		{"-7", false},
		{"a--b", true},
		{"a-_b", false},
		{"a_-b", true},
		{"a-", true},
		{"a-b-", true},
		{"7-", true},
		{"a.b", false},
		{"a b", false},
		{"a/b", false},
		{"가", false},
		{"CON", false},
		{"con", false},
		{"Prn", false},
		{"aux", false},
		{"NUL", false},
		{"com1", false},
		{"COM9", false},
		{"LPT0", false},
		{"lpt9", false},
		{"COM10", true},
		{"CONX", true},
		{long, true},
		{long + "a", false},
	}
	for _, c := range cases {
		name := c.key
		if len(name) > 12 {
			name = name[:12] + "..."
		}
		t.Run(name, func(t *testing.T) {
			if got := keyFault(c.key) == ""; got != c.ok {
				t.Fatalf("keyFault(%q) = %q; ok = %v, want %v", c.key, keyFault(c.key), got, c.ok)
			}
		})
	}
}

// TestSplitLink: every input pins 10#foreign-keys-as-links (the link form of a
// foreign key value).
func TestSplitLink(t *testing.T) {
	cases := []struct {
		in, key, target string
		ok              bool
	}{
		{"[T1](T1.flowmark.md)", "T1", "T1.flowmark.md", true},
		{"[P3](../posts/P3.flowmark.md)", "P3", "../posts/P3.flowmark.md", true},
		{"[T2](#items-row-t2)", "T2", "#items-row-t2", true},
		{"[](x)", "", "", false},
		{"T1", "", "", false},
		{"[T1]T1.flowmark.md", "", "", false},
		{"[T1](T1.flowmark.md", "", "", false},
		{"[a]b](t)", "", "", false},
		{"[T1](T1.flowmark.md) x", "", "", false},
	}
	for _, c := range cases {
		key, target, ok := splitLink(c.in)
		if key != c.key || target != c.target || ok != c.ok {
			t.Fatalf("splitLink(%q) = %q, %q, %v; want %q, %q, %v", c.in, key, target, ok, c.key, c.target, c.ok)
		}
	}
}

// TestTitleEscape: every input pins 10#single-line-places (the escape of a title
// in an H1 and its read-back).
func TestTitleEscape(t *testing.T) {
	cases := []struct {
		title, text string
		ok          bool // the title writes and reads back unchanged
	}{
		{"Plain", "Plain", true},
		{"a#", "a#", true},
		{"Count the #", `Count the \#`, true},
		{"x ##", `x #\#`, true},
		{"a\t#", "a\t\\#", true},
		{"#", `\#`, true},
		{"##", `#\#`, true},
		{"x \\#", `x \#`, false},
		{"x\\#", `x\#`, false},
		{"#a", "#a", true},
	}
	for _, c := range cases {
		got := escapeTitle(c.title)
		if c.ok && got != c.text {
			t.Fatalf("escapeTitle(%q) = %q, want %q", c.title, got, c.text)
		}
		back := unescapeTitle(got)
		if (back == c.title) != c.ok {
			t.Fatalf("title %q writes %q and reads back %q; round trip ok = %v, want %v", c.title, got, back, back == c.title, c.ok)
		}
	}
	for _, text := range []string{`a \#`, `x #\#`, `Count the \#`, "plain"} {
		if got := escapeTitle(unescapeTitle(text)); got != text {
			t.Fatalf("H1 text %q reads as %q and writes back as %q", text, unescapeTitle(text), got)
		}
	}
}

// TestUnescapeTitle: every input pins 10#single-line-places (the H1 title read:
// CommonMark's leading and trailing spaces and tabs and its closing sequence are
// removed, and a final \# reads as #).
func TestUnescapeTitle(t *testing.T) {
	cases := []struct{ text, title string }{
		{"a #", "a"},
		{" a", "a"},
		{"a\t#", "a"},
		{"a ", "a"},
		{"#", ""},
		{`\#`, "#"},
		{`C\#`, "C#"},
		{`x #\#`, "x ##"},
		{`a\\#`, `a\#`},
		{"a#", "a#"},
		{"a \\#b", "a \\#b"},
	}
	for _, c := range cases {
		if got := unescapeTitle(c.text); got != c.title {
			t.Fatalf("unescapeTitle(%q) = %q, want %q", c.text, got, c.title)
		}
	}
}
