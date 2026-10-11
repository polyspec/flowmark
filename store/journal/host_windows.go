//go:build windows

package journal

import (
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// hostID is HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid, read from the 64-bit view.
func hostID() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", fmt.Errorf(`HKLM\SOFTWARE\Microsoft\Cryptography cannot be opened, so a lock holder cannot be identified: %w`, err)
	}
	v, _, err := k.GetStringValue("MachineGuid")
	cerr := k.Close()
	if err := errors.Join(err, cerr); err != nil {
		return "", fmt.Errorf(`MachineGuid cannot be read, so a lock holder cannot be identified: %w`, err)
	}
	if checkPart("host", v) != nil {
		return "", fmt.Errorf("MachineGuid holds %q, which cannot name a host", v)
	}
	return v, nil
}

// creation is the GetProcessTimes creation time of a process, in 100-nanosecond intervals since 1601.
func creation(h windows.Handle) (string, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10), nil
}

func processStart(pid int) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", fmt.Errorf("opening process %d: %w", pid, err)
	}
	start, err := creation(h)
	return start, errors.Join(err, windows.CloseHandle(h))
}

// processState judges a holder of this host (REVIEW 4-a): OpenProcess refuses an ID that names no process with
// ERROR_INVALID_PARAMETER and a process of another user with ERROR_ACCESS_DENIED (it exists, and its start cannot be
// read). Liveness is read from the process handle — WaitForSingleObject with no wait — never from the exit code,
// since an exit code of 259 equals STILL_ACTIVE.
func processState(pid int, start string) (HolderState, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	switch {
	case errors.Is(err, windows.ERROR_INVALID_PARAMETER):
		return HolderEnded, nil
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return HolderRunning, nil
	case err != nil:
		return 0, fmt.Errorf("opening process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return 0, fmt.Errorf("waiting on process %d: %w", pid, err)
	}
	if ev == windows.WAIT_OBJECT_0 {
		return HolderEnded, nil
	}
	got, err := creation(h)
	if err != nil {
		return HolderRunning, nil
	}
	if got != start {
		return HolderEnded, nil
	}
	return HolderRunning, nil
}
