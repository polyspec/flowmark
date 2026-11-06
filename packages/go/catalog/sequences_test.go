package catalog_test

import (
	"encoding/json"
	"maps"
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

// TestSequencesFile pins the file of the counters: the catalog takes dbspec/sequences.json as its own file, a JSON
// object of positive integer counters by table, which RenderSequences writes back by table name.
func TestSequencesFile(t *testing.T) {
	if _, diags := catalog.Load(writeCorpus(t, map[string]string{"counted.dbs": sequenceSet, "sequences.json": `{"counted": 3}`})); len(diags) > 0 {
		t.Fatalf("a corpus with the file of its counters: %v", diags)
	}
	accepted := []struct {
		name, data string
		want       map[string]int64
	}{
		{"no counter", `{}`, map[string]int64{}},
		{"counters", `{"b": 7, "a": 3}`, map[string]int64{"a": 3, "b": 7}},
		{"the largest int64", `{"a": 9223372036854775807}`, map[string]int64{"a": 9223372036854775807}},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			got, err := catalog.ParseSequences([]byte(tc.data))
			if err != nil || !maps.Equal(got, tc.want) {
				t.Fatalf("%s reads %v, %v, want %v", tc.data, got, err, tc.want)
			}
			back, err := catalog.ParseSequences(catalog.RenderSequences(got))
			if len(got) > 0 && (err != nil || !maps.Equal(back, got)) {
				t.Fatalf("the rendered counters read %v, %v", back, err)
			}
		})
	}
	if got, err := catalog.ParseSequences(nil); got != nil || err != nil {
		t.Fatalf("no file reads %v, %v", got, err)
	}
	if got := catalog.RenderSequences(nil); got != nil {
		t.Fatalf("no counter renders %q, want no file", got)
	}
	if got := string(catalog.RenderSequences(map[string]int64{"b": 7, "a": 3})); got != "{\n  \"a\": 3,\n  \"b\": 7\n}\n" {
		t.Fatalf("the counters render %q", got)
	}
	rejected := map[string]string{
		"a zero counter":     `{"a": 0}`,
		"a negative counter": `{"a": -1}`,
		"a fraction":         `{"a": 1.5}`,
		"a text":             `{"a": "1"}`,
		"past int64":         `{"a": 9223372036854775808}`,
		"null":               `null`,
		"an array":           `[1]`,
		"no JSON":            `{"a": 1`,
	}
	for name, data := range rejected {
		t.Run(name, func(t *testing.T) {
			if got, err := catalog.ParseSequences([]byte(data)); err == nil {
				t.Fatalf("%s reads %v", data, got)
			}
		})
	}
}
