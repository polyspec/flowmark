package journal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAwaitNotice pins the test seam that orders a change made outside the engine before the operation that reads it: it
// returns once a notification names the path, a folder above it or an overflow, holds what it took for the next Refresh,
// and fails, naming the path, when no notification names it.
func TestAwaitNotice(t *testing.T) {
	for _, tc := range []struct {
		name    string
		next    Notices
		drain   error
		wantErr string
		dropped bool
	}{
		{name: "the path changed", next: Notices{Changed: []string{"notes/N1.flowmark.md"}}, dropped: true},
		{name: "the path moved", next: Notices{Moved: []string{"notes/N1.flowmark.md"}}, dropped: true},
		{name: "a folder above it moved", next: Notices{Moved: []string{"notes"}}, dropped: true},
		{name: "an overflow", next: Notices{Overflow: true}, dropped: true},
		{name: "the entries of the corpus root changed", next: Notices{Changed: []string{"."}}},
		{name: "another path", next: Notices{Changed: []string{"notes/N2.flowmark.md"}}, wantErr: "no notification named notes/N1.flowmark.md"},
		{name: "a path that only shares a prefix", next: Notices{Moved: []string{"notes/N1"}}, wantErr: "no notification named notes/N1.flowmark.md"},
		{name: "nothing", wantErr: "no notification named notes/N1.flowmark.md"},
		{name: "a failed drain", drain: errors.New("drain"), wantErr: "drain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr, w, _, _ := scriptedTree(t, map[string]string{"notes/N1.flowmark.md": "# One\n"})
			if _, err := tr.ReadFile("notes/N1.flowmark.md"); err != nil {
				t.Fatal(err)
			}
			w.next, w.drainErr = tc.next, tc.drain
			err := tr.AwaitNotice("notes/N1.flowmark.md", time.Second)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("AwaitNotice = %v, want an error holding %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := tr.Refresh(); err != nil {
				t.Fatal(err)
			}
			if _, ok := tr.files.Get("notes/N1.flowmark.md"); ok == tc.dropped {
				t.Fatalf("after the held notification the tree holds the file: %v, want %v", ok, !tc.dropped)
			}
		})
	}
}

// TestAwaitNoticeUnheld: a path the tree holds neither the bytes of nor the entries of its folder, and a tree whose
// notifications a test scripts, need no notification: the seam returns at once without one.
func TestAwaitNoticeUnheld(t *testing.T) {
	tr, _, _, _ := scriptedTree(t, map[string]string{"notes/N1.flowmark.md": "# One\n"})
	if err := tr.AwaitNotice("notes/N1.flowmark.md", time.Second); err != nil {
		t.Fatalf("a path the tree never read: %v", err)
	}
	if _, err := tr.ReadFile("notes/N1.flowmark.md"); err != nil {
		t.Fatal(err)
	}
	tr.SetNotices(func(Notices) Notices { return Notices{} })
	if err := tr.AwaitNotice("notes/N1.flowmark.md", time.Second); err != nil {
		t.Fatalf("a scripted tree: %v", err)
	}
}

// TestAwaitNoticeLive: the platform's own watch delivers the notification of a file written outside the engine, so the
// seam returns on every platform of the matrix, ReadDirectoryChangesW's late report included.
func TestAwaitNoticeLive(t *testing.T) {
	root := t.TempDir()
	tr := liveTree(t, root, &recorder{})
	// The watches of macOS and Linux cover what a read read, so the folder is read before the file is written in it.
	if _, err := tr.ReadDir("."); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "N1.flowmark.md"), []byte("# One\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tr.AwaitNotice("N1.flowmark.md", 30*time.Second); err != nil {
		t.Fatal(err)
	}
}
