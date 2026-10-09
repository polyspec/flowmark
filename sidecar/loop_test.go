// Tests of the soksak sidecar face: the protocol v1 message loop over stdio.
package sidecar

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func corpusDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "docs", "plans", "list.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<!-- doc-id: list -->\n# L\n\n## G\n\n- [o] T1 One\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func lines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var messages []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		messages = append(messages, message)
	}
	return messages
}

func TestItemsRequest(t *testing.T) {
	root := corpusDir(t)
	input := `{"surface":1,"root":"` + root + `","body":{"operation":"items"}}` + "\n"
	var out bytes.Buffer
	Serve(strings.NewReader(input), &out)
	messages := lines(t, out.String())
	if len(messages) != 1 || messages[0]["surface"].(float64) != 1 {
		t.Fatalf("messages: %v", messages)
	}
	if !strings.Contains(messages[0]["body"].(string), "T1") {
		t.Fatalf("body: %v", messages[0]["body"])
	}
}

func TestClosedAnswer(t *testing.T) {
	input := "{\"surface\":7,\"root\":\"/tmp\",\"closed\":true}\n"
	var out bytes.Buffer
	Serve(strings.NewReader(input), &out)
	messages := lines(t, out.String())
	if len(messages) != 1 || messages[0]["surface"].(float64) != 7 || messages[0]["closed"] != true {
		t.Fatalf("messages: %v", messages)
	}
}

func TestInvalidLineFails(t *testing.T) {
	var out bytes.Buffer
	Serve(strings.NewReader("not json\n"), &out)
	if out.Len() != 0 {
		t.Fatalf("out: %q", out.String())
	}
}
