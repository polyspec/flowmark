//go:build windows

package journal

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// treeFilter is what the corpus tree's ReadDirectoryChangesW reports: names of files and folders added, removed or
// renamed, and files whose last write time or size changed.
const treeFilter = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_LAST_WRITE | windows.FILE_NOTIFY_CHANGE_SIZE

// windowsTree watches the corpus root and its subtree with one overlapped ReadDirectoryChangesW, opened as the lock
// watch opens `.flowmark/` (watch_windows.go) and read through its parser (notifyEntries). It is never read by a
// goroutine: drain takes each completed read without waiting and issues the next. Zero bytes, a buffer whose entries
// cannot be read, or ERROR_NOTIFY_ENUM_DIR is an overflow. The file system reports a write when it records the last
// write time, which may come after the write returned; the write path reads its commit targets again under the lock
// for that reason.
type windowsTree struct {
	root string
	dir  windows.Handle
	ov   windows.Overlapped
	buf  []byte
}

func newTreeWatch(c *Corpus) (treeWatch, error) {
	name, err := windows.UTF16PtrFromString(c.root)
	if err != nil {
		return nil, err
	}
	dir, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY, shareAll, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", c.root, err)
	}
	w := &windowsTree{root: c.root, dir: dir, buf: make([]byte, 64*1024)}
	if w.ov.HEvent, err = windows.CreateEvent(nil, 1, 0, nil); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(dir))
	}
	if err := w.issue(); err != nil {
		return nil, errors.Join(watchArmError(err), w.closeHandles())
	}
	return w, nil
}

func (w *windowsTree) issue() error {
	return windows.ReadDirectoryChanges(w.dir, &w.buf[0], uint32(len(w.buf)), true, treeFilter, nil, &w.ov, 0)
}

// watch arms nothing: the subtree of the root is watched from the start.
func (w *windowsTree) watch(string, bool) error { return nil }

func (w *windowsTree) drain() (Notices, error) {
	var n Notices
	for {
		var k uint32
		err := windows.GetOverlappedResult(w.dir, &w.ov, &k, false)
		switch {
		case errors.Is(err, windows.ERROR_IO_INCOMPLETE):
			return n, nil
		case errors.Is(err, windows.ERROR_NOTIFY_ENUM_DIR):
			n.Overflow = true
		case err != nil:
			return Notices{}, fmt.Errorf("ReadDirectoryChangesW: %w", err)
		default:
			whole := notifyEntries(w.buf, k, func(action uint32, name string) {
				p, ok := w.longName(name)
				if !ok {
					// A short (8.3) name whose long name cannot be found names no path the tree holds; what it
					// changed is unknown, so it is taken as an overflow.
					n.Overflow = true
					return
				}
				if action == windows.FILE_ACTION_MODIFIED {
					n.Changed = append(n.Changed, p)
				} else {
					n.Moved = append(n.Moved, p)
				}
			})
			n.Overflow = n.Overflow || !whole
		}
		if err := w.issue(); err != nil {
			return Notices{}, watchArmError(err)
		}
	}
}

// longName returns the corpus path of the name a notification gave, relative to the root. ReadDirectoryChangesW may
// give the short (8.3) name of a file that has one; a name with a component holding `~` is resolved to its long name
// with GetLongPathNameW, and ok is false when that fails (the file is gone) or leads outside the root.
func (w *windowsTree) longName(name string) (string, bool) {
	if !strings.Contains(name, "~") {
		return filepath.ToSlash(name), true
	}
	short, err := windows.UTF16PtrFromString(filepath.Join(w.root, name))
	if err != nil {
		return "", false
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetLongPathName(short, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return "", false
	}
	rel, err := filepath.Rel(w.root, windows.UTF16ToString(buf[:n]))
	if err != nil || !filepath.IsLocal(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func (w *windowsTree) close() error {
	var n uint32
	err := windows.CancelIoEx(w.dir, &w.ov)
	if err == nil {
		// Wait for the canceled read to complete before its buffer is released; it ends as aborted.
		if werr := windows.GetOverlappedResult(w.dir, &w.ov, &n, true); werr != nil && !errors.Is(werr, windows.ERROR_OPERATION_ABORTED) {
			err = fmt.Errorf("ReadDirectoryChangesW: %w", werr)
		}
	} else if errors.Is(err, windows.ERROR_NOT_FOUND) {
		// No read is pending.
		err = nil
	}
	return errors.Join(err, w.closeHandles())
}

func (w *windowsTree) closeHandles() error {
	var errs []error
	for _, h := range []windows.Handle{w.dir, w.ov.HEvent} {
		if h != 0 {
			errs = append(errs, windows.CloseHandle(h))
		}
	}
	return errors.Join(errs...)
}
