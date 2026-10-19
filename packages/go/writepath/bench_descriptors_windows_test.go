//go:build windows

package writepath

import "testing"

// openDescriptorCount is 0 on Windows: the watch there is one handle over the corpus root and its subtree, not one
// for each folder and document.
func openDescriptorCount(b testing.TB) int { return 0 }
