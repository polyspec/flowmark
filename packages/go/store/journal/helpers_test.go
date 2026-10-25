package journal

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// patience bounds every wait of a test for something the code under test must publish; it is a guard against a hung
// test, never the condition a test checks.
const patience = 60 * time.Second

// recorder is the observer of a test: it keeps every event in order.
type recorder struct{ Recorder }

func (r *recorder) kinds() []Kind {
	kinds := []Kind{}
	for _, e := range r.Events() {
		kinds = append(kinds, e.Kind)
	}
	return kinds
}

func (r *recorder) reset() { r.Reset() }

// waitFor blocks until n published events satisfy match and returns them.
func (r *recorder) waitFor(t *testing.T, n int, match func(Event) bool) []Event {
	t.Helper()
	found, ok := r.Wait(n, match, patience)
	if !ok {
		t.Fatalf("waited %s for %d events; got %v", patience, n, r.kinds())
	}
	return found
}

func kindIs(k Kind) func(Event) bool { return func(e Event) bool { return e.Kind == k } }

func wantKinds(t *testing.T, r *recorder, want ...Kind) {
	t.Helper()
	if want == nil {
		want = []Kind{}
	}
	if got := r.kinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func newCorpus(t *testing.T, root string) (*Corpus, *recorder) {
	t.Helper()
	rec := &recorder{}
	c, err := New(root, rec)
	if err != nil {
		t.Fatalf("New(%s): %v", root, err)
	}
	return c, rec
}

func acquire(t *testing.T, c *Corpus, session string) *Lock {
	t.Helper()
	l, err := c.Acquire(context.Background(), session, 0)
	if err != nil {
		t.Fatalf("Acquire(%s): %v", session, err)
	}
	return l
}

// countAttempts installs a fault seam that counts the acquisition attempts of c and calls each hook at its step.
func countAttempts(c *Corpus, hooks map[string]func()) *atomic.Int64 {
	var n atomic.Int64
	c.fault = func(step string, _ int) error {
		if step == "lock-attempt" {
			n.Add(1)
		}
		if h := hooks[step]; h != nil {
			h()
		}
		return nil
	}
	return &n
}

func lockFile(root string) string { return filepath.Join(root, ".flowmark", "lock") }

func writeFile(t *testing.T, root, rel, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// snapshot returns every file under root outside .flowmark/ by slash path, with a symbolic link as its target and a
// folder as "folder".
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == ".flowmark" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			files[filepath.ToSlash(rel)] = "symlink to " + target
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func stages(t *testing.T, c *Corpus) []string {
	t.Helper()
	entries, err := os.ReadDir(c.txDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// symlink creates a symbolic link or skips the test where this platform refuses to create one without privileges
// (Windows without developer mode); the refusal it tests cannot arise where no link can be made.
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("the platform refuses a symbolic link, which the refusal test needs: %v", err)
	}
}

// selfRecord returns the holder record this process writes for session.
func selfRecord(t *testing.T, c *Corpus, session string) Holder {
	t.Helper()
	h, err := c.self(session)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// plantRecord writes the holder record h at rel, as a holder or a waiter would have written it.
func plantRecord(t *testing.T, root, rel string, h Holder) {
	t.Helper()
	writeFile(t, root, rel, string(h.record()))
}

// wantRecovered requires the recovery of c under the lock l to succeed and to leave the corpus at root as want.
func wantRecovered(t *testing.T, c *Corpus, l *Lock, root string, want map[string]string) {
	t.Helper()
	if err := c.Recover(l); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("corpus = %v, want %v", got, want)
	}
}

// wantCommitted requires the commit of changes by c under the lock l to succeed and to leave the corpus at root as want.
func wantCommitted(t *testing.T, c *Corpus, l *Lock, changes []Change, root string, want map[string]string) {
	t.Helper()
	if err := c.Commit(l, changes); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("corpus = %v, want %v", got, want)
	}
}

// refusesLinkedStage opens the corpus at root, whose journal folder holds a symbolic link, takes its lock, and requires
// the recovery to refuse the link; it returns the corpus and the lock.
func refusesLinkedStage(t *testing.T, root string) (*Corpus, *Lock) {
	t.Helper()
	c, _ := newCorpus(t, root)
	l := acquire(t, c, "s1")
	if err := c.Recover(l); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Recover = %v, want a symlink refusal", err)
	}
	return c, l
}

// otherHostLock returns the root of a fresh corpus whose lock file names a holder on another host, the corpus and the
// holder.
func otherHostLock(t *testing.T) (string, *Corpus, Holder) {
	t.Helper()
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	other := selfRecord(t, c, "s9")
	other.Host = "another-machine"
	plantRecord(t, root, LockPath, other)
	return root, c, other
}

// wantReplacedRelease plants the record other as the lock file of root and requires the release of l to refuse the
// replaced lock and to leave the file as planted.
func wantReplacedRelease(t *testing.T, l *Lock, root string, other Holder) {
	t.Helper()
	plantRecord(t, root, LockPath, other)
	if err := l.Release(); err == nil || !strings.Contains(err.Error(), "replaced") {
		t.Fatalf("Release = %v, want a replaced-lock error", err)
	}
	data, err := os.ReadFile(lockFile(root))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(other.record()) {
		t.Fatalf("lock file changed to %q", data)
	}
}

// wantLinkErrors requires linkUnsupported to read the failure of a link with each errno of cases as its want, a refusal
// for want of hard links, and linkError to name that cause for each such errno.
func wantLinkErrors(t *testing.T, cases map[error]bool) {
	t.Helper()
	for errno, want := range cases {
		err := &os.LinkError{Op: "link", Old: "a", New: "b", Err: errno}
		if got := linkUnsupported(err); got != want {
			t.Errorf("linkUnsupported(%v) = %v, want %v", errno, got, want)
		}
		if want && !strings.Contains(linkError(err).Error(), "hard links") {
			t.Errorf("linkError(%v) = %v, want the hard-link cause", errno, linkError(err))
		}
	}
}
