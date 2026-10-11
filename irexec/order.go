package irexec

import (
	"fmt"
	"strconv"

	"github.com/polyspec/flowmark/store"
)

// The order column (spec 20 Order): a row's 1-based position among its siblings,
// the rows with the same placement parent and the same parent. An insert without a
// value appends, and with k places the row at k; an update to k moves the row and
// the siblings follow; a parent change without a value appends to the new siblings;
// the old siblings close the gap, as they do on a delete. k lies in 1..n + 1, n the
// number of destination siblings without the row itself.

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

// setOrder gives row y the order value v and marks its file changed.
func (w *writer) setOrder(y *rec, v int64) {
	setValue(y.tb, y.row, y.tb.Order, cell{text: strconv.FormatInt(v, 10)})
	w.touched[y.file] = true
}

// placeInOrder places the inserted row g at k among its siblings, or appends it.
func (w *writer) placeInOrder(g *given, k *cell) *Error {
	return w.insertAt(g, w.next.byRow[g.row], k)
}

// insertAt puts x at position k of its sibling group, the siblings at k or after it
// moving one place on, or at the end when k is nil.
func (w *writer) insertAt(g *given, x *rec, k *cell) *Error {
	sibs, err := w.siblings(x)
	if err != nil {
		return err
	}
	n := int64(len(sibs))
	if k == nil {
		w.setOrder(x, n+1)
		return nil
	}
	pos, perr := strconv.ParseInt(k.text, 10, 64)
	if perr != nil || pos < 1 || pos > n+1 {
		w.fault(g, x.tb.Name, x.tb.Order, codeConstraint, fmt.Sprintf("the order %s lies outside 1..%d among %d siblings", k.text, n+1, n))
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

// move moves the updated row g from its old sibling group, which closes the gap, to
// position k of its group after the update, or to its end when k is nil. The old
// group is the one the row held before the update: oldGroup names it.
func (w *writer) move(g *given, k *cell, oldParent *store.Row) *Error {
	x := w.next.byRow[g.row]
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
