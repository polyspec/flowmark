// Tests of the MCP server face: one stdio session (JSON-RPC 2.0, one message per line).
package mcp

import (
	"bytes"
	"strings"
	"testing"
)

func session(t *testing.T, input string) string {
	t.Helper()
	var out bytes.Buffer
	Serve(strings.NewReader(input), &out)
	return out.String()
}

func TestInitialize(t *testing.T) {
	out := session(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test"}}}`+"\n")
	if !strings.Contains(out, `"protocolVersion"`) || !strings.Contains(out, `"tools"`) {
		t.Fatalf("initialize: %s", out)
	}
}

func TestToolsList(t *testing.T) {
	out := session(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t"}}}`+"\n"+
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`+"\n")
	if !strings.Contains(out, `"flowmark_items"`) || !strings.Contains(out, `"flowmark_transition"`) || !strings.Contains(out, `"flowmark_validate"`) {
		t.Fatalf("tools/list: %s", out)
	}
}

func TestToolsCallItems(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir+"/list.md", "<!-- doc-id: list -->\n# L\n\n## G\n\n- [o] T1 One\n"); err != nil {
		t.Fatal(err)
	}
	request := strings.Replace(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"flowmark_items","arguments":{"root":"DIR"}}}`, "DIR", dir, 1)
	out := session(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t"}}}`+"\n"+request+"\n")
	if !strings.Contains(out, "[o]") || !strings.Contains(out, "T1") {
		t.Fatalf("tools/call: %s", out)
	}
}
