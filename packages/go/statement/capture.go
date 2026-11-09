package statement

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/min-0-max/sqlite"
	sqlite3 "github.com/min-0-max/sqlite/lib"
	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/irexec"
)

// capture records the rows a statement changes on the writer connection, from SQLite's pre-update hook (spec 40 SQL
// statements): for each table of the main database, the image of each row before the statement's first change of it,
// and the identity of each row it inserted, updated or deleted, in the order of their first change. The identity of a
// row is its rowid, and of a row of a table without a rowid, whose rowid SQLite leaves undefined, its primary key. The
// hook reports the rows a REPLACE deletes, the rows of foreign key actions and the rows of triggers as it reports the
// statement's own, so the difference of a statement is formed from the rows it changed and not from every row. A text
// of an image is read by its length, so a text holding a NUL comes whole (the fork of the driver the engine pins).
type capture struct {
	mu     sync.Mutex
	on     bool
	tables map[string]*changedRows
	err    error
	// schema is the main schema_version the keys were read at, keys the primary key of each table without a rowid, names
	// the name a statement gives the rowid of each table with one, and ids the tables keyed by their rowid.
	schema int64
	keys   map[string]*primaryKey
	names  rowids
	ids    rowids
	// largest is what the hook follows of the largest rowid while a statement runs (queryLargest), nil outside it.
	largest *largest
}

// primaryKey is the primary key of a table without a rowid, in the order of its index: the position of each of its
// columns among the values of an image, its name, its collation and whether it is descending.
type primaryKey struct {
	at    []int
	names []string
	colls []string
	desc  []bool
}

// changedRows are the rows of one table a statement changed: rowid is the name a statement gives the rowid of a table
// with one, and key the primary key of one without; olds holds the image of each row before its first change,
// by its identity, in the order of the table's columns in the database; ids holds the identity values of each row the
// statement changed, old and new, in the order of their first change: its rowid, or the values of key; and rowids the
// rowid of each, for a table with a rowid.
type changedRows struct {
	rowid  string
	key    *primaryKey
	olds   map[string][]any
	seen   map[string]bool
	ids    [][]any
	rowids []int64
}

// identity is the identity of the row whose identity values are vs: two rows have the same identity exactly when each
// value is of the same type and equal.
func identity(vs []any) string {
	var b strings.Builder
	for _, v := range vs {
		switch x := v.(type) {
		case nil:
			b.WriteString("n;")
		case int64:
			b.WriteString("i" + strconv.FormatInt(x, 10) + ";")
		case float64:
			b.WriteString("f" + strconv.FormatUint(math.Float64bits(x), 16) + ";")
		case string:
			b.WriteString("s" + strconv.Itoa(len(x)) + ":" + x)
		case []byte:
			b.WriteString("b" + strconv.Itoa(len(x)) + ":" + string(x))
		default:
			fmt.Fprintf(&b, "%T:%v;", v, v)
		}
	}
	return b.String()
}

// note records the change of the row whose identity values are vs, at rowid in a table with a rowid.
func (t *changedRows) note(vs []any, rowid int64) {
	if id := identity(vs); !t.seen[id] {
		t.seen[id] = true
		t.ids = append(t.ids, vs)
		if t.key == nil {
			t.rowids = append(t.rowids, rowid)
		}
	}
}

// keyOf is the values of the primary key k in the image values.
func keyOf(k *primaryKey, values []any) []any {
	out := make([]any, len(k.at))
	for i, at := range k.at {
		out[i] = values[at]
	}
	return out
}

// hook is the pre-update hook of the writer connection; it records only while a statement runs.
func (cp *capture) hook(data sqlite.SQLitePreUpdateData) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if !cp.on {
		return
	}
	if cp.largest != nil {
		var err error
		switch {
		case data.DatabaseName == "temp" && data.TableName == "flowmark_largest" && data.Op == sqlite3.SQLITE_INSERT:
			err = cp.largest.noted(data)
		case data.DatabaseName == "main":
			err = cp.largest.changed(data)
		}
		cp.err = errors.Join(cp.err, err)
	}
	if data.DatabaseName != "main" {
		return
	}
	t := cp.tables[data.TableName]
	if t == nil {
		t = &changedRows{rowid: cp.names[data.TableName], key: cp.keys[data.TableName], olds: map[string][]any{}, seen: map[string]bool{}}
		cp.tables[data.TableName] = t
	}
	if data.Op != sqlite3.SQLITE_INSERT {
		image := make([]any, data.Count())
		if err := data.Old(image...); err != nil {
			cp.err = errors.Join(cp.err, err)
		}
		vs := []any{data.OldRowID}
		if t.key != nil {
			vs = keyOf(t.key, image)
		}
		// A row the statement inserted first has no image before it.
		if id := identity(vs); !t.seen[id] {
			t.olds[id] = image
		}
		t.note(vs, data.OldRowID)
	}
	if data.Op != sqlite3.SQLITE_DELETE {
		vs := []any{data.NewRowID}
		if t.key != nil {
			values := make([]any, data.Count())
			if err := data.New(values...); err != nil {
				cp.err = errors.Join(cp.err, err)
			}
			vs = keyOf(t.key, values)
		}
		t.note(vs, data.NewRowID)
	}
}

// prepare reads, when the schema of the main database changed since it last did, the primary key of each table without
// a rowid, which the hook takes a row's identity from, and the tables keyed by their rowid (readRowids). It runs on db
// before a statement, as the hook cannot read.
func (cp *capture) prepare(ctx context.Context, db runner) error {
	var version int64
	if err := db.QueryRowContext(ctx, "PRAGMA main.schema_version").Scan(&version); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	cp.mu.Lock()
	known := cp.keys != nil && cp.schema == version
	cp.mu.Unlock()
	if known {
		return nil
	}
	names, err := column[string](ctx, db, "SELECT name FROM pragma_table_list WHERE schema = 'main' AND type = 'table' AND wr = 1", nil)
	if err != nil {
		return err
	}
	keys := map[string]*primaryKey{}
	for _, name := range names {
		if keys[name], err = primaryKeyOf(ctx, db, name); err != nil {
			return err
		}
	}
	rowidNames, ids, err := readRowids(ctx, db)
	if err != nil {
		return err
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.schema, cp.keys, cp.names, cp.ids = version, keys, rowidNames, ids
	return nil
}

// rowidTables are the tables keyed by their rowid that prepare read.
func (cp *capture) rowidTables() rowids {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	return cp.ids
}

// follow makes l what the hook follows of the largest rowid, nil for none.
func (cp *capture) follow(l *largest) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.largest = l
}

// failed is the fault the hook recorded since start, nil for none.
func (cp *capture) failed() error {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.err != nil {
		return &Error{codeInternal, "reading the rows a statement changed: " + cp.err.Error()}
	}
	return nil
}

// primaryKeyOf reads the primary key of the table name without a rowid from its index.
func primaryKeyOf(ctx context.Context, db runner, name string) (_ *primaryKey, err error) {
	cols, err := tableColumns(ctx, db, name)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, "SELECT x.name, x.coll, x.desc FROM pragma_index_list(?1) AS l, pragma_index_xinfo(l.name) AS x"+
		" WHERE l.origin = 'pk' AND x.key = 1 ORDER BY x.seqno", name)
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	defer closed(&err, rows)
	k := &primaryKey{}
	for rows.Next() {
		var col, coll string
		var desc bool
		if err := rows.Scan(&col, &coll, &desc); err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		at := slices.Index(cols, col)
		if at < 0 {
			return nil, &Error{codeInternal, fmt.Sprintf("the primary key of %s names no column %s", name, col)}
		}
		k.at, k.names, k.colls, k.desc = append(k.at, at), append(k.names, col), append(k.colls, coll), append(k.desc, desc)
	}
	if err := rows.Err(); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	if len(k.at) == 0 {
		return nil, &Error{codeInternal, "the table " + name + " without a rowid has no primary key"}
	}
	return k, nil
}

// order is the ORDER BY of the rows of a table in the order of the primary key k, each term prefixed by prefix.
func (k *primaryKey) order(prefix string, names []string) string {
	terms := make([]string, len(names))
	for i, name := range names {
		terms[i] = prefix + name + " COLLATE " + quote(k.colls[i])
		if k.desc[i] {
			terms[i] += " DESC"
		}
	}
	return " ORDER BY " + strings.Join(terms, ", ")
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

// rowidList is the JSON array of the rowids of t, a table with a rowid, the one parameter of a read of their rows.
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
// the columns of tb, as rowsOf reads a row, with the rowid as the column rowid that the catalog gives a table keyed by
// it, which the database does not hold.
func image(tb *catalog.Table, cols []string, rowid int64, values []any) ([]any, error) {
	if len(values) != len(cols) {
		return nil, &Error{codeInternal, fmt.Sprintf("an image of %s holds %d values, and the table %d columns", tb.Name, len(values), len(cols))}
	}
	row := make([]any, len(tb.Columns))
	for i, col := range tb.Columns {
		j := slices.Index(cols, col.Name)
		// The column rowid the catalog gives a table keyed by its rowid is no column of the database: it is the rowid.
		if j < 0 && col.Name == catalog.RowIDColumn {
			row[i] = rowid
			continue
		}
		if j < 0 {
			return nil, &Error{codeInternal, fmt.Sprintf("the table %s of the database has no column %s", tb.Name, col.Name)}
		}
		row[i] = values[j]
	}
	return row, nil
}

// readColumns are the columns of tb as a read of its rows names them: without its declared type the driver reads the
// stored value itself, as the hook reports it.
func readColumns(tb *catalog.Table) string {
	names := columnNames(tb)
	for i := range names {
		names[i] = "+" + names[i]
	}
	return strings.Join(names, ", ")
}

// rowsAt reads the rows of tb at the identities of t: of a table with a rowid in the order of their rowids, and of a
// table without one in the order of its primary key, whose values keys holds.
func rowsAt(ctx context.Context, db runner, tb *catalog.Table, t *changedRows, keys string, k *cost.Counts) (_ [][]any, err error) {
	text := "SELECT " + readColumns(tb) + " FROM " + quote(tb.Name)
	var args []any
	if t.key == nil {
		if t.rowid == "" {
			return nil, unnamedRowid(tb.Name)
		}
		text += " WHERE " + t.rowid + " IN (SELECT value FROM json_each(?)) ORDER BY " + t.rowid
		args = []any{t.rowidList()}
	} else {
		cols := quoted(t.key.names)
		text += " WHERE (" + strings.Join(cols, ", ") + ") IN (SELECT " + keyColumnList(len(cols)) + " FROM " + keys + ")" +
			t.key.order("", cols)
	}
	return readRows(ctx, db, tb, text, args, k)
}

// quoted are the names, each quoted as an identifier.
func quoted(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = quote(name)
	}
	return out
}

// readRows runs the query text of the columns readColumns names of tb with args and returns its rows, each counted as a
// row read from the database.
func readRows(ctx context.Context, db runner, tb *catalog.Table, text string, args []any, k *cost.Counts) ([][]any, error) {
	return queryRows(ctx, db, text, args, k, func(rows *sql.Rows) ([]any, error) { return scan(rows, len(tb.Columns)) })
}

// keyColumnList is the columns k1, ..., kn of a table of keys.
func keyColumnList(n int) string {
	cols := make([]string, n)
	for i := range cols {
		cols[i] = "k" + strconv.Itoa(i+1)
	}
	return strings.Join(cols, ", ")
}

// keysTable fills the temporary table of the identities of t, a table without a rowid, whose rows are i, the position
// of an identity in ids, and its values k1, ..., kn, and returns its name: the database orders and matches the values
// of a primary key under its collations, which the engine does not repeat. The hook records nothing of the temporary
// database, and the table is dropped once the rows of t are read, on every path, so no statement and no read of the
// schema finds it.
func keysTable(ctx context.Context, db runner, n int, ids [][]any) (_ string, err error) {
	name := "temp.flowmark_keys_" + strconv.Itoa(n)
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+name+" (i INTEGER, "+keyColumnList(n)+")"); err != nil {
		return "", &Error{codeInternal, err.Error()}
	}
	insert, err := db.PrepareContext(ctx, "INSERT INTO "+name+" VALUES (?"+strings.Repeat(", ?", n)+")")
	if err != nil {
		return "", &Error{codeInternal, err.Error()}
	}
	defer closed(&err, insert)
	for i, vs := range ids {
		if _, err := insert.ExecContext(ctx, append([]any{i}, vs...)...); err != nil {
			return "", &Error{codeInternal, err.Error()}
		}
	}
	return name, nil
}

// changedTables returns the rows of each table of the catalog of c that a statement changed, before and after it: the
// images the hook recorded and the rows the database holds at the identities it changed, both in the order of the
// table's rowids, or of its primary key for a table without a rowid.
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
		if err := tableChange(ctx, db, c, tb, t, cols, before, after); err != nil {
			return nil, nil, err
		}
	}
	return before, after, nil
}

// tableChange puts the rows of the table tb that the statement changed, t, before and after it, into before and after.
func tableChange(ctx context.Context, db runner, c *irexec.Corpus, tb *catalog.Table, t *changedRows, cols []string, before, after map[string]tableRows) (err error) {
	keys := ""
	if t.key != nil {
		// The table of the identities goes with the rows it served, so no statement and no read of the schema finds it.
		defer func() {
			err = errors.Join(err, dropKeys(ctx, db, len(t.key.at)))
		}()
		if keys, err = keysTable(ctx, db, len(t.key.at), t.ids); err != nil {
			return err
		}
	}
	olds, err := oldOrder(ctx, db, t, keys)
	if err != nil {
		return err
	}
	var rows [][]any
	for _, i := range olds {
		var rowid int64
		if t.key == nil {
			rowid = t.rowids[i]
		}
		row, err := image(tb, cols, rowid, t.olds[identity(t.ids[i])])
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	before[tb.Name] = keyed(tb, rows)
	now, err := rowsAt(ctx, db, tb, t, keys, c.Cost())
	if err != nil {
		return err
	}
	after[tb.Name] = keyed(tb, now)
	return nil
}

// dropKeys drops the temporary table of the identities of n values that keysTable made, when it stands.
func dropKeys(ctx context.Context, db runner, n int) error {
	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS temp.flowmark_keys_"+strconv.Itoa(n)); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	return nil
}

// oldOrder returns the positions in t.ids of the rows of t that have an image, in the order of their rowids, or of the
// primary key for a table without a rowid, whose identities the table keys holds.
func oldOrder(ctx context.Context, db runner, t *changedRows, keys string) ([]int, error) {
	var at []int
	for i, vs := range t.ids {
		if _, ok := t.olds[identity(vs)]; ok {
			at = append(at, i)
		}
	}
	if t.key == nil {
		slices.SortFunc(at, func(a, b int) int { return cmp.Compare(t.rowids[a], t.rowids[b]) })
		return at, nil
	}
	if len(at) < 2 {
		return at, nil
	}
	cols := strings.Split(keyColumnList(len(t.key.at)), ", ")
	list, err := json.Marshal(at)
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	order, err := column[int64](ctx, db, "SELECT i FROM "+keys+" WHERE i IN (SELECT value FROM json_each(?))"+t.key.order("", cols), nil, string(list))
	if err != nil {
		return nil, err
	}
	out := make([]int, len(order))
	for j, i := range order {
		out[j] = int(i)
	}
	return out, nil
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
