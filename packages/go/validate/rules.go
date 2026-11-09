package validate

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
)

// The rule names the validator reports. The format rules and parent.cycle are
// the store's; the others are the validator's own (spec 50 Diagnostics).
const (
	ruleType        = "type"
	ruleShardPeriod = "shard.period"
	ruleScaleFile   = "scale.file"
	ruleScaleFolder = "scale.folder"
	ruleScaleCorpus = "scale.corpus"
	ruleDangling    = "reference.dangling"
	ruleDuplicate   = "key.duplicate"
)

// report appends a diagnostic of rule at the element of column of the row p.
// A column "" is the row's first line.
func (c *corpus) report(p placed, column, rule, msg string) {
	line, col := p.row.At(column)
	c.diags = append(c.diags, catalog.Diagnostic{File: p.file, Line: line, Column: col, Rule: rule, Message: msg})
}

// rules reports the validator rules over the rows of every table. A rule that
// needs the complete rows of a table is not reported for an unavailable table:
// reference.dangling into it, key.duplicate and order within it.
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
		}
		c.parentCycles(tb, rows)
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
	c.danglingComposite(p, tb)
}

// danglingComposite reports each composite foreign key that names no row: such a key holds plain values, not a link, so
// the store resolves nothing for it. A column NULL names no row, and a value not of its type is its type fault alone.
func (c *corpus) danglingComposite(p placed, tb *catalog.Table) {
	operands := RowOperands(tb, p.row, nil)
	for _, fk := range tb.ForeignKeys {
		if len(fk.Columns) < 2 || c.isBlocked(fk.Table) {
			continue
		}
		target, ok := c.cat.Table(fk.Table)
		if !ok {
			continue
		}
		values := make([]string, len(target.KeyParts))
		named := true
		for i, name := range target.KeyParts {
			at := slices.Index(fk.References, name)
			if at < 0 {
				named = false
				break
			}
			v := operands(fk.Columns[at])
			if v.Null || v.Raw {
				named = false
				break
			}
			values[i] = v.Text
		}
		if !named {
			continue
		}
		key := catalog.JoinKey(values)
		if !slices.ContainsFunc(c.rows[fk.Table], func(q placed) bool { return q.row.Key == key }) {
			c.report(p, fk.Columns[0], ruleDangling, fmt.Sprintf("the foreign key %s names no %s row with the key %s", fk.Name, fk.Table, key))
		}
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
