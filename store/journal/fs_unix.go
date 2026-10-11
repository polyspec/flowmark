//go:build linux || darwin

package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// stageFoldersDefault: Linux and macOS make a created folder durable by syncing its parent, so no folder is staged.
const stageFoldersDefault = false

// openShared opens a file of the journal or the lock without following a symbolic link at its name.
func openShared(p string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(p, flag|syscall.O_NOFOLLOW, perm)
}

// fileIdentity returns the device and inode of an open file.
func fileIdentity(f *os.File) (fileID, error) {
	info, err := f.Stat()
	if err != nil {
		return fileID{}, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}, fmt.Errorf("%s has no device and inode", f.Name())
	}
	return fileID{dev: uint64(st.Dev), ino: st.Ino}, nil
}

// syncDir syncs a folder, so the names created, renamed or removed in it are on disk (POSIX); the runtime's Sync is
// F_FULLFSYNC on macOS.
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

// moveDurable renames src to dst and syncs the folders of both names.
func moveDurable(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(dst)); err != nil {
		return err
	}
	if filepath.Dir(src) != filepath.Dir(dst) {
		return syncDir(filepath.Dir(src))
	}
	return nil
}

// moveFolderDurable moves a folder to a name that does not exist, as moveDurable does a file.
func moveFolderDurable(src, dst string) error { return moveDurable(src, dst) }

// mkdirDurable creates a folder and syncs its parent.
func mkdirDurable(p string) error {
	if err := os.Mkdir(p, 0o755); err != nil {
		return err
	}
	return syncDir(filepath.Dir(p))
}

// syncRemoved makes the removal of a name in dir durable.
func syncRemoved(dir string) error { return syncDir(dir) }

// deviceOf returns the device of a path, st_dev.
func deviceOf(p string) (uint64, error) {
	var st unix.Stat_t
	if err := unix.Lstat(p, &st); err != nil {
		return 0, &os.PathError{Op: "lstat", Path: p, Err: err}
	}
	return uint64(st.Dev), nil
}

// linkUnsupported reports a link refused because the file system has no hard links (FAT32, exFAT): EPERM or ENOTSUP
// (EOPNOTSUPP, which macOS numbers apart).
func linkUnsupported(err error) bool {
	return errors.Is(err, unix.EPERM) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP)
}
