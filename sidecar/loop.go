// Package sidecar is the soksak sidecar face: the protocol v1 message loop of the soksak
// core specification — one JSON object per line over standard input and output, a page
// request as {surface, root, body}, the answers as {surface, body} or a closed handshake.
package sidecar

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/polyspec/flowmark/app"
	"github.com/polyspec/flowmark/corpus"
	"github.com/polyspec/flowmark/executor"
)

type message struct {
	Surface int             `json:"surface"`
	Root    string          `json:"root"`
	Closed  bool            `json:"closed"`
	Body    json.RawMessage `json:"body"`
}

type operation struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
	ID        string `json:"id"`
	State     string `json:"state"`
	Evidence  string `json:"evidence"`
}

// Serve reads the host messages and answers each: a closed handshake answers closed, a page
// request runs its operation over the corpus of the surface root. A line that is not a
// message of the protocol produces no answer; the host treats a protocol breach by killing
// the process, so the loop stops reading.
func Serve(in io.Reader, out io.Writer) {
	writer := bufio.NewWriter(out)
	defer writer.Flush()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var received message
		if err := json.Unmarshal([]byte(line), &received); err != nil {
			return // an invalid line ends the loop; the host reports the breach
		}
		if received.Closed {
			write(writer, map[string]any{"surface": received.Surface, "closed": true})
			continue
		}
		var operation operation
		if err := json.Unmarshal(received.Body, &operation); err != nil {
			return
		}
		answer, err := run(received.Root, &operation)
		if err != nil {
			write(writer, map[string]any{"surface": received.Surface, "body": err.Error()})
			continue
		}
		write(writer, map[string]any{"surface": received.Surface, "body": answer})
	}
}

func run(root string, operation *operation) (string, error) {
	switch operation.Operation {
	case "items":
		opened, err := executor.Open(root)
		if err != nil {
			return "", err
		}
		rows, err := opened.All("items", executor.Query{})
		if err != nil {
			return "", err
		}
		lines := make([]string, 0, len(rows))
		for _, row := range rows {
			lines = append(lines, fmt.Sprintf("%s\t%s\t%s", row["state"], row["id"], row["title"]))
		}
		return strings.Join(lines, "\n"), nil
	case "validate":
		findings, err := corpus.Validate(root)
		if err != nil {
			return "", err
		}
		lines := make([]string, 0, len(findings))
		for _, finding := range findings {
			lines = append(lines, fmt.Sprintf("%s: line %d: %s: %s", finding.File, finding.Line, finding.Rule, finding.Message))
		}
		if len(lines) == 0 {
			lines = append(lines, "the corpus passes")
		}
		return strings.Join(lines, "\n"), nil
	case "transition":
		if err := app.Transition(root, operation.Path, operation.ID, operation.State, operation.Evidence); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s is %s", operation.ID, operation.State), nil
	}
	return "", fmt.Errorf("the operation %q is unknown", operation.Operation)
}

func write(writer *bufio.Writer, message map[string]any) {
	text, err := json.Marshal(message)
	if err != nil {
		return
	}
	writer.Write(text)
	writer.WriteByte('\n')
	// The host reads each answer while the process runs; a message flushes on its own.
	writer.Flush()
}
