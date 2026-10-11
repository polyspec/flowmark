//go:build linux

package journal

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"golang.org/x/sys/unix"
)

// treeFolderMask arms the watch of each folder the corpus tree reads: the entry events of its names, the changes of
// the files in it, and the events of the folder itself.
const treeFolderMask = unix.IN_CREATE | unix.IN_DELETE | unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_MODIFY |
	unix.IN_CLOSE_WRITE | unix.IN_ATTRIB | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF | unix.IN_UNMOUNT |
	unix.IN_ONLYDIR | unix.IN_DONT_FOLLOW | unix.IN_EXCL_UNLINK

// treeFileMask arms the watch of each document the corpus tree reads: an inode event reaches every watch of the
// inode, so a write through another hard link of the document is reported too, which its folder's watch never sees.
const treeFileMask = unix.IN_MODIFY | unix.IN_CLOSE_WRITE | unix.IN_ATTRIB | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF |
	unix.IN_UNMOUNT | unix.IN_DONT_FOLLOW

// treeEntryEvents are the events that add, remove or rename a name of a watched folder.
const treeEntryEvents = unix.IN_CREATE | unix.IN_DELETE | unix.IN_MOVED_FROM | unix.IN_MOVED_TO

// linuxTree watches each folder and each document the corpus tree reads with one inotify instance of its own, read
// through the parser of the lock watch (inotifyEvents). It is never read by a goroutine: drain reads it without
// waiting, and the kernel queues an event in the system call that makes the change. A queue that overflows
// (IN_Q_OVERFLOW, past fs.inotify.max_queued_events) is reported as an overflow.
//
// inotify gives one watch descriptor per inode, so two paths of one inode — a folder renamed before its old path's
// event was taken, or two hard links of a document — share it; a descriptor holds every path watched through it, and
// is removed when its last path is.
type linuxTree struct {
	c      *Corpus
	fd     int
	byWD   map[int]*inodeWatch
	byPath map[string]int
	buf    []byte
}

// inodeWatch is a watch descriptor: whether it watches a folder, and the paths watched through it.
type inodeWatch struct {
	dir   bool
	paths []string
}

func newTreeWatch(c *Corpus) (treeWatch, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, inotifyArmError(err)
	}
	return &linuxTree{c: c, fd: fd, byWD: map[int]*inodeWatch{}, byPath: map[string]int{}, buf: make([]byte, 64*1024)}, nil
}

// watch watches the folder rel, or the document rel and its folder.
func (w *linuxTree) watch(rel string, dir bool) error {
	if !dir {
		if err := w.add(path.Dir(rel), true); err != nil {
			return err
		}
	}
	return w.add(rel, dir)
}

func (w *linuxTree) add(rel string, dir bool) error {
	if _, ok := w.byPath[rel]; ok {
		return nil
	}
	mask := uint32(treeFileMask)
	if dir {
		mask = treeFolderMask
	}
	wd, err := unix.InotifyAddWatch(w.fd, w.c.abs(rel), mask)
	switch {
	case errors.Is(err, unix.ENOENT), errors.Is(err, unix.ENOTDIR), errors.Is(err, unix.EACCES):
		return pathError("inotify_add_watch", rel, err)
	case err != nil:
		return inotifyArmError(err)
	}
	iw := w.byWD[wd]
	if iw == nil {
		iw = &inodeWatch{dir: dir}
		w.byWD[wd] = iw
	}
	iw.paths = append(iw.paths, rel)
	w.byPath[rel] = wd
	return nil
}

func (w *linuxTree) drain() (Notices, error) {
	var n Notices
	for {
		k, err := unix.Read(w.fd, w.buf)
		switch {
		case errors.Is(err, unix.EAGAIN):
			return n, nil
		case errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			return n, fmt.Errorf("inotify: %w", err)
		}
		var errs []error
		perr := inotifyEvents(w.buf[:k], func(wd int, mask uint32, name string) {
			if mask&unix.IN_Q_OVERFLOW != 0 {
				n.Overflow = true
				return
			}
			iw, ok := w.byWD[wd]
			if !ok {
				return
			}
			for _, p := range slices.Clone(iw.paths) {
				switch {
				case mask&inotifySelf != 0:
					n.Moved = append(n.Moved, p)
					errs = append(errs, w.unwatchUnder(p))
				case iw.dir && name != "" && mask&treeEntryEvents != 0:
					q := path.Join(p, name)
					n.Moved = append(n.Moved, q)
					errs = append(errs, w.unwatchUnder(q))
				case iw.dir && name != "":
					n.Changed = append(n.Changed, path.Join(p, name))
				default:
					n.Changed = append(n.Changed, p)
				}
			}
		})
		if err := errors.Join(append(errs, perr)...); err != nil {
			return n, err
		}
	}
}

// unwatch removes the path rel from its watch descriptor, and the descriptor with its last path; a watch the kernel
// already removed (IN_IGNORED) is only forgotten.
func (w *linuxTree) unwatch(rel string) error {
	wd, ok := w.byPath[rel]
	if !ok {
		return nil
	}
	delete(w.byPath, rel)
	iw := w.byWD[wd]
	iw.paths = slices.DeleteFunc(iw.paths, func(p string) bool { return p == rel })
	if len(iw.paths) > 0 {
		return nil
	}
	delete(w.byWD, wd)
	if _, err := unix.InotifyRmWatch(w.fd, uint32(wd)); err != nil && !errors.Is(err, unix.EINVAL) {
		return fmt.Errorf("inotify_rm_watch: %w", err)
	}
	return nil
}

// unwatchUnder removes the watches of rel and of every path under it, which moved or went away.
func (w *linuxTree) unwatchUnder(rel string) error {
	var errs []error
	for p := range w.byPath {
		if p == rel || rel == "." || strings.HasPrefix(p, rel+"/") {
			errs = append(errs, w.unwatch(p))
		}
	}
	return errors.Join(errs...)
}

func (w *linuxTree) close() error {
	return unix.Close(w.fd)
}
