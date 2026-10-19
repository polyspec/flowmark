// The watch of macOS holds one descriptor for every folder and document (spec 40, the scale envelope), so these tests
// hold only there; the watch of Linux holds one inotify descriptor and counts its watches against
// fs.inotify.max_user_watches, which a test cannot lower without privilege.
//go:build darwin

package writepath

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store/journal"
	"golang.org/x/sys/unix"
)

// openDescriptors returns the number of descriptors the process holds.
func openDescriptors(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

// limitDescriptors lowers the soft limit of open files to room more than the process holds now, and returns the
// function that restores it.
func limitDescriptors(t *testing.T, room int) (restore func()) {
	t.Helper()
	var old unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &old); err != nil {
		t.Fatal(err)
	}
	limit := old
	limit.Cur = uint64(openDescriptors(t) + room)
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &old); err != nil {
			t.Errorf("restoring the limit of open files: %v", err)
		}
	}
}

// TestOpenWithoutDescriptors pins that opening a corpus that the process cannot watch whole releases the corpus lock
// (spec 40 Opening a corpus, the lock): on macOS the watch holds a descriptor for every folder and document, and
// releasing the lock opens the lock file, so the watch is closed before the lock is released. The next open takes
// the lock without waiting.
func TestOpenWithoutDescriptors(t *testing.T) {
	docs := map[string]string{}
	for i := 0; i < 120; i++ {
		docs[fmt.Sprintf("cards/C%d.flowmark.md", i)] = fmt.Sprintf("# Card %d\n", i)
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{"dbspec/test.dbs": cardsDBS})
	writeTree(t, root, docs)
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("catalog: %+v", diags)
	}
	open := func() (*Corpus, error) {
		j, err := journal.New(root, &recorder{})
		if err != nil {
			t.Fatal(err)
		}
		return Open(context.Background(), cat, j, 0)
	}
	func() {
		defer limitDescriptors(t, 40)()
		if c, err := open(); err == nil {
			c.Close()
			t.Fatal("40#the-lock: the corpus opened with fewer descriptors than it has documents; want the open to fail")
		}
	}()
	if _, err := os.Lstat(filepath.Join(root, ".flowmark", "lock")); err == nil {
		t.Fatal("40#the-lock: a corpus that failed to open leaves its lock file")
	}
	c, err := open()
	if err != nil {
		t.Fatalf("40#the-lock: the open after the failed one: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestWatchDescriptors pins what the watch of an opened corpus costs on a platform where it holds a descriptor for
// every folder and document (spec 40 Opening a corpus): the folders and the documents of the corpus, and nothing
// that grows with the operations. It is the figure the scale envelope states (T12.5-3), so a change of the watch
// shows here.
func TestWatchDescriptors(t *testing.T) {
	const documents = 150
	docs := map[string]string{}
	for i := 0; i < documents; i++ {
		docs[fmt.Sprintf("cards/C%d.flowmark.md", i)] = fmt.Sprintf("# Card %d\n", i)
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{"dbspec/test.dbs": cardsDBS})
	writeTree(t, root, docs)
	cat, diags := catalog.Load(root)
	if len(diags) > 0 || cat == nil {
		t.Fatalf("catalog: %+v", diags)
	}
	before := openDescriptors(t)
	j, err := journal.New(root, &recorder{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Open(context.Background(), cat, j, 0)
	if err != nil {
		t.Fatal(err)
	}
	held := openDescriptors(t) - before
	// The documents, the table folder and the corpus root, and the few descriptors of the kqueue, the lock and the
	// journal, which do not grow with the corpus.
	if held < documents || held > documents+10 {
		t.Errorf("40#opening-a-corpus: the watch of %d documents holds %d descriptors; want the documents and at most ten more", documents, held)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if after := openDescriptors(t) - before; after != 0 {
		t.Errorf("40#opening-a-corpus: %d descriptors stay open after the corpus is closed", after)
	}
}
