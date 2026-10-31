//go:build windows

package journal

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// newHandleWatch returns a windowsWatch with its rearm event and no run: the test drives armHolder and reads the
// handles it holds. The caller closes it with closeHandles.
func newHandleWatch(t *testing.T) *windowsWatch {
	t.Helper()
	rearm, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &windowsWatch{ch: make(chan wake, wakeBuffer), done: make(chan struct{}), rearm: rearm}
}

// handleOpen reports whether h is still open: a wait on a closed handle fails, one on an open process handle does not.
func handleOpen(t *testing.T, h windows.Handle) bool {
	t.Helper()
	event, _ := windows.WaitForSingleObject(h, 0)
	return event != windows.WAIT_FAILED
}

// TestReplacedHolderHandleStaysOpen: armHolder replaces the holder's process handle, and the replaced handle stays
// open. run's wait may still hold it; run closes it only at the top of its next loop, after that wait has returned.
func TestReplacedHolderHandleStaysOpen(t *testing.T) {
	w := newHandleWatch(t)
	defer func() { _ = w.closeHandles() }()
	self := Holder{PID: os.Getpid()}
	if err := w.armHolder(self); err != nil {
		t.Fatal(err)
	}
	first := w.proc
	if first == 0 {
		t.Fatal("armHolder holds no process handle for a live process")
	}
	if err := w.armHolder(self); err != nil {
		t.Fatal(err)
	}
	if w.proc == first {
		t.Fatal("armHolder kept the first process handle")
	}
	if !handleOpen(t, first) {
		t.Fatal("armHolder closed the replaced process handle, which a wait in run may hold")
	}
}

// TestWatchStopClosesReplacedHandles: stopping the watch closes the handles that armHolder replaced, so none leaks.
func TestWatchStopClosesReplacedHandles(t *testing.T) {
	w := newHandleWatch(t)
	self := Holder{PID: os.Getpid()}
	if err := w.armHolder(self); err != nil {
		t.Fatal(err)
	}
	first := w.proc
	if err := w.armHolder(self); err != nil {
		t.Fatal(err)
	}
	if err := w.closeHandles(); err != nil {
		t.Fatal(err)
	}
	if handleOpen(t, first) {
		t.Fatal("stopping the watch left the replaced process handle open")
	}
}
