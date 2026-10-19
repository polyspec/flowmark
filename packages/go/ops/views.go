package ops

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/validate"
	"github.com/polyspec/flowmark/packages/go/writepath"
	"github.com/polyspec/orm/packages/go/engine/runtimemodel"
)

// viewAnswer is the answer of views with a key: the view row and the rows of its entity through its filter and sort.
type viewAnswer struct {
	View *irexec.Row   `json:"view"`
	Rows []*irexec.Row `json:"rows"`
}

// view runs the view of the view table tb whose key is key (spec 40 The faces, spec 100 Views) in one read-only
// transaction, so the view and its rows are read from one corpus: the view is judged as the validator judges it, and a
// faulty view is CONSTRAINT naming each fault; otherwise its entity's rows are read with the stored filter's where and
// params and the stored sort.
func (c *Corpus) view(ctx context.Context, s *writepath.Session, tb *catalog.Table, key json.RawMessage) (_ any, err error) {
	if err := s.Begin(ctx, true); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			if rerr := s.Rollback(); rerr != nil {
				err = rerr
			}
		}
	}()
	m := message{hash: c.cat.ManifestHash, table: tb.Name}
	a, err := c.exec(ctx, s, m.request("one", m.byKey(tb, key)))
	if err != nil {
		return nil, err
	}
	if len(a.Rows) == 0 {
		return nil, noRow(tb, key)
	}
	row := a.Rows[0]
	v := tb.View
	vals := map[string]string{}
	for _, col := range []string{v.Kind, v.Entity, v.Filter, v.Sort, v.Group} {
		if x, ok := row.Get(col); ok {
			if str, ok := x.(string); ok {
				vals[col] = str
			}
		}
	}
	model, diags := runtimemodel.Build(c.cat.Documents)
	if len(diags) > 0 {
		return nil, errf(codeSchemaInvalid, "%s", runtimemodel.DiagnosticsError(diags))
	}
	if faults := validate.JudgeView(c.cat, model, v, vals); len(faults) > 0 {
		msgs := make([]string, len(faults))
		for i, f := range faults {
			msgs[i] = f.Column + ": " + f.Message
		}
		return nil, errf("CONSTRAINT", "the view %s of %s is invalid (view): %s; repair the view row", key, tb.Name, strings.Join(msgs, "; "))
	}
	entity, ok := vals[v.Entity]
	if !ok {
		return nil, errf("CONSTRAINT", "the view %s of %s names no entity", key, tb.Name)
	}
	var filter struct {
		Where  json.RawMessage   `json:"where"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal([]byte(vals[v.Filter]), &filter); err != nil {
		return nil, errf(codeInternal, "the judged filter of the view %s does not decode: %v", key, err)
	}
	body := map[string]any{"where": filter.Where}
	if sort := vals[v.Sort]; sort != "" && sort != "[]" {
		body["order"] = json.RawMessage(sort)
	}
	rm := message{hash: c.cat.ManifestHash, table: entity, params: filter.Params}
	rows, err := c.exec(ctx, s, rm.request("all", body))
	if err != nil {
		return nil, err
	}
	if err := s.Commit(); err != nil {
		return nil, err
	}
	out := rows.Rows
	if out == nil {
		out = []*irexec.Row{}
	}
	return viewAnswer{View: row, Rows: out}, nil
}
