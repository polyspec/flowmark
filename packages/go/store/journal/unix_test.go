//go:build linux || darwin

package journal

import (
	"syscall"
	"testing"
)

// A link that the file system refuses for want of hard links is classified as such; any other failure is not.
func TestLinkUnsupportedClassified(t *testing.T) {
	wantLinkErrors(t, map[error]bool{
		syscall.EPERM:   true,
		syscall.ENOTSUP: true,
		syscall.EEXIST:  false,
		syscall.ENOENT:  false,
		syscall.EACCES:  false,
	})
}
