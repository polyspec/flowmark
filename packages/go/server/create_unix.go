//go:build !windows

package server

import "os"

// createOwnerOnly creates the new file p for writing, readable and writable by its owner only: mode 0600.
func createOwnerOnly(p string) (*os.File, error) {
	return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}
