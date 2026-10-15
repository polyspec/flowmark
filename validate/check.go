package validate

import (
	"cmp"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/polyspec/orm/packages/go/engine/dbspec"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store"
)

// ruleCheck is the validator rule of a check that is false over a row.
const ruleCheck = "check"

// Truth is a value of SQL's three-valued logic. Its order is the order of Kleene
// logic, so and is the minimum and or the maximum.
type Truth int8

// The three truth values.
const (
	TruthFalse Truth = iota
	TruthUnknown
	TruthTrue
)

// Operand is the value a check reads from one column of a row: NULL, a value in
// its canonical readable form, or a raw value, one not of its column type, whose
// Text is the text the file writes.
type Operand struct {
	Text string
	Null bool
	Raw  bool
}

// RowOperands returns the operands a check reads from row r of table tb: every
// column as a read returns it, the key, the title, the body and the sections
// included, and the placement foreign key of a block row as the key of parent, its
// placement parent row (nil for a row of a files or document table). A column with
// no value is NULL.
func RowOperands(tb *catalog.Table, r *store.Row, parent *store.Row) func(column string) Operand {
	placement := ""
	if tb.Storage.Kind == catalog.StorageBlock {
		placement = placementOf(tb)
	}
	return func(col string) Operand {
		switch {
		case col == tb.Key:
			return Operand{Text: r.Key}
		case col == placement && parent != nil:
			return Operand{Text: parent.Key}
		case col == tb.Title:
			return Operand{Text: r.Title}
		case col == tb.Body:
			return Operand{Text: r.Body}
		}
		if s, ok := r.Raw[col]; ok {
			return Operand{Text: s, Raw: true}
		}
		if v, ok := r.Sections[col]; ok {
			if v == nil {
				return Operand{Null: true}
			}
			return Operand{Text: *v}
		}
		if v := r.Scalars[col]; v != nil {
			return Operand{Text: *v}
		}
		return Operand{Null: true}
	}
}

// EvalCheck evaluates the check predicate e of table tb over the operands values
// returns, in SQL's three-valued logic. An atom — a comparison or an in — with a
// NULL operand is unknown; otherwise an atom with a raw operand takes the truth
// value raw, since a raw value has no comparison; a raw value is not NULL for is
// [not] null. Text and uuid compare by code point, as orm's one binary collation
// does; integers, decimals and f64 values by value; dates, times and datetimes
// chronologically, which their canonical readable forms, of fixed width for one
// column type, order as text. A dbspec predicate has no not, so the result is
// monotone in the atoms: with raw TRUE it is the greatest result any values of the
// raw operands could give, and with raw FALSE the least.
func EvalCheck(tb *catalog.Table, e dbspec.Expr, values func(column string) Operand, raw Truth) Truth {
	switch e := e.(type) {
	case dbspec.Binary:
		switch e.Op {
		case "and":
			return min(EvalCheck(tb, e.Left, values, raw), EvalCheck(tb, e.Right, values, raw))
		case "or":
			return max(EvalCheck(tb, e.Left, values, raw), EvalCheck(tb, e.Right, values, raw))
		}
		return comparison(tb, e, values, raw)
	case dbspec.IsNull:
		null := values(e.Operand.(dbspec.ColumnRef).Name).Null
		return truth(null != e.Negated)
	case dbspec.In:
		col := e.Operand.(dbspec.ColumnRef).Name
		o := values(col)
		switch {
		case o.Null:
			return TruthUnknown
		case o.Raw:
			return raw
		}
		kind := columnKind(tb, col)
		found := false
		for _, l := range e.List {
			c, ok := compare(kind, o.Text, literalText(l))
			if !ok {
				return raw
			}
			if c == 0 {
				found = true
				break
			}
		}
		return truth(found != e.Negated)
	}
	panic(fmt.Sprintf("validate: a check of table %s holds the expression %T, which is not a predicate", tb.Name, e))
}

// comparison evaluates `<left> <op> <right>`, at least one side being a column of
// tb; the type of a column side decides how the two compare, since dbspec admits
// only operands that meet.
func comparison(tb *catalog.Table, e dbspec.Binary, values func(string) Operand, raw Truth) Truth {
	var kind dbspec.TypeKind
	operand := func(x dbspec.Expr) Operand {
		switch x := x.(type) {
		case dbspec.ColumnRef:
			if kind == "" {
				kind = columnKind(tb, x.Name)
			}
			return values(x.Name)
		case dbspec.Literal:
			return Operand{Text: literalText(x)}
		}
		panic(fmt.Sprintf("validate: a check of table %s compares %T, which is not an operand", tb.Name, x))
	}
	left, right := operand(e.Left), operand(e.Right)
	switch {
	case left.Null || right.Null:
		return TruthUnknown
	case left.Raw || right.Raw:
		return raw
	}
	c, ok := compare(kind, left.Text, right.Text)
	if !ok {
		return raw
	}
	switch e.Op {
	case "=":
		return truth(c == 0)
	case "<>":
		return truth(c != 0)
	case "<":
		return truth(c < 0)
	case "<=":
		return truth(c <= 0)
	case ">":
		return truth(c > 0)
	case ">=":
		return truth(c >= 0)
	}
	panic(fmt.Sprintf("validate: a check of table %s holds the operator %s", tb.Name, e.Op))
}

// compare orders two values of a column of kind, each in its canonical readable
// form. A text that does not read as kind is reported as not ok: it is no value
// of the type, so its atom is judged as a raw value's is.
func compare(kind dbspec.TypeKind, a, b string) (int, bool) {
	switch kind {
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64:
		x, errA := strconv.ParseInt(a, 10, 64)
		y, errB := strconv.ParseInt(b, 10, 64)
		return cmp.Compare(x, y), errA == nil && errB == nil
	case dbspec.TypeDecimal:
		x, okA := new(big.Rat).SetString(a)
		y, okB := new(big.Rat).SetString(b)
		if !okA || !okB {
			return 0, false
		}
		return x.Cmp(y), true
	case dbspec.TypeF64:
		x, errA := strconv.ParseFloat(a, 64)
		y, errB := strconv.ParseFloat(b, 64)
		return cmp.Compare(x, y), errA == nil && errB == nil
	}
	return strings.Compare(a, b), true
}

// literalText returns the value a canonical literal writes: a quoted literal
// without its quotes and with each doubled quote single, any other as it is.
func literalText(l dbspec.Literal) string {
	t := l.Text
	if len(t) >= 2 && t[0] == '\'' && t[len(t)-1] == '\'' {
		return strings.ReplaceAll(t[1:len(t)-1], "''", "'")
	}
	return t
}

// columnKind returns the type kind of column col of table tb.
func columnKind(tb *catalog.Table, col string) dbspec.TypeKind {
	for _, c := range tb.Columns {
		if c.Name == col {
			return c.Type.Kind
		}
	}
	panic(fmt.Sprintf("validate: a check of table %s names the column %s, which the table does not declare", tb.Name, col))
}

func truth(b bool) Truth {
	if b {
		return TruthTrue
	}
	return TruthFalse
}

// CheckVerdict is the commit's judgement of one check over a row it writes.
type CheckVerdict int

// The verdicts of JudgeCheck.
const (
	// CheckPasses: the check is not false whatever the raw values are.
	CheckPasses CheckVerdict = iota
	// CheckFails: the check is false whatever the raw values are, so the commit is
	// CONSTRAINT naming the check.
	CheckFails
	// CheckNeedsType: the check is false for some values of its raw operands and
	// not for others, so the commit is CONSTRAINT naming the type diagnostic of the
	// raw values it reads.
	CheckNeedsType
)

// JudgeCheck judges the check predicate e of table tb over the operands values
// returns, for a commit: the predicate is evaluated with its raw atoms TRUE and
// then FALSE (spec 50, Checks). It fails when it is false with them TRUE, needs the
// type of a raw value when it is false only with them FALSE, and passes otherwise.
// The validator reports check exactly when the verdict is CheckFails.
func JudgeCheck(tb *catalog.Table, e dbspec.Expr, values func(column string) Operand) CheckVerdict {
	switch {
	case EvalCheck(tb, e, values, TruthTrue) == TruthFalse:
		return CheckFails
	case EvalCheck(tb, e, values, TruthFalse) == TruthFalse:
		return CheckNeedsType
	}
	return CheckPasses
}

// checks reports each check of tb that is false over the row p, whatever its raw
// values are, at the row's first line, naming the check and the row. A check needs
// only its own row, so it is reported in an unavailable table too.
func (c *corpus) checks(tb *catalog.Table, p placed) {
	if len(tb.Checks) == 0 {
		return
	}
	values := RowOperands(tb, p.row, p.parent)
	for _, k := range tb.Checks {
		if JudgeCheck(tb, k.Expression, values) == CheckFails {
			c.report(p, "", ruleCheck, fmt.Sprintf("the check %s is false for %s", k.Name, p.row.Key))
		}
	}
}
