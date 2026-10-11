package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// TestReadRefusalPosition requires a refusal that the writer makes of a row read
// from a file to be located where the parser read the element it names: a title
// fault at the title of the H1 line, a field fault at the field's item, and a
// whole-row fault at the row's first line, column 1. The refusals name the
// elements as the writer would, since the reader accepts the file.
func TestReadRefusalPosition(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	const rel = "tasks/T1.flowmark.md"
	row, diags := ParseFile(cat, "tasks", rel, []byte("# Title\n- position: 1\n- done: true\n"), lk)
	if len(diags) > 0 || row == nil {
		t.Fatalf("the fixture file is refused on read: %+v", diags)
	}
	rf := []Refusal{
		{Table: "tasks", Key: "T1", Index: -1, Column: "title", Rule: RuleConstraint, Message: "title fault"},
		{Table: "tasks", Key: "T1", Index: -1, Column: "done", Rule: RuleConstraint, Message: "field fault"},
		{Table: "tasks", Key: "T1", Index: -1, Column: "", Rule: RuleConstraint, Message: "row fault"},
	}
	got := readDiagnostics(rel, rf, []*Row{row})
	want := []struct{ line, col int }{{1, 3}, {3, 3}, {1, 1}}
	if len(got) != len(want) {
		t.Fatalf("%d diagnostics; want %d: %+v", len(got), len(want), got)
	}
	for k, w := range want {
		if got[k].Line != w.line || got[k].Column != w.col {
			t.Fatalf("diagnostic %d at %d:%d; want %d:%d: %+v", k, got[k].Line, got[k].Column, w.line, w.col, got[k])
		}
	}
}

// TestReadRefusalPositionDocument requires the same of a list row of a document file:
// a title fault at the title, a field fault at the field's token, and a whole-row
// fault at the row's line, column 1.
func TestReadRefusalPositionDocument(t *testing.T) {
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	const rel = "items.flowmark.md"
	rows, diags, err := ParseDocument(cat, "items", rel, []byte(itemsOf("- [ ] T1 Plan done:false", "- [ ] T2 Other done:true")), lk)
	if err != nil || len(diags) > 0 || len(rows) != 2 {
		t.Fatalf("the fixture file is refused on read: %v %+v", err, diags)
	}
	rf := []Refusal{
		{Table: "items", Key: "T2", Index: -1, Column: "title", Rule: RuleConstraint, Message: "title fault"},
		{Table: "items", Key: "T1", Index: -1, Column: "done", Rule: RuleConstraint, Message: "field fault"},
		{Table: "items", Key: "T1", Index: -1, Column: "", Rule: RuleConstraint, Message: "row fault"},
	}
	got := readDiagnostics(rel, rf, rows)
	// Line 2 is the first row, line 3 the second; "- [ ] T2 Other" puts the title at column 10.
	want := []struct{ line, col int }{{3, 10}, {2, 15}, {2, 1}}
	if len(got) != len(want) {
		t.Fatalf("%d diagnostics; want %d: %+v", len(got), len(want), got)
	}
	for k, w := range want {
		if got[k].Line != w.line || got[k].Column != w.col {
			t.Fatalf("diagnostic %d at %d:%d; want %d:%d: %+v", k, got[k].Line, got[k].Column, w.line, w.col, got[k])
		}
	}
}

// TestDefinitionBeforeSetextSection requires that the link reference definitions
// CommonMark 0.31.2 reads at the start of a setext section heading's paragraph are not
// part of the heading text (§4.7): they are content of the body before it, and the
// heading names its section by the text after them.
func TestDefinitionBeforeSetextSection(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	t.Run("10#sections-and-their-values: a definition before a setext section heading is not its text", func(t *testing.T) {
		row, diags := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", []byte(withBase("[r]: /u", "evidence", "--------", "", "x")), lk)
		if len(diags) != 1 || diags[0].Rule != RuleNoncanonical || diags[0].Line != 6 || row == nil {
			t.Fatalf("diagnostics %+v; want the section evidence read, its setext underline noncanonical at line 6", diags)
		}
		if row.Body != "[r]: /u\n" || row.Sections["evidence"] == nil || *row.Sections["evidence"] != "x\n" {
			t.Fatalf("body %q, sections %v; want the body [r]: /u and evidence x", row.Body, row.Sections)
		}
	})
	t.Run("10#sections-and-their-values: a definition before a setext heading that names no section", func(t *testing.T) {
		_, diags := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", []byte(withBase("[r]: /u", "x", "---")), lk)
		if len(diags) == 0 || diags[0].Rule != RuleSection || diags[0].Line != 6 {
			t.Fatalf("diagnostics %+v; want format.section at line 6", diags)
		}
	})
}

// TestFilesSectionHeading requires that the faults of a files row's section and block
// headings are located at the first character of the heading text, and that a setext
// underline in place of the hashes is a noncanonical spelling of either, reported by
// the heading rule itself.
func TestFilesSectionHeading(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	cases := []struct {
		name, text, rule, message string
		line, col                 int
	}{
		{"10#diagnostics: an undeclared section heading is located at its text", withBase("##  nope", "", "x"), RuleSection, "is not a section", 5, 5},
		{"10#diagnostics: a section heading of another level is located at its text", withBase("# nope", "", "x"), RuleSection, "is not a section", 5, 3},
		{"10#diagnostics: a section written twice is located at its text", withBase("## evidence", "", "a", "", "## evidence", "", "b"), RuleSection, "written twice", 9, 4},
		{"10#diagnostics: a noncanonical section heading is located at its text", withBase("##  evidence", "", "x"), RuleNoncanonical, "section heading", 5, 5},
		{"10#diagnostics: a noncanonical block heading is located at its text", withBase("##  task_steps", "- S1 Draft done:true"), RuleNoncanonical, "block heading", 5, 5},
		{"10#detail-headings-and-anchors: a setext section heading is a noncanonical section heading", withBase("evidence", "--------", "", "x"), RuleNoncanonical, "section heading", 5, 1},
		{"10#detail-headings-and-anchors: a setext block heading is a noncanonical block heading", withBase("task_steps", "----------", "- S1 Draft done:true"), RuleNoncanonical, "block heading", 5, 1},
		{"10#detail-headings-and-anchors: a section heading after a definition is located at its text", withBase("[r]: /u", "evidence", "--------", "", "x"), RuleNoncanonical, "section heading", 6, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", []byte(c.text), lk)
			if !hasDiagAt(diags, c.rule, c.message, c.line, c.col) {
				t.Fatalf("diagnostics %+v; want %s %q at %d:%d", diags, c.rule, c.message, c.line, c.col)
			}
		})
	}
}

// renderFileOf writes row as a file of table, failing the test when the table is not
// a files table of the catalog.
func renderFileOf(t testing.TB, cat *catalog.Catalog, table string, row *Row, lk Linker) ([]byte, []Refusal) {
	t.Helper()
	out, rf, err := RenderFile(cat, table, row, lk)
	if err != nil {
		t.Fatal(err)
	}
	return out, rf
}

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
	raw  string // the scalar column that is kept as its raw text, when accepted
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
		accept("10#encoding-and-line-structure: canonical base", withBase()),
		reject("10#encoding-and-line-structure: invalid UTF-8", RuleEncoding, 2, "# Title\n\xff\n"),
		reject("10#encoding-and-line-structure: U+0000 in a value", RuleEncoding, 5, withBase("a\x00b")),
		{name: "10#encoding-and-line-structure: byte order mark", text: "\xef\xbb\xbf" + withBase(), rule: RuleNoncanonical, line: 1, kept: true},
		noncanonical("10#encoding-and-line-structure: CRLF line endings", 1, strings.ReplaceAll(withBase(), "\n", "\r\n")),
		noncanonical("10#encoding-and-line-structure: bare CR line ending", 2, "# Title\n- position: 1\r- done: true\n"),
		noncanonical("10#encoding-and-line-structure: no final line feed", 3, "# Title\n- position: 1\n- done: true"),
		reject("10#encoding-and-line-structure: empty file", RuleBlock, 1, ""),

		// The file name: the key is the file name without the suffix.
		{name: "20#keys: key with underscore and dash", rel: "tasks/a_b-c1.flowmark.md", text: withBase(), kept: true},
		{name: "20#keys: key that is COM10", rel: "tasks/COM10.flowmark.md", text: withBase(), kept: true},
		{name: "20#keys: key that begins with CONX", rel: "tasks/CONX.flowmark.md", text: withBase(), kept: true},
		{name: "20#keys: key that ends with a dash", rel: "tasks/a-.flowmark.md", text: withBase(), kept: true},
		{name: "20#keys: key with an underscore after a dash", rel: "tasks/a_-b.flowmark.md", text: withBase(), kept: true},
		{name: "20#keys: key that begins with an underscore", rel: "tasks/_a.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: key that begins with a dash", rel: "tasks/-a.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: negative integer key", rel: "tasks/-7.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: key with a double dash", rel: "tasks/a--b.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: key with a dash before an underscore", rel: "tasks/a-_b.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: key with a dot", rel: "tasks/a.b.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: key with a space", rel: "tasks/a b.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: key with a non-ASCII letter", rel: "tasks/가.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: reserved CON", rel: "tasks/CON.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: reserved lpt1 in lower case", rel: "tasks/lpt1.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#keys: key longer than the varchar", rel: "tasks/this-key-is-longer-than-thirty-two-bytes.flowmark.md", text: withBase(), rule: RuleBlock, line: 1},
		{name: "20#corpus-documents-and-the-walk: not a flowmark document", rel: "tasks/T1.md", text: withBase(), rule: RuleWalk, line: 1},
		{name: "20#corpus-documents-and-the-walk: another table's folder", rel: "memos/T1.flowmark.md", text: withBase(), rule: RuleWalk, line: 1},
		{name: "20#corpus-documents-and-the-walk: nested in a folder", rel: "tasks/sub/T1.flowmark.md", text: withBase(), rule: RuleWalk, line: 1},
		{name: "20#corpus-documents-and-the-walk: suffix in another case", rel: "tasks/T1.FLOWMARK.md", text: withBase(), rule: RuleWalk, line: 1},

		// The H1 title.
		reject("10#single-line-places: title line without a hash", RuleBlock, 1, doc("Title", "- position: 1", "- done: true", "")),
		reject("10#single-line-places: H2 instead of H1", RuleBlock, 1, doc("## Title", "- position: 1", "- done: true", "")),
		reject("10#single-line-places: no space after the hash", RuleBlock, 1, doc("#Title", "- position: 1", "- done: true", "")),
		reject("10#single-line-places: empty title", RuleBlock, 1, doc("# ", "- position: 1", "- done: true", "")),
		reject("10#single-line-places: title that is only a closing sequence", RuleBlock, 1, doc("# #", "- position: 1", "- done: true", "")),
		noncanonical("10#single-line-places: title with a leading space", 1, doc("#  Title", "- position: 1", "- done: true", "")),
		noncanonical("10#single-line-places: title with a trailing space", 1, doc("# Title ", "- position: 1", "- done: true", "")),
		reject("10#single-line-places: title with a comment opener", RuleBlock, 1, doc("# a <!-- b", "- position: 1", "- done: true", "")),
		reject("10#single-line-places: title with a closing tag", RuleBlock, 1, doc("# a </b", "- position: 1", "- done: true", "")),
		accept("10#single-line-places: title with a bare angle bracket", doc("# a <3", "- position: 1", "- done: true", "")),
		accept("10#single-line-places: title that ends in an escaped hash", doc(`# Count the \#`, "- position: 1", "- done: true", "")),
		accept("10#single-line-places: title that is the escaped text a backslash hash", doc(`# a \#`, "- position: 1", "- done: true", "")),
		accept("10#single-line-places: title that ends in a hash run", doc(`# x #\#`, "- position: 1", "- done: true", "")),
		accept("10#single-line-places: title that is only a hash", doc(`# \#`, "- position: 1", "- done: true", "")),
		accept("10#single-line-places: title that is only two hashes", doc(`# #\#`, "- position: 1", "- done: true", "")),
		noncanonical("10#single-line-places: title with an unescaped closing hash", 1, doc("# a #", "- position: 1", "- done: true", "")),
		noncanonical("10#single-line-places: title with a tab before the closing hash", 1, doc("# a\t#", "- position: 1", "- done: true", "")),
		noncanonical("10#single-line-places: title with a tab after its hash", 1, doc("#\tTitle", "- position: 1", "- done: true", "")),
		accept("10#single-line-places: a hash run without a space is text", doc("# Title", "- position: 1", "- done: true", "", "##x", "")),
		accept("10#single-line-places: seven hashes are text", doc("# Title", "- position: 1", "- done: true", "", "#######", "")),
		reject("10#sections-and-their-values: hashes alone at the end of a line are an empty section heading", RuleSection, 8, doc("# Title", "- position: 1", "- done: true", "", "## evidence", "", "x", "##", "")),
		noncanonical("10#sections-and-their-values: section heading with a tab after its hashes", 5, doc("# Title", "- position: 1", "- done: true", "", "##\tevidence", "", "x", "")),
		noncanonical("10#single-line-places: escaped hash the write rule would not produce", 1, doc(`# C\#`, "- position: 1", "- done: true", "")),

		// The field list: the stored scalar columns in column order.
		reject("10#the-field-list-of-a-files-row: field of an unknown column", RuleBlock, 4, withBaseFields("- nope: 1", "")),
		reject("10#the-field-list-of-a-files-row: field out of column order", RuleBlock, 3, doc("# Title", "- done: true", "- position: 1", "")),
		reject("10#the-field-list-of-a-files-row: non-null field missing", RuleBlock, 3, doc("# Title", "- position: 1", "")),
		reject("10#the-field-list-of-a-files-row: field without a colon", RuleBlock, 2, doc("# Title", "- position 1", "- done: true", "")),
		reject("10#the-field-list-of-a-files-row: field without a space after the colon", RuleBlock, 2, doc("# Title", "- position:1", "- done: true", "")),
		noncanonical("10#readable-values: integer with leading zero", 2, doc("# Title", "- position: 01", "- done: true", "")),
		acceptRaw("10#readable-values: integer with a decimal point", "position", doc("# Title", "- position: 1.0", "- done: true", "")),
		acceptRaw("10#readable-values: integer past i32", "position", doc("# Title", "- position: 2147483648", "- done: true", "")),
		acceptRaw("10#readable-values: integer with a plus sign", "position", doc("# Title", "- position: +1", "- done: true", "")),
		noncanonical("10#readable-values: decimal without its scale", 3, doc("# Title", "- position: 1", "- estimate: 12.5", "- done: true", "")),
		acceptRaw("10#readable-values: decimal past its scale", "estimate", doc("# Title", "- position: 1", "- estimate: 12.555", "- done: true", "")),
		accept("10#readable-values: decimal at its scale", doc("# Title", "- position: 1", "- estimate: 12.50", "- done: true", "")),
		acceptRaw("10#readable-values: bool with a capital", "done", doc("# Title", "- position: 1", "- done: True", "")),
		acceptRaw("10#readable-values: bool written as a digit", "done", doc("# Title", "- position: 1", "- done: 1", "")),
		noncanonical("10#readable-values: f64 with trailing zeros", 4, doc("# Title", "- position: 1", "- done: true", "- score: 0.50", "")),
		accept("10#readable-values: f64 in its shortest form", doc("# Title", "- position: 1", "- done: true", "- score: 0.5", "")),
		acceptRaw("10#readable-values: date that is not a day", "seen_on", doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-02-30", "")),
		acceptRaw("10#readable-values: date without padding", "seen_on", doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-2-1", "")),
		acceptRaw("10#readable-values: datetime with T", "seen_at", doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11T09:30:00", "")),
		acceptRaw("10#readable-values: datetime with fraction at precision 0", "seen_at", doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00.5", "")),
		acceptRaw("10#readable-values: datetime now", "seen_at", doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: now", "")),
		noncanonical("10#readable-values: time without its fraction digits", 6, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00", "- clock: 09:30:00", "")),
		acceptRaw("10#readable-values: time at hour 24", "clock", doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00", "- clock: 24:00:00", "")),
		noncanonical("10#readable-values: uuid in upper case", 7, doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00", "- clock: 09:30:00.250", "- ref: 0F8FAD5B-D9CB-469F-A165-8C9D2E7F1A2B", "")),
		acceptRaw("10#readable-values: uuid that is not one", "ref", doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: 2026-10-11 09:30:00", "- clock: 09:30:00.250", "- ref: not-a-uuid", "")),
		noncanonical("10#readable-values: bytes without padding", 4, doc("# Title", "- position: 1", "- done: true", "- blob: AAEC/w", "")),
		acceptRaw("10#readable-values: bytes that are not base64", "blob", doc("# Title", "- position: 1", "- done: true", "- blob: !!", "")),
		accept("10#readable-values: empty bytes", doc("# Title", "- position: 1", "- done: true", "- blob:", "")),
		acceptRaw("10#readable-values: i16 past its range", "weight", doc("# Title", "- position: 1", "- done: true", "- weight: 32768", "")),
		acceptRaw("10#readable-values: i64 past its range", "big", doc("# Title", "- position: 1", "- done: true", "- big: 9223372036854775808", "")),
		reject("10#single-line-places: text with raw HTML", RuleBlock, 4, doc("# Title", "- position: 1", "- done: true", "- note: <b>x", "")),
		noncanonical("10#single-line-places: text with a trailing space", 4, doc("# Title", "- position: 1", "- done: true", "- note: x ", "")),
		noncanonical("10#single-line-places: text with two spaces after the colon", 4, doc("# Title", "- position: 1", "- done: true", "- note:  x", "")),
		noncanonical("10#single-line-places: owner with a trailing space", 3, doc("# Title", "- position: 1", "- owner: max ", "- done: true", "")),
		accept("10#readable-values: text with a bare angle bracket", doc("# Title", "- position: 1", "- done: true", "- note: <3", "")),
		accept("10#readable-values: empty text of a nullable column", doc("# Title", "- position: 1", "- done: true", "- note:", "")),
		acceptRaw("10#readable-values: varchar past its length", "owner", doc("# Title", "- position: 1", "- owner: 0123456789012345678901234567890123", "- done: true", "")),
		acceptRaw("10#readable-values: empty text of a non-null column that is not text", "done", doc("# Title", "- position: 1", "- done:", "")),
		acceptRaw("10#readable-values: field that writes now", "seen_at", doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-10-11", "- seen_at: now", "")),
		accept("10#readable-values: text that is the word now", doc("# Title", "- position: 1", "- done: true", "- note: now", "")),
		reject("10#the-field-list-of-a-files-row: body line directly after the field list", RuleBlock, 4, "# Title\n- position: 1\n- done: true\nBody.\n"),
		noncanonical("10#the-field-list-of-a-files-row: missing blank line before a section after the fields", 4, "# Title\n- position: 1\n- done: true\n## evidence\n\nx\n"),

		// The parent link: a foreign key is a link with its derived target.
		accept("10#foreign-keys-as-links: parent link to a row", doc("# Title", "- parent: [T2](T2.flowmark.md)", "- position: 1", "- done: true", "")),
		accept("10#foreign-keys-as-links: dangling parent keeps its written target", doc("# Title", "- parent: [T9](T9.flowmark.md)", "- position: 1", "- done: true", "")),
		accept("10#foreign-keys-as-links: dangling parent keeps another written target", doc("# Title", "- parent: [T9](../x/T9.flowmark.md)", "- position: 1", "- done: true", "")),
		noncanonical("10#foreign-keys-as-links: parent link with another target", 2, doc("# Title", "- parent: [T2](../tasks/T2.flowmark.md)", "- position: 1", "- done: true", "")),
		noncanonical("10#foreign-keys-as-links: parent link whose text is another row", 2, doc("# Title", "- parent: [T2](T1.flowmark.md)", "- position: 1", "- done: true", "")),
		acceptRaw("10#readable-values: a parent link whose key is not of its column type", "parent", doc("# Title", "- parent: [ABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJKLMN](ABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJKLMN.flowmark.md)", "- position: 1", "- done: true", "")),
		reject("10#foreign-keys-as-links: parent without a link", RuleBlock, 2, doc("# Title", "- parent: T2", "- position: 1", "- done: true", "")),
		reject("10#foreign-keys-as-links: parent empty", RuleBlock, 2, doc("# Title", "- parent:", "- position: 1", "- done: true", "")),
		reject("10#foreign-keys-as-links: parent link with no brackets", RuleBlock, 2, doc("# Title", "- parent: [T2]T2.flowmark.md", "- position: 1", "- done: true", "")),

		// The body: payload, never a list item first and never a heading of level 2 or less.
		accept("10#sections-and-their-values: body with a deeper heading", withBase("Body text.", "", "### Deeper", "", "Last.")),
		accept("10#sections-and-their-values: body with a heading inside a fence", withBase("```", "## x", "```")),
		accept("10#sections-and-their-values: body with an open fence that holds a heading of level 2 as body text", withBase("```", "## evidence", "x")),
		accept("10#sections-and-their-values: open fence at the end of a body", withBase("```", "x")),
		noncanonical("10#sections-and-their-values: fence open in a list item before a top-level section", 8, withBase("Body.", "", "- ```", "## evidence", "", "x")),
		noncanonical("10#sections-and-their-values: body that is one blank line", 5, raw("# Title", "- position: 1", "- done: true", "", "")),
		noncanonical("10#sections-and-their-values: trailing blank line after the fields", 4, raw("# Title", "- position: 1", "- done: true", "")),
		accept("10#sections-and-their-values: fields with no body and no section", doc("# Title", "- position: 1", "- done: true")),
		noncanonical("10#sections-and-their-values: body that begins with a blank line", 5, doc("# Title", "- position: 1", "- done: true", "", "", "Body.")),
		accept("10#sections-and-their-values: body with one space of indent after the fields", withBase(" x")),
		reject("10#the-field-list-of-a-files-row: body that begins with a list item", RuleBlock, 5, withBase("- item")),
		reject("10#the-field-list-of-a-files-row: body indented by two spaces after the fields", RuleBlock, 5, withBase("  x")),
		reject("10#the-field-list-of-a-files-row: body indented by a tab after the fields", RuleBlock, 5, withBase("\tx")),
		reject("10#sections-and-their-values: body with an H1 line", RuleSection, 5, withBase("# Other")),
		reject("10#sections-and-their-values: body with a setext H1", RuleSection, 5, withBase("Title", "===")),
		reject("10#sections-and-their-values: body with a setext H2 that names no section", RuleSection, 5, withBase("x", "---")),
		accept("10#sections-and-their-values: body with a thematic break after a blank line", withBase("x", "", "---")),
		noncanonical("10#sections-and-their-values: body with a setext heading that names a section", 5, withBase("evidence", "---", "", "Obs.")),

		// The block structure: a heading inside a container (a list item, a block
		// quote) or inside a fence or an HTML block is content, never a section
		// boundary; a heading at the top level ends the body (CommonMark 0.31.2).
		accept("10#sections-and-their-values: heading inside a list item of the body", withBase("Body.", "", "- a", "  ## x")),
		accept("10#sections-and-their-values: heading inside a list item after a blank line", withBase("Body.", "", "- a", "", "  ## x")),
		accept("10#sections-and-their-values: heading indented past the content of a list item", withBase("Body.", "", "- a", "    ## x")),
		accept("10#sections-and-their-values: heading inside an ordered list item", withBase("Body.", "", "1. a", "   ## x")),
		accept("10#sections-and-their-values: heading inside a nested list item", withBase("Body.", "", "- a", "  - b", "    ## x")),
		accept("10#sections-and-their-values: heading inside a tab-indented list item", withBase("Body.", "", "-\tfoo", "\t## x")),
		accept("10#sections-and-their-values: heading inside a block quote", withBase("Body.", "", "> ## x")),
		accept("10#sections-and-their-values: heading inside a fence in a list item", withBase("Body.", "", "- ```", "  ## x", "  ```")),
		accept("10#sections-and-their-values: heading inside an HTML comment", withBase("<!--", "## evidence", "-->")),
		accept("10#sections-and-their-values: open HTML comment that holds a section heading line as body text", withBase("<!--", "", "## evidence", "", "x")),
		accept("10#sections-and-their-values: HTML block of type 6 that holds a section heading line without a blank line", withBase("<div>", "## evidence", "x")),
		accept("10#sections-and-their-values: HTML block of type 6 followed by a blank line before a section", withBase("<div>", "", "## evidence", "", "x")),
		accept("10#sections-and-their-values: HTML block of type 7 that holds a section heading line as body text", withBase("<span>", "## evidence", "x")),
		noncanonical("10#sections-and-their-values: HTML block of type 7 that is a line of a paragraph", 7, withBase("Body.", "<span>", "## evidence", "", "x")),
		accept("10#sections-and-their-values: indented code is not a heading", withBase("Body.", "", "    ## x")),
		accept("10#sections-and-their-values: indented line that continues a paragraph is not a heading", withBase("Body.", "    ## x")),
		accept("10#sections-and-their-values: setext underline inside a list item is not a section", withBase("Body.", "", "- a", "  ===")),
		accept("10#sections-and-their-values: lazy continuation of a list item that is not a heading", withBase("Body.", "", "- a", "b", "===")),
		accept("10#sections-and-their-values: thematic break ends a lazy list item", withBase("Body.", "", "- a", "b", "---")),
		accept("10#sections-and-their-values: setext underline after a block quote line is lazy text", withBase("Body.", "", "> a", "===")),
		accept("10#sections-and-their-values: thematic break after a block quote line", withBase("Body.", "", "> a", "---")),
		accept("10#sections-and-their-values: list item heading before a top-level section", withBase("Body.", "", "- a", "  ## x", "", "## evidence", "", "Obs.")),
		accept("10#sections-and-their-values: block quote that ends before a top-level section", withBase("Body.", "", "> a", "", "## evidence", "", "Obs.")),
		reject("10#sections-and-their-values: top-level heading after a list item", RuleSection, 9, withBase("Body.", "", "- a", "", "## nope", "", "x")),
		reject("10#sections-and-their-values: top-level heading after a lazy list item", RuleSection, 9, withBase("Body.", "", "- a", "b", "## nope", "", "x")),
		reject("10#sections-and-their-values: top-level heading after a nested list item", RuleSection, 9, withBase("Body.", "", "- a", "  - b", "## nope", "", "x")),
		reject("10#sections-and-their-values: top-level heading after an ordered list item", RuleSection, 8, withBase("Body.", "", "1. a", "  ## nope", "", "x")),
		reject("10#sections-and-their-values: top-level heading after a block quote", RuleSection, 8, withBase("Body.", "", "> a", "## nope", "", "x")),
		reject("10#sections-and-their-values: top-level heading after a heading in a block quote", RuleSection, 8, withBase("Body.", "", "> ## x", "## nope", "", "x")),
		reject("10#sections-and-their-values: top-level heading closes a fence of a list item", RuleSection, 8, withBase("Body.", "", "- ```", "## nope", "", "x")),
		reject("10#sections-and-their-values: setext heading at the top level after a list item", RuleSection, 9, withBase("Body.", "", "- a", "", "b", "===")),

		// The sections: the nullable markdown columns, in column order.
		accept("10#sections-and-their-values: evidence section", withBase("## evidence", "", "Obs.")),
		accept("10#sections-and-their-values: present empty section", withBase("## cause")),
		accept("10#sections-and-their-values: section after the body", withBase("Body.", "", "## evidence", "", "Obs.")),
		accept("10#sections-and-their-values: section with a fenced heading", withBase("## evidence", "", "```", "## x", "```")),
		reject("10#sections-and-their-values: section named like the body", RuleSection, 5, withBase("## body", "", "x")),
		reject("10#sections-and-their-values: section of a non-markdown column", RuleSection, 5, withBase("## seen_on", "", "x")),
		reject("10#sections-and-their-values: undeclared section", RuleSection, 5, withBase("## nope", "", "x")),
		reject("10#sections-and-their-values: duplicate section", RuleSection, 9, withBase("## evidence", "", "a", "", "## evidence", "", "b")),
		reject("10#sections-and-their-values: a section written twice with equal values", RuleSection, 9, withBase("## cause", "", "A.", "", "## cause", "", "A.")),
		reject("10#sections-and-their-values: a section written twice under another spelling of its heading", RuleSection, 9, withBase("## cause", "", "A.", "", "##  cause", "", "B.")),
		noncanonical("10#sections-and-their-values: present empty section with a trailing blank", 6, withBase("## cause", "")),
		noncanonical("10#sections-and-their-values: sections out of column order", 5, withBase("## cause", "", "c", "", "## evidence", "", "e")),
		noncanonical("10#sections-and-their-values: section heading with a trailing space", 5, withBase("## evidence ", "", "x")),
		noncanonical("10#sections-and-their-values: missing blank line before a section", 4, doc("# Title", "- position: 1", "- done: true", "## evidence", "", "e")),
	}
}

// acceptRaw is an accepted input with a scalar that is not of its column type:
// the row is kept with no diagnostic, the scalar is kept as its raw text, and
// the validator reports its type.
func acceptRaw(name, col, text string) parseCase {
	return parseCase{name: name, text: text, kept: true, raw: col}
}

// withBaseFields returns the canonical base with extra field lines after the
// H1 and before the blank line.
func withBaseFields(extra ...string) string {
	return doc(append([]string{"# Title", "- position: 1", "- done: true"}, extra...)...)
}

func TestParseFileRules(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	for _, c := range append(parseCases(), blockParseCases()...) {
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
				var raws []string
				for col := range row.Raw {
					raws = append(raws, col)
				}
				if want := []string{c.raw}; (c.raw == "" && len(raws) > 0) || (c.raw != "" && !reflect.DeepEqual(raws, want)) {
					t.Fatalf("columns kept raw %q; want %q", raws, c.raw)
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
	out, rf := renderFileOf(t, cat, row.Table, row, lk)
	if len(rf) > 0 {
		t.Fatalf("the row is refused on write: %+v", rf)
	}
	rel := row.Table + "/" + row.Key + ".flowmark.md"
	back, diags := ParseFile(cat, row.Table, rel, out, lk)
	if len(diags) > 0 || back == nil {
		t.Fatalf("emitted file does not parse cleanly: %+v\n%s", diags, out)
	}
	again, rf := renderFileOf(t, cat, row.Table, back, lk)
	if len(rf) > 0 || !bytes.Equal(again, out) {
		t.Fatalf("P2: emit(parse(emit(d))) != emit(d)\n got: %q\nwant: %q", again, out)
	}
	return out
}

func TestGoldenP1(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	for _, rel := range []string{"tasks/T1.flowmark.md", "tasks/T2.flowmark.md", "tasks/T3.flowmark.md", "tasks/T4.flowmark.md", "memos/M1.flowmark.md", "memos/M2.flowmark.md"} {
		t.Run(rel, func(t *testing.T) {
			data := readGolden(t, rel)
			table := strings.Split(rel, "/")[0]
			row, diags := ParseFile(cat, table, rel, data, lk)
			if len(diags) > 0 || row == nil {
				t.Fatalf("golden file has diagnostics: %+v", diags)
			}
			out, rf := renderFileOf(t, cat, table, row, lk)
			if len(rf) > 0 || !bytes.Equal(out, data) {
				t.Fatalf("P1 failed: %+v\n got: %q\nwant: %q", rf, out, data)
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
		{"10#encoding-and-line-structure: CRLF", "tasks/T2.flowmark.md", []byte(strings.ReplaceAll(string(t2), "\n", "\r\n")), t2},
		{"10#encoding-and-line-structure: byte order mark", "memos/M1.flowmark.md", append([]byte("\xef\xbb\xbf"), m1...), m1},
		{"10#encoding-and-line-structure: no final line feed", "memos/M2.flowmark.md", bytes.TrimSuffix(m2, []byte("\n")), m2},
		{"10#encoding-and-line-structure: trailing blank line after the fields", "tasks/T2.flowmark.md", append(append([]byte(nil), t2...), '\n'), t2},
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
			out, rf := renderFileOf(t, cat, table, row, lk)
			if len(rf) > 0 || !bytes.Equal(out, c.golden) {
				t.Fatalf("emit = %q, %+v; want the golden %q", out, rf, c.golden)
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
		name   string
		mut    func(r *Row)
		rule   string // empty accepts
		column string // the column the first refusal names; "" for a fault of the whole row
		out    string // the bytes of an accepted row
	}
	baseOut := withBase()
	cases := []rc{
		{name: "10#document-order: base row", mut: func(r *Row) {}, out: baseOut},
		{name: "10#single-line-places: title that ends in a hash", mut: func(r *Row) { r.Title = "Count the #" }, out: doc(`# Count the \#`, "- position: 1", "- done: true", "")},
		{name: "10#single-line-places: title that is only a hash", mut: func(r *Row) { r.Title = "#" }, out: doc(`# \#`, "- position: 1", "- done: true", "")},
		{name: "10#single-line-places: title that is only two hashes", mut: func(r *Row) { r.Title = "##" }, out: doc(`# #\#`, "- position: 1", "- done: true", "")},
		{name: "10#single-line-places: title that ends in a tab and a hash", mut: func(r *Row) { r.Title = "a\t#" }, out: doc("# a\t\\#", "- position: 1", "- done: true", "")},
		{name: "10#single-line-places: title that reads back as another", mut: func(r *Row) { r.Title = `a \#` }, rule: RuleConstraint, column: "title"},
		{name: "10#single-line-places: title that ends in a backslash hash", mut: func(r *Row) { r.Title = `a\#` }, rule: RuleConstraint, column: "title"},
		{name: "10#single-line-places: empty title", mut: func(r *Row) { r.Title = "" }, rule: RuleConstraint, column: "title"},
		{name: "10#single-line-places: title with a line feed", mut: func(r *Row) { r.Title = "a\nb" }, rule: RuleConstraint, column: "title"},
		{name: "10#single-line-places: title with a leading space", mut: func(r *Row) { r.Title = " a" }, rule: RuleConstraint, column: "title"},
		{name: "10#single-line-places: title with raw HTML", mut: func(r *Row) { r.Title = "a <!b" }, rule: RuleConstraint, column: "title"},
		{name: "10#single-line-places: title that is not UTF-8", mut: func(r *Row) { r.Title = "a\xff" }, rule: RuleConstraint, column: "title"},
		{name: "10#encoding-and-line-structure: title with a U+0000", mut: func(r *Row) { r.Title = "a\x00b" }, rule: RuleConstraint, column: "title"},
		{name: "20#keys: key CON", mut: func(r *Row) { r.Key = "CON" }, rule: RuleConstraint, column: "id"},
		{name: "20#keys: key with a double dash", mut: func(r *Row) { r.Key = "a--b" }, rule: RuleConstraint, column: "id"},
		{name: "20#keys: key with a dash before an underscore", mut: func(r *Row) { r.Key = "a-_b" }, rule: RuleConstraint, column: "id"},
		{name: "20#keys: key that ends with a dash", mut: func(r *Row) { r.Key = "a-" }, out: baseOut},
		{name: "20#keys: key with a space", mut: func(r *Row) { r.Key = "a b" }, rule: RuleConstraint, column: "id"},
		{name: "20#keys: key longer than the varchar", mut: func(r *Row) { r.Key = strings.Repeat("a", 33) }, rule: RuleConstraint, column: "id"},
		{name: "10#document-order: row of another table", mut: func(r *Row) { r.Table = "memos" }, rule: RuleConstraint, column: ""},
		{name: "10#readable-values: non-null column without a value", mut: func(r *Row) { delete(r.Scalars, "position") }, rule: RuleConstraint, column: "position"},
		{name: "10#readable-values: integer with leading zero is written canonically", mut: func(r *Row) { r.Scalars["position"] = sp("01") }, out: baseOut},
		{name: "10#readable-values: integer that is not one", mut: func(r *Row) { r.Scalars["position"] = sp("x") }, rule: RuleConstraint, column: "position"},
		{name: "10#readable-values: decimal is padded to its scale", mut: func(r *Row) { r.Scalars["estimate"] = sp("1.5") }, out: doc("# Title", "- position: 1", "- estimate: 1.50", "- done: true", "")},
		{name: "10#readable-values: f64 is written in its shortest form", mut: func(r *Row) { r.Scalars["score"] = sp("0.50") }, out: doc("# Title", "- position: 1", "- done: true", "- score: 0.5", "")},
		{name: "10#readable-values: bytes are written padded", mut: func(r *Row) { r.Scalars["blob"] = sp("AAEC/w") }, out: doc("# Title", "- position: 1", "- done: true", "- blob: AAEC/w==", "")},
		{name: "10#readable-values: text now is text", mut: func(r *Row) { r.Scalars["note"] = sp("now") }, out: doc("# Title", "- position: 1", "- done: true", "- note: now", "")},
		{name: "10#readable-values: decimal that CanonicalLiteral refuses", mut: func(r *Row) { r.Scalars["estimate"] = sp("12.555") }, rule: RuleConstraint, column: "estimate"},
		{name: "10#readable-values: bool with a capital", mut: func(r *Row) { r.Scalars["done"] = sp("True") }, rule: RuleConstraint, column: "done"},
		{name: "10#readable-values: now is refused", mut: func(r *Row) {
			r.Scalars["seen_on"] = sp("2026-10-11")
			r.Scalars["seen_at"] = sp("now")
		}, rule: RuleConstraint, column: "seen_at"},
		{name: "10#single-line-places: text with a line feed", mut: func(r *Row) { r.Scalars["owner"] = sp("a\nb") }, rule: RuleConstraint, column: "owner"},
		{name: "10#single-line-places: text with a leading space", mut: func(r *Row) { r.Scalars["owner"] = sp(" a") }, rule: RuleConstraint, column: "owner"},
		{name: "10#readable-values: text past the varchar", mut: func(r *Row) { r.Scalars["owner"] = sp(strings.Repeat("x", 33)) }, rule: RuleConstraint, column: "owner"},
		{name: "10#readable-values: empty text of a nullable column", mut: func(r *Row) { r.Scalars["note"] = sp("") }, out: doc("# Title", "- position: 1", "- done: true", "- note:", "")},
		{name: "10#readable-values: absent nullable column", mut: func(r *Row) { r.Scalars["owner"] = nil }, out: baseOut},
		{name: "10#readable-values: raw value kept as read", mut: func(r *Row) {
			r.Scalars["seen_on"] = sp("2026-02-30")
			r.Raw = map[string]string{"seen_on": "2026-02-30"}
		}, out: doc("# Title", "- position: 1", "- done: true", "- seen_on: 2026-02-30", "")},
		{name: "10#readable-values: raw value changed by the write", mut: func(r *Row) {
			r.Scalars["seen_on"] = sp("2026-02-31")
			r.Raw = map[string]string{"seen_on": "2026-02-30"}
		}, rule: RuleConstraint, column: "seen_on"},
		{name: "10#the-field-list-of-a-files-row: column of another table", mut: func(r *Row) { r.Scalars["nope"] = sp("x") }, rule: RuleConstraint, column: "nope"},
		{name: "10#the-field-list-of-a-files-row: the key column as a scalar", mut: func(r *Row) { r.Scalars["id"] = sp("T1") }, rule: RuleConstraint, column: "id"},
		{name: "10#foreign-keys-as-links: a foreign key to a row that is not there", mut: func(r *Row) { r.Scalars["parent"] = sp("T9") }, rule: RuleForeignKey, column: "parent"},
		{name: "10#foreign-keys-as-links: a dangling foreign key keeps its written target", mut: func(r *Row) {
			r.Scalars["parent"] = sp("T9")
			r.Dangling = map[string]DanglingLink{"parent": {Key: "T9", Target: "../x/T9.flowmark.md"}}
		}, out: doc("# Title", "- parent: [T9](../x/T9.flowmark.md)", "- position: 1", "- done: true", "")},
		{name: "10#foreign-keys-as-links: a written target of another key is not kept", mut: func(r *Row) {
			r.Scalars["parent"] = sp("T9")
			r.Dangling = map[string]DanglingLink{"parent": {Key: "T8", Target: "../x/T8.flowmark.md"}}
		}, rule: RuleForeignKey, column: "parent"},
		{name: "10#readable-values: a write of a foreign key whose key is not of its column type is CONSTRAINT alone", mut: func(r *Row) {
			r.Scalars["parent"] = sp(strings.Repeat("T", 40))
		}, rule: RuleConstraint, column: "parent"},
		{name: "10#readable-values: a foreign key read with a key not of its column type is kept as read", mut: func(r *Row) {
			long := strings.Repeat("T", 40)
			r.Scalars["parent"] = sp(long)
			r.Raw = map[string]string{"parent": "[" + long + "](" + long + ".flowmark.md)"}
			r.Dangling = map[string]DanglingLink{"parent": {Key: long, Target: long + ".flowmark.md"}}
		}, out: doc("# Title", "- parent: ["+strings.Repeat("T", 40)+"]("+strings.Repeat("T", 40)+".flowmark.md)", "- position: 1", "- done: true", "")},
		{name: "10#readable-values: a foreign key changed from a key not of its column type is checked", mut: func(r *Row) {
			long := strings.Repeat("T", 40)
			r.Scalars["parent"] = sp(long + "U")
			r.Raw = map[string]string{"parent": "[" + long + "](" + long + ".flowmark.md)"}
		}, rule: RuleConstraint, column: "parent"},
		{name: "10#foreign-keys-as-links: a foreign key to a row", mut: func(r *Row) { r.Scalars["parent"] = sp("T2") }, out: doc("# Title", "- parent: [T2](T2.flowmark.md)", "- position: 1", "- done: true", "")},
		{name: "10#sections-and-their-values: body without a line feed", mut: func(r *Row) { r.Body = "x" }, rule: RuleConstraint, column: "body"},
		{name: "10#the-field-list-of-a-files-row: body that begins with a list item", mut: func(r *Row) { r.Body = "- x\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#the-field-list-of-a-files-row: body indented by two spaces after the fields", mut: func(r *Row) { r.Body = "  x\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#the-field-list-of-a-files-row: body indented by a tab after the fields", mut: func(r *Row) { r.Body = "\tx\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a leading blank line", mut: func(r *Row) { r.Body = "\nx\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a trailing blank line", mut: func(r *Row) { r.Body = "x\n\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a leading white space line", mut: func(r *Row) { r.Body = "  \nx\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a level-2 heading", mut: func(r *Row) { r.Body = "x\n## y\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a heading inside a list item", mut: func(r *Row) { r.Body = "Body.\n\n- a\n  ## x\n" }, out: withBase("Body.", "", "- a", "  ## x")},
		{name: "10#sections-and-their-values: body with a heading inside a block quote", mut: func(r *Row) { r.Body = "Body.\n\n> ## x\n" }, out: withBase("Body.", "", "> ## x")},
		{name: "10#sections-and-their-values: body with a top-level heading after a block quote", mut: func(r *Row) { r.Body = "Body.\n\n> a\n## x\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a level-1 heading", mut: func(r *Row) { r.Body = "# y\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a deeper heading", mut: func(r *Row) { r.Body = "### y\n" }, out: withBase("### y")},
		{name: "10#sections-and-their-values: body with a heading inside a fence", mut: func(r *Row) { r.Body = "```\n## y\n```\n" }, out: withBase("```", "## y", "```")},
		{name: "10#sections-and-their-values: body with an open fence before a section", mut: func(r *Row) {
			r.Body = "```\n"
			r.Sections["evidence"] = sp("x\n")
		}, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a setext H2 underline", mut: func(r *Row) { r.Body = "x\n---\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a setext H1 underline", mut: func(r *Row) { r.Body = "x\n===\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a thematic break after a blank line", mut: func(r *Row) { r.Body = "x\n\n---\n" }, out: withBase("x", "", "---")},
		{name: "10#sections-and-their-values: body with an open HTML comment before a section", mut: func(r *Row) {
			r.Body = "<!--\n"
			r.Sections["evidence"] = sp("x\n")
		}, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: body with a closed HTML comment before a section", mut: func(r *Row) {
			r.Body = "<!-- c -->\n"
			r.Sections["evidence"] = sp("x\n")
		}, out: withBase("<!-- c -->", "", "## evidence", "", "x")},
		{name: "10#sections-and-their-values: body with an open HTML comment at the end", mut: func(r *Row) { r.Body = "<!--\nx\n" }, out: withBase("<!--", "x")},
		{name: "10#sections-and-their-values: body with an open HTML block of type 6 that holds a heading as body text", mut: func(r *Row) { r.Body = "<div>\n## y\n" }, out: withBase("<div>", "## y")},
		{name: "10#sections-and-their-values: body with a heading taken into an HTML block of type 6 closed by a blank line", mut: func(r *Row) { r.Body = "<div>\n## y\n\nx\n" }, out: withBase("<div>", "## y", "", "x")},
		{name: "10#sections-and-their-values: body with an open HTML block of type 6 before a section after a blank line", mut: func(r *Row) {
			r.Body = "<div>\n"
			r.Sections["evidence"] = sp("x\n")
		}, out: withBase("<div>", "", "## evidence", "", "x")},
		{name: "10#sections-and-their-values: body with an open HTML block of type 7 before a section after a blank line", mut: func(r *Row) {
			r.Body = "<span>\n"
			r.Sections["evidence"] = sp("x\n")
		}, out: withBase("<span>", "", "## evidence", "", "x")},
		{name: "10#sections-and-their-values: body with a type 7 line of a paragraph before a section", mut: func(r *Row) {
			r.Body = "Body.\n<span>\n"
			r.Sections["evidence"] = sp("x\n")
		}, out: withBase("Body.", "<span>", "", "## evidence", "", "x")},
		{name: "10#sections-and-their-values: body with an open fence that holds a heading as body text", mut: func(r *Row) { r.Body = "```\n## y\n" }, out: withBase("```", "## y")},
		{name: "10#sections-and-their-values: body with an open fence at the end", mut: func(r *Row) { r.Body = "```\nx\n" }, out: withBase("```", "x")},
		{name: "10#sections-and-their-values: body with an open fence in a list item before a section", mut: func(r *Row) {
			r.Body = "Body.\n\n- ```\n"
			r.Sections["evidence"] = sp("x\n")
		}, out: withBase("Body.", "", "- ```", "", "## evidence", "", "x")},
		{name: "10#sections-and-their-values: section with an open HTML comment before the next section", mut: func(r *Row) {
			r.Sections["evidence"] = sp("<!--\n")
			r.Sections["cause"] = sp("c\n")
		}, rule: RuleConstraint, column: "evidence"},
		{name: "10#encoding-and-line-structure: body with a bare CR", mut: func(r *Row) { r.Body = "a\rb\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#encoding-and-line-structure: body with CRLF", mut: func(r *Row) { r.Body = "a\r\nb\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#encoding-and-line-structure: body with a U+0000", mut: func(r *Row) { r.Body = "a\x00b\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#encoding-and-line-structure: body that is not UTF-8", mut: func(r *Row) { r.Body = "\xff\n" }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: section without a line feed", mut: func(r *Row) { r.Sections["evidence"] = sp("x") }, rule: RuleConstraint, column: "evidence"},
		{name: "10#sections-and-their-values: section with a level-2 heading", mut: func(r *Row) { r.Sections["evidence"] = sp("## x\n") }, rule: RuleConstraint, column: "evidence"},
		{name: "10#sections-and-their-values: section with a setext underline", mut: func(r *Row) { r.Sections["evidence"] = sp("a\n===\n") }, rule: RuleConstraint, column: "evidence"},
		{name: "10#sections-and-their-values: section with a leading blank line", mut: func(r *Row) { r.Sections["evidence"] = sp("\nx\n") }, rule: RuleConstraint, column: "evidence"},
		{name: "10#sections-and-their-values: section that is not declared", mut: func(r *Row) { r.Sections["nope"] = sp("x\n") }, rule: RuleConstraint, column: "nope"},
		{name: "10#sections-and-their-values: body given as a section", mut: func(r *Row) { r.Sections["body"] = sp("x\n") }, rule: RuleConstraint, column: "body"},
		{name: "10#sections-and-their-values: present empty section", mut: func(r *Row) { r.Sections["cause"] = sp("") }, out: withBase("## cause")},
		{name: "10#sections-and-their-values: section with its value", mut: func(r *Row) { r.Sections["evidence"] = sp("Obs.\n") }, out: withBase("## evidence", "", "Obs.")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := validRow()
			c.mut(row)
			out, diags := renderFileOf(t, cat, "tasks", row, lk)
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
			if diags[0].Column != c.column || diags[0].Table != row.Table || diags[0].Key != row.Key || diags[0].Index != -1 {
				t.Fatalf("first refusal at column %q of %s/%s (index %d); want column %q of %s/%s (index -1): %+v",
					diags[0].Column, diags[0].Table, diags[0].Key, diags[0].Index, c.column, row.Table, row.Key, diags[0])
			}
		})
	}
}

// invisibleClass is one character of the invisible-character rule of spec 10
// (Single-line places). An invisible character is refused at an end of a value
// on write and is format.block on read; a space or a tab, which CommonMark
// strips at the ends of a heading and of a paragraph, is format.noncanonical on
// read; a character that is text is accepted at either end.
type invisibleClass struct {
	name      string
	r         rune
	invisible bool
	stripped  bool
}

var invisibleClasses = []invisibleClass{
	{"U+0020 SPACE (White_Space, stripped by CommonMark)", ' ', true, true},
	{"U+0009 TAB (Cc, stripped by CommonMark)", '\t', true, true},
	{"U+000B VT (Cc and White_Space)", 0x0B, true, false},
	{"U+001C FILE SEPARATOR (Cc, not White_Space)", 0x1C, true, false},
	{"U+0085 NEXT LINE (Cc and White_Space)", 0x85, true, false},
	{"U+00A0 NO-BREAK SPACE (White_Space)", 0xA0, true, false},
	{"U+3000 IDEOGRAPHIC SPACE (White_Space)", 0x3000, true, false},
	{"U+200B ZERO WIDTH SPACE (Cf and Default_Ignorable_Code_Point)", 0x200B, true, false},
	{"U+FEFF ZERO WIDTH NO-BREAK SPACE (Cf and Default_Ignorable_Code_Point)", 0xFEFF, true, false},
	{"U+3164 HANGUL FILLER (Default_Ignorable_Code_Point only)", 0x3164, true, false},
	{"U+034F COMBINING GRAPHEME JOINER (Default_Ignorable_Code_Point only)", 0x034F, true, false},
	{"U+FE0F VARIATION SELECTOR-16 (Default_Ignorable_Code_Point only)", 0xFE0F, true, false},
	{"U+1680 OGHAM SPACE MARK (White_Space, text)", 0x1680, false, false},
	{"U+0600 ARABIC NUMBER SIGN (Cf and Prepended_Concatenation_Mark, text)", 0x0600, false, false},
	{"U+06DD ARABIC END OF AYAH (Cf and Prepended_Concatenation_Mark, text)", 0x06DD, false, false},
}

// TestInvisibleEdges applies the invisible-character rule to the two single-line
// places of a files row that the codec holds: the H1 title and a field-list
// value. Every class is tried at the lead, at the trail and inside the value.
func TestInvisibleEdges(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	type place struct {
		name string
		row  func(v string) *Row
		file func(v string) string
	}
	places := []place{
		{
			name: "10#single-line-places: H1 title",
			row: func(v string) *Row {
				r := validRow()
				r.Title = v
				return r
			},
			file: func(v string) string { return doc("# "+v, "- position: 1", "- done: true", "") },
		},
		{
			name: "10#single-line-places: field-list value",
			row: func(v string) *Row {
				r := validRow()
				r.Scalars["note"] = sp(v)
				return r
			},
			file: func(v string) string { return doc("# Title", "- position: 1", "- done: true", "- note: "+v, "") },
		},
	}
	for _, pl := range places {
		for _, c := range invisibleClasses {
			for _, end := range []string{"lead", "trail", "interior"} {
				v := "a" + string(c.r) + "b"
				switch end {
				case "lead":
					v = string(c.r) + "a"
				case "trail":
					v = "a" + string(c.r)
				}
				edge := end != "interior"
				t.Run(pl.name+" "+end+" "+c.name, func(t *testing.T) {
					// Write: an invisible character at an end is CONSTRAINT; any other is written as it is.
					out, diags := renderFileOf(t, cat, "tasks", pl.row(v), lk)
					if c.invisible && edge {
						if out != nil || len(diags) == 0 {
							t.Fatalf("written %q; want CONSTRAINT", out)
						}
						for _, d := range diags {
							if d.Rule != RuleConstraint {
								t.Fatalf("diagnostic %+v; want CONSTRAINT", d)
							}
						}
					} else {
						if len(diags) > 0 {
							t.Fatalf("refused: %+v", diags)
						}
						if string(out) != pl.file(v) {
							t.Fatalf("written %q; want %q", out, pl.file(v))
						}
						checkP2(t, cat, pl.row(v), lk)
					}
					// Read: an invisible character at an end is format.block, except that a
					// space or a tab is format.noncanonical; the interior and the text are read.
					text := pl.file(v)
					row, rd := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", []byte(text), lk)
					switch {
					case c.invisible && edge && !c.stripped:
						if row != nil || len(rd) == 0 {
							t.Fatalf("read %q with row %v; want format.block", text, row != nil)
						}
						for _, d := range rd {
							if d.Rule != RuleBlock {
								t.Fatalf("diagnostic %+v; want format.block", d)
							}
						}
					case c.stripped && edge:
						if row == nil || len(rd) == 0 {
							t.Fatalf("read %q with row %v; want format.noncanonical", text, row != nil)
						}
						for _, d := range rd {
							if d.Rule != RuleNoncanonical {
								t.Fatalf("diagnostic %+v; want format.noncanonical", d)
							}
						}
						if out := checkP2(t, cat, row, lk); string(out) == text {
							t.Fatalf("noncanonical %q emits itself", text)
						}
					default:
						if len(rd) > 0 || row == nil {
							t.Fatalf("read %q: %+v; want accepted", text, rd)
						}
						if out := checkP2(t, cat, row, lk); string(out) != text {
							t.Fatalf("P1: emit %q; want %q", out, text)
						}
					}
				})
			}
		}
	}
}

// TestMemosHaveNoFieldList covers a files table without a non-null stored
// scalar, whose file has no field list: the body follows the H1 after one blank.
func TestMemosHaveNoFieldList(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	cases := []parseCase{
		accept("10#document-order: body after one blank line", "# Memo\n\nBody.\n"),
		accept("10#document-order: empty body", "# Memo\n"),
		accept("10#the-field-list-of-a-files-row: indented body without a field list", "# Memo\n\n  x\n"),
		noncanonical("10#document-order: extra blank line before the body", 3, "# Memo\n\n\nBody.\n"),
		noncanonical("10#document-order: body without the blank line", 2, "# Memo\nBody.\n"),
		noncanonical("10#document-order: blank line with no body", 2, "# Memo\n\n"),
		reject("10#the-field-list-of-a-files-row: body that begins with a list item", RuleBlock, 3, "# Memo\n\n- item\n"),
		reject("10#sections-and-their-values: undeclared level-2 heading after the body", RuleSection, 5, "# Memo\n\nBody.\n\n## nope\n"),
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

// TestDiagnosticColumnsCountCodePoints pins 10#diagnostics: a column is counted in
// code points, so the element after multi-byte text is at its code-point column.
func TestDiagnosticColumnsCountCodePoints(t *testing.T) {
	cat := loadFixture(t)
	lk := fixtureLinker()
	cases := []struct {
		name, text string
		line, col  int
		rule       string
	}{
		{"invalid UTF-8 after multi-byte text", "# Title\n- position: 1\n- done: true\n- owner: 한\xff\n", 4, 11, RuleEncoding},
		{"U+0000 after multi-byte text", "# 한글\n- position: 1\n- done: true\n\n한\x00\n", 5, 2, RuleEncoding},
		{"raw HTML after multi-byte text", "# Title\n- position: 1\n- done: true\n- note: 한 <b>x\n", 4, 9, RuleBlock},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags := ParseFile(cat, "tasks", "tasks/T1.flowmark.md", []byte(c.text), lk)
			if len(diags) == 0 || diags[0].Rule != c.rule || diags[0].Line != c.line || diags[0].Column != c.col {
				t.Fatalf("diagnostics %+v; want %s at %d:%d", diags, c.rule, c.line, c.col)
			}
		})
	}
}
