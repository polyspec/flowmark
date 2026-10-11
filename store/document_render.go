package store

import (
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/orm/engine/dbspec"
)

// RenderDocument writes the document file of the rows of a document table. The rows
// may come in any order: the file takes their storage order. Each row carries its
// child blocks, which the file writes in the row's detail section. A row set the
// writer refuses returns no bytes and diagnostics, located at the line of the row;
// two rows whose keys are equal without regard to case are DUPLICATE_KEY.
func RenderDocument(cat *catalog.Catalog, table string, rows []*Row, lk Linker) ([]byte, []Diagnostic, error) {
	lay, err := docLayoutOf(cat, table)
	if err != nil {
		return nil, nil, err
	}
	out, diags := renderDocument(lay, rows, lk, true)
	if len(diags) > 0 {
		return nil, diags, nil
	}
	if rd := anchorRefusals(docFileName(lay), out, tableLevels(lay.table, lay.scope-1, lay.children)); len(rd) > 0 {
		return nil, rd, nil
	}
	return out, nil, nil
}

// renderDocument writes the document of rows in its canonical form. A write refuses
// two rows whose keys are equal without regard to case; the read check of
// ParseDocument does not, since the validator reports those keys.
func renderDocument(lay *docLayout, rows []*Row, lk Linker, write bool) ([]byte, []Diagnostic) {
	w := &writer{rel: docFileName(lay)}
	dw := newDocWriter(w, lk, write, "", "")
	dw.index(lay, rows)
	w.put("# " + lay.table + "\n")
	dw.rows(lay, rows)
	w.finish()
	if len(w.diags) > 0 {
		return nil, w.diags
	}
	return []byte(w.b.String()), nil
}

// docWriter writes the lists and tables of a file with their detail sections and
// child blocks. idx holds every row of the file by table and key, the rows that a
// link may name; lower holds the table and key of each row written, without regard
// to case, for the duplicate check of a write.
type docWriter struct {
	w     *writer
	lk    Linker
	write bool
	idx   map[string]map[string]fileRow
	lower map[string]bool
	// ownTable and ownKey name the files row this file holds; empty for a document file.
	ownTable, ownKey string
}

// fileRow is a row of the file with the layout that holds it.
type fileRow struct {
	lay *docLayout
	row *Row
}

// newDocWriter returns a writer of the rows of a file that writes into w. The files
// row ownTable/ownKey is the one the file holds, empty for a document file.
func newDocWriter(w *writer, lk Linker, write bool, ownTable, ownKey string) *docWriter {
	return &docWriter{w: w, lk: lk, write: write, idx: map[string]map[string]fileRow{}, lower: map[string]bool{},
		ownTable: ownTable, ownKey: ownKey}
}

// index adds the rows of lay and, recursively, their child rows to the rows of the file.
func (dw *docWriter) index(lay *docLayout, rows []*Row) {
	for _, r := range rows {
		if dw.idx[lay.table] == nil {
			dw.idx[lay.table] = map[string]fileRow{}
		}
		if _, dup := dw.idx[lay.table][r.Key]; !dup {
			dw.idx[lay.table][r.Key] = fileRow{lay: lay, row: r}
		}
		for _, c := range lay.children {
			dw.index(c, r.Blocks[c.table])
		}
	}
}

// rows writes the list or table of rows of lay in storage order, then the detail
// section of each row that has one. Each row is checked at the line it is written
// on; a parent cycle is refused where the first row of the cycle would be written.
func (dw *docWriter) rows(lay *docLayout, rows []*Row) {
	if len(rows) == 0 {
		return
	}
	w := dw.w
	order, depth, unreached := storageOrder(lay, rows)
	pos := make([]int, len(rows))
	for k, i := range order {
		pos[i] = k
	}
	first := w.next()
	if !lay.list {
		first += 2 // the header row and the delimiter row
	}
	rowLine := func(i int) int { return first + pos[i] }
	byKey := map[string]int{}
	for i, r := range rows {
		if _, dup := byKey[r.Key]; !dup {
			byKey[r.Key] = i
		}
	}
	for i, r := range rows {
		line := rowLine(i)
		w.checkRow(lay, byKey, r, line)
		if !dw.write {
			continue
		}
		k := lay.table + "/" + strings.ToLower(r.Key)
		if dw.lower[k] {
			w.refuseRule(RuleDuplicateKey, line, 1, "the key "+r.Key+" equals another key of "+lay.table+" without regard to case")
		}
		dw.lower[k] = true
	}
	checkOrder(w, lay, rows, rowLine)
	if len(unreached) > 0 {
		for _, i := range unreached {
			if rowOnCycle(lay, rows, i) {
				w.refuse(rowLine(i), 1, "the parents of the rows form a cycle")
				break
			}
		}
		return
	}
	if lay.list {
		dw.writeList(lay, rows, order, depth)
	} else {
		dw.writeTable(lay, rows, order)
	}
	for _, i := range order {
		if r := rows[i]; hasDetail(lay, r) {
			dw.detail(lay, r)
		}
	}
}

// detail writes the detail section of row r of lay: its heading at the level of
// the detail (the row scope less one), then its contents.
func (dw *docWriter) detail(lay *docLayout, r *Row) {
	dw.w.put("\n" + strings.Repeat("#", lay.scope-1) + " " + lay.table + " row " + r.Key + "\n")
	dw.w.elem()
	dw.contents(lay.scope, lay.sections, lay.children, r, false, false)
}

// contents writes what a row scope of level scope holds after its heading or its
// H1 and field list: the body, the markdown sections in column order, and the child
// blocks in table name order, each a heading of the scope level with its rows. A
// file row writes its body as a file body, which a field list precedes.
func (dw *docWriter) contents(scope int, sections []section, children []*docLayout, r *Row, fileBody, afterFields bool) {
	w := dw.w
	marks := strings.Repeat("#", scope)
	if r.Body != "" {
		w.put("\n")
		dw.value("the body", r.Body, scope, fileBody, afterFields)
	}
	for _, s := range sections {
		v := r.Sections[s.name]
		if v == nil {
			continue
		}
		w.put("\n" + marks + " " + s.name + "\n")
		w.elem()
		if *v != "" {
			w.put("\n")
			dw.value("the section "+s.name, *v, scope, false, false)
		}
	}
	for _, c := range children {
		rows := r.Blocks[c.table]
		if len(rows) == 0 {
			continue
		}
		w.put("\n" + marks + " " + c.table + "\n")
		w.elem()
		dw.rows(c, rows)
	}
}

// value writes the text v of a body or a section, of the row scope level, and
// records it for its framing check, which the writer runs once the file is complete.
func (dw *docWriter) value(name, v string, level int, body, afterFields bool) {
	w := dw.w
	w.values = append(w.values, pendingValue{name: name, v: v, line: w.next(), level: level, body: body, afterFields: afterFields, k: w.elems})
	w.elems++
	w.put(v)
}

// checkRow refuses the values of row r that the document cannot hold: a row of
// another table, a key that is not a key, a value or section that is no column of
// the table, a title, body or non-null value that is missing or unwritable, a
// state without a checkbox and a parent that names no row.
func (w *writer) checkRow(lay *docLayout, byKey map[string]int, r *Row, line int) {
	if r.Table != lay.table {
		w.refuse(line, 1, "the row is of table "+r.Table+", not "+lay.table)
	}
	if msg := keyFault(r.Key); msg != "" {
		w.refuse(line, 1, "the key: "+msg)
	} else if canon, ok := readable(lay.key.Type, r.Key); !ok || canon != r.Key {
		w.refuse(line, 1, "the key is not the canonical text of a "+lay.key.Type.String()+" key")
	}
	for _, name := range sortedKeys(r.Scalars) {
		if !lay.writable(name) {
			w.refuse(line, 1, name+" is not a stored scalar column of "+lay.table)
		}
	}
	for _, name := range sortedKeys(r.Sections) {
		if _, ok := lay.sectionOf(name); !ok {
			w.refuse(line, 1, name+" is not a markdown section of "+lay.table)
		}
	}
	for _, name := range sortedBlocks(r) {
		if lay.childNamed(name) == nil {
			w.refuse(line, 1, name+" is not a block of "+lay.table)
		}
	}
	if r.Body != "" && lay.body == "" {
		w.refuse(line, 1, "the table has no body column")
	}
	switch {
	case lay.title == "" && r.Title != "":
		w.refuse(line, 1, "the table has no title column")
	case lay.title != "":
		if msg := titleFault(lay, r.Title); msg != "" {
			w.refuse(line, 1, msg)
		}
	}
	for _, c := range lay.required() {
		if r.Scalars[c.Name] == nil && !c.Null {
			w.refuse(line, 1, "the non-null column "+c.Name+" has no value")
		}
	}
	for _, s := range lay.sections {
		if r.Sections[s.name] == nil && !s.nullable {
			w.refuse(line, 1, "the non-null section "+s.name+" has no value")
		}
	}
	if lay.list && lay.state != "" {
		if st := r.Scalars[lay.state]; st != nil {
			if _, ok := lay.glyphs[*st]; !ok {
				w.refuse(line, 1, "the state "+*st+" has no checkbox")
			}
		}
	}
	if lay.parent != "" {
		if v := r.Scalars[lay.parent]; v != nil {
			if _, ok := byKey[*v]; !ok {
				w.refuse(line, 1, "the parent "+*v+" names no row")
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
		for _, t := range strings.Split(title, " ") {
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
func checkOrder(w *writer, lay *docLayout, rows []*Row, rowLine func(int) int) {
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
		idx := groups[g]
		var vals []int
		ok := true
		for _, i := range idx {
			v := rows[i].Scalars[lay.order]
			if v == nil {
				w.refuse(rowLine(i), 1, "the order is missing")
				ok = false
				continue
			}
			n, err := strconv.Atoi(*v)
			if err != nil || strconv.Itoa(n) != *v {
				w.refuse(rowLine(i), 1, "the order "+*v+" is not a canonical integer")
				ok = false
				continue
			}
			vals = append(vals, n)
		}
		if !ok {
			continue
		}
		sort.Ints(vals)
		for k, n := range vals {
			if n != k+1 {
				who := "the root"
				if g != "" {
					who = "the parent " + g
				}
				w.refuse(rowLine(idx[0]), 1, "the order of the siblings under "+who+" is not 1.."+strconv.Itoa(len(idx)))
				break
			}
		}
	}
}

// writeList writes the list rows in storage order, each indented by two spaces per depth.
func (dw *docWriter) writeList(lay *docLayout, rows []*Row, order, depth []int) {
	w := dw.w
	for _, i := range order {
		r := rows[i]
		line := w.next()
		var b strings.Builder
		b.WriteString(strings.Repeat("  ", depth[i]) + "- ")
		if lay.state != "" {
			b.WriteString("[" + lay.glyphs[*r.Scalars[lay.state]] + "] ")
		}
		b.WriteString(r.Key + " " + listTitle(lay, r.Title))
		for _, c := range lay.fields {
			v := r.Scalars[c.Name]
			if v == nil {
				continue
			}
			if text, ok := dw.valueText(lay, r, c, *v, line); ok {
				b.WriteString(" " + listToken(c.Name, text))
			}
		}
		w.put(b.String() + "\n")
	}
}

// writeTable writes the header, the delimiter row and the rows, in storage order.
func (dw *docWriter) writeTable(lay *docLayout, rows []*Row, order []int) {
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
		line := w.next()
		cells := []string{cellText(r.Key)}
		for _, c := range lay.scalars {
			v := r.Scalars[c.Name]
			switch {
			case c.Name == lay.title:
				cells = append(cells, cellText(r.Title))
			case v == nil:
				cells = append(cells, "")
			default:
				if text, ok := dw.valueText(lay, r, c, *v, line); ok {
					cells = append(cells, cellText(text))
				} else {
					cells = append(cells, "")
				}
			}
		}
		w.put("| " + strings.Join(cells, " | ") + " |\n")
	}
}

// valueText returns the text that a field or a cell writes for the value v of
// column c of row r, found at line, and whether it can be written: a link for a
// foreign key, the raw text the row was read with, or the readable form of the
// column's type.
func (dw *docWriter) valueText(lay *docLayout, r *Row, c dbspec.Column, v string, line int) (string, bool) {
	w := dw.w
	if table, isFK := lay.fks[c.Name]; isFK {
		return dw.linkText(r, c, table, v, line)
	}
	switch {
	case !utf8.ValidString(v):
		w.refuse(line, 1, c.Name+" is not UTF-8")
		return "", false
	case singleLineFault(v) != "":
		w.refuse(line, 1, c.Name+" has "+singleLineFault(v))
		return "", false
	}
	if raw, kept := r.Raw[c.Name]; kept && raw == v {
		return v, true
	}
	canon, ok := readable(c.Type, v)
	if !ok {
		w.refuse(line, 1, c.Name+" is not a "+c.Type.String()+" value")
		return "", false
	}
	return canon, true
}

// linkText writes the link of a foreign key value: its text is the referenced key
// and its target is derived from the referenced row. A key that names no row keeps
// the target it was read with, and any other is FOREIGN_KEY.
func (dw *docWriter) linkText(r *Row, c dbspec.Column, table, key string, line int) (string, bool) {
	target, err := dw.linkTarget(table, key)
	if err != nil {
		d, kept := r.Dangling[c.Name]
		if !kept || d.Key != key {
			dw.w.refuseRule(RuleForeignKey, line, 1, "the foreign key names no row: "+key)
			return "", false
		}
		target = d.Target
	}
	return "[" + key + "](" + target + ")", true
}

// linkTarget derives the target of a link to the row key of table: from this file
// for a row the file holds, else from the linker.
func (dw *docWriter) linkTarget(table, key string) (string, error) {
	if table == dw.ownTable && key == dw.ownKey {
		return path.Base(dw.w.rel), nil
	}
	if fr, ok := dw.idx[table][key]; ok {
		return localLink(fr.lay, dw.w.rel, fr.row), nil
	}
	return dw.lk.Target(dw.w.rel, table, key)
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
// column and a colon takes a backslash before the colon.
func listTitle(lay *docLayout, title string) string {
	toks := strings.Split(title, " ")
	for i, t := range toks {
		for _, n := range lay.names() {
			if strings.HasPrefix(t, n+":") {
				toks[i] = n + `\:` + t[len(n)+1:]
				break
			}
		}
	}
	return strings.Join(toks, " ")
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
