package journal

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// Two acquisitions of one session in one process write different records — each its own acquisition token — so the
// second never takes the first's lock for its own: it meets a running holder.
func TestSameSessionAcquiresOnce(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	first := acquire(t, c, "s1")
	_, err := c.Acquire(context.Background(), "s1", 0)
	var timeout *LockTimeout
	if !errors.As(err, &timeout) || timeout.State != HolderRunning || timeout.Holder != first.Holder() {
		t.Fatalf("second Acquire = %v, want LOCK_TIMEOUT naming the running first acquisition", err)
	}
	wantKinds(t, rec, LockAcquired)
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
}

// A record that differs from the acquirer's own only in its acquisition token is another acquisition's lock.
func TestRecordOfAnotherAcquisitionRefused(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	other := selfRecord(t, c, "s1")
	plantRecord(t, root, LockPath, other)
	_, err := c.Acquire(context.Background(), "s1", 0)
	var timeout *LockTimeout
	if !errors.As(err, &timeout) || timeout.State != HolderRunning || timeout.Holder != other {
		t.Fatalf("Acquire = %v, want LOCK_TIMEOUT naming the planted acquisition", err)
	}
}

// Release removes only its own acquisition's lock: a lock replaced by a record of the same session and another
// acquisition is left in place.
func TestReleaseRefusesSameSessionOtherAcquisition(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	l := acquire(t, c, "s1")
	other := selfRecord(t, c, "s1")
	if other.Acquisition == l.Holder().Acquisition {
		t.Fatal("two acquisitions share a token")
	}
	plantRecord(t, root, LockPath, other)
	if err := l.Release(); err == nil || !strings.Contains(err.Error(), "replaced") {
		t.Fatalf("Release = %v, want a replaced-lock error", err)
	}
	data, readErr := os.ReadFile(lockFile(root))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(other.record()) {
		t.Fatalf("lock file changed to %q", data)
	}
	wantKinds(t, rec, LockAcquired)
}
