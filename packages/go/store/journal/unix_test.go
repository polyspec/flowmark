//go:build linux || darwin

package journal

import (
	"syscall"
	"testing"
)

// otherUserPID is the process of another user that the host has: process 1, launchd of root on macOS and init on Linux.
const otherUserPID = 1

// killedExitCode is the exit code that os/exec reports for a process ended by Kill: -1, since a signal ended it.
const killedExitCode = -1

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
