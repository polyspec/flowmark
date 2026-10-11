package mcp

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

// serve runs the server over the input lines and returns its output lines.
func serve(t *testing.T, root string, lines ...string) []string {
	t.Helper()
	var out bytes.Buffer
	if err := Serve(root, 0, strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	text := strings.TrimSuffix(out.String(), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// TestServer pins the MCP face (spec 40 The faces): JSON-RPC 2.0 over one line per message, one tool per operation.
func TestServer(t *testing.T) {
	root := corpusRoot(t)
	t.Run("40#the-faces: initialize and the tool list", func(t *testing.T) {
		got := serve(t, root,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
			`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		)
		if len(got) != 2 || !strings.Contains(got[0], `"serverInfo":{"name":"flowmark"`) {
			t.Fatalf("answers: %q", got)
		}
		var list struct {
			Result struct {
				Tools []struct {
					Name        string         `json:"name"`
					InputSchema map[string]any `json:"inputSchema"`
				} `json:"tools"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(got[1]), &list); err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, tl := range list.Result.Tools {
			names = append(names, tl.Name)
			if tl.InputSchema["type"] != "object" {
				t.Fatalf("tool %s schema: %v", tl.Name, tl.InputSchema)
			}
		}
		if want := "row_delete row_get row_insert row_move row_update rows schema transition"; strings.Join(names, " ") != want {
			t.Fatalf("tools: %v, want %s", names, want)
		}
	})
	t.Run("40#the-faces: a tool answers the operation's JSON, and a failure with isError", func(t *testing.T) {
		got := serve(t, root,
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"rows","arguments":{"table":"people"}}}`,
			`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"row_get","arguments":{"table":"people","key":"P9"}}}`,
			`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"row_insert","arguments":{"table":"people","values":{"id":"P2","name":"Max & Co"}}}}`,
		)
		want := []string{
			`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"{\"rows\":[{\"id\":\"P1\",\"name\":\"Ada\"}]}"}],"isError":false}}`,
			`{"jsonrpc":"2.0","id":4,"result":{"content":[{"type":"text","text":"{\"code\":\"NO_ROWS\",\"error\":\"the table people has no row with the key \\\"P9\\\"\"}"}],"isError":true}}`,
			`{"jsonrpc":"2.0","id":5,"result":{"content":[{"type":"text","text":"{\"affected\":1,\"keys\":[\"P2\"],\"rows\":[{\"id\":\"P1\",\"name\":\"Ada\"},{\"id\":\"P2\",\"name\":\"Max & Co\"}]}"}],"isError":false}}`,
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("answers:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})
	t.Run("40#the-faces: protocol errors are JSON-RPC errors", func(t *testing.T) {
		got := serve(t, root,
			`not json`,
			`{"jsonrpc":"2.0","id":6,"method":"resources/list"}`,
			`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"frobnicate","arguments":{}}}`,
			`{"jsonrpc":"2.0","id":8,"method":"ping"}`,
		)
		want := []string{
			`{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"the line is not JSON: invalid character 'o' in literal null (expecting 'u')"}}`,
			`{"jsonrpc":"2.0","id":6,"error":{"code":-32601,"message":"the method resources/list is unknown"}}`,
			`{"jsonrpc":"2.0","id":7,"error":{"code":-32602,"message":"the tool frobnicate is unknown"}}`,
			`{"jsonrpc":"2.0","id":8,"result":{}}`,
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("answers:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})
}
