package statement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// capture records the rows a statement changes on the writer connection, from SQLite's pre-update hook (spec 40 SQL
// statements): for each table of the main database, the image of each row before the statement's first change of it,
// and the rowid of each row it inserted, updated or deleted, in the order of their first change. The hook reports the
// rows a REPLACE deletes, the rows of foreign key actions and the rows of triggers as it reports the statement's own, so
// the difference of a statement is formed from the rows it changed and not from every row. A text of an image is read
// by its length, so a text holding a NUL comes whole (polyspec's fork of the driver).
type capture struct {
	mu     sync.Mutex
	on     bool
	tables map[string]*changedRows
	err    error
}

// changedRows are the rows of one table a statement changed: olds holds the image of each row before its first change,
// by its rowid, in the order of the table's columns in the database; rowids holds each rowid the statement changed,
// old and new, in the order of their first change.
type changedRows struct {
	olds   map[int64][]any
	seen   map[int64]bool
	rowids []int64
}

// note records the change of the row at rowid.
func (t *changedRows) note(rowid int64) {
	if !t.seen[rowid] {
		t.seen[rowid] = true
		t.rowids = append(t.rowids, rowid)
	}
}

// hook is the pre-update hook of the writer connection; it records only while a statement runs.
func (cp *capture) hook(data sqlite.SQLitePreUpdateData) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if !cp.on || data.DatabaseName != "main" {
		return
	}
	t := cp.tables[data.TableName]
	if t == nil {
		t = &changedRows{olds: map[int64][]any{}, seen: map[int64]bool{}}
		cp.tables[data.TableName] = t
	}
	if data.Op != sqlite3.SQLITE_INSERT {
		if !t.seen[data.OldRowID] {
			// A row the statement inserted first has no image before it.
			image := make([]any, data.Count())
			if err := data.Old(image...); err != nil {
				cp.err = errors.Join(cp.err, err)
			}
			t.olds[data.OldRowID] = image
		}
		t.note(data.OldRowID)
	}
	if data.Op != sqlite3.SQLITE_DELETE {
		t.note(data.NewRowID)
	}
}

// start begins the record of a statement's changes.
func (cp *capture) start() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.on, cp.tables, cp.err = true, map[string]*changedRows{}, nil
}

// changes returns the changes recorded since start, which go on being recorded.
func (cp *capture) changes() (map[string]*changedRows, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.err != nil {
		return nil, &Error{codeInternal, "reading the rows a statement changed: " + cp.err.Error()}
	}
	return cp.tables, nil
}

// stop ends the record of a statement's changes.
func (cp *capture) stop() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.on, cp.tables, cp.err = false, nil, nil
}

// register makes cp the pre-update hook of the driver connection of w.
func (cp *capture) register(dc any) error {
	h, ok := dc.(sqlite.HookRegisterer)
	if !ok {
		return &Error{codeInternal, fmt.Sprintf("the driver connection %T registers no pre-update hook", dc)}
	}
	h.RegisterPreUpdateHook(cp.hook)
	return nil
}

// rowidList is the JSON array of the rowids of t, the one parameter of a read of their rows.
func (t *changedRows) rowidList() string {
	b, err := json.Marshal(t.rowids)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// tableColumns returns the names of the columns of the table name in the database, in their order, the order of the
// values of an image.
func tableColumns(ctx context.Context, db runner, name string) (_ []string, err error) {
	rows, err := db.QueryContext(ctx, "SELECT name FROM pragma_table_xinfo(?) ORDER BY cid", name)
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	defer closed(&err, rows)
	var out []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		out = append(out, col)
	}
	if err := rows.Err(); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	return out, nil
}

// image is the row of tb from the values of an image of the database, whose columns are cols, at rowid: in the order of
// the columns of tb, as rowsOf reads a row.
func image(tb *catalog.Table, cols []string, rowid int64, values []any) ([]any, error) {
	if len(values) != len(cols) {
		return nil, &Error{codeInternal, fmt.Sprintf("an image of %s holds %d values, and the table %d columns", tb.Name, len(values), len(cols))}
	}
	row := make([]any, len(tb.Columns))
	for i, col := range tb.Columns {
		if col.Name == catalog.RowIDColumn {
			row[i] = rowid
			continue
		}
		j := slices.Index(cols, col.Name)
		if j < 0 {
			return nil, &Error{codeInternal, fmt.Sprintf("the table %s of the database has no column %s", tb.Name, col.Name)}
		}
		row[i] = values[j]
	}
	return row, nil
}

// rowsAt reads the rows of tb at the rowids of t, in the order of their rowids.
func rowsAt(ctx context.Context, db runner, tb *catalog.Table, t *changedRows, k *cost.Counts) (_ [][]any, err error) {
	names := columnNames(tb)
	for i := range names {
		// Without its declared type the driver reads the stored value itself, as the hook reports it.
		names[i] = "+" + names[i]
	}
	text := "SELECT " + strings.Join(names, ", ") + " FROM " + quote(tb.Name) +
		" WHERE rowid IN (SELECT value FROM json_each(?)) ORDER BY rowid"
	rows, err := db.QueryContext(ctx, text, t.rowidList())
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	defer closed(&err, rows)
	var out [][]any
	for rows.Next() {
		row, err := scan(rows, len(names))
		if err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		k.Add(cost.DatabaseRowsRead, 1)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	return out, nil
}

// changedTables returns the rows of each table of the catalog of c that a statement changed, before and after it: the
// images the hook recorded, in the order of their rowids, and the rows the database holds at the rowids it changed.
func changedTables(ctx context.Context, db runner, c *irexec.Corpus, changed map[string]*changedRows) (before, after map[string]tableRows, err error) {
	before, after = map[string]tableRows{}, map[string]tableRows{}
	for _, tb := range c.Catalog().Tables {
		t := changed[tb.Name]
		if t == nil {
			continue
		}
		cols, err := tableColumns(ctx, db, tb.Name)
		if err != nil {
			return nil, nil, err
		}
		olds := make([]int64, 0, len(t.olds))
		for rowid := range t.olds {
			olds = append(olds, rowid)
		}
		slices.Sort(olds)
		var rows [][]any
		for _, rowid := range olds {
			row, err := image(tb, cols, rowid, t.olds[rowid])
			if err != nil {
				return nil, nil, err
			}
			rows = append(rows, row)
		}
		before[tb.Name] = keyed(tb, rows)
		now, err := rowsAt(ctx, db, tb, t, c.Cost())
		if err != nil {
			return nil, nil, err
		}
		after[tb.Name] = keyed(tb, now)
	}
	return before, after, nil
}

// keyed is rows by their key, in their order.
func keyed(tb *catalog.Table, rows [][]any) tableRows {
	t := tableRows{byKey: make(map[string][]any, len(rows)), keys: make([]string, 0, len(rows))}
	for _, row := range rows {
		key := rowKey(tb, row)
		t.byKey[key] = row
		t.keys = append(t.keys, key)
	}
	return t
}
