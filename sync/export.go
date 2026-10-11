// Package sync is the sync engine (spec 70). Its first adapter is the database: the
// md→db export, a derived and deterministic set of statements that rebuilds the
// corpus in a database. The export holds no state of its own; it reads the corpus
// on every run.
package sync

import (
	"fmt"
	"slices"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/irexec"
	"github.com/polyspec/orm/engine/dbspec"
	"github.com/polyspec/orm/engine/dialect"
	"github.com/polyspec/orm/engine/ir"
	"github.com/polyspec/orm/engine/plan"
	"github.com/polyspec/orm/engine/planner"
	"github.com/polyspec/orm/engine/runtimemodel"
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
// an identity or a codec column is CAPABILITY_UNSUPPORTED, naming the table and what
// orm lacks; an unavailable table or a value not of its type is CONSTRAINT, naming
// the diagnostic.
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
	}
	return out, nil
}

// The error codes of orm errors.yaml the export fails with.
const (
	codeCapability = "CAPABILITY_UNSUPPORTED"
	codeInternal   = "INTERNAL"
)

// supported fails as CAPABILITY_UNSUPPORTED when orm renders no insert that keeps
// the stored values of the table of e. orm's insert rule refuses an assignment of an
// identity column, which the executor assigns, and orm renders no statement that
// moves a PostgreSQL identity past the keys an export inserts; a codec column's
// stored value is the encoded one, while orm's insert takes the decoded value and
// encodes it, and the engine decodes no value yet (spec 60 Surfaces, codec columns).
func supported(e *runtimemodel.Entity) error {
	if e.Identity != "" {
		return &irexec.Error{Code: codeCapability, Msg: fmt.Sprintf("the table %s has the identity column %s: orm's insert refuses an identity value and renders no identity restart, so the export cannot keep its keys", e.Table, e.Identity)}
	}
	for _, f := range e.Fields {
		if len(f.Codec) > 0 {
			return &irexec.Error{Code: codeCapability, Msg: fmt.Sprintf("the table %s has the codec column %s: the engine decodes no stored codec value for orm's insert", e.Table, f.Name)}
		}
	}
	return nil
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
	req := &ir.Request{IRVersion: ir.Version, ManifestHash: p.M.ManifestHash, Kind: "insert", Query: ir.Query{Entity: e.Name}}
	var params []any
	for i, col := range tb.Columns {
		if row[i] == nil {
			req.Set = append(req.Set, ir.Assign{Column: col.Name, Null: true})
			continue
		}
		n := len(params)
		req.Set = append(req.Set, ir.Assign{Column: col.Name, P: &n})
		params = append(params, row[i])
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
