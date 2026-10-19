// Package mcp is the MCP server face (spec 40 The faces): JSON-RPC 2.0 over standard input and output, one message per
// line, with one tool per operation of the ops layer over the corpus the command line names.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/polyspec/flowmark/packages/go/ops"
)

// maxLine is the longest message line the server reads: the sidecar protocol's message limit, which holds the answer
// of any operation the faces share.
const maxLine = 64 << 20

// The JSON-RPC 2.0 error codes the server answers with.
const (
	parseError     = -32700
	invalidRequest = -32600
	methodNotFound = -32601
	invalidParams  = -32602
)

// request is one JSON-RPC message from the client; a message without an id is a notification.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// rpcError is a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// response is one JSON-RPC response; exactly one of Result and Error is set.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve answers the messages of in on out until in ends: each tool call opens the corpus at root through the write path,
// each operation waiting wait for the lock, so every call reads the catalog as it stands. It returns an error only when
// in or out fails.
func Serve(root string, wait time.Duration, in io.Reader, out io.Writer) error {
	w := bufio.NewWriter(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), maxLine+1)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		resp := handle(root, wait, line)
		if resp == nil {
			continue
		}
		b, err := ops.JSON(resp)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
		// The client reads each response while the session runs, so each one is flushed on its own.
		if err := w.Flush(); err != nil {
			return err
		}
	}
	return sc.Err()
}

// handle answers one message line, nil for a notification.
func handle(root string, wait time.Duration, line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{parseError, "the line is not JSON: " + err.Error()}}
	}
	if len(req.ID) == 0 {
		return nil // a notification is not answered
	}
	resp := &response{JSONRPC: "2.0", ID: req.ID}
	if req.JSONRPC != "2.0" || req.Method == "" {
		resp.Error = &rpcError{invalidRequest, "the message is no JSON-RPC 2.0 request with a method"}
		return resp
	}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "flowmark", "version": "0.0.1"},
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": tools()}
	case "tools/call":
		result, err := call(root, wait, req.Params)
		if err != nil {
			resp.Error = err
		} else {
			resp.Result = result
		}
	default:
		resp.Error = &rpcError{methodNotFound, "the method " + req.Method + " is unknown"}
	}
	return resp
}

// toolName is the tool of the operation op: its name with _ for . (spec 40 The faces).
func toolName(op string) string { return strings.ReplaceAll(op, ".", "_") }

// memberSchemas are the JSON schemas of the members of the operations.
var memberSchemas = map[string]map[string]any{
	"table":    {"type": "string", "description": "the table, as the catalog declares it"},
	"key":      {"description": "the primary key value of the row, in the runtime value model JSON"},
	"values":   {"type": "object", "description": "the column values to assign, each in the runtime value model JSON"},
	"position": {"type": "integer", "description": "the 1-based position among the row's siblings"},
	"parent":   {"description": "the key of the new parent row, or null for the top"},
	"state":    {"type": "string", "description": "the state to enter"},
}

// descriptions describe the tools.
var descriptions = map[string]string{
	"schema":     "Answer the catalog of the corpus: its tables with their storage, keys, settings, columns and state machines",
	"rows":       "Answer every row of a table in storage order",
	"row.get":    "Answer the row of a table whose primary key is key",
	"row.insert": "Insert a row into a table, and answer its key and the table's rows",
	"row.update": "Assign column values to the row whose key is key, and answer the table's rows",
	"row.move":   "Move the row whose key is key to a position among its siblings, optionally under a new parent",
	"row.delete": "Delete the row whose key is key, and answer the table's rows",
	"transition": "Move the row whose key is key to a state of its state machine, with the columns the transition requires",
	"views":      "Answer the rows of the view table, or, with key, that view and the rows of its entity through its stored filter and sort",
}

// tools returns the tool of each operation.
func tools() []map[string]any {
	var out []map[string]any
	for _, op := range ops.Operations() {
		required, optional := ops.Members(op)
		props := map[string]any{}
		for _, m := range append(required, optional...) {
			props[m] = memberSchemas[m]
		}
		schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
		if len(required) > 0 {
			schema["required"] = required
		}
		out = append(out, map[string]any{"name": toolName(op), "description": descriptions[op], "inputSchema": schema})
	}
	return out
}

// call runs a tools/call: the operation's answer as one text content, or its failure with isError.
func call(root string, wait time.Duration, params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string                     `json:"name"`
		Arguments map[string]json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{invalidParams, "the params of tools/call: " + err.Error()}
	}
	op := ""
	for _, o := range ops.Operations() {
		if toolName(o) == p.Name {
			op = o
		}
	}
	if op == "" {
		return nil, &rpcError{invalidParams, "the tool " + p.Name + " is unknown"}
	}
	members := p.Arguments
	if members == nil {
		members = map[string]json.RawMessage{}
	}
	answer, err := runOp(root, wait, op, members)
	if err != nil {
		code, msg := ops.Failure(err)
		b, merr := ops.JSON(map[string]string{"code": code, "error": msg})
		if merr != nil {
			return nil, &rpcError{invalidParams, merr.Error()}
		}
		return content(string(b), true), nil
	}
	return content(answer, false), nil
}

// runOp opens the corpus and runs the operation op with its members, answering its JSON.
func runOp(root string, wait time.Duration, op string, members map[string]json.RawMessage) (_ string, err error) {
	r, err := ops.Parse(op, members)
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	c, err := ops.Open(ctx, root, wait)
	if err != nil {
		return "", err
	}
	// The corpus holds the watch of its tree; it is closed on every way out, and a failure to close it fails the call.
	defer func() {
		if cerr := c.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()
	a, err := c.Run(ctx, "mcp", r)
	if err != nil {
		return "", err
	}
	b, err := ops.JSON(a)
	if err != nil {
		return "", fmt.Errorf("the answer of %s does not marshal: %w", op, err)
	}
	return string(b), nil
}

// textContent is one text content of a tool result, and toolResult the result of a tool call.
type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError"`
}

// content is a tool result of one text content.
func content(text string, isError bool) toolResult {
	return toolResult{Content: []textContent{{Type: "text", Text: text}}, IsError: isError}
}
