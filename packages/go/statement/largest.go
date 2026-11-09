package statement

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/min-0-max/sqlite"
	sqlite3 "github.com/min-0-max/sqlite/lib"
	"github.com/polyspec/flowmark/packages/go/irexec"
)

// The largest key (spec 40 SQL statements): a table whose key is its rowid gives an insert one more than its largest
// rowid, and when that is the largest int64, SQLite picks a random unused rowid, which no other database gives — MySQL and
// PostgreSQL refuse an identity past its maximum, as SQLite refuses an AUTOINCREMENT table. The engine refuses it as
// DRIVER (SQLITE_FULL). SQLite gives a BEFORE INSERT trigger the rowid -1 for a rowid it chooses, and for a rowid -1 or
// NULL the insert gives, so a guard, one temporary BEFORE INSERT trigger on each table that may hold the largest int64,
// notes for each row in temp.flowmark_largest whether its rowid was -1. The pre-update hook takes the note at the depth of
// the trigger and pairs it with the insert of its row, one level above: inserts nest, so a note pairs with the next
// insert of its table at its row's depth, and a row a conflict ignores leaves its note to the next row of that depth,
// which notes again. The hook follows which tables hold the largest int64 from the corpus and the rows the statement
// changes, as SQLite chooses the rowid of a row after its BEFORE triggers ran and before a REPLACE deletes what it meets;
// a row whose rowid SQLite chose while its table held the largest int64, and whose rowid is not -1, fails the statement.
// The guard stands only while its statement runs: it is made after a savepoint before the statement and taken away after
// it, so no other statement and no read of the schema finds it. A table that comes to hold the largest int64 during a
// statement whose table was not guarded has the statement run again, from its savepoint, with the guard.

// largestRowid is the largest int64, the largest rowid of SQLite.
const largestRowid = math.MaxInt64

// largestSavepoint is the savepoint a statement runs after, so that it can run again with the guard.
const largestSavepoint = "flowmark_largest"

// guardPrefix is the prefix of the names of the triggers of the guard.
const guardPrefix = "flowmark_largest_"

// rowids are the tables of the main database whose key is their rowid, by name, with the name a statement gives their
// rowid.
type rowids map[string]string

// readRowids reads the tables of db that have a rowid, with the name a statement gives it, and those of them keyed by
// it.
func readRowids(ctx context.Context, db runner) (names, keyed rowids, err error) {
	tables, err := column[string](ctx, db, "SELECT name FROM pragma_table_list WHERE schema = 'main' AND type = 'table' AND wr = 0 AND name NOT GLOB 'sqlite_*' ORDER BY name", nil)
	if err != nil {
		return nil, nil, err
	}
	names, keyed = rowids{}, rowids{}
	for _, table := range tables {
		name, key, err := rowidName(ctx, db, table)
		if err != nil {
			return nil, nil, err
		}
		names[table] = name
		if key {
			keyed[table] = name
		}
	}
	return names, keyed, nil
}

// rowidName is the name a statement gives the rowid of the table, a table with a rowid, and whether the rowid is the
// table's key, as the catalog keys a table (spec 40 SQL statements, Tables). The name is the first of rowid, _rowid_
// and oid that names no column, as SQLite lets each name the rowid while no column takes it, or else the column of an
// INTEGER PRIMARY KEY, which is the rowid; "" when there is none, a table the catalog refuses (unnamedRowid). The rowid
// is the key of a table of an INTEGER PRIMARY KEY, of one without a primary key and of one whose primary key SQLite
// lets hold NULL; the corpus holds the rowid of the last two as their column rowid, a name the catalog lets no column of
// theirs take. A table keyed otherwise keeps a rowid the corpus does not hold.
func rowidName(ctx context.Context, db runner, table string) (_ string, keyed bool, err error) {
	rows, err := db.QueryContext(ctx, "SELECT name, type, \"notnull\", pk FROM pragma_table_info(?)", table)
	if err != nil {
		return "", false, &Error{codeInternal, err.Error()}
	}
	defer closed(&err, rows)
	var keys []string
	integer, nullable := false, false
	taken := map[string]bool{}
	for rows.Next() {
		var name, typ string
		var notNull, pk int
		if err := rows.Scan(&name, &typ, &notNull, &pk); err != nil {
			return "", false, &Error{codeInternal, err.Error()}
		}
		taken[strings.ToLower(name)] = true
		if pk > 0 {
			keys = append(keys, name)
			integer = strings.EqualFold(typ, "INTEGER")
			nullable = nullable || notNull == 0
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, &Error{codeInternal, err.Error()}
	}
	alias := len(keys) == 1 && integer
	keyed = alias || len(keys) == 0 || nullable
	for _, name := range []string{"rowid", "_rowid_", "oid"} {
		if !taken[name] {
			return name, keyed, nil
		}
	}
	if alias {
		return quote(keys[0]), keyed, nil
	}
	return "", keyed, nil
}

// unnamedRowid is the refusal of a statement on the table, a table with a rowid that no statement can name: its columns
// take rowid, _rowid_ and oid, and its key is no INTEGER PRIMARY KEY.
func unnamedRowid(table string) error {
	return &Error{codeConstraint, fmt.Sprintf("the table %s has a rowid that no statement names, as its columns take rowid, _rowid_ and oid and its key is no INTEGER PRIMARY KEY: name a column otherwise", table)}
}

// largest is what the hook follows of the largest rowid while a statement runs.
type largest struct {
	// rowids are the tables keyed by their rowid; top holds those that hold the largest int64 now, and seen those that
	// held it at some point of the statement; guarded holds those that have the guard.
	rowids  rowids
	top     map[string]bool
	seen    map[string]bool
	guarded map[string]bool
	// notes are the notes of the guard not yet paired, by table and the depth of their row.
	notes map[noteAt]*note
	// full is a table a row of which took a rowid SQLite chose past the largest int64, and again the tables without the
	// guard that took a row after they came to hold it.
	full  string
	again map[string]bool
}

// noteAt is the place of a note: its table and the depth of its row.
type noteAt struct {
	table string
	depth int
}

// note is a note of the guard: whether its row's rowid was -1 in the trigger, and, once SQLite chose the rowid of the
// row, whether its table held the largest int64 then.
type note struct {
	chosen, frozen, held bool
}

// noted takes a note of the guard from the insert into temp.flowmark_largest that its trigger made, one level deeper
// than the row it notes.
func (l *largest) noted(data sqlite.SQLitePreUpdateData) error {
	values := make([]any, 2)
	if err := data.New(values...); err != nil {
		return err
	}
	table, ok := values[0].(string)
	chosen, also := values[1].(int64)
	if !ok || !also {
		return fmt.Errorf("the guard of the largest rowid noted %v", values)
	}
	l.notes[noteAt{table, data.Depth() - 1}] = &note{chosen: chosen == 1}
	return nil
}

// changed follows a change of a row of the main database that the hook reports.
func (l *largest) changed(data sqlite.SQLitePreUpdateData) error {
	table := data.TableName
	if _, keyed := l.rowids[table]; !keyed {
		return nil
	}
	at := noteAt{table, data.Depth()}
	n := l.notes[at]
	if n != nil && !n.frozen {
		// The first change of the table at the depth of the row after its note: SQLite chose its rowid before it.
		n.held, n.frozen = l.top[table], true
	}
	if data.Op == sqlite3.SQLITE_INSERT {
		switch {
		case n != nil:
			if n.chosen && n.held && data.NewRowID != -1 {
				l.full = table
			}
			delete(l.notes, at)
		case l.guarded[table]:
			return fmt.Errorf("the guard of the largest rowid of %s noted no row of the insert of the rowid %d", table, data.NewRowID)
		case l.seen[table]:
			l.again[table] = true
		}
	}
	switch data.Op {
	case sqlite3.SQLITE_INSERT:
		if data.NewRowID == largestRowid {
			l.top[table], l.seen[table] = true, true
		}
	case sqlite3.SQLITE_DELETE:
		if data.OldRowID == largestRowid {
			l.top[table] = false
		}
	default:
		if data.OldRowID == largestRowid {
			l.top[table] = false
		}
		if data.NewRowID == largestRowid {
			l.top[table], l.seen[table] = true, true
		}
	}
	return nil
}

// queryLargest runs the statement text on the writer connection db of the capture cp as query does, after a savepoint,
// with the guard on each table of c that holds the largest int64, and on each table that came to hold it in a run
// before, which the savepoint takes back: the statement fails as DRIVER (SQLITE_FULL) when SQLite chose a rowid past
// the largest int64.
func queryLargest(ctx context.Context, db runner, cp *capture, c *irexec.Corpus, text string, args []any) (*Answer, error) {
	ids := cp.rowidTables()
	guarded := map[string]bool{}
	for table := range ids {
		if c.LargestKey(table) == largestRowid {
			guarded[table] = true
		}
	}
	for {
		answer, l, err := runLargest(ctx, db, cp, c, ids, guarded, text, args)
		if err != nil {
			return nil, err
		}
		if len(l.again) == 0 {
			return answer, nil
		}
		for table := range l.again {
			guarded[table] = true
		}
		cp.start()
	}
}

// runLargest is one run of queryLargest with the guard on the tables guarded. Its result keeps the statement's changes
// unless it needs the guard on another table, whose run the savepoint takes back.
func runLargest(ctx context.Context, db runner, cp *capture, c *irexec.Corpus, ids rowids, guarded map[string]bool, text string, args []any) (_ *Answer, _ *largest, err error) {
	if _, err := db.ExecContext(ctx, "SAVEPOINT "+largestSavepoint); err != nil {
		return nil, nil, &Error{codeInternal, err.Error()}
	}
	undo := func(cause error) error {
		if _, err := db.ExecContext(ctx, "ROLLBACK TO "+largestSavepoint); err != nil {
			// The statement ended the transaction, and the savepoint with it: what it changed is gone.
			if strings.Contains(err.Error(), "no such savepoint") {
				return cause
			}
			return errors.Join(cause, &Error{codeInternal, err.Error()})
		}
		if _, err := db.ExecContext(ctx, "RELEASE "+largestSavepoint); err != nil {
			return errors.Join(cause, &Error{codeInternal, err.Error()})
		}
		return cause
	}
	guards := slices.Sorted(func(yield func(string) bool) {
		for table := range guarded {
			if !yield(table) {
				return
			}
		}
	})
	if err := makeGuards(ctx, db, ids, guards); err != nil {
		return nil, nil, undo(err)
	}
	l := &largest{rowids: ids, top: map[string]bool{}, seen: map[string]bool{}, guarded: guarded, notes: map[noteAt]*note{}, again: map[string]bool{}}
	for table := range ids {
		if c.LargestKey(table) == largestRowid {
			l.top[table], l.seen[table] = true, true
		}
	}
	cp.follow(l)
	answer, qerr := query(ctx, db, text, args)
	cp.follow(nil)
	if qerr != nil {
		return nil, nil, undo(qerr)
	}
	if err := cp.failed(); err != nil {
		return nil, nil, undo(err)
	}
	switch {
	case l.full != "":
		return nil, nil, undo(&Error{codeDriver, fmt.Sprintf("database or disk is full: the table %s gives no rowid past %d (13)", l.full, int64(largestRowid)) + resultCodeSuffix(sqlite3.SQLITE_FULL)})
	case len(l.again) > 0:
		return nil, l, undo(nil)
	}
	if err := dropGuards(ctx, db, guards); err != nil {
		return nil, nil, undo(err)
	}
	if _, err := db.ExecContext(ctx, "RELEASE "+largestSavepoint); err != nil {
		return nil, nil, &Error{codeInternal, err.Error()}
	}
	return answer, l, nil
}

// makeGuards makes the guard of each table of tables, and temp.flowmark_largest of their notes.
func makeGuards(ctx context.Context, db runner, ids rowids, tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	texts := []string{"CREATE TEMP TABLE flowmark_largest (t TEXT, chosen INTEGER)"}
	for i, table := range tables {
		if ids[table] == "" {
			return unnamedRowid(table)
		}
		texts = append(texts, "CREATE TEMP TRIGGER "+quote(guardPrefix+strconv.Itoa(i))+" BEFORE INSERT ON main."+quote(table)+
			" BEGIN INSERT INTO temp.flowmark_largest VALUES ('"+strings.ReplaceAll(table, "'", "''")+"', NEW."+ids[table]+" = -1); END")
	}
	for _, text := range texts {
		if _, err := db.ExecContext(ctx, text); err != nil {
			return &Error{codeInternal, "the guard of the largest rowid: " + err.Error()}
		}
	}
	return nil
}

// dropGuards takes away the guard that makeGuards made of tables.
func dropGuards(ctx context.Context, db runner, tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	var texts []string
	for i := range tables {
		texts = append(texts, "DROP TRIGGER temp."+quote(guardPrefix+strconv.Itoa(i)))
	}
	for _, text := range append(texts, "DROP TABLE temp.flowmark_largest") {
		if _, err := db.ExecContext(ctx, text); err != nil {
			return &Error{codeInternal, "the guard of the largest rowid: " + err.Error()}
		}
	}
	return nil
}
