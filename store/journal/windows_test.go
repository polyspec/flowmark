//go:build windows

package journal

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// These tests run only on Windows; this repository's hosts are macOS, so they are compiled with
// `GOOS=windows go test -c` and run on a Windows host.

func TestLinkUnsupportedClassified(t *testing.T) {
	for errno, want := range map[windows.Errno]bool{
		windows.ERROR_INVALID_FUNCTION: true,
		windows.ERROR_NOT_SUPPORTED:    true,
		windows.ERROR_ALREADY_EXISTS:   false,
		windows.ERROR_ACCESS_DENIED:    false,
	} {
		err := &os.LinkError{Op: "link", Old: "a", New: "b", Err: errno}
		if got := linkUnsupported(err); got != want {
			t.Errorf("linkUnsupported(%v) = %v, want %v", errno, got, want)
		}
		if want && !strings.Contains(linkError(err).Error(), "hard links") {
			t.Errorf("linkError(%v) = %v", errno, linkError(err))
		}
	}
}

// notifyBuffer encodes FILE_NOTIFY_INFORMATION entries as ReadDirectoryChangesW returns them.
func notifyBuffer(entries ...struct {
	action uint32
	name   string
}) []byte {
	var buf []byte
	for i, e := range entries {
		name := utf16.Encode([]rune(e.name))
		size := 12 + 2*len(name)
		size = (size + 3) &^ 3
		entry := make([]byte, size)
		if i < len(entries)-1 {
			binary.LittleEndian.PutUint32(entry[0:], uint32(size))
		}
		binary.LittleEndian.PutUint32(entry[4:], e.action)
		binary.LittleEndian.PutUint32(entry[8:], uint32(2*len(name)))
		for j, u := range name {
			binary.LittleEndian.PutUint16(entry[12+2*j:], u)
		}
		buf = append(buf, entry...)
	}
	return buf
}

type note = struct {
	action uint32
	name   string
}

func TestNotifyInformationWakes(t *testing.T) {
	cases := []struct {
		entries []note
		want    int
	}{
		{[]note{{windows.FILE_ACTION_REMOVED, "lock"}}, 1},
		{[]note{{windows.FILE_ACTION_RENAMED_OLD_NAME, "lock"}}, 1},
		{[]note{{windows.FILE_ACTION_ADDED, "lock"}}, 0},
		{[]note{{windows.FILE_ACTION_REMOVED, "lock.00000000000000000000000000000001.pending"}}, 0},
		{[]note{{windows.FILE_ACTION_REMOVED, "a"}, {windows.FILE_ACTION_REMOVED, "lock"}}, 1},
	}
	for _, tc := range cases {
		buf := notifyBuffer(tc.entries...)
		if got := notifyWakes(buf, uint32(len(buf))); got != tc.want {
			t.Errorf("notifyWakes(%v) = %d, want %d", tc.entries, got, tc.want)
		}
	}
	// Zero bytes returned is an overflow: one wake.
	if got := notifyWakes(make([]byte, 64), 0); got != 1 {
		t.Errorf("an overflow woke %d times", got)
	}
}

// A file opened for reading by the journal does not block its delete.
func TestOpenSharedAllowsDelete(t *testing.T) {
	p := filepath.Join(t.TempDir(), "record")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := openShared(p, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Remove(p); err != nil {
		t.Fatalf("the delete was blocked by a shared open: %v", err)
	}
}
