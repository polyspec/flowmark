package irexec

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/engine/dialect"
	"github.com/polyspec/orm/packages/go/engine/ir"
	"github.com/polyspec/orm/packages/go/engine/runtimemodel"
)

// tri is a truth value of SQL's three-valued logic: a comparison with NULL is
// unknown, and a row matches a condition only when it is true (SQL parity, as
// spec 50 rules a check whose predicate is unknown).
type tri int8

const (
	tFalse tri = iota
	tUnknown
	tTrue
)

func and3(a, b tri) tri {
	switch {
	case a == tFalse || b == tFalse:
		return tFalse
	case a == tUnknown || b == tUnknown:
		return tUnknown
	}
	return tTrue
}

func or3(a, b tri) tri {
	switch {
	case a == tTrue || b == tTrue:
		return tTrue
	case a == tUnknown || b == tUnknown:
		return tUnknown
	}
	return tFalse
}

func not3(a tri) tri {
	switch a {
	case tTrue:
		return tFalse
	case tFalse:
		return tTrue
	}
	return tUnknown
}

func bool3(b bool) tri {
	if b {
		return tTrue
	}
	return tFalse
}

// scope is the query of a statement at its join path: "" for the statement's
// root, "a/b" for the join b of the join a.
type scope struct {
	q     *ir.Query
	ent   *runtimemodel.Entity
	tb    *catalog.Table
	path  string
	joins []*joined
	byRel map[string]*scope
	// extra are the columns a relation step binds or keys on, always selected.
	extra []string
}

// joined is a join of a scope with the scope of its child.
type joined struct {
	def *ir.Join
	sc  *scope
}

// env binds each join path of a statement to a row, nil for a left join that
// matched none, and holds the statement's root scope and the query that owns a
// subquery, for its "^".
type env struct {
	t     map[string]*rec
	root  *scope
	outer *frame
}

// frame is the scope and the env of the query that owns a subquery.
type frame struct {
	sc *scope
	ev *env
}

// with returns a copy of ev that binds path to x.
func (ev *env) with(path string, x *rec) *env {
	t := make(map[string]*rec, len(ev.t)+1)
	for k, v := range ev.t {
		t[k] = v
	}
	t[path] = x
	return &env{t: t, root: ev.root, outer: ev.outer}
}

// buildScope returns the scope of q at path and of its joins.
func (r *reader) buildScope(q *ir.Query, path string) *scope {
	ent := r.e.model.Entities[q.Entity]
	tb, _ := r.c.cat.Table(ent.Table)
	s := &scope{q: q, ent: ent, tb: tb, path: path, byRel: map[string]*scope{}}
	for _, j := range q.Joins {
		js := r.buildScope(j.Query, joinPath(path, j.Rel))
		s.joins = append(s.joins, &joined{def: j, sc: js})
		s.byRel[j.Rel] = js
	}
	return s
}

func joinPath(path, rel string) string {
	if path == "" {
		return rel
	}
	return path + "/" + rel
}

// col returns the value and the type of column name of the row ev binds to s.
func (r *reader) col(s *scope, ev *env, name string) (cell, dbspec.Type, *Error) {
	f := s.ent.Field(name)
	if f == nil {
		return cell{}, dbspec.Type{}, errf(codeColumnUnknown, "%s.%s", s.ent.Name, name)
	}
	t := fieldType(f)
	x := ev.t[s.path]
	if x == nil {
		return cell{null: true}, t, nil
	}
	v, err := r.c.value(x, name)
	return v, t, err
}

// param reads parameter i as a value of type t for a comparison.
func (r *reader) param(i int, t dbspec.Type) (cell, *Error) {
	v, ok := decodeParam(r.cl.params[i], t)
	if !ok {
		// An orm message's parameters are checked before any row is read; a face operation's are not, so a view
		// filter's parameter outside its slot is refused here, as a bound value outside its slot is.
		return cell{}, errf(codeCodecEncode, "parameter %d is not a value of %s", i, t.String())
	}
	return v, nil
}

// cmpCells compares two values: unknown when either is NULL. Two values whose types
// read as different kinds do not meet, which orm's validator refuses before the
// executor reads a row (spec 60 Surfaces, column comparisons).
func cmpCells(at dbspec.Type, a cell, bt dbspec.Type, b cell) (int, bool, *Error) {
	if a.null || b.null {
		return 0, true, nil
	}
	sa, ok := scalarOf(at, a.text)
	if !ok {
		return 0, false, errf(codeInternal, "%q is not a readable %s", a.text, at.String())
	}
	sb, ok := scalarOf(bt, b.text)
	if !ok {
		return 0, false, errf(codeInternal, "%q is not a readable %s", b.text, bt.String())
	}
	if sa.kind != sb.kind {
		return 0, false, errf(codeInternal, "a %s value compared with a %s value, which ir.Validate refuses", at.String(), bt.String())
	}
	return compare(sa, sb), false, nil
}

// opHolds reports whether the comparison op holds for the order c of two values.
func opHolds(op string, c int) bool {
	switch strings.TrimSuffix(op, "_col") {
	case "eq":
		return c == 0
	case "not_eq":
		return c != 0
	case "gt":
		return c > 0
	case "gte":
		return c >= 0
	case "lt":
		return c < 0
	case "lte":
		return c <= 0
	}
	return false
}

// cmp3 is the truth of a op b.
func cmp3(op string, at dbspec.Type, a cell, bt dbspec.Type, b cell) (tri, *Error) {
	c, unknown, err := cmpCells(at, a, bt, b)
	if err != nil {
		return tFalse, err
	}
	if unknown {
		return tUnknown, nil
	}
	return bool3(opHolds(op, c)), nil
}

// group evaluates a where or on group of s. Items join their previous item by
// their connector, and AND binds tighter than OR, as the SQL the protocol renders
// reads them. Every item is evaluated, so whether a request fails on a value it
// cannot read does not depend on the order of its items.
func (r *reader) group(s *scope, ev *env, g *ir.Group) (tri, *Error) {
	if g == nil || len(g.Items) == 0 {
		return tTrue, nil
	}
	orAcc, andAcc := tFalse, tTrue
	for i, it := range g.Items {
		v, conn, err := r.item(s, ev, &it)
		if err != nil {
			return tFalse, err
		}
		switch {
		case i == 0:
			andAcc = v
		case conn == "or":
			orAcc = or3(orAcc, andAcc)
			andAcc = v
		default:
			andAcc = and3(andAcc, v)
		}
	}
	res := or3(orAcc, andAcc)
	if g.Not {
		res = not3(res)
	}
	return res, nil
}

// item evaluates one item of a group and returns its connector.
func (r *reader) item(s *scope, ev *env, it *ir.Item) (tri, string, *Error) {
	switch {
	case it.Joined != nil:
		js := s.byRel[it.Joined.Join]
		if js == nil {
			return tFalse, "", errf(codeNotJoined, "%s is not joined in this statement", it.Joined.Join)
		}
		v, err := r.group(js, ev, js.q.Where)
		return v, it.Joined.Conn, err
	case it.Group != nil:
		v, err := r.group(s, ev, it.Group)
		return v, it.Group.Conn, err
	case it.Pred != nil:
		v, err := r.pred(s, ev, it.Pred)
		return v, it.Pred.Conn, err
	}
	return tFalse, "", errf(codeIRInvalid, "where item must be exactly one of pred/group/joined")
}

// pred evaluates one predicate of s.
func (r *reader) pred(s *scope, ev *env, p *ir.Pred) (tri, *Error) {
	switch p.Op {
	case "contains", "contains_binary":
		return r.contains(s, ev, p)
	case "tuple_in", "tuple_not_in":
		return r.tupleIn(s, ev, p)
	}
	if p.Sub != nil {
		return r.inSub(s, ev, p)
	}
	v, t, err := r.col(s, ev, p.Column)
	if err != nil {
		return tFalse, err
	}
	if p.Fn != nil {
		fv, ft, err := columnFunction(p.Fn.Name, t, v)
		if err != nil {
			return tFalse, err
		}
		return r.compareParams(p, ft, fv)
	}
	if p.Value != nil {
		vv, vt, err := r.valueFunction(p.Value)
		if err != nil {
			return tFalse, err
		}
		return cmp3(p.Op, t, v, vt, vv)
	}
	switch p.Op {
	case "is_null":
		return bool3(v.null), nil
	case "is_not_null":
		return bool3(!v.null), nil
	case "eq_col", "not_eq_col", "gt_col", "gte_col", "lt_col", "lte_col":
		rs, rev, err := r.resolveRef(s, ev, p.Ref.Path)
		if err != nil {
			return tFalse, err
		}
		rv, rt, err := r.col(rs, rev, p.Ref.Column)
		if err != nil {
			return tFalse, err
		}
		return cmp3(p.Op, t, v, rt, rv)
	}
	return r.compareParams(p, t, v)
}

// contains evaluates contains (the Lk operator) and contains_binary (the Lb
// operator): true when the column's text holds the parameter's text as a substring
// (spec 60 Surfaces). contains compares the ASCII letters A-Z and a-z without regard
// to case and every other character by code point, as PostgreSQL ILIKE under
// COLLATE "C", SQLite LIKE and orm's MySQL ASCII fold do; contains_binary compares
// every character by code point. A NULL column or parameter is unknown, and %, _ and
// \ in the parameter are literal. On valid UTF-8 a byte match is a code point match,
// and no byte of a multi-byte sequence lies in A-Z, so folding those bytes alone
// folds the ASCII letters alone.
func (r *reader) contains(s *scope, ev *env, p *ir.Pred) (tri, *Error) {
	v, _, err := r.col(s, ev, p.Column)
	if err != nil {
		return tFalse, err
	}
	pv, err := r.param(*p.P, dbspec.Type{Kind: dbspec.TypeText})
	if err != nil {
		return tFalse, err
	}
	if v.null || pv.null {
		return tUnknown, nil
	}
	if p.Op == "contains" {
		return bool3(strings.Contains(foldASCII(v.text), foldASCII(pv.text))), nil
	}
	return bool3(strings.Contains(v.text, pv.text)), nil
}

// foldASCII lower-cases the bytes A-Z of s and keeps every other byte.
func foldASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// compareParams evaluates a comparison, in, not_in or between of the value v of
// type t with the predicate's parameters.
func (r *reader) compareParams(p *ir.Pred, t dbspec.Type, v cell) (tri, *Error) {
	switch p.Op {
	case "in", "not_in":
		res := tFalse
		for _, i := range p.Ps {
			pv, err := r.param(i, t)
			if err != nil {
				return tFalse, err
			}
			e, err := cmp3("eq", t, v, t, pv)
			if err != nil {
				return tFalse, err
			}
			res = or3(res, e)
		}
		if p.Op == "not_in" {
			res = not3(res)
		}
		return res, nil
	case "between":
		lo, err := r.param(p.Ps[0], t)
		if err != nil {
			return tFalse, err
		}
		hi, err := r.param(p.Ps[1], t)
		if err != nil {
			return tFalse, err
		}
		a, err := cmp3("gte", t, v, t, lo)
		if err != nil {
			return tFalse, err
		}
		b, err := cmp3("lte", t, v, t, hi)
		if err != nil {
			return tFalse, err
		}
		return and3(a, b), nil
	case "eq", "not_eq", "gt", "gte", "lt", "lte":
		pv, err := r.param(*p.P, t)
		if err != nil {
			return tFalse, err
		}
		return cmp3(p.Op, t, v, t, pv)
	}
	return tFalse, errf(codeIRInvalid, "operator %q", p.Op)
}

// tupleIn evaluates tuple_in and tuple_not_in: the columns as one tuple compared
// with each row of parameters.
func (r *reader) tupleIn(s *scope, ev *env, p *ir.Pred) (tri, *Error) {
	vals := make([]cell, len(p.Cols))
	types := make([]dbspec.Type, len(p.Cols))
	for i, c := range p.Cols {
		v, t, err := r.col(s, ev, c)
		if err != nil {
			return tFalse, err
		}
		vals[i], types[i] = v, t
	}
	res := tFalse
	for i := 0; i+len(p.Cols) <= len(p.Ps); i += len(p.Cols) {
		row := tTrue
		for k := range p.Cols {
			pv, err := r.param(p.Ps[i+k], types[k])
			if err != nil {
				return tFalse, err
			}
			e, err := cmp3("eq", types[k], vals[k], types[k], pv)
			if err != nil {
				return tFalse, err
			}
			row = and3(row, e)
		}
		res = or3(res, row)
	}
	if p.Op == "tuple_not_in" {
		res = not3(res)
	}
	return res, nil
}

// inSub evaluates in and not_in over the values of a subquery's column.
func (r *reader) inSub(s *scope, ev *env, p *ir.Pred) (tri, *Error) {
	v, t, err := r.col(s, ev, p.Column)
	if err != nil {
		return tFalse, err
	}
	vals, st, err := r.subColumn(p.Sub, &frame{sc: s, ev: ev})
	if err != nil {
		return tFalse, err
	}
	res := tFalse
	for _, sv := range vals {
		e, err := cmp3("eq", t, v, st, sv)
		if err != nil {
			return tFalse, err
		}
		res = or3(res, e)
	}
	if p.Op == "not_in" {
		res = not3(res)
	}
	return res, nil
}

// resolveRef returns the scope and the env a column reference's path names: "" the
// statement's root, "a/b" a join of it, "^" the query that owns the subquery.
func (r *reader) resolveRef(s *scope, ev *env, path string) (*scope, *env, *Error) {
	if path == "^" {
		if ev.outer == nil {
			return nil, nil, errf(codeIRInvalid, "^ reference outside a subquery")
		}
		return ev.outer.sc, ev.outer.ev, nil
	}
	if path == "" {
		return ev.root, ev, nil
	}
	cur := ev.root
	for _, seg := range strings.Split(path, "/") {
		next, ok := cur.byRel[seg]
		if !ok {
			return nil, nil, errf(codeNotJoined, "%s.%s", cur.ent.Name, seg)
		}
		cur = next
	}
	return cur, ev, nil
}

// columnFunction applies an ORM column function to a date or datetime value:
// year, month and day_of_week (1 for Sunday to 7 for Saturday, as every dialect
// renders it) give an i64, and date the date of the value.
func columnFunction(name string, t dbspec.Type, v cell) (cell, dbspec.Type, *Error) {
	i64 := dbspec.Type{Kind: dbspec.TypeI64}
	date := dbspec.Type{Kind: dbspec.TypeDate}
	out := i64
	if name == "date" {
		out = date
	}
	if v.null {
		return cell{null: true}, out, nil
	}
	sv, ok := scalarOf(t, v.text)
	if !ok || sv.kind != kindTime {
		return cell{}, out, errf(codeOpNotAllowed, "%s on a %s value", name, t.String())
	}
	tm := sv.tm
	switch name {
	case "year":
		return cell{text: strconv.Itoa(tm.Year())}, out, nil
	case "month":
		return cell{text: strconv.Itoa(int(tm.Month()))}, out, nil
	case "day_of_week":
		return cell{text: strconv.Itoa(int(tm.Weekday()) + 1)}, out, nil
	case "date":
		return cell{text: tm.Format("2006-01-02")}, out, nil
	}
	return cell{}, out, errf("FUNCTION_UNKNOWN", "%s", name)
}

// valueFunction returns the value of an ORM value function at the request's
// clock: now, today, and the relative forms that move the clock by a parameter
// of their unit (seconds as an f64, the others as an i32). A month moves to the
// same day of the target month, or to its last day when it has fewer days, as
// MySQL's DATE_SUB and SQLite's floor modifier do. The clock keeps six digits when
// compared (orm protocol section 2, Clock; spec 40 The clock), as MySQL and
// PostgreSQL compare a datetime(p) column with NOW(6).
func (r *reader) valueFunction(f *ir.Func) (cell, dbspec.Type, *Error) {
	now := r.cl.now
	dt := dbspec.Type{Kind: dbspec.TypeDatetime, Precision: 6}
	switch f.Name {
	case "now":
		return cell{text: formatDatetime(now, 6)}, dt, nil
	case "today":
		return cell{text: now.Format("2006-01-02")}, dbspec.Type{Kind: dbspec.TypeDate}, nil
	}
	unit, ok := dialect.ValueFunctionUnits[f.Name]
	if !ok {
		return cell{}, dt, errf("FUNCTION_UNKNOWN", "%s", f.Name)
	}
	sign := 1
	if strings.HasSuffix(f.Name, "_ago") {
		sign = -1
	}
	argType := dbspec.Type{Kind: dbspec.TypeI32}
	if unit == "second" {
		argType = dbspec.Type{Kind: dbspec.TypeF64}
	}
	pv, err := r.param(f.Ps[0], argType)
	if err != nil {
		return cell{}, dt, err
	}
	if pv.null {
		return cell{null: true}, dt, nil
	}
	// A result outside the datetime range, or an interval past every result in it, is
	// NULL (spec 60 Surfaces): the bounds below lie beyond the span of the range, so
	// no in-range result is lost and no arithmetic overflows.
	var at time.Time
	switch unit {
	case "second", "minute", "hour":
		x, _ := strconv.ParseFloat(pv.text, 64)
		x *= float64(sign) * map[string]float64{"second": 1, "minute": 60, "hour": 3600}[unit]
		if math.Abs(x) > 1e13 {
			return cell{null: true}, dt, nil
		}
		whole := math.Trunc(x)
		shifted := now.Add(time.Duration(math.Round((x - whole) * float64(time.Second))))
		at = time.Unix(shifted.Unix()+int64(whole), int64(shifted.Nanosecond())).UTC()
	default:
		n, _ := strconv.Atoi(pv.text)
		n *= sign
		switch unit {
		case "day":
			if n > 3652059 || n < -3652059 {
				return cell{null: true}, dt, nil
			}
			at = now.AddDate(0, 0, n)
		case "month":
			if n > 119987 || n < -119987 {
				return cell{null: true}, dt, nil
			}
			at = addMonths(now, n)
		}
	}
	if at.Before(datetimeMin) || at.After(datetimeMax) {
		return cell{null: true}, dt, nil
	}
	return cell{text: formatDatetime(at, 9)}, dbspec.Type{Kind: dbspec.TypeDatetime, Precision: 9}, nil
}

// datetimeMin and datetimeMax bound orm's datetime range (orm dialects.md).
var (
	datetimeMin = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	datetimeMax = time.Date(9999, 12, 31, 23, 59, 59, 999999000, time.UTC)
)

// addMonths moves tm by n months, to the last day of the target month when the
// day does not exist there.
func addMonths(tm time.Time, n int) time.Time {
	y, m, d := tm.Date()
	first := time.Date(y, m, 1, tm.Hour(), tm.Minute(), tm.Second(), tm.Nanosecond(), time.UTC).AddDate(0, n, 0)
	last := first.AddDate(0, 1, -1).Day()
	if d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, tm.Hour(), tm.Minute(), tm.Second(), tm.Nanosecond(), time.UTC)
}
