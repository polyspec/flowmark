package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A commit that stops says whether its prepare record is durable: before it the commit wrote nothing and recovery
// discards its stage; from it on the commit is decided and recovery completes it. The cause stays reachable.
func TestCommitErrorSaysPrepared(t *testing.T) {
	for _, p := range crashPoints {
		t.Run(fmt.Sprintf("%s#%d", p.step, p.index), func(t *testing.T) {
			root, changes, _, _ := fixture(t)
			c, _ := newCorpus(t, root)
			l := acquire(t, c, "s1")
			c.SetFault(crashAt(p.step, p.index))
			err := c.Commit(l, changes)
			var ce *CommitError
			if !errors.As(err, &ce) || !errors.Is(err, errCrash) {
				t.Fatalf("Commit = %v, want a *CommitError wrapping the crash", err)
			}
			if ce.Prepared != p.new {
				t.Fatalf("Prepared = %v, want %v", ce.Prepared, p.new)
			}
		})
	}
	t.Run("a refusal before staging", func(t *testing.T) {
		root, _, _, _ := fixture(t)
		c, _ := newCorpus(t, root)
		l := acquire(t, c, "s1")
		err := c.Commit(l, []Change{{Path: "../out.md", Data: []byte("x\n")}})
		var ce *CommitError
		if !errors.As(err, &ce) || ce.Prepared {
			t.Fatalf("Commit = %v, want an unprepared *CommitError", err)
		}
		if _, err := os.Lstat(filepath.Join(root, ".flowmark", "tx")); err == nil {
			entries, _ := os.ReadDir(filepath.Join(root, ".flowmark", "tx"))
			if len(entries) != 0 {
				t.Fatalf("stages %v", entries)
			}
		}
	})
}
