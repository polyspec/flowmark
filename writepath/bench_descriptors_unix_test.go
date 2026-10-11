//go:build unix

package writepath

import (
	"os"
	"testing"
)

// openDescriptorCount returns the number of descriptors the process holds.
func openDescriptorCount(b testing.TB) int {
	b.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		b.Fatal(err)
	}
	return len(entries)
}
