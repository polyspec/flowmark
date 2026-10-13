package irexec

import (
	"fmt"
	"maps"
	"math/big"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store"
	"github.com/polyspec/orm/engine/dbspec"
	"github.com/polyspec/orm/engine/ir"
	"github.com/polyspec/orm/engine/runtimemodel"
)

// writer computes the post-image of one write: it changes a copy of the corpus,
// the next corpus, and records the files whose rows it changed.
type writer struct {
	r       *reader
	c, next *Corpus
	tb      *catalog.Table
	ent     *runtimemodel.Entity
	touched map[string]bool
	given   []*given
	faults  []fault
	// dupFaulted holds the rows that have their one duplicate fault.
	dupFaulted map[*given]bool
	answer     *Answer
	// copies maps each corpus row to its copy in the next corpus, before the
	// write changes the copy.
	copies map[*rec]*rec
}

// fault is a fault of a refused write with the position of its row in request
// order: seq is the place of the row among the rows the write takes, an insert's in
// their given order and an update's or delete's in storage order.
type fault struct {
	store.Refusal
	seq int
}

// given is a row the write writes, in the next corpus: its position among the rows
// given, its place among the rows the write takes, whether the write inserts it,
// the columns the request assigns, and its values before the write (nil for an
// insert).
type given struct {
	row      *store.Row
	index    int
	seq      int
	inserted bool
	assigned map[string]bool
	before   map[string]cell
	// entered is the state the write puts the row in, "" when it puts it in none:
	// the state of an inserted row, or a new state of an updated row.
	entered string
}

// write answers a write request with its post-image.
func (e *Executor) write(c *Corpus, cl *call) (*Result, *Error) {
	r := &reader{e: e, c: c, cl: cl}
	req := cl.req
	if err := r.prepare(&req.Query); err != nil {
		return nil, err
	}
	ent := e.model.Entities[req.Entity]
	tb, _ := c.cat.Table(ent.Table)
	w := &writer{r: r, c: c, next: c.clone(), tb: tb, ent: ent, touched: map[string]bool{}, answer: &Answer{Kind: req.Kind}}
	var err *Error
	switch req.Kind {
	case "insert":
		err = w.insert()
	case "update":
		err = w.update()
	case "delete":
		err = w.delete()
	case "restore":
		err = w.restore()
	}
	if err != nil {
		return nil, err
	}
	post, err := w.finish()
	if err != nil {
		return nil, err
	}
	return &Result{Answer: w.answer, Post: post}, nil
}

// clone returns a deep copy of the corpus's rows, which a write changes.
func (c *Corpus) clone() *Corpus {
	n := &Corpus{cat: c.cat, files: map[string]*file{}, unavailable: c.unavailable, walk: c.walk}
	for p, f := range c.files {
		rows := make([]*store.Row, len(f.rows))
		for i, r := range f.rows {
			rows[i] = cloneRow(r)
		}
		n.files[p] = &file{table: f.table, data: f.data, rows: rows, diags: f.diags}
	}
	n.index()
	return n
}

// cloneRow copies a row and its block rows.
func cloneRow(r *store.Row) *store.Row {
	cp := *r
	cp.Scalars = maps.Clone(r.Scalars)
	cp.Sections = maps.Clone(r.Sections)
	cp.Raw = maps.Clone(r.Raw)
	cp.Dangling = maps.Clone(r.Dangling)
	cp.Shared = maps.Clone(r.Shared)
	if r.Blocks != nil {
		cp.Blocks = map[string][]*store.Row{}
		for name, rows := range r.Blocks {
			out := make([]*store.Row, len(rows))
			for i, b := range rows {
				out[i] = cloneRow(b)
			}
			cp.Blocks[name] = out
		}
	}
	return &cp
}

// nextOf returns the row of the next corpus that is the corpus row x. The copy is
// indexed in the corpus's order, so a row and its copy share their position; the
// write asks for it before it changes the copy.
func (w *writer) nextOf(x *rec) *rec {
	if w.copies == nil {
		w.copies = map[*rec]*rec{}
		for t, xs := range w.c.recs {
			for i, y := range xs {
				w.copies[y] = w.next.recs[t][i]
			}
		}
	}
	return w.copies[x]
}

// fault records a fault of the row g at column.
func (w *writer) fault(g *given, table, column, rule, msg string) {
	idx := -1
	if g != nil && g.inserted && w.sharedGiven(g) {
		idx = g.index
	}
	key, seq := "", 0
	if g != nil {
		key, seq = g.row.Key, g.seq
	}
	w.faults = append(w.faults, fault{store.Refusal{Table: table, Key: key, Index: idx, Column: column, Rule: rule, Message: msg}, seq})
	if rule == codeDuplicateKey && g != nil {
		w.duplicated(g)
	}
}

// duplicated records that g has its one duplicate fault.
func (w *writer) duplicated(g *given) {
	if w.dupFaulted == nil {
		w.dupFaulted = map[*given]bool{}
	}
	w.dupFaulted[g] = true
}

// take records g as the next row the write takes, in request order.
func (w *writer) take(g *given) {
	g.seq = len(w.given)
	w.given = append(w.given, g)
}

// sharedGiven reports whether another row given to the write shares g's key byte
// for byte, so that its faults carry its index.
func (w *writer) sharedGiven(g *given) bool {
	for _, o := range w.given {
		if o != g && o.inserted && o.row.Key == g.row.Key && o.row.Table == g.row.Table {
			return true
		}
	}
	return false
}

// valueOf returns the value of column col of row in the next corpus.
func (w *writer) valueOf(row *store.Row, col string) (cell, *Error) {
	return w.next.value(w.next.byRow[row], col)
}

// assignValue computes the value an assignment gives column f of a row whose value
// was old: a parameter, NULL, or the old value plus or minus a parameter (NULL stays
// NULL, and minus stores 0 instead of a value below the parameter, as the planner's
// CASE does). ok is false, with a fault message, when the value is not of the
// column type.
func (w *writer) assignValue(a *ir.Assign, f *runtimemodel.Field, old cell, p *int) (cell, string) {
	t := fieldType(f)
	if a.Null {
		return cell{null: true}, ""
	}
	idx := a.P
	if p != nil {
		idx = p
	}
	if idx != nil {
		v, ok := decodeParam(w.r.cl.params[*idx], t)
		if !ok {
			return cell{}, fmt.Sprintf("the value of parameter %d is not a value of %s %s", *idx, f.Name, t.String())
		}
		return v, ""
	}
	pi := a.PlusP
	if pi == nil {
		pi = a.MinusP
	}
	pv, ok := decodeParam(w.r.cl.params[*pi], t)
	if !ok {
		return cell{}, fmt.Sprintf("the value of parameter %d is not a value of %s %s", *pi, f.Name, t.String())
	}
	if old.null || pv.null {
		return cell{null: true}, ""
	}
	x, _ := new(big.Rat).SetString(old.text)
	y, _ := new(big.Rat).SetString(pv.text)
	var z *big.Rat
	if a.PlusP != nil {
		z = new(big.Rat).Add(x, y)
	} else if x.Cmp(y) > 0 {
		z = new(big.Rat).Sub(x, y)
	} else {
		z = new(big.Rat)
	}
	text := z.RatString()
	switch t.Kind {
	case dbspec.TypeDecimal:
		text = z.FloatString(t.Scale)
	case dbspec.TypeF64:
		fl, _ := z.Float64()
		text = strconv.FormatFloat(fl, 'f', -1, 64)
	}
	v, ok := store.Readable(t, text)
	if !ok {
		return cell{}, fmt.Sprintf("%s %s is not a value of %s", f.Name, text, t.String())
	}
	return cell{text: v}, ""
}

// setValue gives column col of row the value v in its place: the key, the title,
// the body, a markdown section or a stored scalar. A markdown value's last line ends
// with a line feed in the row, as the store holds it.
func setValue(tb *catalog.Table, row *store.Row, col string, v cell) {
	withLF := func(s string) string {
		if s == "" {
			return ""
		}
		return s + "\n"
	}
	switch {
	case col == tb.Key:
		row.Key = v.text
	case col == tb.Title:
		row.Title = v.text
	case col == tb.Body:
		row.Body = withLF(v.text)
	case isMarkdown(tb, col):
		if row.Sections == nil {
			row.Sections = map[string]*string{}
		}
		if v.null {
			delete(row.Sections, col)
		} else {
			s := withLF(v.text)
			row.Sections[col] = &s
		}
	default:
		if row.Scalars == nil {
			row.Scalars = map[string]*string{}
		}
		if v.null {
			delete(row.Scalars, col)
		} else {
			s := v.text
			row.Scalars[col] = &s
		}
		delete(row.Raw, col)
		delete(row.Dangling, col)
		delete(row.Shared, col)
	}
}

// defaultOf returns the value a column takes when an insert omits it: its declared
// default, written explicitly, or the request's clock for a default now.
func (w *writer) defaultOf(c dbspec.Column) (cell, bool) {
	if c.Default == nil {
		return cell{}, false
	}
	if c.Default.Now {
		return cell{text: formatDatetime(w.r.cl.now.Truncate(precisionUnit(c.Type.Precision)), c.Type.Precision)}, true
	}
	lit := c.Default.Literal
	if strings.HasPrefix(lit, "'") && strings.HasSuffix(lit, "'") && len(lit) >= 2 {
		lit = strings.ReplaceAll(lit[1:len(lit)-1], "''", "'")
	}
	v, ok := store.Readable(c.Type, lit)
	return cell{text: v}, ok
}

// clock returns the request's clock as a value of a datetime column.
func (w *writer) clock(t dbspec.Type) cell {
	return cell{text: formatDatetime(w.r.cl.now.Truncate(precisionUnit(t.Precision)), t.Precision)}
}

// insert inserts the rows of the request: the set row and each row of rows.
func (w *writer) insert() *Error {
	req := w.r.cl.req
	for _, a := range req.Set {
		f := w.ent.Field(a.Column)
		if f.Identity {
			return errf(codeIRInvalid, "cannot set identity column %s", a.Column)
		}
	}
	if err := w.requireAssigned(req.Set); err != nil {
		return err
	}
	rows := [][]*int{nil}
	for _, ps := range req.Rows {
		row := make([]*int, len(ps))
		for i := range ps {
			row[i] = &ps[i]
		}
		rows = append(rows, row)
	}
	for i, ps := range rows {
		if len(req.OnDuplicate) > 0 {
			done, err := w.onDuplicate(ps)
			if err != nil {
				return err
			}
			if done {
				continue
			}
		}
		if err := w.insertRow(i, ps); err != nil {
			return err
		}
	}
	w.answer.Affected = int64(len(rows))
	return nil
}

// requireAssigned refuses an insert that omits a non-null column with no default
// that the executor does not assign (orm protocol 1.4): the identity, key_prefix and
// order columns are executor-assigned.
func (w *writer) requireAssigned(set []ir.Assign) *Error {
	for _, f := range w.ent.Fields {
		if f.Null || f.Default || f.Identity || f.Name == w.tb.Order || (f.Name == w.tb.Key && w.keyPrefix() != "") {
			continue
		}
		if !slices.ContainsFunc(set, func(a ir.Assign) bool { return a.Column == f.Name }) {
			return errf(codeIRInvalid, "required column %s.%s is not set", w.ent.Name, f.Name)
		}
	}
	return nil
}

// keyPrefix returns the key_prefix of the table, "" when it has none.
func (w *writer) keyPrefix() string {
	for _, d := range w.c.cat.Documents {
		for _, t := range d.Tables {
			if t.Name == w.tb.Name && t.Settings != nil && t.Settings.KeyPrefix != nil {
				return t.Settings.KeyPrefix.Prefix
			}
		}
	}
	return ""
}

// insertRow inserts the i-th row of the request, whose set parameters are ps (nil
// for the set row itself).
func (w *writer) insertRow(i int, ps []*int) *Error {
	req := w.r.cl.req
	tb := w.tb
	g := &given{row: &store.Row{Table: tb.Name, Scalars: map[string]*string{}, Sections: map[string]*string{}}, index: i, inserted: true, assigned: map[string]bool{}}
	vals := map[string]cell{}
	type valueFault struct{ column, msg string }
	var bad []valueFault
	for k, a := range req.Set {
		var p *int
		if ps != nil {
			p = ps[k]
		}
		f := w.ent.Field(a.Column)
		v, msg := w.assignValue(&a, f, cell{null: true}, p)
		if msg != "" {
			bad = append(bad, valueFault{a.Column, msg})
			continue
		}
		vals[a.Column] = v
		g.assigned[a.Column] = true
	}
	for _, c := range tb.Columns {
		if slices.ContainsFunc(req.Set, func(a ir.Assign) bool { return a.Column == c.Name }) {
			continue
		}
		f := w.ent.Field(c.Name)
		switch {
		case f.Identity:
			vals[c.Name] = w.nextIdentity(tb, c.Name)
		case c.Name == tb.Key && w.keyPrefix() != "":
			vals[c.Name] = w.nextPrefixed(tb, w.keyPrefix())
		case c.Name == tb.Order:
			// assigned once the row is placed
		default:
			if v, ok := w.defaultOf(c); ok {
				vals[c.Name] = v
			} else {
				vals[c.Name] = cell{null: true}
			}
		}
	}
	key, keyGiven := vals[tb.Key]
	g.row.Key = key.text
	w.take(g)
	for _, b := range bad {
		w.fault(g, tb.Name, b.column, codeConstraint, b.msg)
	}
	if !keyGiven {
		// The key's value is not of its type: its fault stands for the row.
		return nil
	}
	if key.null {
		w.fault(g, tb.Name, tb.Key, codeConstraint, "the key has no value")
		return nil
	}
	placement := placementColumn(tb)
	for _, c := range tb.Columns {
		v, ok := vals[c.Name]
		if !ok || c.Name == placement || c.Name == tb.Order {
			continue
		}
		setValue(tb, g.row, c.Name, v)
	}
	if sm := tb.StateMachine; sm != nil {
		if s := vals[sm.Column]; !s.null {
			if !slices.Contains(sm.Initial, s.text) {
				w.fault(g, tb.Name, sm.Column, codeConstraint, fmt.Sprintf("the state %s of an inserted row is not an initial state of %s", s.text, sm.Column))
			}
			g.entered = s.text
		}
	}
	// Place the row: a files row in its own file, a document row in the table's
	// document, a block row in the scope of the row its placement key names.
	switch tb.Storage.Kind {
	case catalog.StorageFiles:
		path, perr := w.rowPath(tb, g, vals)
		if perr != nil {
			return perr
		}
		if path == "" {
			return nil
		}
		g.row.Path = path
		if _, exists := w.next.files[path]; exists {
			// The key equals an existing row's key byte for byte: the key check
			// reports it; the row is placed nowhere.
			return nil
		}
		w.next.files[path] = &file{table: tb.Name, rows: []*store.Row{g.row}}
		w.touched[path] = true
	case catalog.StorageDocument:
		path, perr := w.rowPath(tb, g, vals)
		if perr != nil {
			return perr
		}
		if path == "" {
			return nil
		}
		f := w.next.files[path]
		if f == nil {
			f = &file{table: tb.Name}
			w.next.files[path] = f
		}
		f.rows = append(f.rows, g.row)
		w.touched[path] = true
	case catalog.StorageBlock:
		pv := vals[placement]
		p, err := w.placementRow(tb, pv)
		if err != nil {
			return err
		}
		if p == nil {
			w.fault(g, tb.Name, placement, codeForeignKey, fmt.Sprintf("the foreign key %s names no %s row with the key %s", placement, tb.Storage.Parent, pv.text))
			return nil
		}
		if p.row.Blocks == nil {
			p.row.Blocks = map[string][]*store.Row{}
		}
		p.row.Blocks[tb.Name] = append(p.row.Blocks[tb.Name], g.row)
		w.touched[p.file] = true
	}
	w.next.index()
	if tb.Order != "" {
		var k *cell
		if v, ok := vals[tb.Order]; ok && !v.null {
			k = &v
		}
		if err := w.placeInOrder(g, k); err != nil {
			return err
		}
	}
	key0, _ := w.valueOf(g.row, tb.Key)
	return w.answerKey(key0)
}

// answerKey appends the key k of a written row to the answer's keys.
func (w *writer) answerKey(k cell) *Error {
	o, err := output(columnOf(w.tb, w.tb.Key).Type, k)
	if err != nil {
		return err
	}
	w.answer.Keys = append(w.answer.Keys, o)
	return nil
}

// placementRow returns the row of the placement parent's table that the placement
// key v names byte for byte, nil when none does. The parent table is needed.
func (w *writer) placementRow(tb *catalog.Table, v cell) (*rec, *Error) {
	if err := w.next.need(tb.Storage.Parent); err != nil {
		return nil, err
	}
	if v.null {
		return nil, nil
	}
	for _, x := range w.next.recs[tb.Storage.Parent] {
		if x.row.Key == v.text {
			return x, nil
		}
	}
	return nil, nil
}

// nextIdentity allocates max+1 over the identity column of the table's rows.
func (w *writer) nextIdentity(tb *catalog.Table, col string) cell {
	max := int64(0)
	for _, x := range w.next.recs[tb.Name] {
		v, err := w.next.value(x, col)
		if err != nil || v.null {
			continue
		}
		if n, perr := strconv.ParseInt(v.text, 10, 64); perr == nil && n > max {
			max = n
		}
	}
	return cell{text: strconv.FormatInt(max+1, 10)}
}

// nextPrefixed allocates <p><n + 1>, n the largest decimal suffix without a leading
// zero among the keys of the form <p><n> (spec 20 Keys).
func (w *writer) nextPrefixed(tb *catalog.Table, p string) cell {
	max := new(big.Int)
	for _, x := range w.next.recs[tb.Name] {
		k := x.row.Key
		if !strings.HasPrefix(k, p) {
			continue
		}
		s := k[len(p):]
		if s == "" || (len(s) > 1 && s[0] == '0') || strings.Trim(s, "0123456789") != "" {
			continue
		}
		n, _ := new(big.Int).SetString(s, 10)
		if n.Cmp(max) > 0 {
			max = n
		}
	}
	return cell{text: p + new(big.Int).Add(max, big.NewInt(1)).String()}
}

// onDuplicate applies the on_duplicate assignments to the row that the inserted row
// meets on its primary key, without regard to case, or on a unique key, byte for
// byte, and reports whether it did. A row met on no key is inserted.
func (w *writer) onDuplicate(ps []*int) (bool, *Error) {
	req := w.r.cl.req
	vals := map[string]cell{}
	for k, a := range req.Set {
		var p *int
		if ps != nil {
			p = ps[k]
		}
		v, msg := w.assignValue(&a, w.ent.Field(a.Column), cell{null: true}, p)
		if msg != "" {
			return false, nil // the insert reports the fault
		}
		vals[a.Column] = v
	}
	// The primary key meets the row whose key equals the inserted key without regard
	// to case, as flowmark's primary key compares: the byte-equal key first, else the
	// least in byte order (spec 60 Surfaces, primary key comparison). The met row keeps
	// its key, since a key is never updated.
	var met *rec
	if k, ok := vals[w.tb.Key]; ok && !k.null {
		for _, x := range w.next.recs[w.tb.Name] {
			switch {
			case x.row.Key == k.text:
				met = x
			case met != nil && met.row.Key == k.text:
			case store.FoldKey(x.row.Key) == store.FoldKey(k.text) && (met == nil || x.row.Key < met.row.Key):
				met = x
			}
		}
	}
	for _, x := range w.next.recs[w.tb.Name] {
		for _, k := range w.tb.Uniques {
			equal := true
			for _, col := range k {
				v, ok := vals[col]
				if !ok || v.null {
					equal = false
					break
				}
				xv, err := w.next.value(x, col)
				if err != nil {
					return false, err
				}
				if xv.null || xv.text != v.text {
					equal = false
					break
				}
			}
			if equal {
				if met != nil && met != x {
					return false, errf(codeInternal, "an insert with on_duplicate met two rows, which ir.Validate refuses by allowing one conflict key")
				}
				met = x
			}
		}
	}
	if met == nil {
		return false, nil
	}
	if err := w.updateRows([]*rec{met}, req.OnDuplicate, false); err != nil {
		return false, err
	}
	key, _ := w.valueOf(met.row, w.tb.Key)
	return true, w.answerKey(key)
}

// matched returns the rows of the next corpus that the write's where matches, once
// each, in storage order. deleted selects the soft-deleted rows instead of the live
// ones.
func (w *writer) matched(deleted bool) ([]*rec, *Error) {
	req := w.r.cl.req
	s, envs, err := w.r.statement(&req.Query, nil, deleted, nil)
	if err != nil {
		return nil, err
	}
	var out []*rec
	seen := map[*rec]bool{}
	for _, ev := range envs {
		if req.Optimistic != nil {
			v, t, err := w.r.col(s, ev, req.Optimistic.Column)
			if err != nil {
				return nil, err
			}
			pv, err := w.r.param(req.Optimistic.P, t)
			if err != nil {
				return nil, err
			}
			eq, err := cmp3("eq", t, v, t, pv)
			if err != nil {
				return nil, err
			}
			if eq != tTrue {
				continue
			}
		}
		x := ev.t[""]
		if !seen[x] {
			seen[x] = true
			out = append(out, w.nextOf(x))
		}
	}
	return out, nil
}

// update applies the set assignments to the matched rows.
func (w *writer) update() *Error {
	req := w.r.cl.req
	rows, err := w.matched(false)
	if err != nil {
		return err
	}
	if req.Optimistic != nil && len(rows) == 0 {
		return errf(codeOptimistic, "the optimistic update of %s matched no row", w.ent.Name)
	}
	if err := w.updateRows(rows, req.Set, true); err != nil {
		return err
	}
	w.answer.Affected = int64(len(rows))
	return nil
}

// updateRows applies the assignments set to rows. Every assignment reads the row's
// values before the update. With stamp, the updated column takes the request's clock
// unless the request assigns it.
func (w *writer) updateRows(rows []*rec, set []ir.Assign, stamp bool) *Error {
	tb := w.tb
	if tb.Order != "" && slices.ContainsFunc(set, func(a ir.Assign) bool { return a.Column == tb.Order }) && len(rows) != 1 {
		return errf(codeConstraint, "an update that assigns the order column %s matches %d rows; it must match exactly one", tb.Order, len(rows))
	}
	for i, x := range rows {
		x = w.next.byRow[x.row] // an earlier row's move may have re-indexed the corpus
		g := &given{row: x.row, index: i, assigned: map[string]bool{}, before: map[string]cell{}}
		w.take(g)
		for _, c := range tb.Columns {
			v, err := w.next.value(x, c.Name)
			if err != nil {
				if slices.ContainsFunc(set, func(a ir.Assign) bool { return a.Column == c.Name && a.P == nil && !a.Null }) {
					return err
				}
				continue
			}
			g.before[c.Name] = v
		}
		vals := map[string]cell{}
		for _, a := range set {
			v, msg := w.assignValue(&a, w.ent.Field(a.Column), g.before[a.Column], nil)
			if msg != "" {
				w.fault(g, tb.Name, a.Column, codeConstraint, msg)
				continue
			}
			vals[a.Column] = v
			g.assigned[a.Column] = true
		}
		if stamp && w.ent.Updated != "" && !g.assigned[w.ent.Updated] {
			vals[w.ent.Updated] = w.clock(fieldType(w.ent.Field(w.ent.Updated)))
		}
		if err := w.transition(g, x, vals); err != nil {
			return err
		}
		placement := placementColumn(tb)
		for _, c := range tb.Columns {
			v, ok := vals[c.Name]
			if !ok || c.Name == placement || c.Name == tb.Order {
				continue
			}
			setValue(tb, x.row, c.Name, v)
		}
		x = w.next.byRow[x.row]
		w.touched[x.file] = true
		if sh := tb.Storage.Shard; sh != nil && g.assigned[sh.Column] {
			if err := w.moveShard(g, x, vals[sh.Column]); err != nil {
				return err
			}
			x = w.next.byRow[x.row]
		}
		moved := false
		var oldParent *store.Row
		if x.parent != nil {
			oldParent = x.parent.row
		}
		if tb.Storage.Kind == catalog.StorageBlock {
			if v, ok := vals[placement]; ok && (v.null || v.text != x.parent.row.Key) {
				if err := w.movePlacement(g, x, v); err != nil {
					return err
				}
				moved = true
			}
		}
		if tb.Order != "" {
			k, assignsOrder := vals[tb.Order]
			parentChanged := false
			if pc := parentColumn(tb); pc != "" {
				if v, ok := vals[pc]; ok && v != g.before[pc] {
					parentChanged = true
				}
			}
			if assignsOrder || parentChanged || moved {
				var kp *cell
				if assignsOrder && !k.null {
					kp = &k
				}
				if err := w.move(g, kp, oldParent); err != nil {
					return err
				}
			}
		}
		w.next.index()
	}
	return nil
}

// moveShard moves the row x of a sharded table, whose shard column was just assigned v, to the file or the
// document of the period v names (spec 20 Time shards), when that is not where it stands. A files row moves to
// its new file; a document row leaves its document, which is removed when the row was its last, and joins the
// document of its period, created when the period has none. A row whose new value is no time stays: the
// value is a fault already.
func (w *writer) moveShard(g *given, x *rec, v cell) *Error {
	tb := x.tb
	if v.null {
		return nil
	}
	to, err := tb.RowPath(x.row.Key, v.text)
	if err != nil {
		return errf(codeInternal, "%v", err)
	}
	if to == x.file {
		return nil
	}
	if tb.Storage.Kind == catalog.StorageFiles {
		if _, exists := w.next.files[to]; exists {
			w.fault(g, tb.Name, tb.Storage.Shard.Column, codeDuplicateKey, fmt.Sprintf("the key %s already stands in %s", x.row.Key, to))
			return nil
		}
		delete(w.next.files, x.file)
		x.row.Path = to
		w.next.files[to] = &file{table: tb.Name, rows: []*store.Row{x.row}}
	} else {
		from := w.next.files[x.file]
		from.rows = slices.DeleteFunc(slices.Clone(from.rows), func(r *store.Row) bool { return r == x.row })
		if len(from.rows) == 0 {
			delete(w.next.files, x.file)
		}
		dest := w.next.files[to]
		if dest == nil {
			dest = &file{table: tb.Name}
			w.next.files[to] = dest
		}
		dest.rows = append(slices.Clone(dest.rows), x.row)
	}
	w.touched[x.file] = true
	w.touched[to] = true
	w.next.index()
	return nil
}

// movePlacement moves a block row to the scope of the row its new placement key
// names: a block row lives in the scope of its placement parent.
func (w *writer) movePlacement(g *given, x *rec, v cell) *Error {
	tb := w.tb
	p, err := w.placementRow(tb, v)
	if err != nil {
		return err
	}
	if p == nil {
		w.fault(g, tb.Name, placementColumn(tb), codeForeignKey, fmt.Sprintf("the foreign key %s names no %s row with the key %s", placementColumn(tb), tb.Storage.Parent, v.text))
		return nil
	}
	old := x.parent.row
	old.Blocks[tb.Name] = slices.DeleteFunc(slices.Clone(old.Blocks[tb.Name]), func(r *store.Row) bool { return r == x.row })
	if len(old.Blocks[tb.Name]) == 0 {
		delete(old.Blocks, tb.Name)
	}
	if p.row.Blocks == nil {
		p.row.Blocks = map[string][]*store.Row{}
	}
	p.row.Blocks[tb.Name] = append(p.row.Blocks[tb.Name], x.row)
	w.touched[x.file] = true
	w.touched[p.file] = true
	w.next.index()
	return nil
}

// delete deletes the matched rows; on a table with soft_delete it sets the soft
// delete column to the request's clock instead.
func (w *writer) delete() *Error {
	rows, err := w.matched(false)
	if err != nil {
		return err
	}
	w.answer.Affected = int64(len(rows))
	if sd := w.ent.SoftDelete; sd != "" {
		for i, x := range rows {
			g := &given{row: x.row, index: i, assigned: map[string]bool{sd: true}}
			w.take(g)
			setValue(w.tb, x.row, sd, w.clock(fieldType(w.ent.Field(sd))))
			w.touched[x.file] = true
		}
		return nil
	}
	for i, x := range rows {
		if err := w.restrict(x, i); err != nil {
			return err
		}
	}
	if len(w.faults) > 0 {
		return nil
	}
	for _, x := range rows {
		x = w.next.byRow[x.row]
		if err := w.removeRow(x); err != nil {
			return err
		}
		w.next.index()
	}
	return nil
}

// restrict refuses the delete of row x while a row references it: through a foreign
// key, its parent, the placement key of a block row in its scope, or a history row
// (spec 50 Keys and references, restrict-only). Restrict holds against the rows
// before the write, so a referencing row the same delete removes still refuses it.
// Every table that may reference the row is needed. seq is the place of x among
// the rows the delete takes.
func (w *writer) restrict(x *rec, seq int) *Error {
	for _, t := range w.c.cat.Tables {
		for _, fk := range t.ForeignKeys {
			if fk.Table != x.tb.Name || len(fk.Columns) != 1 {
				continue
			}
			if err := w.next.need(t.Name); err != nil {
				return err
			}
			for _, y := range w.next.recs[t.Name] {
				v, err := w.next.value(y, fk.Columns[0])
				if err != nil {
					return err
				}
				if v.null || v.text != x.row.Key {
					continue
				}
				w.faults = append(w.faults, fault{store.Refusal{Table: x.tb.Name, Key: x.row.Key, Index: -1, Rule: codeForeignKey,
					Message: fmt.Sprintf("the row is referenced by %s %s through %s in %s", t.Name, y.row.Key, fk.Columns[0], y.file)}, seq})
				return nil
			}
		}
	}
	return nil
}

// removeRow removes row x from its file, or the file of a files row; the siblings
// of an ordered block or document table close the gap, and those of a files table
// keep their ranks, since a gap changes no position.
func (w *writer) removeRow(x *rec) *Error {
	tb := x.tb
	if tb.Order != "" && tb.Storage.Kind != catalog.StorageFiles {
		if err := w.closeGap(x); err != nil {
			return err
		}
	}
	switch tb.Storage.Kind {
	case catalog.StorageFiles:
		delete(w.next.files, x.file)
	case catalog.StorageDocument:
		f := w.next.files[x.file]
		f.rows = slices.DeleteFunc(slices.Clone(f.rows), func(r *store.Row) bool { return r == x.row })
		if len(f.rows) == 0 && tb.Storage.Shard != nil {
			delete(w.next.files, x.file)
		}
	case catalog.StorageBlock:
		p := x.parent.row
		p.Blocks[tb.Name] = slices.DeleteFunc(slices.Clone(p.Blocks[tb.Name]), func(r *store.Row) bool { return r == x.row })
		if len(p.Blocks[tb.Name]) == 0 {
			delete(p.Blocks, tb.Name)
		}
	}
	w.touched[x.file] = true
	return nil
}

// restore clears the soft delete column of the soft-deleted row that its key names, with the new values of its set
// (orm protocol 1.5). orm's ir.RestoreKey picks the key after the request hook, as the validator did: the set values
// of the key columns become conditions after the where, whose other conditions narrow the restore as a hook narrows an
// update, and the other set values are the new values.
func (w *writer) restore() *Error {
	req := w.r.cl.req
	parts, err := ir.RestoreKey(w.ent, req)
	if err != nil {
		return fromIR(err)
	}
	req.Where = &parts.Where
	rows, xerr := w.matched(true)
	if xerr != nil {
		return xerr
	}
	set := append(slices.Clone(parts.Set), ir.Assign{Column: w.ent.SoftDelete, Null: true})
	if xerr := w.updateRows(rows, set, false); xerr != nil {
		return xerr
	}
	w.answer.Affected = int64(len(rows))
	return nil
}

// sortedTables returns the names of a set of tables in order.
func sortedTables(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// rowPath returns the corpus path of the file or the document that holds the inserted row g of tb (spec 20 Time
// shards). A row without a usable value for its shard column has no path, and "" is returned.
func (w *writer) rowPath(tb *catalog.Table, g *given, vals map[string]cell) (string, *Error) {
	value := ""
	if sh := tb.Storage.Shard; sh != nil {
		v, ok := vals[sh.Column]
		if !ok || v.null {
			// The column is non-null: a value that is no value of its type, or a missing one, is already a fault
			// of the row, and a row with a fault is placed nowhere.
			return "", nil
		}
		value = v.text
	}
	path, err := tb.RowPath(g.row.Key, value)
	if err != nil {
		// A value of the column's type is a date or a datetime, which has a period: this is a defect.
		return "", errf(codeInternal, "%v", err)
	}
	return path, nil
}
