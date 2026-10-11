package journal

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EntryKind is what stands at a path of the corpus.
type EntryKind int

// The kinds of entry: none, a regular file, a folder, and anything else — a symbolic link, a device, a socket.
const (
	EntryAbsent EntryKind = iota
	EntryFile
	EntryFolder
	EntryOther
)

// Entry returns what stands at the slash path rel of the corpus, following no symbolic link: a symbolic link at rel is
// EntryOther, and one on the way to rel, or a file where a folder of rel must be, is an error that names it.
func (c *Corpus) Entry(rel string) (EntryKind, error) {
	if err := checkRel(rel); err != nil {
		return 0, fmt.Errorf("journal: %w", err)
	}
	cur := c.root
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return EntryAbsent, nil
		}
		if err != nil {
			return 0, fmt.Errorf("journal: %w", err)
		}
		last := i == len(parts)-1
		switch {
		case last && fi.Mode().IsRegular():
			return EntryFile, nil
		case last && fi.IsDir():
			return EntryFolder, nil
		case last:
			return EntryOther, nil
		case fi.Mode()&fs.ModeSymlink != 0:
			return 0, fmt.Errorf("journal: %w", symlinkError(cur))
		case !fi.IsDir():
			return 0, fmt.Errorf("journal: %s is not a folder", cur)
		}
	}
	return EntryAbsent, nil
}

// ReadFile returns the bytes of the regular file at rel under the held lock l, or nil when it is absent: what a commit
// of l would record as the file's pre-image.
func (c *Corpus) ReadFile(l *Lock, rel string) ([]byte, error) {
	if err := c.checkHeld(l); err != nil {
		return nil, err
	}
	if err := checkRel(rel); err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	_, data, err := c.readTarget(rel)
	if err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	return data, nil
}

// MakeFolder creates the folder rel and every absent folder on its way under the held lock l, following no symbolic
// link; a folder in place is kept. On Linux and macOS each folder created is made durable by syncing its parent; on
// Windows, which has no folder flush, a folder that a power loss takes back is absent again — an absent table folder is
// an empty table (spec 20 Storage) — and the idempotent step that created it creates it again.
func (c *Corpus) MakeFolder(l *Lock, rel string) error {
	if err := c.checkHeld(l); err != nil {
		return err
	}
	if err := checkRel(rel); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if err := c.ensureDir(rel); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	return nil
}
