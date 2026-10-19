//go:build linux || darwin

package journal

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// pidGone reports whether no process has this ID: kill with signal 0 checks without sending one; ESRCH means none,
// and success or EPERM (a process of another user) means one exists (REVIEW 4-a).
func pidGone(pid int) (bool, error) {
	err := unix.Kill(pid, 0)
	switch {
	case err == nil, errors.Is(err, unix.EPERM):
		return false, nil
	case errors.Is(err, unix.ESRCH):
		return true, nil
	}
	return false, fmt.Errorf("checking process %d: %w", pid, err)
}

// processState judges a holder of this host from its process: ended when no process has its ID, when that process is
// a zombie or started at another time; running when its start matches, or when it exists and its start cannot be read.
func processState(pid int, start string) (HolderState, error) {
	gone, err := pidGone(pid)
	if err != nil || gone {
		return HolderEnded, err
	}
	got, zombie, err := processInfo(pid)
	if err != nil {
		// The start cannot be read: ask again whether the process exists (it may have ended meanwhile).
		gone, err := pidGone(pid)
		if err != nil || gone {
			return HolderEnded, err
		}
		return HolderRunning, nil
	}
	if zombie || got != start {
		return HolderEnded, nil
	}
	return HolderRunning, nil
}

// processStart returns the start of a running process.
func processStart(pid int) (string, error) {
	start, _, err := processInfo(pid)
	return start, err
}
