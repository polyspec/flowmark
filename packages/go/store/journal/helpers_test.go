package journal

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// patience bounds every wait of a test for something the code under test must publish; it is a guard against a hung
// test, never the condition a test checks.
const patience = 60 * time.Second

type recorder struct {
	mu     sync.Mutex
	events []Event
	notify chan struct{}
}

func (r *recorder) Observe(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	if r.notify != nil {
		close(r.notify)
		r.notify = nil
	}
}

func (r *recorder) kinds() []Kind {
	r.mu.Lock()
	defer r.mu.Unlock()
	kinds := []Kind{}
	for _, e := range r.events {
		kinds = append(kinds, e.Kind)
	}
	return kinds
}

func (r *recorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

// waitFor blocks until n published events satisfy match and returns them.
func (r *recorder) waitFor(t *testing.T, n int, match func(Event) bool) []Event {
	t.Helper()
	deadline := time.After(patience)
	for {
		r.mu.Lock()
		var found []Event
		for _, e := range r.events {
			if match(e) {
				found = append(found, e)
			}
		}
		if len(found) >= n {
			r.mu.Unlock()
			return found
		}
		if r.notify == nil {
			r.notify = make(chan struct{})
		}
		ch := r.notify
		r.mu.Unlock()
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("waited %s for %d events; got %v", patience, n, r.kinds())
		}
	}
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
