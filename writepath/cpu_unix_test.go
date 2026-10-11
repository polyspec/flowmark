//go:build unix

package writepath

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// processCPU returns the user and system CPU time the process has used (getrusage).
func processCPU(t testing.TB) time.Duration {
	t.Helper()
	var u unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &u); err != nil {
		t.Fatal(err)
	}
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}
