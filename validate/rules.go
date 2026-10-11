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
	ruleShardPeriod  = "shard.period"
	ruleScaleFile    = "scale.file"
	ruleScaleFolder  = "scale.folder"
	ruleScaleCorpus  = "scale.corpus"
	ruleDangling     = "reference.dangling"
	ruleDuplicate    = "key.duplicate"
	ruleOrder        = "order"
	ruleLimit        = "state.limit"
	ruleHistoryGap   = "history.gap"
	ruleHistoryChain = "history.chain"
	ruleTransition   = "state.transition"
	ruleRequired     = "state.required"
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
func (c *corpus) rules() error {
	c.scale()
	for _, tb := range c.cat.Tables {
		rows := corpusOrder(c.rows[tb.Name])
		for _, p := range rows {
			c.types(tb, p)
			c.shardPeriod(tb, p)
			c.dangling(p)
			c.checks(tb, p)
			if err := c.judgeRow(tb, p); err != nil {
				return err
			}
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
	return nil
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

// shardPeriod reports a row of a sharded table that stands in the file of another period than the one its shard
// column names (spec 20 Time shards). A row whose column is no value of its type is its type fault alone, and
// a block row stands where its placement row does, so only the rows of a file or document table are judged.
func (c *corpus) shardPeriod(tb *catalog.Table, p placed) {
	sh := tb.Storage.Shard
	if sh == nil {
		return
	}
	v := p.row.Scalars[sh.Column]
	if _, raw := p.row.Raw[sh.Column]; raw || v == nil {
		return
	}
	want, err := tb.RowPath(p.row.Key, *v)
	if err != nil || want == p.file {
		return
	}
	c.report(p, sh.Column, ruleShardPeriod, fmt.Sprintf("%s %s names the period %s, so the row stands in %s, not in %s", sh.Column, *v, periodOfPath(want), want, p.file))
}

// periodOfPath returns the period folder or document name of a sharded row's path.
func periodOfPath(rel string) string {
	rel = strings.TrimSuffix(rel, docSuffix)
	parts := strings.Split(rel, "/")
	if len(parts) >= 3 {
		return parts[1]
	}
	return parts[len(parts)-1]
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
// is at each occurrence after the first. A row whose key equals an earlier key
// byte for byte names the first such row and says byte for byte; any other names
// the first row whose key equals its key without regard to case. A row has one
// key.duplicate.
func (c *corpus) duplicates(tb *catalog.Table, rows []placed) {
	byBytes := map[string]placed{}
	byFold := map[string]placed{}
	for _, p := range rows {
		fold := store.FoldKey(p.row.Key)
		if f, ok := byBytes[p.row.Key]; ok {
			c.report(p, tb.Key, ruleDuplicate, fmt.Sprintf("the key %s equals the key %s of %s byte for byte", p.row.Key, f.row.Key, f.file))
			continue
		}
		byBytes[p.row.Key] = p
		if f, ok := byFold[fold]; ok {
			c.report(p, tb.Key, ruleDuplicate, fmt.Sprintf("the key %s equals the key %s of %s without regard to case", p.row.Key, f.row.Key, f.file))
			continue
		}
		byFold[fold] = p
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

// orders reports the sibling order of a files table, whose order item holds a
// rank (spec 20 Order): within each group of rows with the same parent, a row
// whose rank is shared with a sibling is reported at its order item, since the
// rank does not order the two. A rank not of the column's type is its type fault
// alone and is never shared. A row whose parent is not of its column's type
// belongs to no known group: its own order is not judged.
func (c *corpus) orders(tb *catalog.Table, rows []placed) {
	if tb.Storage.Kind != catalog.StorageFiles || tb.Order == "" {
		return
	}
	parent := parentColumn(tb)
	groups := map[string][]placed{}
	var keys []string
	for _, p := range rows {
		g := ""
		if parent != "" {
			if _, raw := p.row.Raw[parent]; raw {
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
		ranks := map[placed]int64{}
		for _, m := range members {
			if _, raw := m.row.Raw[tb.Order]; raw {
				continue
			}
			if v, ok := orderValue(m.row, tb.Order); ok {
				ranks[m] = v
				counts[v]++
			}
		}
		for _, m := range members {
			if v, ok := ranks[m]; ok && counts[v] > 1 {
				c.report(m, tb.Order, ruleOrder, fmt.Sprintf("the order rank %d of %s is shared with a sibling", v, m.row.Key))
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

// history reports the history gap of each row of a state table, and over each of
// its history rows the chain (the first from is an initial state, each other from
// is the to before it), the transition (its pair is declared) and the required
// columns (a declared pair records a value in each column that the transition's
// require list or the to-terminal's names). A state, from or to not of its column
// type is its type fault alone: no rule is judged on it.
func (c *corpus) history(tb *catalog.Table, rows []placed) {
	sm := tb.StateMachine
	if sm == nil || sm.History.Table == "" {
		return
	}
	ht, found := c.cat.Table(sm.History.Table)
	of, ok := c.historyOf(tb, sm)
	if !found || !ok {
		return
	}
	for _, p := range rows {
		hist := of(p)
		c.gap(sm, p, hist)
		for i, hp := range hist {
			h := hp.row
			from, okFrom := readable(h, sm.History.From)
			to, okTo := readable(h, sm.History.To)
			// The chain of a row reads its own from and the previous row's to, so
			// a raw to of the row itself leaves its chain judged.
			if okFrom {
				if i == 0 {
					if !slices.Contains(sm.Initial, from) {
						c.report(hp, "", ruleHistoryChain, fmt.Sprintf("the history row %s goes from %s, which is not an initial state", h.Key, from))
					}
				} else if prev, ok := readable(hist[i-1].row, sm.History.To); ok && from != prev {
					c.report(hp, "", ruleHistoryChain, fmt.Sprintf("the history row %s goes from %s, which differs from the to %s of the row before it", h.Key, from, prev))
				}
			}
			if !okFrom || !okTo {
				continue
			}
			t, ok := transition(sm, from, to)
			if !ok {
				c.report(hp, "", ruleTransition, fmt.Sprintf("the history row %s goes from %s to %s, which is not a declared transition", h.Key, from, to))
				continue
			}
			values := RowOperands(ht, h, hp.parent)
			for _, col := range required(sm, t) {
				if v := values(col); v.Null || (!v.Raw && v.Text == "") {
					c.report(hp, col, ruleRequired, fmt.Sprintf("the history row %s goes from %s to %s, which requires %s, and %s holds no value", h.Key, from, to, col, col))
				}
			}
		}
	}
}

// historyOf returns the function that gives the history rows of a state row of tb,
// in corpus order: the rows of the history table whose foreign key names it. A
// history table stored as a block of the state table through that key holds them in
// the row's block, whose order in its file is corpus order; any other history table
// holds them among its rows, whose key is matched byte for byte. ok is false when
// such a history table is unavailable, since the history rules then need its
// complete rows (spec 50 Diagnostics), and the file's own diagnostic stands for them.
func (c *corpus) historyOf(tb *catalog.Table, sm *catalog.StateMachine) (func(placed) []placed, bool) {
	ht, found := c.cat.Table(sm.History.Table)
	if !found {
		return nil, false
	}
	if ht.Storage.Kind == catalog.StorageBlock && ht.Storage.Parent == tb.Name && placementOf(ht) == sm.History.Row {
		// A readable row's block is read whole with the row's own file.
		return func(p placed) []placed {
			var out []placed
			for _, h := range p.row.Blocks[ht.Name] {
				out = append(out, placed{row: h, file: p.file, parent: p.row})
			}
			return out
		}, true
	}
	if c.isBlocked(ht.Name) {
		return nil, false
	}
	byKey := map[string][]placed{}
	for _, h := range corpusOrder(c.rows[ht.Name]) {
		if _, raw := h.row.Raw[sm.History.Row]; raw {
			continue
		}
		if v := h.row.Scalars[sm.History.Row]; v != nil {
			byKey[*v] = append(byKey[*v], h)
		}
	}
	return func(p placed) []placed { return byKey[p.row.Key] }, true
}

// placementOf returns the placement foreign key column of a block table.
func placementOf(tb *catalog.Table) string {
	for _, fk := range tb.ForeignKeys {
		if fk.Name == tb.Storage.ParentForeignKey && len(fk.Columns) == 1 {
			return fk.Columns[0]
		}
	}
	return ""
}

// gap reports the history gap of the row p of a state table, whose history rows
// are hist: its state differs from the to of its last history row, or, without
// history rows, is not an initial state. A state or a last to not of its column
// type, or a NULL state, leaves no gap to judge.
func (c *corpus) gap(sm *catalog.StateMachine, p placed, hist []placed) {
	state, ok := readable(p.row, sm.Column)
	if !ok || p.row.Scalars[sm.Column] == nil {
		return
	}
	if len(hist) == 0 {
		if !slices.Contains(sm.Initial, state) {
			c.report(p, "", ruleHistoryGap, fmt.Sprintf("the state %s of %s has no history row and is not an initial state", state, p.row.Key))
		}
		return
	}
	last, ok := readable(hist[len(hist)-1].row, sm.History.To)
	if ok && last != state {
		c.report(p, "", ruleHistoryGap, fmt.Sprintf("the state %s of %s differs from the to %s of its last history row", state, p.row.Key, last))
	}
}

// readable returns the readable text of the scalar column of row r, "" when it is
// NULL, and false when it is not a value of its column type.
func readable(r *store.Row, column string) (string, bool) {
	if _, raw := r.Raw[column]; raw {
		return "", false
	}
	return text(r, column), true
}

// transition returns the declared transition from -> to of the machine.
func transition(sm *catalog.StateMachine, from, to string) (catalog.Transition, bool) {
	for _, t := range sm.Transitions {
		if t.From == from && t.To == to {
			return t, true
		}
	}
	return catalog.Transition{}, false
}

// required returns the columns that the transition t records: its require list,
// then the require list of its to when that is a terminal state, each once.
func required(sm *catalog.StateMachine, t catalog.Transition) []string {
	out := slices.Clone(t.Require)
	for _, term := range sm.Terminals {
		if term.State != t.To {
			continue
		}
		for _, col := range term.Require {
			if !slices.Contains(out, col) {
				out = append(out, col)
			}
		}
	}
	return out
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
func parentColumn(tb *catalog.Table) string { return tb.Hierarchy() }

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

// scale reports the corpus past the scale envelope (spec 40 The scale envelope): a document of more bytes than
// catalog.Envelope.FileBytes (scale.file), a folder of more documents than Envelope.FolderFiles (scale.folder), and a
// corpus of more documents than Envelope.CorpusFiles (scale.corpus). Each names what is past, its size, its table and
// the limit of the envelope. A corpus past the envelope is read and written all the same.
func (c *corpus) scale() {
	limit := catalog.Envelope
	paths := slices.Sorted(maps.Keys(c.files))
	folders := map[string]int{}
	for _, path := range paths {
		f := c.files[path]
		if n := len(f.Data); n > limit.FileBytes {
			c.diags = append(c.diags, scaleDiag(path, ruleScaleFile, fmt.Sprintf("the document of the table %s holds %d bytes, past the %d bytes of the scale envelope", f.Table, n, limit.FileBytes)))
		}
		folders[folderOf(path)]++
	}
	for _, folder := range slices.Sorted(maps.Keys(folders)) {
		if n := folders[folder]; folder != "" && n > limit.FolderFiles {
			c.diags = append(c.diags, scaleDiag(folder, ruleScaleFolder, fmt.Sprintf("the folder %s holds %d documents, past the %d documents of the scale envelope", folder, n, limit.FolderFiles)))
		}
	}
	if n := len(paths); n > limit.CorpusFiles {
		c.diags = append(c.diags, scaleDiag(".", ruleScaleCorpus, fmt.Sprintf("the corpus holds %d documents, past the %d documents of the scale envelope", n, limit.CorpusFiles)))
	}
}

// folderOf returns the folder of a corpus path, "" for a document at the root.
func folderOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return ""
	}
	return path[:i]
}

// scaleDiag is a scale diagnostic located at the first line of a file or a folder.
func scaleDiag(file, rule, msg string) catalog.Diagnostic {
	return catalog.Diagnostic{File: file, Line: 1, Column: 1, Rule: rule, Message: msg}
}
