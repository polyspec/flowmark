package codec

import (
	"fmt"
	"regexp"
	"strings"
)

// ChecklistItem is one row of a checklist block: the kit grammar `- [state] ID text` with its
// indented continuation lines (spec 20-document-model).
type ChecklistItem struct {
	State         string
	ID            string
	Title         string
	Line          int
	Indent        int
	Continuations []string
}

// Table is one markdown table block: a header row, a separator and data rows. The header
// row the document writes is the definition of the columns.
type Table struct {
	Line     int
	Headers  []string
	Rows     [][]string
	RowLines []int // the absolute line of each data row
}

// Column finds a column by header name, case-insensitive; -1 when the header holds none.
func (t *Table) Column(name string) int {
	for i, header := range t.Headers {
		if strings.EqualFold(header, name) {
			return i
		}
	}
	return -1
}

// ItemsTable finds the items table of a document: a markdown table whose header row names
// the state, id and title columns — the table form of the checklist.
func ItemsTable(document *Document) *Table {
	for i := range document.Tables {
		table := &document.Tables[i]
		if table.Column("state") >= 0 && table.Column("id") >= 0 && table.Column("title") >= 0 {
			return table
		}
	}
	return nil
}

var itemLine = regexp.MustCompile(`^( *)- (\[[^]]+\]) (\S+)\s*(.*)$`)

// scanBlocks walks the unfenced payload lines and collects the checklist items and the
// markdown tables with their absolute line numbers. The blocks live in the payload: this
// scan changes nothing about the rendered document.
func scanBlocks(lines []string, fenced []bool) (items []ChecklistItem, tables []Table, diagnostics []Diagnostic) {
	lastItem := -1
	at := 0
	for at < len(lines) {
		if fenced[at] || strings.TrimSpace(lines[at]) == "" {
			at++
			continue
		}
		if match := itemLine.FindStringSubmatch(lines[at]); match != nil {
			item := ChecklistItem{
				Indent: len(match[1]),
				State:  match[2],
				ID:     match[3],
				Title:  match[4],
				Line:   at + 1,
			}
			items = append(items, item)
			lastItem = len(items) - 1
			at++
			// Continuation lines: indented deeper than the item and not items themselves.
			for at < len(lines) && !fenced[at] && strings.HasPrefix(lines[at], " ") {
				if itemLine.FindStringSubmatch(lines[at]) != nil {
					break
				}
				if strings.TrimSpace(lines[at]) == "" {
					break
				}
				items[lastItem].Continuations = append(items[lastItem].Continuations, lines[at])
				at++
			}
			continue
		}
		if strings.HasPrefix(lines[at], "- [") {
			diagnostics = append(diagnostics, Diagnostic{Line: at + 1, Column: 1, Rule: RuleBlock,
				Message: fmt.Sprintf("the checklist line %q is an item \"- [state] ID text\"", strings.TrimSpace(lines[at]))})
			at++
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(lines[at]), "|") {
			table, next := scanTable(lines, at)
			if len(table.Headers) > 0 {
				tables = append(tables, table)
			}
			at = next
			continue
		}
		at++
	}
	return items, tables, diagnostics
}

// scanTable reads one markdown table from `at`: header row, separator, data rows.
func scanTable(lines []string, at int) (Table, int) {
	table := Table{Line: at + 1}
	for at < len(lines) {
		trimmed := strings.TrimSpace(lines[at])
		if !strings.HasPrefix(trimmed, "|") {
			break
		}
		cells := splitCells(trimmed)
		if isSeparator(cells) {
			at++
			continue
		}
		if table.Headers == nil {
			table.Headers = cells
		} else {
			table.Rows = append(table.Rows, cells)
			table.RowLines = append(table.RowLines, at+1)
		}
		at++
	}
	return table, at
}

// splitCells splits one table row into its trimmed cells.
func splitCells(row string) []string {
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	parts := strings.Split(row, "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells
}

// isSeparator names the separator row of a table.
func isSeparator(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !regexp.MustCompile(`^:?-{3,}:?$`).MatchString(cell) {
			return false
		}
	}
	return true
}
