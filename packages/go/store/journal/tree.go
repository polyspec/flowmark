package journal

import (
	"errors"
	"fmt"
	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/polyspec/flowmark/packages/go/internal/pmap"
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

	mu sync.Mutex
	w  treeWatch
	// dirs and files hold what the reads asked for, by corpus path, ordered so that what lies under a path is one range.
	dirs  pmap.Map[string, []fs.DirEntry]
	files pmap.Map[string, []byte]
	// changed holds the paths dropped since the last Changes that the tree held, and all whether everything was.
	changed map[string]bool
	all     bool
	// fault is a failure of the watch met by a read; Fault returns it and Refresh clears it, since a read that met it
	// cached nothing.
	fault error
	// notices is the test seam that rewrites the notifications each Refresh takes; nil outside the tests.
	notices func(Notices) Notices
	// cost counts the folders and files read from the disk.
	cost *cost.Counts
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
func (c *Corpus) Tree(k *cost.Counts) (*Tree, error) {
	w, err := newTreeWatch(c)
	if err != nil {
		return nil, fmt.Errorf("watching the corpus %s: %w", c.root, err)
	}
	return &Tree{c: c, disk: store.DirFS(c.root), w: w, changed: map[string]bool{}, cost: k}, nil
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
		t.dropAll()
		return fmt.Errorf("taking the change notifications of the corpus: %w", err)
	}
	if t.notices != nil {
		n = t.notices(n)
	}
	if n.Overflow {
		t.dropAll()
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
			t.dropDir(d)
		}
		t.dropDir(".")
	}
}

// drop drops the bytes and the entries held at p; moved drops what lies under p and the entries of its folder too.
// A path the tree held is published as corpus.changed.
func (t *Tree) drop(p string, moved bool) {
	held := t.dropFile(p)
	held = t.dropDir(p) || held
	if moved {
		if p == "." {
			held = held || t.files.Len() > 0 || t.dirs.Len() > 0
			t.dropAll()
		} else {
			under := p + "/"
			for _, q := range underKeys(t.files, under) {
				held = t.dropFile(q) || held
			}
			for _, q := range underKeys(t.dirs, under) {
				held = t.dropDir(q) || held
			}
			held = t.dropDir(path.Dir(p)) || held
		}
	}
	if held {
		t.c.obs.Observe(Event{Kind: CorpusChanged, Path: t.c.abs(p)})
	}
}

// underKeys returns the keys of m that begin with under, which stand together in its order.
func underKeys[V any](m pmap.Map[string, V], under string) []string {
	var out []string
	for q := range m.From(under) {
		if !strings.HasPrefix(q, under) {
			break
		}
		out = append(out, q)
	}
	return out
}

// dropFile drops the bytes held at p, recording p as changed, and reports whether the tree held them.
func (t *Tree) dropFile(p string) bool {
	if _, ok := t.files.Get(p); !ok {
		return false
	}
	t.files = t.files.Delete(p)
	t.changed[p] = true
	return true
}

// dropDir drops the entries held at p, recording p as changed, and reports whether the tree held them.
func (t *Tree) dropDir(p string) bool {
	if _, ok := t.dirs.Get(p); !ok {
		return false
	}
	t.dirs = t.dirs.Delete(p)
	t.changed[p] = true
	return true
}

// dropAll drops everything the tree holds, so the next read reads the whole corpus and Changes reports all.
func (t *Tree) dropAll() {
	t.dirs, t.files = pmap.Map[string, []fs.DirEntry]{}, pmap.Map[string, []byte]{}
	t.changed, t.all = map[string]bool{}, true
}

// Changes returns, sorted, the paths of the folders and files the tree held and dropped since the last Changes —
// by a notification, by Forget, or by an overflow, which reports all instead — and starts the next record. A read that
// kept what it made of the tree reads again only these paths (plan, Ruling: an operation costs what it touches).
func (t *Tree) Changes() (paths []string, all bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	paths, all = slices.Sorted(maps.Keys(t.changed)), t.all
	t.changed, t.all = map[string]bool{}, false
	return paths, all
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
	if es, ok := t.dirs.Get(name); ok {
		return es, nil
	}
	if err := t.arm(name, true); err != nil {
		return nil, err
	}
	t.c.obs.Observe(Event{Kind: CorpusRead, Path: t.c.abs(name)})
	t.cost.Add(cost.FilesRead, 1)
	es, err := t.disk.ReadDir(name)
	if err != nil {
		return nil, err
	}
	t.dirs = t.dirs.Set(name, es)
	return es, nil
}

// ReadFile returns the bytes of the file name, read from the disk when the tree does not hold them.
func (t *Tree) ReadFile(name string) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if b, ok := t.files.Get(name); ok {
		return b, nil
	}
	if err := t.arm(name, false); err != nil {
		return nil, err
	}
	t.c.obs.Observe(Event{Kind: CorpusRead, Path: t.c.abs(name)})
	t.cost.Add(cost.FilesRead, 1)
	b, err := t.disk.ReadFile(name)
	if err != nil {
		return nil, err
	}
	t.files = t.files.Set(name, b)
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
	t.dirs, t.files = pmap.Map[string, []fs.DirEntry]{}, pmap.Map[string, []byte]{}
	return err
}

// pathError is the *fs.PathError of an operation on the corpus path rel that the read of rel would meet too.
func pathError(op, rel string, err error) error {
	return &fs.PathError{Op: op, Path: rel, Err: err}
}
