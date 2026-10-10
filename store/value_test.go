package store

import (
	"strings"
	"testing"

	"github.com/polyspec/orm/engine/dbspec"
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
		{"i32 accepts a canonical integer", typ(dbspec.TypeI32), "-3", "-3", true},
		{"i32 canonicalizes leading zeros", typ(dbspec.TypeI32), "007", "7", true},
		{"i32 refuses a plus sign", typ(dbspec.TypeI32), "+3", "", false},
		{"i32 refuses a decimal point", typ(dbspec.TypeI32), "3.0", "", false},
		{"i32 refuses the top of the range plus one", typ(dbspec.TypeI32), "2147483648", "", false},
		{"i32 accepts the top of the range", typ(dbspec.TypeI32), "2147483647", "2147483647", true},
		{"i16 refuses out of range", typ(dbspec.TypeI16), "32768", "", false},
		{"i64 accepts the maximum", typ(dbspec.TypeI64), "9223372036854775807", "9223372036854775807", true},
		{"i64 refuses past the maximum", typ(dbspec.TypeI64), "9223372036854775808", "", false},
		{"decimal keeps its scale", dec62, "12.50", "12.50", true},
		{"decimal pads to its scale", dec62, "12.5", "12.50", true},
		{"decimal refuses more fraction digits than its scale", dec62, "12.555", "", false},
		{"decimal refuses more integer digits than its precision", dec62, "1234567.00", "", false},
		{"f64 writes its shortest form", typ(dbspec.TypeF64), "0.50", "0.5", true},
		{"f64 writes zero as 0", typ(dbspec.TypeF64), "0.0", "0", true},
		{"f64 refuses a word", typ(dbspec.TypeF64), "NaN", "", false},
		{"bool accepts true", typ(dbspec.TypeBool), "true", "true", true},
		{"bool refuses a capital", typ(dbspec.TypeBool), "True", "", false},
		{"bool refuses a digit", typ(dbspec.TypeBool), "1", "", false},
		{"date accepts a date", typ(dbspec.TypeDate), "2026-10-11", "2026-10-11", true},
		{"date accepts a leap day", typ(dbspec.TypeDate), "2024-02-29", "2024-02-29", true},
		{"date refuses a day past its month", typ(dbspec.TypeDate), "2026-02-30", "", false},
		{"date refuses a leap day of a common year", typ(dbspec.TypeDate), "2026-02-29", "", false},
		{"date refuses an unpadded month", typ(dbspec.TypeDate), "2026-1-1", "", false},
		{"date refuses now", typ(dbspec.TypeDate), "now", "", false},
		{"date refuses a quote inside", typ(dbspec.TypeDate), "2026'-10-11", "", false},
		{"time writes the column's fraction digits", time3, "09:30:00", "09:30:00.000", true},
		{"time keeps exact fraction digits", time3, "09:30:00.250", "09:30:00.250", true},
		{"time refuses more fraction digits", time3, "09:30:00.2500", "", false},
		{"time refuses hour 24", time3, "24:00:00", "", false},
		{"time without fraction digits", time0, "09:30:00", "09:30:00", true},
		{"time refuses a fraction for precision zero", time0, "09:30:00.5", "", false},
		{"datetime accepts the space form", dt0, "2026-10-11 09:30:00", "2026-10-11 09:30:00", true},
		{"datetime refuses the T form", dt0, "2026-10-11T09:30:00", "", false},
		{"datetime refuses now", dt0, "now", "", false},
		{"uuid is lowercase", typ(dbspec.TypeUUID), "0F8FAD5B-D9CB-469F-A165-8C9D2E7F1A2B", "0f8fad5b-d9cb-469f-a165-8c9d2e7f1a2b", true},
		{"uuid refuses a non-uuid", typ(dbspec.TypeUUID), "not-a-uuid", "", false},
		{"bytes are padded base64", typ(dbspec.TypeBytes), "AAEC/w==", "AAEC/w==", true},
		{"bytes pad unpadded base64", typ(dbspec.TypeBytes), "AAEC/w", "AAEC/w==", true},
		{"bytes accept the empty text", typ(dbspec.TypeBytes), "", "", true},
		{"bytes refuse what is not base64", typ(dbspec.TypeBytes), "!!", "", false},
		{"text is written as it is", typ(dbspec.TypeText), "  any <thing> ", "  any <thing> ", true},
		{"varchar counts runes", vc3, "한글!", "한글!", true},
		{"varchar refuses more runes than its length", vc3, "한글!!", "", false},
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
	}
	for _, c := range cases {
		if got := singleLineFault(c.in) != ""; got != c.want {
			t.Fatalf("singleLineFault(%q) = %q; breaks = %v, want %v", c.in, singleLineFault(c.in), got, c.want)
		}
	}
}

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
		{"a--b", false},
		{"a-", false},
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

func TestTitleEscape(t *testing.T) {
	cases := []struct {
		title, text string
		ok          bool // the title writes and reads back unchanged
	}{
		{"Plain", "Plain", true},
		{"a#", "a#", true},
		{"Count the #", `Count the \#`, true},
		{"x ##", `x #\#`, true},
		{"x \\#", `x \#`, false},
		{"x\\#", `x\#`, true},
		{"#", "#", true},
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
