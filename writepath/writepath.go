// Package writepath is the write path of spec 40: it joins the IR executor (irexec), which computes a request's answer
// and post-image against a corpus, and the portable lock and the journal (store/journal), which serialize the
// operations and make a commit atomic and recoverable.
//
// Every operation — a read, a write, a history repair, a rewrite — takes the corpus lock and recovers an interrupted
// commit before it reads. A request outside a transaction is one transaction under the lock; a transaction holds the
// lock from Begin to Commit or Rollback, and its requests read its post-image. A commit checks that its targets are
// unchanged since they were read (step 1), runs the constraint checks on the corpus re-read under the lock with the
// post-image applied (step 2), and writes through the journal (steps 3 to 8). The events of the lock and the journal
// reach the observer of the journal corpus.
package writepath

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/irexec"
	"github.com/polyspec/flowmark/store/journal"
	"github.com/polyspec/flowmark/validate"
)

// The orm codes (orm errors.yaml) and the face names (spec 40 Failure vocabulary) of the write path's own failures.
// The executor's failures are *irexec.Error with their own codes.
const (
	codeCanceled      = "CANCELED"
	codeWriteConflict = "WRITE_CONFLICT"
	codeConfig        = "CONFIG"
	codeReadOnly      = "READ_ONLY"
	codeConstraint    = "CONSTRAINT"
	codeDriver        = "DRIVER"
	codeInternal      = "INTERNAL"

	faceLockTimeout    = "LOCK_TIMEOUT"
	faceCorpusConflict = "CORPUS_CONFLICT"
)

// Error is a failure of the write path: Code is its orm code and Face its name on the CLI, MCP and sidecar faces (spec
// 40 Failure vocabulary), which differ only for LOCK_TIMEOUT (CANCELED) and CORPUS_CONFLICT (WRITE_CONFLICT). A conflict
// names each file and line in Conflicts; the rewrite refusal carries the file's diagnostic; Err is the cause — a
// *journal.LockTimeout, a *journal.RecoveryRefusal, a storage error — when there is one.
type Error struct {
	Code       string
	Face       string
	Msg        string
	Conflicts  []Conflict
	Diagnostic *catalog.Diagnostic
	Err        error
}

// Conflict is a file that changed since a transaction read it, and the first line at which it changed.
type Conflict struct {
	File string
	Line int
}

func (e *Error) Error() string {
	if e.Face != e.Code {
		return e.Code + " (" + e.Face + "): " + e.Msg
	}
	return e.Code + ": " + e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

func newError(code, format string, a ...any) *Error {
	return &Error{Code: code, Face: code, Msg: fmt.Sprintf(format, a...)}
}

// driverError is a failure of the storage under the engine: a journal or file system error, or a recovery refusal.
// It is DRIVER, orm's code of a database error no other code names (as SQLite's SQLITE_CORRUPT or SQLITE_IOERR).
func driverError(what string, err error) *Error {
	return &Error{Code: codeDriver, Face: codeDriver, Msg: what + ": " + err.Error(), Err: err}
}

// lockError is the failure of a lock acquisition: a wait that ended is CANCELED — LOCK_TIMEOUT on the faces when it
// ended at its deadline or at an ended holder, CANCELED when its context (a closed connection) ended it.
func lockError(err error) *Error {
	var timeout *journal.LockTimeout
	switch {
	case errors.As(err, &timeout):
		return &Error{Code: codeCanceled, Face: faceLockTimeout, Msg: err.Error(), Err: err}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return &Error{Code: codeCanceled, Face: codeCanceled, Msg: err.Error(), Err: err}
	}
	return driverError("the corpus lock", err)
}

// Corpus is a corpus opened through the write path: its catalog and executor, its journal corpus, and the lock wait
// of its operations.
type Corpus struct {
	cat   *catalog.Catalog
	exec  *irexec.Executor
	j     *journal.Corpus
	tree  *journal.Tree
	memo  *validate.Memo
	root  string
	wait  time.Duration
	clock func() time.Time
	// readHook is the seam of the tests that lands a change between the read and the commit of an operation outside a
	// transaction; it is called after each read with the attempt's index. It is nil outside the tests.
	readHook func(attempt int)
}

// Open opens the corpus of the journal corpus j with the catalog cat: it runs recovery first, under the lock (spec 40
// Opening a corpus), and releases the lock. wait is the lock wait of every operation of the corpus (the faces'
// --lock-wait-ms, the server's lock_wait_ms); 0 refuses at once. ctx ends a wait of the opening.
func Open(ctx context.Context, cat *catalog.Catalog, j *journal.Corpus, wait time.Duration) (*Corpus, error) {
	if wait < 0 {
		return nil, newError(codeConfig, "the lock wait %s is negative; give 0 to refuse at once or a positive wait", wait)
	}
	e, err := irexec.New(cat)
	if err != nil {
		return nil, err
	}
	tree, err := j.Tree()
	if err != nil {
		return nil, driverError("watching the corpus", err)
	}
	c := &Corpus{cat: cat, exec: e, j: j, tree: tree, memo: validate.NewMemo(), root: j.Root(), wait: wait, clock: engineClock}
	l, err := c.acquire(ctx, "open")
	if err != nil {
		return nil, errors.Join(err, c.Close())
	}
	// The first read fills the tree: it reads every corpus document once, and later operations read only the files
	// the notifications name.
	_, err = c.read()
	if err = release(l, err); err != nil {
		return nil, errors.Join(err, c.Close())
	}
	return c, nil
}

// engineClock is the engine's UTC clock truncated to microseconds (spec 40 The clock): the time of an engine-assigned
// value outside an orm request, such as a repaired history row's at.
func engineClock() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// acquire takes the corpus lock for session and recovers an interrupted commit before anything reads the corpus
// (spec 40 Opening a corpus): a stage found under the lock is a commit of a holder that ended.
// A recovery that fails, or panics, releases the lock.
func (c *Corpus) acquire(ctx context.Context, session string) (_ *journal.Lock, err error) {
	l, err := c.j.Acquire(ctx, session, c.wait)
	if err != nil {
		e := lockError(err)
		if errors.Is(context.Cause(ctx), errSessionClosed) {
			e.Msg = "the session was closed while its request waited for the lock: " + e.Msg
		}
		return nil, e
	}
	recovered := false
	defer func() {
		if !recovered {
			err = release(l, err)
		}
	}()
	if err := c.j.Recover(l); err != nil {
		return nil, driverError("recovering the corpus", err)
	}
	recovered = true
	return l, nil
}

// release releases the lock l and joins its failure to err.
func release(l *journal.Lock, err error) error {
	return releaseAfter(l, false, err)
}

// releaseAfter releases the lock l after an operation that wrote the corpus or not, and joins its failure to err. A
// release that fails after a written commit says that the commit is written, so it is never taken for a failed one.
func releaseAfter(l *journal.Lock, written bool, err error) error {
	rerr := l.Release()
	switch {
	case rerr == nil:
		return err
	case written && err == nil:
		return driverError("the commit is written; releasing the corpus lock after it failed, so remove the lock with flowmark unlock once this process has ended", rerr)
	}
	return errors.Join(err, driverError("releasing the corpus lock", rerr))
}

// read reads the corpus as a request reads it.
func (c *Corpus) read() (*irexec.Corpus, error) {
	if err := c.tree.Refresh(); err != nil {
		return nil, driverError("reading the corpus", err)
	}
	cur, err := irexec.Open(c.cat, c.tree, c.memo)
	if err == nil {
		// A path whose watch could not be armed was not read: the read has no answer.
		err = c.tree.Fault()
	}
	if err != nil {
		return nil, driverError("reading the corpus", err)
	}
	return cur, nil
}

// Close closes the corpus: the watch of its tree. An operation of a closed corpus fails.
func (c *Corpus) Close() error {
	if err := c.tree.Close(); err != nil {
		return driverError("closing the watch of the corpus", err)
	}
	return nil
}
