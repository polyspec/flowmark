//go:build darwin

package journal

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

// watchFactory arms the lock waits of the corpus c: on macOS each has its own kqueue, so they share nothing.
func watchFactory(c *Corpus) func() (lockWatch, error) {
	return func() (lockWatch, error) { return newLockWatch(c) }
}

// darwinWatch watches the lock file with kqueue (REVIEW 1-a): the lock is opened with O_EVTONLY and registered for
// EVFILT_VNODE NOTE_DELETE | NOTE_RENAME | NOTE_REVOKE with EV_CLEAR, and the holder's exit with EVFILT_PROC NOTE_EXIT
// in the same kqueue. An EVFILT_USER event stops the reader on close.
type darwinWatch struct {
	c    *Corpus
	lock string
	kq   int
	ch   chan wake
	done chan struct{}

	mu  sync.Mutex
	fd  int
	id  fileID
	pid int
	// reported: a change of the watched file was reported; its later events wake nothing, since one removal starts
	// one attempt and the next armLock registers the lock that file was replaced by.
	reported bool
}

func newLockWatch(c *Corpus) (lockWatch, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("kqueue: %w", err)
	}
	unix.CloseOnExec(kq)
	stop := []unix.Kevent_t{{Ident: 0, Filter: unix.EVFILT_USER, Flags: unix.EV_ADD | unix.EV_CLEAR}}
	if _, err := unix.Kevent(kq, stop, nil, nil); err != nil {
		return nil, errors.Join(fmt.Errorf("kqueue EVFILT_USER: %w", err), unix.Close(kq))
	}
	w := &darwinWatch{c: c, lock: c.abs(LockPath), kq: kq, ch: make(chan wake, wakeBuffer), done: make(chan struct{}), fd: -1}
	go w.run()
	return w, nil
}

func (w *darwinWatch) wakes() <-chan wake { return w.ch }

func (w *darwinWatch) run() {
	defer close(w.done)
	events := make([]unix.Kevent_t, 8)
	for {
		n, err := unix.Kevent(w.kq, nil, events, nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			send(w.ch, wake{kind: wakeError, err: fmt.Errorf("kevent: %w", err)})
			return
		}
		for _, ev := range events[:n] {
			switch ev.Filter {
			case unix.EVFILT_USER:
				return
			case unix.EVFILT_PROC:
				send(w.ch, wake{kind: wakeHolderExit})
			case unix.EVFILT_VNODE:
				changed, err := w.reportChange()
				if err != nil {
					send(w.ch, wake{kind: wakeError, err: err})
					return
				}
				woke := 0
				if changed {
					woke = 1
					send(w.ch, wake{kind: wakeLock})
				}
				if err := w.c.step("vnode-event", woke); err != nil {
					send(w.ch, wake{kind: wakeError, err: err})
					return
				}
			}
		}
	}
}

// lockChanged compares the watched file with the lock path: a link count of 0, an absent lock or another device and
// inode mean the lock was removed or replaced. When the lock path still names the watched inode with links left, the
// event came from another name of that file (an acquirer's pending record) and starts no attempt.
// reportChange reports a change of the watched file once.
func (w *darwinWatch) reportChange() (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.reported {
		return false, nil
	}
	changed, err := w.lockChangedLocked()
	w.reported = changed
	return changed, err
}

func (w *darwinWatch) lockChangedLocked() (bool, error) {
	if w.fd < 0 {
		return false, nil
	}
	var watched, current unix.Stat_t
	if err := unix.Fstat(w.fd, &watched); err != nil {
		return false, fmt.Errorf("fstat of the watched lock: %w", err)
	}
	if watched.Nlink == 0 {
		return true, nil
	}
	err := unix.Lstat(w.lock, &current)
	if errors.Is(err, unix.ENOENT) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("lstat %s: %w", w.lock, err)
	}
	return fileID{uint64(current.Dev), current.Ino} != w.id, nil
}

// armLock opens the lock with O_EVTONLY and registers it; a lock that is absent, or is another file than the one the
// try read, starts one more attempt at once. After registering, the watched file is compared with the lock path, so a
// removal between the open and the registration is never lost.
func (w *darwinWatch) armLock(judged fileID) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// The file already watched is kept unless its change was reported: a removed file whose inode number the file
	// system gave to the new lock must be registered again.
	if w.fd >= 0 && w.id == judged && !w.reported {
		return false, nil
	}
	fd, err := unix.Open(w.lock, unix.O_EVTONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("open %s: %w", w.lock, err)
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return false, errors.Join(fmt.Errorf("fstat %s: %w", w.lock, err), unix.Close(fd))
	}
	if id := (fileID{uint64(st.Dev), st.Ino}); id != judged {
		return true, unix.Close(fd)
	}
	if w.fd >= 0 {
		// Closing a descriptor removes its registration.
		if err := unix.Close(w.fd); err != nil {
			return false, errors.Join(err, unix.Close(fd))
		}
	}
	w.fd, w.id, w.reported = fd, judged, false
	vnode := []unix.Kevent_t{{Ident: uint64(fd), Filter: unix.EVFILT_VNODE, Flags: unix.EV_ADD | unix.EV_CLEAR,
		Fflags: unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_REVOKE}}
	if _, err := unix.Kevent(w.kq, vnode, nil, nil); err != nil {
		return false, fmt.Errorf("kqueue EVFILT_VNODE: %w", err)
	}
	// A change found here starts the attempt at once; later events of this file wake nothing more.
	changed, err := w.lockChangedLocked()
	w.reported = changed
	return changed, err
}

// armHolder registers EVFILT_PROC NOTE_EXIT for the holder; a process that is already gone reports its exit at once.
func (w *darwinWatch) armHolder(h Holder) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pid != 0 {
		old := []unix.Kevent_t{{Ident: uint64(w.pid), Filter: unix.EVFILT_PROC, Flags: unix.EV_DELETE}}
		// The registration of a process that has exited is already gone.
		if _, err := unix.Kevent(w.kq, old, nil, nil); err != nil && !errors.Is(err, unix.ENOENT) && !errors.Is(err, unix.ESRCH) {
			return fmt.Errorf("kqueue EVFILT_PROC delete: %w", err)
		}
		w.pid = 0
	}
	proc := []unix.Kevent_t{{Ident: uint64(h.PID), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT}}
	_, err := unix.Kevent(w.kq, proc, nil, nil)
	if errors.Is(err, unix.ESRCH) {
		send(w.ch, wake{kind: wakeHolderExit})
		return nil
	}
	if err != nil {
		return fmt.Errorf("kqueue EVFILT_PROC: %w", err)
	}
	w.pid = h.PID
	return nil
}

func (w *darwinWatch) close() error {
	trigger := []unix.Kevent_t{{Ident: 0, Filter: unix.EVFILT_USER, Fflags: unix.NOTE_TRIGGER}}
	_, err := unix.Kevent(w.kq, trigger, nil, nil)
	if err == nil {
		<-w.done
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fd >= 0 {
		err = errors.Join(err, unix.Close(w.fd))
		w.fd = -1
	}
	return errors.Join(err, unix.Close(w.kq))
}
