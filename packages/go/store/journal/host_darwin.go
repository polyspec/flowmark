//go:build darwin

package journal

import (
	"encoding/hex"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// hostID is the host UUID of gethostuuid(2). golang.org/x/sys has no wrapper for it, so it is called through
// unix.Syscall with its number, SYS_GETHOSTUUID; the system answers it like any other call.
func hostID() (string, error) {
	var id [16]byte
	wait := unix.Timespec{Sec: 5}
	if _, _, e := unix.Syscall(unix.SYS_GETHOSTUUID, uintptr(unsafe.Pointer(&id[0])), uintptr(unsafe.Pointer(&wait)), 0); e != 0 {
		return "", fmt.Errorf("gethostuuid(2): %w", e)
	}
	return hex.EncodeToString(id[:]), nil
}

// sZomb is SZOMB of <sys/proc.h>, the p_stat of a process that has exited and is not yet reaped.
const sZomb = 5

// processInfo reads the start of a process — kp_proc.p_starttime of sysctl kern.proc.pid.<pid> — and whether it is a
// zombie.
func processInfo(pid int) (string, bool, error) {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", false, err
	}
	t := k.Proc.P_starttime
	return fmt.Sprintf("%d.%06d", t.Sec, t.Usec), k.Proc.P_stat == sZomb, nil
}
