package irexec

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/engine/ir"
	"github.com/polyspec/orm/packages/go/engine/runtimemodel"
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
	// copies maps each row of the corpus the write read to its copy in the next corpus, for the documents the write made
	// its own (ownFile).
	copies map[*store.Row]*store.Row
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
	if cl.seed {
		ent = e.loading.Entities[req.Entity]
		if cl.stated {
			ent = e.stated.Entities[req.Entity]
		}
	}
	tb, _ := c.cat.Table(ent.Table)
	if cl.stated && tb.Audit != nil {
		// The triggers of the corpus's database wrote the audit: its rows are in the same difference.
		plain := *tb
		plain.Audit = nil
		tb = &plain
	}
	w := &writer{r: r, c: c, next: c.fork(), tb: tb, ent: ent, touched: map[string]bool{}, answer: &Answer{Kind: req.Kind}}
	var err *Error
	switch {
	case cl.stated && req.Kind == "update" && len(cl.updates) > 0:
		err = w.statedUpdate()
	case cl.stated && req.Kind == "delete" && len(cl.keys) > 0:
		err = w.statedDelete()
	default:
		err = w.request()
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

// request runs the write its request states.
func (w *writer) request() *Error {
	var err *Error
	switch w.r.cl.req.Kind {
	case "insert":
		err = w.insert()
	case "update":
		err = w.update()
	case "delete":
		err = w.delete()
	case "restore":
		err = w.restore()
	}
	return err
}

// statedRows returns a function that finds the row of the next corpus that a key of a difference names. SQLite judged the
// statement, so a row the corpus does not hold is a fault of the engine.
func (w *writer) statedRows() func(ps []int) (*rec, *Error) {
	names := w.tb.KeyParts
	if len(names) == 0 {
		names = []string{w.tb.Key}
	}
	return func(ps []int) (*rec, *Error) {
		values := make([]string, len(ps))
		for i, p := range ps {
			col, _ := w.tb.ColumnNamed(names[i])
			if v, ok := decodeParam(w.r.cl.params[p], col.Type); ok {
				values[i] = v.text
			} else {
				values[i] = rawParam(w.r.cl.params[p])
			}
		}
		key := values[0]
		if len(w.tb.KeyParts) > 0 {
			key = catalog.JoinKey(values)
		}
		// The last row of the key in corpus order, as a map of the table's rows by key holds it.
		var x *rec
		if xs := w.c.lookup(keyIndex(w.tb.Name, key)); len(xs) > 0 {
			x = xs[len(xs)-1]
		}
		if x == nil {
			return nil, errf(codeInternal, "the difference of a statement names the row %s of %s, which the corpus does not hold", key, w.tb.Name)
		}
		return w.nextOf(x), nil
	}
}

// statedDelete deletes the rows of a difference, all at once: SQLite judged their foreign keys, and the rows a cascade
// took are in the same difference.
func (w *writer) statedDelete() *Error {
	find := w.statedRows()
	rows := make([]*rec, len(w.r.cl.keys))
	for i, ps := range w.r.cl.keys {
		x, err := find(ps)
		if err != nil {
			return err
		}
		rows[i] = x
	}
	if err := w.refuseImmutable(rows); err != nil {
		return err
	}
	w.answer.Affected = int64(len(rows))
	if w.tb.Storage.Kind == catalog.StorageDocument && w.tb.Order == "" {
		gone := make(map[*store.Row]bool, len(rows))
		for _, x := range rows {
			gone[x.row] = true
			w.touched[x.file] = true
		}
		for path := range w.touched {
			f := w.ownFile(path)
			f.rows = slices.DeleteFunc(slices.Clone(f.rows), func(r *store.Row) bool { return gone[r] })
			if len(f.rows) == 0 && w.tb.Storage.Shard != nil {
				w.next.dropFile(path)
			}
		}
		return nil
	}
	return w.removeBackward(rows)
}

// statedUpdate gives each row of a difference its own values.
func (w *writer) statedUpdate() *Error {
	find := w.statedRows()
	for _, u := range w.r.cl.updates {
		x, err := find(u.Key)
		if err != nil {
			return err
		}
		if err := w.refuseImmutable([]*rec{x}); err != nil {
			return err
		}
		if err := w.updateRows([]*rec{x}, u.Set, false); err != nil {
			return err
		}
	}
	w.answer.Affected = int64(len(w.r.cl.updates))
	return nil
}

// fork returns the corpus a write computes from c: it shares every document of c, and the write makes its own copy of
// a document before it changes one (ownFile), so the write copies the documents it changes, not the corpus (plan,
// Ruling: an operation costs what it touches).
func (c *Corpus) fork() *Corpus {
	c.docsMu.Lock()
	defer c.docsMu.Unlock()
	c.freshLocked()
	n := &Corpus{cat: c.cat, files: c.files, unavailable: c.unavailable, walk: c.walk, cost: c.cost, seq: c.seq}
	n.idx, n.keys = c.shareKeysLocked()
	return n
}

// ownFile returns the document at path of the next corpus as the write's own copy, which it may change, nil when the
// next corpus holds none. A document the next corpus still shares with the corpus the write read is copied with its
// rows, and the records of the next corpus that hold them take the copies, so a record the write holds stays the
// record of its row.
func (w *writer) ownFile(path string) *file {
	f := w.ownDoc(path)
	if f != nil {
		w.next.changed(path)
	}
	return f
}

// ownDoc returns the document at path of the next corpus as ownFile does, for a change of the values of its rows: the
// document is marked only when it is copied, and the change marks each row it changes (own).
func (w *writer) ownDoc(path string) *file {
	f := w.next.file(path)
	if f == nil || f != w.c.file(path) {
		return f
	}
	cp := &file{table: f.table, data: f.data, diags: f.diags, rows: make([]*store.Row, len(f.rows))}
	copies := map[*store.Row]*store.Row{}
	for i, r := range f.rows {
		cp.rows[i] = cloneRowInto(r, copies)
	}
	w.c.cost.Add(cost.RowsCopied, len(copies))
	w.next.setFile(path, cp)
	if w.copies == nil {
		w.copies = map[*store.Row]*store.Row{}
	}
	maps.Copy(w.copies, copies)
	w.next.repoint(path, copies)
	return cp
}

// own returns the record x of the next corpus after the write made its document its own (ownFile), so x.row may be
// changed.
func (w *writer) own(x *rec) *rec {
	if w.next.recOf(x.row) != x {
		panic("irexec: a write changes a record that is not of its next corpus")
	}
	w.ownDoc(x.file)
	w.next.changedRow(x.file, x.row)
	return x
}

// cloneRowInto copies the row r and its block rows as cloneRow does, recording each copy by the row it copies.
func cloneRowInto(r *store.Row, copies map[*store.Row]*store.Row) *store.Row {
	cp := cloneRow(r)
	copies[r] = cp
	var walk func(a, b *store.Row)
	walk = func(a, b *store.Row) {
		for name, rows := range a.Blocks {
			for i, child := range rows {
				copies[child] = b.Blocks[name][i]
				walk(child, b.Blocks[name][i])
			}
		}
	}
	walk(r, cp)
	return cp
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

// nextOf returns the record of the next corpus that is the corpus row x, its document made the write's own (ownFile),
// since the write asks for it before it changes it.
func (w *writer) nextOf(x *rec) *rec {
	row := x.row
	if cp, ok := w.copies[row]; ok {
		row = cp
	}
	y := w.next.recOf(row)
	if y == nil {
		return nil
	}
	return w.own(y)
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
	return w.next.value(w.next.recOf(row), col)
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
		if !ok && w.r.cl.stated {
			return cell{text: rawParam(w.r.cl.params[*idx]), raw: true}, ""
		}
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

// setColumns sets on row the value of vals of each column of tb, but its placement and order columns, which the place of
// the row holds.
func setColumns(tb *catalog.Table, row *store.Row, vals map[string]cell) {
	placement := tb.Placement()
	for _, c := range tb.Columns {
		v, ok := vals[c.Name]
		if !ok || c.Name == placement || c.Name == tb.Order {
			continue
		}
		setValue(tb, row, c.Name, v)
	}
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
	if i := tb.KeyPart(col); i >= 0 {
		values := catalog.SplitKey(row.Key)
		for len(values) < len(tb.KeyParts) {
			values = append(values, "")
		}
		values[i] = v.text
		row.Key = catalog.JoinKey(values)
		return
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
		if v.raw {
			if row.Raw == nil {
				row.Raw = map[string]string{}
			}
			row.Raw[col] = v.text
		}
	}
}

// rawParam is the text of a parameter: a JSON string's own text, or the JSON of a number.
func rawParam(raw json.RawMessage) string {
	var literal struct {
		Literal string `json:"literal"`
	}
	if json.Unmarshal(raw, &literal) == nil && literal.Literal != "" {
		return literal.Literal
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return string(raw)
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
		if f.Identity && !req.Load {
			return errf(codeIRInvalid, "cannot set identity column %s", a.Column)
		}
	}
	if req.Load && w.ent.Identity != "" && !slices.ContainsFunc(req.Set, func(a ir.Assign) bool { return a.Column == w.ent.Identity && a.P != nil }) {
		return errf(codeIRInvalid, "load of %s needs a value for the identity column %s", w.ent.Name, w.ent.Identity)
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
		if f.Null || f.Default || f.Identity || f.Name == w.tb.Order || (f.Name == w.tb.Key && w.keyPrefix() != "") || (w.tb.Audit != nil && f.Name == w.tb.Audit.Column) {
			continue
		}
		if !slices.ContainsFunc(set, func(a ir.Assign) bool { return a.Column == f.Name }) {
			return errf(codeIRInvalid, "required column %s.%s is not set", w.ent.Name, f.Name)
		}
	}
	return nil
}

// keyPrefix returns the key_prefix of the table, "" when it has none.
func (w *writer) keyPrefix() string { return w.c.keyPrefix(w.tb.Name) }

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
		case tb.Audit != nil && c.Name == tb.Audit.Column:
			v, err := w.auditKey()
			if err != nil {
				return err
			}
			vals[c.Name] = v
		case f.Identity:
			vals[c.Name] = w.nextIdentity(tb, c.Name)
		case c.Name == tb.Key && w.keyPrefix() != "":
			vals[c.Name] = w.nextPrefixed(tb)
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
	joinKey(tb, vals)
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
	setColumns(tb, g.row, vals)
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
		if exists := w.next.file(path) != nil; exists {
			// The key equals an existing row's key byte for byte: the key check
			// reports it; the row is placed nowhere.
			return nil
		}
		w.next.setFile(path, &file{table: tb.Name, rows: []*store.Row{g.row}})
		w.touched[path] = true
	case catalog.StorageDocument:
		path, perr := w.rowPath(tb, g, vals)
		if perr != nil {
			return perr
		}
		if path == "" {
			return nil
		}
		f := w.ownFile(path)
		if f == nil {
			f = &file{table: tb.Name}
			w.next.setFile(path, f)
		}
		f.rows = append(f.rows, g.row)
		w.touched[path] = true
	case catalog.StorageBlock:
		pv := vals[tb.Placement()]
		p, err := w.placementRow(tb, pv)
		if err != nil {
			return err
		}
		if p == nil {
			w.fault(g, tb.Name, tb.Placement(), codeForeignKey, fmt.Sprintf("the foreign key %s names no %s row with the key %s", tb.Placement(), tb.Storage.Parent, pv.text))
			return nil
		}
		p = w.own(p)
		if p.row.Blocks == nil {
			p.row.Blocks = map[string][]*store.Row{}
		}
		p.row.Blocks[tb.Name] = append(p.row.Blocks[tb.Name], g.row)
		w.next.changed(p.file)
		w.touched[p.file] = true
	}
	if tb.Order != "" {
		var k *cell
		if v, ok := vals[tb.Order]; ok && !v.null {
			k = &v
		}
		if err := w.placeInOrder(g, k); err != nil {
			return err
		}
	}
	if err := w.auditRow(g.row, "insert", cell{null: true}); err != nil {
		return err
	}
	key0, err := w.valueOf(g.row, tb.Key)
	if err != nil {
		return err
	}
	return w.answerKey(key0)
}

// answerKey appends the key k of a written row to the answer's keys.
func (w *writer) answerKey(k cell) *Error {
	o, err := output(w.tb.KeyColumn().Type, k)
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
	return w.next.lookupOne(keyIndex(tb.Storage.Parent, v.text)), nil
}

// nextIdentity allocates max+1 over the identity column of the table's rows: the largest positive integer of the column,
// which the number index holds (numberColumns).
func (w *writer) nextIdentity(tb *catalog.Table, col string) cell {
	if !slices.Contains(numberColumns(tb), col) {
		panic("irexec: an identity is allocated over the column " + col + " of " + tb.Name + ", whose numbers the index does not hold")
	}
	max := int64(0)
	if col == tb.Key && tb.Autoincrement {
		return cell{text: strconv.FormatInt(w.next.Sequence(tb.Name)+1, 10)}
	}
	if n := w.next.maxNumber(tb.Name, col); n != "" {
		// The index holds the canonical digits of a positive int64 (indexKeys).
		m, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			panic("irexec: the number index holds " + n + ", which is no int64")
		}
		max = m
	}
	return cell{text: strconv.FormatInt(max+1, 10)}
}

// nextPrefixed allocates <p><n + 1> under the key_prefix p of tb, n the largest decimal suffix without a leading zero
// among the keys of the form <p><n> (spec 20 Keys): the last number of the prefix the index holds.
func (w *writer) nextPrefixed(tb *catalog.Table) cell {
	p := w.next.keyPrefix(tb.Name)
	var keys []string
	if n := w.next.maxNumber(tb.Name, ""); n != "" {
		keys = []string{p + n}
	}
	return cell{text: NextPrefixed(p, keys)}
}

// NextPrefixed is the key that key_prefix p allocates after keys: <p><n + 1>, where n is the largest decimal suffix
// without a leading zero among the keys of the form <p><n> (spec 20 "Keys").
func NextPrefixed(p string, keys []string) string {
	max := new(big.Int)
	for _, k := range keys {
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
	return p + new(big.Int).Add(max, big.NewInt(1)).String()
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
	joinKey(w.tb, vals)
	if k, ok := vals[w.tb.Key]; ok && !k.null {
		for _, x := range w.next.lookup(foldIndex(w.tb.Name, k.text)) {
			switch {
			case x.row.Key == k.text:
				met = x
			case met != nil && met.row.Key == k.text:
			case store.FoldKey(x.row.Key) == store.FoldKey(k.text) && (met == nil || x.row.Key < met.row.Key):
				met = x
			}
		}
	}
	for _, x := range w.uniqueCandidates(vals) {
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
	key, err := w.valueOf(met.row, w.tb.Key)
	if err != nil {
		return false, err
	}
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
	if err := w.refuseImmutable(rows); err != nil {
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
		x = w.own(w.next.recOf(x.row)) // an earlier row's move may have re-indexed the corpus
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
		if tb.Audit != nil {
			v, err := w.auditKey()
			if err != nil {
				return err
			}
			vals[tb.Audit.Column] = v
			g.assigned[tb.Audit.Column] = true
		}
		if stamp && w.ent.Updated != "" && !g.assigned[w.ent.Updated] {
			vals[w.ent.Updated] = w.clock(fieldType(w.ent.Field(w.ent.Updated)))
		}
		if err := w.transition(g, x, vals); err != nil {
			return err
		}
		x = w.own(x)
		setColumns(tb, x.row, vals)
		x = w.next.recOf(x.row)
		w.touched[x.file] = true
		if sh := tb.Storage.Shard; sh != nil && g.assigned[sh.Column] {
			if err := w.moveShard(g, x, vals[sh.Column]); err != nil {
				return err
			}
			x = w.next.recOf(x.row)
		}
		moved := false
		var oldParent *store.Row
		if x.parent != nil {
			oldParent = x.parent.row
		}
		if tb.Storage.Kind == catalog.StorageBlock {
			if v, ok := vals[tb.Placement()]; ok && (v.null || v.text != x.parent.row.Key) {
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
		if tb.Audit != nil {
			if err := w.auditRow(x.row, "update", g.before[tb.Audit.Column]); err != nil {
				return err
			}
		}
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
		if exists := w.next.file(to) != nil; exists {
			w.fault(g, tb.Name, tb.Storage.Shard.Column, codeDuplicateKey, fmt.Sprintf("the key %s already stands in %s", x.row.Key, to))
			return nil
		}
		w.next.dropFile(x.file)
		x.row.Path = to
		w.next.setFile(to, &file{table: tb.Name, rows: []*store.Row{x.row}})
	} else {
		from := w.ownFile(x.file)
		from.rows = slices.DeleteFunc(slices.Clone(from.rows), func(r *store.Row) bool { return r == x.row })
		if len(from.rows) == 0 {
			w.next.dropFile(x.file)
		}
		dest := w.ownFile(to)
		if dest == nil {
			dest = &file{table: tb.Name}
			w.next.setFile(to, dest)
		}
		dest.rows = append(slices.Clone(dest.rows), x.row)
	}
	w.touched[x.file] = true
	w.touched[to] = true
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
		w.fault(g, tb.Name, tb.Placement(), codeForeignKey, fmt.Sprintf("the foreign key %s names no %s row with the key %s", tb.Placement(), tb.Storage.Parent, v.text))
		return nil
	}
	old := w.own(x.parent).row
	old.Blocks[tb.Name] = slices.DeleteFunc(slices.Clone(old.Blocks[tb.Name]), func(r *store.Row) bool { return r == x.row })
	w.next.changed(x.file)
	if len(old.Blocks[tb.Name]) == 0 {
		delete(old.Blocks, tb.Name)
	}
	p = w.own(p)
	if p.row.Blocks == nil {
		p.row.Blocks = map[string][]*store.Row{}
	}
	p.row.Blocks[tb.Name] = append(p.row.Blocks[tb.Name], x.row)
	w.next.changed(p.file)
	w.touched[x.file] = true
	w.touched[p.file] = true
	return nil
}

// delete deletes the matched rows; on a table with soft_delete it sets the soft
// delete column to the request's clock instead.
func (w *writer) delete() *Error {
	rows, err := w.matched(false)
	if err != nil {
		return err
	}
	if err := w.refuseImmutable(rows); err != nil {
		return err
	}
	w.answer.Affected = int64(len(rows))
	if sd := w.ent.SoftDelete; sd != "" && !w.r.cl.seed {
		for i, x := range rows {
			g := &given{row: x.row, index: i, assigned: map[string]bool{sd: true}}
			w.take(g)
			x = w.own(x)
			setValue(w.tb, x.row, sd, w.clock(fieldType(w.ent.Field(sd))))
			w.touched[x.file] = true
			if a := w.tb.Audit; a != nil {
				key, err := w.auditKey()
				if err != nil {
					return err
				}
				previous, err := w.next.value(x, a.Column)
				if err != nil {
					return err
				}
				g.assigned[a.Column] = true
				x = w.own(x)
				setValue(w.tb, x.row, a.Column, key)
				if err := w.auditRow(x.row, "update", previous); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if w.tb.Audit != nil && len(rows) > 0 && !w.r.cl.seed {
		return errf(codeConstraint, "table %s is audited: a row is deleted through its soft delete column", w.tb.Name)
	}
	gone, err := w.cascaded(rows)
	if err != nil {
		return err
	}
	for i, x := range gone {
		if err := w.restrict(x, i, gone); err != nil {
			return err
		}
	}
	if len(w.faults) > 0 {
		return nil
	}
	// A referencing row goes before the row it references, so that a block row still has its parent.
	return w.removeBackward(gone)
}

// removeBackward removes rows from the last to the first, each as the next tree holds it: a row that an earlier removal
// took with its block or its file is gone already.
func (w *writer) removeBackward(rows []*rec) *Error {
	for _, x := range slices.Backward(rows) {
		x = w.next.recOf(x.row)
		if x == nil {
			continue // it stood in a block or a file that an earlier removal took
		}
		if err := w.removeRow(x); err != nil {
			return err
		}
	}
	return nil
}

// refuseImmutable refuses a change of the matched rows of an immutable table (spec 50 Immutable).
func (w *writer) refuseImmutable(rows []*rec) *Error {
	if w.tb.Immutable && len(rows) > 0 {
		return errf(codeConstraint, "table %s is immutable", w.tb.Name)
	}
	return nil
}

// joinKey sets the value of the key cell of a composite key from the values of its columns: absent when one has no value
// yet, NULL when one is NULL.
func joinKey(tb *catalog.Table, vals map[string]cell) {
	if len(tb.KeyParts) == 0 {
		return
	}
	values := make([]string, len(tb.KeyParts))
	for i, name := range tb.KeyParts {
		v, ok := vals[name]
		if !ok {
			return
		}
		if v.null {
			vals[tb.Key] = cell{null: true}
			return
		}
		values[i] = v.text
	}
	vals[tb.Key] = cell{text: catalog.JoinKey(values)}
}

// referencedKey is the key of the row the foreign key fk of row y names in the table target, false when one of its
// columns is NULL, since a NULL names no row.
func (c *Corpus) referencedKey(y *rec, fk dbspec.ForeignKey, target *catalog.Table) (string, bool, *Error) {
	if len(fk.Columns) == 1 {
		v, err := c.value(y, fk.Columns[0])
		if err != nil || v.null {
			return "", false, err
		}
		return v.text, true, nil
	}
	values := make([]string, len(target.KeyParts))
	for i, name := range target.KeyParts {
		at := slices.Index(fk.References, name)
		if at < 0 {
			return "", false, errf(codeInternal, "foreign key %s does not reference %s", fk.Name, name)
		}
		v, err := c.value(y, fk.Columns[at])
		if err != nil || v.null {
			return "", false, err
		}
		values[i] = v.text
	}
	return catalog.JoinKey(values), true, nil
}

// referencing calls visit for each row whose foreign key names the row x, the rows of every table that may reference it.
func (w *writer) referencing(x *rec, visit func(y *rec, t *catalog.Table, fk dbspec.ForeignKey) *Error) *Error {
	for _, t := range w.c.cat.Tables {
		for i, fk := range t.ForeignKeys {
			if fk.Table != x.tb.Name {
				continue
			}
			if err := w.next.need(t.Name); err != nil {
				return err
			}
			for _, y := range w.next.candidates(refIndex(t.Name, i, x.row.Key), refFault(t.Name, i)) {
				key, named, err := w.next.referencedKey(y, fk, x.tb)
				if err != nil {
					return err
				}
				if !named || key != x.row.Key {
					continue
				}
				if err := visit(y, t, fk); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// cascaded is the rows a delete of rows removes: those rows, then each row that an `on delete cascade` foreign key takes
// with them, level by level (spec 50 Referential actions).
func (w *writer) cascaded(rows []*rec) ([]*rec, *Error) {
	gone := slices.Clone(rows)
	seen := map[*store.Row]bool{}
	for _, x := range gone {
		seen[x.row] = true
	}
	for i := 0; i < len(gone); i++ {
		err := w.referencing(gone[i], func(y *rec, _ *catalog.Table, fk dbspec.ForeignKey) *Error {
			if fk.OnDelete == dbspec.ActionCascade && !seen[y.row] {
				seen[y.row] = true
				gone = append(gone, y)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return gone, nil
}

// restrict refuses the delete of row x while a row outside an action references it: through a restrict foreign key, its
// parent, the placement key of a block row in its scope, or a history row (spec 50 Keys and references). An `on delete
// set_null` foreign key instead clears its column in the referencing row, unless that row goes too. Restrict holds against
// the rows before the write, so a referencing row the same delete removes still refuses it, unless a cascade takes it.
// seq is the place of x among the rows the delete takes.
func (w *writer) restrict(x *rec, seq int, gone []*rec) *Error {
	if w.r.cl.noForeignKeys {
		return nil
	}
	return w.referencing(x, func(y *rec, t *catalog.Table, fk dbspec.ForeignKey) *Error {
		taken := slices.ContainsFunc(gone, func(g *rec) bool { return g.row == y.row })
		switch fk.OnDelete {
		case dbspec.ActionCascade:
			return nil
		case dbspec.ActionSetNull:
			if !taken {
				y = w.own(y)
				for _, col := range fk.Columns {
					setValue(t, y.row, col, cell{null: true})
				}
				w.touched[y.file] = true
			}
			return nil
		}
		for _, f := range w.faults {
			if f.Key == x.row.Key && f.Table == x.tb.Name {
				return nil // the first referencing row names the refusal
			}
		}
		w.faults = append(w.faults, fault{store.Refusal{Table: x.tb.Name, Key: x.row.Key, Index: -1, Rule: codeForeignKey,
			Message: fmt.Sprintf("the row is referenced by %s %s through %s in %s", t.Name, y.row.Key, fk.Columns[0], y.file)}, seq})
		return nil
	})
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
		w.next.dropFile(x.file)
	case catalog.StorageDocument:
		f := w.ownFile(x.file)
		f.rows = slices.DeleteFunc(slices.Clone(f.rows), func(r *store.Row) bool { return r == x.row })
		if len(f.rows) == 0 && tb.Storage.Shard != nil {
			w.next.dropFile(x.file)
		}
	case catalog.StorageBlock:
		p := w.own(x.parent).row
		p.Blocks[tb.Name] = slices.DeleteFunc(slices.Clone(p.Blocks[tb.Name]), func(r *store.Row) bool { return r == x.row })
		w.next.changed(x.file)
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

// uniqueCandidates returns the rows of the table the on_duplicate judgement of the inserted values vals may meet or fail
// on, in corpus order: for each unique key, the rows that hold its values and the rows whose values of it fail their
// evaluation; every row of the table when the index does not hold one of its unique keys (indexedUnique).
func (w *writer) uniqueCandidates(vals map[string]cell) []*rec {
	var keys []string
	for _, u := range w.tb.Uniques {
		if !indexedUnique(w.tb, u) {
			return scan(w.next, w.tb.Name)
		}
		keys = append(keys, uniqueFault(w.tb.Name, u))
		parts := make([]string, len(u))
		complete := true
		for i, col := range u {
			v, ok := vals[col]
			if !ok || v.null {
				complete = false
				break
			}
			parts[i] = v.text
		}
		if complete {
			keys = append(keys, uniqueIndex(w.tb.Name, u, strings.Join(parts, "\x00")))
		}
	}
	return w.next.candidates(keys...)
}
