// Package sync is the sync engine (spec 70). Its first adapter is the database: the
// md→db export, a derived and deterministic set of statements that rebuilds the
// corpus in a database. The export holds no state of its own; it reads the corpus
// on every run.
package sync

import (
	"encoding/base64"
	"fmt"
	"slices"
	"strings"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/irexec"
	"github.com/polyspec/flowmark/validate"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/engine/dialect"
	"github.com/polyspec/orm/packages/go/engine/ir"
	"github.com/polyspec/orm/packages/go/engine/plan"
	"github.com/polyspec/orm/packages/go/engine/planner"
	"github.com/polyspec/orm/packages/go/engine/runtimemodel"
	"github.com/polyspec/orm/packages/go/orm"
)

// Statement is one statement of an export (spec 70 The export): its SQL text as
// orm renders it, the table it creates, changes or fills, and, for a row insert, its
// bind slots as orm's planner writes them with the request parameters they read, each
// in the runtime value model JSON (spec 60 The wire). A schema statement has no slot.
type Statement struct {
	Table  string
	SQL    string
	Slots  []plan.BindSlot
	Params []any
}

// Export renders the corpus c as the statements that rebuild it in a database of the
// dialect d (spec 70 The export): the schema statements orm's renderer writes for the
// catalog's document set, then one insert a row, compiled by orm's planner from the
// catalog's runtime model. The tables fill in the order of their foreign keys, a
// table after the tables it references, and each table's rows in storage order. An
// export that cannot be rendered whole fails and returns no statement: a table with
// a codec column is CAPABILITY_UNSUPPORTED, naming the table and what orm lacks; an
// unavailable table or a value not of its type is CONSTRAINT, naming
// the diagnostic. A table with an identity column loads its rows with their keys (orm's
// load insert) and is followed by orm's restart of the identity past its largest key.
func Export(c *irexec.Corpus, d dbspec.Dialect) ([]Statement, error) {
	cat := c.Catalog()
	model, diags := runtimemodel.Build(cat.Documents)
	if len(diags) > 0 {
		return nil, &irexec.Error{Code: codeInternal, Msg: "the catalog builds no runtime model: " + runtimemodel.DiagnosticsError(diags)}
	}
	entities := map[string]*runtimemodel.Entity{}
	for _, e := range model.Entities {
		entities[e.Table] = e
	}
	for _, tb := range cat.Tables {
		if err := supported(entities[tb.Name]); err != nil {
			return nil, err
		}
	}
	schema, diags := dbspec.RenderStatements(cat.Documents, d)
	if len(diags) > 0 {
		return nil, &irexec.Error{Code: codeInternal, Msg: "orm renders no statement for the catalog: " + runtimemodel.DiagnosticsError(diags)}
	}
	out := make([]Statement, 0, len(schema))
	for _, s := range schema {
		out = append(out, Statement{Table: s.Table, SQL: s.SQL})
	}
	p := &planner.Planner{M: model, D: dialectOf(d)}
	for _, tb := range fillOrder(cat, schema) {
		rows, err := c.Rows(tb.Name)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			s, err := insert(p, entities[tb.Name], tb, row)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		if e := entities[tb.Name]; e.Identity != "" {
			s, err := restartIdentity(p, e, tb)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
	}
	return out, nil
}

// The error codes of orm errors.yaml the export fails with.
const (
	codeCapability = "CAPABILITY_UNSUPPORTED"
	codeInternal   = "INTERNAL"
)

// supported fails as CAPABILITY_UNSUPPORTED when orm renders no insert that keeps the
// stored values of the table of e. A codec column's stored value is the encoded cell:
// the stages orm applies in the client (gz, base64, serialize, yaml, ordered_json) are
// done in it, so orm's insert binds it as it is; the stages orm applies on the host
// while it binds (hex, ip) must end the list, so that the export can undo them (see
// hostSplit) and orm applies them again.
func supported(e *runtimemodel.Entity) error {
	for _, f := range e.Fields {
		if _, _, ok := hostSplit(f.Codec); !ok {
			return &irexec.Error{Code: codeCapability, Msg: fmt.Sprintf("the table %s has the codec column %s with a host stage before a client stage: the engine can undo a host stage only at the end of the list", e.Table, f.Name)}
		}
	}
	return nil
}

// hostSplit splits the stages of a codec into those orm applies in the client and the
// host stages (hex, ip) at the end. ok is false when a host stage is followed by a
// client stage.
func hostSplit(stages []string) (client, host []string, ok bool) {
	n := len(stages)
	for n > 0 && slices.Contains(validate.HostStages, stages[n-1]) {
		n--
	}
	for _, st := range stages[:n] {
		if slices.Contains(validate.HostStages, st) {
			return nil, nil, false
		}
	}
	return stages[:n], stages[n:], true
}

// storedParam returns the parameter of the stored value v of a column of kind kind with
// the codec stages: v itself, or, for a column whose list ends in host stages, the cell
// without them, which orm's bind applies again (hex and ip need no key).
func storedParam(kind dbspec.TypeKind, stages []string, v any) (any, error) {
	_, host, _ := hostSplit(stages)
	if len(host) == 0 {
		return v, nil
	}
	text, ok := v.(string)
	if !ok {
		return nil, &irexec.Error{Code: codeInternal, Msg: fmt.Sprintf("a stored codec value is %T, not text", v)}
	}
	cell := []byte(text)
	if kind == dbspec.TypeBytes {
		b, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return nil, &irexec.Error{Code: codeInternal, Msg: "a stored bytes value is not base64: " + err.Error()}
		}
		cell = b
	}
	out, err := orm.HostDecode(cell, host, "")
	if err != nil {
		return nil, &irexec.Error{Code: codeInternal, Msg: "a stored value does not decode its host stages " + strings.Join(host, " ") + ": " + err.Error()}
	}
	decoded, ok := out.(string)
	if !ok {
		return nil, &irexec.Error{Code: codeInternal, Msg: fmt.Sprintf("orm decodes the host stages %s to a %T, not text", strings.Join(host, " "), out)}
	}
	if kind == dbspec.TypeBytes {
		return base64.StdEncoding.EncodeToString([]byte(decoded)), nil
	}
	return decoded, nil
}

// dialectOf returns orm's SQL dialect of the database d; d is one of dbspec's three
// dialects, as dbspec.RenderStatements requires.
func dialectOf(d dbspec.Dialect) dialect.Dialect {
	switch d {
	case dbspec.DialectMySQL:
		return dialect.MySQL{}
	case dbspec.DialectPostgres:
		return dialect.Postgres{}
	}
	return dialect.SQLite{}
}

// fillOrder returns the tables of cat in the order their rows are inserted: a table
// after every other table its foreign keys reference, ties in the order the schema
// statements create the tables; the tables of a cycle of references across tables
// stand in that creation order once no other table can come first.
func fillOrder(cat *catalog.Catalog, schema []dbspec.RenderedStatement) []*catalog.Table {
	created := map[string]int{}
	for i, s := range schema {
		if _, ok := created[s.Table]; !ok {
			created[s.Table] = i
		}
	}
	rest := slices.Clone(cat.Tables)
	slices.SortStableFunc(rest, func(a, b *catalog.Table) int { return created[a.Name] - created[b.Name] })
	done := map[string]bool{}
	var out []*catalog.Table
	for len(rest) > 0 {
		next := 0
		for i, tb := range rest {
			if slices.IndexFunc(tb.ForeignKeys, func(fk dbspec.ForeignKey) bool { return fk.Table != tb.Name && !done[fk.Table] }) < 0 {
				next = i
				break
			}
		}
		done[rest[next].Name] = true
		out = append(out, rest[next])
		rest = slices.Delete(rest, next, next+1)
	}
	return out
}

// insert compiles the insert of one row of tb, the values of its columns in
// declaration order, through orm's planner: every column is assigned, by its value or
// NULL, so the row keeps every stored value and no database default applies.
func insert(p *planner.Planner, e *runtimemodel.Entity, tb *catalog.Table, row []any) (Statement, error) {
	req := &ir.Request{IRVersion: ir.Version, ManifestHash: p.M.ManifestHash, Kind: "insert", Load: e.Identity != "", Query: ir.Query{Entity: e.Name}}
	var params []any
	for i, col := range tb.Columns {
		if row[i] == nil {
			req.Set = append(req.Set, ir.Assign{Column: col.Name, Null: true})
			continue
		}
		n := len(params)
		req.Set = append(req.Set, ir.Assign{Column: col.Name, P: &n})
		v, err := storedParam(col.Type.Kind, tb.Codecs[col.Name], row[i])
		if err != nil {
			return Statement{}, err
		}
		params = append(params, v)
	}
	req.NParams = len(params)
	pl, err := p.Compile(req)
	if err != nil {
		return Statement{}, &irexec.Error{Code: codeInternal, Msg: fmt.Sprintf("orm compiles no insert of a row of %s: %v", tb.Name, err)}
	}
	if len(pl.Steps) != 1 {
		return Statement{}, &irexec.Error{Code: codeInternal, Msg: fmt.Sprintf("orm compiles the insert of a row of %s in %d steps", tb.Name, len(pl.Steps))}
	}
	st := pl.Steps[0]
	return Statement{Table: tb.Name, SQL: st.SQL, Slots: st.BindSlots, Params: params}, nil
}

// restartIdentity compiles, through orm's planner, the statement that restarts the
// identity of tb past the largest key its loaded rows hold.
func restartIdentity(p *planner.Planner, e *runtimemodel.Entity, tb *catalog.Table) (Statement, error) {
	req := &ir.Request{IRVersion: ir.Version, ManifestHash: p.M.ManifestHash, Kind: "restart_identity", Query: ir.Query{Entity: e.Name}}
	pl, err := p.Compile(req)
	if err != nil {
		return Statement{}, &irexec.Error{Code: codeInternal, Msg: fmt.Sprintf("orm compiles no identity restart of %s: %v", tb.Name, err)}
	}
	if len(pl.Steps) != 1 {
		return Statement{}, &irexec.Error{Code: codeInternal, Msg: fmt.Sprintf("orm compiles the identity restart of %s in %d steps", tb.Name, len(pl.Steps))}
	}
	return Statement{Table: tb.Name, SQL: pl.Steps[0].SQL}, nil
}
