package statement

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/min-0-max/sqlite"
	sqlite3 "github.com/min-0-max/sqlite/lib"
	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store/journal"
	"github.com/polyspec/flowmark/packages/go/writepath"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
)

// runner is what a statement runs on: a connection of a database, a database of one connection, or a transaction.
type runner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

// queryer is a runner that begins transactions: a connection, or a database of one connection.
type queryer interface {
	runner
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Readers is the size of the pool of reader connections of a database; a read that finds every one in use waits for the
// release of one (spec 40 SQL statements).
const Readers = 8

// walCheckpoint is the wal_autocheckpoint of a database: SQLite checkpoints the write-ahead log into the database when a
// commit leaves it longer than this many pages, SQLite's default.
const walCheckpoint = 1000

// database is the SQLite database of an opening of a corpus (spec 40 SQL statements): a file of the opening in WAL mode,
// written by one writer connection held for its life and read by a pool of reader connections, which read what its last
// commit holds while the writer writes; in memory, for a corpus the process may only read, the writer connection is the
// only one and reads take it one at a time. The database is a cache of the corpus, which stays the only store: it is
// built again from the corpus whenever it cannot follow it, so it is opened with synchronous off.
type database struct {
	file    *journal.Database
	pool    *sql.DB
	w       *sql.Conn
	readers *sql.DB
	// cap records the rows each statement changes on w, from its pre-update hook.
	cap *capture

	// wmu is the use of the writer connection: a statement's, a refresh's, or a transaction's from its first statement
	// to its end.
	wmu sync.Mutex
	// Under wmu: corpus is the corpus the writer connection holds, an open transaction's included, nil before the fill;
	// replaced is the value each PRAGMA setting of a session replaced on it.
	corpus   *irexec.Corpus
	replaced map[string]string

	mu sync.Mutex
	// Under mu: published is the corpus of the database's last commit, what a reader connection reads; refs counts the
	// uses of the database, and retired says that it is no longer the database of the corpus, so its last use closes it.
	published *irexec.Corpus
	refs      int
	retired   bool
}

// textAsStored is the driver setting that reads the text of a column declared DATE, DATETIME or TIMESTAMP as SQLite
// holds it, as a SQLite connection answers it, instead of a time the driver parsed from it.
const textAsStored = "_decltime=0"

// memoryURI is the SQLite URI of a new in-memory database.
const memoryURI = ":memory:?" + textAsStored

// databaseURI is the SQLite URI of the file p with the PRAGMA settings pragmas, each run on every connection it opens.
func databaseURI(p string, pragmas ...string) string {
	slash := filepath.ToSlash(p)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash // a Windows volume: file:///C:/...
	}
	u := url.URL{Scheme: "file", Path: slash}
	q := []string{textAsStored}
	for _, p := range pragmas {
		q = append(q, "_pragma="+url.QueryEscape(p))
	}
	return u.String() + "?" + strings.Join(q, "&")
}

// openDatabase opens a new database of the corpus of s, empty: its first use fills it.
func openDatabase(ctx context.Context, s *writepath.Session) (_ *database, err error) {
	file, err := s.NewSQLFile()
	if err != nil {
		return nil, err
	}
	d := &database{file: file, cap: &capture{}}
	defer func() {
		if err != nil {
			err = errors.Join(err, d.close())
		}
	}()
	writer := memoryURI
	if file != nil {
		common := []string{"journal_mode(WAL)", "synchronous(OFF)", "wal_autocheckpoint(" + strconv.Itoa(walCheckpoint) + ")"}
		writer = databaseURI(file.Path, common...)
		if d.readers, err = sql.Open(sqlite.DriverName, databaseURI(file.Path, append(common, "query_only(1)")...)); err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		d.readers.SetMaxOpenConns(Readers)
		d.readers.SetMaxIdleConns(Readers)
	}
	if d.pool, err = sql.Open(sqlite.DriverName, writer); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	d.pool.SetMaxOpenConns(1)
	if d.w, err = d.pool.Conn(ctx); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	if err := d.w.Raw(d.cap.register); err != nil {
		return nil, err
	}
	return d, nil
}

// close closes the connections of the database and removes its file.
func (d *database) close() error {
	var err error
	if d.w != nil {
		err = errors.Join(err, d.w.Close())
	}
	if d.pool != nil {
		err = errors.Join(err, d.pool.Close())
	}
	if d.readers != nil {
		err = errors.Join(err, d.readers.Close())
	}
	if d.file != nil {
		err = errors.Join(err, d.file.Remove())
	}
	if err != nil {
		return &Error{codeInternal, "closing the SQL database of the corpus: " + err.Error()}
	}
	return nil
}

// Close is the end of the corpus: the database is retired, and closed when its last use ends.
func (d *database) Close() error { return d.retire() }

// acquire counts a use of the database, false when it is retired.
func (d *database) acquire() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.retired {
		return false
	}
	d.refs++
	return true
}

// release ends a use of the database; the last use of a retired database closes it.
func (d *database) release() error {
	d.mu.Lock()
	d.refs--
	last := d.retired && d.refs == 0
	d.mu.Unlock()
	if last {
		return d.close()
	}
	return nil
}

// retire makes the database no longer the database of the corpus; it is closed when its last use ends.
func (d *database) retire() error {
	d.mu.Lock()
	if d.retired {
		d.mu.Unlock()
		return nil
	}
	d.retired = true
	last := d.refs == 0
	d.mu.Unlock()
	if last {
		return d.close()
	}
	return nil
}

// currentDatabase returns the database of the corpus of s, with a use counted, opening it at the first statement and
// again after it was retired.
func currentDatabase(ctx context.Context, s *writepath.Session) (*database, error) {
	for {
		old := s.SQL()
		if d, ok := old.(*database); ok && d.acquire() {
			return d, nil
		}
		d, err := openDatabase(ctx, s)
		if err != nil {
			return nil, err
		}
		d.refs = 1
		if s.SwapSQL(old, d) {
			return d, nil
		}
		// Another session opened it first.
		if err := d.close(); err != nil {
			return nil, err
		}
	}
}

// drop retires the database d of the corpus of s, which holds what the corpus does not: the next statement builds it again.
func drop(s *writepath.Session, d *database) error {
	s.SwapSQL(d, nil)
	return d.retire()
}

// commitAs runs commit, the commit of the writer connection, and publishes c, the corpus the committed database holds,
// at once: a reader connection that begins to read sees both or neither.
func (d *database) commitAs(c *irexec.Corpus, commit func() error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := commit(); err != nil {
		return err
	}
	d.published = c
	return nil
}

// errRebuild is the refusal of a database to follow a corpus of another catalog, which builds a new database.
var errRebuild = errors.New("the database is of another catalog")

// bring brings the database to the corpus c, under wmu with no transaction of the writer open: it fills an empty
// database, refreshes one of the same catalog that differs (refresh), and refuses one of another catalog with errRebuild.
func (d *database) bring(ctx context.Context, c *irexec.Corpus) error {
	switch {
	case d.corpus == nil:
		if err := fillAs(ctx, d, c); err != nil {
			return err
		}
	case d.corpus.Catalog() != c.Catalog():
		return errRebuild
	case len(d.corpus.Differs(c)) > 0:
		if err := refreshAs(ctx, d, d.corpus, c); err != nil {
			return err
		}
	}
	// No transaction of the writer is open, so its last commit holds c: a refresh that changed no row commits nothing.
	d.mu.Lock()
	d.published = c
	d.mu.Unlock()
	d.corpus = c
	return nil
}

// fillAs fills the empty database d from c and publishes c with its commit.
func fillAs(ctx context.Context, d *database, c *irexec.Corpus) error {
	return fillWith(ctx, d.w, c, func(commit func() error) error { return d.commitAs(c, commit) })
}

// refreshAs refreshes the database d of the corpus before to c and publishes c with its commit.
func refreshAs(ctx context.Context, d *database, before, c *irexec.Corpus) error {
	return refreshWith(ctx, d.w, before, c, func(commit func() error) error { return d.commitAs(c, commit) })
}

// writerFor returns the database of the statements of s, its writer connection taken and brought to c, the corpus s
// reads under the lock, with the settings of s; release gives it back. In a transaction of s the writer follows it from
// here on (txn), so a transaction's statements see its own writes and a reader connection sees none of them: the writer
// begins at the corpus the transaction began from and takes each savepoint and the rows the transaction changed before
// its first statement, and a writer that follows already takes the rows changed outside its statements since, as an
// IR request of the session. A database of another catalog, and one whose refresh fails, is retired and a new one built.
func writerFor(ctx context.Context, s *writepath.Session, c *irexec.Corpus) (d *database, release func() error, err error) {
	if t := following(s); t != nil {
		if err := t.d.settle(ctx, s); err != nil {
			return nil, nil, err
		}
		if err := t.d.catchUp(ctx, c); err != nil {
			return nil, nil, err
		}
		return t.d, func() error { return nil }, nil
	}
	base, marks := s.Begun()
	at := c
	if base != nil {
		at = base
	}
	for {
		if d, err = currentDatabase(ctx, s); err != nil {
			return nil, nil, err
		}
		d.wmu.Lock()
		release = func() error {
			d.wmu.Unlock()
			return d.release()
		}
		if err = d.bring(ctx, at); err == nil {
			err = d.settle(ctx, s)
		}
		if err == nil {
			break
		}
		err = errors.Join(err, drop(s, d), release())
		if !errors.Is(err, errRebuild) {
			return nil, nil, err
		}
	}
	if base == nil {
		return d, release, nil
	}
	if err := follow(ctx, s, d, release); err != nil {
		return nil, nil, err
	}
	for _, m := range marks {
		if err := d.catchUp(ctx, m.Cur); err != nil {
			return nil, nil, err
		}
		if _, err := d.w.ExecContext(ctx, "SAVEPOINT "+quote(m.Name)); err != nil {
			return nil, nil, driverError(err)
		}
	}
	if err := d.catchUp(ctx, c); err != nil {
		return nil, nil, err
	}
	return d, func() error { return nil }, nil
}

// catchUp brings the open transaction of the writer connection to c, under wmu: the rows that differ between the corpus
// it holds and c (refreshIn).
func (d *database) catchUp(ctx context.Context, c *irexec.Corpus) error {
	if d.corpus == c || len(d.corpus.Differs(c)) == 0 {
		d.corpus = c
		return nil
	}
	if d.corpus.Catalog() != c.Catalog() {
		return &Error{codeInternal, "the catalog of the corpus changed inside a transaction"}
	}
	if err := refreshIn(ctx, d.w, d.corpus, c); err != nil {
		return err
	}
	d.corpus = c
	return nil
}

// settle gives the writer connection the settings of the session: it is shared by the sessions of the corpus, and a PRAGMA
// is the setting of one connection, so a setting of another session goes back to the value it replaced.
func (d *database) settle(ctx context.Context, s *writepath.Session) error {
	queryOnly := "OFF"
	if s.QueryOnly() {
		queryOnly = "ON"
	}
	if _, err := d.w.ExecContext(ctx, "PRAGMA query_only = "+queryOnly); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	foreignKeys := "ON"
	if s.ForeignKeysOff() {
		foreignKeys = "OFF"
	}
	if _, err := d.w.ExecContext(ctx, "PRAGMA foreign_keys = "+foreignKeys); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	if d.replaced == nil {
		d.replaced = map[string]string{}
	}
	return settleSettings(ctx, d.w, s.Settings(), d.replaced)
}

// settleSettings gives the connection q the settings want, keeping in replaced the value each one replaced, and gives a
// setting replaced holds that want does not hold its value back.
func settleSettings(ctx context.Context, q runner, want, replaced map[string]string) error {
	for name, value := range replaced {
		if _, kept := want[name]; kept {
			continue
		}
		if _, err := q.ExecContext(ctx, "PRAGMA "+name+" = "+value); err != nil {
			return driverError(err)
		}
		delete(replaced, name)
	}
	for name, value := range want {
		if _, kept := replaced[name]; !kept {
			var old string
			switch err := q.QueryRowContext(ctx, "PRAGMA "+name).Scan(&old); {
			case err == nil:
				replaced[name] = old
			case !errors.Is(err, sql.ErrNoRows):
				return driverError(err)
			}
		}
		if _, err := q.ExecContext(ctx, "PRAGMA "+name+" = "+value); err != nil {
			return driverError(err)
		}
	}
	return nil
}

// txn is the SQLite transaction of the writer connection that follows a transaction of a session (writepath.Follower):
// it holds the writer connection from its BEGIN IMMEDIATE to its end, commits after the corpus commit is written and rolls
// back on every other end; base is the corpus it began from.
type txn struct {
	d    *database
	s    *writepath.Session
	base *irexec.Corpus
	// release gives the writer connection and the use of the database back.
	release func() error
}

// following returns the transaction of the writer that follows the transaction of s, nil when none does.
func following(s *writepath.Session) *txn {
	t, ok := s.Following().(*txn)
	if !ok {
		return nil
	}
	return t
}

// follow begins a transaction of the writer connection of d, which writerFor gave, and makes it follow the transaction of
// s, which is open, or the schema statement s commits.
func follow(ctx context.Context, s *writepath.Session, d *database, release func() error) error {
	if _, err := d.w.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return errors.Join(driverError(err), release())
	}
	s.Follow(&txn{d: d, s: s, base: d.corpus, release: release})
	return nil
}

func (t *txn) Savepoint(name string) error {
	if _, err := t.d.w.ExecContext(t.s.Life(), "SAVEPOINT "+quote(name)); err != nil {
		return driverError(err)
	}
	return nil
}

func (t *txn) Release(name string) error {
	if _, err := t.d.w.ExecContext(t.s.Life(), "RELEASE "+quote(name)); err != nil {
		return driverError(err)
	}
	return nil
}

func (t *txn) RollbackTo(name string, cur *irexec.Corpus) error {
	if _, err := t.d.w.ExecContext(t.s.Life(), "ROLLBACK TO "+quote(name)); err != nil {
		return driverError(err)
	}
	t.d.corpus = cur
	return nil
}

// Ready refuses the commit of a transaction whose deferred foreign keys SQLite has not resolved, as SQLite refuses its
// COMMIT, before the corpus commit: the transaction stays open.
func (t *txn) Ready() error {
	var pending int
	err := t.d.w.Raw(func(dc any) error {
		st, ok := dc.(sqlite.DBStatus)
		if !ok {
			return errors.New("the SQLite driver reads no status of a connection")
		}
		var err error
		pending, _, err = st.Status(sqlite.DBStatusDeferredFKs, false)
		return err
	})
	if err != nil {
		return &Error{codeInternal, err.Error()}
	}
	if pending > 0 {
		return &Error{codeForeignKey, "constraint failed: FOREIGN KEY constraint failed" + resultCodeSuffix(sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY)}
	}
	return nil
}

// End commits the transaction of the writer connection after a written corpus commit, publishing what the database then
// holds, and rolls it back otherwise; a COMMIT or a ROLLBACK that fails leaves a database that may hold what the corpus
// does not, so it is built again. A transaction that a statement ended, as RAISE(ROLLBACK) does, SQLite has already
// rolled back, so no ROLLBACK is sent, and the database holds the corpus the transaction began on.
func (t *txn) End(written bool) error {
	d := t.d
	var err error
	switch {
	case written:
		err = d.commitAs(d.corpus, func() error {
			_, err := d.w.ExecContext(context.Background(), "COMMIT")
			return err
		})
	case t.s.Ended():
		d.corpus = t.base
	default:
		_, err = d.w.ExecContext(context.Background(), "ROLLBACK")
		d.corpus = t.base
	}
	if err != nil {
		err = errors.Join(driverError(err), drop(t.s, d))
	}
	return errors.Join(err, t.release())
}

// refreshIn brings the open transaction of the connection w from the corpus before to c, a corpus under the same catalog
// whose rows a transaction changed outside its statements, as an IR request of the session or a savepoint the writer
// began after: in a savepoint of its own, with the standing triggers dropped and the checks ignored, as a fill. A row
// whose key both corpora hold is updated in place, so no foreign key action of a delete fires on the rows that reference
// it; a row only before holds is deleted, whose actions the corpus took too; a row only c holds is inserted. A table whose
// availability differs takes every row of both corpora the same way.
func refreshIn(ctx context.Context, w runner, before, c *irexec.Corpus) (err error) {
	changes, err := c.Changed(before)
	if err != nil {
		return err
	}
	const name = "flowmark_refresh"
	if _, err := w.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return driverError(err)
	}
	defer func() {
		if err != nil {
			if _, undo := w.ExecContext(ctx, "ROLLBACK TO "+name); undo != nil {
				err = errors.Join(err, driverError(undo))
			}
		}
		if _, rerr := w.ExecContext(ctx, "RELEASE "+name); rerr != nil {
			err = errors.Join(err, driverError(rerr))
		}
	}()
	triggers, err := standingTriggers(ctx, w)
	if err != nil {
		return err
	}
	if _, err := w.ExecContext(ctx, "PRAGMA ignore_check_constraints = ON"); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	for _, ch := range changes {
		tb, _ := c.Catalog().Table(ch.Table)
		gone, rows := ch.Gone, ch.Rows
		if ch.Whole {
			if gone, err = before.StoredRows(tb.Name); err != nil {
				return err
			}
			if rows, err = c.StoredRows(tb.Name); err != nil {
				return err
			}
		}
		if err := mergeRows(ctx, w, c, tb, gone, rows); err != nil {
			return err
		}
	}
	for _, tb := range c.Catalog().Tables {
		if tb.Autoincrement && !sameSequence(before, c, tb.Name) {
			if err := setSequence(ctx, w, c, tb); err != nil {
				return err
			}
		}
	}
	if _, err := w.ExecContext(ctx, "PRAGMA ignore_check_constraints = OFF"); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	for _, text := range triggers {
		if _, err := w.ExecContext(ctx, text); err != nil {
			return &Error{codeInternal, fmt.Sprintf("the schema statement %s: %v", text, err)}
		}
	}
	return nil
}

// mergeRows takes the stored rows gone out of the table tb of w and the stored rows rows into it: a row of a key both
// hold is updated, and inserted when an action of a delete took it away; a row of gone alone is deleted.
func mergeRows(ctx context.Context, w runner, c *irexec.Corpus, tb *catalog.Table, gone, rows [][]any) error {
	keys := keyColumns(tb)
	at := make([]int, len(keys))
	where := make([]string, len(keys))
	for i, k := range keys {
		at[i] = slices.IndexFunc(tb.Columns, func(col dbspec.Column) bool { return col.Name == k })
		where[i] = quote(k) + " = ?"
	}
	keyOf := func(row []any) (string, []any, error) {
		args := make([]any, len(at))
		for i, j := range at {
			v, err := stored(tb.Columns[j].Type, row[j])
			if err != nil {
				return "", nil, err
			}
			args[i] = v
		}
		return fmt.Sprintf("%#v", args), args, nil
	}
	staying := map[string]bool{}
	for _, row := range rows {
		k, _, err := keyOf(row)
		if err != nil {
			return err
		}
		staying[k] = true
	}
	for _, row := range gone {
		k, args, err := keyOf(row)
		if err != nil {
			return err
		}
		if staying[k] {
			continue
		}
		if _, err := w.ExecContext(ctx, "DELETE FROM "+quote(tb.Name)+" WHERE "+strings.Join(where, " AND "), args...); err != nil {
			return &Error{codeInternal, fmt.Sprintf("a row of %s: %v", tb.Name, err)}
		}
	}
	names := columnNames(tb)
	set := make([]string, len(names))
	for i, n := range names {
		set[i] = n + " = ?"
	}
	update := "UPDATE " + quote(tb.Name) + " SET " + strings.Join(set, ", ") + " WHERE " + strings.Join(where, " AND ")
	for _, row := range rows {
		_, keyArgs, err := keyOf(row)
		if err != nil {
			return err
		}
		args, err := storedArgs(tb, row)
		if err != nil {
			return err
		}
		res, err := w.ExecContext(ctx, update, append(args, keyArgs...)...)
		if err != nil {
			return &Error{codeInternal, fmt.Sprintf("a row of %s: %v", tb.Name, err)}
		}
		n, err := res.RowsAffected()
		if err != nil {
			return &Error{codeInternal, err.Error()}
		}
		if n == 0 {
			if _, err := w.ExecContext(ctx, insertText(tb), args...); err != nil {
				return &Error{codeInternal, fmt.Sprintf("a row of %s: %v", tb.Name, err)}
			}
		}
		c.Cost().Add(cost.DatabaseRowsWritten, 1)
	}
	return nil
}

// reader is a connection a read runs on and what gives it back: a reader connection of the pool, the connection a
// deferred transaction holds, or the writer connection of a database in memory.
type reader struct {
	d       *database
	q       runner
	release func() error
}

// readerFor returns a connection that reads c or the database's last commit before it, with the settings of s, from the
// database of the corpus of s (spec 40 SQL statements). When the writer connection is free, the database is brought to c
// first; when it is in use, by a statement or a transaction of this process, the read answers from the database's last
// commit: no other process can commit while a session of this one holds the lock. A database in memory reads on its
// writer connection, one read at a time, brought to c.
func readerFor(ctx context.Context, s *writepath.Session, c *irexec.Corpus) (*reader, error) {
	for {
		d, err := currentDatabase(ctx, s)
		if err != nil {
			return nil, err
		}
		memory := d.readers == nil
		d.mu.Lock()
		empty := d.published == nil
		d.mu.Unlock()
		held := false
		switch {
		case memory, empty:
			d.wmu.Lock()
			held = true
		default:
			held = d.wmu.TryLock()
		}
		if held {
			err = d.bring(ctx, c)
			if err != nil {
				d.wmu.Unlock()
				err = errors.Join(err, drop(s, d), d.release())
				if errors.Is(err, errRebuild) {
					continue
				}
				return nil, err
			}
			if !memory {
				d.wmu.Unlock()
			}
		}
		if memory {
			if _, err := d.w.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
				d.wmu.Unlock()
				return nil, errors.Join(&Error{codeInternal, err.Error()}, d.release())
			}
			if d.replaced == nil {
				d.replaced = map[string]string{}
			}
			if err := settleSettings(ctx, d.w, s.Settings(), d.replaced); err != nil {
				d.wmu.Unlock()
				return nil, errors.Join(err, d.release())
			}
			return &reader{d: d, q: d.w, release: func() error {
				d.wmu.Unlock()
				return d.release()
			}}, nil
		}
		return pooled(ctx, s, d)
	}
}

// pooled takes a reader connection of d, waiting for the release of one when every one is in use, with the settings of
// s, which its release gives back.
func pooled(ctx context.Context, s *writepath.Session, d *database) (*reader, error) {
	conn, err := d.readers.Conn(ctx)
	if err != nil {
		return nil, errors.Join(driverError(err), d.release())
	}
	replaced := map[string]string{}
	restore := func() error {
		var err error
		for name, value := range replaced {
			if _, rerr := conn.ExecContext(context.Background(), "PRAGMA "+name+" = "+value); rerr != nil {
				err = errors.Join(err, driverError(rerr))
			}
		}
		return errors.Join(err, conn.Close(), d.release())
	}
	if err := settleSettings(ctx, conn, s.Settings(), replaced); err != nil {
		return nil, errors.Join(err, restore())
	}
	return &reader{d: d, q: conn, release: restore}, nil
}

// pinned is the reader connection a deferred transaction holds from its first read: a read transaction of SQLite, whose
// snapshot is the database's commit of the corpus the transaction pinned. A database in memory holds none: each read of
// the transaction brings its one connection to that corpus.
type pinned struct {
	r *reader
}

// Close ends the read transaction and gives the connection back.
func (p *pinned) Close() error {
	if p.r == nil {
		return nil
	}
	_, err := p.r.q.ExecContext(context.Background(), "ROLLBACK")
	if err != nil {
		err = driverError(err)
	}
	return errors.Join(err, p.r.release())
}

// pin begins the read transaction of the deferred transaction of s on a reader connection, at the database's last
// commit after it was brought to c when the writer was free, and returns it and the corpus of that commit.
func pin(ctx context.Context, s *writepath.Session, c *irexec.Corpus) (*pinned, *irexec.Corpus, error) {
	r, err := readerFor(ctx, s, c)
	if err != nil {
		return nil, nil, err
	}
	d := r.d
	if d.readers == nil {
		return &pinned{}, c, r.release()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := begunRead(ctx, r.q); err != nil {
		return nil, nil, errors.Join(err, r.release())
	}
	return &pinned{r: r}, d.published, nil
}

// begunRead begins a read transaction on q and takes its snapshot with a first read.
func begunRead(ctx context.Context, q runner) error {
	if _, err := q.ExecContext(ctx, "BEGIN"); err != nil {
		return driverError(err)
	}
	var n int
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master").Scan(&n); err != nil {
		return driverError(err)
	}
	return nil
}
