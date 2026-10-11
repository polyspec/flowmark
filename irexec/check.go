package irexec

import (
	"fmt"
	"strings"

	"github.com/polyspec/flowmark/store"
	"github.com/polyspec/orm/engine/dbspec"
)

// checks refuses a written row that a check of its table makes false; a check
// whose predicate is unknown (NULL) passes, as in SQL (spec 50 Checks). A value of
// the row not of its type, which the check evaluates, fails the write.
func (w *writer) checks() *Error {
	var checks []dbspec.Check
	for _, d := range w.c.cat.Documents {
		for _, t := range d.Tables {
			if t.Name == w.tb.Name {
				checks = t.Checks
			}
		}
	}
	if len(checks) == 0 {
		return nil
	}
	for _, g := range w.given {
		x := w.next.byRow[g.row]
		if x == nil {
			continue
		}
		for _, ck := range checks {
			v, err := w.evalCheck(x, ck.Expression)
			if err != nil {
				return err
			}
			if v == tFalse {
				w.fault(g, w.tb.Name, "", codeConstraint, fmt.Sprintf("the check %s of %s is false for %s", ck.Name, w.tb.Name, g.row.Key))
			}
		}
	}
	return nil
}

// evalCheck evaluates a check expression over row x.
func (w *writer) evalCheck(x *rec, e dbspec.Expr) (tri, *Error) {
	switch e := e.(type) {
	case dbspec.Binary:
		switch e.Op {
		case "and", "or":
			a, err := w.evalCheck(x, e.Left)
			if err != nil {
				return tFalse, err
			}
			b, err := w.evalCheck(x, e.Right)
			if err != nil {
				return tFalse, err
			}
			if e.Op == "and" {
				return and3(a, b), nil
			}
			return or3(a, b), nil
		}
		lt, lv, err := w.operand(x, e.Left, e.Right)
		if err != nil {
			return tFalse, err
		}
		rt, rv, err := w.operand(x, e.Right, e.Left)
		if err != nil {
			return tFalse, err
		}
		op := map[string]string{"=": "eq", "<>": "not_eq", "<": "lt", "<=": "lte", ">": "gt", ">=": "gte"}[e.Op]
		return cmp3(op, lt, lv, rt, rv)
	case dbspec.In:
		ot, ov, err := w.operand(x, e.Operand, nil)
		if err != nil {
			return tFalse, err
		}
		res := tFalse
		for _, l := range e.List {
			lt, lv, err := w.operand(x, l, e.Operand)
			if err != nil {
				return tFalse, err
			}
			eq, err := cmp3("eq", ot, ov, lt, lv)
			if err != nil {
				return tFalse, err
			}
			res = or3(res, eq)
		}
		if e.Negated {
			res = not3(res)
		}
		return res, nil
	case dbspec.IsNull:
		_, v, err := w.operand(x, e.Operand, nil)
		if err != nil {
			return tFalse, err
		}
		return bool3(v.null != e.Negated), nil
	}
	return tFalse, errf(codeInternal, "an unknown check expression %T", e)
}

// operand returns the type and the value of a check operand: a column of row x, or a
// literal in the canonical form of the column it meets (other).
func (w *writer) operand(x *rec, e, other dbspec.Expr) (dbspec.Type, cell, *Error) {
	switch e := e.(type) {
	case dbspec.ColumnRef:
		v, err := w.next.value(x, e.Name)
		return columnOf(x.tb, e.Name).Type, v, err
	case dbspec.Literal:
		ref, ok := other.(dbspec.ColumnRef)
		if !ok {
			return dbspec.Type{}, cell{}, errf(codeInternal, "a check literal %s meets no column", e.Text)
		}
		t := columnOf(x.tb, ref.Name).Type
		text := e.Text
		if strings.HasPrefix(text, "'") && strings.HasSuffix(text, "'") && len(text) >= 2 {
			text = strings.ReplaceAll(text[1:len(text)-1], "''", "'")
		}
		v, ok := store.Readable(t, text)
		if !ok {
			return t, cell{}, errf(codeInternal, "the check literal %s is not a %s", e.Text, t.String())
		}
		return t, cell{text: v}, nil
	}
	return dbspec.Type{}, cell{}, errf(codeInternal, "an unknown check operand %T", e)
}
