// Package mcp is the MCP server face: JSON-RPC 2.0 over standard input and output, one
// message per line. The tools are the corpus operations — items, transition and validate.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/polyspec/flowmark/app"
	"github.com/polyspec/flowmark/corpus"
	"github.com/polyspec/flowmark/executor"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  struct {
		Name      string `json:"name"`
		Arguments struct {
			Root     string `json:"root"`
			Path     string `json:"path"`
			ID       string `json:"id"`
			State    string `json:"state"`
			Evidence string `json:"evidence"`
		} `json:"arguments"`
	} `json:"params"`
}

// Serve reads one JSON-RPC message per line and writes one response per line.
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
		var message request
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			continue // a line that is not a request has no response
		}
		if len(message.ID) == 0 {
			continue // a notification has no response
		}
		result, err := handle(&message)
		reply(writer, message.ID, result, err)
	}
}

func handle(message *request) (any, error) {
	switch message.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "flowmark", "version": "0.0.1"},
		}, nil
	case "tools/list":
		return map[string]any{"tools": []map[string]any{
			tool("flowmark_items", "List the checklist items of a corpus", `{"type":"object","properties":{"root":{"type":"string"}},"required":["root"]}`),
			tool("flowmark_transition", "Apply one state transition to an item", `{"type":"object","properties":{"root":{"type":"string"},"path":{"type":"string"},"id":{"type":"string"},"state":{"type":"string"},"evidence":{"type":"string"}},"required":["root","path","id","state"]}`),
			tool("flowmark_validate", "Validate a corpus: keys, references and the strict parse", `{"type":"object","properties":{"root":{"type":"string"}},"required":["root"]}`),
		}}, nil
	case "tools/call":
		return callTool(message)
	}
	return nil, fmt.Errorf("the method %q is unknown", message.Method)
}

func tool(name, description, schema string) map[string]any {
	var inputSchema any
	json.Unmarshal([]byte(schema), &inputSchema)
	return map[string]any{"name": name, "description": description, "inputSchema": inputSchema}
}

func callTool(message *request) (any, error) {
	arguments := message.Params.Arguments
	switch message.Params.Name {
	case "flowmark_items":
		opened, err := executor.Open(arguments.Root)
		if err != nil {
			return nil, err
		}
		rows, err := opened.All("items", executor.Query{})
		if err != nil {
			return nil, err
		}
		lines := make([]string, 0, len(rows))
		for _, row := range rows {
			lines = append(lines, fmt.Sprintf("%s\t%s\t%s", row["state"], row["id"], row["title"]))
		}
		return content(strings.Join(lines, "\n")), nil
	case "flowmark_transition":
		if err := app.Transition(arguments.Root, arguments.Path, arguments.ID, arguments.State, arguments.Evidence); err != nil {
			return nil, err
		}
		return content(fmt.Sprintf("%s is %s", arguments.ID, arguments.State)), nil
	case "flowmark_validate":
		findings, err := corpus.Validate(arguments.Root)
		if err != nil {
			return nil, err
		}
		lines := make([]string, 0, len(findings))
		for _, finding := range findings {
			lines = append(lines, fmt.Sprintf("%s: line %d: %s: %s", finding.File, finding.Line, finding.Rule, finding.Message))
		}
		if len(lines) == 0 {
			lines = append(lines, "the corpus passes")
		}
		return content(strings.Join(lines, "\n")), nil
	}
	return nil, fmt.Errorf("the tool %q is unknown", message.Params.Name)
}

func content(text string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
}

func reply(writer *bufio.Writer, id json.RawMessage, result any, err error) {
	response := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id)}
	if err != nil {
		response["error"] = map[string]any{"code": -32000, "message": err.Error()}
	} else {
		response["result"] = result
	}
	text, err := json.Marshal(response)
	if err != nil {
		return
	}
	writer.Write(text)
	writer.WriteByte('\n')
	// A client reads each response while the session runs; a message flushes on its own.
	writer.Flush()
}

// writeFile is the test probe that places one document.
func writeFile(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}
