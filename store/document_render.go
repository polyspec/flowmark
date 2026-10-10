package store

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/orm/engine/dbspec"
)

// RenderDocument writes the document file of the rows of a document table. The rows
// may come in any order: the file takes their storage order. A row set the writer
// refuses returns no bytes and diagnostics, located at the row's place in the given
// order; two rows whose keys are equal without regard to case are DUPLICATE_KEY.
// A table whose catalog declares a child block is refused with an error: the rows
// of the block are not in the rows given, so writing would drop them, and the block
// sections are the layout of T12.2.4.
func RenderDocument(cat *catalog.Catalog, table string, rows []*Row, lk Linker) ([]byte, []Diagnostic, error) {
	lay, err := docLayoutOf(cat, table)
	if err != nil {
		return nil, nil, err
	}
	if len(lay.children) > 0 {
		return nil, nil, fmt.Errorf("%s: table %s stores the child block %s, which a write refuses: the block sections are not implemented until T12.2.4", docFileName(lay), table, lay.children[0])
	}
	out, diags := renderDocument(lay, rows, lk, true)
	return out, diags, nil
}

// renderDocument writes the document of rows in its canonical form. A write
// refuses two rows whose keys are equal without regard to case; the read check of
// ParseDocument does not, since the validator reports those keys.
func renderDocument(lay *docLayout, rows []*Row, lk Linker, write bool) ([]byte, []Diagnostic) {
	w := &writer{rel: docFileName(lay)}
	rowLine := func(i int) int {
		if lay.list {
			return 2 + i
		}
		return 4 + i
	}
	byKey := map[string]int{}
	for i, r := range rows {
		if _, dup := byKey[r.Key]; !dup {
			byKey[r.Key] = i
		}
	}
	lower := map[string]int{}
	for i, r := range rows {
		w.checkRow(lay, byKey, r, rowLine(i))
		if !write {
			continue
		}
		k := strings.ToLower(r.Key)
		if first, dup := lower[k]; dup {
			w.refuseRule(RuleDuplicateKey, rowLine(i), 1, "the key "+r.Key+" equals the key of row "+strconv.Itoa(first+1)+" without regard to case")
		} else {
			lower[k] = i
		}
	}
	if len(w.diags) > 0 {
		return nil, w.diags
	}
	checkOrder(w, lay, rows, rowLine)
	if len(w.diags) > 0 {
		return nil, w.diags
	}
	order, depth, unreached := storageOrder(lay, rows)
	if len(unreached) > 0 {
		w.refuse(rowLine(unreached[0]), 1, "the parents of the rows form a cycle")
		return nil, w.diags
	}
	w.put("# " + lay.table + "\n")
	if len(rows) > 0 {
		if lay.list {
			w.writeList(lay, rows, order, depth, lk)
		} else {
			w.writeTable(lay, rows, order, lk)
		}
		w.writeDetails(lay, rows, order, lk)
	}
	if len(w.diags) > 0 {
		return nil, w.diags
	}
	return []byte(w.b.String()), nil
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
func (w *writer) writeList(lay *docLayout, rows []*Row, order, depth []int, lk Linker) {
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
			if text, ok := w.valueText(lay, rows, r, c, *v, line, lk); ok {
				b.WriteString(" " + listToken(c.Name, text))
			}
		}
		w.put(b.String() + "\n")
	}
}

// writeTable writes the header, the delimiter row and the rows, in storage order.
func (w *writer) writeTable(lay *docLayout, rows []*Row, order []int, lk Linker) {
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
				if text, ok := w.valueText(lay, rows, r, c, *v, line, lk); ok {
					cells = append(cells, cellText(text))
				} else {
					cells = append(cells, "")
				}
			}
		}
		w.put("| " + strings.Join(cells, " | ") + " |\n")
	}
}

// docPiece is one element after the list or table: a heading line, or a value
// that follows one.
type docPiece struct {
	isValue bool
	head    string // the heading line, with its line feed, when the piece is a heading
	name    string // what the value is, for a diagnostic
	value   string
}

// writeDetails writes the detail section of each row that has one, in storage
// order: the heading, the body, then the sections in column order. Every element
// is set off by one blank line from the element before it.
func (w *writer) writeDetails(lay *docLayout, rows []*Row, order []int, lk Linker) {
	var pieces []docPiece
	for _, i := range order {
		r := rows[i]
		if !hasDetail(lay, r) {
			continue
		}
		pieces = append(pieces, docPiece{head: "## " + lay.table + " row " + r.Key + "\n"})
		if r.Body != "" {
			pieces = append(pieces, docPiece{isValue: true, name: "the body", value: r.Body})
		}
		for _, s := range lay.sections {
			v := r.Sections[s.name]
			if v == nil {
				continue
			}
			pieces = append(pieces, docPiece{head: "### " + s.name + "\n"})
			if *v != "" {
				pieces = append(pieces, docPiece{isValue: true, name: "the section " + s.name, value: *v})
			}
		}
	}
	for k, pc := range pieces {
		w.put("\n")
		if !pc.isValue {
			w.put(pc.head)
			continue
		}
		w.value(pc.name, pc.value, w.next(), k < len(pieces)-1, false, false, 3)
		w.put(pc.value)
	}
}

// valueText returns the text that a field or a cell writes for the value v of
// column c of row r, found at line, and whether it can be written: a link for a
// foreign key, the raw text the row was read with, or the readable form of the
// column's type.
func (w *writer) valueText(lay *docLayout, rows []*Row, r *Row, c dbspec.Column, v string, line int, lk Linker) (string, bool) {
	if table, isFK := lay.fks[c.Name]; isFK {
		return w.linkText(lay, rows, r, c, table, v, line, lk)
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
func (w *writer) linkText(lay *docLayout, rows []*Row, r *Row, c dbspec.Column, table, key string, line int, lk Linker) (string, bool) {
	target, err := w.linkTarget(lay, rows, table, key, lk)
	if err != nil {
		d, kept := r.Dangling[c.Name]
		if !kept || d.Key != key {
			w.refuseRule(RuleForeignKey, line, 1, "the foreign key names no row: "+key)
			return "", false
		}
		target = d.Target
	}
	return "[" + key + "](" + target + ")", true
}

// linkTarget derives the target of a link to the row key of table: from this file
// for a row of the document's own table, else from the linker.
func (w *writer) linkTarget(lay *docLayout, rows []*Row, table, key string, lk Linker) (string, error) {
	if table == lay.table {
		for _, r := range rows {
			if r.Key == key {
				return localLink(lay, docFileName(lay), r), nil
			}
		}
		return "", fmt.Errorf("no row %s %s in %s", table, key, lay.table)
	}
	return lk.Target(docFileName(lay), table, key)
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
