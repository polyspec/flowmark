//go:build linux

package journal

import (
	"encoding/binary"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// inotifyBuffer encodes inotify events as the kernel returns them.
func inotifyBuffer(events ...struct {
	mask uint32
	name string
}) []byte {
	var buf []byte
	for _, e := range events {
		name := []byte(e.name)
		if len(name) > 0 {
			// The kernel pads the name with NUL bytes to a multiple of the event size.
			pad := 16 - len(name)%16
			name = append(name, make([]byte, pad)...)
		}
		head := make([]byte, unix.SizeofInotifyEvent)
		binary.NativeEndian.PutUint32(head[0:], 1)
		binary.NativeEndian.PutUint32(head[4:], e.mask)
		binary.NativeEndian.PutUint32(head[12:], uint32(len(name)))
		buf = append(buf, head...)
		buf = append(buf, name...)
	}
	return buf
}

type ev = struct {
	mask uint32
	name string
}

// Each removal or rename of the lock, each overflow and each event of the watched folder itself wakes; the removal of
// another name does not.
func TestInotifyEventsWake(t *testing.T) {
	cases := []struct {
		events []ev
		want   int
	}{
		{[]ev{{unix.IN_DELETE, "lock"}}, 1},
		{[]ev{{unix.IN_MOVED_FROM, "lock"}}, 1},
		{[]ev{{unix.IN_DELETE, "lock.00000000000000000000000000000001.pending"}}, 0},
		{[]ev{{unix.IN_DELETE, "lockx"}}, 0},
		{[]ev{{unix.IN_Q_OVERFLOW, ""}}, 1},
		{[]ev{{unix.IN_IGNORED, ""}}, 1},
		{[]ev{{unix.IN_DELETE_SELF, ""}}, 1},
		{[]ev{{unix.IN_MOVE_SELF, ""}}, 1},
		{[]ev{{unix.IN_UNMOUNT, ""}}, 1},
		{[]ev{{unix.IN_DELETE, "a"}, {unix.IN_DELETE, "lock"}, {unix.IN_DELETE, "b"}}, 1},
	}
	for _, tc := range cases {
		got, _, err := inotifyWakes(inotifyBuffer(tc.events...))
		if err != nil || got != tc.want {
			t.Errorf("inotifyWakes(%v) = %d, %v; want %d", tc.events, got, err, tc.want)
		}
	}
	if _, _, err := inotifyWakes([]byte{1, 2, 3}); err == nil {
		t.Error("a truncated event was accepted")
	}
	if _, ignored, err := inotifyWakes(inotifyBuffer(ev{unix.IN_IGNORED, ""})); err != nil || !ignored {
		t.Errorf("IN_IGNORED: ignored = %t, error = %v", ignored, err)
	}
}

// A watch that cannot be armed names its cause.
func TestInotifyArmErrorNamesLimit(t *testing.T) {
	if got := inotifyArmError(syscall.EMFILE).Error(); !strings.Contains(got, "fs.inotify.max_user_instances") {
		t.Errorf("EMFILE: %q", got)
	}
	if got := inotifyArmError(syscall.ENOSPC).Error(); !strings.Contains(got, "fs.inotify.max_user_watches") {
		t.Errorf("ENOSPC: %q", got)
	}
}
