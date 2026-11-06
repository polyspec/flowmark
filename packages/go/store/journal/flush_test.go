package journal

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
)

// flushCase is one operation whose flushes TestFlushes pins: what it writes on a corpus that holds a.flowmark.md,
// b.flowmark.md and the folder notes/, once the lock has been taken and released (so `.flowmark/` and `.flowmark/tx/`
// exist).
type flushCase struct {
	name    string
	changes []Change
	// unix and windows are the flushes of the acquisition, the commit and the release, on Linux and macOS and on Windows.
	unix, windows int64
}

var flushCases = []flushCase{
	{"the lock alone", nil, 1, 1},
	{"an update of one file", []Change{{Path: "a.flowmark.md", Data: []byte("a2\n")}}, 18, 15},
	{"a create in a folder that exists", []Change{{Path: "notes/c.flowmark.md", Data: []byte("c\n")}}, 16, 13},
	{"a delete of one file", []Change{{Path: "b.flowmark.md", Delete: true}}, 14, 11},
	{"a create in a folder that does not exist", []Change{{Path: "new/d.flowmark.md", Data: []byte("d\n")}}, 17, 14},
	{"an update of two files", []Change{{Path: "a.flowmark.md", Data: []byte("a2\n")}, {Path: "b.flowmark.md", Data: []byte("b2\n")}}, 26, 22},
}

// TestFlushes pins the flushes of each operation of the journal, which its corpus counts (T55; plan, Ruling: a commit
// waits for the disk as few times as its atomicity needs), and counts none while the process does not wait for the
// disk.
func TestFlushes(t *testing.T) {
	defer SetSynchronous(!asynchronous.Load())
	for _, synchronous := range []bool{true, false} {
		SetSynchronous(synchronous)
		for _, tc := range flushCases {
			want := tc.unix
			if runtime.GOOS == "windows" {
				want = tc.windows
			}
			if !synchronous {
				want = 0
			}
			root := t.TempDir()
			for rel, data := range map[string]string{"a.flowmark.md": "a\n", "b.flowmark.md": "b\n", "notes/n.flowmark.md": "n\n"} {
				p := filepath.Join(root, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			c, _ := newCorpus(t, root)
			release(t, acquire(t, c, "warm"))
			if err := c.ensureDir(txRel); err != nil {
				t.Fatal(err)
			}
			before := c.Cost().Snapshot()
			l := acquire(t, c, "flush")
			if tc.changes != nil {
				if err := c.Commit(l, tc.changes); err != nil {
					t.Fatalf("%s: %v", tc.name, err)
				}
			}
			release(t, l)
			if got := c.Cost().Snapshot().Since(before).Get(cost.Flushes); got != want {
				t.Errorf("%s, synchronous %v: %d flushes, want %d", tc.name, synchronous, got, want)
			}
		}
	}
}

func release(t *testing.T, l *Lock) {
	t.Helper()
	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}
