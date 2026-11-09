// Package writepath is the write path of spec 40: it joins the IR executor (irexec), which computes a request's answer
// and post-image against a corpus, and the portable lock and the journal (store/journal), which serialize the
// operations and make a commit atomic and recoverable.
//
// Every operation — a read, a write, a rewrite — takes the corpus lock and recovers an interrupted
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
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store/journal"
	"github.com/polyspec/flowmark/packages/go/validate"
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

// Failure is the orm code and the message of the error, as a server answer names them.
func (e *Error) Failure() (code, msg string) { return e.Code, e.Msg }

// ResultCode is the SQLite result code of a lock wait that ended, SQLITE_BUSY, for the answer of a SQL statement.
func (e *Error) ResultCode() int {
	if e.Face == faceLockTimeout {
		return 5
	}
	return 0
}

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
	case errors.Is(err, fs.ErrPermission):
		// The process may not write the corpus: nothing can write it, as a read-only database file.
		return &Error{Code: codeReadOnly, Face: codeReadOnly, Msg: "attempt to write a readonly database: " + err.Error(), Err: err}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return &Error{Code: codeCanceled, Face: codeCanceled, Msg: err.Error(), Err: err}
	}
	return driverError("the corpus lock", err)
}

// Corpus is a corpus opened through the write path: its catalog and executor, its journal corpus, and the lock wait
// of its operations.
type Corpus struct {
	cat  *catalog.Catalog
	exec *irexec.Executor
	j    *journal.Corpus
	tree *journal.Tree
	// live keeps the read of the corpus between its reads, which read again only what changed (plan, Ruling: an
	// operation costs what it touches).
	live *validate.Live
	// cost counts the work of the corpus's operations (plan, Ruling: an operation costs what it touches).
	cost *cost.Counts
	root string
	wait time.Duration
	// readHook is the seam of the tests that lands a change between the read and the commit of an operation outside a
	// transaction; it is called after each read with the attempt's index. It is nil outside the tests.
	readHook func(attempt int)
	// published is the catalog the corpus last took, which a face reads without the lock.
	published atomic.Pointer[catalog.Catalog]
	// last is the corpus last read under the lock with the commit generation it was read at, which a read answers from
	// while another session holds the lock, or without the lock when nothing changed since.
	last atomic.Pointer[readState]
	// readMu serializes the reads of the corpus: under the lock, and without it while another holds it.
	readMu sync.Mutex
	// freeHook is the seam of the tests that lands a change between a read without the lock and its second check; it is
	// nil outside the tests.
	freeHook func()
	// sql is what package statement keeps of the SQL database of the corpus, under sqlMu: it is read without the lock.
	sql   io.Closer
	sqlMu sync.Mutex
}

// readState is a read of the corpus under the lock: the corpus and the commit generation it was read at, kept as one
// so a read without the lock never pairs one read's corpus with another's generation. fresh is false from the start of
// a read until it succeeds, so a read that failed leaves nothing a read without the lock answers from.
type readState struct {
	cur   *irexec.Corpus
	gen   uint64
	fresh bool
}

// committed returns the corpus last read under the lock, nil before the first.
func (c *Corpus) committed() *irexec.Corpus {
	if st := c.last.Load(); st != nil {
		return st.cur
	}
	return nil
}

// unchanged returns the corpus last read under the lock and true when nothing changed since that read (spec 40 Reads):
// no stage is present, the commit generation is the one it was read at, and no notification was queued. The stage is
// checked before the generation, so a commit that ended between the two checks is seen at its generation, and a commit
// whose renames land after them is answered before it, as a SQLite read that began before a commit is.
func (c *Corpus) unchanged() (*irexec.Corpus, bool, error) {
	st := c.last.Load()
	if st == nil || !st.fresh {
		return nil, false, nil
	}
	staged, err := c.j.Staged()
	if err != nil {
		return nil, false, driverError("reading the stages of the corpus", err)
	}
	if staged {
		return nil, false, nil
	}
	g, err := c.j.Generation()
	if err != nil {
		return nil, false, driverError("reading the commit generation of the corpus", err)
	}
	if g.N != st.gen || !c.tree.Quiet() {
		return nil, false, nil
	}
	return st.cur, true, nil
}

// readHeld reads the corpus while another holds the lock, held the refusal of its acquisition (spec 40 Reads). A
// session of this corpus holding it means no other process can commit, so the corpus last read is answered. Otherwise
// the corpus is read without the lock, and the read is kept when the commit generation is the same before and after it
// and no prepare record is present at either check: a commit renames its targets only between its prepare record and
// its generation, so a commit that renamed any target during the read is seen at one of the checks. The generation is
// read before the first check and after the second, so a prepare record that came and went between them is seen at its
// generation. A read that is not kept answers the corpus last read, and what it made of the tree is dropped, so the next
// read builds the corpus again from the tree, whose notifications and the generation's paths name what changed.
func (c *Corpus) readHeld(held error) (*irexec.Corpus, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	prev := c.last.Load()
	if prev == nil {
		return nil, lockError(held)
	}
	if c.j.Holding() {
		return prev.cur, nil
	}
	g, err := c.j.Generation()
	if err != nil {
		return nil, driverError("reading the commit generation of the corpus", err)
	}
	prepared, err := c.j.Prepared()
	if err != nil {
		return nil, driverError("reading the stages of the corpus", err)
	}
	if prepared {
		return prev.cur, nil
	}
	cur, rerr := c.readLocked()
	if c.freeHook != nil {
		c.freeHook()
	}
	prepared, err = c.j.Prepared()
	if err != nil {
		return nil, driverError("reading the stages of the corpus", err)
	}
	after, err := c.j.Generation()
	if err != nil {
		return nil, driverError("reading the commit generation of the corpus", err)
	}
	if !prepared && after.N == g.N {
		return cur, rerr
	}
	c.live.Reset()
	c.last.Store(&readState{cur: prev.cur, gen: prev.gen})
	return prev.cur, nil
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
	k := j.Cost()
	tree, err := j.Tree(k)
	if err != nil {
		return nil, driverError("watching the corpus", err)
	}
	c := &Corpus{cat: cat, exec: e, j: j, tree: tree, live: validate.NewLive(cat, k), cost: k, root: j.Root(), wait: wait}
	c.published.Store(cat)
	l, err := c.acquire(ctx, "open")
	if err != nil {
		return nil, errors.Join(err, c.Close())
	}
	// The first read fills the tree: it reads every corpus document once, and later operations read only the files
	// the notifications name.
	_, err = c.read()
	if err != nil {
		// The corpus is not opened, so its watch is closed before the lock is released: on macOS the watch holds a
		// descriptor for every folder and document, and releasing the lock opens the lock file, which a read that
		// failed for lack of descriptors would leave unreleased.
		err = errors.Join(err, c.Close())
	}
	if err = release(l, err); err != nil {
		return nil, errors.Join(err, c.Close())
	}
	return c, nil
}

// acquire takes the corpus lock for session and recovers an interrupted commit before anything reads the corpus
// (spec 40 Opening a corpus): a stage found under the lock is a commit of a holder that ended.
// A recovery that fails, or panics, releases the lock.
func (c *Corpus) acquire(ctx context.Context, session string) (*journal.Lock, error) {
	return c.acquireWait(ctx, session, c.wait)
}

// acquireWait is acquire with its wait.
func (c *Corpus) acquireWait(ctx context.Context, session string, wait time.Duration) (_ *journal.Lock, err error) {
	l, err := c.j.Acquire(ctx, session, wait)
	if err == nil {
		c.cost.Add(cost.LocksTaken, 1)
	}
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

// Cost returns the counts of the work of the corpus's operations.
func (c *Corpus) Cost() *cost.Counts { return c.cost }

// read reads the corpus as a request reads it, under the lock: the paths the tree dropped since the read before, and
// the paths of the commits since its generation, which the tree forgets first, whatever their notifications said; a
// generation no longer kept reads the whole corpus (spec 40 The commit generation).
func (c *Corpus) read() (*irexec.Corpus, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	return c.readLocked()
}

// readLocked is read, under readMu.
func (c *Corpus) readLocked() (*irexec.Corpus, error) {
	// The generation is read before the tree, under the lock, so no commit lands between the two.
	g, err := c.j.Generation()
	if err != nil {
		return nil, driverError("reading the commit generation of the corpus", err)
	}
	var prev *irexec.Corpus
	var at uint64
	if st := c.last.Load(); st != nil {
		prev, at = st.cur, st.gen
		if st.fresh {
			c.last.Store(&readState{cur: prev, gen: at})
		}
	}
	paths, kept := g.Since(at)
	// The schema statements are read when the corpus is read from nothing and after each commit that wrote them, of any
	// process, as the generation names it: a process takes the schema another committed, as a SQLite connection does.
	if prev == nil || !kept || slices.Contains(paths, catalog.StatementsFile) {
		if err := c.takeStatements(); err != nil {
			return nil, err
		}
	}
	c.tree.Forget(paths...)
	if err := c.tree.Refresh(); err != nil {
		c.live.Reset()
		return nil, driverError("reading the corpus", err)
	}
	changed, all := c.tree.Changes()
	s, err := c.live.Read(c.tree, changed, all || !kept)
	if err == nil {
		// A path whose watch could not be armed was not read: the read has no answer.
		err = c.tree.Fault()
	}
	if err != nil {
		// The next read reads the whole corpus, since this one kept no corpus to read on from.
		c.live.Reset()
		return nil, driverError("reading the corpus", err)
	}
	cur := irexec.FromSnapshot(c.cat, s, prev, c.cost)
	// The sequences' file is read with the corpus from nothing and after a commit that wrote it, whatever process made
	// that commit: the generation names it (spec 40 SQL statements).
	if prev == nil || prev.Catalog() != c.cat || all || !kept || slices.Contains(paths, catalog.SequencesFile) {
		data, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(catalog.SequencesFile)))
		if errors.Is(err, fs.ErrNotExist) {
			data, err = nil, nil
		}
		if err == nil {
			err = cur.SetSequenceFile(data)
		}
		if err != nil {
			c.live.Reset()
			return nil, driverError("reading the AUTOINCREMENT sequences of the corpus", err)
		}
	}
	c.last.Store(&readState{cur: cur, gen: g.N, fresh: true})
	return cur, nil
}

// takeStatements reads the schema statements of the corpus from the disk, under the lock, and takes the catalog they
// give when they are not those of its catalog.
func (c *Corpus) takeStatements() error {
	added, err := catalog.ReadStatements(c.root)
	if err != nil {
		return driverError("reading the schema statements of the corpus", err)
	}
	if slices.Equal(added, c.cat.Statements) {
		return nil
	}
	cat, diags := catalog.LoadStatements(c.root, added)
	if len(diags) > 0 {
		lines := make([]string, len(diags))
		for i, d := range diags {
			lines[i] = d.String()
		}
		return newError(codeConfig, "the schema statements another process committed leave a catalog that does not load: %s", strings.Join(lines, "; "))
	}
	return c.useCatalog(cat)
}

// useCatalog makes cat the catalog of the corpus, under the lock: its executor and its read, which reads the corpus
// from nothing next.
func (c *Corpus) useCatalog(cat *catalog.Catalog) error {
	e, err := irexec.New(cat)
	if err != nil {
		return err
	}
	c.cat, c.exec, c.live = cat, e, validate.NewLive(cat, c.cost)
	c.published.Store(cat)
	return nil
}

// Catalog returns the catalog the corpus last took: the catalog of its opening, or of the schema statements committed
// since.
func (c *Corpus) Catalog() *catalog.Catalog { return c.published.Load() }

// Generation returns the commit generation the corpus was last read at under the lock.
func (c *Corpus) Generation() uint64 {
	if st := c.last.Load(); st != nil {
		return st.gen
	}
	return 0
}

// Close closes the corpus: the watch of its tree. An operation of a closed corpus fails.
func (c *Corpus) Close() error {
	c.sqlMu.Lock()
	var heldErr error
	if c.sql != nil {
		heldErr = c.sql.Close()
		c.sql = nil
	}
	c.sqlMu.Unlock()
	if err := c.tree.Close(); err != nil {
		return driverError("closing the watch of the corpus", err)
	}
	if heldErr != nil {
		return driverError("closing the statement database of the corpus", heldErr)
	}
	return nil
}
