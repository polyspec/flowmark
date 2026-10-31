//go:build windows

package journal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// watchFactory arms the lock waits of the corpus c: on Windows each has its own folder handle, so they share nothing.
func watchFactory(c *Corpus) func() (lockWatch, error) {
	return func() (lockWatch, error) { return newLockWatch(c) }
}

// windowsWatch watches `.flowmark/` with an overlapped ReadDirectoryChangesW (REVIEW 1-c), and the holder's process
// handle; WaitForMultipleObjects waits for the change event, the process, a re-arm and the close together.
type windowsWatch struct {
	dir    windows.Handle
	ov     windows.Overlapped
	buf    []byte
	ch     chan wake
	cancel windows.Handle
	rearm  windows.Handle
	done   chan struct{}
	armed  bool

	// mu guards proc and retired. proc is the holder's process handle that run waits on; retired holds the handles that
	// armHolder replaced, which run closes at the top of its loop, outside every wait.
	mu      sync.Mutex
	proc    windows.Handle
	retired []windows.Handle
}

// watchArmError names the cause of a watch that cannot be armed.
func watchArmError(err error) error {
	if errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
		return fmt.Errorf("the file system sends no change notifications (ReadDirectoryChangesW: ERROR_INVALID_FUNCTION), so a lock wait cannot be armed; it is not supported: %w", err)
	}
	return fmt.Errorf("ReadDirectoryChangesW: %w", err)
}

// openWatchedDir opens the folder path for overlapped ReadDirectoryChangesW.
func openWatchedDir(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	dir, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY, shareAll, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", path, err)
	}
	return dir, nil
}

func newLockWatch(c *Corpus) (lockWatch, error) {
	dir, err := openWatchedDir(c.abs(".flowmark"))
	if err != nil {
		return nil, err
	}
	w := &windowsWatch{dir: dir, buf: make([]byte, 64*1024), ch: make(chan wake, wakeBuffer), done: make(chan struct{})}
	var errs []error
	if w.ov.HEvent, err = windows.CreateEvent(nil, 1, 0, nil); err != nil {
		errs = append(errs, err)
	}
	if w.cancel, err = windows.CreateEvent(nil, 0, 0, nil); err != nil {
		errs = append(errs, err)
	}
	if w.rearm, err = windows.CreateEvent(nil, 0, 0, nil); err != nil {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		if err := w.issue(); err != nil {
			errs = append(errs, watchArmError(err))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(append(errs, w.closeHandles())...)
	}
	go w.run()
	return w, nil
}

func (w *windowsWatch) issue() error {
	return windows.ReadDirectoryChanges(w.dir, &w.buf[0], uint32(len(w.buf)), false, windows.FILE_NOTIFY_CHANGE_FILE_NAME, nil, &w.ov, 0)
}

func (w *windowsWatch) wakes() <-chan wake { return w.ch }

func (w *windowsWatch) run() {
	defer close(w.done)
	for {
		// The retired handles leave the lock here, so the snapshot below holds none of them: a handle is closed only
		// between two waits, never while a wait may still hold it.
		w.mu.Lock()
		retired := w.retired
		w.retired = nil
		handles := []windows.Handle{w.ov.HEvent, w.cancel, w.rearm}
		if w.proc != 0 {
			handles = append(handles, w.proc)
		}
		w.mu.Unlock()
		if err := closeRetired(retired); err != nil {
			send(w.ch, wake{kind: wakeError, err: fmt.Errorf("closing the holder's process handle: %w", err)})
			return
		}
		r, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
		if err != nil {
			send(w.ch, wake{kind: wakeError, err: fmt.Errorf("WaitForMultipleObjects: %w", err)})
			return
		}
		switch r - windows.WAIT_OBJECT_0 {
		case 0:
			var n uint32
			err := windows.GetOverlappedResult(w.dir, &w.ov, &n, false)
			switch {
			case errors.Is(err, windows.ERROR_NOTIFY_ENUM_DIR):
				// The changes overflowed the buffer: one more attempt.
				send(w.ch, wake{kind: wakeLock})
			case err != nil:
				send(w.ch, wake{kind: wakeError, err: fmt.Errorf("ReadDirectoryChangesW: %w", err)})
				return
			default:
				for range notifyWakes(w.buf, n) {
					send(w.ch, wake{kind: wakeLock})
				}
			}
			if err := w.issue(); err != nil {
				send(w.ch, wake{kind: wakeError, err: watchArmError(err)})
				return
			}
		case 1:
			var n uint32
			if err := windows.CancelIoEx(w.dir, &w.ov); err != nil {
				send(w.ch, wake{kind: wakeError, err: fmt.Errorf("CancelIoEx: %w", err)})
				return
			}
			// Wait for the canceled read to complete before its buffer is released; it ends as aborted.
			if err := windows.GetOverlappedResult(w.dir, &w.ov, &n, true); err != nil && !errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
				send(w.ch, wake{kind: wakeError, err: fmt.Errorf("ReadDirectoryChangesW: %w", err)})
			}
			return
		case 2:
		case 3:
			// The exited handle is the one in the snapshot; if armHolder has replaced it since, it is already retired.
			exited := handles[3]
			send(w.ch, wake{kind: wakeHolderExit})
			w.mu.Lock()
			if w.proc == exited {
				w.proc = 0
				w.retired = append(w.retired, exited)
			}
			w.mu.Unlock()
		}
	}
}

// notifyWakes reads a ReadDirectoryChangesW buffer of n bytes: each removal or rename of the lock is one wake; zero
// bytes is an overflow, one wake; so is a buffer whose entries run past n, since its events cannot be read.
func notifyWakes(buf []byte, n uint32) int {
	wakes := 0
	whole := notifyEntries(buf, n, func(action uint32, name string) {
		if (action == windows.FILE_ACTION_REMOVED || action == windows.FILE_ACTION_RENAMED_OLD_NAME) && name == "lock" {
			wakes++
		}
	})
	if !whole {
		wakes++
	}
	return wakes
}

// notifyEntries reads a ReadDirectoryChangesW buffer of n bytes and calls f with the action and the name of each
// entry, in order; the lock watch and the corpus tree's watch both read their changes through it. It reports false
// when the buffer is an overflow: zero bytes, or entries that run past n, whose events cannot be read.
func notifyEntries(buf []byte, n uint32, f func(action uint32, name string)) bool {
	if n == 0 {
		return false
	}
	for off := 0; ; {
		if off+12 > int(n) {
			return false
		}
		next := int(binary.LittleEndian.Uint32(buf[off:]))
		action := binary.LittleEndian.Uint32(buf[off+4:])
		size := int(binary.LittleEndian.Uint32(buf[off+8:]))
		if off+12+size > int(n) {
			return false
		}
		units := make([]uint16, size/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(buf[off+12+2*i:])
		}
		f(action, string(utf16.Decode(units)))
		if next == 0 {
			return true
		}
		off += next
	}
}

func (w *windowsWatch) armLock(fileID) (bool, error) {
	if w.armed {
		return false, nil
	}
	w.armed = true
	return true, nil
}

// armHolder opens the holder's process with SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION; a process that is gone
// reports its exit at once. A process whose handle this user may not open is not watched: its exit is then learned
// from the lock's removal alone, or the wait ends at its deadline.
func (w *windowsWatch) armHolder(h Holder) error {
	proc, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(h.PID))
	switch {
	case errors.Is(err, windows.ERROR_INVALID_PARAMETER):
		send(w.ch, wake{kind: wakeHolderExit})
		proc = 0
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		proc = 0
	case err != nil:
		return fmt.Errorf("OpenProcess %d: %w", h.PID, err)
	}
	// The replaced handle is not closed here: run's wait may hold it. It joins retired, and run closes it at the top of its
	// loop, after the wait that holds it has returned.
	w.mu.Lock()
	if w.proc != 0 {
		w.retired = append(w.retired, w.proc)
	}
	w.proc = proc
	w.mu.Unlock()
	return windows.SetEvent(w.rearm)
}

// closeRetired closes the handles that armHolder replaced. Only run calls it, between two waits.
func closeRetired(handles []windows.Handle) error {
	var errs []error
	for _, h := range handles {
		errs = append(errs, windows.CloseHandle(h))
	}
	return errors.Join(errs...)
}

func (w *windowsWatch) close() error {
	err := windows.SetEvent(w.cancel)
	if err == nil {
		<-w.done
	}
	return errors.Join(err, w.closeHandles())
}

// closeHandles closes every handle of the watch, the replaced ones among them; it runs once the watch has stopped.
func (w *windowsWatch) closeHandles() error {
	w.mu.Lock()
	retired := w.retired
	w.retired = nil
	w.mu.Unlock()
	var errs []error
	for _, h := range []windows.Handle{w.dir, w.ov.HEvent, w.cancel, w.rearm, w.proc} {
		if h != 0 {
			errs = append(errs, windows.CloseHandle(h))
		}
	}
	return errors.Join(append(errs, closeRetired(retired))...)
}
