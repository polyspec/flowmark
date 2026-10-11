package irexec

import (
	"fmt"

	"github.com/polyspec/flowmark/store"
	"github.com/polyspec/flowmark/validate"
	"github.com/polyspec/orm/engine/dbspec"
)

// checks judges each check of the table over each written row with the validator's
// evaluator (spec 50 Checks), so the validator and the write path evaluate a check
// one way — for a write as it runs and for the commit's run of the writes again on
// the corpus re-read under the lock (spec 40 The commit protocol, step 2). A check
// false whatever the row's values not of their column type are is a fault naming
// the check; a check false only for some of those values needs them, so the write
// fails naming the type diagnostic of the first such value it reads; a check whose
// predicate is unknown (NULL) passes, as in SQL. A row the write places nowhere,
// whose key or placement key is faulted, has its checks judged too: a row's value
// faults are reported with its key fault (spec 10 Diagnostics, "Write faults").
func (w *writer) checks() *Error {
	if len(w.tb.Checks) == 0 {
		return nil
	}
	for _, g := range w.given {
		x := w.next.byRow[g.row]
		var parent *store.Row
		if x != nil && x.parent != nil {
			parent = x.parent.row
		}
		values := validate.RowOperands(w.tb, g.row, parent)
		for _, ck := range w.tb.Checks {
			switch validate.JudgeCheck(w.tb, ck.Expression, values) {
			case validate.CheckFails:
				w.fault(g, w.tb.Name, "", codeConstraint, fmt.Sprintf("the check %s of %s is false for %s", ck.Name, w.tb.Name, g.row.Key))
			case validate.CheckNeedsType:
				for _, col := range checkColumns(ck.Expression) {
					if _, raw := g.row.Raw[col]; raw && x != nil {
						return w.next.typeFault(x, col)
					}
				}
				return errf(codeInternal, "the check %s of %s needs a value not of its column type, but reads none", ck.Name, w.tb.Name)
			}
		}
	}
	return nil
}

// rowRules judges the codec and view rules over each written row with the
// validator's judgement (spec 50 Codec values, Views), so the validator and the
// commit judge a row one way. A codec fault is a fault of a value the write
// assigns, every value of an insert: a value the write leaves is no value it
// writes, as a value not of its type it leaves faults it only when a check reads
// it. A view fault is a fault of the whole row, a write that leaves a view invalid
// being refused. Each is CONSTRAINT at its column, its message naming its rule.
func (w *writer) rowRules() *Error {
	if len(w.tb.Codecs) == 0 && w.tb.View == nil {
		return nil
	}
	for _, g := range w.given {
		for _, f := range validate.JudgeRow(w.c.cat, w.r.e.model, w.tb, g.row) {
			if f.Rule == validate.RuleCodec && !g.inserted && !g.assigned[f.Column] {
				continue
			}
			w.fault(g, w.tb.Name, f.Column, codeConstraint, f.Rule+": "+f.Message)
		}
	}
	return nil
}

// checkColumns returns the columns a check predicate reads, in its order.
func checkColumns(e dbspec.Expr) []string {
	switch e := e.(type) {
	case dbspec.ColumnRef:
		return []string{e.Name}
	case dbspec.Binary:
		return append(checkColumns(e.Left), checkColumns(e.Right)...)
	case dbspec.In:
		out := checkColumns(e.Operand)
		for _, l := range e.List {
			out = append(out, checkColumns(l)...)
		}
		return out
	case dbspec.IsNull:
		return checkColumns(e.Operand)
	}
	return nil
}
