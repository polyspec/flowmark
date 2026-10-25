package irexec

import (
	"fmt"
	"slices"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// transition applies the state machine to an update of row x that gives its
// columns vals (spec 50 The state machine): a change of the state column follows a
// declared transition, never from a row with a history gap; the columns its line and
// a terminal entry require are assigned in the same update, neither NULL nor empty;
// and the transition inserts one history row in the same write.
func (w *writer) transition(g *given, x *rec, vals map[string]cell) *Error {
	sm := w.tb.StateMachine
	if sm == nil {
		return nil
	}
	to, ok := vals[sm.Column]
	if !ok || to.null {
		return nil
	}
	from, ok := g.before[sm.Column]
	if !ok {
		return w.next.typeFault(x, sm.Column)
	}
	if !from.null && from.text == to.text {
		return nil
	}
	g.entered = to.text
	if !slices.Contains(sm.States, to.text) {
		w.fault(g, w.tb.Name, sm.Column, codeConstraint, fmt.Sprintf("the state %s is not a state of %s", to.text, sm.Column))
		return nil
	}
	line := declaredLine(sm, from.text, to.text)
	if line == nil {
		w.fault(g, w.tb.Name, sm.Column, codeConstraint, fmt.Sprintf("%s %s cannot go from the state %s to %s: no declared transition", w.tb.Name, x.row.Key, from.text, to.text))
		return nil
	}
	gapped, err := w.gapped(x, sm, from.text)
	if err != nil {
		return err
	}
	if gapped {
		w.fault(g, w.tb.Name, sm.Column, codeConstraint, fmt.Sprintf("%s %s has a history gap: its state %s is not where its history ends; close the gap with flowmark history repair", w.tb.Name, x.row.Key, from.text))
		return nil
	}
	require := requiredColumns(sm, line)
	for _, c := range require {
		v, assigned := vals[c]
		if !assigned || v.null || v.text == "" {
			w.fault(g, w.tb.Name, c, codeConstraint, fmt.Sprintf("the transition of %s from %s to %s requires %s, assigned in the same update and neither null nor empty", x.row.Key, from.text, to.text, c))
		}
	}
	_, err = w.historyRow(x, sm, from.text, to.text, require, vals)
	return err
}

// requiredColumns returns the columns a transition line requires: those of its
// require list, then those its terminal target requires.
func requiredColumns(sm *catalog.StateMachine, line *catalog.Transition) []string {
	require := slices.Clone(line.Require)
	for _, t := range sm.Terminals {
		if t.State == line.To {
			for _, c := range t.Require {
				if !slices.Contains(require, c) {
					require = append(require, c)
				}
			}
		}
	}
	return require
}

// declaredLine returns the transition line of sm from from to to, nil when none is
// declared.
func declaredLine(sm *catalog.StateMachine, from, to string) *catalog.Transition {
	for i := range sm.Transitions {
		if t := &sm.Transitions[i]; t.From == from && t.To == to {
			return t
		}
	}
	return nil
}

// historyRows returns the history rows of state row x, in storage order: its block
// rows when the history table is a block of the state table, else the rows of the
// history table whose foreign key names it.
func (w *writer) historyRows(x *rec, sm *catalog.StateMachine) ([]*rec, *Error) {
	ht, _ := w.c.cat.Table(sm.History.Table)
	if err := w.next.need(ht.Name); err != nil {
		return nil, err
	}
	if ht.Storage.Kind == catalog.StorageBlock && ht.Storage.Parent == x.tb.Name && ht.Placement() == sm.History.Row {
		var out []*rec
		for _, h := range x.row.Blocks[ht.Name] {
			out = append(out, w.next.byRow[h])
		}
		return out, nil
	}
	all, err := w.next.rows(ht.Name)
	if err != nil {
		return nil, err
	}
	var out []*rec
	for _, h := range all {
		v, err := w.next.value(h, sm.History.Row)
		if err != nil {
			return nil, err
		}
		if !v.null && v.text == x.row.Key {
			out = append(out, h)
		}
	}
	return out, nil
}

// gapped reports whether row x, in the state state, has a history gap: its state
// differs from the to of its last history row, or, without history rows, it is not
// an initial state.
func (w *writer) gapped(x *rec, sm *catalog.StateMachine, state string) (bool, *Error) {
	hist, err := w.historyRows(x, sm)
	if err != nil {
		return false, err
	}
	if len(hist) == 0 {
		return !slices.Contains(sm.Initial, state), nil
	}
	last := hist[len(hist)-1]
	v, err := w.next.value(last, sm.History.To)
	if err != nil {
		return false, err
	}
	return v.null || v.text != state, nil
}

// historyRow inserts the history row of a transition of x from from to to: its
// identity key, the state row's key, from, to, at the request's clock, and a copy of
// the required columns that the history table holds. It returns the history row's
// key.
func (w *writer) historyRow(x *rec, sm *catalog.StateMachine, from, to string, require []string, vals map[string]cell) (cell, *Error) {
	ht, _ := w.c.cat.Table(sm.History.Table)
	if err := w.next.need(ht.Name); err != nil {
		return cell{}, err
	}
	row := &store.Row{Table: ht.Name, Scalars: map[string]*string{}, Sections: map[string]*string{}}
	id := w.nextIdentity(ht, ht.Key)
	setValue(ht, row, ht.Key, id)
	placement := ht.Placement()
	if sm.History.Row != placement {
		setValue(ht, row, sm.History.Row, cell{text: x.row.Key})
	}
	setValue(ht, row, sm.History.From, cell{text: from})
	setValue(ht, row, sm.History.To, cell{text: to})
	setValue(ht, row, sm.History.At, w.clock(dbspec.Type{Kind: dbspec.TypeDatetime, Precision: 6}))
	for _, c := range require {
		if hasColumn(ht, c) {
			if v, ok := vals[c]; ok {
				setValue(ht, row, c, v)
			}
		}
	}
	switch ht.Storage.Kind {
	case catalog.StorageBlock:
		if placement != sm.History.Row || ht.Storage.Parent != x.tb.Name {
			return cell{}, errf(codeInternal, "the catalog admitted the history storage block %s of %s", ht.Storage.ParentForeignKey, ht.Name)
		}
		if x.row.Blocks == nil {
			x.row.Blocks = map[string][]*store.Row{}
		}
		x.row.Blocks[ht.Name] = append(x.row.Blocks[ht.Name], row)
		w.touched[x.file] = true
	case catalog.StorageDocument:
		path := ht.Name + docSuffix
		f := w.next.files[path]
		if f == nil {
			f = &file{table: ht.Name}
			w.next.files[path] = f
		}
		f.rows = append(f.rows, row)
		w.touched[path] = true
	default:
		return cell{}, errf(codeInternal, "the catalog admitted the history storage %s of %s", ht.Storage.Kind, ht.Name)
	}
	w.next.index()
	return id, nil
}

// hasColumn reports whether tb has the column col.
func hasColumn(tb *catalog.Table, col string) bool {
	return slices.ContainsFunc(tb.Columns, func(c dbspec.Column) bool { return c.Name == col })
}

// limits refuses the rows the write puts in a state past its limit: when the rows
// of the table in the state outnumber the limit after the write, each row the write
// put in that state is refused at its state column, naming the limit (spec 50 Limit).
func (w *writer) limits() *Error {
	sm := w.tb.StateMachine
	if sm == nil || len(sm.Limits) == 0 {
		return nil
	}
	for _, l := range sm.Limits {
		var entered []*given
		for _, g := range w.given {
			if g.entered == l.State {
				entered = append(entered, g)
			}
		}
		if len(entered) == 0 {
			continue
		}
		n := int64(0)
		for _, x := range w.next.recs[w.tb.Name] {
			v, err := w.next.value(x, sm.Column)
			if err != nil {
				return err
			}
			if !v.null && v.text == l.State {
				n++
			}
		}
		if n <= l.Count {
			continue
		}
		for _, g := range entered {
			w.fault(g, w.tb.Name, sm.Column, codeConstraint, fmt.Sprintf("the state %s of %s is past its limit %d: %d rows of %s would be in it", l.State, g.row.Key, l.Count, n, w.tb.Name))
		}
	}
	return nil
}

// names joins names for a message.
func names(xs []string) string { return strings.Join(xs, ", ") }
