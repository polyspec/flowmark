// Package executor runs the IR kinds over the corpus rows: documents and checklist items as
// the first tables, with where, order and limit (spec 60-dialect). The orm integration at T5
// replaces these types with the shared IR; the op semantics stay.
package executor

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/codec"
)

// Row is one result row: column name to value.
type Row map[string]string

// Condition is one where predicate; the v1 operator set is `=` and `!=`.
type Condition struct {
	Column string
	Op     string
	Value  string
}

// Query is one read request: the kind comes from the method, the rest is shared.
type Query struct {
	Where []Condition
	Order string // a column name; the empty order is the primary key
	Limit int
}

// Corpus is the parsed corpus: the documents and the checklist items as rows.
type Corpus struct {
	documents []Row
	items     []Row
}

// Open walks the markdown documents of `root` (skipping hidden entries) and builds the rows.
func Open(root string) (*Corpus, error) {
	corpus := &Corpus{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		document, _ := codec.Parse(string(text))
		relative, err := filepath.Rel(root, path)
		if err != nil {
			relative = filepath.ToSlash(path)
		}
		corpus.documents = append(corpus.documents, Row{"doc_id": document.DocID, "title": document.Title, "path": filepath.ToSlash(relative)})
		for position, item := range document.Items {
			corpus.items = append(corpus.items, Row{
				"id":     item.ID,
				"state":  item.State,
				"title":  item.Title,
				"doc_id": document.DocID,
				"order":  strconv.Itoa(position + 1),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(corpus.documents, func(i, j int) bool { return taskIDLess(corpus.documents[i]["doc_id"], corpus.documents[j]["doc_id"]) })
	// The items stay in file order — the physical arrangement in the markdown is the ordering,
	// the way a database table orders by a column. An explicit query Order sorts on demand.
	return corpus, nil
}

func (corpus *Corpus) table(name string) ([]Row, string, error) {
	switch name {
	case "documents":
		return corpus.documents, "doc_id", nil
	case "items":
		return corpus.items, "id", nil
	}
	return nil, "", fmt.Errorf("the table %q is not in the corpus", name)
}

// All returns every row of the table the query selects, ordered.
func (corpus *Corpus) All(table string, query Query) ([]Row, error) {
	rows, _, err := corpus.table(table)
	if err != nil {
		return nil, err
	}
	return selectRows(rows, query), nil
}

// One returns the single row the query selects, or found false.
func (corpus *Corpus) One(table string, query Query) (Row, bool, error) {
	rows, _, err := corpus.table(table)
	if err != nil {
		return nil, false, err
	}
	found := selectRows(rows, query)
	if len(found) == 0 {
		return nil, false, nil
	}
	return found[0], true, nil
}

// Count returns the number of rows the query selects.
func (corpus *Corpus) Count(table string, query Query) (int, error) {
	rows, _, err := corpus.table(table)
	if err != nil {
		return 0, err
	}
	return len(selectRows(rows, query)), nil
}

// selectRows applies the where predicates, the order and the limit of the query.
func selectRows(rows []Row, query Query) []Row {
	out := make([]Row, 0, len(rows))
	for _, row := range rows {
		if holds(row, query.Where) {
			out = append(out, row)
		}
	}
	if query.Order != "" {
		column := query.Order
		sort.SliceStable(out, func(i, j int) bool { return taskIDLess(out[i][column], out[j][column]) })
	}
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[:query.Limit]
	}
	return out
}

func holds(row Row, conditions []Condition) bool {
	for _, condition := range conditions {
		value, ok := row[condition.Column]
		switch condition.Op {
		case "=":
			if !ok || value != condition.Value {
				return false
			}
		case "!=":
			if ok && value == condition.Value {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// taskIDLess orders two task-ID values segment-wise numeric — T8.0.9 before T8.0.10 — with
// the suffix comparing numerically after its base path (spec 10-canonical-format). Values
// that are not task IDs compare by codepoint.
func taskIDLess(a, b string) bool {
	as, aok := splitTaskID(a)
	bs, bok := splitTaskID(b)
	if !aok || !bok {
		return a < b
	}
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] != bs[i] {
			return as[i] < bs[i]
		}
	}
	if len(as) != len(bs) {
		return len(as) < len(bs)
	}
	return false
}

// splitTaskID splits a task ID into its numeric segments; T8.0.14-1 is [8 0 14 1].
func splitTaskID(id string) ([]int, bool) {
	if len(id) < 2 || id[0] != 'T' {
		return nil, false
	}
	parts := strings.FieldsFunc(id[1:], func(r rune) bool { return r == '.' || r == '-' })
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		number := 0
		for _, r := range part {
			if r < '0' || r > '9' {
				return nil, false
			}
			number = number*10 + int(r-'0')
		}
		numbers = append(numbers, number)
	}
	return numbers, true
}

// Sum returns the sum of the integer values of a column over the selected rows.
func (corpus *Corpus) Sum(table, column string, query Query) (int, error) {
	rows, _, err := corpus.table(table)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, row := range selectRows(rows, query) {
		number, err := intValue(row[column])
		if err != nil {
			return 0, fmt.Errorf("the column %q of %v is not an integer: %w", column, row, err)
		}
		total += number
	}
	return total, nil
}

// Avg returns the integer average of a column over the selected rows.
func (corpus *Corpus) Avg(table, column string, query Query) (int, error) {
	rows, _, err := corpus.table(table)
	if err != nil {
		return 0, err
	}
	selected := selectRows(rows, query)
	if len(selected) == 0 {
		return 0, nil
	}
	total := 0
	for _, row := range selected {
		number, err := intValue(row[column])
		if err != nil {
			return 0, fmt.Errorf("the column %q of %v is not an integer: %w", column, row, err)
		}
		total += number
	}
	return total / len(selected), nil
}

// GroupCount returns the row count per distinct value of a column.
func (corpus *Corpus) GroupCount(table, column string, query Query) (map[string]int, error) {
	rows, _, err := corpus.table(table)
	if err != nil {
		return nil, err
	}
	groups := map[string]int{}
	for _, row := range selectRows(rows, query) {
		groups[row[column]]++
	}
	return groups, nil
}

// Paginate returns one page of rows — Limit is the page size — with the total row count.
func (corpus *Corpus) Paginate(table string, query Query) ([]Row, int, error) {
	rows, _, err := corpus.table(table)
	if err != nil {
		return nil, 0, err
	}
	selected := selectRows(rows, Query{Where: query.Where, Order: query.Order})
	if query.Limit > 0 && len(selected) > query.Limit {
		selected = selected[:query.Limit]
	}
	return selected, len(selectRows(rows, Query{Where: query.Where})), nil
}

// intValue parses an integer column value.
func intValue(value string) (int, error) {
	number := 0
	if value == "" {
		return 0, fmt.Errorf("the value is empty")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("the value %q holds a non-digit", value)
		}
		number = number*10 + int(r-'0')
	}
	return number, nil
}
