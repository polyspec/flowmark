package irexec

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
)

// PostImage is what a write changes: the files to write and to remove, and the
// corpus as it reads after the write. The executor writes no file; the write path
// commits the targets under the lock and the journal (spec 40 The commit protocol)
// — checking first that each target still holds its Before bytes, and refusing a
// target whose Diagnostics hold a format.* diagnostic unless an exemption applies —
// and a request later in the same transaction reads Next.
type PostImage struct {
	// Targets are sorted by path.
	Targets []Target
	// Next is the corpus with the post-image applied: the staged corpus of a
	// transaction.
	Next *Corpus
}

// Target is one file a write changes. Before is nil for a file the corpus did not
// hold (an insert that creates a folder's first file or a table's document), and
// After is nil for a file the write removes (a delete that removes a files row's
// whole file). Diagnostics are the read diagnostics of the file before the write.
type Target struct {
	Path        string
	Before      []byte
	After       []byte
	Diagnostics []catalog.Diagnostic
}

// finish checks the write and renders its post-image (spec 10 Diagnostics, "Write
// faults"). The key checks, unique keys, references, parents, values, checks and
// limits run on the request against the corpus, and no file renders for them. Their
// faults are reported with the rows in request order and, within a row, its value
// faults before its key fault. A write with a key or value fault renders no file;
// otherwise every changed file is rendered, and the rows a file is rendered from are
// read back from its bytes, its render faults reported next. FOREIGN_KEY is
// reported only when no other fault stands, as SQL checks a foreign key at the end
// of a statement. The code is that of the first fault.
func (w *writer) finish() (*PostImage, *Error) {
	keyFaults := w.keyChecks()
	for _, step := range []func() *Error{w.uniques, w.references, w.parents, w.values, w.checks, w.rowRules, w.limits} {
		if err := step(); err != nil {
			return nil, err
		}
	}
	var stand, fks []fault
	for _, f := range append(keyFaults, w.faults...) {
		if f.Rule == codeForeignKey {
			fks = append(fks, f)
		} else {
			stand = append(stand, f)
		}
	}
	keyLast := func(f fault) int {
		if f.Rule == codeDuplicateKey {
			return 1
		}
		return 0
	}
	sort.SliceStable(stand, func(i, j int) bool {
		a, b := stand[i], stand[j]
		if a.seq != b.seq {
			return a.seq < b.seq
		}
		return keyLast(a) < keyLast(b)
	})
	if len(stand) > 0 {
		return nil, faultError(refusals(stand))
	}
	post, renderFaults, err := w.render()
	if err != nil {
		return nil, err
	}
	var others []store.Refusal
	for _, r := range renderFaults {
		switch {
		case r.Rule != codeForeignKey:
			others = append(others, r)
		case !slices.ContainsFunc(fks, func(f fault) bool { return f.Table == r.Table && f.Key == r.Key && f.Column == r.Column }):
			fks = append(fks, fault{r, len(w.given)})
		}
	}
	if len(others) > 0 {
		return nil, faultError(others)
	}
	if len(fks) > 0 {
		sort.SliceStable(fks, func(i, j int) bool { return fks[i].seq < fks[j].seq })
		return nil, faultError(refusals(fks))
	}
	return post, nil
}

// refusals returns the refusals of faults, in their order.
func refusals(faults []fault) []store.Refusal {
	out := make([]store.Refusal, len(faults))
	for i, f := range faults {
		out[i] = f.Refusal
	}
	return out
}

// values refuses a written value that no file can hold, without rendering one: a
// title that is empty, not UTF-8 or breaks the single-line rule, a stored scalar
// that is not UTF-8 or breaks the single-line rule, and NULL in a non-null column
// (spec 10 Single-line places, spec 50 Readable-document refusals). Each is the
// same refusal the codec makes of the row; the codec's own refusals of the rendered
// file follow only when no such fault stands.
func (w *writer) values() *Error {
	tb := w.tb
	placement := tb.Placement()
	for _, g := range w.given {
		// A key is written only by an insert, since a key is never updated; its shape
		// is a value of the row, so its fault precedes the row's key fault.
		if g.inserted && g.row.Key != "" {
			if msg := store.KeyFault(g.row.Key); msg != "" {
				w.fault(g, tb.Name, tb.Key, codeConstraint, "the key: "+msg)
			}
		}
		for _, c := range tb.Columns {
			col := c.Name
			if !g.inserted && !g.assigned[col] {
				continue
			}
			if col == tb.Key || col == placement || col == tb.Order {
				continue
			}
			switch {
			case col == tb.Title:
				t := g.row.Title
				switch {
				case t == "":
					w.fault(g, tb.Name, col, codeConstraint, "the title is empty")
				case !utf8.ValidString(t):
					w.fault(g, tb.Name, col, codeConstraint, "the title is not UTF-8")
				case store.SingleLineFault(t) != "":
					w.fault(g, tb.Name, col, codeConstraint, "the title has "+store.SingleLineFault(t))
				}
			case col == tb.Body || isMarkdown(tb, col):
				if !c.Null && col != tb.Body && g.assigned[col] && g.row.Sections[col] == nil {
					w.fault(g, tb.Name, col, codeConstraint, "the non-null section "+col+" has no value")
				}
			default:
				v := g.row.Scalars[col]
				switch {
				case v == nil:
					if !c.Null && g.assigned[col] {
						w.fault(g, tb.Name, col, codeConstraint, "the non-null column "+col+" has no value")
					}
				case !utf8.ValidString(*v):
					w.fault(g, tb.Name, col, codeConstraint, col+" is not UTF-8")
				case store.SingleLineFault(*v) != "":
					w.fault(g, tb.Name, col, codeConstraint, col+" has "+store.SingleLineFault(*v))
				}
			}
		}
	}
	return nil
}

// keyChecks runs both key checks of the rows the write inserts, byte for byte and
// without regard to case, within the request and against the table's other rows
// (spec 10 Diagnostics, "Write faults"). A row has at most one duplicate fault, at
// its key column. A row that shares its key byte for byte with a row given before it,
// or else with a row of the table the request does not give, names the first such
// row given, or else that row of the table. Otherwise, of the rows whose keys equal
// its key only without regard to case and whose pair places the fault at it — the
// row whose key the write introduces, or of two rows the write introduces the one
// whose key is greater in byte order — it names the one whose key is least in byte
// order, the first given when several given rows hold that key. A row whose key
// another given row shares byte for byte carries its index.
func (w *writer) keyChecks() []fault {
	var ins []*given
	for _, g := range w.given {
		if g.inserted {
			ins = append(ins, g)
		}
	}
	if len(ins) == 0 {
		return nil
	}
	// The table's rows before the write: the rows the request does not give.
	others := w.c.recs[w.tb.Name]
	var faults []fault
	for i, g := range ins {
		k := g.row.Key
		idx := -1
		for j, o := range ins {
			if j != i && o.row.Key == k {
				idx = g.index
			}
		}
		refuse := func(msg string) {
			faults = append(faults, fault{store.Refusal{Table: w.tb.Name, Key: k, Index: idx, Column: w.tb.Key, Rule: codeDuplicateKey, Message: msg}, g.seq})
			w.duplicated(g)
		}
		byteGiven := -1
		for j := 0; j < i; j++ {
			if ins[j].row.Key == k {
				byteGiven = j
				break
			}
		}
		if byteGiven >= 0 {
			refuse(fmt.Sprintf("the key %s equals the key of the row at index %d of the rows given", k, ins[byteGiven].index))
			continue
		}
		if x := firstByKey(others, func(o string) bool { return o == k }); x != nil {
			refuse(fmt.Sprintf("the key %s equals the key of the %s row in %s", k, w.tb.Name, x.file))
			continue
		}
		// The case-only pairs that place the fault at this row: every row of the
		// table, and every given row whose key is less in byte order.
		var least string
		var leastGiven *given
		var leastRow *rec
		consider := func(o string) bool {
			return o != k && store.FoldKey(o) == store.FoldKey(k) && (least == "" || o < least)
		}
		for _, o := range ins {
			if o.row.Key < k && consider(o.row.Key) {
				least, leastGiven, leastRow = o.row.Key, o, nil
			}
		}
		for _, x := range others {
			if consider(x.row.Key) {
				least, leastGiven, leastRow = x.row.Key, nil, x
			}
		}
		switch {
		case leastGiven != nil:
			refuse(fmt.Sprintf("the key %s equals the key %s of the row at index %d of the rows given without regard to case", k, least, leastGiven.index))
		case leastRow != nil:
			refuse(fmt.Sprintf("the key %s equals the key %s of the %s row in %s without regard to case", k, least, w.tb.Name, leastRow.file))
		}
	}
	return faults
}

// firstByKey returns the first row whose key match accepts, in file order.
func firstByKey(rows []*rec, match func(string) bool) *rec {
	for _, x := range rows {
		if match(x.row.Key) {
			return x
		}
	}
	return nil
}

// uniques refuses a written row whose unique key equals another row's, byte for
// byte in canonical form; a NULL in the key takes no part (spec 50 Keys). A value
// of another row not of its type is evaluated by the check, so it fails the write.
// A row has at most one duplicate fault (spec 10 Diagnostics, "Write faults"): a row
// the key checks fault, or one a unique key faulted, still holds its unique keys for
// the rows after it but takes no further fault. A row placed nowhere — its placement
// key names no row — is read from its own values.
func (w *writer) uniques() *Error {
	for _, u := range w.tb.Uniques {
		seen := map[string]*given{}
		for _, g := range w.given {
			if !g.inserted && !assignsAny(g, u) {
				continue
			}
			x := w.next.byRow[g.row]
			if x == nil {
				x = &rec{row: g.row, tb: w.tb}
			}
			k, ok, err := w.uniqueText(x, u)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if w.dupFaulted[g] {
				if seen[k] == nil {
					seen[k] = g
				}
				continue
			}
			if first := seen[k]; first != nil {
				w.fault(g, w.tb.Name, u[0], codeDuplicateKey, fmt.Sprintf("the unique key (%s) of %s equals that of the row %s given before it", names(u), g.row.Key, first.row.Key))
				continue
			}
			seen[k] = g
			for _, x := range w.next.recs[w.tb.Name] {
				if x.row == g.row || isGiven(w.given, x.row) {
					continue
				}
				xk, ok, err := w.uniqueText(x, u)
				if err != nil {
					return err
				}
				if ok && xk == k {
					w.fault(g, w.tb.Name, u[0], codeDuplicateKey, fmt.Sprintf("the unique key (%s) of %s equals that of %s %s in %s", names(u), g.row.Key, w.tb.Name, x.row.Key, x.file))
					break
				}
			}
		}
	}
	return nil
}

// uniqueText returns the values of the unique key u of row x joined, false when one
// is NULL.
func (w *writer) uniqueText(x *rec, u []string) (string, bool, *Error) {
	parts := make([]string, len(u))
	for i, col := range u {
		if x.parent == nil && w.tb.Storage.Kind == catalog.StorageBlock && col == w.tb.Placement() {
			return "", false, nil // a block row placed nowhere: its placement key names no row
		}
		v, err := w.next.value(x, col)
		if err != nil {
			return "", false, err
		}
		if v.null {
			return "", false, nil
		}
		parts[i] = v.text
	}
	return strings.Join(parts, "\x00"), true, nil
}

func assignsAny(g *given, cols []string) bool {
	for _, c := range cols {
		if g.assigned[c] {
			return true
		}
	}
	return false
}

func isGiven(gs []*given, r *store.Row) bool {
	for _, g := range gs {
		if g.row == r {
			return true
		}
	}
	return false
}

// references refuses a written foreign key that names no row of its table byte for
// byte: FOREIGN_KEY at its column (spec 50 Keys and references). The referenced
// table is needed. The placement key of a block row was checked as the row was
// placed.
func (w *writer) references() *Error {
	placement := w.tb.Placement()
	for _, g := range w.given {
		x := w.next.byRow[g.row]
		if x == nil {
			continue
		}
		for _, fk := range w.tb.ForeignKeys {
			col := fk.Columns[0]
			if col == placement || (!g.inserted && !g.assigned[col]) {
				continue
			}
			v, err := w.next.value(x, col)
			if err != nil {
				return err
			}
			if v.null {
				continue
			}
			if err := w.next.need(fk.Table); err != nil {
				return err
			}
			if firstByKey(w.next.recs[fk.Table], func(k string) bool { return k == v.text }) == nil {
				w.fault(g, w.tb.Name, col, codeForeignKey, fmt.Sprintf("the foreign key %s names no %s row with the key %s", col, fk.Table, v.text))
			}
		}
	}
	return nil
}

// parents refuses a written parent outside the row's block and a parent cycle: a
// block or document row's parent is a row of its own block (spec 20 Hierarchy), and
// a cycle is a fault at each written row on it, at column parent.
func (w *writer) parents() *Error {
	pc := parentColumn(w.tb)
	if pc == "" {
		return nil
	}
	for _, g := range w.given {
		x := w.next.byRow[g.row]
		if x == nil || (!g.inserted && !g.assigned[pc]) {
			continue
		}
		v, err := w.next.value(x, pc)
		if err != nil {
			return err
		}
		if v.null {
			continue
		}
		p := firstByKey(w.next.recs[w.tb.Name], func(k string) bool { return k == v.text })
		if p == nil {
			continue // the reference check reports it
		}
		if w.tb.Storage.Kind != catalog.StorageFiles && (p.file != x.file || p.parent != x.parent) {
			w.fault(g, w.tb.Name, pc, codeConstraint, fmt.Sprintf("the parent %s of %s is not a row of its block", v.text, g.row.Key))
			continue
		}
		seen := map[*rec]bool{x: true}
		for cur := p; cur != nil; {
			if cur == x {
				w.fault(g, w.tb.Name, pc, codeConstraint, fmt.Sprintf("the parent %s of %s makes a cycle of parents", v.text, g.row.Key))
				break
			}
			if seen[cur] {
				break
			}
			seen[cur] = true
			cv, err := w.next.value(cur, pc)
			if err != nil {
				return err
			}
			if cv.null {
				break
			}
			cur = firstByKey(w.next.recs[w.tb.Name], func(k string) bool { return k == cv.text })
		}
	}
	return nil
}

// linker derives link targets over a corpus, as the validator's index does: the
// file that holds a row and the anchor of its detail section.
type linker struct {
	c       *Corpus
	targets map[string]linkTarget
}

type linkTarget struct {
	file, anchor string
	shared       bool
}

func newLinker(c *Corpus) (*linker, error) {
	lk := &linker{c: c, targets: map[string]linkTarget{}}
	for _, tb := range c.cat.Tables {
		for _, x := range c.recs[tb.Name] {
			k := tb.Name + "\x00" + x.row.Key
			if t, ok := lk.targets[k]; ok {
				t.shared = true
				lk.targets[k] = t
				continue
			}
			has, err := store.HasDetail(c.cat, tb.Name, x.row)
			if err != nil {
				return nil, err
			}
			t := linkTarget{file: x.file}
			if has {
				t.anchor = store.DetailAnchor(tb.Name, x.row.Key)
			}
			lk.targets[k] = t
		}
	}
	return lk, nil
}

// Target implements store.Linker.
func (lk *linker) Target(fromFile, table, key string) (string, error) {
	t, ok := lk.targets[table+"\x00"+key]
	if !ok {
		return "", fmt.Errorf("no %s row has the key %s", table, key)
	}
	if t.shared {
		return "", store.ErrKeyShared
	}
	return store.RelLink(fromFile, t.file, t.anchor)
}

// render renders every file the write changed, and every file holding a link to a
// row whose detail section appears or disappears, whose link target the write
// moves. A file is a target when its bytes change. The rows of a rendered file are
// read back from its bytes into the next corpus; the refusals of the codec are the
// write's faults and are returned with no post-image, and a DUPLICATE_KEY from the
// codec is internal, since the key checks ran before.
func (w *writer) render() (*PostImage, []store.Refusal, *Error) {
	before, err := newLinker(w.c)
	if err != nil {
		return nil, nil, errf(codeInternal, "%v", err)
	}
	w.next.index()
	after, err := newLinker(w.next)
	if err != nil {
		return nil, nil, errf(codeInternal, "%v", err)
	}
	if err := w.linkTargets(before, after); err != nil {
		return nil, nil, err
	}
	paths := sortedTables(w.touched)
	for _, path := range paths {
		if f := w.next.files[path]; f != nil {
			if err := w.needLinks(f); err != nil {
				return nil, nil, err
			}
		}
	}
	post := &PostImage{Next: w.next}
	var faults []store.Refusal
	for _, path := range paths {
		f := w.next.files[path]
		var prev []byte
		var diags []catalog.Diagnostic
		if old := w.c.files[path]; old != nil {
			prev, diags = old.data, old.diags
		}
		if f == nil {
			if prev != nil {
				post.Targets = append(post.Targets, Target{Path: path, Before: prev, Diagnostics: diags})
			}
			continue
		}
		tb, _ := w.c.cat.Table(f.table)
		var out []byte
		var rf []store.Refusal
		var rerr error
		if tb.Storage.Kind == catalog.StorageFiles {
			out, rf, rerr = store.RenderFile(w.c.cat, f.table, path, f.rows[0], after)
		} else {
			out, rf, rerr = store.RenderDocument(w.c.cat, f.table, path, f.rows, after)
		}
		if rerr != nil {
			return nil, nil, errf(codeInternal, "rendering %s: %v", path, rerr)
		}
		for _, r := range rf {
			if r.Rule == codeDuplicateKey {
				return nil, nil, errf(codeInternal, "the codec refused a duplicate key that the key checks passed: %s %s: %s", r.Table, r.Key, r.Message)
			}
		}
		if len(rf) > 0 {
			faults = append(faults, rf...)
			continue
		}
		if err := w.readBack(path, f, out, after); err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(out, prev) || prev == nil {
			post.Targets = append(post.Targets, Target{Path: path, Before: prev, After: out, Diagnostics: diags})
		}
	}
	if len(faults) > 0 {
		return nil, faults, nil
	}
	w.next.index()
	return post, nil, nil
}

// readBack replaces the rows of the rendered file path with the rows read from its
// bytes, so the next corpus holds what a read of the file would. A rendered file
// that does not read back clean is internal.
func (w *writer) readBack(path string, f *file, out []byte, lk store.Linker) *Error {
	tb, _ := w.c.cat.Table(f.table)
	var rows []*store.Row
	var diags []catalog.Diagnostic
	if tb.Storage.Kind == catalog.StorageFiles {
		row, ds := store.ParseFile(w.c.cat, f.table, path, out, lk)
		if row != nil {
			rows = []*store.Row{row}
		}
		diags = ds
	} else {
		rs, ds, err := store.ParseDocument(w.c.cat, f.table, path, out, lk)
		if err != nil {
			return errf(codeInternal, "reading back %s: %v", path, err)
		}
		rows, diags = rs, ds
	}
	if len(diags) > 0 {
		d := diags[0]
		return errf(codeInternal, "the rendered %s does not read back clean: %d:%d %s: %s", path, d.Line, d.Column, d.Rule, d.Message)
	}
	w.next.files[path] = &file{table: f.table, data: out, rows: rows}
	return nil
}

// linkTargets adds to the changed files every file that holds a link to a row whose
// derived target the write changes: a row whose detail section appears or
// disappears, and a row the write inserts, which a dangling link may name (spec 10
// Foreign keys as links). Every table that may hold such a link is needed.
func (w *writer) linkTargets(before, after *linker) *Error {
	moved := map[string]bool{}
	for k, a := range after.targets {
		if b, ok := before.targets[k]; !ok || b.anchor != a.anchor || b.file != a.file {
			moved[k] = true
		}
	}
	if len(moved) == 0 {
		return nil
	}
	for _, tb := range w.c.cat.Tables {
		placement := tb.Placement()
		for _, fk := range tb.ForeignKeys {
			col := fk.Columns[0]
			if col == placement {
				continue
			}
			links := false
			for k := range moved {
				if strings.HasPrefix(k, fk.Table+"\x00") {
					links = true
				}
			}
			if !links {
				continue
			}
			if err := w.next.need(tb.Name); err != nil {
				return err
			}
			for _, x := range w.next.recs[tb.Name] {
				v, err := w.next.value(x, col)
				if err != nil || v.null {
					continue
				}
				if moved[fk.Table+"\x00"+v.text] {
					w.touched[x.file] = true
				}
			}
		}
	}
	return nil
}

// needLinks needs every table that a link of file f names: a link's target is
// derived from the rows of the table it names.
func (w *writer) needLinks(f *file) *Error {
	tables := map[string]bool{}
	var visit func(r *store.Row)
	visit = func(r *store.Row) {
		tb, _ := w.c.cat.Table(r.Table)
		placement := tb.Placement()
		for _, fk := range tb.ForeignKeys {
			col := fk.Columns[0]
			if col == placement {
				continue
			}
			if v := r.Scalars[col]; v != nil {
				tables[fk.Table] = true
			}
		}
		names := make([]string, 0, len(r.Blocks))
		for n := range r.Blocks {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			for _, b := range r.Blocks[n] {
				visit(b)
			}
		}
	}
	for _, r := range f.rows {
		visit(r)
	}
	for _, t := range sortedTables(tables) {
		if err := w.next.need(t); err != nil {
			return err
		}
	}
	return nil
}
