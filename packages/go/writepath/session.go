package writepath

import (
	"bytes"
	"context"
	"errors"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store/journal"
)

// Session is one session of a corpus: a server connection, or one command of the CLI, MCP or sidecar. Its requests
// run one at a time; Close may be called from another goroutine (a closed connection, an idle transaction), and waits
// for a request in progress, whose lock wait ends with its context.
type Session struct {
	c      *Corpus
	name   string
	mu     sync.Mutex
	tx     *tx
	closed bool
	// queryOnly is the query_only of a SQL connection: a transaction the session's statements open is read-only.
	queryOnly atomic.Bool
	// foreignKeysOff is the foreign_keys of a SQL connection, read before each of the session's statements.
	foreignKeysOff atomic.Bool
	// cursor is the rows of the session's last statement that go on.
	cursor atomic.Pointer[io.Closer]
	// ended is a transaction a statement ended, as RAISE(ROLLBACK) ends one.
	ended atomic.Bool
	// settings are the other PRAGMA settings of a SQL connection, which only its own statements see.
	settings map[string]string
	// busyMS is the busy_timeout of a SQL connection plus one, the wait of its lock; 0 is the corpus's wait.
	busyMS atomic.Int64
	// kept is the lock that the session holds across its transactions, under locking_mode EXCLUSIVE.
	kept atomic.Pointer[journal.Lock]
	// exclusive is the locking_mode EXCLUSIVE of a SQL connection.
	exclusive atomic.Bool
	// deferred is the transaction of a plain BEGIN, which holds no lock until its first write.
	deferred *Deferred
	// follower follows the session's transaction: package statement's SQLite transaction.
	follower Follower
	// life ends when Close is called, before Close waits for a request in progress: it ends that request's lock
	// wait, as a closed connection does.
	life context.Context
	end  context.CancelFunc
}

// errSessionClosed is the cause of a lock wait that Close ended.
var errSessionClosed = errors.New("the session was closed")

// bind returns ctx ended also by the session's Close, and the function that releases it.
func (s *Session) bind(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(s.life, func() { cancel(errSessionClosed) })
	return ctx, func() {
		stop()
		cancel(nil)
	}
}

// tx is an open transaction: the lock it holds, the corpus read at its begin, its post-image, the writes that made
// it, and its savepoints.
type tx struct {
	lock     *journal.Lock
	readOnly bool
	base     *irexec.Corpus
	cur      *irexec.Corpus
	writes   []*write
	marks    []mark
}

// write is a write of a transaction: how to run it again on a corpus (the commit's step 2), the exemptions of its
// rewrite refusal, and what it answered and changed.
type write struct {
	run     func(*irexec.Corpus) (*irexec.Result, error)
	exempt  Exempt
	answer  *irexec.Answer
	targets []irexec.Target
}

// mark is a savepoint: its name, and the post-image and the number of writes when it was set.
type mark struct {
	name string
	cur  *irexec.Corpus
	n    int
}

// Exempt says whether the rewrite refusal passes the format diagnostic d of the target path: true for the targets of
// flowmark canon, and for the diagnostics an added column causes in the targets of the add-column migration.
type Exempt func(path string, d catalog.Diagnostic) bool

// Rewrite is a file that an operation outside the executor rewrites: its new bytes, or nil to remove it.
type Rewrite struct {
	Path  string
	After []byte
}

// op is one operation of a session: how it runs against a corpus, whether it writes, the exemptions of its rewrite
// refusal, and whether it is its own transaction.
type op struct {
	run    func(*irexec.Corpus) (*irexec.Result, error)
	write  bool
	exempt Exempt
	own    bool
}

// Session returns a new session named name, the session of its lock holder record; each acquisition of the lock
// carries its own token, so sessions of one name never take each other's lock. name holds no '/', '@' or line break.
func (c *Corpus) Session(name string) (*Session, error) {
	if name == "" || strings.ContainsAny(name, "/@\r\n") {
		return nil, newError(codeConfig, "the session name %q is empty or holds '/', '@' or a line break, which a lock holder name cannot hold", name)
	}
	life, end := context.WithCancel(context.Background())
	return &Session{c: c, name: name, life: life, end: end}, nil
}

// Name returns the session's name, which the lock names as the holder's session.
func (s *Session) Name() string { return s.name }

// InTransaction reports whether a transaction is open in the session; it waits for a request in progress.
func (s *Session) InTransaction() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tx != nil
}

// Exec runs one request message of the IR-executor protocol. Outside a transaction it is one transaction under the
// lock, and a write that meets a conflict at its commit is run once more on the corpus read again (spec 40
// Transactions). Inside a transaction it reads and changes the transaction's post-image; a failed request changes
// nothing, and the transaction stays open.
func (s *Session) Exec(ctx context.Context, msg []byte) (*irexec.Answer, error) {
	msg = bytes.Clone(msg)
	return s.do(ctx, op{write: irexec.IsWrite(msg), run: func(c *irexec.Corpus) (*irexec.Result, error) { return s.c.exec.Execute(c, msg) }})
}

// ExecFace runs one request message that a face operation formed, as Exec runs one of an orm client; the values
// are the engine's own write, so their parameters are not checked as an orm client's are.
func (s *Session) ExecFace(ctx context.Context, msg []byte) (*irexec.Answer, error) {
	msg = bytes.Clone(msg)
	return s.do(ctx, op{write: irexec.IsWrite(msg), run: func(c *irexec.Corpus) (*irexec.Result, error) { return s.c.exec.ExecuteFace(c, msg) }})
}

// RaiseSequences raises the AUTOINCREMENT counter of each table of raise to its value where the corpus's sequence of the
// table is lower, as a write of the session's transaction, or as one transaction under the lock (irexec
// Corpus.RaiseSequences).
func (s *Session) RaiseSequences(ctx context.Context, raise map[string]int64) error {
	_, err := s.do(ctx, op{write: true, run: func(c *irexec.Corpus) (*irexec.Result, error) { return c.RaiseSequences(raise) }})
	return err
}

// acquire takes the corpus lock with the wait of the session.
func (s *Session) acquire(ctx context.Context) (*journal.Lock, error) {
	if l := s.kept.Load(); l != nil {
		return l, nil
	}
	wait := s.c.wait
	if ms := s.busyMS.Load(); ms > 0 {
		wait = time.Duration(ms-1) * time.Millisecond
	}
	l, err := s.c.acquireWait(ctx, s.name, wait)
	if err == nil && s.exclusive.Load() {
		l.Keep(true)
		s.kept.Store(l)
	}
	return l, err
}

// SetExclusive sets the locking_mode of a SQL connection: under EXCLUSIVE the lock its next transaction takes stays when
// the transaction ends, and NORMAL gives it back when no transaction holds it.
func (s *Session) SetExclusive(on bool) error {
	s.exclusive.Store(on)
	if on {
		return nil
	}
	l := s.kept.Swap(nil)
	if l == nil {
		return nil
	}
	l.Keep(false)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tx != nil {
		return nil
	}
	return release(l, nil)
}

// Deferred is the transaction of a plain BEGIN of a SQL connection before its first write: it holds no lock, and what it
// reads is the corpus Pinned, which its first read fixes, on the connection Reader, which the session's end closes.
// Savepoints are those it has set.
type Deferred struct {
	Pinned     *irexec.Corpus
	Reader     io.Closer
	Savepoints []string
}

// Follower follows the transaction of a session, as package statement's SQLite transaction follows it: it sets, releases
// and rolls back to each savepoint before the transaction does, Ready refuses a commit before the corpus commit as
// SQLite refuses a COMMIT, leaving the transaction open, and End ends it after the transaction ended, written telling
// whether the corpus commit is written.
type Follower interface {
	Savepoint(name string) error
	Release(name string) error
	RollbackTo(name string, cur *irexec.Corpus) error
	Ready() error
	End(written bool) error
}

// Follow makes f the follower of the session's open transaction, or of the schema statement Reschema commits, until it
// ends.
func (s *Session) Follow(f Follower) { s.follower = f }

// Following returns the follower of the session's transaction, nil when it has none.
func (s *Session) Following() Follower { return s.follower }

// Mark is a savepoint of a transaction: its name and the post-image it holds.
type Mark struct {
	Name string
	Cur  *irexec.Corpus
}

// Begun returns the corpus the session's open transaction read at its begin and its savepoints, in the order they were
// set, for a follower that begins after them.
func (s *Session) Begun() (*irexec.Corpus, []Mark) {
	if s.tx == nil {
		return nil, nil
	}
	marks := make([]Mark, len(s.tx.marks))
	for i, m := range s.tx.marks {
		marks[i] = Mark{Name: m.name, Cur: m.cur}
	}
	return s.tx.base, marks
}

// endFollower ends the follower of the session's transaction, if it has one.
func (s *Session) endFollower(written bool) error {
	f := s.follower
	if f == nil {
		return nil
	}
	s.follower = nil
	return f.End(written)
}

// Deferred returns the session's deferred transaction, nil when it has none.
func (s *Session) Deferred() *Deferred { return s.deferred }

// SetDeferred sets the session's deferred transaction.
func (s *Session) SetDeferred(d *Deferred) { s.deferred = d }

// Snapshot returns the corpus that a read without the lock sees: what is committed, read now when the lock is free or no
// one can hold it, and the corpus last committed while another session writes one.
func (s *Session) Snapshot(ctx context.Context) (*irexec.Corpus, error) {
	if s.kept.Load() != nil {
		return s.c.read()
	}
	l, err := s.c.j.Acquire(ctx, s.name, 0)
	if err == nil {
		if err := s.c.j.Recover(l); err != nil {
			return nil, release(l, driverError("recovering the corpus", err))
		}
		cur, err := s.c.read()
		if err := release(l, err); err != nil {
			return nil, err
		}
		return cur, nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return s.c.read()
	}
	var timeout *journal.LockTimeout
	if c := s.c.committed.Load(); c != nil && errors.As(err, &timeout) {
		return c, nil
	}
	return nil, lockError(err)
}

// SetBusyTimeout sets the wait of the session's lock.
func (s *Session) SetBusyTimeout(ms int64) { s.busyMS.Store(ms + 1) }

// Settings returns the PRAGMA settings of the session's statements. Requests of a session run one at a time.
func (s *Session) Settings() map[string]string { return s.settings }

// SetSetting sets a PRAGMA setting of the session's statements.
func (s *Session) SetSetting(name, value string) {
	if s.settings == nil {
		s.settings = map[string]string{}
	}
	s.settings[name] = value
}

// SetEnded records that a statement ended the session's transaction, whose ROLLBACK then has nothing to do.
func (s *Session) SetEnded(ended bool) { s.ended.Store(ended) }

// Ended reports whether a statement ended the session's transaction, until the next one begins.
func (s *Session) Ended() bool { return s.ended.Load() }

// Transaction reports whether a transaction is open in the session, for the holder of its lock.
func (s *Session) Transaction() bool { return s.tx != nil }

// ReadCommitted calls read with the corpus last committed and reports true, when another session holds the lock; a free
// lock, or a corpus not read yet, is false and read is not called.
func (s *Session) ReadCommitted(ctx context.Context, read func(c *irexec.Corpus) error) (bool, error) {
	if s.kept.Load() != nil {
		return false, nil
	}
	c := s.c.committed.Load()
	l, err := s.c.j.Acquire(ctx, s.name, 0)
	if err == nil {
		// The lock is free: the statement reads under it, from the database of its writers.
		return false, l.Release()
	}
	var timeout *journal.LockTimeout
	switch {
	case errors.Is(err, fs.ErrPermission):
		// No one can hold the lock of a corpus the process may only read, and reading needs none.
		if c, err = s.c.read(); err != nil {
			return false, err
		}
	case !errors.As(err, &timeout), c == nil:
		return false, nil
	}
	return true, read(c)
}

// Cost returns the counts of the work of the session's corpus.
func (s *Session) Cost() *cost.Counts { return s.c.cost }

// Life is the context that the session's Close ends.
func (s *Session) Life() context.Context { return s.life }

// SetCursor keeps the rows of the session's last statement, which go on.
func (s *Session) SetCursor(c io.Closer) { s.cursor.Store(&c) }

// Cursor returns the rows of the session's last statement that go on, nil when none.
func (s *Session) Cursor() io.Closer {
	if c := s.cursor.Load(); c != nil {
		return *c
	}
	return nil
}

// TakeCursor returns the rows that go on and forgets them.
func (s *Session) TakeCursor() io.Closer {
	if c := s.cursor.Swap(nil); c != nil {
		return *c
	}
	return nil
}

// ForeignKeysOff reports whether the session's statements run with foreign_keys off.
func (s *Session) ForeignKeysOff() bool { return s.foreignKeysOff.Load() }

// SetForeignKeysOff sets the foreign_keys of the session's statements.
func (s *Session) SetForeignKeysOff(off bool) { s.foreignKeysOff.Store(off) }

// QueryOnly reports the query_only of the session's statements.
func (s *Session) QueryOnly() bool { return s.queryOnly.Load() }

// SetQueryOnly sets the query_only of the session's statements.
func (s *Session) SetQueryOnly(on bool) { s.queryOnly.Store(on) }

// SQL returns what package statement keeps of the SQL database of the corpus, nil before its first statement. It is read
// without the lock.
func (s *Session) SQL() io.Closer {
	s.c.sqlMu.Lock()
	defer s.c.sqlMu.Unlock()
	return s.c.sql
}

// SwapSQL makes h what package statement keeps of the SQL database of the corpus when it is old, and reports whether it
// did.
func (s *Session) SwapSQL(old, h io.Closer) bool {
	s.c.sqlMu.Lock()
	defer s.c.sqlMu.Unlock()
	if s.c.sql != old {
		return false
	}
	s.c.sql = h
	return true
}

// NewSQLFile makes the place of a new SQL database of the corpus (journal Corpus.NewDatabase); it is nil for a corpus the
// process may only read, whose database is in memory.
func (s *Session) NewSQLFile() (*journal.Database, error) {
	d, err := s.c.j.NewDatabase()
	if errors.Is(err, fs.ErrPermission) {
		return nil, nil
	}
	if err != nil {
		return nil, driverError("making the SQL database of the corpus", err)
	}
	return d, nil
}

// View calls read with the corpus the session reads: its transaction's post-image, or the corpus read now under the
// lock. A statement of SQL reads through it and writes what it changed as requests (package statement).
func (s *Session) View(ctx context.Context, read func(*irexec.Corpus) error) error {
	_, err := s.do(ctx, op{run: func(c *irexec.Corpus) (*irexec.Result, error) {
		if err := read(c); err != nil {
			return nil, err
		}
		return &irexec.Result{Answer: &irexec.Answer{}}, nil
	}})
	return err
}

// Reschema commits the files and the schema statements that compute derives from the corpus, as its own transaction under
// the lock, and takes the catalog it returns: the corpus has the tables of a schema statement from here on (spec 40 SQL
// statements). A follower that compute sets ends with it.
func (s *Session) Reschema(ctx context.Context, compute func(*irexec.Corpus) (*catalog.Catalog, []journal.Change, error)) (err error) {
	ctx, leave, err := s.enter(ctx)
	if err != nil {
		return err
	}
	defer leave()
	if s.tx != nil {
		return newError(codeConfig, "a schema statement is its own transaction; commit or roll back the open transaction first")
	}
	l, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	written := false
	defer func() { err = releaseAfter(l, written, errors.Join(err, s.endFollower(written))) }()
	base, err := s.c.read()
	if err != nil {
		return err
	}
	cat, changes, err := compute(base)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return nil
	}
	paths := make([]string, len(changes))
	for i, ch := range changes {
		paths[i] = ch.Path
	}
	s.c.tree.Forget(paths...)
	defer s.c.tree.Forget(paths...)
	if written, err = s.c.journalCommit(l, changes); err != nil {
		return err
	}
	return s.c.useCatalog(cat)
}

// Executor returns the executor of the corpus's catalog. Under the lock.
func (s *Session) Executor() *irexec.Executor { return s.c.exec }

// Root returns the root of the corpus.
func (s *Session) Root() string { return s.c.root }

// Repair runs the history repair of the row of table whose key is key (spec 50 Repair), from the initial state from
// when it is given, at the engine clock: a write, inside or outside a transaction.
func (s *Session) Repair(ctx context.Context, table, key, from string) (*irexec.Answer, error) {
	now := s.c.clock()
	return s.do(ctx, op{write: true, run: func(c *irexec.Corpus) (*irexec.Result, error) { return s.c.exec.Repair(c, table, key, from, now) }})
}

// Rewrite commits the files that compute derives from the corpus outside the executor — flowmark canon, the
// add-column migration — as its own transaction under the lock, and returns the number of files it changed. exempt
// passes the format diagnostics of its targets that the operation is exempt for (spec 40 The commit protocol, step 2);
// a nil exempt passes none. compute may run more than once: on the corpus read again when the commit meets a change.
func (s *Session) Rewrite(ctx context.Context, compute func(*irexec.Corpus) ([]Rewrite, error), exempt Exempt) (int, error) {
	run := func(c *irexec.Corpus) (*irexec.Result, error) {
		rws, err := compute(c)
		if err != nil {
			return nil, err
		}
		var targets []irexec.Target
		seen := map[string]bool{}
		for _, r := range rws {
			if seen[r.Path] {
				return nil, newError(codeInternal, "the rewrite changes %s twice", r.Path)
			}
			seen[r.Path] = true
			before := c.Data(r.Path)
			if before == nil && r.After == nil {
				return nil, newError(codeInternal, "the rewrite removes %s, which the corpus does not hold", r.Path)
			}
			if bytes.Equal(before, r.After) && (before == nil) == (r.After == nil) {
				continue
			}
			targets = append(targets, irexec.Target{Path: r.Path, Before: before, After: r.After, Diagnostics: c.Diagnostics(r.Path)})
		}
		slices.SortFunc(targets, func(a, b irexec.Target) int { return strings.Compare(a.Path, b.Path) })
		return &irexec.Result{Answer: &irexec.Answer{Kind: "rewrite", Affected: int64(len(targets))}, Post: &irexec.PostImage{Targets: targets}}, nil
	}
	a, err := s.do(ctx, op{write: true, own: true, exempt: exempt, run: run})
	if err != nil {
		return 0, err
	}
	return int(a.Affected), nil
}

// enter binds ctx to the session and holds the session for one call; leave releases both. A closed session is CONFIG,
// and then nothing is held.
func (s *Session) enter(ctx context.Context) (_ context.Context, leave func(), _ error) {
	ctx, unbind := s.bind(ctx)
	s.mu.Lock()
	leave = func() {
		s.mu.Unlock()
		unbind()
	}
	if s.closed {
		leave()
		return ctx, nil, newError(codeConfig, "the session %s is closed", s.name)
	}
	return ctx, leave, nil
}

// do runs an operation in the session's transaction, or as one transaction under the lock.
func (s *Session) do(ctx context.Context, o op) (*irexec.Answer, error) {
	ctx, leave, err := s.enter(ctx)
	if err != nil {
		return nil, err
	}
	defer leave()
	if s.tx == nil {
		return s.single(ctx, o)
	}
	if o.own {
		return nil, newError(codeConfig, "a rewrite (flowmark canon, the add-column migration) is its own transaction; commit or roll back the open transaction first")
	}
	t := s.tx
	if o.write && t.readOnly {
		return nil, newError(codeReadOnly, "a write in a read-only transaction; begin a transaction that is not read-only to write")
	}
	res, err := o.run(t.cur)
	if err != nil {
		return nil, err
	}
	if res.Post == nil {
		return res.Answer, nil
	}
	if err := s.c.refuseRewrite(res.Post.Targets, o.exempt); err != nil {
		return nil, err
	}
	t.writes = append(t.writes, &write{run: o.run, exempt: o.exempt, answer: res.Answer, targets: res.Post.Targets})
	t.cur = res.Post.Next
	return res.Answer, nil
}

// single runs an operation as one transaction under the lock. A write whose commit meets WRITE_CONFLICT runs once more
// on the corpus read again, still under the lock; a second conflict is returned (spec 40 Transactions).
// The lock is released on every way out, a panic included.
func (s *Session) single(ctx context.Context, o op) (a *irexec.Answer, err error) {
	l, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	written := false
	defer func() {
		if err = releaseAfter(l, written, err); err != nil {
			a = nil
		}
	}()
	a, written, err = s.c.runSingle(l, o)
	return a, err
}

// runSingle runs an operation under the held lock l and reports whether it wrote the corpus.
func (c *Corpus) runSingle(l *journal.Lock, o op) (*irexec.Answer, bool, error) {
	for attempt := 0; ; attempt++ {
		base, err := c.read()
		if err != nil {
			return nil, false, err
		}
		if c.readHook != nil {
			c.readHook(attempt)
		}
		res, err := o.run(base)
		if err != nil {
			return nil, false, err
		}
		if res.Post == nil {
			return res.Answer, false, nil
		}
		if err := c.refuseRewrite(res.Post.Targets, o.exempt); err != nil {
			return nil, false, err
		}
		t := &tx{lock: l, base: base, writes: []*write{{run: o.run, exempt: o.exempt, answer: res.Answer, targets: res.Post.Targets}}}
		written, err := c.commit(t)
		if err == nil {
			return res.Answer, written, nil
		}
		if attempt > 0 || !isConflict(err) {
			return nil, written, err
		}
	}
}

// Begin opens a transaction: it takes the lock, waiting as the corpus's operations wait, and reads the corpus. A
// read-only transaction takes the lock too and refuses every write as READ_ONLY. A begin that does not open the
// transaction, by an error or a panic, releases the lock.
func (s *Session) Begin(ctx context.Context, readOnly bool) (err error) {
	ctx, leave, err := s.enter(ctx)
	if err != nil {
		return err
	}
	defer leave()
	if s.tx != nil {
		return newError(codeConfig, "a transaction is already open in the session %s; commit or roll it back before begin", s.name)
	}
	l, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if s.tx == nil {
			err = release(l, err)
		}
	}()
	base, err := s.c.read()
	if err != nil {
		return err
	}
	s.tx = &tx{lock: l, readOnly: readOnly, base: base, cur: base}
	return nil
}

// noTx is the failure of a control message that needs an open transaction.
func (s *Session) noTx(what string) error {
	return newError(codeConfig, "%s: no transaction is open in the session %s; begin one first", what, s.name)
}

// Commit commits the transaction and releases the lock, on every way out, a panic included. A commit that fails
// before its prepare record writes nothing; from the prepare record on it is decided (see commit). Either way the
// transaction ends.
func (s *Session) Commit() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tx
	if t == nil {
		return s.noTx("commit")
	}
	if f := s.follower; f != nil {
		if err := f.Ready(); err != nil {
			return err
		}
	}
	s.tx = nil
	written := false
	defer func() { err = releaseAfter(t.lock, written, errors.Join(err, s.endFollower(written))) }()
	written, err = s.c.commit(t)
	return err
}

// Rollback discards the transaction and releases the lock.
func (s *Session) Rollback() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tx
	if t == nil {
		return s.noTx("rollback")
	}
	s.tx = nil
	return release(t.lock, s.endFollower(false))
}

// Savepoint sets a savepoint named name at the transaction's present state: a nested level. A savepoint of the same
// name is replaced, as the SQL standard destroys it.
func (s *Session) Savepoint(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tx
	switch {
	case t == nil:
		return s.noTx("savepoint")
	case name == "":
		return newError(codeConfig, "a savepoint needs a name")
	}
	if f := s.follower; f != nil {
		if err := f.Savepoint(name); err != nil {
			return err
		}
	}
	t.marks = slices.DeleteFunc(t.marks, func(m mark) bool { return m.name == name })
	t.marks = append(t.marks, mark{name: name, cur: t.cur, n: len(t.writes)})
	return nil
}

// find returns the index of the savepoint name of the transaction.
func (s *Session) find(what, name string) (int, error) {
	if s.tx == nil {
		return 0, s.noTx(what)
	}
	for i := len(s.tx.marks) - 1; i >= 0; i-- {
		if s.tx.marks[i].name == name {
			return i, nil
		}
	}
	return 0, newError(codeConfig, "%s: no savepoint %s is set in the transaction", what, name)
}

// Release removes the savepoint name and every savepoint set after it, keeping their writes.
func (s *Session) Release(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.find("release", name)
	if err != nil {
		return err
	}
	if f := s.follower; f != nil {
		if err := f.Release(name); err != nil {
			return err
		}
	}
	s.tx.marks = s.tx.marks[:i]
	return nil
}

// RollbackTo discards the writes since the savepoint name and the savepoints set after it; the savepoint stays.
func (s *Session) RollbackTo(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.find("rollback_to", name)
	if err != nil {
		return err
	}
	if f := s.follower; f != nil {
		if err := f.RollbackTo(name, s.tx.marks[i].cur); err != nil {
			return err
		}
	}
	m := s.tx.marks[i]
	s.tx.cur = m.cur
	s.tx.writes = s.tx.writes[:m.n]
	s.tx.marks = s.tx.marks[:i+1]
	return nil
}

// Close closes the session: an open transaction is rolled back and the lock released. A closed session refuses every
// operation; closing it again does nothing.
func (s *Session) Close() error {
	s.end()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var err error
	if c := s.TakeCursor(); c != nil {
		err = c.Close()
	}
	if t := s.tx; t != nil {
		s.tx = nil
		err = release(t.lock, errors.Join(err, s.endFollower(false)))
	}
	if d := s.deferred; d != nil {
		s.deferred = nil
		if d.Reader != nil {
			err = errors.Join(err, d.Reader.Close())
		}
	}
	if l := s.kept.Swap(nil); l != nil {
		l.Keep(false)
		err = release(l, err)
	}
	return err
}
