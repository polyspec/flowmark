package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func staleLock(t *testing.T, c *Corpus, root string) Holder {
	t.Helper()
	gone := selfRecord(t, c, "gone")
	gone.PID = endedPID(t)
	plantRecord(t, root, LockPath, gone)
	return gone
}

func claimFile(root string, h Holder) string {
	s := sha256.Sum256(h.record())
	return filepath.Join(root, ".flowmark", "lock."+hex.EncodeToString(s[:])[:32]+".clearing")
}

func TestUnlockRemovesEndedHolder(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	gone := staleLock(t, c, root)
	got, err := c.Unlock("")
	if err != nil {
		t.Fatal(err)
	}
	if got != gone {
		t.Fatalf("Unlock removed %v, want %v", got, gone)
	}
	for _, p := range []string{lockFile(root), claimFile(root, gone)} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s after the unlock: %v", p, err)
		}
	}
	acquire(t, c, "s1")
}

func TestUnlockPublishesEvent(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	gone := staleLock(t, c, root)
	if _, err := c.Unlock(""); err != nil {
		t.Fatal(err)
	}
	wantKinds(t, rec, LockCleared)
	if e := rec.Events()[0]; e.Holder != gone.String() || e.Path != lockFile(root) {
		t.Fatalf("event = %+v", e)
	}
}

func TestUnlockRefusesRunningHolder(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	l := acquire(t, c, "s1")
	if _, err := c.Unlock(""); err == nil || !strings.Contains(err.Error(), "runs") {
		t.Fatalf("Unlock = %v, want a running-holder refusal", err)
	}
	if _, err := c.Unlock(l.Holder().String()); err == nil {
		t.Fatal("Unlock with --holder removed a running holder")
	}
	data, readErr := os.ReadFile(lockFile(root))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(l.Holder().record()) {
		t.Fatalf("lock = %q", data)
	}
	wantKinds(t, rec, LockAcquired)
	if _, err := os.Lstat(claimFile(root, l.Holder())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("claim left: %v", err)
	}
	// No lock: nothing to unlock.
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Unlock(""); err == nil {
		t.Fatal("Unlock of a free lock accepted")
	}
}

// A holder on another host is removed only when --holder names it.
func TestUnlockOtherHostRequiresHolder(t *testing.T) {
	root, c, other := otherHostLock(t)
	if _, err := c.Unlock(""); err == nil || !strings.Contains(err.Error(), "--holder") {
		t.Fatalf("Unlock = %v, want a refusal asking for --holder", err)
	}
	if _, err := c.Unlock("1@another-machine/s8"); err == nil {
		t.Fatal("Unlock accepted another holder's name")
	}
	got, err := c.Unlock(other.String())
	if err != nil || got != other {
		t.Fatalf("Unlock = %v, %v", got, err)
	}
	if _, err := os.Lstat(lockFile(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock: %v", err)
	}
}

// Two unlocks of one stale lock never leave two holders: whatever their interleaving with a new acquisition, the
// new holder's lock stays.
func TestUnlockTwoAtOnceNeverTwoHolders(t *testing.T) {
	for _, tc := range []struct {
		name string
		// firstDone: the first unlock has removed its claim before the second one links.
		firstDone bool
	}{{"claim held", false}, {"claim removed", true}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			c, _ := newCorpus(t, root)
			gone := staleLock(t, c, root)
			// The second unlock reads R, then pauses until the first has unlocked and a new holder has acquired.
			secondRead := make(chan struct{})
			resume := make(chan struct{})
			firstRemoved := make(chan struct{})
			finishFirst := make(chan struct{})
			var once sync.Once
			var second sync.Once
			c.fault = func(step string, _ int) error {
				switch step {
				case "unlock-read":
					// The first caller to read is the second unlock (started first), which pauses.
					paused := false
					second.Do(func() { paused = true })
					if paused {
						close(secondRead)
						<-resume
					}
				case "unlock-removed":
					once.Do(func() {
						close(firstRemoved)
						if !tc.firstDone {
							<-finishFirst
						}
					})
				}
				return nil
			}
			secondErr := make(chan error, 1)
			go func() {
				_, err := c.Unlock("")
				secondErr <- err
			}()
			<-secondRead
			firstErr := make(chan error, 1)
			go func() {
				_, err := c.Unlock("")
				firstErr <- err
			}()
			<-firstRemoved
			if tc.firstDone {
				if err := <-firstErr; err != nil {
					t.Fatal(err)
				}
			}
			newHolder := acquire(t, c, "new")
			close(resume)
			if err := <-secondErr; err == nil {
				t.Fatal("the second unlock succeeded")
			}
			if !tc.firstDone {
				close(finishFirst)
				if err := <-firstErr; err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(lockFile(root))
			if err != nil || string(data) != string(newHolder.Holder().record()) {
				t.Fatalf("lock = %q, %v; want the new holder %v", data, err, newHolder.Holder())
			}
			if _, err := os.Lstat(claimFile(root, gone)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("claim left: %v", err)
			}
		})
	}
}
