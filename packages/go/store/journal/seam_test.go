package journal

import (
	"errors"
	"reflect"
	"testing"
)

// The fault seam, installed from outside the package, stops a commit at its step as a crash would, and nil removes
// it; the root is the one declared. The write path above the journal tests its crash points through it.
func TestExportedFaultSeamAndRoot(t *testing.T) {
	root, changes, before, after := fixture(t)
	c, _ := newCorpus(t, root)
	if c.Root() != root {
		t.Fatalf("Root() = %s, want %s", c.Root(), root)
	}
	l := acquire(t, c, "s1")
	var steps []string
	c.SetFault(func(step string, index int) error {
		steps = append(steps, step)
		if step == "prepare" {
			return errCrash
		}
		return nil
	})
	if err := c.Commit(l, changes); !errors.Is(err, errCrash) {
		t.Fatalf("Commit = %v, want the crash at prepare", err)
	}
	if want := []string{"post-images", "pre-images", "prepare"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
	if got := snapshot(t, root); !reflect.DeepEqual(got, before) {
		t.Fatalf("corpus at the crash = %v, want the old state", got)
	}
	c.SetFault(nil)
	wantRecovered(t, c, l, root, after)
}
