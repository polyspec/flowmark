package journal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"sync"
	"time"
)

// LockPath is the lock file of every corpus, under its root (spec 40, the lock).
const LockPath = ".flowmark/lock"

const (
	pendingSuffix  = ".pending"
	clearingSuffix = ".clearing"
)

// pendingName matches a pending record `.flowmark/lock.<32 hex>.pending`.
var pendingName = regexp.MustCompile(`^lock\.[0-9a-f]{32}\.pending$`)

// LockTimeout is LOCK_TIMEOUT (orm CANCELED): the lock wait ended — at its deadline, or at once when the holder has
// ended — naming the holder and its state.
type LockTimeout struct {
	Path   string
	Corpus string
	Holder Holder
	State  HolderState
	Wait   time.Duration
}

func (e *LockTimeout) Error() string {
	switch e.State {
	case HolderEnded:
		return fmt.Sprintf("journal: LOCK_TIMEOUT: %s is held by %s, which has ended without releasing it; check that it no longer runs, then run `flowmark unlock %s`", e.Path, e.Holder.describe(), e.Corpus)
	case HolderUnknown:
		return fmt.Sprintf("journal: LOCK_TIMEOUT: %s is held by %s on another host, whose process cannot be checked here; %s; once that holder has ended, run `flowmark unlock %s --holder %s`", e.Path, e.Holder.describe(), e.waited(), e.Corpus, e.Holder)
	}
	return fmt.Sprintf("journal: LOCK_TIMEOUT: %s is held by %s, which runs; %s; try again once that operation has ended", e.Path, e.Holder.describe(), e.waited())
}

func (e *LockTimeout) waited() string {
	if e.Wait == 0 {
		return "no wait was asked"
	}
	return fmt.Sprintf("the wait of %s ended at its deadline", e.Wait)
}

// Lock is one acquisition of the corpus lock.
type Lock struct {
	c      *Corpus
	path   string
	holder Holder
	rec    []byte
	mu     sync.Mutex
	held   bool
}

// Holder returns the record this lock wrote.
func (l *Lock) Holder() Holder { return l.holder }

// readShared reads a file of the lock or the journal and returns its bytes and identity.
func readShared(p string) ([]byte, fileID, error) {
	f, err := openShared(p, os.O_RDONLY, 0)
	if err != nil {
		return nil, fileID{}, err
	}
	data, err := io.ReadAll(f)
	var id fileID
	if err == nil {
		id, err = fileIdentity(f)
	}
	if err := errors.Join(err, f.Close()); err != nil {
		return nil, fileID{}, err
	}
	return data, id, nil
}

// writeSynced writes data to a file (flag adds os.O_EXCL or os.O_TRUNC) and syncs it before closing.
func writeSynced(p string, data []byte, flag int) error {
	f, err := openShared(p, os.O_WRONLY|os.O_CREATE|flag, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	if werr == nil {
		werr = f.Sync()
	}
	return errors.Join(werr, f.Close())
}

// removeAbsent removes p and accepts that it does not exist.
func removeAbsent(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// linkError names the cause of a failed link of the lock: a file system without hard links is not supported.
func linkError(err error) error {
	if linkUnsupported(err) {
		return fmt.Errorf("journal: the file system of the corpus has no hard links, so the lock cannot appear atomically with its whole record; FAT32 and exFAT are not supported (spec 40, file system): %w", err)
	}
	return fmt.Errorf("journal: linking the lock: %w", err)
}

// acquirer is one acquisition in progress: its record is written once to its pending file, and each attempt links
// that file to the lock.
type acquirer struct {
	c       *Corpus
	self    Holder
	record  []byte
	lock    string
	pending string
	wait    time.Duration
}

// tried is what one attempt found.
type tried struct {
	acquired bool
	holder   Holder
	state    HolderState
	id       fileID
}

// Acquire takes the corpus lock for session. The record is written whole to a pending file, synced, and hard-linked
// to `.flowmark/lock`; the link fails when the lock exists, so the lock appears atomically with its whole record (the
// kit holder-lock precedent; no OS file lock). A lock held by a running holder, or by one on another host, is waited
// for up to wait: the OS notification of the lock's removal starts each further attempt, and the deadline is the one
// timer; a wait of 0 refuses at once. A holder that has ended refuses at once. Every refusal is *LockTimeout naming the
// holder; a canceled ctx (a closed connection) ends the wait with its error.
func (c *Corpus) Acquire(ctx context.Context, session string, wait time.Duration) (l *Lock, err error) {
	if wait < 0 {
		return nil, fmt.Errorf("journal: the lock wait %s is negative", wait)
	}
	self, err := c.self(session)
	if err != nil {
		return nil, err
	}
	if err := c.ensureDir(".flowmark"); err != nil {
		return nil, fmt.Errorf("journal: the lock folder: %w", err)
	}
	if _, err := c.lstat(LockPath); err != nil {
		return nil, fmt.Errorf("journal: the lock file: %w", err)
	}
	a := &acquirer{c: c, self: self, record: self.record(), lock: c.abs(LockPath), wait: wait,
		pending: c.abs(".flowmark/lock." + self.Acquisition + pendingSuffix)}
	if err := writeSynced(a.pending, a.record, os.O_EXCL); err != nil {
		return nil, errors.Join(fmt.Errorf("journal: the pending lock record: %w", err), removeAbsent(a.pending))
	}
	defer func() {
		// The pending record takes no part in exclusion once the attempts end.
		if rmErr := os.Remove(a.pending); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("journal: removing the pending lock record: %w", rmErr))
			if l != nil {
				err = errors.Join(err, l.Release())
				l = nil
			}
		}
	}()
	r, err := a.try()
	if err != nil {
		return nil, err
	}
	if r.acquired {
		return a.held(), nil
	}
	if r.state == HolderEnded || wait == 0 {
		return nil, a.timeout(r.holder, r.state)
	}
	return a.waitFor(ctx, r)
}

func (a *acquirer) timeout(h Holder, state HolderState) error {
	return &LockTimeout{Path: a.lock, Corpus: a.c.root, Holder: h, State: state, Wait: a.wait}
}

func (a *acquirer) held() *Lock {
	l := &Lock{c: a.c, path: a.lock, holder: a.self, rec: a.record, held: true}
	a.c.obs.Observe(Event{Kind: LockAcquired, Path: a.lock, Holder: a.self.String()})
	return l
}

// try is one acquisition attempt. A lock that disappears between the failed link and its read is tried again at
// once; a lock that holds this acquirer's own record was linked by this attempt although its reply was lost (REVIEW
// 2-a: its acquisition token is unique, so no other acquisition writes the same record).
func (a *acquirer) try() (tried, error) {
	for {
		if err := a.c.step("lock-attempt", 0); err != nil {
			return tried{}, err
		}
		err := os.Link(a.pending, a.lock)
		if err == nil {
			return tried{acquired: true}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return tried{}, linkError(err)
		}
		data, id, err := readShared(a.lock)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return tried{}, fmt.Errorf("journal: reading the lock: %w", err)
		}
		if bytes.Equal(data, a.record) {
			return tried{acquired: true}, nil
		}
		h, err := parseRecord(data)
		if err != nil {
			return tried{}, fmt.Errorf("journal: %s %w; it is never taken for a free lock: check that no operation runs, then remove it", a.lock, err)
		}
		state, err := a.c.judge(h)
		if err != nil {
			return tried{}, err
		}
		return tried{holder: h, state: state, id: id}, nil
	}
}

// waitFor waits for the release of the lock that r found held. The watch is armed after the failed try and the
// lock is tried once more after arming, so a release between the two is never lost; then each wake the watch reports
// — a removal or rename of the lock, an overflow, an event of the watched folder itself — starts exactly one attempt.
func (a *acquirer) waitFor(ctx context.Context, r tried) (l *Lock, err error) {
	ctx, cancel := context.WithTimeout(ctx, a.wait)
	defer cancel()
	if err := a.c.step("lock-before-arm", 0); err != nil {
		return nil, err
	}
	w, err := a.c.newWatch()
	if err != nil {
		return nil, fmt.Errorf("journal: the wait for %s cannot be armed, so it is refused: %w", a.lock, err)
	}
	defer func() { err = errors.Join(err, w.close()) }()
	var watched Holder
	for {
		retry, err := w.armLock(r.id)
		if err != nil {
			return nil, fmt.Errorf("journal: watching %s: %w", a.lock, err)
		}
		if !retry {
			if r.holder != watched {
				if err := w.armHolder(r.holder); err != nil {
					return nil, fmt.Errorf("journal: watching the holder %s: %w", r.holder, err)
				}
				// Judged again after arming, so a holder that ended meanwhile, or a reused PID, is never watched.
				state, err := a.c.judge(r.holder)
				if err != nil {
					return nil, err
				}
				if state == HolderEnded {
					return nil, a.timeout(r.holder, state)
				}
				watched = r.holder
				a.c.obs.Observe(Event{Kind: LockWaiting, Path: a.lock, Holder: r.holder.String()})
			}
			attempt, err := a.block(ctx, w, r)
			if err != nil {
				return nil, err
			}
			if !attempt {
				continue
			}
		}
		if r, err = a.try(); err != nil {
			return nil, err
		}
		if r.acquired {
			return a.held(), nil
		}
		if r.state == HolderEnded {
			return nil, a.timeout(r.holder, r.state)
		}
	}
}

// block waits for one wake and reports whether it starts an attempt. The exit of the watched holder starts none: it
// judges the holder again, and an ended holder whose record the lock still holds refuses at once.
func (a *acquirer) block(ctx context.Context, w lockWatch, r tried) (bool, error) {
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return false, a.timeout(r.holder, r.state)
		}
		return false, fmt.Errorf("journal: the wait for %s held by %s ended: %w", a.lock, r.holder, ctx.Err())
	case wk := <-w.wakes():
		switch wk.kind {
		case wakeError:
			return false, fmt.Errorf("journal: the watch of %s failed: %w", a.lock, wk.err)
		case wakeLock:
			return true, nil
		}
		data, _, err := readShared(a.lock)
		if errors.Is(err, fs.ErrNotExist) {
			// Released: the removal's own wake starts the attempt.
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("journal: reading the lock: %w", err)
		}
		if !bytes.Equal(data, r.holder.record()) {
			return false, nil
		}
		state, err := a.c.judge(r.holder)
		if err != nil {
			return false, err
		}
		if state == HolderEnded {
			return false, a.timeout(r.holder, state)
		}
		return false, nil
	}
}

// Release removes the lock file after checking that it still holds this lock's record. Releasing a released lock does
// nothing. The removal is not synced: a power loss ends the holder anyway.
func (l *Lock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held {
		return nil
	}
	if _, err := l.c.lstat(LockPath); err != nil {
		return fmt.Errorf("journal: the lock file: %w", err)
	}
	data, _, err := readShared(l.path)
	if err != nil {
		return fmt.Errorf("journal: %s was removed while %s held it: %w", l.path, l.holder, err)
	}
	if !bytes.Equal(data, l.rec) {
		return fmt.Errorf("journal: %s was replaced while %s held it; it now holds %q and is left in place", l.path, l.holder, data)
	}
	if err := os.Remove(l.path); err != nil {
		return fmt.Errorf("journal: removing the lock: %w", err)
	}
	l.held = false
	l.c.obs.Observe(Event{Kind: LockReleased, Path: l.path, Holder: l.holder.String()})
	return nil
}

// checkHeld accepts only a held lock of this corpus.
func (c *Corpus) checkHeld(l *Lock) error {
	if l == nil {
		return errors.New("journal: no lock; acquire the corpus lock first")
	}
	if l.c != c {
		return fmt.Errorf("journal: the lock %s belongs to another corpus than %s", l.path, c.root)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held {
		return fmt.Errorf("journal: the lock %s of %s is released; acquire it again", l.path, l.holder)
	}
	return nil
}

// Unlock is `flowmark unlock <corpus> [--holder <pid>@<host>/<session>]`: it removes the lock of a holder that has
// ended. A running holder is refused; a holder on another host only when holder names it. The lock is first linked to
// a claim named by its record's hash, so only one unlock of that holder proceeds, and the claim is read back: a record
// other than the one judged means a new holder took the lock, and the unlock is refused. Only the holder and the one
// unlock that holds the claim ever remove the lock, so between the claim and the removal the lock still names the
// judged record.
func (c *Corpus) Unlock(holder string) (Holder, error) {
	if _, err := c.lstat(LockPath); err != nil {
		return Holder{}, fmt.Errorf("journal: the lock file: %w", err)
	}
	lock := c.abs(LockPath)
	data, _, err := readShared(lock)
	if errors.Is(err, fs.ErrNotExist) {
		return Holder{}, fmt.Errorf("journal: %s is not held; there is nothing to unlock", lock)
	}
	if err != nil {
		return Holder{}, fmt.Errorf("journal: reading the lock: %w", err)
	}
	h, err := parseRecord(data)
	if err != nil {
		return Holder{}, fmt.Errorf("journal: %s %w; check that no operation runs, then remove it", lock, err)
	}
	state, err := c.judge(h)
	if err != nil {
		return Holder{}, err
	}
	switch {
	case state == HolderRunning:
		return Holder{}, fmt.Errorf("journal: %s is held by %s, which runs; it releases the lock when its operation ends", lock, h.describe())
	case state == HolderUnknown && holder != h.String():
		return Holder{}, fmt.Errorf("journal: %s is held by %s on another host, whose process cannot be checked here; once it has ended, unlock with --holder %s", lock, h.describe(), h)
	case holder != "" && holder != h.String():
		return Holder{}, fmt.Errorf("journal: --holder names %s, but %s is held by %s", holder, lock, h)
	}
	if err := c.step("unlock-read", 0); err != nil {
		return Holder{}, err
	}
	claim := c.abs(".flowmark/lock." + sum(data)[:32] + clearingSuffix)
	if err := os.Link(lock, claim); err != nil {
		switch {
		case errors.Is(err, fs.ErrExist):
			return Holder{}, fmt.Errorf("journal: %s exists: another unlock of %s runs or was interrupted; check that no unlock runs, then remove it", claim, h)
		case errors.Is(err, fs.ErrNotExist):
			return Holder{}, fmt.Errorf("journal: %s was released while it was unlocked; nothing was removed", lock)
		}
		return Holder{}, linkError(err)
	}
	got, _, err := readShared(claim)
	if err != nil {
		return Holder{}, errors.Join(fmt.Errorf("journal: reading the claim %s: %w", claim, err), os.Remove(claim))
	}
	if !bytes.Equal(got, data) {
		return Holder{}, errors.Join(fmt.Errorf("journal: a new holder took %s while %s was unlocked; it is left in place", lock, h), os.Remove(claim))
	}
	if err := os.Remove(lock); err != nil {
		return Holder{}, fmt.Errorf("journal: removing the lock: %w; the claim %s stays, and is removed after checking that no unlock runs", err, claim)
	}
	if err := c.step("unlock-removed", 0); err != nil {
		return Holder{}, err
	}
	if err := os.Remove(claim); err != nil {
		return Holder{}, fmt.Errorf("journal: the lock is removed, but its claim %s stays: %w", claim, err)
	}
	c.obs.Observe(Event{Kind: LockCleared, Path: lock, Holder: h.String()})
	return h, nil
}

// removeEndedPendings removes the pending records of ended holders, on opening. A running or unknown holder's record
// is a waiter's and stays; a record that does not parse stays, since its writer may still be writing it; a claim of
// an unlock is never touched.
func (c *Corpus) removeEndedPendings() error {
	entries, err := os.ReadDir(c.abs(".flowmark"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	for _, e := range entries {
		if !pendingName.MatchString(e.Name()) {
			continue
		}
		p := c.abs(".flowmark/" + e.Name())
		if e.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("journal: %w", symlinkError(p))
		}
		data, _, err := readShared(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("journal: %w", err)
		}
		h, err := parseRecord(data)
		if err != nil {
			continue
		}
		state, err := c.judge(h)
		if err != nil {
			return err
		}
		if state == HolderEnded {
			if err := removeAbsent(p); err != nil {
				return fmt.Errorf("journal: %w", err)
			}
		}
	}
	return nil
}
