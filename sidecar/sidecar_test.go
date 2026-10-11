package sidecar

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const peopleDBS = `dbspec 1 people

table people {
  id varchar(32)
  name varchar(64)
  primary key (id)
  settings {
    store document table
    title name
  }
}
`

// corpusRoot writes a corpus of one people table with one row and returns its root.
func corpusRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for p, text := range map[string]string{
		"dbspec/people.dbs":  peopleDBS,
		"people.flowmark.md": "# people\n| id | name |\n| --- | --- |\n| P1 | Ada |\n",
	} {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// serve runs the sidecar over the input lines and returns its output lines and its standard error.
func serve(t *testing.T, lines ...string) ([]string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	if err := Serve(0, strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, &errOut); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	text := strings.TrimSuffix(out.String(), "\n")
	if text == "" {
		return nil, errOut.String()
	}
	return strings.Split(text, "\n"), errOut.String()
}

// req is a request line of the surface s over root with body.
func req(t *testing.T, s, root, body string, extra ...string) string {
	t.Helper()
	r, _ := json.Marshal(root)
	line := `{"surface":"` + s + `","root":` + string(r) + `,"body":` + body
	for _, e := range extra {
		line += "," + e
	}
	return line + "}"
}

// TestSidecar pins the sidecar face (spec 40 The faces): protocol v1 of the soksak core specification over the
// operations of any table.
func TestSidecar(t *testing.T) {
	root := corpusRoot(t)
	t.Run("40#the-faces: a request is answered with the operation's answer, its id and its cids", func(t *testing.T) {
		got, errText := serve(t,
			req(t, "s1", root, `{"operation":"rows","table":"people","id":7}`, `"cids":["c1"]`),
			req(t, "s1", root, `{"operation":"row.insert","table":"people","values":{"id":"P2","name":"Max & Co"}}`),
		)
		want := []string{
			`{"surface":"s1","cids":["c1"],"body":{"id":7,"rows":[{"id":"P1","name":"Ada"}]}}`,
			`{"surface":"s1","body":{"affected":1,"keys":["P2"],"rows":[{"id":"P1","name":"Ada"},{"id":"P2","name":"Max & Co"}]}}`,
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") || errText != "" {
			t.Fatalf("answers:\n%s\nwant:\n%s\nstderr: %s", strings.Join(got, "\n"), strings.Join(want, "\n"), errText)
		}
	})
	t.Run("40#the-faces: a failure body carries the code and the error", func(t *testing.T) {
		got, _ := serve(t,
			req(t, "s2", root, `{"operation":"row.get","table":"people","key":"P9","id":"a"}`),
			req(t, "s2", root, `{"table":"people"}`),
			req(t, "s2", root, `["rows"]`),
			req(t, "s2", "corpus", `{"operation":"schema"}`),
		)
		want := []string{
			`{"surface":"s2","body":{"id":"a","code":"NO_ROWS","error":"the table people has no row with the key \"P9\""}}`,
			`{"surface":"s2","body":{"code":"CONFIG","error":"the request body names no operation"}}`,
			`{"surface":"s2","body":{"code":"CONFIG","error":"the request body is not an object of an operation and its members"}}`,
			`{"surface":"s2","body":{"code":"CONFIG","error":"the root corpus is not an absolute path; the host sends the project directory"}}`,
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("answers:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})
	t.Run("40#the-faces: closed is answered, and a line that is no request goes to standard error", func(t *testing.T) {
		r, _ := json.Marshal(root)
		got, errText := serve(t,
			`{"surface":"s3","root":`+string(r)+`,"closed":true}`,
			`not json`,
			`{"root":"/x","body":{}}`,
		)
		if want := `{"surface":"s3","closed":true}`; strings.Join(got, "\n") != want {
			t.Fatalf("answers: %q, want %s", got, want)
		}
		if strings.Count(errText, "\n") != 2 || !strings.Contains(errText, "not json") || !strings.Contains(errText, "no surface") {
			t.Fatalf("stderr: %q", errText)
		}
	})
}
