//go:build linux || darwin

package journal

import (
	"os"
	"strings"
	"syscall"
	"testing"
)

// A link that the file system refuses for want of hard links is classified as such; any other failure is not.
func TestLinkUnsupportedClassified(t *testing.T) {
	for errno, want := range map[syscall.Errno]bool{
		syscall.EPERM:   true,
		syscall.ENOTSUP: true,
		syscall.EEXIST:  false,
		syscall.ENOENT:  false,
		syscall.EACCES:  false,
	} {
		err := &os.LinkError{Op: "link", Old: "a", New: "b", Err: errno}
		if got := linkUnsupported(err); got != want {
			t.Errorf("linkUnsupported(%v) = %v, want %v", errno, got, want)
		}
		if want && !strings.Contains(linkError(err).Error(), "hard links") {
			t.Errorf("linkError(%v) = %v, want the hard-link cause", errno, linkError(err))
		}
	}
}
