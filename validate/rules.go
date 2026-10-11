package validate

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/store"
)

// The rule names the validator reports. The format rules and parent.cycle are
// the store's; the others are the validator's own (spec 50 Diagnostics).
const (
	ruleType         = "type"
	ruleDangling     = "reference.dangling"
	ruleDuplicate    = "key.duplicate"
	ruleOrder        = "order"
	ruleLimit        = "state.limit"
	ruleHistoryGap   = "history.gap"
	ruleHistoryChain = "history.chain"
)

// report appends a diagnostic of rule at the element of column of the row p.
// A column "" is the row's first line.
func (c *corpus) report(p placed, column, rule, msg string) {
	line, col := p.row.At(column)
	c.diags = append(c.diags, catalog.Diagnostic{File: p.file, Line: line, Column: col, Rule: rule, Message: msg})
}

// rules reports the validator rules over the rows of every table. A rule that
// needs the complete rows of a table is not reported for an unavailable table:
// reference.dangling into it, key.duplicate, order and state.limit within it.
// The validator's parent.cycle across files is reported among the rows read, since
// no unread row can undo a cycle that the rows' own parent values make.
func (c *corpus) rules() {
	for _, tb := range c.cat.Tables {
		rows := corpusOrder(c.rows[tb.Name])
		for _, p := range rows {
			c.types(tb, p)
			c.dangling(p)
		}
		if !c.isBlocked(tb.Name) {
			c.duplicates(tb, rows)
			c.uniques(tb, rows)
			c.orders(tb, rows)
			c.limits(tb, rows)
		}
		c.parentCycles(tb, rows)
		c.history(tb, rows)
	}
}

// corpusOrder returns the rows in corpus order: by the corpus path of the file
// that holds each row, in byte order, then by the line and the column where the
// row begins. It is total for every readable corpus, whereas a storage order is
// undefined across a parent cycle of files rows or a sibling group with shared
// order values.
func corpusOrder(rows []placed) []placed {
	out := slices.Clone(rows)
	slices.SortStableFunc(out, func(a, b placed) int {
		if a.file != b.file {
			return strings.Compare(a.file, b.file)
		}
		al, ac := a.row.At("")
		bl, bc := b.row.At("")
		if al != bl {
			return al - bl
		}
		return ac - bc
	})
	return out
}

// types reports each stored scalar whose text is not a value of its column type.
func (c *corpus) types(tb *catalog.Table, p placed) {
	for _, col := range slices.Sorted(maps.Keys(p.row.Raw)) {
		c.report(p, col, ruleType, fmt.Sprintf("%s is not a value of type %s", col, columnType(tb, col)))
	}
}

// dangling reports each foreign key that names no row, unless its referenced table
// is unavailable. A key not of its column type is its type fault alone.
func (c *corpus) dangling(p placed) {
	tb, ok := c.cat.Table(p.row.Table)
	if !ok {
		return
	}
	for _, col := range slices.Sorted(maps.Keys(p.row.Dangling)) {
		target := fkTable(tb, col)
		if _, raw := p.row.Raw[col]; raw || c.isBlocked(target) {
			continue
		}
		d := p.row.Dangling[col]
		c.report(p, col, ruleDangling, fmt.Sprintf("the foreign key %s names no %s row with the key %s", col, target, d.Key))
	}
}

// duplicates reports each row whose key equals the key of an earlier row of the
// table without regard to case, a byte-equal key being one of them, over every
// block of every file that holds the table: rows is in corpus order, so the fault
// is at each occurrence after the first, naming the first.
func (c *corpus) duplicates(tb *catalog.Table, rows []placed) {
	first := map[string]placed{}
	for _, p := range rows {
		k := strings.ToLower(p.row.Key)
		f, ok := first[k]
		if !ok {
			first[k] = p
			continue
		}
		c.report(p, tb.Key, ruleDuplicate, fmt.Sprintf("the key %s equals the key %s of %s without regard to case", p.row.Key, f.row.Key, f.file))
	}
}

// uniques reports each row whose unique key equals that of an earlier row in
// corpus order, naming the first. A NULL or a value not of its type takes no part:
// NULLs are distinct as in SQL, and an unreadable value has no comparison.
func (c *corpus) uniques(tb *catalog.Table, rows []placed) {
	for _, u := range tb.Uniques {
		first := map[string]placed{}
	row:
		for _, p := range rows {
			parts := make([]string, len(u))
			for i, col := range u {
				v := p.row.Scalars[col]
				if _, raw := p.row.Raw[col]; raw || v == nil {
					continue row
				}
				parts[i] = *v
			}
			k := strings.Join(parts, "\x00")
			if f, ok := first[k]; ok {
				c.report(p, u[0], ruleDuplicate, fmt.Sprintf("the unique key (%s) of %s equals that of %s in %s", strings.Join(u, ", "), p.row.Key, f.row.Key, f.file))
				continue
			}
			first[k] = p
		}
	}
}

// orders reports the sibling order of a files table: within each group of rows
// with the same parent, of size n, the order values are exactly 1..n, so a row
// whose value lies outside 1..n or is shared with a sibling is reported at its
// order item. A value not of the column's type is its type fault alone: it counts
// toward n and is neither outside 1..n nor shared. A row whose parent is not of
// its column's type belongs to no known group: it counts toward no group's n and
// its own order is not judged, and a fault of a group is reported only when it
// stands whichever groups the u such rows join — a value shared within the group,
// or one outside 1..n + u.
func (c *corpus) orders(tb *catalog.Table, rows []placed) {
	if tb.Storage.Kind != catalog.StorageFiles || tb.Order == "" {
		return
	}
	parent := parentColumn(tb)
	groups := map[string][]placed{}
	var keys []string
	unknown := int64(0)
	for _, p := range rows {
		g := ""
		if parent != "" {
			if _, raw := p.row.Raw[parent]; raw {
				unknown++
				continue
			}
			g = text(p.row, parent)
		}
		if _, ok := groups[g]; !ok {
			keys = append(keys, g)
		}
		groups[g] = append(groups[g], p)
	}
	for _, g := range keys {
		members := groups[g]
		counts := map[int64]int{}
		for _, m := range members {
			if v, ok := orderValue(m.row, tb.Order); ok {
				counts[v]++
			}
		}
		n := int64(len(members))
		for _, m := range members {
			if _, raw := m.row.Raw[tb.Order]; raw {
				continue
			}
			v, ok := orderValue(m.row, tb.Order)
			switch {
			case !ok:
				c.report(m, tb.Order, ruleOrder, fmt.Sprintf("the order of %s is missing", m.row.Key))
			case counts[v] > 1:
				c.report(m, tb.Order, ruleOrder, fmt.Sprintf("the order %d of %s is shared with a sibling", v, m.row.Key))
			case v < 1 || v > n+unknown:
				c.report(m, tb.Order, ruleOrder, fmt.Sprintf("the order %d of %s is outside 1..%d among its %d siblings", v, m.row.Key, n+unknown, n))
			}
		}
	}
}

// limits reports the rows of a state table past each limit of its machine: when
// m readable rows, in every block and file that holds the table, are in the state
// and m > n, each row after the n-th in corpus order (rows is in corpus order) is
// reported at its state element, naming the state, the limit and m.
func (c *corpus) limits(tb *catalog.Table, rows []placed) {
	sm := tb.StateMachine
	if sm == nil {
		return
	}
	for _, l := range sm.Limits {
		var in []placed
		for _, p := range rows {
			if _, raw := p.row.Raw[sm.Column]; raw {
				continue
			}
			if s := p.row.Scalars[sm.Column]; s != nil && *s == l.State {
				in = append(in, p)
			}
		}
		m := int64(len(in))
		if m <= l.Count {
			continue
		}
		for _, p := range in[l.Count:] {
			c.report(p, sm.Column, ruleLimit, fmt.Sprintf("%s is past the limit %d of the state %s, which %d rows of %s are in", p.row.Key, l.Count, l.State, m, tb.Name))
		}
	}
}

// parentCycles reports each row of a files table on a cycle of parent keys. The
// chain of a row is followed through the rows of the table, so a cycle may cross
// files; a parent that names no row ends the chain. A cycle in a block or a
// document table lies within one file and is the read rule, so it is not checked
// here.
func (c *corpus) parentCycles(tb *catalog.Table, rows []placed) {
	parent := parentColumn(tb)
	if parent == "" || tb.Storage.Kind != catalog.StorageFiles {
		return
	}
	byKey := map[string]int{}
	for i, p := range rows {
		if _, ok := byKey[p.row.Key]; !ok {
			byKey[p.row.Key] = i
		}
	}
	const (
		unseen = iota
		onPath
		done
	)
	state := make([]int, len(rows))
	for start := range rows {
		if state[start] != unseen {
			continue
		}
		var path []int
		for cur := start; ; {
			if state[cur] == done {
				break
			}
			if state[cur] == onPath {
				for i := len(path) - 1; i >= 0; i-- {
					m := path[i]
					c.report(rows[m], parent, store.RuleParentCycle, fmt.Sprintf("the parent of %s is on a cycle of parents", rows[m].row.Key))
					if m == cur {
						break
					}
				}
				break
			}
			state[cur] = onPath
			path = append(path, cur)
			pv := rows[cur].row.Scalars[parent]
			if _, raw := rows[cur].row.Raw[parent]; raw || pv == nil {
				break
			}
			next, ok := byKey[*pv]
			if !ok {
				break
			}
			cur = next
		}
		for _, m := range path {
			state[m] = done
		}
	}
}

// history reports the history gap of each row of a state table, and the chain
// of each of its history rows: the first from is an initial state, each from is
// the to before it, and each pair is a declared transition.
func (c *corpus) history(tb *catalog.Table, rows []placed) {
	sm := tb.StateMachine
	if sm == nil || sm.History.Table == "" {
		return
	}
	for _, p := range rows {
		s := p.row.Scalars[sm.Column]
		if s == nil {
			continue
		}
		state := *s
		hist := p.row.Blocks[sm.History.Table]
		if len(hist) == 0 {
			if !slices.Contains(sm.Initial, state) {
				c.report(p, "", ruleHistoryGap, fmt.Sprintf("the state %s of %s has no history row and is not an initial state", state, p.row.Key))
			}
		} else if last := text(hist[len(hist)-1], sm.History.To); last != state {
			c.report(p, "", ruleHistoryGap, fmt.Sprintf("the state %s of %s differs from the to %s of its last history row", state, p.row.Key, last))
		}
		for i, h := range hist {
			from, to := text(h, sm.History.From), text(h, sm.History.To)
			var follows bool
			if i == 0 {
				follows = slices.Contains(sm.Initial, from)
			} else {
				follows = from == text(hist[i-1], sm.History.To)
			}
			if follows && declared(sm, from, to) {
				continue
			}
			c.report(placed{row: h, file: p.file}, "", ruleHistoryChain, fmt.Sprintf("the history row %s goes from %s to %s, which does not follow the row before it or is not a declared transition", h.Key, from, to))
		}
	}
}

// declared reports whether the transition from -> to is declared by the machine.
func declared(sm *catalog.StateMachine, from, to string) bool {
	for _, t := range sm.Transitions {
		if t.From == from && t.To == to {
			return true
		}
	}
	return false
}

// text returns the readable text of the scalar column of row r, "" when it is NULL.
func text(r *store.Row, column string) string {
	if v := r.Scalars[column]; v != nil {
		return *v
	}
	return ""
}

// orderValue returns the order value of row r, false when it is missing or not an
// integer.
func orderValue(r *store.Row, column string) (int64, bool) {
	v, err := strconv.ParseInt(text(r, column), 10, 64)
	return v, err == nil
}

// parentColumn returns the self foreign key that holds a row's parent, "" when the
// table has none.
func parentColumn(tb *catalog.Table) string {
	for _, fk := range tb.ForeignKeys {
		if fk.Table == tb.Name && len(fk.Columns) == 1 && fk.Columns[0] == "parent" {
			return "parent"
		}
	}
	return ""
}

// fkTable returns the table that the one-column foreign key on col references, "".
func fkTable(tb *catalog.Table, col string) string {
	for _, fk := range tb.ForeignKeys {
		if len(fk.Columns) == 1 && fk.Columns[0] == col {
			return fk.Table
		}
	}
	return ""
}

// columnType returns the type text of column col of table tb.
func columnType(tb *catalog.Table, col string) string {
	for _, c := range tb.Columns {
		if c.Name == col {
			return c.Type.String()
		}
	}
	return "?"
}
