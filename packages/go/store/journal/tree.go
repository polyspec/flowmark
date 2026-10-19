package journal

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/polyspec/flowmark/packages/go/store"
)

// Tree is the corpus as the operations of a long-lived corpus read it (spec 40 Opening a corpus): a mirror of the
// folders and files that the reads asked for, kept current by the operating system's file notifications, so an
// operation reads from the disk only what changed since the last one. A folder or file is watched before it is read,
// so no change after its read is missed. Each operation calls Refresh under the lock first: it takes every
// notification queued since the last one and drops what each names, and the read that follows reads those paths from
// the disk again. A notification that reports lost events (an overflow) drops everything, and the next read reads
// the whole corpus again: the notification contract requires that path, and it is published as corpus.overflow.
//
// The notifications: kqueue EVFILT_VNODE on each folder and file read, on macOS; inotify on each folder and file
// read, on Linux; one ReadDirectoryChangesW over the corpus root and its subtree, on Windows. A failure while taking
// them loses those taken, so it drops everything as an overflow does. On macOS and Linux the kernel
// queues a notification in the system call that makes the change, so Refresh takes every change that completed before
// it; ReadDirectoryChangesW reports a write when the file system records it, which may be later. The commit targets
// are read again under the lock whatever the notifications say (Forget), so a commit's check of its targets never
// depends on that latency.
//
// A Tree is used by one operation at a time, under the corpus lock; its methods are safe to call from several
// goroutines.
type Tree struct {
	c    *Corpus
	disk store.FS

	mu    sync.Mutex
	w     treeWatch
	dirs  map[string][]fs.DirEntry
	files map[string][]byte
	// fault is a failure of the watch met by a read; Fault returns it and Refresh clears it, since a read that met it
	// cached nothing.
	fault error
	// notices is the test seam that rewrites the notifications each Refresh takes; nil outside the tests.
	notices func(Notices) Notices
}

// Notices are the notifications one Refresh takes: Changed names each path whose bytes or entries changed, Moved each
// path that appeared, disappeared or was renamed — with what lies under it and the entries of its folder — and
// Overflow reports that notifications were lost, so every path may have changed.
type Notices struct {
	Overflow bool
	Changed  []string
	Moved    []string
}

// treeWatch is the platform's change notification of the corpus tree.
type treeWatch interface {
	// watch arms the notification of the folder or file rel before it is read. An error that the read of rel would
	// meet too (the path is absent, or is a symbolic link) is returned as a *fs.PathError; any other error is a
	// failure of the watch.
	watch(rel string, dir bool) error
	// drain returns, without waiting, the notifications queued since the last drain.
	drain() (Notices, error)
	close() error
}

// Tree returns the corpus tree of c, its watch armed.
func (c *Corpus) Tree() (*Tree, error) {
	w, err := newTreeWatch(c)
	if err != nil {
		return nil, fmt.Errorf("watching the corpus %s: %w", c.root, err)
	}
	return &Tree{c: c, disk: store.DirFS(c.root), w: w, dirs: map[string][]fs.DirEntry{}, files: map[string][]byte{}}, nil
}

// SetNotices installs the test seam that rewrites the notifications each Refresh takes — to hold one back, as a
// late notification, or to report an overflow; nil removes it. Like SetFault it is a test seam of this module,
// exported only for the tests of the write path.
func (t *Tree) SetNotices(f func(Notices) Notices) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.notices = f
}

// Refresh takes the notifications queued since the last Refresh and drops what they name; each path dropped is
// published as corpus.changed, and an overflow as corpus.overflow.
func (t *Tree) Refresh() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.w == nil {
		return errors.New("the corpus tree is closed")
	}
	t.fault = nil
	n, err := t.w.drain()
	if err != nil {
		// The notifications the failed drain took are lost with it, so nothing the tree holds is known current: the
		// next read reads the whole corpus again, as after an overflow, and this operation fails naming the cause.
		t.dirs = map[string][]fs.DirEntry{}
		t.files = map[string][]byte{}
		return fmt.Errorf("taking the change notifications of the corpus: %w", err)
	}
	if t.notices != nil {
		n = t.notices(n)
	}
	if n.Overflow {
		t.dirs = map[string][]fs.DirEntry{}
		t.files = map[string][]byte{}
		t.c.obs.Observe(Event{Kind: CorpusOverflow, Path: t.c.root})
		return nil
	}
	for _, p := range n.Changed {
		t.drop(p, false)
	}
	for _, p := range n.Moved {
		t.drop(p, true)
	}
	return nil
}

// Forget drops what the tree holds of each path, what lies under it, and the entries of every folder above it, so
// the next read reads them from the disk whatever the notifications said: the write path forgets its commit targets
// before it reads them under the lock, and after it wrote them.
func (t *Tree) Forget(paths ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, p := range paths {
		t.drop(p, true)
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			delete(t.dirs, d)
		}
		delete(t.dirs, ".")
	}
}

// drop drops the bytes and the entries held at p; moved drops what lies under p and the entries of its folder too.
// A path the tree held is published as corpus.changed.
func (t *Tree) drop(p string, moved bool) {
	held := false
	if _, ok := t.files[p]; ok {
		delete(t.files, p)
		held = true
	}
	if _, ok := t.dirs[p]; ok {
		delete(t.dirs, p)
		held = true
	}
	if moved {
		if p == "." {
			held = held || len(t.files) > 0 || len(t.dirs) > 0
			t.files = map[string][]byte{}
			t.dirs = map[string][]fs.DirEntry{}
		} else {
			under := p + "/"
			for q := range t.files {
				if strings.HasPrefix(q, under) {
					delete(t.files, q)
					held = true
				}
			}
			for q := range t.dirs {
				if strings.HasPrefix(q, under) {
					delete(t.dirs, q)
					held = true
				}
			}
			if _, ok := t.dirs[path.Dir(p)]; ok {
				delete(t.dirs, path.Dir(p))
				held = true
			}
		}
	}
	if held {
		t.c.obs.Observe(Event{Kind: CorpusChanged, Path: t.c.abs(p)})
	}
}

// Fault returns the failure of the watch that a read since the last Refresh met; the write path fails the operation
// with it, since that read answered without the path it could not watch.
func (t *Tree) Fault() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.fault
}

// arm watches rel before its read; a failure of the watch is kept as the fault.
func (t *Tree) arm(rel string, dir bool) error {
	if t.w == nil {
		return errors.New("the corpus tree is closed")
	}
	err := t.w.watch(rel, dir)
	var pe *fs.PathError
	if err != nil && !errors.As(err, &pe) {
		err = fmt.Errorf("watching %s: %w", t.c.abs(rel), err)
		t.fault = errors.Join(t.fault, err)
	}
	return err
}

// ReadDir returns the entries of the folder name, read from the disk when the tree does not hold them.
func (t *Tree) ReadDir(name string) ([]fs.DirEntry, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if es, ok := t.dirs[name]; ok {
		return es, nil
	}
	if err := t.arm(name, true); err != nil {
		return nil, err
	}
	t.c.obs.Observe(Event{Kind: CorpusRead, Path: t.c.abs(name)})
	es, err := t.disk.ReadDir(name)
	if err != nil {
		return nil, err
	}
	t.dirs[name] = es
	return es, nil
}

// ReadFile returns the bytes of the file name, read from the disk when the tree does not hold them.
func (t *Tree) ReadFile(name string) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if b, ok := t.files[name]; ok {
		return b, nil
	}
	if err := t.arm(name, false); err != nil {
		return nil, err
	}
	t.c.obs.Observe(Event{Kind: CorpusRead, Path: t.c.abs(name)})
	b, err := t.disk.ReadFile(name)
	if err != nil {
		return nil, err
	}
	t.files[name] = b
	return b, nil
}

// Close closes the watch; a closed tree refuses every read.
func (t *Tree) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.w == nil {
		return nil
	}
	err := t.w.close()
	t.w = nil
	t.dirs, t.files = nil, nil
	return err
}

// pathError is the *fs.PathError of an operation on the corpus path rel that the read of rel would meet too.
func pathError(op, rel string, err error) error {
	return &fs.PathError{Op: op, Path: rel, Err: err}
}
