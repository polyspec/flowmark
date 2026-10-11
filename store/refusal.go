package store

// Refusal is a fault of a refused write. A refused write writes no bytes, so its
// faults are located in the request, not in a file: Table and Key name the row the
// rule refuses, Column the column whose value the rule names, and "" a fault of the
// whole row. Index is the 0-based position of the row among the rows given to the
// write when its key is shared byte for byte with another row given, and -1 otherwise.
type Refusal struct {
	Table, Key string
	Index      int
	Column     string
	Rule       string
	Message    string
}

// elementAt is where the parser read an element of a row: the 1-based line and the
// 1-based column, in code points, of its first character.
type elementAt struct{ line, col int }

// setAt records where the parser read column of row r: its element at line and col.
func (r *Row) setAt(column string, line, col int) {
	if r.at == nil {
		r.at = map[string]elementAt{}
	}
	r.at[column] = elementAt{line: line, col: col}
}

// readDiagnostics returns the read diagnostics of the refusals that the writer makes of
// the rows read from rel. A refusal is located where the parser read the element its
// column names; a whole-row refusal, and a column the parser did not read, at the row's
// first line. The refusal names its row and column.
func readDiagnostics(rel string, rf []Refusal, rows []*Row) []Diagnostic {
	byRow := map[string]*Row{}
	var index func(rs []*Row)
	index = func(rs []*Row) {
		for _, r := range rs {
			if _, ok := byRow[r.Table+" "+r.Key]; !ok {
				byRow[r.Table+" "+r.Key] = r
			}
			for _, bs := range r.Blocks {
				index(bs)
			}
		}
	}
	index(rows)
	diags := make([]Diagnostic, 0, len(rf))
	for _, r := range rf {
		at := "the whole row"
		if r.Column != "" {
			at = "column " + r.Column
		}
		loc := elementAt{line: 1, col: 1}
		if row := byRow[r.Table+" "+r.Key]; row != nil {
			if a, ok := row.at[r.Column]; ok && r.Column != "" {
				loc = a
			} else if a, ok := row.at[""]; ok {
				loc = a
			}
		}
		diags = append(diags, Diagnostic{File: rel, Line: loc.line, Column: loc.col, Rule: r.Rule,
			Message: r.Message + " (" + r.Table + " " + r.Key + ", " + at + ")"})
	}
	return diags
}

// rowRef names the row that a write is writing, by table and key, with its index
// among the rows given (-1 unless its key is shared byte for byte).
type rowRef struct {
	table, key string
	index      int
}

// rowRefOf returns the reference of row r, which is at index i among the rows given.
func rowRefOf(r *Row, i int) rowRef { return rowRef{table: r.Table, key: r.Key, index: i} }

// origin is the row and the column of the text written on a line: the writer keeps
// the origin of every line, so that a diagnostic of the written bytes can name the
// row and column that wrote them.
type origin struct {
	row    rowRef
	column string
}

// colSpan is the origin of the text that a line holds from the byte offset at on, up to
// the next span of the line. A line of a row holds several columns, a table row all of
// its cells, so the writer keeps the origin of each piece of text it writes.
type colSpan struct {
	at int
	o  origin
}

// originAt returns the origin of the byte at offset at of a line, by its spans.
func originAt(spans []colSpan, at int) origin {
	var o origin
	for _, s := range spans {
		if s.at > at {
			break
		}
		o = s.o
	}
	return o
}

// refusal returns the refusal of rule at column of the row r.
func (r rowRef) refusal(column, rule, msg string) Refusal {
	return Refusal{Table: r.table, Key: r.key, Index: r.index, Column: column, Rule: rule, Message: msg}
}
