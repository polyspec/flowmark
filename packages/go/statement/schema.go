package statement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store/journal"
	"github.com/polyspec/flowmark/packages/go/writepath"
)

// schemaChange reports whether the statement changes the schema.
func schemaChange(text string) bool {
	switch firstKeyword(statementTokens(text)) {
	case "CREATE", "ALTER", "DROP":
		return true
	}
	return false
}

// conditional reads a CREATE IF NOT EXISTS or a DROP IF EXISTS of a table, an index, a view or a trigger of the main
// schema from its tokens: the name it states and whether it states NOT. ok is false for any other statement.
func conditional(toks []token) (name string, not, ok bool) {
	at := func(i int) string {
		if i < len(toks) {
			return toks[i].keyword()
		}
		return ""
	}
	verb := at(0)
	if verb != "CREATE" && verb != "DROP" {
		return "", false, false
	}
	i := 1
	switch at(i) {
	case "TEMP", "TEMPORARY", "UNIQUE", "VIRTUAL":
		i++
	}
	switch at(i) {
	case "TABLE", "INDEX", "VIEW", "TRIGGER":
	default:
		return "", false, false
	}
	if at(i+1) != "IF" {
		return "", false, false
	}
	i += 2
	if at(i) == "NOT" {
		not = true
		i++
	}
	if at(i) != "EXISTS" || i+1 >= len(toks) {
		return "", false, false
	}
	named := toks[i+1]
	if i+3 < len(toks) && toks[i+2].is(".") {
		if !strings.EqualFold(named.text, "main") {
			return "", false, false
		}
		named = toks[i+3]
	}
	if named.kind != tokenWord && named.kind != tokenQuoted {
		return "", false, false
	}
	return named.text, not, true
}

// alreadySo reports whether a CREATE IF NOT EXISTS or a DROP IF EXISTS finds what it states, and so changes nothing. Such
// a statement asks SQLite for no write lock, so it does not wait for a writer.
func alreadySo(ctx context.Context, s *writepath.Session, text string) (bool, error) {
	name, not, ok := conditional(statementTokens(text))
	if !ok {
		return false, nil
	}
	c, err := s.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	r, err := readerFor(ctx, s, c)
	if err != nil {
		return false, err
	}
	found, open, err := readBatch(ctx, r.q, "SELECT 1 FROM sqlite_master WHERE name = ?", []any{name})
	if open != nil {
		err = errors.Join(err, open.Close())
	}
	if err = errors.Join(err, r.release()); err != nil {
		return false, err
	}
	return (len(found.Rows) > 0) == not, nil
}

// runSchema runs a schema statement. Outside a transaction it is its own transaction; inside one it is a write of the
// transaction, as on SQLite (schemaInTransaction). The statement runs in the corpus's SQLite database, and it stands
// when the commit writes it after the corpus's schema statements, with the rows of the tables it changed written again
// under the new columns.
func runSchema(ctx context.Context, s *writepath.Session, text string, args []any) (_ *Answer, err error) {
	if s.InTransaction() {
		return schemaInTransaction(ctx, s, text, args)
	}
	if s.QueryOnly() {
		return nil, &Error{codeReadOnly, "a schema statement on a read-only connection"}
	}
	var answer *Answer
	err = s.Reschema(ctx, func(c *irexec.Corpus) (*catalog.Catalog, []journal.Change, error) {
		d, release, err := writerFor(ctx, s, c)
		if err != nil {
			return nil, nil, err
		}
		// The writer's transaction ends with the schema statement's commit: it commits after the corpus commit is written.
		if err := follow(ctx, s, d, release); err != nil {
			return nil, nil, err
		}
		cat, next, changes, err := apply(ctx, s, d.w, c, text, args, &answer)
		if err != nil {
			return nil, nil, err
		}
		// The database holds what the corpus then holds, its AUTOINCREMENT counters included.
		d.corpus = next
		return cat, changes, nil
	})
	if err != nil {
		return nil, err
	}
	return answer, nil
}

// schemaInTransaction runs a schema statement in the open transaction of the session, under a savepoint that the writer
// connection follows, as runInTransaction runs a statement: the writer's SQLite transaction holds the statement, and the
// session's transaction holds the catalog and the files it gives (writepath Restructure), so a rollback to a savepoint
// undoes it and the commit writes it.
func schemaInTransaction(ctx context.Context, s *writepath.Session, text string, args []any) (_ *Answer, err error) {
	if s.QueryOnly() {
		return nil, &Error{codeReadOnly, "a schema statement on a read-only connection"}
	}
	d, err := transactionWriter(ctx, s)
	if err != nil {
		return nil, err
	}
	var answer *Answer
	if err := underSavepoint(s, func() error {
		return s.Restructure(ctx, func(c *irexec.Corpus) (*irexec.Corpus, []journal.Change, error) {
			_, next, changes, err := apply(ctx, s, d.w, c, text, args, &answer)
			if err != nil {
				return nil, nil, err
			}
			// The database holds what the transaction's corpus then holds, its AUTOINCREMENT counters included.
			d.corpus = next
			return next, changes, nil
		})
	}); err != nil {
		return nil, err
	}
	return answer, nil
}

// apply runs the schema statement on db and returns the catalog, the corpus under it that the database then holds, and
// the changes that make it stand.
func apply(ctx context.Context, s *writepath.Session, db runner, c *irexec.Corpus, text string, args []any, answer **Answer) (*catalog.Catalog, *irexec.Corpus, []journal.Change, error) {
	before, err := columnsOf(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	schemaBefore, err := schemaText(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	a, err := query(ctx, db, text, args)
	if err != nil {
		return nil, nil, nil, err
	}
	*answer = a
	schemaAfter, err := schemaText(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	// CREATE ... IF NOT EXISTS of what exists changes nothing, so it does not stand in the corpus.
	if schemaBefore == schemaAfter {
		return c.Catalog(), c, nil, nil
	}
	after, err := columnsOf(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	added := append(slices.Clone(c.Catalog().Statements), text)
	cat, diags := catalog.LoadStatements(s.Root(), added)
	if len(diags) > 0 {
		return nil, nil, nil, &Error{codeConstraint, fmt.Sprintf("the schema statement leaves a corpus that does not load: %v", diags)}
	}
	file, err := json.MarshalIndent(added, "", "  ")
	if err != nil {
		return nil, nil, nil, &Error{codeInternal, err.Error()}
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
	old := c.Catalog()
	var drop []string
	for _, name := range gone {
		tb, ok := old.Table(name)
		if !ok {
			continue
		}
		paths, err := c.FilesOf(tb.Name)
		if err != nil {
			return nil, nil, nil, err
		}
		drop = append(drop, paths...)
	}
	next := c.Rebased(cat, drop)
	exec, err := irexec.New(cat)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, tb := range fillOrder(cat) {
		if !slices.Contains(rewrite, tb) {
			continue
		}
		rows, err := rowsOf(ctx, db, tb, c.Cost())
		if err != nil {
			return nil, nil, nil, err
		}
		if len(rows) == 0 {
			continue
		}
		msg, err := insertMessage(cat, tb, rows, true)
		if err != nil {
			return nil, nil, nil, err
		}
		res, err := exec.ExecuteFace(next, msg)
		if err != nil {
			return nil, nil, nil, err
		}
		next = res.Post.Next
	}
	if next, err = sequencedUnder(ctx, db, c, next, cat); err != nil {
		return nil, nil, nil, err
	}
	for _, p := range next.Differs(c) {
		if p == catalog.StatementsFile {
			continue // written above, as the statements stand
		}
		if data := next.Data(p); data != nil {
			changes = append(changes, journal.Change{Path: p, Data: data})
		} else {
			changes = append(changes, journal.Change{Path: p, Delete: true})
		}
	}
	return cat, next, changes, nil
}

// sequencedUnder gives next, the corpus of the schema statement under the catalog cat, the rows of sqlite_sequence the
// statement left in db, as SQLite keeps them: a renamed table keeps its sequence under its new name, and a dropped
// table leaves none, as SQLite drops its row. The sequences' file of c keeps no name of a table that is no AUTOINCREMENT
// table of cat.
func sequencedUnder(ctx context.Context, db runner, c, next *irexec.Corpus, cat *catalog.Catalog) (*irexec.Corpus, error) {
	counters, err := catalog.ParseSequences(c.Data(catalog.SequencesFile))
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	maps.DeleteFunc(counters, func(table string, _ *int64) bool {
		tb, ok := cat.Table(table)
		return !ok || !tb.Autoincrement
	})
	if err := next.SetSequenceFile(catalog.RenderSequences(counters)); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	held, err := sequencesOf(ctx, db, cat)
	if err != nil {
		return nil, err
	}
	res, err := next.SetSequences(held)
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	if res.Post != nil {
		next = res.Post.Next
	}
	return next, nil
}

// schemaText is the text of the schema of db.
func schemaText(ctx context.Context, db runner) (string, error) {
	var text string
	row := db.QueryRowContext(ctx, "SELECT COALESCE(group_concat(type || name || COALESCE(sql, ''), char(10)), '') FROM sqlite_master")
	if err := row.Scan(&text); err != nil {
		return "", &Error{codeInternal, err.Error()}
	}
	return text, nil
}

// columnsOf is the columns of each table of db, in order.
func columnsOf(ctx context.Context, db runner) (_ map[string][]string, err error) {
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
