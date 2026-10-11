package irexec

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/polyspec/flowmark/store"
)

// Repair is the history repair `flowmark history repair <table> <key> [--from
// <initial state>]` (spec 50 A state changed outside the engine, Repair): it closes
// the history gap of the row of table whose key is key, byte for byte, by inserting
// one history row from the last to — for a row without history rows the one initial
// state, or from, which must name one when the machine has several — to the row's
// current state, at now (the engine clock), with the columns the transition requires
// copied from the row. It answers {affected: 1, keys: [the history row's key]} with
// the post-image, which the write path commits as any write.
//
// It refuses as CONSTRAINT a row without a gap, a pair that is not a declared
// transition, a required column that is NULL or empty on the row, and a state whose
// limit the rows in it, this row counted, exceed; as CONFIG a table without a state
// machine with a history, and a from that is given for a row with history rows, that
// is not an initial state, or that is missing where the machine has several; and as
// NO_ROWS a key that names no row.
func (e *Executor) Repair(c *Corpus, table, key, from string, now time.Time) (*Result, error) {
	res, err := e.repair(c, table, key, from, now)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (e *Executor) repair(c *Corpus, table, key, from string, now time.Time) (*Result, *Error) {
	if c.cat != e.cat {
		return nil, errf(codeInternal, "the corpus was read with another catalog than the executor's")
	}
	tb, ok := c.cat.Table(table)
	if !ok {
		return nil, errf(codeConfig, "the catalog has no table %s", table)
	}
	sm := tb.StateMachine
	if sm == nil || sm.History.Table == "" {
		return nil, errf(codeConfig, "the table %s has no state machine with a history, so it has no history to repair", table)
	}
	rows, err := c.rows(table)
	if err != nil {
		return nil, err
	}
	var x *rec
	for _, y := range rows {
		if y.row.Key == key {
			x = y
			break
		}
	}
	if x == nil {
		return nil, errf(codeNoRows, "no %s row has the key %s", table, key)
	}
	w := &writer{r: &reader{e: e, c: c, cl: &call{now: now}}, c: c, next: c.clone(), tb: tb, touched: map[string]bool{}, answer: &Answer{Kind: "insert"}}
	x = w.nextOf(x)
	refuse := func(column, msg string) *Error {
		return faultError([]store.Refusal{{Table: table, Key: key, Index: -1, Column: column, Rule: codeConstraint, Message: msg}})
	}
	state, err := w.next.value(x, sm.Column)
	if err != nil {
		return nil, err
	}
	if state.null {
		return nil, refuse(sm.Column, fmt.Sprintf("%s %s has no state, so it has no history gap: nothing to repair", table, key))
	}
	hist, err := w.historyRows(x, sm)
	if err != nil {
		return nil, err
	}
	var start string
	if len(hist) > 0 {
		last, err := w.next.value(hist[len(hist)-1], sm.History.To)
		if err != nil {
			return nil, err
		}
		if from != "" {
			return nil, errf(codeConfig, "--from %s is given for %s %s, which has history rows: the repair goes from the to %s of its last history row; repair without --from", from, table, key, last.text)
		}
		start = last.text
	} else {
		switch {
		case from != "":
			if !slices.Contains(sm.Initial, from) {
				return nil, errf(codeConfig, "--from %s is not an initial state of %s (%s); name one of them", from, table, strings.Join(sm.Initial, ", "))
			}
			start = from
		case len(sm.Initial) == 1:
			start = sm.Initial[0]
		default:
			return nil, errf(codeConfig, "%s %s has no history rows and the machine of %s has several initial states (%s); name the one it started in with --from", table, key, table, strings.Join(sm.Initial, ", "))
		}
	}
	gap := start != state.text
	if len(hist) == 0 {
		gap = !slices.Contains(sm.Initial, state.text)
	}
	if !gap {
		return nil, refuse(sm.Column, fmt.Sprintf("%s %s has no history gap: its state %s is where its history ends; nothing to repair", table, key, state.text))
	}
	line := declaredLine(sm, start, state.text)
	if line == nil {
		return nil, refuse(sm.Column, fmt.Sprintf("%s %s cannot be repaired from %s to %s: the pair is not a declared transition of %s; restore a legal state by editing the row", table, key, start, state.text, sm.Column))
	}
	require := requiredColumns(sm, line)
	vals := map[string]cell{}
	var faults []store.Refusal
	for _, col := range require {
		v, err := w.next.value(x, col)
		if err != nil {
			return nil, err
		}
		if v.null || v.text == "" {
			faults = append(faults, store.Refusal{Table: table, Key: key, Index: -1, Column: col, Rule: codeConstraint,
				Message: fmt.Sprintf("the transition from %s to %s requires %s, which is null or empty on %s %s; fill it by editing the row", start, state.text, col, table, key)})
			continue
		}
		vals[col] = v
	}
	if len(faults) > 0 {
		return nil, faultError(faults)
	}
	for _, l := range sm.Limits {
		if l.State != state.text {
			continue
		}
		n := int64(0)
		for _, y := range w.next.recs[table] {
			v, err := w.next.value(y, sm.Column)
			if err != nil {
				return nil, err
			}
			if !v.null && v.text == l.State {
				n++
			}
		}
		if n > l.Count {
			return nil, refuse(sm.Column, fmt.Sprintf("the state %s of %s %s is past its limit %d: %d rows of %s are in it, this row counted; restore a legal state by editing", l.State, table, key, l.Count, n, table))
		}
	}
	id, err := w.historyRow(x, sm, start, state.text, require, vals)
	if err != nil {
		return nil, err
	}
	ht, _ := c.cat.Table(sm.History.Table)
	w.answer.Affected = 1
	w.answer.Keys = []any{output(columnOf(ht, ht.Key).Type, id)}
	post, err := w.finish()
	if err != nil {
		return nil, err
	}
	return &Result{Answer: w.answer, Post: post}, nil
}
