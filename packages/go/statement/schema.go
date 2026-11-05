package statement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store/journal"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

// schemaChange reports whether the statement changes the schema.
func schemaChange(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}
	switch strings.ToUpper(fields[0]) {
	case "CREATE", "ALTER", "DROP":
		return true
	}
	return false
}

var conditionalSchema = regexp.MustCompile(`(?i)^\s*(CREATE|DROP)\s+(?:(?:TEMP|TEMPORARY|UNIQUE|VIRTUAL)\s+)?(?:TABLE|INDEX|VIEW|TRIGGER)\s+IF\s+(NOT\s+)?EXISTS\s+(?:"([^"]+)"|([^\s(;]+))`)

// alreadySo reports whether a CREATE IF NOT EXISTS or a DROP IF EXISTS finds what it states, and so changes nothing. Such
// a statement asks SQLite for no write lock, so it does not wait for a writer.
func alreadySo(ctx context.Context, s *writepath.Session, text string) (bool, error) {
	m := conditionalSchema.FindStringSubmatch(text)
	if m == nil {
		return false, nil
	}
	c, err := s.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	var found *Answer
	err = s.ReadSnapshot(func(slot *io.Closer, unlock func()) (bool, error) {
		found, _, err = readInto(ctx, s, c, slot, unlock, "SELECT 1 FROM sqlite_master WHERE name = ?", []any{m[3] + m[4]})
		return false, err
	})
	if err != nil {
		return false, err
	}
	return (len(found.Rows) > 0) == (m[2] != ""), nil
}

// runSchema runs a schema statement. It is its own transaction: an open transaction commits first and goes on after it,
// as a MySQL connection commits before a schema statement, since the catalog of the transaction's corpus is the one it
// began with. The statement runs in the corpus's SQLite database, and it stands when the commit writes it after the
// corpus's schema statements, with the rows of the tables it changed written again under the new columns.
func runSchema(ctx context.Context, s *writepath.Session, text string, args []any) (_ *Answer, err error) {
	if s.InTransaction() {
		if err := commit(s); err != nil {
			return nil, err
		}
		defer func() {
			err = errors.Join(err, begin(ctx, s))
		}()
	}
	if s.QueryOnly() {
		return nil, &Error{codeReadOnly, "a schema statement on a read-only connection"}
	}
	var answer *Answer
	err = s.Reschema(ctx, func(c *irexec.Corpus) (*catalog.Catalog, []journal.Change, error) {
		h, err := heldFor(ctx, s, c)
		if err != nil {
			return nil, nil, err
		}
		cat, changes, err := apply(ctx, s, h.db, c, text, args, &answer)
		if err != nil {
			return nil, nil, err
		}
		return cat, changes, nil
	})
	if err != nil {
		return nil, errors.Join(err, drop(s))
	}
	// The database already holds what the corpus now holds, its AUTOINCREMENT counters included.
	err = s.View(ctx, func(c *irexec.Corpus) error {
		if h, ok := s.Held().(*held); ok {
			h.corpus = c
		}
		return nil
	})
	if err != nil {
		return nil, errors.Join(err, drop(s))
	}
	return answer, nil
}

// apply runs the schema statement on db and returns the catalog and the changes that make it stand.
func apply(ctx context.Context, s *writepath.Session, db *sql.DB, c *irexec.Corpus, text string, args []any, answer **Answer) (*catalog.Catalog, []journal.Change, error) {
	before, err := columnsOf(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	schemaBefore, err := schemaText(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	a, err := query(ctx, db, text, args)
	if err != nil {
		return nil, nil, err
	}
	*answer = a
	schemaAfter, err := schemaText(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	// CREATE ... IF NOT EXISTS of what exists changes nothing, so it does not stand in the corpus.
	if schemaBefore == schemaAfter {
		return c.Catalog(), nil, nil
	}
	after, err := columnsOf(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	added := append(slices.Clone(c.Catalog().Statements), text)
	cat, diags := catalog.LoadStatements(s.Root(), added)
	if len(diags) > 0 {
		return nil, nil, &Error{codeConstraint, fmt.Sprintf("the schema statement leaves a corpus that does not load: %v", diags)}
	}
	file, err := json.MarshalIndent(added, "", "  ")
	if err != nil {
		return nil, nil, &Error{codeInternal, err.Error()}
	}
	changes := []journal.Change{{Path: catalog.StatementsFile, Data: append(file, '\n')}}
	var rewrite []*catalog.Table
	var gone []string
	for name, cols := range before {
		now, held := after[name]
		if !held {
			gone = append(gone, name)
			continue
		}
		if !slices.Equal(cols, now) {
			if tb, ok := cat.Table(name); ok {
				rewrite = append(rewrite, tb)
				gone = append(gone, name)
			}
		}
	}
	// A table a rename or a CREATE TABLE AS made comes with rows.
	for name := range after {
		if _, held := before[name]; !held {
			if tb, ok := cat.Table(name); ok {
				rewrite = append(rewrite, tb)
			}
		}
	}
	if len(gone) == 0 && len(rewrite) == 0 {
		return cat, changes, nil
	}
	old := c.Catalog()
	var drop []string
	for _, name := range gone {
		tb, ok := old.Table(name)
		if !ok {
			continue
		}
		paths, err := c.FilesOf(tb.Name)
		if err != nil {
			return nil, nil, err
		}
		drop = append(drop, paths...)
	}
	next := c.Rebased(cat, drop)
	exec, err := irexec.New(cat)
	if err != nil {
		return nil, nil, err
	}
	for _, tb := range fillOrder(cat) {
		if !slices.Contains(rewrite, tb) {
			continue
		}
		rows, err := rowsOf(ctx, db, tb, c.Cost())
		if err != nil {
			return nil, nil, err
		}
		if len(rows) == 0 {
			continue
		}
		msg, err := insertMessage(cat, tb, rows, true)
		if err != nil {
			return nil, nil, err
		}
		res, err := exec.ExecuteFace(next, msg)
		if err != nil {
			return nil, nil, err
		}
		next = res.Post.Next
	}
	if next, err = sequencedUnder(ctx, db, c, next, cat); err != nil {
		return nil, nil, err
	}
	for _, p := range next.Differs(c) {
		if data := next.Data(p); data != nil {
			changes = append(changes, journal.Change{Path: p, Data: data})
		} else {
			changes = append(changes, journal.Change{Path: p, Delete: true})
		}
	}
	return cat, changes, nil
}

// sequencedUnder gives next, the corpus of the schema statement under the catalog cat, the AUTOINCREMENT counters the
// statement left in db: a counter of c whose table is still an AUTOINCREMENT table stays, and a sequence SQLite keeps
// past the table's sequence in next, as the counter of a table the statement renamed, is its counter. A dropped table
// leaves no counter, as SQLite drops its row of sqlite_sequence.
func sequencedUnder(ctx context.Context, db *sql.DB, c, next *irexec.Corpus, cat *catalog.Catalog) (*irexec.Corpus, error) {
	counters, err := catalog.ParseSequences(c.Data(catalog.SequencesFile))
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	maps.DeleteFunc(counters, func(table string, _ int64) bool {
		tb, ok := cat.Table(table)
		return !ok || !tb.Autoincrement
	})
	if err := next.SetSequences(catalog.RenderSequences(counters)); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	given, err := sequencesOf(ctx, db, cat)
	if err != nil {
		return nil, err
	}
	risen := false
	for table, n := range given {
		if n > next.Sequence(table) {
			if counters == nil {
				counters = map[string]int64{}
			}
			counters[table], risen = n, true
		}
	}
	if risen {
		if err := next.SetSequences(catalog.RenderSequences(counters)); err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
	}
	return next, nil
}

// schemaText is the text of the schema of db.
func schemaText(ctx context.Context, db *sql.DB) (string, error) {
	var text string
	row := db.QueryRowContext(ctx, "SELECT COALESCE(group_concat(type || name || COALESCE(sql, ''), char(10)), '') FROM sqlite_master")
	if err := row.Scan(&text); err != nil {
		return "", &Error{codeInternal, err.Error()}
	}
	return text, nil
}

// columnsOf is the columns of each table of db, in order.
func columnsOf(ctx context.Context, db *sql.DB) (_ map[string][]string, err error) {
	rows, err := db.QueryContext(ctx, `SELECT m.name, p.name FROM sqlite_master m, pragma_table_info(m.name) p WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%' ORDER BY m.name, p.cid`)
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	defer closed(&err, rows)
	out := map[string][]string{}
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		out[table] = append(out[table], column)
	}
	return out, rows.Err()
}
