// Tests of the soksak sidecar face: the protocol v1 message loop over stdio.
package sidecar

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

// The host reads answers while the session runs: a message flushes before the input ends.
func TestInteractiveFlush(t *testing.T) {
	requests, requestWriter := io.Pipe()
	responses := &lockedBuffer{}
	go func() {
		requestWriter.Write([]byte("{\"surface\":1,\"root\":\".\",\"body\":{\"operation\":\"validate\"}}\n"))
	}()
	done := make(chan struct{})
	go func() {
		Serve(requests, responses)
		close(done)
	}()
	answer := responses.waitForLine(t, 5*time.Second)
	if !strings.Contains(answer, "\"surface\":1") {
		t.Fatalf("answer: %s", answer)
	}
	requestWriter.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not end after the input closed")
	}
}

// lockedBuffer collects the answers of one session.
type lockedBuffer struct {
	mu     sync.Mutex
	writer *bytes.Buffer
	lines  []string
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.writer == nil {
		b.writer = &bytes.Buffer{}
	}
	n, err := b.writer.Write(p)
	for strings.Contains(b.writer.String(), "\n") {
		text := b.writer.String()
		at := strings.IndexByte(text, '\n')
		b.lines = append(b.lines, text[:at])
		b.writer.Reset()
		rest := text[at+1:]
		b.writer.WriteString(rest)
	}
	return n, err
}

// waitForLine returns the first answer line or fails after the timeout.
func (b *lockedBuffer) waitForLine(t *testing.T, timeout time.Duration) string {
	t.Helper()
	deadline := time.After(timeout)
	for {
		b.mu.Lock()
		if len(b.lines) > 0 {
			line := b.lines[0]
			b.mu.Unlock()
			return line
		}
		b.mu.Unlock()
		select {
		case <-deadline:
			t.Fatal("no answer line within the timeout")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
