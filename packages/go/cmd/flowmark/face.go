package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/polyspec/flowmark/packages/go/ops"
)

// faceCommand is the command line of one operation (spec 40 The faces): its positional arguments after the corpus, an
// optional last positional argument, and whether assignments follow them.
type faceCommand struct {
	usage    string
	fixed    []string
	optional string
	assigns  bool
}

var faceCommands = map[string]faceCommand{
	"schema":     {usage: "<corpus>"},
	"rows":       {usage: "<corpus> <table>", fixed: []string{"table"}},
	"row.get":    {usage: "<corpus> <table> <key>", fixed: []string{"table", "key"}},
	"row.insert": {usage: "<corpus> <table> [assignment…]", fixed: []string{"table"}, assigns: true},
	"row.update": {usage: "<corpus> <table> <key> [assignment…]", fixed: []string{"table", "key"}, assigns: true},
	"row.move":   {usage: "<corpus> <table> <key> <position> [<parent>]", fixed: []string{"table", "key", "position"}, optional: "parent"},
	"row.delete": {usage: "<corpus> <table> <key>", fixed: []string{"table", "key"}},
	"transition": {usage: "<corpus> <table> <key> <state> [assignment…]", fixed: []string{"table", "key", "state"}, assigns: true},
	"views":      {usage: "<corpus> [<key>]", optional: "key"},
}

// arg is a value of the command line: its readable text, or the wire value written after `:=`.
type arg struct {
	text string
	raw  json.RawMessage
}

// valueArg reads a value of the command line; false when a `:=` value is not JSON.
func valueArg(s string) (arg, bool) {
	if j, ok := strings.CutPrefix(s, ":="); ok {
		if !json.Valid([]byte(j)) {
			return arg{}, false
		}
		return arg{raw: json.RawMessage(j)}, true
	}
	return arg{text: s}, true
}

// face runs the command of the operation name.
func face(out, errOut io.Writer, name string, args []string) (code int) {
	spec := faceCommands[name]
	cmd := newCommand(name, errOut, true)
	pos, ok := cmd.parse(args)
	if !ok {
		return 2
	}
	bad := func(why string) int {
		return usage(errOut, fmt.Sprintf("%s: %s", name, why))
	}
	need := 1 + len(spec.fixed)
	if len(pos) < need {
		return bad("the command line lacks " + strings.Join(append([]string{"corpus"}, spec.fixed...)[len(pos):], ", "))
	}
	positional := map[string]arg{}
	for i, m := range spec.fixed {
		if m == "table" || m == "state" {
			positional[m] = arg{text: pos[1+i]} // a name, never a value
			continue
		}
		a, ok := valueArg(pos[1+i])
		if !ok {
			return bad(fmt.Sprintf("the %s %s is not JSON after :=", m, pos[1+i]))
		}
		positional[m] = a
	}
	rest := pos[need:]
	if spec.optional != "" && len(rest) > 0 {
		a, ok := valueArg(rest[0])
		if !ok {
			return bad(fmt.Sprintf("the %s %s is not JSON after :=", spec.optional, rest[0]))
		}
		positional[spec.optional] = a
		rest = rest[1:]
	}
	if !spec.assigns && len(rest) > 0 {
		return bad("the command takes no argument " + rest[0])
	}
	values := map[string]arg{}
	for _, a := range rest {
		var col, text string
		raw := false
		if i := strings.Index(a, ":="); i >= 0 && (strings.IndexByte(a, '=') == i+1) {
			col, text, raw = a[:i], a[i+2:], true
		} else if i := strings.IndexByte(a, '='); i >= 0 {
			col, text = a[:i], a[i+1:]
		} else {
			return bad("the assignment " + a + " is not <column>=<text> or <column>:=<json>")
		}
		if col == "" {
			return bad("the assignment " + a + " names no column")
		}
		if _, dup := values[col]; dup {
			return bad("the column " + col + " is assigned twice")
		}
		v := arg{text: text}
		if raw {
			if !json.Valid([]byte(text)) {
				return bad("the value of " + col + " is not JSON after :=")
			}
			v = arg{raw: json.RawMessage(text)}
		}
		values[col] = v
	}
	ctx := context.Background()
	c, err := ops.Open(ctx, pos[0], cmd.lockWait())
	if err != nil {
		return fail(errOut, name, err)
	}
	defer closeCorpus(errOut, name, c, &code)
	table := positional["table"].text
	tb, _ := c.Catalog().Table(table)
	if name == "views" {
		// The key is a key of the view table, which the catalog names.
		if tb = c.ViewTable(); tb != nil {
			table = tb.Name
		}
	}
	column := func(member string) string {
		if tb == nil {
			return ""
		}
		switch member {
		case "key":
			return tb.Key
		case "position":
			return tb.Order
		case "parent":
			return tb.Hierarchy()
		}
		return ""
	}
	wire := func(col string, a arg) json.RawMessage {
		if a.raw != nil {
			return a.raw
		}
		return c.TextValue(table, col, a.text)
	}
	members := map[string]json.RawMessage{}
	for m, a := range positional {
		switch m {
		case "table", "state":
			b, err := json.Marshal(a.text)
			if err != nil {
				return fail(errOut, name, err)
			}
			members[m] = b
		default:
			members[m] = wire(column(m), a)
		}
	}
	if spec.assigns && len(values) > 0 {
		obj := map[string]json.RawMessage{}
		for col, a := range values {
			obj[col] = wire(col, a)
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return fail(errOut, name, err)
		}
		members["values"] = b
	} else if name == "row.insert" || name == "row.update" {
		members["values"] = json.RawMessage("{}")
	}
	r, err := ops.Parse(name, members)
	if err != nil {
		return fail(errOut, name, err)
	}
	a, err := c.Run(ctx, "cli", r)
	if err != nil {
		return fail(errOut, name, err)
	}
	b, err := ops.JSON(a)
	if err != nil {
		return fail(errOut, name, err)
	}
	pr := &printer{w: out}
	pr.printf("%s\n", b)
	return pr.status(errOut, name)
}
