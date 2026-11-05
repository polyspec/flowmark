// Package statement runs one SQL statement on a corpus (spec 40 SQL statements). The corpus is read into an in-memory
// SQLite database of the schema orm renders for its document set, the statement runs there in SQLite's dialect, and
// the rows it changed come back as writes of the executor, which holds every rule of the corpus. The SQLite database
// serves no one else and is rebuilt whenever the corpus differs from it: the corpus stays the only store.
package statement

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/flowmark/packages/go/writepath"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/engine/ir"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Column is one column of a statement's result: its name and the type SQLite declares for it.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Answer is the result of one statement. Each value of Rows is a JSON number, string or null, or {"bytes": base64}
// for a blob.
type Answer struct {
	Columns      []Column `json:"columns"`
	Rows         [][]any  `json:"rows"`
	Affected     int64    `json:"affected"`
	LastInsertID int64    `json:"last_insert_id"`
	// More says that the rows go on: the fetch control message answers the next ones, and close ends them.
	More bool `json:"more,omitempty"`
}

// Error is a coded failure of a statement, in the codes of orm errors.yaml.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

// Failure returns the code and the message of the error.
func (e *Error) Failure() (code, msg string) { return e.Code, e.Msg }

const (
	codeConstraint   = "CONSTRAINT"
	codeDriver       = "DRIVER"
	codeDuplicateKey = "DUPLICATE_KEY"
	codeForeignKey   = "FOREIGN_KEY"
	codeCanceled     = "CANCELED"
	codeReadOnly     = "READ_ONLY"
	codeInternal     = "INTERNAL"
)

// savepoint is the name under which a statement inside a transaction changes the corpus: a refused write of its diff
// leaves the transaction as it was.
const savepoint = "flowmark_statement"

// Run runs text with params in the session s: in its transaction, or as one transaction of its own.
func Run(ctx context.Context, s *writepath.Session, text string, params []json.RawMessage) (_ *Answer, err error) {
	defer func() {
		// A lock wait that ended is SQLite's SQLITE_BUSY, whose name ends the message of a statement as its other results do.
		var locked *writepath.Error
		if errors.As(err, &locked) && locked.ResultCode() == sqlite3.SQLITE_BUSY {
			locked.Msg += resultCodeSuffix(sqlite3.SQLITE_BUSY)
		}
	}()
	// A closed connection ends what its statement is doing, as SQLite's interrupt does.
	ctx = s.Life()
	if err := closeCursor(s); err != nil {
		return nil, err
	}
	if done, err := control(ctx, s, text); done {
		if err != nil {
			return nil, err
		}
		return &Answer{Columns: []Column{}, Rows: [][]any{}}, nil
	}
	args, err := arguments(params)
	if err != nil {
		return nil, err
	}
	if !s.InTransaction() && schemaChange(text) {
		if noChange, err := alreadySo(ctx, s, text); err != nil {
			return nil, err
		} else if noChange {
			return &Answer{Columns: []Column{}, Rows: [][]any{}}, nil
		}
	}
	if s.Deferred() != nil {
		if readsOnly(text) {
			return readDeferred(ctx, s, text, args)
		}
		if err := upgrade(ctx, s); err != nil {
			return nil, err
		}
	}
	if schemaChange(text) {
		return runSchema(ctx, s, text, args)
	}
	if s.InTransaction() {
		return runInTransaction(ctx, s, text, args)
	}
	return runAlone(ctx, s, text, args)
}

// runAlone runs a statement as one transaction of its own. A read that finds the lock held reads what is committed: only
// a writer waits for a writer, as on a SQLite database in WAL mode.
func runAlone(ctx context.Context, s *writepath.Session, text string, args []any) (_ *Answer, err error) {
	if readsOnly(text) {
		answer, read, err := readCommitted(ctx, s, text, args)
		if read {
			return answer, err
		}
	}
	if err := s.Begin(ctx, s.QueryOnly()); err != nil {
		return nil, err
	}
	var open *sql.Rows
	defer func() {
		switch {
		case err != nil:
			err = errors.Join(err, s.Rollback())
		case open != nil:
			// The rows go on, and the transaction with them: the lock is held until they end, as a SQLite read holds its own.
			s.SetCursor(&cursor{rows: open, after: s.Commit})
		default:
			err = s.Commit()
		}
	}()
	var writes [][]byte
	var answer *Answer
	err = s.View(ctx, func(c *irexec.Corpus) error {
		h, err := heldFor(ctx, s, c)
		if err != nil {
			return err
		}
		answer, open, writes, err = execute(ctx, h.db, c, text, args, s.ForeignKeysOff())
		return err
	})
	if err != nil {
		drop(s)
		return nil, err
	}
	if err := applyWrites(ctx, s, writes); err != nil {
		drop(s)
		return nil, err
	}
	return answer, nil
}

// applyWrites writes the difference of a statement to the session's transaction: the database holds what the corpus
// then holds.
func applyWrites(ctx context.Context, s *writepath.Session, writes [][]byte) error {
	for _, msg := range writes {
		if _, err := s.ExecFace(ctx, msg); err != nil {
			return err
		}
	}
	if len(writes) == 0 {
		return nil
	}
	return s.View(ctx, func(c *irexec.Corpus) error {
		s.Held().(*held).corpus = c
		return nil
	})
}

// runInTransaction runs a statement in the open transaction of the session, under a savepoint on both sides: a statement
// that fails, or whose difference a corpus rule refuses, leaves the transaction as it was. When the statement ended the
// SQLite transaction itself, as RAISE(ROLLBACK) does, the session's transaction ends with it.
func runInTransaction(ctx context.Context, s *writepath.Session, text string, args []any) (_ *Answer, err error) {
	if readsOnly(text) {
		var answer *Answer
		err := s.View(ctx, func(c *irexec.Corpus) error {
			h, err := heldFor(ctx, s, c)
			if err != nil {
				return err
			}
			var open *sql.Rows
			if answer, open, err = readBatch(ctx, h.db, text, args); err == nil && open != nil {
				s.SetCursor(&cursor{rows: open})
			}
			return err
		})
		return answer, err
	}
	if err := s.Savepoint(savepoint); err != nil {
		return nil, err
	}
	var writes [][]byte
	var answer *Answer
	var h *held
	err = s.View(ctx, func(c *irexec.Corpus) error {
		var err error
		if h, err = heldFor(ctx, s, c); err != nil {
			return err
		}
		if _, err := h.db.ExecContext(ctx, "SAVEPOINT "+savepoint); err != nil {
			return driverError(err)
		}
		answer, _, writes, err = execute(ctx, h.db, c, text, args, s.ForeignKeysOff())
		return err
	})
	if err == nil {
		err = applyWrites(ctx, s, writes)
	}
	if err == nil {
		if _, err := h.db.ExecContext(ctx, "RELEASE "+savepoint); err != nil {
			drop(s)
			return nil, errors.Join(driverError(err), s.Rollback())
		}
		return answer, s.Release(savepoint)
	}
	if h == nil {
		return nil, errors.Join(err, s.RollbackTo(savepoint), s.Release(savepoint))
	}
	if _, undo := h.db.ExecContext(ctx, "ROLLBACK TO "+savepoint); undo != nil {
		drop(s)
		s.SetEnded(true)
		return nil, errors.Join(err, s.Rollback())
	}
	if _, undo := h.db.ExecContext(ctx, "RELEASE "+savepoint); undo != nil {
		drop(s)
		return nil, errors.Join(err, driverError(undo), s.Rollback())
	}
	err = errors.Join(err, s.RollbackTo(savepoint), s.Release(savepoint))
	return nil, errors.Join(err, s.View(ctx, func(c *irexec.Corpus) error {
		h.corpus = c
		return nil
	}))
}

// readCommitted answers a read from what is committed when another session holds the lock, on a database of its own, and
// reports whether it did. It is not a read when the lock is free, since the session then reads under it.
func readCommitted(ctx context.Context, s *writepath.Session, text string, args []any) (*Answer, bool, error) {
	var answer *Answer
	read, err := s.ReadCommitted(ctx, func(c *irexec.Corpus, slot *io.Closer, unlock func()) (keep bool, err error) {
		answer, keep, err = readInto(ctx, s, c, slot, unlock, text, args)
		return keep, err
	})
	return answer, read, err
}

// staleSnapshot is the refusal of the first write of a transaction that read a corpus committed over since.
type staleSnapshot struct{}

func (staleSnapshot) Error() string {
	return "database is locked" + resultCodeSuffix(sqlite3.SQLITE_BUSY_SNAPSHOT)
}

// Failure is CANCELED, the code of every lock result of SQLite.
func (e staleSnapshot) Failure() (code, msg string) { return codeCanceled, e.Error() }

// ResultCode is SQLITE_BUSY_SNAPSHOT, for the answer of the statement.
func (staleSnapshot) ResultCode() int { return sqlite3.SQLITE_BUSY_SNAPSHOT }

// readInto answers a read from the corpus c on the database of the reads without the lock.
func readInto(ctx context.Context, s *writepath.Session, c *irexec.Corpus, slot *io.Closer, unlock func(), text string, args []any) (*Answer, bool, error) {
	h, ok := (*slot).(*held)
	if !ok || h.corpus.Catalog() != c.Catalog() || len(h.corpus.Differs(c)) > 0 {
		if ok {
			_ = h.Close()
		}
		*slot = nil
		db, err := materialize(ctx, c)
		if err != nil {
			return nil, false, err
		}
		h = &held{db: db, corpus: c}
		*slot = h
	}
	if err := h.settle(ctx, s); err != nil {
		return nil, false, err
	}
	if _, err := h.db.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		return nil, false, driverError(err)
	}
	answer, open, err := readBatch(ctx, h.db, text, args)
	if err != nil {
		return nil, false, err
	}
	if open == nil {
		return answer, false, nil
	}
	s.SetCursor(&cursor{rows: open, unlock: unlock})
	return answer, true, nil
}

// readDeferred answers a read of a deferred transaction from the corpus its first read fixes.
func readDeferred(ctx context.Context, s *writepath.Session, text string, args []any) (*Answer, error) {
	d := s.Deferred()
	if d.Pinned == nil {
		c, err := s.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		d.Pinned = c
	}
	var answer *Answer
	err := s.ReadSnapshot(func(slot *io.Closer, unlock func()) (keep bool, err error) {
		answer, keep, err = readInto(ctx, s, d.Pinned, slot, unlock, text, args)
		return keep, err
	})
	return answer, err
}

// upgrade turns a deferred transaction into one that holds the lock, at its first write. A wait that ends leaves it
// deferred, and so does a corpus committed since the transaction's first read, whose snapshot can no longer be written:
// SQLITE_BUSY and SQLITE_BUSY_SNAPSHOT, for the ROLLBACK that follows.
func upgrade(ctx context.Context, s *writepath.Session) error {
	d := s.Deferred()
	if err := s.Begin(ctx, s.QueryOnly()); err != nil {
		return err
	}
	abandon := func(err error) error {
		drop(s)
		return errors.Join(err, s.Rollback())
	}
	var stale bool
	err := s.View(ctx, func(c *irexec.Corpus) error {
		if d.Pinned != nil && len(d.Pinned.Differs(c)) > 0 {
			stale = true
			return nil
		}
		h, err := heldFor(ctx, s, c)
		if err != nil {
			return err
		}
		if _, err := h.db.ExecContext(ctx, "BEGIN"); err != nil {
			return driverError(err)
		}
		h.owner, h.base = s, c
		return nil
	})
	if err != nil {
		return abandon(err)
	}
	if stale {
		return errors.Join(staleSnapshot{}, s.Rollback())
	}
	for _, name := range d.Savepoints {
		if err := s.Savepoint(name); err != nil {
			return abandon(err)
		}
		if err := mirror(ctx, s, "SAVEPOINT "+quote(name)); err != nil {
			return err
		}
	}
	s.SetDeferred(nil)
	return nil
}

// A statement's rows go in batches, so a result takes the memory of what its client reads, as on a SQLite connection.
const (
	batchRows  = 1000
	batchBytes = 1 << 20
)

// cursor is the rows of a statement that go on. Close frees what they hold, and after is what the statement still owes
// its session once they end.
type cursor struct {
	rows   *sql.Rows
	after  func() error
	unlock func()
}

// Close closes the rows, and it is also what the end of a session calls.
func (c *cursor) Close() error {
	err := c.rows.Close()
	if c.unlock != nil {
		c.unlock()
		c.unlock = nil
	}
	return err
}

// closeCursor ends the rows of the session's last statement, if they go on.
func closeCursor(s *writepath.Session) error {
	c, ok := s.TakeCursor().(*cursor)
	if !ok {
		return nil
	}
	err := c.Close()
	if c.after != nil {
		err = errors.Join(err, c.after())
	}
	return err
}

// Fetch answers the next rows of the session's last statement.
func Fetch(s *writepath.Session) (*Answer, error) {
	c, ok := s.Cursor().(*cursor)
	if !ok {
		return nil, &Error{codeDriver, "no statement of the session has rows that go on"}
	}
	a := &Answer{Rows: [][]any{}}
	more, err := nextBatch(c.rows, a, len(mustColumns(c.rows)))
	if err != nil {
		return nil, errors.Join(err, closeCursor(s))
	}
	a.More = more
	if !more {
		if err := closeCursor(s); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// Close ends the rows of the session's last statement.
func Close(s *writepath.Session) error { return closeCursor(s) }

func mustColumns(rows *sql.Rows) []string {
	names, err := rows.Columns()
	if err != nil {
		return nil
	}
	return names
}

// nextBatch reads a batch of rows into a, and reports whether rows may go on.
func nextBatch(rows *sql.Rows, a *Answer, columns int) (bool, error) {
	size := 0
	for rows.Next() {
		row, err := scan(rows, columns)
		if err != nil {
			return false, driverError(err)
		}
		for i, v := range row {
			row[i] = answerValue(v)
			size += 16
			if text, ok := v.(string); ok {
				size += len(text)
			} else if b, ok := v.([]byte); ok {
				size += len(b)
			}
		}
		a.Rows = append(a.Rows, row)
		if len(a.Rows) >= batchRows || size >= batchBytes {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, driverError(err)
	}
	return false, nil
}

// readBatch runs a read and answers its first batch, with the rows when they go on.
func readBatch(ctx context.Context, db *sql.DB, text string, args []any) (*Answer, *sql.Rows, error) {
	rows, err := db.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, nil, driverError(err)
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, nil, errors.Join(driverError(err), rows.Close())
	}
	a := &Answer{Columns: make([]Column, len(types)), Rows: [][]any{}}
	for i, t := range types {
		a.Columns[i] = Column{Name: t.Name(), Type: t.DatabaseTypeName()}
	}
	more, err := nextBatch(rows, a, len(types))
	if err != nil {
		return nil, nil, errors.Join(err, rows.Close())
	}
	if more {
		a.More = true
		return a, rows, nil
	}
	if err := rows.Close(); err != nil {
		return nil, nil, driverError(err)
	}
	return a, nil, nil
}

// held is the SQLite database of a corpus, kept between statements while the corpus stays the one it was filled from:
// a statement costs its own run, not a read of every table. It lives under the corpus lock, which a statement holds.
type held struct {
	db     *sql.DB
	corpus *irexec.Corpus
	// replaced is the value each PRAGMA setting of a session replaced.
	replaced map[string]string
	// owner is the session whose transaction the database's own transaction is, and base the corpus it began from.
	owner *writepath.Session
	base  *irexec.Corpus
}

func (h *held) Close() error { return h.db.Close() }

// heldFor returns the database that holds the corpus c, filling a new one when the corpus has changed since.
func heldFor(ctx context.Context, s *writepath.Session, c *irexec.Corpus) (*held, error) {
	h, ok := s.Held().(*held)
	// A transaction whose session ended without a word to the database, as an idle one does, went with it.
	if ok && (h.owner == nil || h.owner == s) && len(h.corpus.Differs(c)) == 0 {
		h.corpus = c
	} else {
		drop(s)
		db, err := materialize(ctx, c)
		if err != nil {
			return nil, err
		}
		h = &held{db: db, corpus: c}
		s.SetHeld(h)
	}
	if err := h.settle(ctx, s); err != nil {
		return nil, err
	}
	return h, nil
}

// settle gives the database the settings of the session: it is shared by the sessions of the corpus, and a PRAGMA is
// the setting of one connection, so a setting of another session goes back to the value it replaced.
func (h *held) settle(ctx context.Context, s *writepath.Session) error {
	queryOnly := "OFF"
	if s.QueryOnly() {
		queryOnly = "ON"
	}
	if _, err := h.db.ExecContext(ctx, "PRAGMA query_only = "+queryOnly); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	foreignKeys := "ON"
	if s.ForeignKeysOff() {
		foreignKeys = "OFF"
	}
	if _, err := h.db.ExecContext(ctx, "PRAGMA foreign_keys = "+foreignKeys); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	want := s.Settings()
	for name, value := range h.replaced {
		if _, kept := want[name]; kept {
			continue
		}
		if _, err := h.db.ExecContext(ctx, "PRAGMA "+name+" = "+value); err != nil {
			return driverError(err)
		}
		delete(h.replaced, name)
	}
	for name, value := range want {
		if _, kept := h.replaced[name]; !kept {
			var old string
			switch err := h.db.QueryRowContext(ctx, "PRAGMA "+name).Scan(&old); {
			case err == nil:
				if h.replaced == nil {
					h.replaced = map[string]string{}
				}
				h.replaced[name] = old
			case !errors.Is(err, sql.ErrNoRows):
				return driverError(err)
			}
		}
		if _, err := h.db.ExecContext(ctx, "PRAGMA "+name+" = "+value); err != nil {
			return driverError(err)
		}
	}
	return nil
}

// drop closes the database of the corpus: it may hold what the corpus does not.
func drop(s *writepath.Session) {
	if h := s.Held(); h != nil {
		_ = h.Close()
		s.SetHeld(nil)
	}
}

// temporalWire writes a text SQLite holds for a datetime or a time in its wire form, six fraction digits: a
// statement may store any fraction and a date with no time, as a SQLite column of those types does.
func temporalWire(typ dbspec.Type, text string) string {
	if typ.Kind != dbspec.TypeDatetime && typ.Kind != dbspec.TypeTime {
		return text
	}
	text = strings.TrimSuffix(strings.Replace(text, "T", " ", 1), "Z")
	if typ.Kind == dbspec.TypeDatetime && len(text) == len("2006-01-02") {
		text += " 00:00:00"
	}
	whole, fraction, _ := strings.Cut(text, ".")
	if len(fraction) > 6 {
		fraction = fraction[:6]
	}
	return whole + "." + fraction + strings.Repeat("0", 6-len(fraction))
}

// setting reads a PRAGMA that sets a value.
var setting = regexp.MustCompile(`(?i)^PRAGMA\s+([a-z_]+)\s*=\s*"?([a-z0-9_-]+)"?$`)

// begin opens the session's transaction and the database's own, so that SQLite judges at COMMIT what it defers to it.
func begin(ctx context.Context, s *writepath.Session) error {
	if err := s.Begin(ctx, s.QueryOnly()); err != nil {
		return err
	}
	s.SetEnded(false)
	err := s.View(ctx, func(c *irexec.Corpus) error {
		h, err := heldFor(ctx, s, c)
		if err != nil {
			return err
		}
		if _, err := h.db.ExecContext(ctx, "BEGIN"); err != nil {
			return driverError(err)
		}
		h.owner, h.base = s, c
		return nil
	})
	if err != nil {
		drop(s)
		return errors.Join(err, s.Rollback())
	}
	return nil
}

// owned is the database whose transaction is that of the session, nil when it has none.
func owned(s *writepath.Session) *held {
	if s.Transaction() {
		if h, ok := s.Held().(*held); ok && h.owner == s {
			return h
		}
	}
	return nil
}

// commit commits the database's transaction and then the session's. What SQLite refuses at COMMIT leaves both open, as
// on a SQLite connection, for the ROLLBACK that follows.
func commit(s *writepath.Session) error {
	if h := owned(s); h != nil {
		if _, err := h.db.Exec("COMMIT"); err != nil {
			return driverError(err)
		}
		h.owner, h.base = nil, nil
	}
	if err := s.Commit(); err != nil {
		drop(s)
		return err
	}
	return nil
}

// rollback rolls back both transactions.
func rollback(s *writepath.Session) error {
	if h := owned(s); h != nil {
		if _, err := h.db.Exec("ROLLBACK"); err != nil {
			drop(s)
			return errors.Join(driverError(err), s.Rollback())
		}
		h.corpus, h.owner, h.base = h.base, nil, nil
	}
	return s.Rollback()
}

// rollbackTo rolls back to a savepoint on both sides.
func rollbackTo(ctx context.Context, s *writepath.Session, text, name string) error {
	if err := s.RollbackTo(name); err != nil {
		return err
	}
	if err := mirror(ctx, s, text); err != nil {
		return err
	}
	return s.View(ctx, func(c *irexec.Corpus) error {
		if h := s.Held(); h != nil {
			h.(*held).corpus = c
		}
		return nil
	})
}

// mirror runs a savepoint statement on the database's transaction.
func mirror(ctx context.Context, s *writepath.Session, text string) error {
	if h := owned(s); h != nil {
		if _, err := h.db.ExecContext(ctx, text); err != nil {
			drop(s)
			return errors.Join(driverError(err), s.Rollback())
		}
	}
	return nil
}

// control runs a statement that controls the session as the session's own operation, and reports whether text is one:
// the transaction of a SQLite connection, and its query_only, which reads the corpus without writing it.
func control(ctx context.Context, s *writepath.Session, text string) (bool, error) {
	fields := strings.Fields(strings.ToUpper(strings.TrimRight(strings.TrimSpace(text), "; ")))
	if len(fields) == 0 {
		return false, nil
	}
	name := func(i int) string {
		words := strings.Fields(strings.TrimRight(strings.TrimSpace(text), "; "))
		return strings.Trim(words[i], `"`)
	}
	switch fields[0] {
	case "BEGIN":
		if s.Deferred() != nil {
			return true, &Error{codeDriver, "cannot start a transaction within a transaction"}
		}
		// A plain BEGIN takes the lock at its first write and IMMEDIATE and EXCLUSIVE at once, as on SQLite.
		if !slices.Contains(fields, "IMMEDIATE") && !slices.Contains(fields, "EXCLUSIVE") && !s.InTransaction() {
			s.SetEnded(false)
			s.SetDeferred(&writepath.Deferred{})
			return true, nil
		}
		return true, begin(ctx, s)
	case "COMMIT", "END":
		if s.Deferred() != nil {
			s.SetDeferred(nil)
			return true, nil
		}
		return true, commit(s)
	case "ROLLBACK":
		if s.Ended() {
			return true, nil
		}
		whole := len(fields) == 1 || (len(fields) == 2 && fields[1] == "TRANSACTION")
		if d := s.Deferred(); d != nil {
			if whole {
				s.SetDeferred(nil)
				return true, nil
			}
			i := slices.Index(d.Savepoints, name(len(fields)-1))
			if i < 0 {
				return true, &Error{codeDriver, "no such savepoint: " + name(len(fields)-1)}
			}
			d.Savepoints = d.Savepoints[:i+1]
			return true, nil
		}
		if whole {
			return true, rollback(s)
		}
		return true, rollbackTo(ctx, s, text, name(len(fields)-1))
	case "SAVEPOINT":
		if d := s.Deferred(); d != nil {
			d.Savepoints = append(d.Savepoints, name(1))
			return true, nil
		}
		if err := s.Savepoint(name(1)); err != nil {
			return true, err
		}
		return true, mirror(ctx, s, text)
	case "RELEASE":
		if s.Ended() {
			return true, nil
		}
		if d := s.Deferred(); d != nil {
			i := slices.Index(d.Savepoints, name(len(fields)-1))
			if i < 0 {
				return true, &Error{codeDriver, "no such savepoint: " + name(len(fields)-1)}
			}
			d.Savepoints = d.Savepoints[:i]
			return true, nil
		}
		if err := s.Release(name(len(fields) - 1)); err != nil {
			return true, err
		}
		return true, mirror(ctx, s, text)
	case "PRAGMA":
		switch strings.ReplaceAll(strings.Join(fields[1:], ""), " ", "") {
		case "QUERY_ONLY=ON", "QUERY_ONLY=1":
			s.SetQueryOnly(true)
			return true, nil
		case "QUERY_ONLY=OFF", "QUERY_ONLY=0":
			s.SetQueryOnly(false)
			return true, nil
		case "FOREIGN_KEYS=ON", "FOREIGN_KEYS=1":
			s.SetForeignKeysOff(false)
			return true, nil
		case "FOREIGN_KEYS=OFF", "FOREIGN_KEYS=0":
			s.SetForeignKeysOff(true)
			return true, nil
		}
		if m := setting.FindStringSubmatch(strings.TrimRight(strings.TrimSpace(text), "; ")); m != nil {
			name, value := strings.ToLower(m[1]), m[2]
			if name == "busy_timeout" {
				ms, err := strconv.ParseInt(value, 10, 64)
				if err != nil || ms < 0 {
					return true, &Error{codeDriver, "busy_timeout is a count of milliseconds"}
				}
				s.SetBusyTimeout(ms)
				return true, nil
			}
			if name == "locking_mode" {
				return true, s.SetExclusive(strings.EqualFold(value, "EXCLUSIVE"))
			}
			s.SetSetting(name, value)
			return true, nil
		}
	}
	return false, nil
}

// arguments reads the bound values of a statement: a JSON number, string, boolean or null, or {"bytes": base64}.
func arguments(params []json.RawMessage) ([]any, error) {
	out := make([]any, len(params))
	for i, raw := range params {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, &Error{codeDriver, fmt.Sprintf("parameter %d: %v", i, err)}
		}
		switch t := v.(type) {
		case json.Number:
			if n, err := t.Int64(); err == nil {
				out[i] = n
			} else if f, err := t.Float64(); err == nil {
				out[i] = f
			} else {
				return nil, &Error{codeDriver, fmt.Sprintf("parameter %d: %s is no number", i, t)}
			}
		case map[string]any:
			text, ok := t["bytes"].(string)
			if !ok || len(t) != 1 {
				return nil, &Error{codeDriver, fmt.Sprintf("parameter %d: an object is {\"bytes\": base64}", i)}
			}
			b, err := base64.StdEncoding.DecodeString(text)
			if err != nil {
				return nil, &Error{codeDriver, fmt.Sprintf("parameter %d: %v", i, err)}
			}
			out[i] = b
		default:
			out[i] = v
		}
	}
	return out, nil
}

// materialize returns an in-memory SQLite database that holds the schema and the rows of the corpus.
func materialize(ctx context.Context, c *irexec.Corpus) (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	db.SetMaxOpenConns(1)
	if err := fill(ctx, db, c); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func fill(ctx context.Context, db *sql.DB, c *irexec.Corpus) error {
	cat := c.Catalog()
	schema, err := catalog.SchemaStatements(cat.Documents, cat.Statements)
	if err != nil {
		return &Error{codeInternal, err.Error()}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return &Error{codeInternal, err.Error()}
	}
	defer func() { _ = tx.Rollback() }()
	for _, text := range schema {
		if _, err := tx.ExecContext(ctx, text); err != nil {
			return &Error{codeInternal, fmt.Sprintf("the schema statement %s: %v", text, err)}
		}
	}
	// The triggers that stand wait for the rows: a stored row has had its audit and its checks.
	triggers, err := standingTriggers(ctx, tx)
	if err != nil {
		return err
	}
	// A stored row has had its checks too; a statement can have stored it past them, as ignore_check_constraints lets it.
	if _, err := tx.ExecContext(ctx, "PRAGMA ignore_check_constraints = ON"); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	for _, tb := range cat.Tables {
		rows, err := c.StoredRows(tb.Name)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			continue
		}
		insert, err := tx.PrepareContext(ctx, insertText(tb))
		if err != nil {
			return &Error{codeInternal, err.Error()}
		}
		defer func() { _ = insert.Close() }()
		for _, row := range rows {
			args := make([]any, len(row))
			for i, v := range row {
				if args[i], err = stored(tb.Columns[i].Type, v); err != nil {
					return err
				}
			}
			if _, err := insert.ExecContext(ctx, args...); err != nil {
				return &Error{codeInternal, fmt.Sprintf("a row of %s: %v", tb.Name, err)}
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA ignore_check_constraints = OFF"); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	for _, text := range triggers {
		if _, err := tx.ExecContext(ctx, text); err != nil {
			return &Error{codeInternal, fmt.Sprintf("the schema statement %s: %v", text, err)}
		}
	}
	if err := tx.Commit(); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	// The rows are in; a statement meets the foreign keys as a SQLite connection of orm does.
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	return nil
}

// standingTriggers drops the triggers of the schema and returns the statements that create them.
func standingTriggers(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT name, sql FROM sqlite_master WHERE type = 'trigger'")
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	out := map[string]string{}
	for rows.Next() {
		var name, text string
		if err := rows.Scan(&name, &text); err != nil {
			_ = rows.Close()
			return nil, &Error{codeInternal, err.Error()}
		}
		out[name] = text
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	for name := range out {
		if _, err := tx.ExecContext(ctx, "DROP TRIGGER "+quote(name)); err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
	}
	return out, nil
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func insertText(tb *catalog.Table) string {
	names := make([]string, len(tb.Columns))
	marks := make([]string, len(tb.Columns))
	for i, col := range tb.Columns {
		names[i] = quote(col.Name)
		if col.Name == catalog.RowIDColumn {
			names[i] = catalog.RowIDColumn
		}
		marks[i] = "?"
	}
	return "INSERT INTO " + quote(tb.Name) + " (" + strings.Join(names, ", ") + ") VALUES (" + strings.Join(marks, ", ") + ")"
}

// stored is the SQLite value of the value v of a column, read from the corpus in the runtime value model JSON: orm's
// SQLite schema keeps a decimal as the integer of its scale, a boolean as 0 or 1 and bytes as a blob.
func stored(t dbspec.Type, v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case store.Literal:
		return literalValue(string(x))
	case bool:
		if x {
			return int64(1), nil
		}
		return int64(0), nil
	case json.Number:
		if t.Kind == dbspec.TypeF64 {
			f, err := x.Float64()
			if err != nil {
				return nil, &Error{codeInternal, err.Error()}
			}
			return f, nil
		}
		n, err := x.Int64()
		if err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		return n, nil
	case string:
		switch t.Kind {
		case dbspec.TypeBytes:
			b, err := base64.StdEncoding.DecodeString(x)
			if err != nil {
				return nil, &Error{codeInternal, err.Error()}
			}
			return b, nil
		case dbspec.TypeDecimal:
			return scaled(x, t.Scale)
		}
		return x, nil
	}
	return nil, &Error{codeInternal, fmt.Sprintf("a stored value is %T", v)}
}

// scaled is the integer of a decimal text at scale.
func scaled(text string, scale int) (int64, error) {
	whole, fraction, _ := strings.Cut(text, ".")
	for len(fraction) < scale {
		fraction += "0"
	}
	n, err := strconv.ParseInt(whole+fraction[:scale], 10, 64)
	if err != nil {
		return 0, &Error{codeInternal, fmt.Sprintf("the decimal %q: %v", text, err)}
	}
	return n, nil
}

// decimalText is the decimal text of the integer n at scale.
func decimalText(n int64, scale int) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	if scale == 0 {
		return sign + digits
	}
	for len(digits) <= scale {
		digits = "0" + digits
	}
	return sign + digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
}

// execute runs the statement on db, and returns its answer and the request messages of the rows it changed.
func execute(ctx context.Context, db *sql.DB, c *irexec.Corpus, text string, args []any, foreignKeysOff bool) (*Answer, *sql.Rows, [][]byte, error) {
	if readsOnly(text) {
		answer, open, err := readBatch(ctx, db, text, args)
		return answer, open, nil, err
	}
	before, err := allRows(ctx, db, c)
	if err != nil {
		return nil, nil, nil, err
	}
	answer, err := query(ctx, db, text, args)
	if err != nil {
		return nil, nil, nil, err
	}
	after, err := allRows(ctx, db, c)
	if err != nil {
		return nil, nil, nil, err
	}
	writes, err := diff(c, before, after, foreignKeysOff)
	if err != nil {
		return nil, nil, nil, err
	}
	return answer, nil, writes, nil
}

// readsOnly reports whether the statement cannot change a row, by its first word; a statement of WITH is known by the
// word its common table expressions lead to.
func readsOnly(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return true
	}
	first := strings.ToUpper(fields[0])
	if first == "WITH" {
		first = afterCommonTables(text)
	}
	switch first {
	case "SELECT", "VALUES", "EXPLAIN", "PRAGMA":
		return true
	}
	return false
}

// afterCommonTables is the first main verb of a WITH statement outside its parentheses and its quotes.
func afterCommonTables(text string) string {
	depth := 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case depth == 0 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'):
			j := i
			for j < len(text) && (text[j] >= 'a' && text[j] <= 'z' || text[j] >= 'A' && text[j] <= 'Z' || text[j] == '_') {
				j++
			}
			switch word := strings.ToUpper(text[i:j]); word {
			case "SELECT", "VALUES", "INSERT", "UPDATE", "DELETE", "REPLACE":
				return word
			}
			i = j - 1
		}
	}
	return ""
}

// query runs the statement and reads its result.
func query(ctx context.Context, db *sql.DB, text string, args []any) (*Answer, error) {
	rows, err := db.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, driverError(err)
	}
	defer func() { _ = rows.Close() }()
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, driverError(err)
	}
	a := &Answer{Columns: make([]Column, len(types)), Rows: [][]any{}}
	for i, t := range types {
		a.Columns[i] = Column{Name: t.Name(), Type: t.DatabaseTypeName()}
	}
	for rows.Next() {
		row, err := scan(rows, len(types))
		if err != nil {
			return nil, driverError(err)
		}
		for i, v := range row {
			row[i] = answerValue(v)
		}
		a.Rows = append(a.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, driverError(err)
	}
	if err := rows.Close(); err != nil {
		return nil, driverError(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT changes(), last_insert_rowid()").Scan(&a.Affected, &a.LastInsertID); err != nil {
		return nil, driverError(err)
	}
	return a, nil
}

func scan(rows *sql.Rows, n int) ([]any, error) {
	row := make([]any, n)
	dest := make([]any, n)
	for i := range row {
		dest[i] = &row[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}
	return row, nil
}

func answerValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return map[string]string{"bytes": base64.StdEncoding.EncodeToString(t)}
	case time.Time:
		return t.UTC().Format("2006-01-02 15:04:05.000000")
	case float64:
		// JSON has no infinity, and SQLite stores one.
		if math.IsInf(t, 1) {
			return map[string]string{"float": "+Inf"}
		}
		if math.IsInf(t, -1) {
			return map[string]string{"float": "-Inf"}
		}
	case string:
		// JSON text is UTF-8, and a SQLite text holds whatever bytes it was given.
		if !utf8.ValidString(t) {
			return map[string]string{"text": base64.StdEncoding.EncodeToString([]byte(t))}
		}
	}
	return v
}

// driverError is the failure of a statement SQLite refused, under the code orm gives that result code on a SQLite
// connection (errors.yaml).
func driverError(err error) error {
	var e *sqlite.Error
	if !errors.As(err, &e) {
		return &Error{codeDriver, err.Error()}
	}
	n := e.Code()
	suffix := resultCodeSuffix(n)
	msg := strings.TrimSuffix(e.Error(), suffix) + suffix
	switch {
	case n&0xff == sqlite3.SQLITE_BUSY, n == sqlite3.SQLITE_INTERRUPT:
		return &Error{codeCanceled, msg}
	case n == sqlite3.SQLITE_CONSTRAINT_UNIQUE, n == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return &Error{codeDuplicateKey, msg}
	case n == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
		return &Error{codeForeignKey, msg}
	case n == sqlite3.SQLITE_CONSTRAINT_TRIGGER && strings.Contains(e.Error(), "FOREIGN KEY constraint failed"):
		return &Error{codeForeignKey, msg}
	case n == sqlite3.SQLITE_CONSTRAINT_CHECK:
		return &Error{codeConstraint, msg}
	case n&0xff == sqlite3.SQLITE_READONLY:
		return &Error{codeReadOnly, msg}
	}
	return &Error{codeDriver, msg}
}

// allRows reads the rows of every table by their key.
func allRows(ctx context.Context, db *sql.DB, c *irexec.Corpus) (map[string]map[string][]any, error) {
	out := map[string]map[string][]any{}
	for _, tb := range c.Catalog().Tables {
		rows, err := rowsOf(ctx, db, tb)
		if err != nil {
			return nil, err
		}
		byKey := make(map[string][]any, len(rows))
		for _, row := range rows {
			byKey[rowKey(tb, row)] = row
		}
		out[tb.Name] = byKey
	}
	return out, nil
}

// rowsOf reads the rows of tb from db.
func rowsOf(ctx context.Context, db *sql.DB, tb *catalog.Table) ([][]any, error) {
	names := make([]string, len(tb.Columns))
	for i, col := range tb.Columns {
		names[i] = quote(col.Name)
		if col.Name == catalog.RowIDColumn {
			names[i] = catalog.RowIDColumn
		}
		// Without its declared type the driver reads the stored value itself, not a time of a DATE or DATETIME column.
		names[i] = "+" + names[i]
	}
	rows, err := db.QueryContext(ctx, "SELECT "+strings.Join(names, ", ")+" FROM "+quote(tb.Name))
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	defer func() { _ = rows.Close() }()
	var out [][]any
	for rows.Next() {
		row, err := scan(rows, len(names))
		if err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	return out, nil
}

// keyColumns are the names of the columns of the primary key of tb.
func keyColumns(tb *catalog.Table) []string {
	if len(tb.KeyParts) > 0 {
		return tb.KeyParts
	}
	return []string{tb.Key}
}

func rowKey(tb *catalog.Table, row []any) string {
	var key strings.Builder
	for _, name := range keyColumns(tb) {
		i := slices.IndexFunc(tb.Columns, func(col dbspec.Column) bool { return col.Name == name })
		fmt.Fprintf(&key, "%v\x00", row[i])
	}
	return key.String()
}

// diff forms the writes that take the corpus from before to after: the inserts with a table after the tables it
// references, then the updates, then the deletes with a table before the tables it references. A delete is the
// statement's own, so a soft delete column does not turn it into an update, and the rows of its foreign key actions
// are in the difference already.
func diff(c *irexec.Corpus, before, after map[string]map[string][]any, foreignKeysOff bool) ([][]byte, error) {
	cat := c.Catalog()
	var inserts, changes, deletes [][]byte
	order := fillOrder(cat)
	for _, tb := range order {
		var added [][]any
		var changed [][2][]any
		for key, row := range after[tb.Name] {
			old, held := before[tb.Name][key]
			if !held {
				added = append(added, row)
			} else if !slices.EqualFunc(old, row, same) {
				changed = append(changed, [2][]any{old, row})
			}
		}
		if len(changed) > 0 {
			msg, err := updateMessage(cat, tb, changed, foreignKeysOff)
			if err != nil {
				return nil, err
			}
			changes = append(changes, msg)
		}
		if len(added) > 0 {
			msg, err := insertMessage(cat, tb, added, foreignKeysOff)
			if err != nil {
				return nil, err
			}
			inserts = append(inserts, msg)
		}
	}
	for _, tb := range slices.Backward(order) {
		var gone [][]any
		for key, row := range before[tb.Name] {
			if _, held := after[tb.Name][key]; !held {
				gone = append(gone, row)
			}
		}
		if len(gone) > 0 {
			msg, err := deleteMessage(cat, tb, gone, foreignKeysOff)
			if err != nil {
				return nil, err
			}
			deletes = append(deletes, msg)
		}
	}
	return slices.Concat(inserts, changes, deletes), nil
}

// fillOrder is the tables with each after the tables it references.
func fillOrder(cat *catalog.Catalog) []*catalog.Table {
	rest := slices.Clone(cat.Tables)
	done := map[string]bool{}
	var out []*catalog.Table
	for len(rest) > 0 {
		next := 0
		for i, tb := range rest {
			if !slices.ContainsFunc(tb.ForeignKeys, func(fk dbspec.ForeignKey) bool { return fk.Table != tb.Name && !done[fk.Table] }) {
				next = i
				break
			}
		}
		done[rest[next].Name] = true
		out = append(out, rest[next])
		rest = slices.Delete(rest, next, next+1)
	}
	return out
}

// message forms one write of the statement's difference.
type message struct {
	keys    [][]int
	updates []map[string]any
	params  []json.RawMessage
}

func (m *message) param(t dbspec.Type, v any) (map[string]any, error) {
	raw, err := wire(t, v)
	if err != nil {
		return nil, err
	}
	m.params = append(m.params, raw)
	return map[string]any{"p": len(m.params) - 1}, nil
}

func (m *message) assign(col dbspec.Column, v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{"column": col.Name, "null": true}, nil
	}
	a, err := m.param(col.Type, v)
	if err != nil {
		return nil, err
	}
	a["column"] = col.Name
	return a, nil
}

func (m *message) where(tb *catalog.Table, row []any) (map[string]any, error) {
	var items []any
	for _, name := range keyColumns(tb) {
		i := slices.IndexFunc(tb.Columns, func(col dbspec.Column) bool { return col.Name == name })
		pred, err := m.param(tb.Columns[i].Type, row[i])
		if err != nil {
			return nil, err
		}
		pred["column"] = name
		pred["op"] = "eq"
		items = append(items, map[string]any{"pred": pred})
	}
	return map[string]any{"items": items}, nil
}

func (m *message) request(cat *catalog.Catalog, tb *catalog.Table, kind string, body map[string]any, foreignKeysOff bool) ([]byte, error) {
	req := map[string]any{"ir_version": ir.Version, "manifest_hash": cat.ManifestHash, "kind": kind, "entity": tb.Name, "n_params": len(m.params)}
	for k, v := range body {
		req[k] = v
	}
	params := m.params
	if params == nil {
		params = []json.RawMessage{}
	}
	now := time.Now().UTC().Truncate(time.Microsecond).Format("2006-01-02 15:04:05.000000")
	out := map[string]any{"ir_version": ir.Version, "manifest_hash": cat.ManifestHash, "request": req, "params": params, "now": now, "seed": true, "foreign_keys_off": &foreignKeysOff}
	if m.keys != nil {
		out["keys"] = m.keys
	}
	if m.updates != nil {
		out["updates"] = m.updates
	}
	return json.Marshal(out)
}

// insertMessage is the insert of the rows of a table: one request, whose rows the corpus takes at once.
func insertMessage(cat *catalog.Catalog, tb *catalog.Table, rows [][]any, foreignKeysOff bool) ([]byte, error) {
	m := &message{}
	set := make([]map[string]any, len(tb.Columns))
	load := false
	more := make([][]int, 0, len(rows)-1)
	for r, row := range rows {
		var params []int
		for i, col := range tb.Columns {
			a, err := m.param(col.Type, row[i])
			if err != nil {
				return nil, err
			}
			if r == 0 {
				a["column"] = col.Name
				set[i] = a
				load = load || col.Identity
				continue
			}
			params = append(params, a["p"].(int))
		}
		if r > 0 {
			more = append(more, params)
		}
	}
	body := map[string]any{"set": set}
	if load {
		body["load"] = true
	}
	if len(more) > 0 {
		body["rows"] = more
	}
	return m.request(cat, tb, "insert", body, foreignKeysOff)
}

// keyParams adds the key of row to the parameters of m.
func (m *message) keyParams(tb *catalog.Table, row []any) ([]int, error) {
	var out []int
	for _, name := range keyColumns(tb) {
		i := slices.IndexFunc(tb.Columns, func(col dbspec.Column) bool { return col.Name == name })
		a, err := m.param(tb.Columns[i].Type, row[i])
		if err != nil {
			return nil, err
		}
		out = append(out, a["p"].(int))
	}
	return out, nil
}

// updateMessage is the update of the rows that differ, each old row paired with its new one: one request, whose rows the
// corpus takes at once, each with the columns that differ.
func updateMessage(cat *catalog.Catalog, tb *catalog.Table, changed [][2][]any, foreignKeysOff bool) ([]byte, error) {
	m := &message{}
	var first []map[string]any
	m.updates = []map[string]any{}
	for r, pair := range changed {
		var set []map[string]any
		for i, col := range tb.Columns {
			if same(pair[0][i], pair[1][i]) {
				continue
			}
			a, err := m.assign(col, pair[1][i])
			if err != nil {
				return nil, err
			}
			set = append(set, a)
		}
		key, err := m.keyParams(tb, pair[1])
		if err != nil {
			return nil, err
		}
		if r == 0 {
			first = set
		}
		m.updates = append(m.updates, map[string]any{"key": key, "set": set})
	}
	where, err := m.where(tb, changed[0][1])
	if err != nil {
		return nil, err
	}
	return m.request(cat, tb, "update", map[string]any{"set": first, "where": where}, foreignKeysOff)
}

// deleteMessage is the delete of rows: one request, whose rows the corpus takes at once.
func deleteMessage(cat *catalog.Catalog, tb *catalog.Table, rows [][]any, foreignKeysOff bool) ([]byte, error) {
	m := &message{}
	for _, row := range rows {
		key, err := m.keyParams(tb, row)
		if err != nil {
			return nil, err
		}
		m.keys = append(m.keys, key)
	}
	where, err := m.where(tb, rows[0])
	if err != nil {
		return nil, err
	}
	return m.request(cat, tb, "delete", map[string]any{"where": where}, foreignKeysOff)
}

func same(a, b any) bool {
	x, xb := a.([]byte)
	y, yb := b.([]byte)
	if xb && yb {
		return string(x) == string(y)
	}
	return a == b
}

// literal is the parameter of a value that its column's type does not describe.
func literal(text string) (json.RawMessage, error) {
	b, err := json.Marshal(map[string]string{"literal": text})
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	return b, nil
}

// literalValue is the SQLite value of a stored literal.
func literalValue(text string) (any, error) {
	body := text[2 : len(text)-1]
	switch text[0] {
	case 'n':
		if n, err := strconv.ParseInt(body, 10, 64); err == nil {
			return n, nil
		}
		f, err := strconv.ParseFloat(body, 64)
		if err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		return f, nil
	default:
		b, err := hex.DecodeString(body)
		if err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		if text[0] == 't' {
			return string(b), nil
		}
		if b == nil {
			b = []byte{}
		}
		return b, nil
	}
}

// wire is the runtime value model JSON of a SQLite value in a column of type t, as a request parameter. A value that its
// column's type or a cell cannot hold is a stored literal; a text that is not of the type stays as SQLite holds it, for
// the column's rule to refuse.
func wire(typ dbspec.Type, v any) (json.RawMessage, error) {
	var out any = v
	switch x := v.(type) {
	case int64:
		switch typ.Kind {
		case dbspec.TypeBool:
			out = x != 0
		case dbspec.TypeDecimal:
			out = decimalText(x, typ.Scale)
		case dbspec.TypeText, dbspec.TypeVarchar:
			out = strconv.FormatInt(x, 10)
		case dbspec.TypeBytes:
			return literal("n'" + strconv.FormatInt(x, 10) + "'")
		}
	case float64:
		if typ.Kind != dbspec.TypeF64 || math.IsInf(x, 0) {
			return literal("n'" + strconv.FormatFloat(x, 'g', -1, 64) + "'")
		}
	case []byte:
		if typ.Kind != dbspec.TypeBytes {
			return literal("x'" + hex.EncodeToString(x) + "'")
		}
		out = base64.StdEncoding.EncodeToString(x)
	case time.Time:
		out = x.UTC().Format("2006-01-02 15:04:05.000000")
	case string:
		if typ.Kind == dbspec.TypeBytes || !utf8.ValidString(x) || store.QuotedFault(x) != "" {
			return literal("t'" + hex.EncodeToString([]byte(x)) + "'")
		}
		out = temporalWire(typ, x)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	return b, nil
}
