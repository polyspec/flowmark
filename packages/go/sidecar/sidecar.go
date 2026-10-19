// Package sidecar is the soksak sidecar face (spec 40 The faces): protocol v1 of the soksak core specification
// (docs/spec/sidecars.md) — one JSON object per line over standard input and output, a page request
// {surface, root, body} answered {surface, body}, and a closed handshake — whose bodies are the operations of the ops
// layer over the corpus whose root is the request's root.
package sidecar

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/polyspec/flowmark/packages/go/ops"
)

// maxLine is the message limit of the protocol: 64 MiB before the line feed.
const maxLine = 64 << 20

// message is one message of the host: a page request or a closed handshake.
type message struct {
	Surface json.RawMessage `json:"surface"`
	Root    string          `json:"root"`
	CIDs    json.RawMessage `json:"cids"`
	Closed  bool            `json:"closed"`
	Body    json.RawMessage `json:"body"`
}

// answer is one message of the sidecar: the answer of a request, or of a closed handshake.
type answer struct {
	Surface json.RawMessage `json:"surface"`
	CIDs    json.RawMessage `json:"cids,omitempty"`
	Closed  bool            `json:"closed,omitempty"`
	Body    json.RawMessage `json:"body,omitempty"`
}

// Serve answers the host's messages of in on out until in ends, each operation waiting wait for the corpus lock. A line
// that is no request names no surface to answer, so it is written to errOut, which the host logs. It returns an error
// only when in or out fails, or a line passes the message limit.
func Serve(wait time.Duration, in io.Reader, out, errOut io.Writer) error {
	w := bufio.NewWriter(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), maxLine+1)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var m message
		if err := json.Unmarshal(line, &m); err != nil {
			fmt.Fprintf(errOut, "flowmark sidecar: the line %.200q is no message of protocol v1: %v\n", line, err)
			continue
		}
		if len(m.Surface) == 0 || string(m.Surface) == "null" {
			fmt.Fprintf(errOut, "flowmark sidecar: the message %.200q names no surface, so it has no answer\n", line)
			continue
		}
		a := answer{Surface: m.Surface}
		if m.Closed {
			// A surface holds no resource: every operation opens the corpus and closes its session.
			a.Closed = true
		} else {
			a.CIDs = m.CIDs
			a.Body = body(wait, m.Root, m.Body)
		}
		b, err := ops.JSON(a)
		if err != nil {
			return err
		}
		w.Write(b)
		w.WriteByte('\n')
		// The host reads each answer while the process runs, so each one is flushed on its own.
		if err := w.Flush(); err != nil {
			return err
		}
	}
	return sc.Err()
}

// body runs the operation of a request body over the corpus at root and returns the answer body: the operation's
// answer, or {code, error}, each with the request's id when it has one.
func body(wait time.Duration, root string, raw json.RawMessage) (answer json.RawMessage) {
	var members map[string]json.RawMessage
	if json.Unmarshal(raw, &members) != nil || members == nil {
		return failure(nil, &ops.Error{Code: "CONFIG", Msg: "the request body is not an object of an operation and its members"})
	}
	id := members["id"]
	delete(members, "id")
	opRaw, ok := members["operation"]
	delete(members, "operation")
	var op string
	if !ok {
		return failure(id, &ops.Error{Code: "CONFIG", Msg: "the request body names no operation"})
	}
	if json.Unmarshal(opRaw, &op) != nil {
		return failure(id, &ops.Error{Code: "CONFIG", Msg: "the operation of the request body is not a string"})
	}
	if !filepath.IsAbs(root) {
		return failure(id, &ops.Error{Code: "CONFIG", Msg: fmt.Sprintf("the root %s is not an absolute path; the host sends the project directory", root)})
	}
	r, err := ops.Parse(op, members)
	if err != nil {
		return failure(id, err)
	}
	ctx := context.Background()
	c, err := ops.Open(ctx, root, wait)
	if err != nil {
		return failure(id, err)
	}
	// The corpus holds the watch of its tree; it is closed on every way out, and a failure to close it is the answer.
	defer func() {
		if cerr := c.Close(); cerr != nil {
			answer = failure(id, cerr)
		}
	}()
	a, err := c.Run(ctx, "sidecar", r)
	if err != nil {
		return failure(id, err)
	}
	b, err := ops.JSON(a)
	if err != nil {
		return failure(id, err)
	}
	return withID(id, b)
}

// failure is the answer body of a failed request: {id, code, error}.
func failure(id json.RawMessage, err error) json.RawMessage {
	code, msg := ops.Failure(err)
	b, _ := ops.JSON(struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}{code, msg}) // two strings always marshal
	return withID(id, b)
}

// withID returns the JSON object obj with the member id first, when id is given.
func withID(id json.RawMessage, obj []byte) json.RawMessage {
	if id == nil {
		return obj
	}
	out := append([]byte(`{"id":`), id...)
	if rest := obj[1:]; len(rest) > 1 {
		out = append(out, ',')
		return append(out, rest...)
	}
	return append(out, '}')
}
