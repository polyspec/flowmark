//go:build !windows

package server

import (
	"os"
	"testing"
)

// wantOwnerOnly fails unless the file p is readable and writable by its owner only: mode 0600.
func wantOwnerOnly(t *testing.T, p string) {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("%s mode %v, want 0600", p, fi.Mode().Perm())
	}
}
