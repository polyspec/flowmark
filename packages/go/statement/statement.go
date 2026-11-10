// Package statement runs one SQL statement on a corpus (spec 40 SQL statements). The corpus is read into a SQLite
// database of the schema orm renders for its document set, the statement runs there in SQLite's dialect, and the rows it
// changed come back as writes of the executor, which holds every rule of the corpus. The database is the opening's own
// (database.go): its writer connection follows the session's transaction and commits after the corpus commit, its
// reader connections read what its last commit holds, and it is brought to the corpus whenever the corpus differs from
// it: the corpus stays the only store.
package statement

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/min-0-max/sqlite"
	sqlite3 "github.com/min-0-max/sqlite/lib"
	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/flowmark/packages/go/writepath"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/engine/ir"
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

// Run runs text with params in the session s: in its transaction, or as one transaction of its own. A closed connection
// ends what its statement is doing, as SQLite's interrupt does, so the statement runs in the session's life.
func Run(s *writepath.Session, text string, params []json.RawMessage) (_ *Answer, err error) {
	defer func() {
		// A lock wait that ended is SQLite's SQLITE_BUSY, whose name ends the message of a statement as its other results do.
		var locked *writepath.Error
		if errors.As(err, &locked) && locked.ResultCode() == sqlite3.SQLITE_BUSY {
			locked.Msg += resultCodeSuffix(sqlite3.SQLITE_BUSY)
		}
	}()
	ctx := s.Life()
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
		return readCommitted(ctx, s, text, args)
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
	var writes written
	var answer *Answer
	err = s.View(ctx, func(c *irexec.Corpus) error {
		d, _, err := writerFor(ctx, s, c)
		if err != nil {
			return err
		}
		answer, open, writes, err = execute(ctx, s, d, c, text, args)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := applyWrites(ctx, s, writes); err != nil {
		return nil, err
	}
	return answer, nil
}

// written is what a statement changed in its database: the writes of its difference, and the rows of sqlite_sequence
// after it, the sequence SQLite keeps for each AUTOINCREMENT table that has a row there.
type written struct {
	writes [][]byte
	held   map[string]int64
}

// applyWrites writes the difference of a statement to the session's transaction, which the writer connection follows:
// the database holds what the corpus then holds. The corpus takes the sequences SQLite keeps after the statement when
// they differ from its own: a key the database gave past the sequence of the corpus, as a key the statement inserted
// and deleted again, and an explicit write of sqlite_sequence, a lower value and a deleted row included (spec 40 SQL
// statements).
func applyWrites(ctx context.Context, s *writepath.Session, w written) error {
	t := following(s)
	if t == nil {
		return &Error{codeInternal, "no transaction of the writer connection follows the session"}
	}
	for _, msg := range w.writes {
		if _, err := s.ExecFace(ctx, msg); err != nil {
			return err
		}
	}
	differs := false
	if w.held != nil {
		err := s.View(ctx, func(c *irexec.Corpus) error {
			for _, tb := range c.Catalog().Tables {
				if !tb.Autoincrement {
					continue
				}
				n, held := c.Counter(tb.Name)
				if m, ok := w.held[tb.Name]; ok != held || m != n {
					differs = true
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if differs {
		if err := s.SetSequences(ctx, w.held); err != nil {
			return err
		}
	}
	if len(w.writes) == 0 && !differs {
		return nil
	}
	return s.View(ctx, func(c *irexec.Corpus) error {
		t.d.corpus = c
		return nil
	})
}

// runInTransaction runs a statement in the open transaction of the session, under a savepoint that the writer
// connection follows: a statement that fails, or whose difference a corpus rule refuses, leaves the transaction as it
// was. When the statement ended the SQLite transaction itself, as RAISE(ROLLBACK) does, the session's transaction ends
// with it.
func runInTransaction(ctx context.Context, s *writepath.Session, text string, args []any) (_ *Answer, err error) {
	if readsOnly(text) {
		var answer *Answer
		err := s.View(ctx, func(c *irexec.Corpus) error {
			d, _, err := writerFor(ctx, s, c)
			if err != nil {
				return err
			}
			var open *sql.Rows
			if answer, open, err = readBatch(ctx, d.w, text, args); err == nil && open != nil {
				s.SetCursor(&cursor{rows: open})
			}
			return err
		})
		return answer, err
	}
	// The writer follows the transaction before the savepoint, so it takes the savepoint too.
	var d *database
	if err := s.View(ctx, func(c *irexec.Corpus) (err error) {
		d, _, err = writerFor(ctx, s, c)
		return err
	}); err != nil {
		return nil, err
	}
	if err := s.Savepoint(savepoint); err != nil {
		return nil, err
	}
	var writes written
	var answer *Answer
	err = s.View(ctx, func(c *irexec.Corpus) error {
		var err error
		answer, _, writes, err = execute(ctx, s, d, c, text, args)
		return err
	})
	if err == nil {
		err = applyWrites(ctx, s, writes)
	}
	if err == nil {
		if err := s.Release(savepoint); err != nil {
			return nil, errors.Join(err, s.Rollback())
		}
		return answer, nil
	}
	if undo := s.RollbackTo(savepoint); undo != nil {
		// The statement ended the SQLite transaction, and the session's goes with it.
		s.SetEnded(true)
		return nil, errors.Join(err, s.Rollback())
	}
	if undo := s.Release(savepoint); undo != nil {
		return nil, errors.Join(err, undo, s.Rollback())
	}
	return nil, err
}

// readCommitted answers a read outside a transaction on a reader connection, from the corpus Committed returns: it takes
// no corpus lock when nothing changed since the last read under it, and its rows that go on hold none (spec 40 Reads).
func readCommitted(ctx context.Context, s *writepath.Session, text string, args []any) (*Answer, error) {
	c, err := s.Committed(ctx)
	if err != nil {
		return nil, err
	}
	r, err := readerFor(ctx, s, c)
	if err != nil {
		return nil, err
	}
	return readOn(ctx, s, r, text, args)
}

// readOn answers a read on the connection r; rows that go on keep it until they end.
func readOn(ctx context.Context, s *writepath.Session, r *reader, text string, args []any) (*Answer, error) {
	answer, open, err := readBatch(ctx, r.q, text, args)
	if err != nil {
		return nil, errors.Join(err, r.release())
	}
	if open == nil {
		return answer, r.release()
	}
	s.SetCursor(&cursor{rows: open, release: r.release})
	return answer, nil
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

// readDeferred answers a read of a deferred transaction on the reader connection its first read pinned, which reads the
// corpus of the database's commit it began at.
func readDeferred(ctx context.Context, s *writepath.Session, text string, args []any) (*Answer, error) {
	d := s.Deferred()
	if d.Pinned == nil {
		c, err := s.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		p, at, err := pin(ctx, s, c)
		if err != nil {
			return nil, err
		}
		d.Pinned, d.Reader = at, p
	}
	if p, ok := d.Reader.(*pinned); ok && p.r != nil {
		answer, open, err := readBatch(ctx, p.r.q, text, args)
		if err == nil && open != nil {
			s.SetCursor(&cursor{rows: open})
		}
		return answer, err
	}
	r, err := readerFor(ctx, s, d.Pinned)
	if err != nil {
		return nil, err
	}
	return readOn(ctx, s, r, text, args)
}

// endDeferred ends the deferred transaction of the session and gives its reader connection back.
func endDeferred(s *writepath.Session) error {
	d := s.Deferred()
	s.SetDeferred(nil)
	if d == nil || d.Reader == nil {
		return nil
	}
	return d.Reader.Close()
}

// upgrade turns a deferred transaction into one that holds the lock, at its first write. A wait that ends leaves it
// deferred, and so does a corpus committed since the transaction's first read, whose snapshot can no longer be written:
// SQLITE_BUSY and SQLITE_BUSY_SNAPSHOT, for the ROLLBACK that follows.
func upgrade(ctx context.Context, s *writepath.Session) error {
	d := s.Deferred()
	if err := s.Begin(ctx, s.QueryOnly()); err != nil {
		return err
	}
	var stale bool
	err := s.View(ctx, func(c *irexec.Corpus) error {
		if d.Pinned != nil && len(d.Pinned.Differs(c)) > 0 {
			stale = true
			return nil
		}
		_, _, err := writerFor(ctx, s, c)
		return err
	})
	if err != nil {
		return errors.Join(err, s.Rollback())
	}
	if stale {
		return errors.Join(staleSnapshot{}, s.Rollback())
	}
	for _, name := range d.Savepoints {
		if err := s.Savepoint(name); err != nil {
			return errors.Join(err, s.Rollback())
		}
	}
	return endDeferred(s)
}

// A statement's rows go in batches, so a result takes the memory of what its client reads, as on a SQLite connection.
const (
	batchRows  = 1000
	batchBytes = 1 << 20
)

// cursor is the rows of a statement that go on. Close frees what they hold, and after is what the statement still owes
// its session once they end; release gives back the connection they read on.
type cursor struct {
	rows    *sql.Rows
	after   func() error
	release func() error
}

// Close closes the rows, and it is also what the end of a session calls.
func (c *cursor) Close() error {
	err := c.rows.Close()
	if c.release != nil {
		err = errors.Join(err, c.release())
		c.release = nil
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

// emptyAnswer is the answer of rows with their columns and no row yet, and the number of the columns.
func emptyAnswer(rows *sql.Rows) (*Answer, int, error) {
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, 0, driverError(err)
	}
	a := &Answer{Columns: make([]Column, len(types)), Rows: [][]any{}}
	for i, t := range types {
		a.Columns[i] = Column{Name: t.Name(), Type: t.DatabaseTypeName()}
	}
	return a, len(types), nil
}

// readBatch runs a read and answers its first batch, with the rows when they go on.
func readBatch(ctx context.Context, db runner, text string, args []any) (*Answer, *sql.Rows, error) {
	rows, err := db.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, nil, driverError(err)
	}
	a, n, err := emptyAnswer(rows)
	if err != nil {
		return nil, nil, errors.Join(err, rows.Close())
	}
	more, err := nextBatch(rows, a, n)
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

// pragmaSetting reads a PRAGMA that sets a value, PRAGMA name = value, from its tokens: the name in lower case and the
// value as written, a negative number with its sign.
func pragmaSetting(toks []token) (name, value string, ok bool) {
	if len(toks) < 4 || toks[1].kind != tokenWord || !toks[2].is("=") {
		return "", "", false
	}
	name, rest := strings.ToLower(toks[1].text), toks[3:]
	sign := ""
	if len(rest) == 2 && rest[0].is("-") {
		sign, rest = "-", rest[1:]
	}
	if len(rest) != 1 || rest[0].kind == tokenOther && !digit(rest[0].text[0]) {
		return "", "", false
	}
	return name, sign + rest[0].text, true
}

// begin opens the session's transaction, which the writer connection follows from here on, so that SQLite judges at
// COMMIT what it defers to it.
func begin(ctx context.Context, s *writepath.Session) error {
	if err := s.Begin(ctx, s.QueryOnly()); err != nil {
		return err
	}
	s.SetEnded(false)
	err := s.View(ctx, func(c *irexec.Corpus) error {
		_, _, err := writerFor(ctx, s, c)
		return err
	})
	if err != nil {
		return errors.Join(err, s.Rollback())
	}
	return nil
}

// control runs a statement that controls the session as the session's own operation, and reports whether text is one:
// the transaction of a SQLite connection, and its query_only, which reads the corpus without writing it.
func control(ctx context.Context, s *writepath.Session, text string) (bool, error) {
	toks := statementTokens(text)
	if len(toks) == 0 {
		return false, nil
	}
	fields := make([]string, len(toks))
	for i, t := range toks {
		fields[i] = strings.ToUpper(t.text)
	}
	name := func(i int) string { return toks[i].text }
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
			return true, endDeferred(s)
		}
		return true, s.Commit()
	case "ROLLBACK":
		if s.Ended() {
			return true, nil
		}
		whole := len(fields) == 1 || (len(fields) == 2 && fields[1] == "TRANSACTION")
		if d := s.Deferred(); d != nil {
			if whole {
				return true, endDeferred(s)
			}
			i := slices.Index(d.Savepoints, name(len(fields)-1))
			if i < 0 {
				return true, &Error{codeDriver, "no such savepoint: " + name(len(fields)-1)}
			}
			d.Savepoints = d.Savepoints[:i+1]
			return true, nil
		}
		if whole {
			return true, s.Rollback()
		}
		return true, s.RollbackTo(name(len(fields) - 1))
	case "SAVEPOINT":
		if d := s.Deferred(); d != nil {
			d.Savepoints = append(d.Savepoints, name(1))
			return true, nil
		}
		return true, s.Savepoint(name(1))
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
		return true, s.Release(name(len(fields) - 1))
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
		if name, value, ok := pragmaSetting(toks); ok {
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

// arguments reads the bound values of a statement: a JSON number, string, boolean or null, or one of the forms that an
// answer writes for a value that JSON cannot hold (answerValue): {"bytes": base64} for a blob, {"text": base64} for a text
// that is not UTF-8 and {"float": "+Inf"} or {"float": "-Inf"} for an infinity.
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
			value, err := argumentForm(t)
			if err != nil {
				return nil, &Error{codeDriver, fmt.Sprintf("parameter %d: %v", i, err)}
			}
			out[i] = value
		case []any:
			return nil, &Error{codeDriver, fmt.Sprintf("parameter %d: an array is no value", i)}
		default:
			out[i] = v
		}
	}
	return out, nil
}

// argumentForm is the value of an object parameter, one form with one member.
func argumentForm(t map[string]any) (any, error) {
	const forms = `an object is {"bytes": base64}, {"text": base64} or {"float": "+Inf" | "-Inf"}`
	if len(t) != 1 {
		return nil, errors.New(forms)
	}
	for name, member := range t {
		text, ok := member.(string)
		if !ok {
			return nil, errors.New(forms)
		}
		switch name {
		case "bytes", "text":
			b, err := base64.StdEncoding.DecodeString(text)
			if err != nil {
				return nil, err
			}
			if name == "text" {
				return string(b), nil
			}
			return b, nil
		case "float":
			switch text {
			case "+Inf":
				return math.Inf(1), nil
			case "-Inf":
				return math.Inf(-1), nil
			}
		}
	}
	return nil, errors.New(forms)
}

// materialize returns an in-memory SQLite database that holds the schema and the rows of the corpus.
func materialize(ctx context.Context, c *irexec.Corpus) (*sql.DB, error) {
	db, err := sql.Open(sqlite.DriverName, memoryURI)
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	db.SetMaxOpenConns(1)
	if err := fill(ctx, db, c); err != nil {
		if cerr := db.Close(); cerr != nil {
			return nil, errors.Join(err, &Error{codeInternal, cerr.Error()})
		}
		return nil, err
	}
	return db, nil
}

// fill creates the schema of the corpus c in the empty database db and inserts its stored rows.
func fill(ctx context.Context, db queryer, c *irexec.Corpus) error { return fillWith(ctx, db, c, nil) }

// fillWith is fill whose commit publish runs, nil for a plain commit.
func fillWith(ctx context.Context, db queryer, c *irexec.Corpus, publish func(commit func() error) error) error {
	cat := c.Catalog()
	schema, err := catalog.SchemaStatements(cat.Documents, cat.Statements)
	if err != nil {
		return &Error{codeInternal, err.Error()}
	}
	return asFill(ctx, db, schema, func(tx *sql.Tx) error {
		for _, tb := range cat.Tables {
			if err := fillTable(ctx, tx, c, tb); err != nil {
				return err
			}
		}
		for _, tb := range cat.Tables {
			if err := setSequence(ctx, tx, c, tb); err != nil {
				return err
			}
		}
		return nil
	}, publish)
}

// setSequence gives the AUTOINCREMENT table tb of the database of tx the sequence of the corpus c, the row SQLite keeps
// in sqlite_sequence (irexec.Corpus.Counter), or no row there when the corpus holds none.
func setSequence(ctx context.Context, tx runner, c *irexec.Corpus, tb *catalog.Table) error {
	if !tb.Autoincrement {
		return nil
	}
	n, held := c.Counter(tb.Name)
	if !held {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sqlite_sequence WHERE name = ?", tb.Name); err != nil {
			return &Error{codeInternal, err.Error()}
		}
		return nil
	}
	res, err := tx.ExecContext(ctx, "UPDATE sqlite_sequence SET seq = ? WHERE name = ?", n, tb.Name)
	if err != nil {
		return &Error{codeInternal, err.Error()}
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return &Error{codeInternal, err.Error()}
	}
	if updated > 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO sqlite_sequence (name, seq) VALUES (?, ?)", tb.Name, n); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	return nil
}

// sequencesOf returns the rows of sqlite_sequence of db, the sequence SQLite keeps for each AUTOINCREMENT table of cat
// that has a row there, nil when db has no sqlite_sequence. The corpus holds one integer sequence of an AUTOINCREMENT
// table or none (irexec.Corpus.Counter), so a row of a name that is no AUTOINCREMENT table of cat, byte for byte as
// SQLite matches it, a second row of a table and a sequence that is no integer are CONSTRAINT: the statement that left
// them fails, and its transaction, or its savepoint, leaves the database as it was (spec 40 SQL statements).
func sequencesOf(ctx context.Context, db runner, cat *catalog.Catalog) (_ map[string]int64, err error) {
	if !slices.ContainsFunc(cat.Tables, func(tb *catalog.Table) bool { return tb.Autoincrement }) {
		// A database whose AUTOINCREMENT tables were dropped keeps sqlite_sequence, as SQLite keeps it.
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_schema WHERE name = 'sqlite_sequence'").Scan(&n); err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		if n == 0 {
			return nil, nil
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT name, seq FROM sqlite_sequence")
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	defer closed(&err, rows)
	out := map[string]int64{}
	for rows.Next() {
		var name, seq any
		if err := rows.Scan(&name, &seq); err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		table, text := name.(string)
		if tb, ok := cat.Table(table); !text || !ok || !tb.Autoincrement || tb.Name != table {
			return nil, &Error{codeConstraint, fmt.Sprintf("sqlite_sequence holds a row of %s, which is no AUTOINCREMENT table: the corpus keeps the sequence of an AUTOINCREMENT table alone", sqlValueText(name))}
		}
		n, integer := seq.(int64)
		if !integer {
			return nil, &Error{codeConstraint, fmt.Sprintf("the sequence of %s in sqlite_sequence is %s, which is no integer: the corpus keeps an integer sequence", table, sqlValueText(seq))}
		}
		if _, twice := out[table]; twice {
			return nil, &Error{codeConstraint, fmt.Sprintf("sqlite_sequence holds two rows of %s: the corpus keeps one sequence of a table", table)}
		}
		out[table] = n
	}
	if err := rows.Err(); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	return out, nil
}

// sqlValueText is the text of a value SQLite gave, as a message names it: a text quoted, NULL, a blob in hex.
func sqlValueText(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'"
	case []byte:
		return fmt.Sprintf("X'%X'", x)
	}
	return fmt.Sprint(v)
}

// sameSequence reports whether the corpora a and b hold the same sequence of the AUTOINCREMENT table, the same row of
// sqlite_sequence or none in both.
func sameSequence(a, b *irexec.Corpus, table string) bool {
	n, held := a.Counter(table)
	m, also := b.Counter(table)
	return held == also && n == m
}

// refresh brings the database db of the corpus before to the corpus c, a corpus under the same catalog: the rows of the
// tables that differ (irexec.Corpus.Changed) leave it by their keys and enter it as c stores them, and a table whose
// availability differs is filled again, as a fill writes them (asFill), so the database holds what a fill from c would
// (plan, Ruling: an operation costs what it touches). A row keyed by rowid keeps its stored rowid; the rowid of another
// row is the database's own.
func refresh(ctx context.Context, db queryer, before, c *irexec.Corpus) error {
	return refreshWith(ctx, db, before, c, nil)
}

// refreshWith is refresh whose commit publish runs, nil for a plain commit.
func refreshWith(ctx context.Context, db queryer, before, c *irexec.Corpus, publish func(commit func() error) error) error {
	changes, err := c.Changed(before)
	if err != nil {
		return err
	}
	// A table whose rows or whose sequence changed takes the sequence of c again: an insert of a stored row raises it.
	var sequenced []*catalog.Table
	for _, tb := range c.Catalog().Tables {
		if tb.Autoincrement && (slices.ContainsFunc(changes, func(ch irexec.TableChange) bool { return ch.Table == tb.Name }) ||
			!sameSequence(before, c, tb.Name)) {
			sequenced = append(sequenced, tb)
		}
	}
	if len(changes) == 0 && len(sequenced) == 0 {
		return nil
	}
	return asFill(ctx, db, nil, func(tx *sql.Tx) error {
		for _, ch := range changes {
			tb, _ := c.Catalog().Table(ch.Table)
			if err := refreshTable(ctx, tx, c, tb, ch); err != nil {
				return err
			}
		}
		for _, tb := range sequenced {
			if err := setSequence(ctx, tx, c, tb); err != nil {
				return err
			}
		}
		return nil
	}, publish)
}

// asFill runs the statements schema and then rows in one transaction of db under the conditions of a fill: the foreign
// keys off, the triggers that stand dropped and the checks ignored, since a stored row has had its audit and its checks
// and a statement can have stored it past them, as ignore_check_constraints lets it. The triggers stand again and the
// foreign keys are on when it commits, so a statement meets them as a SQLite connection of orm does. publish runs the
// commit, nil for a plain one.
func asFill(ctx context.Context, db queryer, schema []string, rows func(*sql.Tx) error, publish func(commit func() error) error) (err error) {
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return &Error{codeInternal, err.Error()}
	}
	defer func() {
		// The rollback after the commit finds the transaction done.
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			err = errors.Join(err, &Error{codeInternal, rerr.Error()})
		}
	}()
	for _, text := range schema {
		if _, err := tx.ExecContext(ctx, text); err != nil {
			return &Error{codeInternal, fmt.Sprintf("the schema statement %s: %v", text, err)}
		}
	}
	triggers, err := standingTriggers(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA ignore_check_constraints = ON"); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	if err := rows(tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA ignore_check_constraints = OFF"); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	for _, text := range triggers {
		if _, err := tx.ExecContext(ctx, text); err != nil {
			return &Error{codeInternal, fmt.Sprintf("the schema statement %s: %v", text, err)}
		}
	}
	if publish == nil {
		publish = func(commit func() error) error { return commit() }
	}
	if err := publish(tx.Commit); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	return nil
}

// refreshTable takes the change ch of the table tb into the database of tx.
func refreshTable(ctx context.Context, tx runner, c *irexec.Corpus, tb *catalog.Table, ch irexec.TableChange) (err error) {
	if ch.Whole {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+quote(tb.Name)); err != nil {
			return &Error{codeInternal, err.Error()}
		}
		return fillTable(ctx, tx, c, tb)
	}
	keys := keyColumns(tb)
	where := make([]string, len(keys))
	at := make([]int, len(keys))
	for i, k := range keys {
		where[i] = quote(k) + " = ?"
		at[i] = slices.IndexFunc(tb.Columns, func(col dbspec.Column) bool { return col.Name == k })
	}
	del, err := tx.PrepareContext(ctx, "DELETE FROM "+quote(tb.Name)+" WHERE "+strings.Join(where, " AND "))
	if err != nil {
		return &Error{codeInternal, err.Error()}
	}
	defer closed(&err, del)
	for _, row := range ch.Gone {
		args := make([]any, len(keys))
		for i, j := range at {
			if args[i], err = stored(tb.Columns[j].Type, row[j]); err != nil {
				return err
			}
		}
		if _, err := del.ExecContext(ctx, args...); err != nil {
			return &Error{codeInternal, fmt.Sprintf("a row of %s: %v", tb.Name, err)}
		}
	}
	return insertRows(ctx, tx, c, tb, ch.Rows)
}

// fillTable inserts the stored rows of the table tb.
func fillTable(ctx context.Context, tx runner, c *irexec.Corpus, tb *catalog.Table) (err error) {
	rows, err := c.StoredRows(tb.Name)
	if err != nil {
		return err
	}
	return insertRows(ctx, tx, c, tb, rows)
}

// insertRows inserts the stored rows rows of the table tb.
func insertRows(ctx context.Context, tx runner, c *irexec.Corpus, tb *catalog.Table, rows [][]any) (err error) {
	if len(rows) == 0 {
		return nil
	}
	insert, err := tx.PrepareContext(ctx, insertText(tb))
	if err != nil {
		return &Error{codeInternal, err.Error()}
	}
	defer closed(&err, insert)
	for _, row := range rows {
		args, err := storedArgs(tb, row)
		if err != nil {
			return err
		}
		if _, err := insert.ExecContext(ctx, args...); err != nil {
			return &Error{codeInternal, fmt.Sprintf("a row of %s: %v", tb.Name, err)}
		}
		c.Cost().Add(cost.DatabaseRowsWritten, 1)
	}
	return nil
}

// storedArgs are the arguments that write the stored row row of tb, each value in the form its column type stores.
func storedArgs(tb *catalog.Table, row []any) ([]any, error) {
	args := make([]any, len(row))
	for i, v := range row {
		var err error
		if args[i], err = stored(tb.Columns[i].Type, v); err != nil {
			return nil, err
		}
	}
	return args, nil
}

// closed closes c when its function returns and adds the failure of the close to that function's error *err.
func closed(err *error, c io.Closer) {
	*err = errors.Join(*err, closeError(c))
}

// closeError closes c and returns the failure of the close as an internal error, nil when it closed.
func closeError(c io.Closer) error {
	if err := c.Close(); err != nil {
		return &Error{codeInternal, err.Error()}
	}
	return nil
}

// standingTriggers drops the triggers of the schema and returns the statements that create them.
func standingTriggers(ctx context.Context, tx runner) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT name, sql FROM sqlite_master WHERE type = 'trigger'")
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	out := map[string]string{}
	for rows.Next() {
		var name, text string
		if err := rows.Scan(&name, &text); err != nil {
			return nil, errors.Join(&Error{codeInternal, err.Error()}, closeError(rows))
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

// columnNames are the names of the columns of tb in a statement: each quoted, and the row id column as SQLite names it.
func columnNames(tb *catalog.Table) []string {
	names := make([]string, len(tb.Columns))
	for i, col := range tb.Columns {
		names[i] = quote(col.Name)
		if col.Name == catalog.RowIDColumn {
			names[i] = catalog.RowIDColumn
		}
	}
	return names
}

func insertText(tb *catalog.Table) string {
	names := columnNames(tb)
	marks := make([]string, len(tb.Columns))
	for i := range marks {
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
		case dbspec.TypeUUID:
			return uuidBytes(x)
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

// fullDiffs is the seam of the tests that forms the difference of each write statement of a session from every row of
// the database as well, before and after it, and gives both differences to compare (TestCapture); it is empty outside
// the tests. It is kept by session, so tests of different sessions run in parallel.
var fullDiffs sync.Map // *writepath.Session → fullDiff

// fullDiff gives the difference of a write statement formed from the rows it changed and from every row.
type fullDiff func(text string, captured, full [][]byte)

// fullDiffOf is the full difference seam of s, nil when none is installed.
func fullDiffOf(s *writepath.Session) fullDiff {
	f, _ := fullDiffs.Load(s)
	if check, ok := f.(fullDiff); ok {
		return check
	}
	return nil
}

// execute runs the statement on the writer connection of d, and returns its answer and the request messages of the rows
// it changed: the difference of the rows the pre-update hook reports it changed, before and after it, so a statement
// reads the rows it changed and not every row (spec 40 SQL statements).
func execute(ctx context.Context, s *writepath.Session, d *database, c *irexec.Corpus, text string, args []any) (_ *Answer, _ *sql.Rows, _ written, err error) {
	db := d.w
	foreignKeysOff := s.ForeignKeysOff()
	check := fullDiffOf(s)
	if readsOnly(text) {
		answer, open, err := readBatch(ctx, db, text, args)
		return answer, open, written{}, err
	}
	var full map[string]tableRows
	if check != nil {
		if full, err = allRows(ctx, db, c); err != nil {
			return nil, nil, written{}, err
		}
	}
	if err := d.cap.prepare(ctx, db); err != nil {
		return nil, nil, written{}, err
	}
	d.cap.start()
	defer d.cap.stop()
	answer, err := queryLargest(ctx, db, d.cap, c, text, args)
	if err != nil {
		return nil, nil, written{}, err
	}
	changed, err := d.cap.changes()
	if err != nil {
		return nil, nil, written{}, err
	}
	before, after, err := changedTables(ctx, db, c, changed)
	if err != nil {
		return nil, nil, written{}, err
	}
	writes, err := diff(c, before, after, foreignKeysOff)
	if err != nil {
		return nil, nil, written{}, err
	}
	if check != nil {
		now, err := allRows(ctx, db, c)
		if err != nil {
			return nil, nil, written{}, err
		}
		reference, err := diff(c, full, now, foreignKeysOff)
		if err != nil {
			return nil, nil, written{}, err
		}
		check(text, writes, reference)
	}
	held, err := sequencesOf(ctx, db, c.Catalog())
	if err != nil {
		return nil, nil, written{}, err
	}
	return answer, nil, written{writes: writes, held: held}, nil
}

// readsOnly reports whether the statement cannot change a row, by its first word; a statement of WITH is known by the
// word its common table expressions lead to. The words are read as SQLite's tokenizer reads them, so a comment before
// them is whitespace.
func readsOnly(text string) bool {
	toks := statementTokens(text)
	if len(toks) == 0 {
		return true
	}
	first := firstKeyword(toks)
	if first == "WITH" {
		first = afterCommonTables(toks[1:])
	}
	switch first {
	case "SELECT", "VALUES", "EXPLAIN", "PRAGMA":
		return true
	}
	return false
}

// afterCommonTables is the first main verb of a WITH statement outside its parentheses.
func afterCommonTables(toks []token) string {
	depth := 0
	for _, t := range toks {
		switch {
		case t.is("("):
			depth++
		case t.is(")"):
			depth--
		case depth == 0:
			switch word := t.keyword(); word {
			case "SELECT", "VALUES", "INSERT", "UPDATE", "DELETE", "REPLACE":
				return word
			}
		}
	}
	return ""
}

// query runs the statement and reads its result.
func query(ctx context.Context, db runner, text string, args []any) (_ *Answer, err error) {
	rows, err := db.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, driverError(err)
	}
	defer closed(&err, rows)
	a, n, err := emptyAnswer(rows)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		row, err := scan(rows, n)
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
		return realValue(t)
	case string:
		// JSON text is UTF-8, and a SQLite text holds whatever bytes it was given.
		if !utf8.ValidString(t) {
			return map[string]string{"text": base64.StdEncoding.EncodeToString([]byte(t))}
		}
	}
	return v
}

// realValue is a finite REAL of an answer. JSON has one number type, and a REAL of a whole value written as Go writes a
// float64 reads as an INTEGER, so an answer writes it with a fraction or an exponent (spec 40 "Values").
type realValue float64

// MarshalJSON writes the shortest text that reads back as r, with .0 when that text has neither a fraction nor an
// exponent: 2.0 is 2.0, -0.0 is -0.0, 1e21 is 1e+21 and 2.5 is 2.5. A value JSON cannot hold is an error, as for a float64.
func (r realValue) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(float64(r))
	if err != nil {
		return nil, err
	}
	return realForm(b), nil
}

// realForm is the shortest text of a finite float, with .0 appended when it has neither a fraction nor an exponent, so
// that it reads back as a REAL and never as an INTEGER.
func realForm(text []byte) []byte {
	if bytes.ContainsAny(text, ".eE") {
		return text
	}
	return append(text, ".0"...)
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

// tableRows are the rows of a table by their key, and the keys in the order the database read them.
type tableRows struct {
	byKey map[string][]any
	keys  []string
}

// allRows reads the rows of every table: the difference a test forms from every row (fullDiff).
func allRows(ctx context.Context, db runner, c *irexec.Corpus) (map[string]tableRows, error) {
	out := map[string]tableRows{}
	for _, tb := range c.Catalog().Tables {
		rows, err := rowsOf(ctx, db, tb, c.Cost())
		if err != nil {
			return nil, err
		}
		out[tb.Name] = keyed(tb, rows)
	}
	return out, nil
}

// rowsOf reads the rows of tb from db: of a table with a rowid in the order of their rowids, so the rows that an insert
// leaves to the corpus to place take their positions in the order they were inserted, and of a table without a rowid
// in the order of its primary key.
func rowsOf(ctx context.Context, db runner, tb *catalog.Table, k *cost.Counts) ([][]any, error) {
	text := "SELECT " + readColumns(tb) + " FROM " + quote(tb.Name)
	without, err := column[bool](ctx, db, "SELECT wr FROM pragma_table_list WHERE schema = 'main' AND name = ?", nil, tb.Name)
	if err != nil {
		return nil, err
	}
	if len(without) == 1 && without[0] {
		key, err := primaryKeyOf(ctx, db, tb.Name)
		if err != nil {
			return nil, err
		}
		text += key.order("", quoted(key.names))
	} else {
		name, _, err := rowidName(ctx, db, tb.Name)
		if err != nil {
			return nil, err
		}
		if name == "" {
			return nil, unnamedRowid(tb.Name)
		}
		text += " ORDER BY " + name
	}
	return readRows(ctx, db, tb, text, nil, k)
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
// are in the difference already. A row gone whose key equals the key of a row added without regard to case, which the
// corpus compares keys by (spec 20 Keys), is deleted before the inserts, as the database already holds the new key in
// its place: SQLite judged the foreign keys of the statement, so that delete is not judged by them again.
func diff(c *irexec.Corpus, before, after map[string]tableRows, foreignKeysOff bool) ([][]byte, error) {
	cat := c.Catalog()
	var displaced, inserts, changes, deletes [][]byte
	order := fillOrder(cat)
	moved := map[string]map[string]bool{}
	for _, tb := range order {
		folded := map[string]bool{}
		for _, key := range after[tb.Name].keys {
			if _, held := before[tb.Name].byKey[key]; !held {
				folded[foldKey(key)] = true
			}
		}
		var gone [][]any
		for _, key := range before[tb.Name].keys {
			if _, held := after[tb.Name].byKey[key]; !held && folded[foldKey(key)] {
				gone = append(gone, before[tb.Name].byKey[key])
				if moved[tb.Name] == nil {
					moved[tb.Name] = map[string]bool{}
				}
				moved[tb.Name][key] = true
			}
		}
		if len(gone) > 0 {
			msg, err := deleteMessage(cat, tb, gone, true)
			if err != nil {
				return nil, err
			}
			displaced = append(displaced, msg)
		}
	}
	for _, tb := range order {
		var added [][]any
		var changed [][2][]any
		for _, key := range after[tb.Name].keys {
			row := after[tb.Name].byKey[key]
			old, held := before[tb.Name].byKey[key]
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
		for _, key := range before[tb.Name].keys {
			row := before[tb.Name].byKey[key]
			if _, held := after[tb.Name].byKey[key]; !held && !moved[tb.Name][key] {
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
	return slices.Concat(displaced, inserts, changes, deletes), nil
}

// foldKey is the key k without regard to the case of A–Z, as the corpus compares keys.
func foldKey(k string) string {
	return strings.Map(func(r rune) rune {
		if 'A' <= r && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, k)
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

func (m *message) param(tb *catalog.Table, col dbspec.Column, v any) (map[string]any, error) {
	i, err := m.index(tb, col, v)
	if err != nil {
		return nil, err
	}
	return map[string]any{"p": i}, nil
}

// index adds the value v of the column col of tb to the parameters of m and returns its index.
func (m *message) index(tb *catalog.Table, col dbspec.Column, v any) (int, error) {
	raw, err := wire(tb, col, v)
	if err != nil {
		return 0, err
	}
	m.params = append(m.params, raw)
	return len(m.params) - 1, nil
}

func (m *message) assign(tb *catalog.Table, col dbspec.Column, v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{"column": col.Name, "null": true}, nil
	}
	a, err := m.param(tb, col, v)
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
		pred, err := m.param(tb, tb.Columns[i], row[i])
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
			p, err := m.index(tb, col, row[i])
			if err != nil {
				return nil, err
			}
			if r == 0 {
				set[i] = map[string]any{"p": p, "column": col.Name}
				load = load || col.Identity
				continue
			}
			params = append(params, p)
		}
		if r > 0 {
			more = append(more, params)
		}
	}
	set = slices.DeleteFunc(set, func(a map[string]any) bool { return a == nil })
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
		p, err := m.index(tb, tb.Columns[i], row[i])
		if err != nil {
			return nil, err
		}
		out = append(out, p)
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
			a, err := m.assign(tb, col, pair[1][i])
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

// wire is the runtime value model JSON of a SQLite value in the column col of tb, as a request parameter. A value that the
// column's type or a cell cannot hold is a stored literal, so the corpus read again gives each value the storage class
// SQLite gave it (spec 10 "Stored literals"): the column's affinity decides whether the text of an INTEGER reads back as
// that INTEGER. A key has no literal, so a key value that needs one is CONSTRAINT. A text of a text, date or time type
// stays as SQLite holds it, for the column's rule to refuse one that is not of the type.
func wire(tb *catalog.Table, col dbspec.Column, v any) (json.RawMessage, error) {
	typ := col.Type
	stored := func(text string) (json.RawMessage, error) {
		if slices.Contains(keyColumns(tb), col.Name) {
			return nil, &Error{codeConstraint, fmt.Sprintf("the key %s of %s holds %s, a value no key cell holds: a key has no stored literal", col.Name, tb.Name, text)}
		}
		return literal(text)
	}
	out := v
	switch x := v.(type) {
	case int64:
		switch typ.Kind {
		case dbspec.TypeBool:
			// SQLite's boolean is the INTEGER 0 or 1; another INTEGER of the column is kept as itself.
			if x != 0 && x != 1 {
				return stored("n'" + strconv.FormatInt(x, 10) + "'")
			}
			out = x != 0
		case dbspec.TypeDecimal:
			out = decimalText(x, typ.Scale)
		case dbspec.TypeText, dbspec.TypeVarchar:
			// A column of NUMERIC affinity reads the text of the INTEGER's digits back as the INTEGER; one of BLOB
			// affinity keeps it a TEXT, so there it is a literal.
			if tb.AffinityOf(col) == catalog.AffinityBlob {
				return stored("n'" + strconv.FormatInt(x, 10) + "'")
			}
			out = strconv.FormatInt(x, 10)
		case dbspec.TypeBytes:
			return stored("n'" + strconv.FormatInt(x, 10) + "'")
		}
	case float64:
		if typ.Kind != dbspec.TypeF64 || math.IsInf(x, 0) {
			text := []byte(strconv.FormatFloat(x, 'g', -1, 64))
			if !math.IsInf(x, 0) {
				// A literal of a whole REAL without a fraction would read back as an INTEGER (literalValue).
				text = realForm(text)
			}
			return stored("n'" + string(text) + "'")
		}
	case []byte:
		switch {
		case typ.Kind == dbspec.TypeUUID && len(x) == 16:
			out = uuidText(x)
		case typ.Kind != dbspec.TypeBytes:
			return stored("x'" + hex.EncodeToString(x) + "'")
		default:
			out = base64.StdEncoding.EncodeToString(x)
		}
	case time.Time:
		out = x.UTC().Format("2006-01-02 15:04:05.000000")
	case string:
		if !textCell(typ) || !utf8.ValidString(x) || store.QuotedFault(x) != "" {
			return stored("t'" + hex.EncodeToString([]byte(x)) + "'")
		}
		out = temporalWire(typ, x)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	return b, nil
}

// textCell reports whether a cell of type t holds a TEXT as its text: a column of a text, date or time type, whose values
// SQLite stores as TEXT. A TEXT of a column of a number, boolean, bytes or uuid type is one its affinity kept, such as
// 'abc' in an INTEGER column, and is a stored literal: as the column's value it would be read back as a number, bytes or
// the 16 bytes of a uuid, or not at all.
func textCell(t dbspec.Type) bool {
	switch t.Kind {
	case dbspec.TypeI16, dbspec.TypeI32, dbspec.TypeI64, dbspec.TypeF64, dbspec.TypeDecimal, dbspec.TypeBool, dbspec.TypeBytes, dbspec.TypeUUID:
		return false
	}
	return true
}

// uuidText is the lower-case canonical text of the 16 bytes b of a uuid, in their order (orm DIA-27): the cell of a uuid
// column in its document.
func uuidText(b []byte) string {
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// uuidBytes is the 16 bytes of the canonical text u of a uuid cell, which SQLite holds in a uuid column (orm DIA-27). The
// corpus refuses a uuid cell that is not canonical text, so another text is INTERNAL.
func uuidBytes(u string) ([]byte, error) {
	b, err := hex.DecodeString(strings.ReplaceAll(u, "-", ""))
	if err != nil || len(b) != 16 || uuidText(b) != u {
		return nil, &Error{codeInternal, fmt.Sprintf("the uuid cell %q is no canonical uuid text", u)}
	}
	return b, nil
}

// column reads the one column of the rows of text.
func column[T any](ctx context.Context, db runner, text string, k *cost.Counts, args ...any) ([]T, error) {
	return queryRows(ctx, db, text, args, k, func(rows *sql.Rows) (T, error) {
		var v T
		err := rows.Scan(&v)
		return v, err
	})
}

// queryRows runs the query text with args and returns what read takes from each of its rows, each counted as a row read
// from the database.
func queryRows[T any](ctx context.Context, db runner, text string, args []any, k *cost.Counts, read func(*sql.Rows) (T, error)) (_ []T, err error) {
	rows, err := db.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	defer closed(&err, rows)
	var out []T
	for rows.Next() {
		v, err := read(rows)
		if err != nil {
			return nil, &Error{codeInternal, err.Error()}
		}
		k.Add(cost.DatabaseRowsRead, 1)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, &Error{codeInternal, err.Error()}
	}
	return out, nil
}
