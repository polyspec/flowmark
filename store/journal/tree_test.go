package journal

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// scriptedWatch is a tree watch whose notifications and failures a test sets.
type scriptedWatch struct {
	watched  []string
	fail     error
	drainErr error
	next     Notices
	closed   bool
}

func (w *scriptedWatch) watch(rel string, dir bool) error {
	if w.fail != nil {
		return w.fail
	}
	w.watched = append(w.watched, rel)
	return nil
}

func (w *scriptedWatch) drain() (Notices, error) {
	n, err := w.next, w.drainErr
	w.next, w.drainErr = Notices{}, nil
	return n, err
}

func (w *scriptedWatch) close() error {
	w.closed = true
	return nil
}

// scriptedTree returns a tree of a corpus holding files, watched by a scripted watch, and the observer of its events.
func scriptedTree(t *testing.T, files map[string]string) (*Tree, *scriptedWatch, *recorder, string) {
	t.Helper()
	root := t.TempDir()
	for p, text := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rec := &recorder{}
	c, err := New(root, rec)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := c.Tree()
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.w.close(); err != nil {
		t.Fatal(err)
	}
	w := &scriptedWatch{}
	tr.w = w
	return tr, w, rec, root
}

// reads returns the corpus paths of the corpus.read events since the last reset, and resets.
func reads(t *testing.T, rec *recorder, root string) []string {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	out := []string{}
	for _, e := range rec.events {
		if e.Kind == CorpusRead {
			rel, err := filepath.Rel(root, e.Path)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, filepath.ToSlash(rel))
		}
	}
	rec.events = nil
	return out
}

// readAll reads the folders and files of paths through the tree.
func readAll(t *testing.T, tr *Tree, paths ...string) {
	t.Helper()
	for _, p := range paths {
		var err error
		if strings.HasSuffix(p, "/") || p == "." {
			_, err = tr.ReadDir(strings.TrimSuffix(p, "/"))
		} else {
			_, err = tr.ReadFile(p)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTreeKeepsWhatItRead(t *testing.T) {
	files := map[string]string{"a.flowmark.md": "a\n", "t/x.flowmark.md": "x\n", "t/y.flowmark.md": "y\n", "t/s/z.flowmark.md": "z\n"}
	all := []string{".", "t/", "t/s/", "a.flowmark.md", "t/x.flowmark.md", "t/y.flowmark.md", "t/s/z.flowmark.md"}
	cases := []struct {
		name string
		n    Notices
		// again is what the next read of every path reads from the disk.
		again []string
	}{
		{name: "40#opening-a-corpus: no notification reads nothing again", again: []string{}},
		{name: "40#opening-a-corpus: a changed file is read again", n: Notices{Changed: []string{"t/x.flowmark.md"}},
			again: []string{"t/x.flowmark.md"}},
		{name: "40#opening-a-corpus: a changed folder reads its entries again, not its files", n: Notices{Changed: []string{"t"}},
			again: []string{"t"}},
		{name: "40#opening-a-corpus: a moved file reads it and its folder's entries again", n: Notices{Moved: []string{"t/x.flowmark.md"}},
			again: []string{"t", "t/x.flowmark.md"}},
		{name: "40#opening-a-corpus: a moved folder reads it, all under it and its folder's entries again", n: Notices{Moved: []string{"t"}},
			again: []string{".", "t", "t/s", "t/x.flowmark.md", "t/y.flowmark.md", "t/s/z.flowmark.md"}},
		{name: "40#opening-a-corpus: a moved root reads everything again", n: Notices{Moved: []string{"."}},
			again: []string{".", "t", "t/s", "a.flowmark.md", "t/x.flowmark.md", "t/y.flowmark.md", "t/s/z.flowmark.md"}},
		{name: "40#opening-a-corpus: an overflow reads everything again", n: Notices{Overflow: true, Changed: []string{"t/x.flowmark.md"}},
			again: []string{".", "t", "t/s", "a.flowmark.md", "t/x.flowmark.md", "t/y.flowmark.md", "t/s/z.flowmark.md"}},
		{name: "40#opening-a-corpus: a change of a path the tree does not hold drops nothing", n: Notices{Changed: []string{"t/q.flowmark.md"}},
			again: []string{}},
		{name: "40#opening-a-corpus: a path that appears reads its folder's entries again", n: Notices{Moved: []string{"u"}},
			again: []string{"."}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr, w, rec, root := scriptedTree(t, files)
			readAll(t, tr, all...)
			if got := reads(t, rec, root); len(got) != len(all) {
				t.Fatalf("the first read read %v", got)
			}
			readAll(t, tr, all...)
			if got := reads(t, rec, root); len(got) != 0 {
				t.Fatalf("a second read read %v from the disk", got)
			}
			w.next = c.n
			if err := tr.Refresh(); err != nil {
				t.Fatal(err)
			}
			rec.mu.Lock()
			var overflow int
			for _, e := range rec.events {
				if e.Kind == CorpusOverflow {
					overflow++
				}
			}
			rec.mu.Unlock()
			if want := map[bool]int{true: 1}[c.n.Overflow]; overflow != want {
				t.Fatalf("corpus.overflow published %d times, want %d", overflow, want)
			}
			rec.reset()
			readAll(t, tr, all...)
			got := reads(t, rec, root)
			slices.Sort(got)
			want := slices.Clone(c.again)
			slices.Sort(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("read again %v, want %v", got, want)
			}
		})
	}
}

func TestTreeForget(t *testing.T) {
	tr, _, rec, root := scriptedTree(t, map[string]string{"t/s/z.flowmark.md": "z\n", "t/x.flowmark.md": "x\n"})
	readAll(t, tr, ".", "t/", "t/s/", "t/s/z.flowmark.md", "t/x.flowmark.md")
	rec.reset()
	t.Log("40#the-commit-protocol: a forgotten target is read again with the entries of every folder above it")
	tr.Forget("t/s/z.flowmark.md")
	readAll(t, tr, ".", "t/", "t/s/", "t/s/z.flowmark.md", "t/x.flowmark.md")
	got := reads(t, rec, root)
	slices.Sort(got)
	if want := []string{".", "t", "t/s", "t/s/z.flowmark.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("read again %v, want %v", got, want)
	}
}

func TestTreeWatchFailure(t *testing.T) {
	t.Run("40#opening-a-corpus: a watch that cannot be armed fails the read and is the fault", func(t *testing.T) {
		tr, w, rec, root := scriptedTree(t, map[string]string{"a.flowmark.md": "a\n"})
		w.fail = errors.New("the limit of open files is reached")
		if _, err := tr.ReadFile("a.flowmark.md"); err == nil || !strings.Contains(err.Error(), "limit of open files") {
			t.Fatalf("ReadFile = %v, want the watch failure", err)
		}
		if err := tr.Fault(); err == nil || !strings.Contains(err.Error(), "limit of open files") {
			t.Fatalf("Fault = %v, want the watch failure", err)
		}
		if got := reads(t, rec, root); len(got) != 0 {
			t.Fatalf("a path that could not be watched was read: %v", got)
		}
		w.fail = nil
		if err := tr.Refresh(); err != nil {
			t.Fatal(err)
		}
		if err := tr.Fault(); err != nil {
			t.Fatalf("Refresh kept the fault %v", err)
		}
		readAll(t, tr, "a.flowmark.md")
	})
	t.Run("40#opening-a-corpus: a path the read cannot open is the read's own error, not a fault", func(t *testing.T) {
		tr, w, _, _ := scriptedTree(t, nil)
		w.fail = pathError("open", "gone.flowmark.md", fs.ErrNotExist)
		if _, err := tr.ReadFile("gone.flowmark.md"); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("ReadFile = %v, want not exist", err)
		}
		if err := tr.Fault(); err != nil {
			t.Fatalf("Fault = %v, want none", err)
		}
	})
	t.Run("40#opening-a-corpus: a drain that fails loses its notifications, so everything is read again", func(t *testing.T) {
		tr, w, rec, root := scriptedTree(t, map[string]string{"a.flowmark.md": "a\n"})
		readAll(t, tr, ".", "a.flowmark.md")
		rec.reset()
		w.drainErr = errors.New("kevent: interrupted")
		if err := tr.Refresh(); err == nil || !strings.Contains(err.Error(), "kevent: interrupted") {
			t.Fatalf("Refresh = %v, want the drain's failure", err)
		}
		readAll(t, tr, ".", "a.flowmark.md")
		if got := reads(t, rec, root); !reflect.DeepEqual(got, []string{".", "a.flowmark.md"}) {
			t.Fatalf("read again %v, want everything", got)
		}
	})
	t.Run("40#opening-a-corpus: a closed tree refuses", func(t *testing.T) {
		tr, w, _, _ := scriptedTree(t, nil)
		if err := tr.Close(); err != nil || !w.closed {
			t.Fatalf("Close = %v, closed %v", err, w.closed)
		}
		if err := tr.Refresh(); err == nil {
			t.Fatal("Refresh of a closed tree succeeded")
		}
		if _, err := tr.ReadDir("."); err == nil {
			t.Fatal("ReadDir of a closed tree succeeded")
		}
		if err := tr.Close(); err != nil {
			t.Fatalf("a second Close = %v", err)
		}
	})
}

// TestTreeFolderReplacedBetweenRefreshAndRead: a folder whose entries changed is replaced after a Refresh took that
// change and before the read listed it again; the next Refresh still learns that the folder held before was renamed,
// so a file under it is never answered from the folder that went away (spec 40 Opening a corpus).
func TestTreeFolderReplacedBetweenRefreshAndRead(t *testing.T) {
	root := t.TempDir()
	write := func(p, text string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("t/k.flowmark.md", "old\n")
	c, err := New(root, &recorder{})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := c.Tree()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tr.Close(); err != nil {
			t.Error(err)
		}
	})
	read := func() string {
		t.Helper()
		readAll(t, tr, ".", "t/")
		b, err := tr.ReadFile("t/k.flowmark.md")
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	refresh := func() {
		t.Helper()
		if err := tr.Refresh(); err != nil {
			t.Fatal(err)
		}
	}
	refresh()
	read()
	write("t/other.flowmark.md", "x\n")
	refresh()
	if err := os.Rename(filepath.Join(root, "t"), filepath.Join(root, "t.old")); err != nil {
		t.Fatal(err)
	}
	write("t/k.flowmark.md", "new\n")
	read()
	for i := range 3 {
		refresh()
		if got := read(); got != "new\n" {
			t.Fatalf("operation %d answers %q from the folder that went away, the disk holds %q", i+3, got, "new\n")
		}
	}
}

// TestTreeNotifications drives the platform's own watch: each change reaches the next Refresh, so the next read reads
// exactly what changed.
func TestTreeNotifications(t *testing.T) {
	root := t.TempDir()
	write := func(p, text string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("t/x.flowmark.md", "x\n")
	write("t/y.flowmark.md", "y\n")
	rec := &recorder{}
	c, err := New(root, rec)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := c.Tree()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tr.Close(); err != nil {
			t.Error(err)
		}
	})
	// walk reads the corpus as the walk does: each folder's entries, then each file in it.
	walk := func() map[string]string {
		t.Helper()
		if err := tr.Refresh(); err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		var visit func(dir string)
		visit = func(dir string) {
			es, err := tr.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range es {
				p := e.Name()
				if dir != "." {
					p = dir + "/" + p
				}
				if e.IsDir() {
					visit(p)
					continue
				}
				b, err := tr.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				out[p] = string(b)
			}
		}
		visit(".")
		return out
	}
	walk()
	rec.reset()
	step := func(name string, change func(), want map[string]string, read ...string) {
		t.Helper()
		t.Log(name)
		change()
		if got := walk(); !reflect.DeepEqual(got, want) {
			t.Fatalf("the tree holds %v, want %v", got, want)
		}
		got := []string{}
		for _, p := range reads(t, rec, root) {
			if strings.HasSuffix(p, ".flowmark.md") {
				got = append(got, p)
			}
		}
		slices.Sort(got)
		if read == nil {
			read = []string{}
		}
		if !reflect.DeepEqual(got, read) {
			t.Fatalf("files read %v, want %v", got, read)
		}
	}
	step("40#opening-a-corpus: nothing changed, nothing is read", func() {},
		map[string]string{"t/x.flowmark.md": "x\n", "t/y.flowmark.md": "y\n"})
	step("40#opening-a-corpus: a file written in place", func() { write("t/x.flowmark.md", "x2\n") },
		map[string]string{"t/x.flowmark.md": "x2\n", "t/y.flowmark.md": "y\n"}, "t/x.flowmark.md")
	step("40#opening-a-corpus: a file truncated", func() {
		if err := os.Truncate(filepath.Join(root, "t", "y.flowmark.md"), 0); err != nil {
			t.Fatal(err)
		}
	}, map[string]string{"t/x.flowmark.md": "x2\n", "t/y.flowmark.md": ""}, "t/y.flowmark.md")
	step("40#opening-a-corpus: a file created", func() { write("t/z.flowmark.md", "z\n") },
		map[string]string{"t/x.flowmark.md": "x2\n", "t/y.flowmark.md": "", "t/z.flowmark.md": "z\n"}, "t/z.flowmark.md")
	step("40#opening-a-corpus: a file removed", func() {
		if err := os.Remove(filepath.Join(root, "t", "z.flowmark.md")); err != nil {
			t.Fatal(err)
		}
	}, map[string]string{"t/x.flowmark.md": "x2\n", "t/y.flowmark.md": ""})
	step("40#opening-a-corpus: a file replaced by a rename", func() {
		write("t/.swap", "x3\n")
		if err := os.Rename(filepath.Join(root, "t", ".swap"), filepath.Join(root, "t", "x.flowmark.md")); err != nil {
			t.Fatal(err)
		}
	}, map[string]string{"t/x.flowmark.md": "x3\n", "t/y.flowmark.md": ""}, "t/x.flowmark.md")
	step("40#opening-a-corpus: a folder renamed away and another put in its place", func() {
		if err := os.Rename(filepath.Join(root, "t"), filepath.Join(root, "old")); err != nil {
			t.Fatal(err)
		}
		write("t/w.flowmark.md", "w\n")
	}, map[string]string{"old/x.flowmark.md": "x3\n", "old/y.flowmark.md": "", "t/w.flowmark.md": "w\n"},
		"old/x.flowmark.md", "old/y.flowmark.md", "t/w.flowmark.md")
	step("40#opening-a-corpus: a file in the renamed folder written", func() { write("old/y.flowmark.md", "y2\n") },
		map[string]string{"old/x.flowmark.md": "x3\n", "old/y.flowmark.md": "y2\n", "t/w.flowmark.md": "w\n"}, "old/y.flowmark.md")
}
