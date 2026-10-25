package main

import (
	"bytes"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"path/filepath"
	"strings"
	"testing"
)

// boardDBS is the board catalog of corpustest with a files table of notes.
const boardDBS = corpustest.BoardDBS + `
table notes {
  id varchar(16)
  title varchar(64)
  primary key (id)
  settings {
    store files
    title title
  }
}
`

// boardRoot writes a corpus holding only the board catalog and returns its root.
func boardRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{"dbspec/board.dbs": boardDBS})
	return root
}

// cli runs the command line args and returns its exit code, standard output and standard error.
func cli(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

// cliOK runs a command line that must exit 0 and returns its standard output.
func cliOK(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errOut := cli(t, args...)
	if code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, errOut)
	}
	return out
}

// TestFaceCommands pins the CLI face of the operations (spec 40 The faces): one command per operation, its values as
// readable text or JSON, its answer as one line of JSON, and the exit codes 0, 1 and 2.
func TestFaceCommands(t *testing.T) {
	t.Run("40#the-faces: install, then the operations over any table", func(t *testing.T) {
		root := boardRoot(t)
		out := cliOK(t, "install", root)
		for _, line := range []string{
			"[install] created items.flowmark.md\n",
			"[install] created notes\n",
			"[install] created people.flowmark.md\n",
			"[install] added the line *.flowmark.md text eol=lf to .gitattributes\n",
			"[install] 3 created\n",
		} {
			if !strings.Contains(out, line) {
				t.Fatalf("install printed %q, which lacks %q", out, line)
			}
		}
		if got := cliOK(t, "install", root); got != "[install] 0 created\n" {
			t.Fatalf("second install printed %q", got)
		}
		if got := cliOK(t, "row.insert", root, "people", "id=P1", "name=Ada", "age=36"); got != `{"affected":1,"keys":["P1"],"rows":[{"id":"P1","name":"Ada","age":36}]}`+"\n" {
			t.Fatalf("row.insert printed %s", got)
		}
		cliOK(t, "row.insert", "--lock-wait-ms", "0", root, "people", "id=P2", "name=Max & Co", "age:=null")
		if got := cliOK(t, "row.get", root, "people", "P2"); got != `{"row":{"id":"P2","name":"Max & Co","age":null}}`+"\n" {
			t.Fatalf("row.get printed %s", got)
		}
		cliOK(t, "row.insert", root, "items", "title=First", "body=")
		cliOK(t, "row.insert", root, "items", "title=Second", "body=")
		if got := cliOK(t, "row.move", root, "items", "T2", "1", "T1", "--lock-wait-ms", "0"); !strings.Contains(got, `{"id":"T2","parent":"T1"`) {
			t.Fatalf("row.move printed %s", got)
		}
		if got := cliOK(t, "row.move", root, "items", "T2", "2", ":=null"); !strings.Contains(got, `{"id":"T2","parent":null`) {
			t.Fatalf("row.move to the top printed %s", got)
		}
		cliOK(t, "transition", root, "items", "T1", "doing")
		if got := cliOK(t, "transition", root, "items", "T1", "done", "evidence=Tests pass."); !strings.Contains(got, `"state":"done"`) {
			t.Fatalf("transition printed %s", got)
		}
		cliOK(t, "row.update", root, "people", "P2", "age=40")
		if got := cliOK(t, "rows", root, "people"); got != `{"rows":[{"id":"P1","name":"Ada","age":36},{"id":"P2","name":"Max & Co","age":40}]}`+"\n" {
			t.Fatalf("rows printed %s", got)
		}
		cliOK(t, "row.delete", root, "people", "P2")
		if got := cliOK(t, "schema", root); !strings.HasPrefix(got, `{"manifest_hash":`) {
			t.Fatalf("schema printed %s", got)
		}
	})
	t.Run("40#the-faces: views runs a stored view through its filter and sort", func(t *testing.T) {
		root := t.TempDir()
		dbs := strings.Replace(boardDBS, "dbspec 1 board", "dbspec 1 flowmark", 1) + `
table views {
  id i64 identity
  title varchar(255)
  kind varchar(16)
  entity varchar(63)
  filter text
  sort text
  group_by varchar(63) null
  primary key (id)
  settings {
    store document table
    title title
    codec filter ordered_json
    codec sort ordered_json
  }
}
`
		corpustest.WriteTree(t, root, map[string]string{"dbspec/flowmark.dbs": dbs})
		cliOK(t, "install", root)
		cliOK(t, "row.insert", root, "people", "id=P1", "name=Ada", "age=36")
		cliOK(t, "row.insert", root, "people", "id=P2", "name=Max", "age=20")
		cliOK(t, "row.insert", root, "views", "title=Adults", "kind=table", "entity=people",
			`filter={"where":{"items":[{"pred":{"column":"age","op":"gte","p":0}}]},"params":[30]}`, "sort=[]")
		if got := cliOK(t, "views", root, "1"); !strings.HasSuffix(got, `"rows":[{"id":"P1","name":"Ada","age":36}]}`+"\n") {
			t.Fatalf("views printed %s", got)
		}
		if got := cliOK(t, "views", root); !strings.HasPrefix(got, `{"rows":[{"id":1,"title":"Adults"`) {
			t.Fatalf("views printed %s", got)
		}
	})
	t.Run("40#the-faces: a refused operation exits 1 with its code on standard error", func(t *testing.T) {
		root := boardRoot(t)
		cliOK(t, "install", root)
		code, out, errOut := cli(t, "row.get", root, "people", "P9")
		if code != 1 || out != "" || errOut != "flowmark row.get: NO_ROWS: the table people has no row with the key \"P9\"\n" {
			t.Fatalf("code %d, out %q, err %q", code, out, errOut)
		}
		code, _, errOut = cli(t, "row.insert", root, "people", "id=P1", "name=Ada", "age=x")
		if code != 1 || !strings.HasPrefix(errOut, "flowmark row.insert: CONSTRAINT: ") || !strings.Contains(errOut, "age") {
			t.Fatalf("code %d, err %q", code, errOut)
		}
		code, _, errOut = cli(t, "rows", filepath.Join(root, "absent"), "people")
		if code != 1 || !strings.HasPrefix(errOut, "flowmark rows: CONFIG: ") {
			t.Fatalf("code %d, err %q", code, errOut)
		}
		code, _, errOut = cli(t, "unlock", root)
		if code != 1 || !strings.HasPrefix(errOut, "flowmark unlock: CONFIG: ") {
			t.Fatalf("code %d, err %q", code, errOut)
		}
		code, _, errOut = cli(t, "history", "repair", root, "items", "T9")
		if code != 1 || !strings.HasPrefix(errOut, "flowmark history repair: NO_ROWS: ") {
			t.Fatalf("code %d, err %q", code, errOut)
		}
	})
	t.Run("40#the-faces: a wrong command line exits 2", func(t *testing.T) {
		root := boardRoot(t)
		for _, args := range [][]string{
			nil,
			{"frobnicate", root},
			{"rows", root},
			{"rows", root, "people", "extra"},
			{"row.get", root, "people"},
			{"row.insert", root, "people", "noequals"},
			{"row.insert", root, "people", "age:={"},
			{"row.insert", root, "people", "age=1", "age=2"},
			{"row.move", root, "items", "T1"},
			{"rows", "--lock-wait-ms", "x", root, "people"},
			{"history", "mend", root, "items", "T1"},
			{"install"},
			{"unlock"},
		} {
			if code, _, _ := cli(t, args...); code != 2 {
				t.Errorf("%q exited %d, want 2", args, code)
			}
		}
	})
}
