package statement

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// The largest key (spec 40 SQL statements): a table whose key is its rowid gives an insert one more than its largest
// rowid, and when that is the largest int64, SQLite picks a random unused rowid, which no other database gives — MySQL and
// PostgreSQL refuse an identity past its maximum, as SQLite refuses an AUTOINCREMENT table. The engine refuses it: a
// guard of two temporary triggers on each table whose key is its rowid fails an insert whose rowid SQLite chose while the
// largest rowid was the largest int64, as DRIVER (SQLITE_FULL). SQLite gives a BEFORE INSERT trigger the rowid -1 for a
// rowid it chooses, and for a rowid -1 or NULL the insert gives, so the BEFORE trigger notes such a row while the largest
// rowid is the largest int64 in temp.flowmark_largest, and the AFTER trigger of the same row fails it unless its rowid is
// -1, the rowid it gave; a row the insert does not write, as a conflict ignores it, leaves its note to the next BEFORE
// trigger of the table, which clears it. The hook records nothing of the temporary database, and a statement that fails
// leaves the database as it was.

// largestFull is the message the guard fails an insert with, which driverError gives as SQLite's SQLITE_FULL.
const largestFull = "flowmark: database or disk is full: the table gives no rowid past 9223372036854775807"

// guardPrefix is the prefix of the names of the triggers of the guard.
const guardPrefix = "flowmark_largest_"

// guardLargest gives each table of the main database of db whose key is its rowid the triggers of the guard, and takes
// them from the tables that are gone: when the guards it holds are not those of the tables it has, it drops them and
// makes them again. It runs on db before a statement, with the read of the primary keys (capture.prepare), and returns
// the number of the triggers of the guard.
func guardLargest(ctx context.Context, db runner) (int, error) {
	if _, err := db.ExecContext(ctx, "CREATE TEMP TABLE IF NOT EXISTS flowmark_largest (t TEXT)"); err != nil {
		return 0, &Error{codeInternal, err.Error()}
	}
	old, err := column[string](ctx, db, "SELECT name FROM temp.sqlite_master WHERE type = 'trigger' AND name GLOB '"+guardPrefix+"*'", nil)
	if err != nil {
		return 0, err
	}
	for _, name := range old {
		if _, err := db.ExecContext(ctx, "DROP TRIGGER temp."+quote(name)); err != nil {
			return 0, &Error{codeInternal, err.Error()}
		}
	}
	tables, err := column[string](ctx, db, "SELECT name FROM pragma_table_list WHERE schema = 'main' AND type = 'table' AND wr = 0 AND name NOT GLOB 'sqlite_*' ORDER BY name", nil)
	if err != nil {
		return 0, err
	}
	n := 0
	for i, table := range tables {
		rowid, err := rowidName(ctx, db, table)
		if err != nil {
			return 0, err
		}
		if rowid == "" {
			continue
		}
		t, id := quote(table), strconv.Itoa(i)
		tag := "'" + strings.ReplaceAll(table, "'", "''") + "'"
		largest := "(SELECT max(" + rowid + ") FROM main." + t + ") = 9223372036854775807"
		noted := "EXISTS (SELECT 1 FROM temp.flowmark_largest WHERE t = " + tag + ")"
		chosen := "NEW." + rowid + " = -1 AND " + largest
		for _, text := range []string{
			"CREATE TEMP TRIGGER " + quote(guardPrefix+"before_"+id) + " BEFORE INSERT ON main." + t +
				" WHEN (" + chosen + ") OR " + noted + " BEGIN" +
				" DELETE FROM temp.flowmark_largest WHERE t = " + tag + ";" +
				" INSERT INTO temp.flowmark_largest SELECT " + tag + " WHERE " + chosen + "; END",
			"CREATE TEMP TRIGGER " + quote(guardPrefix+"after_"+id) + " AFTER INSERT ON main." + t +
				" WHEN " + noted + " BEGIN" +
				" DELETE FROM temp.flowmark_largest WHERE t = " + tag + ";" +
				" SELECT RAISE(ABORT, '" + largestFull + "') WHERE NEW." + rowid + " <> -1; END",
		} {
			if _, err := db.ExecContext(ctx, text); err != nil {
				return 0, &Error{codeInternal, fmt.Sprintf("the guard of the largest rowid of %s: %v", table, err)}
			}
		}
		n += 2
	}
	return n, nil
}

// guards is the number of the triggers of the guard db holds.
func guards(ctx context.Context, db runner) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM temp.sqlite_master WHERE type = 'trigger' AND name GLOB '"+guardPrefix+"*'").Scan(&n); err != nil {
		return 0, &Error{codeInternal, err.Error()}
	}
	return n, nil
}

// rowidName is the name of the rowid of the table whose key it is, as a trigger names it: the column of an INTEGER
// PRIMARY KEY, which is the rowid, or for a table without a primary key the first of rowid, _rowid_ and oid that names
// no column; "" for a table keyed otherwise, whose rowid the corpus does not hold. A table without a primary key whose
// columns take all three names has no name of its rowid, as in SQLite, and is INTERNAL.
func rowidName(ctx context.Context, db runner, table string) (_ string, err error) {
	rows, err := db.QueryContext(ctx, "SELECT name, type, pk FROM pragma_table_info(?)", table)
	if err != nil {
		return "", &Error{codeInternal, err.Error()}
	}
	defer closed(&err, rows)
	var keys []string
	integer := false
	names := map[string]bool{}
	for rows.Next() {
		var name, typ string
		var pk int
		if err := rows.Scan(&name, &typ, &pk); err != nil {
			return "", &Error{codeInternal, err.Error()}
		}
		names[strings.ToLower(name)] = true
		if pk > 0 {
			keys = append(keys, name)
			integer = strings.EqualFold(typ, "INTEGER")
		}
	}
	if err := rows.Err(); err != nil {
		return "", &Error{codeInternal, err.Error()}
	}
	switch {
	case len(keys) == 1 && integer:
		return quote(keys[0]), nil
	case len(keys) > 0:
		return "", nil
	}
	for _, name := range []string{"rowid", "_rowid_", "oid"} {
		if !names[name] {
			return name, nil
		}
	}
	return "", &Error{codeInternal, "the table " + table + " without a primary key names its columns rowid, _rowid_ and oid, so no trigger names its rowid"}
}
