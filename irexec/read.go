package irexec

import (
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/polyspec/orm/engine/dbspec"
	"github.com/polyspec/orm/engine/ir"
)

// reader runs the read side of one request over one corpus.
type reader struct {
	e  *Executor
	c  *Corpus
	cl *call
}

// read answers a read request: one, all, count, group_count, sum, avg or paginate.
func (e *Executor) read(c *Corpus, cl *call) (*Answer, *Error) {
	r := &reader{e: e, c: c, cl: cl}
	q := &cl.req.Query
	if err := r.prepare(q); err != nil {
		return nil, err
	}
	a := &Answer{Kind: cl.req.Kind}
	switch cl.req.Kind {
	case "one", "all":
		rows, err := r.selectRows(q, cl.req.Kind == "one")
		if err != nil {
			return nil, err
		}
		a.Rows = rows
	case "paginate":
		rows, err := r.selectRows(q, false)
		if err != nil {
			return nil, err
		}
		n, err := r.count(q)
		if err != nil {
			return nil, err
		}
		a.Rows, a.Count = rows, n
	case "count":
		n, err := r.count(q)
		if err != nil {
			return nil, err
		}
		a.Count = n
	case "sum", "avg":
		v, err := r.aggregate(q, cl.req.Kind, cl.req.Agg)
		if err != nil {
			return nil, err
		}
		a.Value = v
	case "group_count":
		rows, err := r.groupCount(q)
		if err != nil {
			return nil, err
		}
		a.Rows = rows
	default:
		return nil, errf(codeIRInvalid, "unknown kind %q", cl.req.Kind)
	}
	return a, nil
}

// prepare refuses the surfaces the executor does not run, and the request when a
// table it needs is unavailable. Every table the request names — its root, joins,
// relations and subqueries — is needed, in name order, so the refusal does not
// depend on the corpus's rows.
func (r *reader) prepare(q *ir.Query) *Error {
	var err *Error
	tables := map[string]bool{}
	walkQueries(q, func(x *ir.Query) {
		if err != nil {
			return
		}
		switch {
		case x.ForceIdx != "":
			err = errf(codeCapability, "force_index: indexes are derived, not named, on a corpus")
		case x.Lock == "update_nowait" || x.Lock == "share_nowait":
			err = errf(codeCapability, "lock %s is not supported on a corpus", x.Lock)
		}
		tables[r.e.model.Entities[x.Entity].Table] = true
	})
	if err != nil {
		return err
	}
	names := make([]string, 0, len(tables))
	for t := range tables {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		if err := r.c.need(t); err != nil {
			return err
		}
	}
	return nil
}

// walkQueries calls f on q and on every query in it: joins, relations, and the
// subqueries of its groups and columns.
func walkQueries(q *ir.Query, f func(*ir.Query)) {
	if q == nil {
		return
	}
	f(q)
	for _, j := range q.Joins {
		walkQueries(j.Query, f)
	}
	for _, rel := range q.Relations {
		walkQueries(rel.Query, f)
	}
	walkGroup(q.Where, f)
	walkGroup(q.On, f)
	if q.Columns != nil {
		for _, s := range q.Columns.Sub {
			if s != nil {
				walkQueries(s.Query, f)
			}
		}
	}
}

func walkGroup(g *ir.Group, f func(*ir.Query)) {
	if g == nil {
		return
	}
	for _, it := range g.Items {
		if it.Group != nil {
			walkGroup(it.Group, f)
		}
		if it.Pred != nil && it.Pred.Sub != nil {
			walkQueries(it.Pred.Sub.Query, f)
		}
	}
}

// statement returns the rows of q that match: the rows of its table in storage
// order, without the soft-deleted rows (only the soft-deleted rows when deleted is
// true), kept by keep when it is given, joined, and filtered by the where of the
// root and of every join whose where no joined item places.
func (r *reader) statement(q *ir.Query, outer *frame, deleted bool, keep func(*rec) (bool, *Error)) (*scope, []*env, *Error) {
	s := r.buildScope(q, "")
	base, err := r.c.rows(s.tb.Name)
	if err != nil {
		return nil, nil, err
	}
	var envs []*env
	for _, x := range base {
		if s.ent.SoftDelete != "" {
			v, err := r.c.value(x, s.ent.SoftDelete)
			if err != nil {
				return nil, nil, err
			}
			if v.null == deleted {
				continue
			}
		}
		if keep != nil {
			ok, err := keep(x)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				continue
			}
		}
		envs = append(envs, &env{t: map[string]*rec{"": x}, root: s, outer: outer})
	}
	envs, err = r.expand(s, envs)
	if err != nil {
		return nil, nil, err
	}
	var out []*env
	for _, ev := range envs {
		v, err := r.group(s, ev, q.Where)
		if err != nil {
			return nil, nil, err
		}
		jv, err := r.joinWheres(s, ev)
		if err != nil {
			return nil, nil, err
		}
		if and3(v, jv) == tTrue {
			out = append(out, ev)
		}
	}
	return s, out, nil
}

// expand joins the rows of each join of s, in declaration order and each join's
// own joins after it: an inner join keeps the rows with a matching child, a left
// join also keeps a row without one, bound to nil. A child matches when its right
// column equals the parent's left column and its on group holds.
func (r *reader) expand(s *scope, envs []*env) ([]*env, *Error) {
	for _, j := range s.joins {
		children, err := r.c.rows(j.sc.tb.Name)
		if err != nil {
			return nil, err
		}
		var next []*env
		for _, ev := range envs {
			matched := false
			if parent := ev.t[s.path]; parent != nil {
				pv, pt, err := r.col(s, ev, j.def.Left)
				if err != nil {
					return nil, err
				}
				for _, cx := range children {
					ev2 := ev.with(j.sc.path, cx)
					cv, ct, err := r.col(j.sc, ev2, j.def.Right)
					if err != nil {
						return nil, err
					}
					eq, err := cmp3("eq", pt, pv, ct, cv)
					if err != nil {
						return nil, err
					}
					on, err := r.group(j.sc, ev2, j.sc.q.On)
					if err != nil {
						return nil, err
					}
					if and3(eq, on) == tTrue {
						next = append(next, ev2)
						matched = true
					}
				}
			}
			if !matched && j.def.Kind == "left" {
				next = append(next, ev.with(j.sc.path, nil))
			}
		}
		envs, err = r.expand(j.sc, next)
		if err != nil {
			return nil, err
		}
	}
	return envs, nil
}

// joinWheres is the conjunction of the where of every join of s, and of their
// joins, that no joined item of its parent's where places.
func (r *reader) joinWheres(s *scope, ev *env) (tri, *Error) {
	placed := map[string]bool{}
	placedJoins(s.q.Where, placed)
	res := tTrue
	for _, j := range s.joins {
		if w := j.sc.q.Where; w != nil && len(w.Items) > 0 && !placed[j.def.Rel] {
			v, err := r.group(j.sc, ev, w)
			if err != nil {
				return tFalse, err
			}
			res = and3(res, v)
		}
		v, err := r.joinWheres(j.sc, ev)
		if err != nil {
			return tFalse, err
		}
		res = and3(res, v)
	}
	return res, nil
}

func placedJoins(g *ir.Group, placed map[string]bool) {
	if g == nil {
		return
	}
	for _, it := range g.Items {
		if it.Joined != nil {
			placed[it.Joined.Join] = true
		}
		if it.Group != nil {
			placedJoins(it.Group, placed)
		}
	}
}

// sortKey is the value an order item sorts a row by.
type sortKey struct {
	v      scalar
	random float64
}

// order sorts envs by the order items of s, stably: rows that every item ties
// stay in storage order. A column sorts by its type, a column function by its
// result, and random by a random number that is not seeded. A NULL among the
// sorted values has no rule yet.
func (r *reader) order(s *scope, envs []*env) ([]*env, *Error) {
	items := s.q.Order
	if len(items) == 0 {
		return envs, nil
	}
	keys := make(map[*env][]sortKey, len(envs))
	for _, ev := range envs {
		ks := make([]sortKey, len(items))
		for i, o := range items {
			if o.Random {
				ks[i] = sortKey{random: rand.Float64()}
				continue
			}
			v, t, err := r.col(s, ev, o.Column)
			if err != nil {
				return nil, err
			}
			if o.Fn != nil {
				if v, t, err = columnFunction(o.Fn.Name, t, v); err != nil {
					return nil, err
				}
			}
			if v.null {
				return nil, unspecified("where an IR order puts a NULL value of " + s.ent.Name + "." + o.Column)
			}
			sv, ok := scalarOf(t, v.text)
			if !ok {
				return nil, errf(codeInternal, "%q is not a readable %s", v.text, t.String())
			}
			ks[i] = sortKey{v: sv}
		}
		keys[ev] = ks
	}
	out := slices.Clone(envs)
	sort.SliceStable(out, func(a, b int) bool {
		ka, kb := keys[out[a]], keys[out[b]]
		for i, o := range items {
			var c int
			if o.Random {
				switch {
				case ka[i].random < kb[i].random:
					c = -1
				case ka[i].random > kb[i].random:
					c = 1
				}
			} else {
				c = compare(ka[i].v, kb[i].v)
			}
			if o.Desc {
				c = -c
			}
			if c != 0 {
				return c < 0
			}
		}
		return false
	})
	return out, nil
}

// selectRows answers the rows of q: the matching rows, ordered, limited (to one
// for a one read), each built to its projection with its joins, and its relations
// attached.
func (r *reader) selectRows(q *ir.Query, one bool) ([]*Row, *Error) {
	if len(q.GroupBy) > 0 {
		return nil, unspecified("which row of a group a row read with group_by answers")
	}
	s, envs, err := r.statement(q, nil, false, nil)
	if err != nil {
		return nil, err
	}
	if envs, err = r.order(s, envs); err != nil {
		return nil, err
	}
	switch {
	case one:
		if len(envs) > 1 {
			envs = envs[:1]
		}
	case q.Limit != nil:
		envs = limitEnvs(envs, q.Limit.Offset, q.Limit.Count)
	}
	return r.assemble(s, envs)
}

// limitEnvs keeps count rows from offset on.
func limitEnvs(envs []*env, offset, count int) []*env {
	if offset >= len(envs) {
		return nil
	}
	end := offset + count
	if end > len(envs) || end < offset {
		end = len(envs)
	}
	return envs[offset:end]
}

// built is a row of a statement as built: its env and the row of each join path.
type built struct {
	ev   *env
	rows map[string]*Row
}

// assemble builds the rows of envs at s and attaches the relations of s and of its
// joins.
func (r *reader) assemble(s *scope, envs []*env) ([]*Row, *Error) {
	bs := make([]*built, len(envs))
	out := make([]*Row, len(envs))
	for i, ev := range envs {
		b := &built{ev: ev, rows: map[string]*Row{}}
		row, err := r.build(s, ev, b)
		if err != nil {
			return nil, err
		}
		bs[i], out[i] = b, row
	}
	if err := r.relations(s, bs); err != nil {
		return nil, err
	}
	return out, nil
}

// outCol is one output of a projection: a column, a column function or a scalar
// subquery, under its name.
type outCol struct {
	name, column string
	fn           *ir.Func
	sub          *ir.Sub
}

// projection resolves the columns of s as the planner does (orm
// engine/planner projection): the default set, every column or the keys; then the
// added and removed columns; the columns that relations bind or key on; the
// primary key first; then the column functions and the scalar subqueries by name.
func projection(s *scope) []outCol {
	c := s.q.Columns
	mode := ""
	if c != nil {
		mode = c.Mode
	}
	var base []string
	for _, f := range s.ent.Fields {
		switch mode {
		case "all":
			base = append(base, f.Name)
		case "none":
			if f.PrimaryKey || f.ForeignKey {
				base = append(base, f.Name)
			}
		default:
			if !f.SelectExplicit {
				base = append(base, f.Name)
			}
		}
	}
	if c != nil {
		for _, a := range c.Add {
			if !slices.Contains(base, a) {
				base = append(base, a)
			}
		}
		if len(c.Remove) > 0 {
			var kept []string
			for _, x := range base {
				if !slices.Contains(c.Remove, x) || s.ent.Field(x).PrimaryKey {
					kept = append(kept, x)
				}
			}
			base = kept
		}
	}
	for _, x := range s.extra {
		if !slices.Contains(base, x) {
			base = append(base, x)
		}
	}
	for _, rel := range s.q.Relations {
		for _, k := range rel.Keys {
			if !slices.Contains(base, k.Left) {
				base = append(base, k.Left)
			}
		}
		if rel.Query.IfParent != nil && !slices.Contains(base, rel.Query.IfParent.Column) {
			base = append(base, rel.Query.IfParent.Column)
		}
	}
	for _, pk := range s.ent.PK {
		if !slices.Contains(base, pk) {
			base = append([]string{pk}, base...)
		}
	}
	out := make([]outCol, 0, len(base))
	for _, x := range base {
		out = append(out, outCol{name: x, column: x})
	}
	if c != nil {
		names := make([]string, 0, len(c.Fn))
		for n := range c.Fn {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			cf := c.Fn[n]
			out = append(out, outCol{name: n, column: cf.Column, fn: &cf.Fn})
		}
		names = names[:0]
		for n := range c.Sub {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			out = append(out, outCol{name: n, sub: c.Sub[n]})
		}
	}
	return out
}

// build builds the row of ev at s and of each of its joins, recording each in b.
func (r *reader) build(s *scope, ev *env, b *built) (*Row, *Error) {
	row := newRow()
	for _, oc := range projection(s) {
		switch {
		case oc.sub != nil:
			v, t, err := r.subScalar(oc.sub, &frame{sc: s, ev: ev})
			if err != nil {
				return nil, err
			}
			row.set(oc.name, output(t, v))
		default:
			v, t, err := r.col(s, ev, oc.column)
			if err != nil {
				return nil, err
			}
			if oc.fn != nil {
				if v, t, err = columnFunction(oc.fn.Name, t, v); err != nil {
					return nil, err
				}
			}
			row.set(oc.name, output(t, v))
		}
	}
	b.rows[s.path] = row
	for _, j := range s.joins {
		if ev.t[j.sc.path] == nil {
			row.set(j.def.Rel, nil)
			continue
		}
		child, err := r.build(j.sc, ev, b)
		if err != nil {
			return nil, err
		}
		row.set(j.def.Rel, child)
	}
	return row, nil
}

// keyText returns a text that equal values of one kind share: the value's
// comparison form.
func keyText(t dbspec.Type, v cell) (string, bool) {
	if v.null {
		return "", false
	}
	sv, ok := scalarOf(t, v.text)
	if !ok {
		return v.text, true
	}
	switch sv.kind {
	case kindNumber:
		return "n" + sv.num.RatString(), true
	case kindTime:
		return "t" + strconv.FormatInt(sv.tm.UnixNano(), 10), true
	case kindTimeOfDay:
		return "d" + strconv.FormatInt(sv.nanos, 10), true
	}
	return "s" + sv.text, true
}

// relations runs the relations of s and of its joins for the rows bs, and attaches
// their rows (orm protocol 1.2 and 2.1): each relation reads the child rows whose
// right keys equal the left keys of a parent row, without NULL keys; with if_parent
// only the parents whose column equals the parameter; a one relation attaches the
// first child row, a many relation the child rows in child order, keyed by key_by
// when it is given (a repeated key keeps the last row); flatten merges the child's
// outputs into the parent row. limit_per_parent keeps the first n child rows of each
// parent, by the child's order or else by its primary key, and a one relation with
// an order keeps the first by that order, as the planner's ROW_NUMBER does.
func (r *reader) relations(s *scope, bs []*built) *Error {
	for _, rel := range s.q.Relations {
		if err := r.relation(s, bs, rel); err != nil {
			return err
		}
	}
	for _, j := range s.joins {
		if err := r.relations(j.sc, bs); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) relation(s *scope, bs []*built, rel *ir.Relation) *Error {
	cq := rel.Query
	// The parent key tuples of the parents that load the relation.
	type parentKey struct {
		b   *built
		key string
	}
	var parents []parentKey
	wanted := map[string]bool{}
	for _, b := range bs {
		if b.ev.t[s.path] == nil {
			continue
		}
		if cq.IfParent != nil {
			v, t, err := r.col(s, b.ev, cq.IfParent.Column)
			if err != nil {
				return err
			}
			pv, err := r.param(cq.IfParent.P, t)
			if err != nil {
				return err
			}
			eq, err := cmp3("eq", t, v, t, pv)
			if err != nil {
				return err
			}
			if eq != tTrue {
				parents = append(parents, parentKey{b: b})
				continue
			}
		}
		parts := make([]string, len(rel.Keys))
		ok := true
		for i, k := range rel.Keys {
			v, t, err := r.col(s, b.ev, k.Left)
			if err != nil {
				return err
			}
			kt, present := keyText(t, v)
			if !present {
				ok = false
				break
			}
			parts[i] = kt
		}
		if !ok {
			parents = append(parents, parentKey{b: b})
			continue
		}
		key := strings.Join(parts, "\x00")
		wanted[key] = true
		parents = append(parents, parentKey{b: b, key: key})
	}
	childKeyOf := func(cs *scope, ev *env) (string, bool, *Error) {
		parts := make([]string, len(rel.Keys))
		for i, k := range rel.Keys {
			v, t, err := r.col(cs, ev, k.Right)
			if err != nil {
				return "", false, err
			}
			kt, present := keyText(t, v)
			if !present {
				return "", false, nil
			}
			parts[i] = kt
		}
		return strings.Join(parts, "\x00"), true, nil
	}
	var cs *scope
	var envs []*env
	if len(wanted) > 0 {
		var err *Error
		cs, envs, err = r.statement(cq, nil, false, nil)
		if err != nil {
			return err
		}
		kept := envs[:0:0]
		for _, ev := range envs {
			k, ok, err := childKeyOf(cs, ev)
			if err != nil {
				return err
			}
			if ok && wanted[k] {
				kept = append(kept, ev)
			}
		}
		envs = kept
		perParent := cq.LimitPerParent
		if perParent == 0 && rel.Kind == "one" && len(cq.Order) > 0 {
			perParent = 1
		}
		if perParent > 0 && len(cq.Order) == 0 {
			// The planner ranks by the primary key when the child has no order.
			cq2 := *cq
			for _, pk := range cs.ent.PK {
				cq2.Order = append(cq2.Order, ir.Order{Column: pk})
			}
			cs.q = &cq2
			envs, err = r.order(cs, envs)
			cs.q = cq
		} else {
			envs, err = r.order(cs, envs)
		}
		if err != nil {
			return err
		}
		if perParent > 0 {
			count := map[string]int{}
			kept := envs[:0:0]
			for _, ev := range envs {
				k, _, err := childKeyOf(cs, ev)
				if err != nil {
					return err
				}
				if count[k] < perParent {
					count[k]++
					kept = append(kept, ev)
				}
			}
			envs = kept
		}
		cs.extra = nil
		for _, k := range rel.Keys {
			cs.extra = append(cs.extra, k.Right)
		}
		if cq.KeyBy != "" {
			cs.extra = append(cs.extra, cq.KeyBy)
		}
	}
	var rows []*Row
	if cs != nil {
		var err *Error
		if rows, err = r.assemble(cs, envs); err != nil {
			return err
		}
	}
	byKey := map[string][]int{}
	for i, ev := range envs {
		k, _, err := childKeyOf(cs, ev)
		if err != nil {
			return err
		}
		byKey[k] = append(byKey[k], i)
	}
	for _, p := range parents {
		row := p.b.rows[s.path]
		var idx []int
		if p.key != "" {
			idx = byKey[p.key]
		}
		switch {
		case rel.Kind == "one" && cq.Flatten:
			if len(idx) == 0 {
				return unspecified("the outputs of a flattened one relation " + rel.Rel + " that loads no row")
			}
			child := rows[idx[0]]
			for _, n := range child.names {
				if row.has(n) {
					return unspecified("a flattened output " + n + " of the relation " + rel.Rel + " that the parent row holds")
				}
				row.set(n, child.values[n])
			}
		case rel.Kind == "one":
			if len(idx) == 0 {
				row.set(rel.Rel, nil)
			} else {
				row.set(rel.Rel, rows[idx[0]])
			}
		case cq.KeyBy != "":
			keyed := newRow()
			for _, i := range idx {
				v, t, err := r.col(cs, envs[i], cq.KeyBy)
				if err != nil {
					return err
				}
				if v.null {
					return unspecified("the key of a many relation " + rel.Rel + " keyed by a NULL " + cq.KeyBy)
				}
				keyed.set(keyName(t, v), rows[i])
			}
			row.set(rel.Rel, keyed)
		default:
			list := make([]*Row, 0, len(idx))
			for _, i := range idx {
				list = append(list, rows[i])
			}
			row.set(rel.Rel, list)
		}
	}
	return nil
}

// keyName is the text of a key_by value as a JSON object key: the value as the
// runtime value model writes it, a string as it is.
func keyName(t dbspec.Type, v cell) string {
	switch o := output(t, v).(type) {
	case string:
		return o
	case json.Number:
		return o.String()
	case bool:
		return strconv.FormatBool(o)
	}
	return v.text
}

// count counts the matching rows of q, or its groups when it has group_by. A
// count ignores the order and the limit, as the planner's count statement does.
func (r *reader) count(q *ir.Query) (int64, *Error) {
	s, envs, err := r.statement(q, nil, false, nil)
	if err != nil {
		return 0, err
	}
	if len(q.GroupBy) == 0 {
		return int64(len(envs)), nil
	}
	groups, _, err := r.groups(s, envs)
	if err != nil {
		return 0, err
	}
	return int64(len(groups)), nil
}

// groups partitions envs by the group_by columns of s, NULLs together, in the order
// each group's first row stands in storage order.
func (r *reader) groups(s *scope, envs []*env) ([][]*env, []*env, *Error) {
	index := map[string]int{}
	var groups [][]*env
	var firsts []*env
	for _, ev := range envs {
		parts := make([]string, len(s.q.GroupBy))
		for i, g := range s.q.GroupBy {
			v, t, err := r.col(s, ev, g)
			if err != nil {
				return nil, nil, err
			}
			kt, present := keyText(t, v)
			if !present {
				kt = "null"
			}
			parts[i] = kt
		}
		k := strings.Join(parts, "\x00")
		i, ok := index[k]
		if !ok {
			i = len(groups)
			index[k] = i
			groups = append(groups, nil)
			firsts = append(firsts, ev)
		}
		groups[i] = append(groups[i], ev)
	}
	return groups, firsts, nil
}

// groupCount answers one row per group: its group columns and row_count.
func (r *reader) groupCount(q *ir.Query) ([]*Row, *Error) {
	s, envs, err := r.statement(q, nil, false, nil)
	if err != nil {
		return nil, err
	}
	groups, firsts, err := r.groups(s, envs)
	if err != nil {
		return nil, err
	}
	for _, o := range q.Order {
		if !o.Random && !slices.Contains(q.GroupBy, o.Column) {
			return nil, unspecified("the order of groups by " + o.Column + ", which is not a group column")
		}
	}
	sizes := map[*env]int{}
	for i, f := range firsts {
		sizes[f] = len(groups[i])
	}
	ordered, err := r.order(s, firsts)
	if err != nil {
		return nil, err
	}
	if q.Limit != nil {
		ordered = limitEnvs(ordered, q.Limit.Offset, q.Limit.Count)
	}
	out := make([]*Row, 0, len(ordered))
	for _, f := range ordered {
		row := newRow()
		for _, g := range q.GroupBy {
			v, t, err := r.col(s, f, g)
			if err != nil {
				return nil, err
			}
			row.set(g, output(t, v))
		}
		row.set("row_count", json.Number(strconv.Itoa(sizes[f])))
		out = append(out, row)
	}
	return out, nil
}

// aggregate answers sum or avg of column col over the matching rows of q. NULLs
// are skipped. A sum of no value is 0 (the planner's COALESCE(SUM(c), 0)) in the
// column's type; an avg of no value is NULL, and an avg is an f64, the type the
// planner gives it.
func (r *reader) aggregate(q *ir.Query, kind, col string) (any, *Error) {
	s, envs, err := r.statement(q, nil, false, nil)
	if err != nil {
		return nil, err
	}
	vals := make([]cell, 0, len(envs))
	var t dbspec.Type
	for _, ev := range envs {
		v, vt, err := r.col(s, ev, col)
		if err != nil {
			return nil, err
		}
		t = vt
		vals = append(vals, v)
	}
	if t.Kind == "" {
		t = fieldType(s.ent.Field(col))
	}
	v, vt := aggregateOf(kind, t, vals)
	return output(vt, v), nil
}

// aggregateOf computes sum or avg of the values of type t exactly, then writes the
// sum in its column type and the avg as the nearest f64.
func aggregateOf(kind string, t dbspec.Type, vals []cell) (cell, dbspec.Type) {
	sum := new(big.Rat)
	n := 0
	for _, v := range vals {
		if v.null {
			continue
		}
		x, ok := new(big.Rat).SetString(v.text)
		if !ok {
			continue
		}
		sum.Add(sum, x)
		n++
	}
	f64 := dbspec.Type{Kind: dbspec.TypeF64}
	if kind == "avg" {
		if n == 0 {
			return cell{null: true}, f64
		}
		avg := new(big.Rat).Quo(sum, new(big.Rat).SetInt64(int64(n)))
		f, _ := avg.Float64()
		return cell{text: strconv.FormatFloat(f, 'f', -1, 64)}, f64
	}
	switch t.Kind {
	case dbspec.TypeDecimal:
		return cell{text: sum.FloatString(t.Scale)}, t
	case dbspec.TypeF64:
		f, _ := sum.Float64()
		return cell{text: strconv.FormatFloat(f, 'f', -1, 64)}, t
	}
	return cell{text: sum.RatString()}, t
}

// subRows returns the matching rows of a subquery and its root scope, owned by the
// query of fr.
func (r *reader) subRows(sub *ir.Sub, fr *frame) (*scope, []*env, *Error) {
	if len(sub.Query.GroupBy) > 0 {
		return nil, nil, unspecified("a subquery with group_by")
	}
	return r.statement(sub.Query, fr, false, nil)
}

// subColumn returns the values of the column of a subquery, for in and not_in.
func (r *reader) subColumn(sub *ir.Sub, fr *frame) ([]cell, dbspec.Type, *Error) {
	s, envs, err := r.subRows(sub, fr)
	if err != nil {
		return nil, dbspec.Type{}, err
	}
	t := fieldType(s.ent.Field(sub.Column))
	vals := make([]cell, 0, len(envs))
	for _, ev := range envs {
		v, _, err := r.col(s, ev, sub.Column)
		if err != nil {
			return nil, t, err
		}
		vals = append(vals, v)
	}
	return vals, t, nil
}

// subScalar returns the value of a scalar subquery: sum (0 for no row), avg (NULL
// for no row), count, or the column of its one row (NULL for no row).
func (r *reader) subScalar(sub *ir.Sub, fr *frame) (cell, dbspec.Type, *Error) {
	switch sub.Agg {
	case "count":
		_, envs, err := r.subRows(sub, fr)
		if err != nil {
			return cell{}, dbspec.Type{}, err
		}
		return cell{text: strconv.Itoa(len(envs))}, dbspec.Type{Kind: dbspec.TypeI64}, nil
	case "sum", "avg":
		vals, t, err := r.subColumn(sub, fr)
		if err != nil {
			return cell{}, t, err
		}
		v, vt := aggregateOf(sub.Agg, t, vals)
		return v, vt, nil
	}
	vals, t, err := r.subColumn(sub, fr)
	if err != nil {
		return cell{}, t, err
	}
	switch len(vals) {
	case 0:
		return cell{null: true}, t, nil
	case 1:
		return vals[0], t, nil
	}
	return cell{}, t, unspecified(fmt.Sprintf("a scalar subquery of %s that matches %d rows", sub.Query.Entity, len(vals)))
}
