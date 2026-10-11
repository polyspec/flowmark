// Package journal is the lower half of the write path of spec 40: the portable lock with its event-driven wait and
// `flowmark unlock`, and the journal that stages a commit's target writes in `.flowmark/tx/<id>/` and replays it on
// open. Transactions, savepoints and the constraint checks sit above it and call it.
//
// Every path is resolved from the declared corpus root, never from the working directory, and no symbolic link is
// followed: a symbolic link where the lock file, the journal, a target or a folder to create must be is refused.
package journal

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Corpus is the declared root of one corpus and the observer of its lock and journal events.
type Corpus struct {
	root string
	obs  Observer
	// fault is the fault-injection seam of the tests: it is called at each step of a commit, a replay, a lock attempt
	// and an unlock, and its error stops the run there as a crash would. It is nil outside the tests.
	fault func(step string, index int) error
	// newWatch arms the watch of a lock wait; the tests replace it to inject wakes.
	newWatch func() (lockWatch, error)
	// stageFolders creates a missing corpus folder in the stage and moves it into place (spec 40, durability); true on
	// Windows, which has no folder flush. The tests set it to run that path on every platform.
	stageFolders bool
	// share is what the lock waits of this corpus share: the inotify instance on Linux.
	share watchShare
}

// New declares the corpus at root, an absolute and clean path of a folder that is no symbolic link. obs receives
// every event.
func New(root string, obs Observer) (*Corpus, error) {
	if obs == nil {
		return nil, errors.New("journal: no observer: pass the Observer that receives the lock and journal events")
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, fmt.Errorf("journal: the corpus root %q is not an absolute clean path; declare it as one", root)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("journal: the corpus root: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("journal: the corpus root %s is a symbolic link, which flowmark never follows; declare the folder it names", root)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("journal: the corpus root %s is not a folder", root)
	}
	c := &Corpus{root: root, obs: obs, stageFolders: stageFoldersDefault}
	c.newWatch = func() (lockWatch, error) { return newLockWatch(c) }
	return c, nil
}

// Root returns the declared corpus root.
func (c *Corpus) Root() string { return c.root }

// SetFault installs the fault-injection seam: f is called at each step of a commit, a replay, a lock attempt and an
// unlock, and its error stops the run there as a crash would; nil removes it. It is exported for the crash tests of the
// write path above the journal, which commits through it; nothing outside the tests installs one.
func (c *Corpus) SetFault(f func(step string, index int) error) { c.fault = f }

func (c *Corpus) step(step string, index int) error {
	if c.fault == nil {
		return nil
	}
	return c.fault(step, index)
}

// checkRel accepts a slash-separated path inside the root in its clean form; anything else is refused, never cleaned.
func checkRel(rel string) error {
	switch {
	case rel == "":
		return errors.New("the path is empty")
	case strings.Contains(rel, `\`):
		return fmt.Errorf("the path %q holds a backslash; paths are slash-separated", rel)
	case path.Clean(rel) != rel || strings.HasPrefix(rel, "/") || !filepath.IsLocal(filepath.FromSlash(rel)):
		return fmt.Errorf("the path %q is not a clean path inside the corpus root", rel)
	}
	return nil
}

func (c *Corpus) abs(rel string) string {
	return filepath.Join(c.root, filepath.FromSlash(rel))
}

func symlinkError(p string) error {
	return fmt.Errorf("%s is a symbolic link, which flowmark never follows; replace it with the folder or file itself", p)
}

// lstat returns the information of rel, or nil when it or one of its folders is absent. Every folder on the way must
// be a folder and nothing on the way may be a symbolic link.
func (c *Corpus) lstat(rel string) (fs.FileInfo, error) {
	cur := c.root
	parts := strings.Split(rel, "/")
	var info fs.FileInfo
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return nil, symlinkError(cur)
		}
		if i < len(parts)-1 && !fi.IsDir() {
			return nil, fmt.Errorf("%s is not a folder", cur)
		}
		info = fi
	}
	return info, nil
}

// nearestFolder returns the deepest existing folder on the way to the folder dir (a slash path, "." for the root):
// the folder itself when it exists, otherwise the one its missing folders would be created in.
func (c *Corpus) nearestFolder(dir string) (string, error) {
	cur := c.root
	if dir == "." {
		return cur, nil
	}
	for _, part := range strings.Split(dir, "/") {
		next := filepath.Join(cur, part)
		fi, err := os.Lstat(next)
		if errors.Is(err, fs.ErrNotExist) {
			return cur, nil
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return "", symlinkError(next)
		}
		if !fi.IsDir() {
			return "", fmt.Errorf("%s is not a folder", next)
		}
		cur = next
	}
	return cur, nil
}

// ensureDir creates every absent folder of rel inside `.flowmark/` (the journal's own folders), following no symlink.
func (c *Corpus) ensureDir(rel string) error {
	cur := c.root
	for _, part := range strings.Split(rel, "/") {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := mkdirDurable(cur); err != nil {
				return err
			}
		case err != nil:
			return err
		case fi.Mode()&fs.ModeSymlink != 0:
			return symlinkError(cur)
		case !fi.IsDir():
			return fmt.Errorf("%s is not a folder", cur)
		}
	}
	return nil
}

// randomHex returns 2n lowercase hex digits of n random bytes from the OS random source.
func randomHex(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read never returns an error; it aborts the program when the system source fails.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
