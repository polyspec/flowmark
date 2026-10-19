package writepath

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

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

// do runs an operation in the session's transaction, or as one transaction under the lock.
func (s *Session) do(ctx context.Context, o op) (*irexec.Answer, error) {
	ctx, unbind := s.bind(ctx)
	defer unbind()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, newError(codeConfig, "the session %s is closed", s.name)
	}
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
	l, err := s.c.acquire(ctx, s.name)
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
	ctx, unbind := s.bind(ctx)
	defer unbind()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(codeConfig, "the session %s is closed", s.name)
	}
	if s.tx != nil {
		return newError(codeConfig, "a transaction is already open in the session %s; commit or roll it back before begin", s.name)
	}
	l, err := s.c.acquire(ctx, s.name)
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
	s.tx = nil
	written := false
	defer func() { err = releaseAfter(t.lock, written, err) }()
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
	return release(t.lock, nil)
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
	if t := s.tx; t != nil {
		s.tx = nil
		return release(t.lock, nil)
	}
	return nil
}
