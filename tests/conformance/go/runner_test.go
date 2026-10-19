package conformance

import (
	"strings"
	"testing"
)

// baseVector is a valid vector of the runner's own tests: a write into a document table that the repeat refuses.
const baseVector = `{
  "name": "runner",
  "storage": "document table",
  "catalog": "document-table",
  "corpus": {
    "tasks.flowmark.md": [
      "# tasks",
      "| id | parent | title | done | due | label |",
      "| --- | --- | --- | --- | --- | --- |",
      "| T1 |  | One | false |  |  |"
    ]
  },
  "request": { "kind": "insert", "entity": "tasks", "set": [ { "column": "id", "p": 0 }, { "column": "title", "p": 1 }, { "column": "body", "p": 2 }, { "column": "done", "p": 3 } ] },
  "params": [ "T2", "Two", "", false ],
  "now": "2026-10-11 09:00:00.000000",
  "expect": {
    "affected": 1,
    "keys": [ "T2" ],
    "corpus": {
      "tasks.flowmark.md": [
        "# tasks",
        "| id | parent | title | done | due | label |",
        "| --- | --- | --- | --- | --- | --- |",
        "| T1 |  | One | false |  |  |",
        "| T2 |  | Two | false |  |  |"
      ]
    }
  },
  "repeat": { "error": "DUPLICATE_KEY" }
}`

// mutate returns the base vector with old replaced by new, failing when old is not in it once.
func mutate(t *testing.T, old, new string) string {
	t.Helper()
	if strings.Count(baseVector, old) != 1 {
		t.Fatalf("the base vector holds %q %d times", old, strings.Count(baseVector, old))
	}
	return strings.Replace(baseVector, old, new, 1)
}

// TestRunner pins the runner's own checks (spec 90 The vector shape): the base vector runs, and each departure from
// it is a failure that names its cause.
func TestRunner(t *testing.T) {
	t.Run("90#the-vector-shape: the base vector runs", func(t *testing.T) {
		v, err := decodeVector("runner.json", []byte(baseVector))
		if err != nil {
			t.Fatal(err)
		}
		if err := runVector(t.TempDir(), v); err != nil {
			t.Fatal(err)
		}
	})
	decodes := []struct {
		name, file, text, want string
	}{
		{"90#the-vector-shape: a member the format does not name", "runner.json", mutate(t, `"now":`, `"clock": 1, "now":`), "unknown field"},
		{"90#the-vector-shape: a name that is not the file name", "other.json", baseVector, "is not the file name"},
		{"90#the-vector-shape: a storage none of the five", "runner.json", mutate(t, `"storage": "document table"`, `"storage": "document"`), "is none of"},
		{"90#the-vector-shape: an error expectation with answer members", "runner.json", mutate(t, `"repeat": { "error": "DUPLICATE_KEY" }`, `"repeat": { "error": "DUPLICATE_KEY", "affected": 1 }`), "names no answer member"},
		{"90#the-vector-shape: a line of a text holding a line feed", "runner.json", mutate(t, `"# tasks",
      "| id`, `"# tasks\n",
      "| id`), "holds a line feed"},
	}
	for _, c := range decodes {
		t.Run(c.name, func(t *testing.T) {
			_, err := decodeVector(c.file, []byte(c.text))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want one naming %q", err, c.want)
			}
		})
	}
	runs := []struct {
		name, text, want string
	}{
		{"90#the-vector-shape: the storage is the entity's", mutate(t, `"storage": "document table"`, `"storage": "files"`), "storage"},
		{"90#required-properties: an undeclared diagnostic of the corpus before", mutate(t, `"| T1 |  | One | false |  |  |"
    ]`, `"| T1 |  | One | false |  |  |",
      ""
    ]`), "diagnostics of the corpus before"},
		{"90#required-properties: a declared diagnostic the corpus before lacks", mutate(t, `"request":`, `"diagnostics": { "tasks.flowmark.md": [ "format.noncanonical" ] }, "request":`), "diagnostics of the corpus before"},
		{"90#the-vector-shape: an answer member the answer does not hold", mutate(t, `"affected": 1,`, `"affected": 1, "count": 1,`), "members"},
		{"90#the-vector-shape: a member compared whole", mutate(t, `"keys": [ "T2" ]`, `"keys": [ "T3" ]`), "keys"},
		{"90#the-vector-shape: a file of the corpus after", mutate(t, `"| T2 |  | Two | false |  |  |"`, `"| T2 |  | 2 | false |  |  |"`), "corpus after"},
		{"90#the-vector-shape: a changed file the expectation does not name", mutate(t, `"corpus": {
      "tasks.flowmark.md": [
        "# tasks",
        "| id | parent | title | done | due | label |",
        "| --- | --- | --- | --- | --- | --- |",
        "| T1 |  | One | false |  |  |",
        "| T2 |  | Two | false |  |  |"
      ]
    }`, `"corpus": {}`), "corpus after"},
		{"90#the-vector-shape: a successful write without its repeat", mutate(t, `,
  "repeat": { "error": "DUPLICATE_KEY" }`, ``), "repeat"},
		{"90#required-properties: a repeat that departs from its expectation", mutate(t, `"repeat": { "error": "DUPLICATE_KEY" }`, `"repeat": { "error": "CONSTRAINT" }`), "repeat"},
	}
	for _, c := range runs {
		t.Run(c.name, func(t *testing.T) {
			v, err := decodeVector("runner.json", []byte(c.text))
			if err != nil {
				t.Fatal(err)
			}
			err = runVector(t.TempDir(), v)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want one naming %q", err, c.want)
			}
		})
	}
	t.Run("90#the-vector-shape: an expected error the request does not meet", func(t *testing.T) {
		v, err := decodeVector("runner.json", []byte(baseVector))
		if err != nil {
			t.Fatal(err)
		}
		code := "CONSTRAINT"
		v.Expect = &expectation{Error: &code}
		if err := runVector(t.TempDir(), v); err == nil || !strings.Contains(err.Error(), "expects the error CONSTRAINT") {
			t.Fatalf("error %v, want one naming the expected error", err)
		}
	})
	t.Run("90#the-vector-shape: a read that states a repeat", func(t *testing.T) {
		v, err := decodeVector("runner.json", []byte(mutate(t, `"repeat": { "error": "DUPLICATE_KEY" }`, `"equivalent": "x"`)))
		if err != nil {
			t.Fatal(err)
		}
		v.Request = []byte(`{"kind":"count","entity":"tasks"}`)
		v.Params = nil
		v.Expect = &expectation{Count: []byte("1"), members: map[string]struct{}{"count": {}}}
		v.Repeat = &expectation{Count: []byte("1"), members: map[string]struct{}{"count": {}}}
		if err := runVector(t.TempDir(), v); err == nil || !strings.Contains(err.Error(), "repeat") {
			t.Fatalf("error %v, want one naming the repeat", err)
		}
	})
	t.Run("90#required-properties: an equivalence group without a block vector", func(t *testing.T) {
		a := &vector{Name: "a", Storage: "files", Equivalent: "g", Request: []byte(`{}`), Now: "n", Expect: &expectation{}}
		b := &vector{Name: "b", Storage: "document list", Equivalent: "g", Request: []byte(`{}`), Now: "n", Expect: &expectation{}}
		if err := checkGroups([]*vector{a, b}); err == nil || !strings.Contains(err.Error(), "block") {
			t.Fatalf("error %v, want one naming the missing block vector", err)
		}
	})
	t.Run("90#required-properties: an equivalence group of two requests", func(t *testing.T) {
		a := &vector{Name: "a", Storage: "files", Equivalent: "g", Request: []byte(`{"kind":"count"}`), Now: "n", Expect: &expectation{}}
		b := &vector{Name: "b", Storage: "document list", Equivalent: "g", Request: []byte(`{"kind":"all"}`), Now: "n", Expect: &expectation{}}
		c := &vector{Name: "c", Storage: "block list", Equivalent: "g", Request: []byte(`{"kind":"count"}`), Now: "n", Expect: &expectation{}}
		if err := checkGroups([]*vector{a, b, c}); err == nil || !strings.Contains(err.Error(), "request") {
			t.Fatalf("error %v, want one naming the request", err)
		}
	})
	t.Run("90#the-vector-shape: a storage without a write vector", func(t *testing.T) {
		var vs []*vector
		for _, s := range storages {
			vs = append(vs, &vector{Name: s + " read", Storage: s, Request: []byte(`{"kind":"all"}`)})
			if s != "block table" {
				vs = append(vs, &vector{Name: s + " write", Storage: s, Request: []byte(`{"kind":"insert"}`)})
			}
		}
		if err := checkCoverage(vs); err == nil || !strings.Contains(err.Error(), "block table") {
			t.Fatalf("error %v, want one naming block table", err)
		}
	})
}
