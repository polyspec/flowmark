//go:build darwin

package journal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// mountImage creates a disk image of the file system fsType and attaches it at mountpoint, detaching it at the end of
// the test. hdiutil attaches a user's image without privileges.
func mountImage(t *testing.T, fsType, mountpoint string) {
	t.Helper()
	image := filepath.Join(t.TempDir(), "volume.dmg")
	if out, err := exec.Command("hdiutil", "create", "-size", "8m", "-fs", fsType, "-volname", "FMTEST", image).CombinedOutput(); err != nil {
		t.Fatalf("hdiutil create: %v\n%s", err, out)
	}
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-mountpoint", mountpoint, image).CombinedOutput(); err != nil {
		t.Fatalf("hdiutil attach: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		if out, err := exec.Command("hdiutil", "detach", "-force", mountpoint).CombinedOutput(); err != nil {
			t.Errorf("hdiutil detach: %v\n%s", err, out)
		}
	})
}

// REVIEW 1-a: the unlink of another name of the watched lock file (a holder's pending record) starts no attempt; the
// unlink of the lock starts one.
func TestLockWaitPendingUnlinkStartsNoAttempt(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	holder := acquire(t, c, "h")
	alias := filepath.Join(root, ".flowmark", "lock.00000000000000000000000000000099.pending")
	if err := os.Link(lockFile(root), alias); err != nil {
		t.Fatal(err)
	}
	events := make(chan int, 8)
	n := countAttempts(c, nil)
	c.fault = func(step string, index int) error {
		switch step {
		case "lock-attempt":
			n.Add(1)
		case "vnode-event":
			events <- index
		}
		return nil
	}
	ch := acquireAsync(c, "w", patience)
	rec.waitFor(t, 1, kindIs(LockWaiting))
	before := n.Load()
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	select {
	case woke := <-events:
		if woke != 0 {
			t.Fatal("the unlink of the pending name woke the waiter")
		}
	case <-time.After(patience):
		t.Fatal("no vnode event for the unlink of another name")
	}
	if got := n.Load(); got != before {
		t.Fatalf("attempts = %d, want %d", got, before)
	}
	if err := holder.Release(); err != nil {
		t.Fatal(err)
	}
	if r := await(t, ch); r.err != nil {
		t.Fatal(r.err)
	}
	if got := n.Load(); got != before+1 {
		t.Fatalf("attempts = %d, want %d", got, before+1)
	}
}

// exFAT has no hard links: the lock refuses naming that cause.
func TestAcquireOnExFATRefuses(t *testing.T) {
	root := filepath.Join(t.TempDir(), "exfat")
	mountImage(t, "ExFAT", root)
	c, _ := newCorpus(t, root)
	_, err := c.Acquire(context.Background(), "s1", 0)
	if err == nil || !strings.Contains(err.Error(), "hard links") {
		t.Fatalf("Acquire = %v, want the hard-link refusal", err)
	}
	pendings, _ := filepath.Glob(filepath.Join(root, ".flowmark", "lock.*.pending"))
	if len(pendings) != 0 {
		t.Fatalf("pending records left: %v", pendings)
	}
}

// REVIEW 12: a target folder, or a folder to create, on another file system than `.flowmark/tx/` refuses the commit,
// and nothing is staged.
func TestCommitRefusesMountPoint(t *testing.T) {
	root := t.TempDir()
	mountImage(t, "HFS+", filepath.Join(root, "tbl"))
	c, _ := newCorpus(t, root)
	l := acquire(t, c, "s1")
	before := snapshot(t, root)
	for _, path := range []string{"tbl/k1.md", "tbl/sub/k1.md"} {
		err := c.Commit(l, []Change{{Path: path, Data: []byte("k1\n")}})
		if err == nil || !strings.Contains(err.Error(), "a mount point inside the corpus is unsupported") {
			t.Fatalf("Commit(%s) = %v, want the mount-point refusal", path, err)
		}
		if s := stages(t, c); len(s) != 0 {
			t.Fatalf("staged: %v", s)
		}
	}
	if got := snapshot(t, root); !reflect.DeepEqual(got, before) {
		t.Fatalf("corpus changed: %v", got)
	}
	// The accepting case: a folder of the corpus's own file system.
	if err := c.Commit(l, []Change{{Path: "k0.md", Data: []byte("k0\n")}}); err != nil {
		t.Fatal(err)
	}
}
