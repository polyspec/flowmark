//go:build linux || darwin

package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
)

// mkdirPrivate creates the folder p with access for its owner alone, and syncs its parent.
func mkdirPrivate(k *cost.Counts, p string) error {
	if err := os.Mkdir(p, 0o700); err != nil {
		return err
	}
	return syncDir(k, filepath.Dir(p))
}

// checkPrivate refuses the folder p unless this process's user owns it and no one else has access to it.
func checkPrivate(p string) error {
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%s has no owner", p)
	}
	if int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("%s is owned by the user %d, not by this process's user %d; the databases' folder is its owner's alone", p, st.Uid, os.Geteuid())
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%s grants access %v to others than its owner; the databases' folder is its owner's alone", p, perm)
	}
	return nil
}
