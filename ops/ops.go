// Package ops is the one operation layer of the CLI, MCP and sidecar faces (spec 40 The faces): it forms each
// operation over any table of the catalog as a request of the IR-executor protocol and runs it through a session of
// the write path, which holds every rule. No face reads or writes a corpus file itself, and neither does this package.
package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/irexec"
	"github.com/polyspec/flowmark/store/journal"
	"github.com/polyspec/flowmark/writepath"
	"github.com/polyspec/orm/engine/dbspec"
	"github.com/polyspec/orm/engine/ir"
)

// The codes this package answers with itself (orm errors.yaml).
const (
	codeConfig        = "CONFIG"
	codeNoRows        = "NO_ROWS"
	codeEntityUnknown = "ENTITY_UNKNOWN"
	codeSchemaInvalid = "SCHEMA_INVALID"
	codeInternal      = "INTERNAL"
)

// Error is a failure of the operation layer itself: an operation a face cannot form, a named row that matches none, a
// corpus that does not open.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

func errf(code, format string, a ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, a...)}
}

// Failure returns the code and the message a face answers for err: the face name of the failure vocabulary (spec 40),
// which is the orm code for every code the vocabulary does not list. An error of no code of the engine is INTERNAL.
func Failure(err error) (code, msg string) {
	var we *writepath.Error
	var ee *irexec.Error
	var oe *Error
	switch {
	case errors.As(err, &we):
		return we.Face, we.Msg
	case errors.As(err, &ee):
		return ee.Code, ee.Msg
	case errors.As(err, &oe):
		return oe.Code, oe.Msg
	}
	return codeInternal, err.Error()
}

// The operations of the faces, each with the members it requires and the members it may take besides.
var operations = map[string]struct{ required, optional []string }{
	"schema":     {},
	"rows":       {required: []string{"table"}},
	"row.get":    {required: []string{"table", "key"}},
	"row.insert": {required: []string{"table", "values"}},
	"row.update": {required: []string{"table", "key", "values"}},
	"row.move":   {required: []string{"table", "key", "position"}, optional: []string{"parent"}},
	"row.delete": {required: []string{"table", "key"}},
	"transition": {required: []string{"table", "key", "state"}, optional: []string{"values"}},
	"views":      {optional: []string{"key"}},
}

// Operations returns the names of the operations, sorted.
func Operations() []string {
	names := make([]string, 0, len(operations))
	for n := range operations {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Members returns the members the operation op requires and the members it may take besides.
func Members(op string) (required, optional []string) {
	spec := operations[op]
	return slices.Clone(spec.required), slices.Clone(spec.optional)
}

// Request is one operation with its members; every value is in the runtime value model JSON of the protocol.
type Request struct {
	Operation string
	Table     string
	Key       json.RawMessage
	Values    map[string]json.RawMessage
	Position  json.RawMessage
	// Parent is the value given for the hierarchy column, nil when the member is absent; JSON null moves the row to
	// the top.
	Parent json.RawMessage
	State  string
}

// Parse forms the operation op from its members. An unknown operation, a missing member, a member the operation does
// not take, a table or state that is not a string and values that are not an object are CONFIG.
func Parse(op string, members map[string]json.RawMessage) (*Request, error) {
	spec, ok := operations[op]
	if !ok {
		return nil, errf(codeConfig, "the operation %q is unknown; the operations are %s", op, strings.Join(Operations(), ", "))
	}
	for _, m := range spec.required {
		if _, ok := members[m]; !ok {
			return nil, errf(codeConfig, "the operation %s needs the member %s", op, m)
		}
	}
	names := make([]string, 0, len(members))
	for m := range members {
		names = append(names, m)
	}
	sort.Strings(names)
	for _, m := range names {
		if !slices.Contains(spec.required, m) && !slices.Contains(spec.optional, m) {
			return nil, errf(codeConfig, "the operation %s takes no member %s", op, m)
		}
	}
	r := &Request{Operation: op, Key: members["key"], Position: members["position"], Parent: members["parent"]}
	if raw, ok := members["table"]; ok {
		if json.Unmarshal(raw, &r.Table) != nil {
			return nil, errf(codeConfig, "the member table of %s is not a string: %s", op, raw)
		}
	}
	if raw, ok := members["state"]; ok {
		if json.Unmarshal(raw, &r.State) != nil {
			return nil, errf(codeConfig, "the member state of %s is not a string: %s", op, raw)
		}
	}
	if raw, ok := members["values"]; ok {
		if json.Unmarshal(raw, &r.Values) != nil || r.Values == nil {
			return nil, errf(codeConfig, "the member values of %s is not an object of column values: %s", op, raw)
		}
	}
	return r, nil
}

// noSubscriber is the observer of the lock and journal events of a face operation: nothing in a face subscribes to
// them yet, so they reach no one.
type noSubscriber struct{}

func (noSubscriber) Observe(journal.Event) {}

// Journal declares the journal corpus at root, an absolute or relative path of a folder, for the faces' operations
// below the write path — flowmark unlock. A root that is no folder is CONFIG.
func Journal(root string) (*journal.Corpus, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, errf(codeConfig, "the corpus root %s: %v", root, err)
	}
	j, err := journal.New(abs, noSubscriber{})
	if err != nil {
		return nil, errf(codeConfig, "%v", err)
	}
	return j, nil
}

// JSON returns the JSON text of an answer as every face writes it: without HTML escaping, so a value reads as it is
// stored, and without a line feed.
func JSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// Corpus is a corpus opened for the faces: its catalog and the write path.
type Corpus struct {
	cat *catalog.Catalog
	wp  *writepath.Corpus
}

// Open opens the corpus at root through the write path (spec 40 Opening a corpus), each operation waiting wait for the
// lock. A root that is no folder is CONFIG, and a catalog that does not load is SCHEMA_INVALID, naming each diagnostic.
func Open(ctx context.Context, root string, wait time.Duration) (*Corpus, error) {
	j, err := Journal(root)
	if err != nil {
		return nil, err
	}
	abs := j.Root()
	cat, diags := catalog.Load(abs)
	if cat == nil || len(diags) > 0 {
		lines := make([]string, len(diags))
		for i, d := range diags {
			lines[i] = fmt.Sprintf("%s:%d:%d %s: %s", d.File, d.Line, d.Column, d.Rule, d.Message)
		}
		return nil, errf(codeSchemaInvalid, "the catalog of %s does not load; repair dbspec/ first: %s", abs, strings.Join(lines, "; "))
	}
	wp, err := writepath.Open(ctx, cat, j, wait)
	if err != nil {
		return nil, err
	}
	return &Corpus{cat: cat, wp: wp}, nil
}

// ViewTable returns the view table of the catalog (spec 100 Views), nil when it has none.
func (c *Corpus) ViewTable() *catalog.Table {
	for _, tb := range c.cat.Tables {
		if tb.View != nil {
			return tb
		}
	}
	return nil
}

// Catalog returns the catalog of the corpus.
func (c *Corpus) Catalog() *catalog.Catalog { return c.cat }

// Session returns a new session of the write path named name, for the operations the faces run outside this package:
// canon, install, history repair.
func (c *Corpus) Session(name string) (*writepath.Session, error) { return c.wp.Session(name) }

// Run runs the operation r in a new session named session and returns its answer, which marshals to the answer's
// JSON (spec 40 The faces).
func (c *Corpus) Run(ctx context.Context, session string, r *Request) (any, error) {
	if r.Operation == "schema" {
		return schemaOf(c.cat), nil
	}
	var tb *catalog.Table
	if r.Operation == "views" {
		if tb = c.ViewTable(); tb == nil {
			return nil, errf(codeConfig, "views: the catalog has no view table, the table %s of the document %s (spec 100 Views)", catalog.ViewTable, catalog.ApplicationDocument)
		}
	} else {
		var ok bool
		if tb, ok = c.cat.Table(r.Table); !ok {
			return nil, errf(codeEntityUnknown, "the catalog declares no table %s", r.Table)
		}
	}
	s, err := c.wp.Session(session)
	if err != nil {
		return nil, err
	}
	a, err := c.run(ctx, s, tb, r)
	if cerr := s.Close(); err == nil && cerr != nil {
		return nil, cerr
	}
	return a, err
}

// rowAnswer is the answer of row.get, and writeAnswer the answer of a write.
type rowAnswer struct {
	Row *irexec.Row `json:"row"`
}

type writeAnswer struct {
	Affected int64         `json:"affected"`
	Keys     []any         `json:"keys"`
	Rows     []*irexec.Row `json:"rows"`
}

// run runs the operation r on the table tb in the session s.
func (c *Corpus) run(ctx context.Context, s *writepath.Session, tb *catalog.Table, r *Request) (any, error) {
	m := message{hash: c.cat.ManifestHash, table: tb.Name}
	switch r.Operation {
	case "rows":
		return c.exec(ctx, s, m.request("all", nil))
	case "views":
		if r.Key == nil {
			return c.exec(ctx, s, m.request("all", nil))
		}
		return c.view(ctx, s, tb, r.Key)
	case "row.get":
		a, err := c.exec(ctx, s, m.request("one", m.byKey(tb, r.Key)))
		if err != nil {
			return nil, err
		}
		if len(a.Rows) == 0 {
			return nil, noRow(tb, r.Key)
		}
		return rowAnswer{Row: a.Rows[0]}, nil
	case "row.insert":
		return c.write(ctx, s, tb, nil, m.request("insert", map[string]any{"set": m.sets(tb, r.Values)}))
	case "row.update":
		return c.write(ctx, s, tb, r.Key, m.request("update", m.update(tb, r.Key, m.sets(tb, r.Values))))
	case "row.delete":
		return c.write(ctx, s, tb, r.Key, m.request("delete", m.byKey(tb, r.Key)))
	case "row.move":
		if tb.Order == "" {
			return nil, errf(codeConfig, "row.move: the table %s declares no order column", tb.Name)
		}
		set := m.sets(tb, map[string]json.RawMessage{tb.Order: r.Position})
		if r.Parent != nil {
			h := tb.Hierarchy()
			if h == "" {
				return nil, errf(codeConfig, "row.move: the table %s has no hierarchy column parent, so a move takes no parent", tb.Name)
			}
			set = append(set, m.assign(h, r.Parent))
		}
		return c.write(ctx, s, tb, r.Key, m.request("update", m.update(tb, r.Key, set)))
	case "transition":
		if tb.StateMachine == nil {
			return nil, errf(codeConfig, "transition: the table %s declares no state machine", tb.Name)
		}
		state, _ := json.Marshal(r.State)
		values := map[string]json.RawMessage{}
		for k, v := range r.Values {
			values[k] = v
		}
		if _, ok := values[tb.StateMachine.Column]; ok {
			return nil, errf(codeConfig, "transition: values assigns the state column %s, which the member state gives", tb.StateMachine.Column)
		}
		values[tb.StateMachine.Column] = state
		return c.write(ctx, s, tb, r.Key, m.request("update", m.update(tb, r.Key, m.sets(tb, values))))
	}
	return nil, errf(codeInternal, "the operation %s has no runner", r.Operation)
}

// write runs the write msg and answers it with the table's rows read after its commit. A write of the row named by key
// that affects none is NO_ROWS.
func (c *Corpus) write(ctx context.Context, s *writepath.Session, tb *catalog.Table, key json.RawMessage, msg []byte) (any, error) {
	a, err := c.exec(ctx, s, msg)
	if err != nil {
		return nil, err
	}
	if key != nil && a.Affected == 0 {
		return nil, noRow(tb, key)
	}
	m := message{hash: c.cat.ManifestHash, table: tb.Name}
	rows, err := c.exec(ctx, s, m.request("all", nil))
	if err != nil {
		return nil, err
	}
	keys := a.Keys
	if keys == nil {
		keys = []any{}
	}
	out := rows.Rows
	if out == nil {
		out = []*irexec.Row{}
	}
	return writeAnswer{Affected: a.Affected, Keys: keys, Rows: out}, nil
}

// exec runs one request message in the session.
func (c *Corpus) exec(ctx context.Context, s *writepath.Session, msg []byte) (*irexec.Answer, error) {
	return s.Exec(ctx, msg)
}

// noRow is the failure of an operation on a named row that matches none.
func noRow(tb *catalog.Table, key json.RawMessage) *Error {
	return errf(codeNoRows, "the table %s has no row with the key %s", tb.Name, key)
}

// message forms the request messages of one table: {ir_version, manifest_hash, request, params, now}, the params
// collected as the request is formed.
type message struct {
	hash   string
	table  string
	params []json.RawMessage
}

// param adds a parameter and returns its index.
func (m *message) param(v json.RawMessage) int {
	m.params = append(m.params, v)
	return len(m.params) - 1
}

// assign is the assignment of the value v to column.
func (m *message) assign(column string, v json.RawMessage) map[string]any {
	return map[string]any{"column": column, "p": m.param(v)}
}

// sets returns the assignments of values in the table's column order; a name the table does not declare follows, in
// name order, and the request names it as the executor reads it.
func (m *message) sets(tb *catalog.Table, values map[string]json.RawMessage) []map[string]any {
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	index := func(n string) int {
		for i, col := range tb.Columns {
			if col.Name == n {
				return i
			}
		}
		return len(tb.Columns)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := index(names[i]), index(names[j])
		if a != b {
			return a < b
		}
		return names[i] < names[j]
	})
	out := make([]map[string]any, len(names))
	for i, n := range names {
		out[i] = m.assign(n, values[n])
	}
	return out
}

// byKey is the where of the row whose primary key is key.
func (m *message) byKey(tb *catalog.Table, key json.RawMessage) map[string]any {
	pred := map[string]any{"column": tb.Key, "op": "eq", "p": m.param(key)}
	return map[string]any{"where": map[string]any{"items": []any{map[string]any{"pred": pred}}}}
}

// update is the body of an update of the row whose key is key, assigning set.
func (m *message) update(tb *catalog.Table, key json.RawMessage, set []map[string]any) map[string]any {
	body := m.byKey(tb, key)
	body["set"] = set
	return body
}

// request returns the message of the request of kind on the table with body, at the engine clock (spec 40 The clock).
func (m *message) request(kind string, body map[string]any) []byte {
	req := map[string]any{}
	for k, v := range body {
		req[k] = v
	}
	params := m.params
	if params == nil {
		params = []json.RawMessage{}
	}
	req["ir_version"] = ir.Version
	req["manifest_hash"] = m.hash
	req["kind"] = kind
	req["entity"] = m.table
	req["n_params"] = len(params)
	now := time.Now().UTC().Truncate(time.Microsecond).Format("2006-01-02 15:04:05.000000")
	b, err := json.Marshal(map[string]any{"ir_version": ir.Version, "manifest_hash": m.hash, "request": req, "params": params, "now": now})
	if err != nil {
		// Every part is a map, a string, a number or a parameter that a face decoded as JSON.
		panic("ops: the request message does not marshal: " + err.Error())
	}
	m.params = nil
	return b
}

// TextValue returns the wire value of text, the readable text of a value of the column of table (spec 40 The faces):
// a JSON number for an integer or a float column when text is a JSON number, a JSON bool for a bool column when text is
// true or false, and a JSON string otherwise — so a text that is no value of its column reaches the executor, which
// names the fault.
func (c *Corpus) TextValue(table, column, text string) json.RawMessage {
	str, _ := json.Marshal(text)
	tb, ok := c.cat.Table(table)
	if !ok {
		return str
	}
	col, ok := tb.Column(column)
	if !ok {
		return str
	}
	switch col.Type.Kind {
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64, dbspec.TypeF64:
		var n json.Number
		d := json.NewDecoder(strings.NewReader(text))
		d.UseNumber()
		if text != "" && (text[0] == '-' || ('0' <= text[0] && text[0] <= '9')) && d.Decode(&n) == nil && !d.More() && n.String() == text {
			return json.RawMessage(text)
		}
	case dbspec.TypeBool:
		if text == "true" || text == "false" {
			return json.RawMessage(text)
		}
	}
	return str
}
