//go:build linux

package journal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// inotifyMask arms the watch of `.flowmark/` (REVIEW 1-c).
const inotifyMask = unix.IN_DELETE | unix.IN_MOVED_FROM | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF | unix.IN_UNMOUNT

// watchShare is the one inotify instance that the lock waits of a corpus share (a server's sessions).
type watchShare struct {
	mu   sync.Mutex
	hub  *inotifyHub
	refs int
}

// inotifyHub reads the shared inotify instance and hands each wake to every subscribed wait.
type inotifyHub struct {
	dir  string
	fd   int
	f    *os.File
	done chan struct{}
	mu   sync.Mutex
	subs map[chan wake]struct{}
}

// inotifyArmError names the cause of a watch that cannot be armed.
func inotifyArmError(err error) error {
	switch {
	case errors.Is(err, unix.EMFILE):
		return fmt.Errorf("inotify: the limit of inotify instances is reached; raise fs.inotify.max_user_instances: %w", err)
	case errors.Is(err, unix.ENOSPC):
		return fmt.Errorf("inotify: the limit of inotify watches is reached; raise fs.inotify.max_user_watches: %w", err)
	}
	return fmt.Errorf("inotify: %w", err)
}

func startHub(dir string) (*inotifyHub, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, inotifyArmError(err)
	}
	if _, err := unix.InotifyAddWatch(fd, dir, inotifyMask); err != nil {
		return nil, errors.Join(inotifyArmError(err), unix.Close(fd))
	}
	h := &inotifyHub{dir: dir, fd: fd, f: os.NewFile(uintptr(fd), "inotify"), done: make(chan struct{}), subs: map[chan wake]struct{}{}}
	go h.run()
	return h, nil
}

func (h *inotifyHub) broadcast(w wake) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		send(ch, w)
	}
}

func (h *inotifyHub) run() {
	defer close(h.done)
	buf := make([]byte, 64*1024)
	for {
		n, err := h.f.Read(buf)
		if errors.Is(err, os.ErrClosed) {
			return
		}
		if err != nil {
			h.broadcast(wake{kind: wakeError, err: fmt.Errorf("inotify: %w", err)})
			return
		}
		wakes, ignored, err := inotifyWakes(buf[:n])
		if err != nil {
			h.broadcast(wake{kind: wakeError, err: err})
			return
		}
		for range wakes {
			h.broadcast(wake{kind: wakeLock})
		}
		if ignored {
			// The watch of `.flowmark/` is gone (the folder was removed or unmounted); it is armed again on the folder
			// that now has its name, and a folder that is absent ends the waits with that cause.
			if _, err := unix.InotifyAddWatch(h.fd, h.dir, inotifyMask); err != nil {
				h.broadcast(wake{kind: wakeError, err: inotifyArmError(err)})
				return
			}
		}
	}
}

// inotifyWakes reads a buffer of inotify events: each removal or rename of the lock, each overflow and each event of
// the watched folder itself (removal, rename, unmount, IN_IGNORED) is one wake; ignored reports IN_IGNORED.
func inotifyWakes(buf []byte) (wakes int, ignored bool, err error) {
	const self = unix.IN_IGNORED | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF | unix.IN_UNMOUNT
	for off := 0; off < len(buf); {
		if len(buf)-off < unix.SizeofInotifyEvent {
			return 0, false, fmt.Errorf("inotify: a truncated event of %d bytes", len(buf)-off)
		}
		mask := binary.NativeEndian.Uint32(buf[off+4:])
		size := int(binary.NativeEndian.Uint32(buf[off+12:]))
		start := off + unix.SizeofInotifyEvent
		if size > len(buf)-start {
			return 0, false, fmt.Errorf("inotify: an event name of %d bytes past the buffer", size)
		}
		name := bytes.TrimRight(buf[start:start+size], "\x00")
		switch {
		case mask&(unix.IN_Q_OVERFLOW|self) != 0:
			wakes++
			ignored = ignored || mask&unix.IN_IGNORED != 0
		case mask&(unix.IN_DELETE|unix.IN_MOVED_FROM) != 0 && string(name) == "lock":
			wakes++
		}
		off = start + size
	}
	return wakes, ignored, nil
}

// linuxWatch is one wait's subscription to the shared inotify instance, and a pidfd of its holder (Linux 5.3+).
type linuxWatch struct {
	c     *Corpus
	ch    chan wake
	armed bool
	mu    sync.Mutex
	pidf  *os.File
}

func newLockWatch(c *Corpus) (lockWatch, error) {
	s := &c.share
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hub == nil {
		hub, err := startHub(c.abs(".flowmark"))
		if err != nil {
			return nil, err
		}
		s.hub = hub
	}
	ch := make(chan wake, wakeBuffer)
	s.hub.mu.Lock()
	s.hub.subs[ch] = struct{}{}
	s.hub.mu.Unlock()
	s.refs++
	return &linuxWatch{c: c, ch: ch}, nil
}

func (w *linuxWatch) wakes() <-chan wake { return w.ch }

// armLock reports one change on its first call: the watch of the folder was armed after the failed try, so one more
// attempt covers a release between the two. Later removals arrive as wakes.
func (w *linuxWatch) armLock(fileID) (bool, error) {
	if w.armed {
		return false, nil
	}
	w.armed = true
	return true, nil
}

// armHolder opens a pidfd of the holder; it becomes readable when the process exits, and the runtime's poller waits
// for that.
func (w *linuxWatch) armHolder(h Holder) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pidf != nil {
		if err := w.pidf.Close(); err != nil {
			return err
		}
		w.pidf = nil
	}
	fd, err := unix.PidfdOpen(h.PID, unix.PIDFD_NONBLOCK)
	switch {
	case errors.Is(err, unix.ESRCH):
		send(w.ch, wake{kind: wakeHolderExit})
		return nil
	case errors.Is(err, unix.ENOSYS):
		return fmt.Errorf("pidfd_open: the holder-exit watch needs Linux 5.3 or later: %w", err)
	case err != nil:
		return fmt.Errorf("pidfd_open: %w", err)
	}
	f := os.NewFile(uintptr(fd), "pidfd")
	rc, err := f.SyscallConn()
	if err != nil {
		return errors.Join(err, f.Close())
	}
	w.pidf = f
	go func() {
		err := rc.Read(func(fd uintptr) bool {
			n, err := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, 0)
			return err == nil && n > 0
		})
		// A closed pidfd ends the read with an error: the watch moved to another holder or closed.
		if err == nil {
			send(w.ch, wake{kind: wakeHolderExit})
		}
	}()
	return nil
}

func (w *linuxWatch) close() error {
	w.mu.Lock()
	var err error
	if w.pidf != nil {
		err = w.pidf.Close()
		w.pidf = nil
	}
	w.mu.Unlock()
	s := &w.c.share
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hub.mu.Lock()
	delete(s.hub.subs, w.ch)
	s.hub.mu.Unlock()
	s.refs--
	if s.refs == 0 {
		err = errors.Join(err, s.hub.f.Close())
		<-s.hub.done
		s.hub = nil
	}
	return err
}
