//go:build linux

package journal

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// hostID is the machine identifier of machine-id(5) and the inode of this process's PID namespace, so two
// containers of one machine, whose process IDs overlap, are two hosts.
func hostID() (string, error) {
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return "", fmt.Errorf("/etc/machine-id (machine-id(5)) cannot be read, so a lock holder cannot be identified; create it with systemd-machine-id-setup or dbus-uuidgen: %w", err)
	}
	id := strings.TrimSuffix(string(data), "\n")
	if !isHex(id, 32) {
		return "", fmt.Errorf("/etc/machine-id holds %q, not 32 lowercase hex digits (machine-id(5))", data)
	}
	var st unix.Stat_t
	if err := unix.Stat("/proc/self/ns/pid", &st); err != nil {
		return "", fmt.Errorf("the PID namespace /proc/self/ns/pid: %w", err)
	}
	return id + ":" + strconv.FormatUint(st.Ino, 10), nil
}

// processInfo reads the start of a process — the boot of /proc/sys/kernel/random/boot_id and field 22 of
// /proc/<pid>/stat, the clock ticks from that boot — and whether it is a zombie (state Z).
func processInfo(pid int) (string, bool, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", false, err
	}
	// The command in parentheses may hold spaces and parentheses; the fields follow its last ')'.
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 || i+2 > len(s) {
		return "", false, fmt.Errorf("/proc/%d/stat holds no fields: %q", pid, s)
	}
	fields := strings.Fields(s[i+2:])
	if len(fields) < 20 {
		return "", false, fmt.Errorf("/proc/%d/stat holds no start time: %q", pid, s)
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(string(boot)) + ":" + fields[19], fields[0] == "Z", nil
}
