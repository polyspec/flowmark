//go:build darwin

package journal

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

// The vnode events of a watched file: its bytes or size changed, or its name went away. Of a watched folder: its
// entries changed (NOTE_WRITE), or its own name went away.
const (
	fileNotes   = unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_REVOKE
	folderNotes = unix.NOTE_WRITE | unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_REVOKE
	folderGone  = unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_REVOKE
)

// darwinTree watches each folder and file the corpus tree reads with kqueue EVFILT_VNODE, as the lock watch watches
// the lock (watch_darwin.go): the path opened with O_EVTONLY, registered with EV_CLEAR. kqueue keeps one state per
// registration and coalesces its events, so it never loses one and reports no overflow. A file's registration reports
// its path once and is then removed; the read that follows the drop arms it again. A folder's registration stays until
// the folder itself goes away, so a rename of a folder is always reported and drops what the tree holds under it. It
// takes one descriptor per folder and file held.
type darwinTree struct {
	c      *Corpus
	kq     int
	byFD   map[int]watchedPath
	byPath map[string]int
}

type watchedPath struct {
	rel string
	dir bool
}

func newTreeWatch(c *Corpus) (treeWatch, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("kqueue: %w", err)
	}
	unix.CloseOnExec(kq)
	return &darwinTree{c: c, kq: kq, byFD: map[int]watchedPath{}, byPath: map[string]int{}}, nil
}

// descriptorError names the cause of a descriptor that cannot be opened for a watch.
func descriptorError(err error) error {
	if errors.Is(err, unix.EMFILE) || errors.Is(err, unix.ENFILE) {
		return fmt.Errorf("the limit of open files is reached; the corpus watch holds one descriptor for each folder and document of the corpus, so raise the limit (ulimit -n, kern.maxfilesperproc): %w", err)
	}
	return err
}

func (w *darwinTree) watch(rel string, dir bool) error {
	if _, ok := w.byPath[rel]; ok {
		return nil
	}
	flags := unix.O_EVTONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	notes := uint32(fileNotes)
	if dir {
		flags |= unix.O_DIRECTORY
		notes = folderNotes
	}
	fd, err := unix.Open(w.c.abs(rel), flags, 0)
	switch {
	case errors.Is(err, unix.ENOENT), errors.Is(err, unix.ELOOP), errors.Is(err, unix.ENOTDIR), errors.Is(err, unix.EACCES):
		return pathError("open", rel, err)
	case err != nil:
		return fmt.Errorf("open %s: %w", w.c.abs(rel), descriptorError(err))
	}
	ev := []unix.Kevent_t{{Ident: uint64(fd), Filter: unix.EVFILT_VNODE, Flags: unix.EV_ADD | unix.EV_CLEAR, Fflags: notes}}
	if _, err := unix.Kevent(w.kq, ev, nil, nil); err != nil {
		return errors.Join(fmt.Errorf("kqueue EVFILT_VNODE: %w", err), unix.Close(fd))
	}
	w.byFD[fd] = watchedPath{rel: rel, dir: dir}
	w.byPath[rel] = fd
	return nil
}

func (w *darwinTree) drain() (Notices, error) {
	var n Notices
	events := make([]unix.Kevent_t, 256)
	zero := unix.Timespec{}
	for {
		k, err := unix.Kevent(w.kq, nil, events, &zero)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return n, fmt.Errorf("kevent: %w", err)
		}
		for _, ev := range events[:k] {
			p, ok := w.byFD[int(ev.Ident)]
			if !ok {
				continue
			}
			switch {
			case p.dir && ev.Fflags&folderGone != 0:
				n.Moved = append(n.Moved, p.rel)
				if err := errors.Join(w.unwatchUnder(p.rel), w.unwatch(p.rel)); err != nil {
					return n, err
				}
			case p.dir:
				// The folder's entries changed; it stays watched, so a later rename of it is reported and drops
				// what the tree holds under it.
				n.Changed = append(n.Changed, p.rel)
			default:
				n.Changed = append(n.Changed, p.rel)
				if err := w.unwatch(p.rel); err != nil {
					return n, err
				}
			}
		}
		if k < len(events) {
			return n, nil
		}
	}
}

// unwatch removes the registration of rel; closing its descriptor removes it from the kqueue.
func (w *darwinTree) unwatch(rel string) error {
	fd, ok := w.byPath[rel]
	if !ok {
		return nil
	}
	delete(w.byPath, rel)
	delete(w.byFD, fd)
	return unix.Close(fd)
}

// unwatchUnder removes the registrations of the paths under the folder rel, which moved or went away.
func (w *darwinTree) unwatchUnder(rel string) error {
	var errs []error
	for p := range w.byPath {
		if rel == "." || strings.HasPrefix(p, rel+"/") {
			errs = append(errs, w.unwatch(p))
		}
	}
	return errors.Join(errs...)
}

func (w *darwinTree) close() error {
	var errs []error
	for p := range w.byPath {
		errs = append(errs, w.unwatch(p))
	}
	return errors.Join(append(errs, unix.Close(w.kq))...)
}
