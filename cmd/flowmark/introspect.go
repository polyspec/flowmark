package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/polyspec/flowmark/introspect"
)

// introspectTo runs `flowmark introspect <corpus>` (spec 30 Introspection) with the arguments after the command
// name. It writes the answer to out as one JSON object and a failure to errOut. It returns 0 when there is no
// diagnostic, 1 when there is one or the command failed, and 2 for a wrong command line.
func introspectTo(out, errOut io.Writer, args []string) int {
	fs := flag.NewFlagSet("introspect", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(errOut, "usage: flowmark introspect <corpus>")
		return 2
	}
	root, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(errOut, "flowmark introspect: the corpus root %s: %v\n", fs.Arg(0), err)
		return 1
	}
	res, err := introspect.Corpus(root)
	if err != nil {
		fmt.Fprintf(errOut, "flowmark introspect: %v\n", err)
		return 1
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(answer(res)); err != nil {
		fmt.Fprintf(errOut, "flowmark introspect: %v\n", err)
		return 1
	}
	if _, err := out.Write(buf.Bytes()); err != nil {
		fmt.Fprintf(errOut, "flowmark introspect: %v\n", err)
		return 1
	}
	if len(res.Diagnostics) > 0 {
		return 1
	}
	return 0
}

// jsonDiagnostic is a diagnostic in the answer's JSON form.
type jsonDiagnostic struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// jsonAnswer is the JSON object the command prints: a null catalog when it does not load, and an empty list when
// there is no diagnostic.
type jsonAnswer struct {
	Catalog     *introspect.Catalog `json:"catalog"`
	Diagnostics []jsonDiagnostic    `json:"diagnostics"`
}

func answer(res *introspect.Result) jsonAnswer {
	a := jsonAnswer{Catalog: res.Catalog, Diagnostics: make([]jsonDiagnostic, 0, len(res.Diagnostics))}
	for _, d := range res.Diagnostics {
		a.Diagnostics = append(a.Diagnostics, jsonDiagnostic{File: d.File, Line: d.Line, Column: d.Column, Rule: d.Rule, Message: d.Message})
	}
	return a
}
