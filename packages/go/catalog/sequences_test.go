package catalog_test

import (
	"encoding/json"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// sequenceSet is a set of one declared table with an identity key and one with a plain i64 key.
const sequenceSet = `dbspec 1 counted

table counted {
  id i64 identity
  title varchar(32)
  primary key (id)
}

table plain {
  id i64
  title varchar(32)
  primary key (id)
}
`

// TestAutoincrement pins which tables keep an AUTOINCREMENT counter (spec 40 SQL statements): a declared identity key,
// which the set renders AUTOINCREMENT, and a table whose schema statement declares it, as SQLite reads the statement; a
// plain INTEGER PRIMARY KEY, a word AUTOINCREMENT that is a name, a text or a comment, and a dropped table keep none, and
// a renamed table keeps its counter under its new name.
func TestAutoincrement(t *testing.T) {
	cases := []struct {
		name       string
		statements []string
		want       []string
	}{
		{"the declared tables", nil, []string{"counted"}},
		{"an AUTOINCREMENT key of SQL", []string{"CREATE TABLE a (id INTEGER PRIMARY KEY AUTOINCREMENT, v TEXT)"}, []string{"a", "counted"}},
		{"in lower case", []string{"create table a (id integer primary key autoincrement)"}, []string{"a", "counted"}},
		{"a plain INTEGER PRIMARY KEY", []string{"CREATE TABLE a (id INTEGER PRIMARY KEY, v TEXT)"}, []string{"counted"}},
		{"a column named autoincrement", []string{`CREATE TABLE a (id INTEGER PRIMARY KEY, "autoincrement" TEXT)`}, []string{"counted"}},
		{"a default text", []string{"CREATE TABLE a (id INTEGER PRIMARY KEY, v TEXT DEFAULT 'AUTOINCREMENT')"}, []string{"counted"}},
		{"a comment", []string{"CREATE TABLE a (id INTEGER PRIMARY KEY /* AUTOINCREMENT */, v TEXT)"}, []string{"counted"}},
		{"a table without a primary key", []string{"CREATE TABLE a (v TEXT)"}, []string{"counted"}},
		{"a renamed table", []string{"CREATE TABLE a (id INTEGER PRIMARY KEY AUTOINCREMENT)", "ALTER TABLE a RENAME TO b"}, []string{"b", "counted"}},
		{"a dropped table", []string{"CREATE TABLE a (id INTEGER PRIMARY KEY AUTOINCREMENT)", "DROP TABLE a"}, []string{"counted"}},
		{"a declared table made again by SQL", []string{"DROP TABLE counted", "CREATE TABLE counted (id INTEGER PRIMARY KEY, title TEXT)"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"counted.dbs": sequenceSet}
			if tc.statements != nil {
				data, err := json.Marshal(tc.statements)
				if err != nil {
					t.Fatal(err)
				}
				files["statements.json"] = string(data)
			}
			cat, diags := catalog.Load(writeCorpus(t, files))
			if len(diags) > 0 {
				t.Fatalf("the catalog does not load: %v", diags)
			}
			var got []string
			for _, tb := range cat.Tables {
				if tb.Autoincrement {
					got = append(got, tb.Name)
				}
			}
			slices.Sort(got)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("the AUTOINCREMENT tables are %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSequencesFile pins the file of the sequences (spec 40 SQL statements, AUTOINCREMENT counters): the catalog takes
// dbspec/sequences.json as its own file, a JSON object of the sequence of each table it names, an integer of int64 —
// zero and a negative value included, as SQLite's sqlite_sequence holds them — or null for no row of sqlite_sequence,
// which RenderSequences writes back by table name; anything else is refused.
func TestSequencesFile(t *testing.T) {
	if _, diags := catalog.Load(writeCorpus(t, map[string]string{"counted.dbs": sequenceSet, "sequences.json": `{"counted": 3}`})); len(diags) > 0 {
		t.Fatalf("a corpus with the file of its sequences: %v", diags)
	}
	n := func(v int64) *int64 { return &v }
	accepted := []struct {
		name, data string
		want       map[string]*int64
	}{
		{"no sequence", `{}`, map[string]*int64{}},
		{"sequences", `{"b": 7, "a": 3}`, map[string]*int64{"a": n(3), "b": n(7)}},
		{"zero", `{"a": 0}`, map[string]*int64{"a": n(0)}},
		{"a negative sequence", `{"a": -1}`, map[string]*int64{"a": n(-1)}},
		{"no row", `{"a": null}`, map[string]*int64{"a": nil}},
		{"the largest int64", `{"a": 9223372036854775807}`, map[string]*int64{"a": n(9223372036854775807)}},
		{"the smallest int64", `{"a": -9223372036854775808}`, map[string]*int64{"a": n(-9223372036854775808)}},
		{"white space", " \n{ \"a\" : 1 }\n", map[string]*int64{"a": n(1)}},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			got, err := catalog.ParseSequences([]byte(tc.data))
			if err != nil || !sameSequences(got, tc.want) {
				t.Fatalf("%s reads %v, %v, want %v", tc.data, got, err, tc.want)
			}
			back, err := catalog.ParseSequences(catalog.RenderSequences(got))
			if len(got) > 0 && (err != nil || !sameSequences(back, got)) {
				t.Fatalf("the rendered sequences read %v, %v", back, err)
			}
		})
	}
	if got, err := catalog.ParseSequences(nil); got != nil || err != nil {
		t.Fatalf("no file reads %v, %v", got, err)
	}
	if got := catalog.RenderSequences(nil); got != nil {
		t.Fatalf("no sequence renders %q, want no file", got)
	}
	if got := string(catalog.RenderSequences(map[string]*int64{"b": n(7), "a": nil})); got != "{\n  \"a\": null,\n  \"b\": 7\n}\n" {
		t.Fatalf("the sequences render %q", got)
	}
	rejected := map[string]string{
		"a fraction":          `{"a": 1.5}`,
		"an exponent":         `{"a": 1e2}`,
		"a text":              `{"a": "1"}`,
		"a boolean":           `{"a": true}`,
		"an object":           `{"a": {}}`,
		"past int64":          `{"a": 9223372036854775808}`,
		"below int64":         `{"a": -9223372036854775809}`,
		"a name twice":        `{"a": 1, "a": 2}`,
		"null":                `null`,
		"an array":            `[1]`,
		"a number":            `1`,
		"no JSON":             `{"a": 1`,
		"data after":          `{"a": 1} {}`,
		"an empty file":       ``,
		"a trailing comma":    `{"a": 1,}`,
		"a name of no string": `{1: 1}`,
	}
	for name, data := range rejected {
		t.Run(name, func(t *testing.T) {
			if got, err := catalog.ParseSequences([]byte(data)); err == nil {
				t.Fatalf("%q reads %v", data, got)
			}
		})
	}
}

// sameSequences reports whether a and b hold the same sequences, a nil one as nil.
func sameSequences(a, b map[string]*int64) bool {
	return maps.EqualFunc(a, b, func(x, y *int64) bool { return (x == nil) == (y == nil) && (x == nil || *x == *y) })
}

// goldenSequences is the golden fixture of the file of the sequences: every form of a sequence, written as
// RenderSequences writes it.
const goldenSequences = "testdata/sequences/golden.json"

// TestSequencesGolden pins the golden fixture of the file of the sequences: it reads every form of a sequence (P1) and
// renders back byte for byte (P2).
func TestSequencesGolden(t *testing.T) {
	data, err := os.ReadFile(goldenSequences)
	if err != nil {
		t.Fatal(err)
	}
	got, err := catalog.ParseSequences(data)
	if err != nil {
		t.Fatal(err)
	}
	n := func(v int64) *int64 { return &v }
	want := map[string]*int64{"absent": nil, "below": n(1), "largest": n(math.MaxInt64), "negative": n(-5), "smallest": n(math.MinInt64), "zero": n(0)}
	if !sameSequences(got, want) {
		t.Fatalf("the golden sequences read %v", got)
	}
	if back := catalog.RenderSequences(got); string(back) != string(data) {
		t.Fatalf("the golden sequences render %q, want %q", back, data)
	}
}

// FuzzSequences requires that a parse of any bytes never panics, and that sequences it reads render to bytes that read
// back to the same sequences and render to the same bytes; the seeds are the golden fixture and the cases of
// TestSequencesFile.
func FuzzSequences(f *testing.F) {
	data, err := os.ReadFile(goldenSequences)
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{string(data), `{}`, `{"a": 3}`, `{"a": null}`, `{"a": 0}`, `{"a": -1}`, `{"a": 1.5}`, `{"a": 1, "a": 2}`, `[1]`, `{"a": 1} {}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := catalog.ParseSequences(data)
		if err != nil {
			return
		}
		out := catalog.RenderSequences(got)
		if len(got) == 0 {
			if out != nil {
				t.Fatalf("no sequence renders %q", out)
			}
			return
		}
		back, err := catalog.ParseSequences(out)
		if err != nil || !sameSequences(back, got) {
			t.Fatalf("%q reads %v, which renders %q, which reads %v, %v", data, got, out, back, err)
		}
		if again := catalog.RenderSequences(back); string(again) != string(out) {
			t.Fatalf("the rendered sequences render again as %q, not %q", again, out)
		}
	})
}
