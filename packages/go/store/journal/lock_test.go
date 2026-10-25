package journal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Spec 40, the lock: the lock file is `.flowmark/lock` under the corpus root, a constant; `.flowmark/` is created
// when absent.
func TestLockPathIsDeclared(t *testing.T) {
	if LockPath != ".flowmark/lock" {
		t.Fatalf("LockPath = %q", LockPath)
	}
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	l := acquire(t, c, "s1")
	if _, err := os.Lstat(lockFile(root)); err != nil {
		t.Fatalf("the lock file: %v", err)
	}
	wantKinds(t, rec, LockAcquired)
	if e := rec.Events()[0]; e.Path != lockFile(root) || e.Holder != l.Holder().String() {
		t.Fatalf("event = %+v", e)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".flowmark"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf(".flowmark/ holds %v, want the lock alone (the pending record removed)", entries)
	}
}

// The record is one line of JSON {pid, start, host, hostname, session, acquisition}; the holder is named
// `<pid>@<host>/<session>`, and the acquisition is the 32 hex digits of its pending record's name.
func TestHolderRecordJSON(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	l := acquire(t, c, "s1")
	h := l.Holder()
	if h.PID != os.Getpid() || h.Session != "s1" || h.Start == "" || h.Host == "" || h.Hostname == "" || !isHex(h.Acquisition, 32) {
		t.Fatalf("holder = %+v", h)
	}
	want := fmt.Sprintf(`{"pid":%d,"start":%q,"host":%q,"hostname":%q,"session":"s1","acquisition":%q}`+"\n", h.PID, h.Start, h.Host, h.Hostname, h.Acquisition)
	data, err := os.ReadFile(lockFile(root))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("lock file = %q, want %q", data, want)
	}
	if got := h.String(); got != fmt.Sprintf("%d@%s/s1", h.PID, h.Host) {
		t.Fatalf("name = %q", got)
	}
	back, err := parseRecord(data)
	if err != nil || back != h {
		t.Fatalf("parseRecord = %+v, %v", back, err)
	}
	for _, bad := range []string{
		"",
		"garbage\n",
		strings.TrimSuffix(want, "\n"),
		want + "\n",
		strings.Replace(want, `"pid":`, `"pid": `, 1),
		strings.Replace(want, `,"session":"s1"`, ``, 1),
		strings.Replace(want, `"session":"s1"`, `"session":""`, 1),
		strings.Replace(want, `"session":"s1"`, `"session":"s1","extra":1`, 1),
		strings.Replace(want, fmt.Sprintf(`"pid":%d`, h.PID), `"pid":0`, 1),
		strings.Replace(want, fmt.Sprintf(`"pid":%d`, h.PID), `"pid":-4`, 1),
		strings.Replace(want, `"session":"s1"`, `"session":"a/b"`, 1),
		strings.Replace(want, fmt.Sprintf(`,"acquisition":%q`, h.Acquisition), ``, 1),
		strings.Replace(want, h.Acquisition, h.Acquisition[:31], 1),
		strings.Replace(want, h.Acquisition, strings.ToUpper(h.Acquisition), 1),
	} {
		if _, err := parseRecord([]byte(bad)); err == nil {
			t.Errorf("parseRecord(%q) accepted", bad)
		}
	}
}

// Release removes the lock file; releasing again is a no-op that publishes nothing.
func TestReleaseIsIdempotent(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	l := acquire(t, c, "s1")
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(lockFile(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock file after release: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("second release: %v", err)
	}
	wantKinds(t, rec, LockAcquired, LockReleased)
	acquire(t, c, "s2")
}

// A held lock refuses with LockTimeout naming its running holder; the lock is untouched.
func TestAcquireHeldLockRefused(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	first := acquire(t, c, "s1")
	_, err := c.Acquire(context.Background(), "s2", 0)
	var timeout *LockTimeout
	if !errors.As(err, &timeout) {
		t.Fatalf("second Acquire = %v, want *LockTimeout", err)
	}
	if timeout.State != HolderRunning || timeout.Holder != first.Holder() {
		t.Fatalf("timeout = %+v, want running %+v", timeout, first.Holder())
	}
	if !strings.Contains(err.Error(), first.Holder().String()) {
		t.Fatalf("message %q does not name the holder", err)
	}
	wantKinds(t, rec, LockAcquired)
	data, readErr := os.ReadFile(lockFile(root))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(first.Holder().record()) {
		t.Fatalf("lock file changed to %q", data)
	}
}

// A lock whose holder has ended is reported as ended, naming `flowmark unlock`, and never removed.
func TestAcquireStaleLockReported(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	gone := selfRecord(t, c, "gone")
	gone.PID = endedPID(t)
	plantRecord(t, root, LockPath, gone)
	_, err := c.Acquire(context.Background(), "s1", 0)
	var timeout *LockTimeout
	if !errors.As(err, &timeout) || timeout.State != HolderEnded || timeout.Holder != gone {
		t.Fatalf("Acquire = %v, want the ended holder %v", err, gone)
	}
	for _, part := range []string{"ended", gone.String(), "flowmark unlock " + root} {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("message %q does not name %q", err, part)
		}
	}
	data, readErr := os.ReadFile(lockFile(root))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(gone.record()) {
		t.Fatalf("stale lock changed to %q", data)
	}
	wantKinds(t, rec)
}

// A holder on another host cannot be checked here: its state is unknown, never judged ended.
func TestAcquireOtherHostUnknown(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	other := selfRecord(t, c, "s9")
	other.Host = "another-machine"
	plantRecord(t, root, LockPath, other)
	_, err := c.Acquire(context.Background(), "s1", 0)
	var timeout *LockTimeout
	if !errors.As(err, &timeout) || timeout.State != HolderUnknown {
		t.Fatalf("Acquire = %v, want an unknown holder", err)
	}
	if !strings.Contains(err.Error(), "--holder "+other.String()) {
		t.Fatalf("message %q does not name the unlock of another host", err)
	}
}

// A lock file that holds no record is an error and never a free lock.
func TestAcquireMalformedRecord(t *testing.T) {
	for _, text := range []string{"", "\n", "garbage\n", "12@h/s\n", `{"pid":1}` + "\n"} {
		t.Run(fmt.Sprintf("%q", text), func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, LockPath, text)
			c, _ := newCorpus(t, root)
			_, err := c.Acquire(context.Background(), "s1", 0)
			var timeout *LockTimeout
			if err == nil || errors.As(err, &timeout) {
				t.Fatalf("Acquire = %v, want a malformed-record error", err)
			}
			data, readErr := os.ReadFile(lockFile(root))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(data) != text {
				t.Fatalf("lock file changed to %q", data)
			}
		})
	}
}

// REVIEW 2-a: a link whose reply was lost leaves the acquirer's own record in the lock; that is the lock held.
func TestAcquireLostReplyOwnRecord(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	// The link takes effect before the attempt's own link runs, as a link whose reply was lost: the attempt's link
	// fails with EEXIST and finds its own record.
	linked := false
	c.SetFault(func(step string, _ int) error {
		if step != "lock-attempt" || linked {
			return nil
		}
		linked = true
		entries, err := os.ReadDir(filepath.Join(root, ".flowmark"))
		if err != nil {
			return err
		}
		for _, e := range entries {
			if pendingName.MatchString(e.Name()) {
				return os.Link(filepath.Join(root, ".flowmark", e.Name()), lockFile(root))
			}
		}
		return errors.New("no pending record to link")
	})
	l, err := c.Acquire(context.Background(), "s1", 0)
	if err != nil {
		t.Fatalf("Acquire = %v, want the lock held by its own record", err)
	}
	data, readErr := os.ReadFile(lockFile(root))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !linked || string(data) != string(l.Holder().record()) {
		t.Fatalf("lock = %q, holder %+v", data, l.Holder())
	}
	wantKinds(t, rec, LockAcquired)
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
}

// Only the holder releases: a lock file replaced by another record is not removed.
func TestReleaseReplacedLockRefused(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	l := acquire(t, c, "s1")
	other := selfRecord(t, c, "s9")
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

func TestAcquireRejectsBadArguments(t *testing.T) {
	for _, session := range []string{"", "a\nb", "a\rb", "a/b"} {
		t.Run(fmt.Sprintf("%q", session), func(t *testing.T) {
			c, rec := newCorpus(t, t.TempDir())
			if _, err := c.Acquire(context.Background(), session, 0); err == nil {
				t.Fatal("Acquire accepted")
			}
			wantKinds(t, rec)
		})
	}
	c, _ := newCorpus(t, t.TempDir())
	if _, err := c.Acquire(context.Background(), "s", -1); err == nil {
		t.Fatal("Acquire accepted a negative wait")
	}
}

// A symlink where the lock file must be is refused, never followed.
func TestAcquireRefusesSymlink(t *testing.T) {
	t.Run("lock file", func(t *testing.T) {
		root := t.TempDir()
		elsewhere := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".flowmark"), 0o755); err != nil {
			t.Fatal(err)
		}
		symlink(t, filepath.Join(elsewhere, "x"), lockFile(root))
		c, _ := newCorpus(t, root)
		if _, err := c.Acquire(context.Background(), "s1", 0); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Acquire = %v, want a symlink refusal", err)
		}
	})
	t.Run("parent folder", func(t *testing.T) {
		root := t.TempDir()
		elsewhere := t.TempDir()
		symlink(t, elsewhere, filepath.Join(root, ".flowmark"))
		c, _ := newCorpus(t, root)
		if _, err := c.Acquire(context.Background(), "s1", 0); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Acquire = %v, want a symlink refusal", err)
		}
		entries, readErr := os.ReadDir(elsewhere)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Fatalf("the link target was written: %v", entries)
		}
	})
}

func TestNewRejectsRoot(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	cases := map[string]string{
		"relative":    "corpus",
		"unclean":     dir + string(filepath.Separator) + "." + string(filepath.Separator),
		"missing":     filepath.Join(dir, "missing"),
		"file":        file,
		"symlink":     link,
		"no observer": "",
	}
	for name, root := range cases {
		t.Run(name, func(t *testing.T) {
			var obs Observer = &recorder{}
			switch name {
			case "symlink":
				symlink(t, dir, link)
			case "no observer":
				root, obs = dir, nil
			}
			if _, err := New(root, obs); err == nil {
				t.Fatal("New accepted")
			}
		})
	}
	if _, err := New(dir, &recorder{}); err != nil {
		t.Fatalf("New(%s) = %v", dir, err)
	}
}

// Opening removes the pending record of an ended holder.
func TestOpenRemovesEndedPendingRecord(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	gone := selfRecord(t, c, "gone")
	gone.PID = endedPID(t)
	pending := ".flowmark/lock.0123456789abcdef0123456789abcdef.pending"
	plantRecord(t, root, pending, gone)
	l := acquire(t, c, "s1")
	if err := c.Recover(l); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(pending))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the ended pending record: %v", err)
	}
}

// Opening keeps the pending record of a running holder (a waiter) and of a holder on another host, and leaves an
// unlock's claim alone.
func TestOpenKeepsRunningPendingRecord(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	waiter := selfRecord(t, c, "waiter")
	other := selfRecord(t, c, "other")
	other.Host = "another-machine"
	keep := map[string]Holder{
		".flowmark/lock.00000000000000000000000000000001.pending":  waiter,
		".flowmark/lock.00000000000000000000000000000002.pending":  other,
		".flowmark/lock.00000000000000000000000000000003.clearing": other,
	}
	for rel, h := range keep {
		plantRecord(t, root, rel, h)
	}
	l := acquire(t, c, "s1")
	if err := c.Recover(l); err != nil {
		t.Fatal(err)
	}
	for rel := range keep {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
	}
}
