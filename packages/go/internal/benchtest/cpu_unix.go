//go:build unix

package benchtest

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ProcessCPU returns the user and system CPU time the process has used (getrusage).
func ProcessCPU(t testing.TB) time.Duration {
	t.Helper()
	var u unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &u); err != nil {
		t.Fatal(err)
	}
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}
