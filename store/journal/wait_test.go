package journal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type result struct {
	l   *Lock
	err error
}

func acquireAsync(c *Corpus, session string, wait time.Duration) chan result {
	return acquireCtx(context.Background(), c, session, wait)
}

func acquireCtx(ctx context.Context, c *Corpus, session string, wait time.Duration) chan result {
	ch := make(chan result, 1)
	go func() {
		l, err := c.Acquire(ctx, session, wait)
		ch <- result{l, err}
	}()
	return ch
}

func await(t *testing.T, ch chan result) result {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(patience):
		t.Fatal("the acquisition did not return")
	}
	return result{}
}

// noWatch fails a test that arms a watch where none may be armed.
func noWatch(t *testing.T) func() (lockWatch, error) {
	return func() (lockWatch, error) {
		t.Error("a watch was armed")
		return nil, errors.New("no watch here")
	}
}

// fakeWatch is the watcher seam: the test sends the wakes an OS notification would.
type fakeWatch struct{ ch chan wake }

func (f *fakeWatch) armLock(fileID) (bool, error) { return false, nil }
func (f *fakeWatch) armHolder(Holder) error       { return nil }
func (f *fakeWatch) wakes() <-chan wake           { return f.ch }
func (f *fakeWatch) close() error                 { return nil }

// A wait of 0 refuses at once and arms nothing.
func TestLockWaitZeroRefusesAtOnce(t *testing.T) {
	c, _ := newCorpus(t, t.TempDir())
	holder := acquire(t, c, "s1")
	c.newWatch = noWatch(t)
	_, err := c.Acquire(context.Background(), "s2", 0)
	var timeout *LockTimeout
	if !errors.As(err, &timeout) || timeout.State != HolderRunning || timeout.Holder != holder.Holder() {
		t.Fatalf("Acquire = %v, want a timeout naming %v", err, holder.Holder())
	}
}

// The wait ends at its deadline with LockTimeout naming the holder and its state.
func TestLockWaitTimesOutNamingHolder(t *testing.T) {
	c, rec := newCorpus(t, t.TempDir())
	holder := acquire(t, c, "s1")
	const wait = 300 * time.Millisecond
	start := time.Now()
	_, err := c.Acquire(context.Background(), "s2", wait)
	elapsed := time.Since(start)
	var timeout *LockTimeout
	if !errors.As(err, &timeout) || timeout.State != HolderRunning || timeout.Holder != holder.Holder() {
		t.Fatalf("Acquire = %v, want a timeout naming %v", err, holder.Holder())
	}
	if elapsed < wait {
		t.Fatalf("the wait ended after %s, before its deadline %s", elapsed, wait)
	}
	if !strings.Contains(err.Error(), holder.Holder().String()) || !strings.Contains(err.Error(), "runs") {
		t.Fatalf("message %q does not name the running holder", err)
	}
	if got := rec.waitFor(t, 1, kindIs(LockWaiting)); got[0].Holder != holder.Holder().String() {
		t.Fatalf("waiting event = %+v", got[0])
	}
}

// A holder that has ended refuses at once, however long the wait.
func TestLockWaitEndedHolderRefusesAtOnce(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	gone := selfRecord(t, c, "gone")
	gone.PID = endedPID(t)
	plantRecord(t, root, LockPath, gone)
	c.newWatch = noWatch(t)
	start := time.Now()
	_, err := c.Acquire(context.Background(), "s1", time.Hour)
	var timeout *LockTimeout
	if !errors.As(err, &timeout) || timeout.State != HolderEnded {
		t.Fatalf("Acquire = %v, want an ended holder", err)
	}
	if elapsed := time.Since(start); elapsed > patience {
		t.Fatalf("took %s", elapsed)
	}
}

// A closed connection (a canceled context) ends the wait.
func TestLockWaitContextCanceled(t *testing.T) {
	c, rec := newCorpus(t, t.TempDir())
	acquire(t, c, "s1")
	ctx, cancel := context.WithCancel(context.Background())
	ch := acquireCtx(ctx, c, "s2", time.Hour)
	rec.waitFor(t, 1, kindIs(LockWaiting))
	cancel()
	r := await(t, ch)
	if !errors.Is(r.err, context.Canceled) {
		t.Fatalf("Acquire = %v, want context.Canceled", r.err)
	}
	pendings, _ := filepath.Glob(filepath.Join(c.root, ".flowmark", "lock.*.pending"))
	if len(pendings) != 0 {
		t.Fatalf("pending records left: %v", pendings)
	}
}

// A watch that cannot be armed refuses the wait at once, naming the cause.
func TestLockWaitArmFailureRefuses(t *testing.T) {
	c, _ := newCorpus(t, t.TempDir())
	acquire(t, c, "s1")
	c.newWatch = func() (lockWatch, error) { return nil, errors.New("the watch cannot be armed: test cause") }
	_, err := c.Acquire(context.Background(), "s2", time.Hour)
	if err == nil || !strings.Contains(err.Error(), "test cause") {
		t.Fatalf("Acquire = %v, want the arming cause", err)
	}
}

// An overflow (any wake the watch reports) starts exactly one more attempt; the wait goes on while the lock is held.
func TestLockWaitOverflowRetries(t *testing.T) {
	c, rec := newCorpus(t, t.TempDir())
	holder := acquire(t, c, "s1")
	fake := &fakeWatch{ch: make(chan wake, 1)}
	c.newWatch = func() (lockWatch, error) { return fake, nil }
	tried := make(chan struct{}, 8)
	n := countAttempts(c, map[string]func(){"lock-attempt": func() { tried <- struct{}{} }})
	ch := acquireAsync(c, "s2", time.Hour)
	rec.waitFor(t, 1, kindIs(LockWaiting))
	before := n.Load()
	for len(tried) > 0 {
		<-tried
	}
	fake.ch <- wake{kind: wakeLock}
	select {
	case <-tried:
	case <-time.After(patience):
		t.Fatal("the overflow started no attempt")
	}
	select {
	case r := <-ch:
		t.Fatalf("the wait ended while the lock was held: %v", r.err)
	default:
	}
	if err := holder.Release(); err != nil {
		t.Fatal(err)
	}
	fake.ch <- wake{kind: wakeLock}
	if r := await(t, ch); r.err != nil {
		t.Fatal(r.err)
	}
	if got := n.Load() - before; got != 2 {
		t.Fatalf("attempts after the waiting event = %d, want 2 (one per wake)", got)
	}
}

// A release between the failed try and the arming of the watch is never lost: the wait acquires without any later
// notification.
func TestLockWaitNoLostWakeup(t *testing.T) {
	c, _ := newCorpus(t, t.TempDir())
	holder := acquire(t, c, "s1")
	countAttempts(c, map[string]func(){"lock-before-arm": func() {
		if err := holder.Release(); err != nil {
			t.Error(err)
		}
	}})
	l, err := c.Acquire(context.Background(), "s2", patience)
	if err != nil {
		t.Fatalf("Acquire = %v, want the lock released before the arming", err)
	}
	if l.Holder().Session != "s2" {
		t.Fatalf("holder = %v", l.Holder())
	}
}

// Waiters and their pending records cause no event storm: changes of other names start no attempt, and one release
// starts exactly one attempt per waiter.
func TestWaitersCauseNoEventStorm(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	holder := acquire(t, c, "h")
	n := countAttempts(c, nil)
	const waiters = 4
	results := make(chan result, waiters)
	for i := range waiters {
		go func() {
			l, err := c.Acquire(context.Background(), fmt.Sprintf("w%d", i), patience)
			results <- result{l, err}
		}()
	}
	rec.waitFor(t, waiters, kindIs(LockWaiting))
	before := n.Load()
	// Noise: names that come and go beside the lock, as the pending records of other waiters do, and another name of
	// the lock file itself, as a holder's pending record is until its holder removes it.
	for i := range 20 {
		p := filepath.Join(root, ".flowmark", fmt.Sprintf("lock.%032x.pending", i))
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, ".flowmark", fmt.Sprintf("lock.%032x.pending", 99))
	if err := os.Link(lockFile(root), alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := holder.Release(); err != nil {
		t.Fatal(err)
	}
	acquired := rec.waitFor(t, 2, kindIs(LockAcquired))
	winner := acquired[1].Holder
	rec.waitFor(t, waiters-1, func(e Event) bool { return e.Kind == LockWaiting && e.Holder == winner })
	if got := n.Load() - before; got != waiters {
		t.Fatalf("attempts after the noise and one release = %d, want %d", got, waiters)
	}
	for range waiters {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if err := r.l.Release(); err != nil {
			t.Fatal(err)
		}
	}
}

// startHolder runs a child that takes the lock and holds it until its stdin closes; it returns the child, its stdin
// and its holder.
func startHolder(t *testing.T, root string) (*childProc, Holder) {
	t.Helper()
	p := startChild(t, "hold", root, "child")
	line := p.line(t)
	name, ok := strings.CutPrefix(line, "acquired ")
	if !ok {
		t.Fatalf("child = %q", line)
	}
	h, err := parseRecord([]byte(name + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return p, h
}

type childProc struct {
	cmd   interface{ Wait() error }
	kill  func() error
	stdin io.WriteCloser
	out   *bufio.Reader
}

func (p *childProc) line(t *testing.T) string {
	t.Helper()
	line, err := p.out.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(line, "\n")
}

func startChild(t *testing.T, mode, root, session string, env ...string) *childProc {
	t.Helper()
	cmd := childCommand(t, mode, root, session, env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return &childProc{cmd: cmd, kill: cmd.Process.Kill, stdin: stdin, out: bufio.NewReader(stdout)}
}

// Another process releases: the waiter learns it from the OS notification and acquires.
func TestLockWaitAcquiresOnRelease(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	child, h := startHolder(t, root)
	ch := acquireAsync(c, "parent", patience)
	if got := rec.waitFor(t, 1, kindIs(LockWaiting)); got[0].Holder != h.String() {
		t.Fatalf("waiting for %q, want %q", got[0].Holder, h)
	}
	if err := child.stdin.Close(); err != nil {
		t.Fatal(err)
	}
	r := await(t, ch)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if err := child.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := r.l.Release(); err != nil {
		t.Fatal(err)
	}
}

// A holder that ends without releasing wakes the waiter through the holder-exit watch, which refuses at once naming
// it as ended.
func TestLockWaitHolderExitWakes(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	child, h := startHolder(t, root)
	ch := acquireAsync(c, "parent", patience)
	rec.waitFor(t, 1, kindIs(LockWaiting))
	start := time.Now()
	if err := child.kill(); err != nil {
		t.Fatal(err)
	}
	r := await(t, ch)
	var timeout *LockTimeout
	if !errors.As(r.err, &timeout) || timeout.State != HolderEnded || timeout.Holder != h {
		t.Fatalf("Acquire = %v, want the ended holder %v", r.err, h)
	}
	if elapsed := time.Since(start); elapsed >= patience/2 {
		t.Fatalf("the exit was learned after %s", elapsed)
	}
	_ = child.cmd.Wait()
}
