package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

const fixtureRoot = "testdata/files"

// stubRow is a row of the stub corpus: the file that holds it and its detail
// anchor, empty when the row has no detail section.
type stubRow struct{ file, anchor string }

// stubLinker is a deterministic Linker over a fixed set of rows keyed by
// "<table>/<key>".
type stubLinker map[string]stubRow

func (s stubLinker) Target(from, table, key string) (string, error) {
	r, ok := s[table+"/"+key]
	if !ok {
		return "", fmt.Errorf("no row %s %s", table, key)
	}
	return RelLink(from, r.file, r.anchor)
}

// fixtureLinker holds the rows of the golden corpus.
func fixtureLinker() stubLinker {
	return stubLinker{
		"tasks/T1": {file: "tasks/T1.flowmark.md"},
		"tasks/T2": {file: "tasks/T2.flowmark.md"},
		"tasks/T3": {file: "tasks/T3.flowmark.md"},
		"memos/M1": {file: "memos/M1.flowmark.md"},
		"memos/M2": {file: "memos/M2.flowmark.md"},
	}
}

func loadFixture(t testing.TB) *catalog.Catalog {
	t.Helper()
	cat, diags := catalog.Load(fixtureRoot)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("fixture catalog: %+v", diags)
	}
	return cat
}

func readGolden(t testing.TB, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureRoot, rel))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sp(v string) *string { return &v }

// doc joins lines as a file: each line is terminated by a line feed. A file
// never ends in a blank line, so one trailing empty line is dropped; raw keeps it.
func doc(lines ...string) string {
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return raw(lines...)
}

// raw joins lines as a file, keeping every line, the trailing empty one too.
func raw(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

// withBase is the canonical tasks row with its two non-null fields, followed by
// lines after one blank line when there are any.
func withBase(lines ...string) string {
	head := []string{"# Title", "- position: 1", "- done: true"}
	if len(lines) == 0 {
		return raw(head...)
	}
	return raw(append(append(head, ""), lines...)...)
}

// parseCase is one input of the file grammar. rule is the rule that every
// diagnostic carries, empty when the input is accepted; line is the line of the
// first diagnostic, 0 when it is not checked; kept says whether a row is
// returned along with the diagnostics.
type parseCase struct {
	name string
	rel  string
	text string
	rule string
	line int
	kept bool
}

func accept(name, text string) parseCase { return parseCase{name: name, text: text, kept: true} }

func reject(name, rule string, line int, text string) parseCase {
	return parseCase{name: name, rule: rule, line: line, text: text}
}

func noncanonical(name string, line int, text string) parseCase {
	return parseCase{name: name, rule: RuleNoncanonical, line: line, text: text, kept: true}
}

func parseCases() []parseCase {
	return []parseCase{
		// The file: encoding, line endings and the final line feed.
		accept("canonical base", withBase()),
		reject("invalid UTF-8", RuleEncoding, 2, "# Title\n\xff\n"),
		{name: "byte order mark", text: "\xef\xbb\xbf" + withBase(), rule: RuleNoncanonical, line: 1, kept: true},
		noncanonical("CRLF line endings", 1, strings.ReplaceAll(withBase(), "\n", "\r\n")),
		noncanonical("no final line feed", 3, "# Title\n- position: 1\n- done: true"),
		reject("empty file", RuleBlock, 1, ""),

		// The file name: the key is the file name without the suffix.
		{name: "key with underscore and dash", rel: "tasks/a_b-c1.flowmark.md", text: withBase(), kept: true},
		{name: "key that is COM10", rel: "tasks/COM10.flowmark.md", text: withBase(), kept: true},
		{name: "key that begins with CONX", rel: "tasks/CONX.flowmark.md", text: withBase(), kept: true},
		{name: "key that begins with an underscore", rel: "tasks/_a.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "key that begins with a dash", rel: "tasks/-a.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "key with a double dash", rel: "tasks/a--b.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "key that ends with a dash", rel: "tasks/a-.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "key with a dot", rel: "tasks/a.b.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "key with a space", rel: "tasks/a b.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "key with a non-ASCII letter", rel: "tasks/가.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "reserved CON", rel: "tasks/CON.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "reserved lpt1 in lower case", rel: "tasks/lpt1.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "key longer than the varchar", rel: "tasks/this-key-is-longer-than-thirty-two-bytes.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "not a flowmark document", rel: "tasks/T1.md", text: withBase(), rule: RuleWalk, line: 1},
		{name: "another table's folder", rel: "memos/T1.flowmark.md", text: withBase(), rule: RuleWalk, line: 1},
		{name: "nested in a folder", rel: "tasks/sub/T1.flowmark.md", text: withBase(), rule: RuleWalk, line: 1},
		{name: "suffix in another case", rel: "tasks/T1.FLOWMARK.md", text: withBase(), rule: RuleWalk, line: 1},

		// The H1 title.
		reject("title line without a hash", RuleBlock, 1, doc("Title", "- position: 1", "- done: true", "")),
		reject("H2 instead of H1", RuleBlock, 1, doc("## Title", "- position: 1", "- done: true", "")),
		reject("no space after the hash", RuleBlock, 1, doc("#Title", "- position: 1", "- done: true", "")),
		reject("empty title", RuleBlock, 1, doc("# ", "- position: 1", "- done: true", "")),
		reject("title with a leading space", RuleBlock, 1, doc("#  Title", "- position: 1", "- done: true", "")),
		reject("title with a trailing space", RuleBlock, 1, doc("# Title ", "- position: 1", "- done: true", "")),
		reject("title with a comment opener", RuleBlock, 1, doc("# a <!-- b", "- position: 1", "- done: true", "")),
		reject("title with a closing tag", RuleBlock, 1, doc("# a </b", "- position: 1", "- done: true", "")),
		accept("title with a bare angle bracket", doc("# a <3", "- position: 1", "- done: true", "")),
		accept("title that ends in an escaped hash", doc(`# Count the \#`, "- position: 1", "- done: true", "")),
		accept("title whose escaped text is a raw backslash hash", doc(`# a \#`, "- position: 1", "- done: true", "")),
		accept("title that ends in a hash run", doc(`# x #\#`, "- position: 1", "- done: true", "")),
		noncanonical("title with an unescaped closing hash", 1, doc("# a #", "- position: 1", "- done: true", "")),

		// The field list: the stored scalar columns in column order.
		reject("field of an unknown column", RuleBlock, 4, withBaseFields("- nope: 1", "")),
		reject("field out of column order", RuleBlock, 3, doc("# Title", "- done: true", "- position: 1", "")),
		reject("non-null field missing", RuleBlock, 3, doc("# Title", "- position: 1", "")),
		reject("field without a colon", RuleBlock, 2, doc("# Title", "- position 1", "- done: true", "")),
		reject("field without a space after the colon", RuleBlock, 2, doc("# Title", "- position:1", "- done: true", "")),
		noncanonical("integer with leading zero", 2, doc("# Title", "- position: 01", "- done: true", "")),
		reject("integer with a decimal point", RuleBlock, 2, doc("# Title", "- position: 1.0", "- done: true", "")),
		reject("integer past i32", RuleBlock, 2, doc("# Title", "- position: 2147483648", "- done: true", "")),
		reject("integer with a plus sign", RuleBlock, 2, doc("# Title", "- position: +1", "- done: true", "")),
		noncanonical("decimal without its scale", 3, doc("# Title", "- position: 1", "- estimate: 12.5", "- done: true", "")),
		reject("decimal past its scale", RuleBlock, 3, doc("# Title", "- position: 1", "- estimate: 12.555", "- done: true", "")),
		accept("decimal at its scale", doc("# Title", "- position: 1", "- estimate: 12.50", "- done: true", "")),
		reject("bool with a capital", RuleBlock, 3, doc("# Title", "- position: 1", "- done: True", "")),
		reject("bool written as a digit", RuleBlock, 3, doc("# Title", "- position: 1", "- done: 1", "")),
		noncanonical("f64 with trailing zeros", 4, doc("# Title", "- position: 1", "- done: true", "- score: 0.50", "")),
		accept("f64 in its shortest form", doc("# Title", "- position: 1", "- done: true", "- score: 0.5", "")),
		reject("date that is not a day", RuleBlock, 4, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-02-30", "")),
		reject("date without padding", RuleBlock, 4, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-2-1", "")),
		reject("datetime with T", RuleBlock, 5, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11T09:30:00", "")),
		reject("datetime with fraction at precision 0", RuleBlock, 5, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00.5", "")),
		reject("datetime now", RuleBlock, 5, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: now", "")),
		noncanonical("time without its fraction digits", 6, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00", "- clock: 09:30:00", "")),
		reject("time at hour 24", RuleBlock, 6, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00", "- clock: 24:00:00", "")),
		noncanonical("uuid in upper case", 7, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00", "- clock: 09:30:00.250", "- ref: 0F8FAD5B-D9CB-469F-A165-8C9D2E7F1A2B", "")),
		reject("uuid that is not one", RuleBlock, 7, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00", "- clock: 09:30:00.250", "- ref: not-a-uuid", "")),
		noncanonical("bytes without padding", 4, doc("# Title", "- position: 1", "- done: true", "- blob: AAEC/w", "")),
		reject("bytes that are not base64", RuleBlock, 4, doc("# Title", "- position: 1", "- done: true", "- blob: !!", "")),
		accept("empty bytes", doc("# Title", "- position: 1", "- done: true", "- blob:", "")),
		reject("i16 past its range", RuleBlock, 4, doc("# Title", "- position: 1", "- done: true", "- weight: 32768", "")),
		reject("i64 past its range", RuleBlock, 4, doc("# Title", "- position: 1", "- done: true", "- big: 9223372036854775808", "")),
		reject("text with raw HTML", RuleBlock, 4, doc("# Title", "- position: 1", "- done: true", "- note: <b>x", "")),
		reject("text with a trailing space", RuleBlock, 4, doc("# Title", "- position: 1", "- done: true", "- note: x ", "")),
		accept("text with a bare angle bracket", doc("# Title", "- position: 1", "- done: true", "- note: <3", "")),
		accept("empty text of a nullable column", doc("# Title", "- position: 1", "- done: true", "- note:", "")),
		reject("varchar past its length", RuleBlock, 3, doc("# Title", "- position: 1", "- owner: 0123456789012345678901234567890123", "- done: true", "")),
		reject("field with no value", RuleBlock, 3, doc("# Title", "- position: 1", "- done:", "")),
		reject("field that writes now", RuleBlock, 5, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: now", "")),
		noncanonical("field list without the blank line", 4, "# Title\n- position: 1\n- done: true\nBody.\n"),
		accept("body that begins with a blank line", doc("# Title", "- position: 1", "- done: true", "", "", "Body.")),

		// The parent link: a foreign key is a link with its derived target.
		accept("parent link to a row", doc("# Title", "- parent: [T2](T2.flowmark.md)", "- position: 1", "- done: true", "")),
		reject("parent link to no row", RuleBlock, 2, doc("# Title", "- parent: [T9](T9.flowmark.md)", "- position: 1", "- done: true", "")),
		noncanonical("parent link with another target", 2, doc("# Title", "- parent: [T2](../tasks/T2.flowmark.md)", "- position: 1", "- done: true", "")),
		noncanonical("parent link whose text is another row", 2, doc("# Title", "- parent: [T2](T1.flowmark.md)", "- position: 1", "- done: true", "")),
		reject("parent without a link", RuleBlock, 2, doc("# Title", "- parent: T2", "- position: 1", "- done: true", "")),
		reject("parent empty", RuleBlock, 2, doc("# Title", "- parent:", "- position: 1", "- done: true", "")),
		reject("parent link with no brackets", RuleBlock, 2, doc("# Title", "- parent: [T2]T2.flowmark.md", "- position: 1", "- done: true", "")),

		// The body: payload, never a list item first and never a heading of level 2 or less.
		accept("body with a deeper heading", withBase("Body text.", "", "### Deeper", "", "Last.")),
		accept("body with a heading inside a fence", withBase("```", "## x", "```")),
		accept("body with an open fence that hides a section heading", withBase("```", "## evidence", "x")),
		accept("body that is one blank line", raw("# Title", "- position: 1", "- done: true", "", "")),
		noncanonical("trailing blank line after the fields", 4, raw("# Title", "- position: 1", "- done: true", "")),
		accept("fields with no body and no section", doc("# Title", "- position: 1", "- done: true")),
		reject("body that begins with a list item", RuleBlock, 5, withBase("- item")),
		reject("body with an H1 line", RuleSection, 5, withBase("# Other")),

		// The sections: the nullable markdown columns, in column order.
		accept("evidence section", withBase("## evidence", "", "Obs.")),
		accept("present empty section", withBase("## cause")),
		accept("section after the body", withBase("Body.", "", "## evidence", "", "Obs.")),
		accept("section with a fenced heading", withBase("## evidence", "", "```", "## x", "```")),
		reject("section named like the body", RuleSection, 5, withBase("## body", "", "x")),
		reject("section of a non-markdown column", RuleSection, 5, withBase("## seen_on", "", "x")),
		reject("undeclared section", RuleSection, 5, withBase("## nope", "", "x")),
		reject("duplicate section", RuleSection, 9, withBase("## evidence", "", "a", "", "## evidence", "", "b")),
		noncanonical("present empty section with a trailing blank", 6, withBase("## cause", "")),
		noncanonical("sections out of column order", 5, withBase("## cause", "", "c", "", "## evidence", "", "e")),
		noncanonical("section heading with a trailing space", 5, withBase("## evidence ", "", "x")),
		noncanonical("missing blank line before a section", 4, doc("# Title", "- position: 1", "- done: true", "## evidence", "", "e")),
	}
}

// withBaseFields returns the canonical base with extra field lines after the
// H1 and before the blank line.
func withBaseFields(extra ...string) string {
	return doc(append([]string{"# Title", "- position: 1", "- done: true"}, extra...)...)
}

func TestParseFileRules(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	for _, c := range parseCases() {
		t.Run(c.name, func(t *testing.T) {
			rel := c.rel
			if rel == "" {
				rel = "tasks/T1.flowmark.md"
			}
			row, diags := ParseFile(cat, "tasks", rel, []byte(c.text), lk)
			if c.rule == "" {
				if len(diags) > 0 {
					t.Fatalf("accepted input has diagnostics: %+v", diags)
				}
				if row == nil {
					t.Fatal("accepted input returned no row")
				}
				out := checkP2(t, cat, row, lk)
				if !bytes.Equal(out, []byte(c.text)) {
					t.Fatalf("P1: emit(parse(s)) != s\n got: %q\nwant: %q", out, c.text)
				}
				return
			}
			if len(diags) == 0 {
				t.Fatalf("no diagnostic; want %s", c.rule)
			}
			for _, d := range diags {
				if d.Rule != c.rule {
					t.Fatalf("diagnostic %+v; want rule %s", d, c.rule)
				}
				if d.File != rel || d.Line < 1 || d.Column < 1 {
					t.Fatalf("diagnostic not located in %s: %+v", rel, d)
				}
			}
			if c.line != 0 && diags[0].Line != c.line {
				t.Fatalf("first diagnostic at line %d; want %d: %+v", diags[0].Line, c.line, diags[0])
			}
			if (row != nil) != c.kept {
				t.Fatalf("row returned = %v; want %v", row != nil, c.kept)
			}
			if row != nil {
				out := checkP2(t, cat, row, lk)
				if bytes.Equal(out, []byte(c.text)) {
					t.Fatalf("noncanonical input emits itself: %q", out)
				}
			}
		})
	}
}

// checkP2 requires that the row emits, that the emitted bytes parse back
// without a diagnostic, and that the row emits the same bytes again.
func checkP2(t *testing.T, cat *catalog.Catalog, row *Row, lk Linker) []byte {
	t.Helper()
	out, diags := RenderFile(cat, row.Table, row, lk)
	if len(diags) > 0 {
		t.Fatalf("the row is refused on write: %+v", diags)
	}
	rel := row.Table + "/" + row.Key + ".flowmark.md"
	back, diags := ParseFile(cat, row.Table, rel, out, lk)
	if len(diags) > 0 || back == nil {
		t.Fatalf("emitted file does not parse cleanly: %+v\n%s", diags, out)
	}
	again, diags := RenderFile(cat, row.Table, back, lk)
	if len(diags) > 0 || !bytes.Equal(again, out) {
		t.Fatalf("P2: emit(parse(emit(d))) != emit(d)\n got: %q\nwant: %q", again, out)
	}
	return out
}

func TestGoldenP1(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	for _, rel := range []string{"tasks/T1.flowmark.md", "tasks/T2.flowmark.md", "tasks/T3.flowmark.md", "memos/M1.flowmark.md", "memos/M2.flowmark.md"} {
		t.Run(rel, func(t *testing.T) {
			data := readGolden(t, rel)
			table := strings.Split(rel, "/")[0]
			row, diags := ParseFile(cat, table, rel, data, lk)
			if len(diags) > 0 || row == nil {
				t.Fatalf("golden file has diagnostics: %+v", diags)
			}
			out, diags := RenderFile(cat, table, row, lk)
			if len(diags) > 0 || !bytes.Equal(out, data) {
				t.Fatalf("P1 failed: %+v\n got: %q\nwant: %q", diags, out, data)
			}
		})
	}
}

func TestGoldenRows(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	read := func(rel string) *Row {
		t.Helper()
		row, diags := ParseFile(cat, strings.Split(rel, "/")[0], rel, readGolden(t, rel), lk)
		if len(diags) > 0 || row == nil {
			t.Fatalf("%s: %+v", rel, diags)
		}
		return row
	}
	scalar := func(r *Row, col string) string {
		t.Helper()
		v := r.Scalars[col]
		if v == nil {
			t.Fatalf("%s: %s is NULL", r.Key, col)
		}
		return *v
	}
	t1 := read("tasks/T1.flowmark.md")
	if t1.Key != "T1" || t1.Title != "Parse and render the files storage" {
		t.Fatalf("T1 key/title = %q/%q", t1.Key, t1.Title)
	}
	if _, ok := t1.Scalars["parent"]; ok && t1.Scalars["parent"] != nil {
		t.Fatalf("T1 has a parent: %q", *t1.Scalars["parent"])
	}
	want := map[string]string{
		"position": "1", "owner": "max", "estimate": "12.50", "done": "false", "score": "0.5",
		"seen_on": "2026-10-11", "seen_at": "2026-10-11 09:30:00", "clock": "09:30:00.250",
		"ref": "0f8fad5b-d9cb-469f-a165-8c9d2e7f1a2b", "blob": "AAEC/w==", "weight": "-3",
		"big": "9223372036854775807", "note": `it's: "quoted" ünï`,
	}
	for col, v := range want {
		if got := scalar(t1, col); got != v {
			t.Fatalf("T1 %s = %q, want %q", col, got, v)
		}
	}
	if !strings.HasPrefix(t1.Body, "Intro line.\n") || !strings.HasSuffix(t1.Body, "Last paragraph.\n") {
		t.Fatalf("T1 body = %q", t1.Body)
	}
	if got := *t1.Sections["evidence"]; got != "Observed: the file round-trips.\n" {
		t.Fatalf("T1 evidence = %q", got)
	}
	if got, ok := t1.Sections["cause"]; !ok || got == nil || *got != "" {
		t.Fatalf("T1 cause should be a present empty section, got %v", got)
	}
	t2 := read("tasks/T2.flowmark.md")
	if *t2.Scalars["parent"] != "T1" || t2.Scalars["owner"] != nil || *t2.Scalars["note"] != "" || t2.Body != "" {
		t.Fatalf("T2 = %+v", t2)
	}
	t3 := read("tasks/T3.flowmark.md")
	if t3.Title != "Count the #" || *t3.Scalars["owner"] != "" || *t3.Scalars["parent"] != "T2" || t3.Sections["evidence"] != nil {
		t.Fatalf("T3 = %+v", t3)
	}
	m1 := read("memos/M1.flowmark.md")
	if m1.Body != "Body.\n" || *m1.Sections["summary"] != "Summary text.\n" {
		t.Fatalf("M1 = %+v", m1)
	}
	m2 := read("memos/M2.flowmark.md")
	if m2.Body != "" || m2.Sections["summary"] != nil || len(m2.Scalars) != 0 {
		t.Fatalf("M2 = %+v", m2)
	}
}

// TestNoncanonicalEmitsGolden requires that the noncanonical spellings of a
// golden file parse with their noncanonical diagnostics, and that the emitted
// bytes are the canonical golden file.
func TestNoncanonicalEmitsGolden(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	t2 := readGolden(t, "tasks/T2.flowmark.md")
	m1 := readGolden(t, "memos/M1.flowmark.md")
	m2 := readGolden(t, "memos/M2.flowmark.md")
	cases := []struct {
		name, rel string
		in        []byte
		golden    []byte
	}{
		{"CRLF", "tasks/T2.flowmark.md", []byte(strings.ReplaceAll(string(t2), "\n", "\r\n")), t2},
		{"byte order mark", "memos/M1.flowmark.md", append([]byte("\xef\xbb\xbf"), m1...), m1},
		{"no final line feed", "memos/M2.flowmark.md", bytes.TrimSuffix(m2, []byte("\n")), m2},
		{"trailing blank line after the fields", "tasks/T2.flowmark.md", append(append([]byte(nil), t2...), '\n'), t2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			table := strings.Split(c.rel, "/")[0]
			row, diags := ParseFile(cat, table, c.rel, c.in, lk)
			if row == nil {
				t.Fatalf("no row: %+v", diags)
			}
			if len(diags) == 0 {
				t.Fatal("noncanonical input has no diagnostic")
			}
			for _, d := range diags {
				if d.Rule != RuleNoncanonical && d.Rule != RuleEncoding {
					t.Fatalf("unexpected diagnostic %+v", d)
				}
			}
			out, diags := RenderFile(cat, table, row, lk)
			if len(diags) > 0 || !bytes.Equal(out, c.golden) {
				t.Fatalf("emit = %q, %+v; want the golden %q", out, diags, c.golden)
			}
		})
	}
}

// validRow is the valid base row of the tasks table.
func validRow() *Row {
	return &Row{
		Table:    "tasks",
		Key:      "T1",
		Title:    "Title",
		Scalars:  map[string]*string{"position": sp("1"), "done": sp("true")},
		Sections: map[string]*string{},
	}
}

func TestRenderRules(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	type rc struct {
		name string
		mut  func(r *Row)
		rule string // empty accepts
		line int
		out  string // the bytes of an accepted row
	}
	baseOut := withBase()
	cases := []rc{
		{name: "base row", mut: func(r *Row) {}, out: baseOut},
		{name: "title that ends in a hash", mut: func(r *Row) { r.Title = "Count the #" }, out: doc(`# Count the \#`, "- position: 1", "- done: true", "")},
		{name: "title that reads back as another", mut: func(r *Row) { r.Title = `a \#` }, rule: RuleConstraint, line: 1},
		{name: "empty title", mut: func(r *Row) { r.Title = "" }, rule: RuleConstraint, line: 1},
		{name: "title with a line feed", mut: func(r *Row) { r.Title = "a\nb" }, rule: RuleConstraint, line: 1},
		{name: "title with a leading space", mut: func(r *Row) { r.Title = " a" }, rule: RuleConstraint, line: 1},
		{name: "title with raw HTML", mut: func(r *Row) { r.Title = "a <!b" }, rule: RuleConstraint, line: 1},
		{name: "title that is not UTF-8", mut: func(r *Row) { r.Title = "a\xff" }, rule: RuleConstraint, line: 1},
		{name: "key CON", mut: func(r *Row) { r.Key = "CON" }, rule: RuleConstraint, line: 1},
		{name: "key with a double dash", mut: func(r *Row) { r.Key = "a--b" }, rule: RuleConstraint, line: 1},
		{name: "key with a space", mut: func(r *Row) { r.Key = "a b" }, rule: RuleConstraint, line: 1},
		{name: "key longer than the varchar", mut: func(r *Row) { r.Key = strings.Repeat("a", 33) }, rule: RuleConstraint, line: 1},
		{name: "row of another table", mut: func(r *Row) { r.Table = "memos" }, rule: RuleConstraint, line: 1},
		{name: "non-null column without a value", mut: func(r *Row) { delete(r.Scalars, "position") }, rule: RuleConstraint, line: 2},
		{name: "integer with leading zero", mut: func(r *Row) { r.Scalars["position"] = sp("01") }, rule: RuleConstraint, line: 2},
		{name: "integer that is not one", mut: func(r *Row) { r.Scalars["position"] = sp("x") }, rule: RuleConstraint, line: 2},
		{name: "now is refused", mut: func(r *Row) {
			r.Scalars["seen_on"] = sp("2026-10-11")
			r.Scalars["seen_at"] = sp("now")
		}, rule: RuleConstraint, line: 5},
		{name: "text with a line feed", mut: func(r *Row) { r.Scalars["owner"] = sp("a\nb") }, rule: RuleConstraint, line: 3},
		{name: "text with a leading space", mut: func(r *Row) { r.Scalars["owner"] = sp(" a") }, rule: RuleConstraint, line: 3},
		{name: "text past the varchar", mut: func(r *Row) { r.Scalars["owner"] = sp(strings.Repeat("x", 33)) }, rule: RuleConstraint, line: 3},
		{name: "empty text of a nullable column", mut: func(r *Row) { r.Scalars["note"] = sp("") }, out: doc("# Title", "- position: 1", "- done: true", "- note:", "")},
		{name: "absent nullable column", mut: func(r *Row) { r.Scalars["owner"] = nil }, out: baseOut},
		{name: "column of another table", mut: func(r *Row) { r.Scalars["nope"] = sp("x") }, rule: RuleConstraint, line: 1},
		{name: "the key column as a scalar", mut: func(r *Row) { r.Scalars["id"] = sp("T1") }, rule: RuleConstraint, line: 1},
		{name: "a foreign key to a row that is not there", mut: func(r *Row) { r.Scalars["parent"] = sp("T9") }, rule: RuleConstraint, line: 2},
		{name: "a foreign key to a row", mut: func(r *Row) { r.Scalars["parent"] = sp("T2") }, out: doc("# Title", "- parent: [T2](T2.flowmark.md)", "- position: 1", "- done: true", "")},
		{name: "body without a line feed", mut: func(r *Row) { r.Body = "x" }, rule: RuleConstraint, line: 5},
		{name: "body that begins with a list item", mut: func(r *Row) { r.Body = "- x\n" }, rule: RuleConstraint, line: 5},
		{name: "body with a level-2 heading", mut: func(r *Row) { r.Body = "x\n## y\n" }, rule: RuleConstraint, line: 6},
		{name: "body with a level-1 heading", mut: func(r *Row) { r.Body = "# y\n" }, rule: RuleConstraint, line: 5},
		{name: "body with a deeper heading", mut: func(r *Row) { r.Body = "### y\n" }, out: withBase("### y")},
		{name: "body with a heading inside a fence", mut: func(r *Row) { r.Body = "```\n## y\n```\n" }, out: withBase("```", "## y", "```")},
		{name: "body with an open fence before a section", mut: func(r *Row) {
			r.Body = "```\n"
			r.Sections["evidence"] = sp("x\n")
		}, rule: RuleConstraint, line: 5},
		{name: "body with CRLF", mut: func(r *Row) { r.Body = "a\r\nb\n" }, rule: RuleConstraint, line: 5},
		{name: "body that is not UTF-8", mut: func(r *Row) { r.Body = "\xff\n" }, rule: RuleConstraint, line: 5},
		{name: "section without a line feed", mut: func(r *Row) { r.Sections["evidence"] = sp("x") }, rule: RuleConstraint, line: 7},
		{name: "section with a level-2 heading", mut: func(r *Row) { r.Sections["evidence"] = sp("## x\n") }, rule: RuleConstraint, line: 7},
		{name: "section that is not declared", mut: func(r *Row) { r.Sections["nope"] = sp("x\n") }, rule: RuleConstraint, line: 1},
		{name: "body given as a section", mut: func(r *Row) { r.Sections["body"] = sp("x\n") }, rule: RuleConstraint, line: 1},
		{name: "present empty section", mut: func(r *Row) { r.Sections["cause"] = sp("") }, out: withBase("## cause")},
		{name: "section with its value", mut: func(r *Row) { r.Sections["evidence"] = sp("Obs.\n") }, out: withBase("## evidence", "", "Obs.")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := validRow()
			c.mut(row)
			out, diags := RenderFile(cat, row.Table, row, lk)
			if c.rule == "" {
				if len(diags) > 0 {
					t.Fatalf("refused: %+v", diags)
				}
				if string(out) != c.out {
					t.Fatalf("emit = %q; want %q", out, c.out)
				}
				checkP2(t, cat, row, lk)
				return
			}
			if out != nil || len(diags) == 0 {
				t.Fatalf("accepted; want refusal %s, output %q", c.rule, out)
			}
			for _, d := range diags {
				if d.Rule != c.rule {
					t.Fatalf("diagnostic %+v; want rule %s", d, c.rule)
				}
			}
			if diags[0].Line != c.line {
				t.Fatalf("first refusal at line %d; want %d: %+v", diags[0].Line, c.line, diags[0])
			}
		})
	}
}

// TestMemosHaveNoFieldList covers a files table without a non-null stored
// scalar, whose file has no field list: the body follows the H1 after one blank.
func TestMemosHaveNoFieldList(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	cases := []parseCase{
		accept("body after one blank line", "# Memo\n\nBody.\n"),
		accept("empty body", "# Memo\n"),
		noncanonical("body without the blank line", 2, "# Memo\nBody.\n"),
		noncanonical("blank line with no body", 2, "# Memo\n\n"),
		reject("body that begins with a list item", RuleBlock, 3, "# Memo\n\n- item\n"),
		reject("undeclared level-2 heading after the body", RuleSection, 5, "# Memo\n\nBody.\n\n## nope\n"),
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row, diags := ParseFile(cat, "memos", "memos/M9.flowmark.md", []byte(c.text), lk)
			if c.rule == "" {
				if len(diags) > 0 || row == nil {
					t.Fatalf("refused: %+v", diags)
				}
				if out := checkP2(t, cat, row, lk); string(out) != c.text {
					t.Fatalf("P1: %q != %q", out, c.text)
				}
				return
			}
			if len(diags) == 0 || diags[0].Rule != c.rule || (c.line != 0 && diags[0].Line != c.line) {
				t.Fatalf("diagnostics %+v; want %s at line %d", diags, c.rule, c.line)
			}
			if (row != nil) != c.kept {
				t.Fatalf("row = %v; want kept %v", row != nil, c.kept)
			}
			if row != nil {
				checkP2(t, cat, row, lk)
			}
		})
	}
}
