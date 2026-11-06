//go:build windows

package journal

import (
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// otherUserPID is the process of another user that the host has: process 4, System, which Windows always numbers 4.
const otherUserPID = 4

// killedExitCode is the exit code that os/exec reports for a process ended by Kill: Kill is TerminateProcess with exit
// code 1 on Windows.
const killedExitCode = 1

// These tests run only on Windows; this repository's hosts are macOS, so they are compiled with
// `GOOS=windows go test -c` and run on a Windows host.

func TestLinkUnsupportedClassified(t *testing.T) {
	wantLinkErrors(t, map[error]bool{
		windows.ERROR_INVALID_FUNCTION: true,
		windows.ERROR_NOT_SUPPORTED:    true,
		windows.ERROR_ALREADY_EXISTS:   false,
		windows.ERROR_ACCESS_DENIED:    false,
	})
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

// pendDelete marks p for deletion through a handle with DELETE access, as DeleteFileW does, and returns that handle:
// until it is closed, the delete of p is pending and Windows refuses every open and every link of the name with
// ERROR_ACCESS_DENIED (STATUS_DELETE_PENDING).
func pendDelete(t *testing.T, p string) windows.Handle {
	t.Helper()
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.DELETE, shareAll, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := struct{ Flags uint32 }{windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS}
	if err := windows.SetFileInformationByHandle(h, windows.FileDispositionInfoEx, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		t.Fatal(err)
	}
	return h
}

// A name whose delete is pending is removed as far as its remover is concerned: reading it or its attributes finds no
// file, an exclusive create of it finds one, and a link finds no file at it as a source and one as a target.
func TestDeletePendingReadsRemoved(t *testing.T) {
	dir := t.TempDir()
	gone, other := filepath.Join(dir, "gone"), filepath.Join(dir, "other")
	for _, p := range []string{gone, other} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := pendDelete(t, gone)
	defer windows.CloseHandle(h)
	if _, _, err := readShared(gone); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("readShared of a delete-pending name = %v, want fs.ErrNotExist", err)
	}
	if _, err := lstatFile(gone); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("lstatFile of a delete-pending name = %v, want fs.ErrNotExist", err)
	}
	if err := writeSynced(gone, []byte("y"), os.O_EXCL); !errors.Is(err, fs.ErrExist) {
		t.Errorf("an exclusive create of a delete-pending name = %v, want fs.ErrExist", err)
	}
	if err := linkFile(other, gone); !errors.Is(err, fs.ErrExist) {
		t.Errorf("a link to a delete-pending name = %v, want fs.ErrExist", err)
	}
	if err := linkFile(gone, filepath.Join(dir, "claim")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a link of a delete-pending name = %v, want fs.ErrNotExist", err)
	}
	if _, _, err := readShared(other); err != nil {
		t.Errorf("readShared of a present name = %v", err)
	}
	if err := linkFile(other, filepath.Join(dir, "linked")); err != nil {
		t.Errorf("a link of a present name = %v", err)
	}
}

// An acquisition that meets a lock whose release is still closing its handle — the lock of a running holder, marked
// deleted — tries again at once and takes the lock once the release ends, as it does for a lock that disappears between
// its link and its read.
func TestAcquireAfterPendingRelease(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	plantRecord(t, root, LockPath, selfRecord(t, c, "s1"))
	h := pendDelete(t, c.abs(LockPath))
	closed := false
	defer func() {
		if !closed {
			windows.CloseHandle(h)
		}
	}()
	var attempts *atomic.Int64
	attempts = countAttempts(c, map[string]func(){"lock-attempt": func() {
		// The release ends while the acquisition tries again: its second attempt finds the name free.
		if attempts.Load() == 2 && !closed {
			closed = true
			if err := windows.CloseHandle(h); err != nil {
				t.Error(err)
			}
		}
	}})
	l, err := c.Acquire(context.Background(), "s2", patience)
	if err != nil {
		t.Fatalf("Acquire during a pending release = %v after %d attempts", err, attempts.Load())
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
}
