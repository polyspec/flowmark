//go:build windows

package writepath

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// processCPU returns the user and kernel CPU time the process has used (GetProcessTimes).
func processCPU(t testing.TB) time.Duration {
	t.Helper()
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &creation, &exit, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	ticks := func(f windows.Filetime) int64 { return int64(f.HighDateTime)<<32 | int64(f.LowDateTime) }
	return time.Duration((ticks(kernel) + ticks(user)) * 100)
}
