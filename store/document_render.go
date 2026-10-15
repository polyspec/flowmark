package store

import (
	"errors"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// RenderDocument writes the document file of the rows of a document table. The rows
// may come in any order: the file takes their storage order. Each row carries its
// child blocks, which the file writes in the row's detail section. A row set the
// writer refuses returns no bytes and its refusals, located in the rows given: two
// rows whose keys are equal without regard to case are DUPLICATE_KEY.
func RenderDocument(cat *catalog.Catalog, table, rel string, rows []*Row, lk Linker) ([]byte, []Refusal, error) {
	lay, err := docLayoutOf(cat, table)
	if err != nil {
		return nil, nil, err
	}
	out, lines, rf := renderDocument(lay, rel, rows, lk, true)
	if len(rf) > 0 {
		return nil, rf, nil
	}
	return out, anchorRefusals(lines, out, tableLevels(lay.table, lay.scope-1, lay.children)), nil
}

// renderDocument writes the document of rows in its canonical form, with the origin of
// each line, or with the refusals of the rows when the writer refuses them. A write
// refuses two rows whose keys are equal without regard to case; the read check of
// ParseDocument does not, since the validator reports those keys.
func renderDocument(lay *docLayout, rel string, rows []*Row, lk Linker, write bool) ([]byte, [][]colSpan, []Refusal) {
	w := &writer{rel: rel}
	dw := newDocWriter(w, lk, write, "", "")
	dw.index(lay, rows)
	dw.prepare()
	w.enter(rowRef{index: -1}, "")
	w.put("# " + lay.table + "\n")
	dw.rows(lay, rows)
	w.finish()
	if len(w.refusals) > 0 {
		return nil, nil, w.refusals
	}
	return []byte(w.b.String()), w.lines, nil
}

// docWriter writes the lists and tables of a file with their detail sections and
// child blocks.
type docWriter struct {
	w     *writer
	lk    Linker
	write bool
	// tables holds every row given to the write, by table name, across all the blocks
	// of the file, in the order they were given: the rows of the file that a link may
	// name. shared holds the index of each row among them when its key is shared byte
	// for byte, and -1 otherwise.
	tables map[string][]tabled
	shared map[*Row]int
	// ownTable and ownKey name the files row this file holds; empty for a document file.
	ownTable, ownKey string
}

// tabled is a row given to the write, with the layout of its table.
type tabled struct {
	lay *docLayout
	row *Row
}

// newDocWriter returns a writer of the rows of a file that writes into w. The files
// row ownTable/ownKey is the one the file holds, empty for a document file.
func newDocWriter(w *writer, lk Linker, write bool, ownTable, ownKey string) *docWriter {
	return &docWriter{w: w, lk: lk, write: write,
		tables: map[string][]tabled{}, shared: map[*Row]int{}, ownTable: ownTable, ownKey: ownKey}
}

// index adds the rows of lay and, recursively, their child rows to the rows of the file.
func (dw *docWriter) index(lay *docLayout, rows []*Row) {
	for _, r := range rows {
		dw.tables[lay.table] = append(dw.tables[lay.table], tabled{lay: lay, row: r})
		for _, c := range lay.children {
			dw.index(c, r.Blocks[c.table])
		}
	}
}

// prepare runs once every row of the write is indexed. It gives each row its index
// among the rows given for its table when its key is shared byte for byte with another
// of them, and, in a write, checks the keys of each table over all its blocks: a key
// that a block repeats in another block is refused as well.
func (dw *docWriter) prepare() {
	names := make([]string, 0, len(dw.tables))
	for name := range dw.tables {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		list := dw.tables[name]
		count := map[string]int{}
		for _, t := range list {
			count[t.row.Key]++
		}
		for i, t := range list {
			dw.shared[t.row] = -1
			if count[t.row.Key] > 1 {
				dw.shared[t.row] = i
			}
		}
		if dw.write {
			dw.duplicates(list)
		}
	}
}

// rows writes the list or table of rows of lay in storage order, then the detail
// section of each row that has one. The checks of the rows come first, so a refused
// write writes nothing; each refusal is located at the row it refuses.
func (dw *docWriter) rows(lay *docLayout, rows []*Row) {
	if len(rows) == 0 {
		return
	}
	w := dw.w
	idx := make([]int, len(rows))
	for i, r := range rows {
		idx[i] = dw.shared[r]
	}
	byKey := map[string]int{}
	for i, r := range rows {
		if _, dup := byKey[r.Key]; !dup {
			byKey[r.Key] = i
		}
	}
	for i, r := range rows {
		w.enter(rowRefOf(r, idx[i]), "")
		w.checkRow(lay, byKey, r)
	}
	order, depth, unreached := storageOrder(lay, rows)
	checkOrder(w, lay, rows, idx)
	if len(unreached) > 0 {
		// A row under a parent that names no row, or under a cycle, has no place in the
		// storage order; only the rows on a cycle are refused for it.
		for _, i := range unreached {
			if rowOnCycle(lay, rows, i) {
				w.enter(rowRefOf(rows[i], idx[i]), lay.parent)
				w.refuse(lay.parent, "the parents of the rows form a cycle")
			}
		}
		return
	}
	if lay.list {
		dw.writeList(lay, rows, idx, order, depth)
	} else {
		dw.writeTable(lay, rows, idx, order)
	}
	for _, i := range order {
		if r := rows[i]; hasDetail(lay, r) {
			dw.detail(lay, rowRefOf(r, idx[i]), r)
		}
	}
}

// duplicates refuses the rows given for one table, over all the blocks of the file,
// whose keys equal another's byte for byte: such a key has no row of its own, so no
// file can hold the rows (a table with byte-equal keys is unavailable). The fault is
// at the key column of each such row after the first, naming the first. Keys equal
// only without regard to case are not refused here: the codec cannot tell a row the
// write introduces from one a human edit left, and a key.duplicate on another row
// never fails a write; the executor's key checks own the case-only rule and run on
// the request before the codec, so a DUPLICATE_KEY that reaches the codec is internal.
func (dw *docWriter) duplicates(list []tabled) {
	w := dw.w
	first := map[string]int{}
	for i, t := range list {
		if _, ok := first[t.row.Key]; !ok {
			first[t.row.Key] = i
		}
	}
	for i, t := range list {
		r := t.row
		w.enter(rowRefOf(r, dw.shared[r]), t.lay.key.Name)
		if j := first[r.Key]; j < i {
			w.refuseRule(RuleDuplicateKey, t.lay.key.Name, "the key "+r.Key+" equals the key of the row at index "+strconv.Itoa(j)+" of the rows given")
		}
	}
}

// detail writes the detail section of row r of lay, which is the row rr: its heading
// at the level of the detail (the row scope less one), then its contents.
func (dw *docWriter) detail(lay *docLayout, rr rowRef, r *Row) {
	dw.w.enter(rr, "")
	dw.w.put("\n" + strings.Repeat("#", lay.scope-1) + " " + lay.table + " row " + r.Key + "\n")
	dw.w.elem()
	dw.contents(lay.scope, lay.body, lay.sections, lay.children, r, rr, false, false)
}

// contents writes what a row scope of level scope holds after its heading or its
// H1 and field list: the body, the markdown sections in column order, and the child
// blocks in table name order, each a heading of the scope level with its rows. A
// file row writes its body as a file body, which a field list precedes.
// rr is the row r, whose text the contents write; bodyCol is the body column, which
// names the body's value and the lines before it.
func (dw *docWriter) contents(scope int, bodyCol string, sections []section, children []*docLayout, r *Row, rr rowRef, fileBody, afterFields bool) {
	w := dw.w
	marks := strings.Repeat("#", scope)
	if r.Body != "" {
		w.enter(rr, bodyCol)
		w.put("\n")
		dw.value(rr, "the body", bodyCol, r.Body, scope, fileBody, afterFields)
	}
	for _, s := range sections {
		v := r.Sections[s.name]
		if v == nil {
			continue
		}
		w.enter(rr, s.name)
		w.put("\n" + marks + " " + s.name + "\n")
		w.elem()
		if *v != "" {
			w.put("\n")
			dw.value(rr, "the section "+s.name, s.name, *v, scope, false, false)
		}
	}
	for _, c := range children {
		rows := r.Blocks[c.table]
		if len(rows) == 0 {
			continue
		}
		w.enter(rr, c.table)
		w.put("\n" + marks + " " + c.table + "\n")
		w.elem()
		dw.rows(c, rows)
	}
}

// value writes the text v of a body or a section, of the row rr at column, of the row
// scope level, and records it for its framing check, which the writer runs once the
// file is complete.
func (dw *docWriter) value(rr rowRef, name, column, v string, level int, body, afterFields bool) {
	w := dw.w
	w.values = append(w.values, pendingValue{name: name, column: column, row: rr, v: v, level: level, body: body, afterFields: afterFields, k: w.elems})
	w.elems++
	w.enter(rr, column)
	w.put(v)
}

// checkRow refuses the values of row r that the document cannot hold: a row of
// another table, a key that is not a key, a value or section that is no column of
// the table, a title, body or non-null value that is missing or unwritable, a
// state without a checkbox and a parent that names no row.
// checkRow refuses the values of row r, which the writer has entered, that the
// document cannot hold, each at the column that names the value.
func (w *writer) checkRow(lay *docLayout, byKey map[string]int, r *Row) {
	if r.Table != lay.table {
		w.refuse("", "the row is of table "+r.Table+", not "+lay.table)
	}
	if msg := keyFault(r.Key); msg != "" {
		w.refuse(lay.key.Name, "the key: "+msg)
	} else if canon, ok := readable(lay.key.Type, r.Key); !ok || canon != r.Key {
		w.refuse(lay.key.Name, "the key is not the canonical text of a "+lay.key.Type.String()+" key")
	}
	for _, name := range sortedKeys(r.Scalars) {
		if !lay.writable(name) {
			w.refuse(name, name+" is not a stored scalar column of "+lay.table)
		}
	}
	for _, name := range sortedKeys(r.Sections) {
		if _, ok := lay.sectionOf(name); !ok {
			w.refuse(name, name+" is not a markdown section of "+lay.table)
		}
	}
	for _, name := range sortedBlocks(r) {
		if lay.childNamed(name) == nil {
			w.refuse(name, name+" is not a block of "+lay.table)
		}
	}
	if r.Body != "" && lay.body == "" {
		w.refuse("", "the table has no body column")
	}
	switch {
	case lay.title == "" && r.Title != "":
		w.refuse("", "the table has no title column")
	case lay.title != "":
		if msg := titleFault(lay, r.Title); msg != "" {
			w.refuse(lay.title, msg)
		}
	}
	for _, c := range lay.required() {
		if r.Scalars[c.Name] == nil && !c.Null {
			w.refuse(c.Name, "the non-null column "+c.Name+" has no value")
		}
	}
	for _, s := range lay.sections {
		if r.Sections[s.name] == nil && !s.nullable {
			w.refuse(s.name, "the non-null section "+s.name+" has no value")
		}
	}
	if lay.list && lay.state != "" {
		if st := r.Scalars[lay.state]; st != nil {
			if _, ok := lay.glyphs[*st]; !ok {
				w.refuse(lay.state, "the state "+*st+" has no checkbox")
			}
		}
	}
	if lay.parent != "" {
		if v := r.Scalars[lay.parent]; v != nil {
			if _, ok := byKey[*v]; !ok {
				w.refuse(lay.parent, "the parent "+*v+" names no row")
			}
		}
	}
}

// titleFault names the rule that the title of a row cannot meet, or "" when it can
// be written. A list title may not hold a token <column>\: , which would read back
// as <column>: .
func titleFault(lay *docLayout, title string) string {
	switch {
	case title == "":
		return "the title is empty"
	case !utf8.ValidString(title):
		return "the title is not UTF-8"
	case singleLineFault(title) != "":
		return "the title has " + singleLineFault(title)
	}
	if lay.list {
		for _, sp := range titleTokens(title) {
			t := title[sp[0]:sp[1]]
			for _, n := range lay.names() {
				if strings.HasPrefix(t, n+`\:`) {
					return "the title token " + n + `\: would read back as ` + n + ":"
				}
			}
		}
	}
	return ""
}

// checkOrder refuses the order values of each sibling group unless they are exactly
// 1..m, m being the size of the group.
// A sibling-order fault is at each row of its group whose value is missing, outside
// 1..m or shared with a sibling, m being the size of the group. A value not of the
// column's type is the type fault alone: it counts toward m and is neither outside
// 1..m nor shared, so the faults of its siblings are those that stand whatever it is.
func checkOrder(w *writer, lay *docLayout, rows []*Row, idx []int) {
	if lay.order == "" {
		return
	}
	groups := map[string][]int{}
	for i, r := range rows {
		g := parentKey(lay, r)
		groups[g] = append(groups[g], i)
	}
	keys := make([]string, 0, len(groups))
	for g := range groups {
		keys = append(keys, g)
	}
	sort.Strings(keys)
	for _, g := range keys {
		members := groups[g]
		who := "the root"
		if g != "" {
			who = "the parent " + g
		}
		m := int64(len(members))
		count := map[int64]int{}
		vals := make([]int64, len(members))
		faults := make([]string, len(members))
		for k, i := range members {
			v := rows[i].Scalars[lay.order]
			if v == nil {
				faults[k] = "the order is missing"
				continue
			}
			canon, ok := readable(lay.orderType, *v)
			n, err := strconv.ParseInt(canon, 10, 64)
			if !ok || err != nil {
				faults[k] = lay.order + " is not a " + lay.orderType.String() + " value"
				continue
			}
			vals[k] = n
			count[n]++
		}
		for k, i := range members {
			if faults[k] == "" {
				switch n := vals[k]; {
				case n < 1 || n > m:
					faults[k] = "the order " + strconv.FormatInt(n, 10) + " is outside 1.." + strconv.FormatInt(m, 10) + " of the siblings under " + who
				case count[n] > 1:
					faults[k] = "the order " + strconv.FormatInt(n, 10) + " is shared with a sibling under " + who
				}
			}
			if faults[k] != "" {
				w.enter(rowRefOf(rows[i], idx[i]), lay.order)
				w.refuse(lay.order, faults[k])
			}
		}
	}
}

// writeList writes the list rows in storage order, each indented by two spaces per depth.
func (dw *docWriter) writeList(lay *docLayout, rows []*Row, idx, order, depth []int) {
	w := dw.w
	for _, i := range order {
		r := rows[i]
		rr := rowRefOf(r, idx[i])
		// Each column of the line is written under its own column, so a refusal names it.
		w.enter(rr, "")
		w.put(strings.Repeat("  ", depth[i]) + "- ")
		if lay.state != "" {
			w.enter(rr, lay.state)
			w.put("[" + lay.glyphs[*r.Scalars[lay.state]] + "] ")
		}
		w.enter(rr, lay.key.Name)
		w.put(r.Key + " ")
		w.enter(rr, lay.title)
		w.put(listTitle(lay, r.Title))
		for _, c := range lay.fields {
			v := r.Scalars[c.Name]
			if v == nil {
				continue
			}
			if text, ok := dw.valueText(lay, r, c, *v, rows); ok {
				w.enter(rr, c.Name)
				w.put(" " + listToken(c.Name, text))
			}
		}
		w.enter(rr, "")
		w.put("\n")
	}
}

// writeTable writes the header, the delimiter row and the rows, in storage order.
func (dw *docWriter) writeTable(lay *docLayout, rows []*Row, idx, order []int) {
	w := dw.w
	names := []string{lay.key.Name}
	delims := []string{"---"}
	for _, c := range lay.scalars {
		names = append(names, c.Name)
		delims = append(delims, "---")
	}
	w.put("| " + strings.Join(names, " | ") + " |\n")
	w.put("| " + strings.Join(delims, " | ") + " |\n")
	for _, i := range order {
		r := rows[i]
		rr := rowRefOf(r, idx[i])
		// Each cell is written under its own column, so a refusal names the cell it is in.
		w.enter(rr, lay.key.Name)
		w.put("| " + cellText(r.Key))
		for _, c := range lay.scalars {
			v := r.Scalars[c.Name]
			cell := ""
			switch {
			case c.Name == lay.title:
				cell = cellText(r.Title)
			case v == nil:
			default:
				if text, ok := dw.valueText(lay, r, c, *v, rows); ok {
					cell = cellText(text)
				}
			}
			w.enter(rr, c.Name)
			w.put(" | " + cell)
		}
		w.enter(rr, "")
		w.put(" |\n")
	}
}

// valueText returns the text that a field or a cell writes for the value v of
// column c of row r, which the writer has entered, and whether it can be written: a
// link for a foreign key, the raw text the row was read with, or the readable form
// of the column's type. block holds the rows of r's block, which a parent names.
func (dw *docWriter) valueText(lay *docLayout, r *Row, c dbspec.Column, v string, block []*Row) (string, bool) {
	w := dw.w
	if c.Name == lay.parent && lay.parent != "" {
		// A parent names a row of its own block, so its target is derived from that row.
		for _, b := range block {
			if b.Key == v {
				return "[" + v + "](" + localLink(lay, w.rel, b) + ")", true
			}
		}
		w.refuse(c.Name, "the parent "+v+" names no row of its block")
		return "", false
	}
	if table, isFK := lay.fks[c.Name]; isFK {
		return dw.linkText(r, c, table, v)
	}
	switch {
	case !utf8.ValidString(v):
		w.refuse(c.Name, c.Name+" is not UTF-8")
		return "", false
	case singleLineFault(v) != "":
		w.refuse(c.Name, c.Name+" has "+singleLineFault(v))
		return "", false
	}
	if raw, kept := r.Raw[c.Name]; kept && raw == v {
		return v, true
	}
	canon, ok := readable(c.Type, v)
	if !ok {
		w.refuse(c.Name, c.Name+" is not a "+c.Type.String()+" value")
		return "", false
	}
	return canon, true
}

// readAs reports whether the foreign key of column was read with the key that is
// not of its type: such a key is kept as read, and any other is checked.
func readAs(r *Row, column, key string) bool {
	raw, kept := r.Raw[column]
	if !kept {
		return false
	}
	k, _, ok := splitLink(raw)
	return ok && k == key
}

// linkText writes the link of a foreign key value: its text is the referenced key
// and its target is derived from the referenced row. A key that names no row keeps
// the target it was read with, and any other is FOREIGN_KEY; a key that two or more
// rows of the table share byte for byte has no derived target, so it keeps the
// target it was read with, and any other is CONSTRAINT.
func (dw *docWriter) linkText(r *Row, c dbspec.Column, table, key string) (string, bool) {
	if !readAs(r, c.Name, key) {
		if _, ok := readable(c.Type, key); !ok {
			dw.w.refuse(c.Name, c.Name+" is not a "+c.Type.String()+" value")
			return "", false
		}
	}
	target, found := dw.linkTarget(table, key)
	switch found {
	case linkNone:
		d, kept := r.Dangling[c.Name]
		if !kept || d.Key != key {
			dw.w.refuseRule(RuleForeignKey, c.Name, "the foreign key names no row: "+key)
			return "", false
		}
		target = d.Target
	case linkShared:
		s, kept := r.Shared[c.Name]
		if !kept || s.Key != key {
			dw.w.refuse(c.Name, "the foreign key names the key "+key+" that two or more rows of "+table+" share byte for byte, so it has no derived target")
			return "", false
		}
		target = s.Target
	}
	return "[" + key + "](" + target + ")", true
}

// linkTarget derives the target of a link to the row key of table from the one row
// the key names: from this file for a row the file holds, else from the linker. A
// key that two or more rows of the table share byte for byte, in this file or, when
// the table's placement chain reaches a files table, in the corpus, names no one row.
func (dw *docWriter) linkTarget(table, key string) (string, int) {
	if table == dw.ownTable && key == dw.ownKey {
		return path.Base(dw.w.rel), linkOne
	}
	var match []tabled
	for _, t := range dw.tables[table] {
		if t.row.Key == key {
			match = append(match, t)
		}
	}
	switch {
	case len(match) > 1:
		return "", linkShared
	case len(match) == 1:
		if match[0].lay.inFiles {
			if _, err := dw.lk.Target(dw.w.rel, table, key); errors.Is(err, ErrKeyShared) {
				return "", linkShared
			}
		}
		return localLink(match[0].lay, dw.w.rel, match[0].row), linkOne
	}
	target, err := dw.lk.Target(dw.w.rel, table, key)
	switch {
	case errors.Is(err, ErrKeyShared):
		return "", linkShared
	case err != nil:
		return "", linkNone
	}
	return target, linkOne
}

// sortedBlocks returns the names of the child blocks of row r, sorted.
func sortedBlocks(r *Row) []string {
	names := make([]string, 0, len(r.Blocks))
	for name := range r.Blocks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// listTitle writes the title of a list row: a token that begins with a declared
// column and a colon takes a backslash before the colon. The white space between
// the tokens is kept byte for byte.
func listTitle(lay *docLayout, title string) string {
	var b strings.Builder
	last := 0
	for _, sp := range titleTokens(title) {
		b.WriteString(title[last:sp[0]])
		last = sp[1]
		t := title[sp[0]:sp[1]]
		for _, n := range lay.names() {
			if strings.HasPrefix(t, n+":") {
				t = n + `\:` + t[len(n)+1:]
				break
			}
		}
		b.WriteString(t)
	}
	b.WriteString(title[last:])
	return b.String()
}

// listToken writes a field of a list row, "<column>:<value>": the value is bare when
// it is non-empty and holds neither a quote, a backslash nor an invisible character;
// otherwise it is quoted, and the empty text is "".
func listToken(name, text string) string {
	if text != "" && !strings.ContainsAny(text, `"\`) && !hasInvisible(text) {
		return name + ":" + text
	}
	return name + `:"` + escapeQuoted(text) + `"`
}

// cellText writes the text of a table cell: the empty text is "", a text that begins
// with a quote is quoted, and each pipe is written \|.
func cellText(v string) string {
	t := v
	switch {
	case v == "":
		t = `""`
	case strings.HasPrefix(v, `"`):
		t = `"` + escapeQuoted(v) + `"`
	}
	return strings.ReplaceAll(t, "|", `\|`)
}

// escapeQuoted escapes the backslashes and the quotes of a quoted value.
func escapeQuoted(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v)
}
