package irexec

import (
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store"
	"github.com/polyspec/orm/engine/dbspec"
)

// The order column (spec 20 Order): a row's 1-based position among its siblings,
// the rows with the same placement parent and the same parent. An insert without a
// value appends, and with k places the row at k; an update to k moves the row; a
// parent change without a value appends to the new siblings. k lies in 1..n + 1, n
// the number of destination siblings without the row itself. In block and document
// storages the position is the line order, so the siblings follow a move and the
// old siblings close the gap, as they do on a delete. In files storage a row's
// field holds its rank, so a write gives the placed row a rank between its
// neighbours' and leaves the other files alone while such a rank exists.

// groupKey names the sibling group of row x: its placement parent and its parent.
func (w *writer) groupKey(x *rec) (string, *Error) {
	k := ""
	if x.parent != nil {
		k = fmt.Sprintf("%p", x.parent.row)
	}
	if pc := parentColumn(x.tb); pc != "" {
		v, err := w.next.value(x, pc)
		if err != nil {
			return "", err
		}
		if v.null {
			k += "\x00null"
		} else {
			k += "\x00=" + v.text
		}
	}
	return k, nil
}

// siblings returns the rows of x's sibling group other than x.
func (w *writer) siblings(x *rec) ([]*rec, *Error) {
	key, err := w.groupKey(x)
	if err != nil {
		return nil, err
	}
	var out []*rec
	for _, y := range w.next.recs[x.tb.Name] {
		if y == x {
			continue
		}
		k, err := w.groupKey(y)
		if err != nil {
			return nil, err
		}
		if k == key {
			out = append(out, y)
		}
	}
	return out, nil
}

// setOrder gives row y the stored order value v — its rank in files storage, its
// position in the others — and marks its file changed.
func (w *writer) setOrder(y *rec, v int64) {
	setValue(y.tb, y.row, y.tb.Order, cell{text: strconv.FormatInt(v, 10)})
	w.touched[y.file] = true
	w.next.reorder()
}

// placeInOrder places the inserted row g at k among its siblings, or appends it.
func (w *writer) placeInOrder(g *given, k *cell) *Error {
	return w.insertAt(g, w.next.byRow[g.row], k)
}

// position returns the 1-based position k names among n siblings, or the end when
// k is nil; ok is false, the fault reported, when k lies outside 1..n + 1.
func (w *writer) position(g *given, x *rec, k *cell, n int64) (int64, bool) {
	if k == nil {
		return n + 1, true
	}
	pos, perr := strconv.ParseInt(k.text, 10, 64)
	if perr != nil || pos < 1 || pos > n+1 {
		w.fault(g, x.tb.Name, x.tb.Order, codeConstraint, fmt.Sprintf("the order %s lies outside 1..%d among %d siblings", k.text, n+1, n))
		return 0, false
	}
	return pos, true
}

// insertAt puts x at position k of its sibling group, or at the end when k is nil.
// In files storage x takes a rank between its new neighbours' (rank); in the other
// storages the siblings at k or after it move one place on.
func (w *writer) insertAt(g *given, x *rec, k *cell) *Error {
	if x.tb.Storage.Kind == catalog.StorageFiles {
		return w.rank(g, x, k)
	}
	sibs, err := w.siblings(x)
	if err != nil {
		return err
	}
	n := int64(len(sibs))
	pos, ok := w.position(g, x, k, n)
	if !ok {
		w.setOrder(x, n+1)
		return nil
	}
	for _, y := range sibs {
		v, err := w.next.orderOf(y, x.tb.Order)
		if err != nil {
			return err
		}
		if v >= pos {
			w.setOrder(y, v+1)
		}
	}
	w.setOrder(x, pos)
	return nil
}

// closeGap moves the siblings after x one place back, as x leaves its group.
func (w *writer) closeGap(x *rec) *Error {
	v, err := w.next.orderOf(x, x.tb.Order)
	if err != nil {
		return err
	}
	sibs, err := w.siblings(x)
	if err != nil {
		return err
	}
	for _, y := range sibs {
		yv, err := w.next.orderOf(y, x.tb.Order)
		if err != nil {
			return err
		}
		if yv > v {
			w.setOrder(y, yv-1)
		}
	}
	return nil
}

// rankSpacing is the distance between the ranks of a files sibling group that the
// engine numbers: an append takes the last rank plus it, and a renumber gives the
// i-th sibling i times it (spec 20 Order).
const rankSpacing = 1000

// rank gives the row x of a files table the rank of position k among its siblings,
// or of their end when k is nil (spec 20 Order): a row whose rank already lies
// between its new neighbours' keeps it; otherwise it takes the midpoint of the rank
// a before it (0 when it stands first) and the rank b after it, or a plus the
// spacing when it stands last. When no rank lies between them, the group is
// renumbered: the i-th row takes i times the spacing, s = min(spacing, max/m) for a
// group of m rows, and only a row whose rank changes is written. A rank not of its
// type stands after every readable one, so a row placed just before such a rank
// stands last among the readable ranks; the rank before x, or any rank of a group
// to renumber, that is not of its type fails as CONSTRAINT.
func (w *writer) rank(g *given, x *rec, k *cell) *Error {
	tb := x.tb
	sibs, err := w.siblings(x)
	if err != nil {
		return err
	}
	n := int64(len(sibs))
	pos, ok := w.position(g, x, k, n)
	if !ok {
		return nil // the write is refused; x keeps the rank it has
	}
	w.next.sortSiblings(tb, sibs)
	max := int64(math.MaxInt64)
	if columnOf(tb, tb.Order).Type.Kind == dbspec.TypeI32 {
		max = math.MaxInt32
	}
	a := int64(0)
	if pos > 1 {
		v, err := w.next.orderOf(sibs[pos-2], tb.Order)
		if err != nil {
			return err
		}
		a = v
	}
	own, ownOK := w.next.orderValue(x, tb.Order)
	// The row after x, when its rank is not of its type, stands after every readable
	// rank: x then stands last among the readable ranks, and b is not read.
	b, bOK := int64(0), false
	if pos <= n {
		b, bOK = w.next.orderValue(sibs[pos-1], tb.Order)
	}
	if bOK {
		if b > a {
			if ownOK && own > a && own < b {
				w.touched[x.file] = true
				return nil
			}
			// b - a as unsigned cannot overflow, since b > a.
			if d := uint64(b) - uint64(a); d >= 2 {
				w.setOrder(x, a+int64(d/2))
				return nil
			}
		}
	} else {
		if ownOK && own > a {
			w.touched[x.file] = true
			return nil
		}
		if a <= max-rankSpacing {
			w.setOrder(x, a+rankSpacing)
			return nil
		}
	}
	// No rank lies between the neighbours: renumber the group.
	for _, y := range sibs {
		if _, err := w.next.orderOf(y, tb.Order); err != nil {
			return err
		}
	}
	m := n + 1
	if m > max {
		return errf(codeConstraint, "the sibling group of %s %s would hold %d rows, more than the %d ranks of %s", tb.Name, x.row.Key, m, max, tb.Order)
	}
	s := min(int64(rankSpacing), max/m)
	group := slices.Insert(slices.Clone(sibs), int(pos-1), x)
	for i, y := range group {
		r := int64(i+1) * s
		if v, ok := w.next.orderValue(y, tb.Order); y == x || !ok || v != r {
			w.setOrder(y, r)
		}
	}
	return nil
}

// move moves the updated row g from its old sibling group to position k of its
// group after the update, or to its end when k is nil. In files storage the old
// group keeps its ranks, since a gap changes no position; in the other storages it
// closes the gap. The old group is the one the row held before the update:
// oldParent names its placement parent.
func (w *writer) move(g *given, k *cell, oldParent *store.Row) *Error {
	x := w.next.byRow[g.row]
	if x.tb.Storage.Kind == catalog.StorageFiles {
		return w.rank(g, x, k)
	}
	old, err := w.next.orderOf(x, x.tb.Order)
	if err != nil {
		return err
	}
	// The old group: the rows that shared the row's placement parent and parent
	// before the update.
	oldKey := ""
	if oldParent != nil {
		oldKey = fmt.Sprintf("%p", oldParent)
	}
	if pc := parentColumn(x.tb); pc != "" {
		if v := g.before[pc]; v.null {
			oldKey += "\x00null"
		} else {
			oldKey += "\x00=" + v.text
		}
	}
	for _, y := range w.next.recs[x.tb.Name] {
		if y == x {
			continue
		}
		key, err := w.groupKey(y)
		if err != nil {
			return err
		}
		if key != oldKey {
			continue
		}
		v, err := w.next.orderOf(y, x.tb.Order)
		if err != nil {
			return err
		}
		if v > old {
			w.setOrder(y, v-1)
		}
	}
	return w.insertAt(g, x, k)
}
