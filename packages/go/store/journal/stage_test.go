package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const stageName = "0123456789abcdef0123456789abcdef"

// crashed commits the fixture under a fault at step#index and returns the corpus, its lock and the stage folder.
func crashed(t *testing.T, step string, index int) (c *Corpus, l *Lock, root, stage string, before, after map[string]string) {
	t.Helper()
	root, changes, before, after := fixture(t)
	c, _ = newCorpus(t, root)
	l = acquire(t, c, "s1")
	c.fault = crashAt(step, index)
	if err := c.Commit(l, changes); !errors.Is(err, errCrash) {
		t.Fatalf("Commit = %v, want the crash", err)
	}
	c.fault = nil
	return c, l, root, filepath.Join(c.txDir(), stages(t, c)[0]), before, after
}

// `.flowmark/tx/` holds at most one stage: a second stage, or an entry of another name, refuses opening and is named.
func TestRecoverRefusesTwoStages(t *testing.T) {
	for _, extra := range []string{stageName, "notastage", strings.ToUpper(stageName)} {
		t.Run(extra, func(t *testing.T) {
			c, l, root, stage, _, _ := crashed(t, "prepare", 0)
			atCrash := snapshot(t, root)
			if err := os.Mkdir(filepath.Join(c.txDir(), extra), 0o755); err != nil {
				t.Fatal(err)
			}
			err := c.Recover(l)
			if err == nil || !strings.Contains(err.Error(), extra) {
				t.Fatalf("Recover = %v, want a refusal naming %s", err, extra)
			}
			if extra == stageName && !strings.Contains(err.Error(), filepath.Base(stage)) {
				t.Fatalf("message %q does not name both stages", err)
			}
			if got := snapshot(t, root); !reflect.DeepEqual(got, atCrash) {
				t.Fatalf("corpus changed: %v", got)
			}
		})
	}
}

// Two targets with the same post-image are two files, and neither is a file of the stage.
func TestCommitIdenticalTargetsAreDistinctFiles(t *testing.T) {
	root, changes, _, _ := fixture(t)
	c, _ := newCorpus(t, root)
	l := acquire(t, c, "s1")
	if err := c.Commit(l, changes); err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(filepath.Join(root, "sub", "b.md"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := os.Stat(filepath.Join(root, "d.md"))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(b, d) {
		t.Fatal("sub/b.md and d.md are one file")
	}
	writeFile(t, root, "d.md", "edited\n")
	if got := snapshot(t, root)["sub/b.md"]; got != "same\n" {
		t.Fatalf("sub/b.md = %q after an edit of d.md", got)
	}
}

// A target renamed from the stage shares no file with the stage that a replay could still rewrite.
func TestTargetSharesNoFileWithStage(t *testing.T) {
	c, _, root, stage, _, after := crashed(t, "renames", 0)
	_ = c
	entries, err := os.ReadDir(stage)
	if err != nil {
		t.Fatal(err)
	}
	for rel := range after {
		target, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			info, err := os.Stat(filepath.Join(stage, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(target, info) {
				t.Fatalf("%s is the stage file %s", rel, e.Name())
			}
		}
	}
}

// A delete renames the target into the stage as `<n>.deleted`; collecting the stage removes it.
func TestCommitDeleteMovesIntoStage(t *testing.T) {
	c, l, root, stage, _, after := crashed(t, "renames", 0)
	data, err := os.ReadFile(filepath.Join(stage, "3.deleted"))
	if err != nil || string(data) != "old c\n" {
		t.Fatalf("3.deleted = %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(root, "c.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("c.md: %v", err)
	}
	wantRecovered(t, c, l, root, after)
	if s := stages(t, c); len(s) != 0 {
		t.Fatalf("stages = %v", s)
	}
}

// A replay after the delete's rename finds the target absent and leaves it; replaying again changes nothing.
func TestDeleteReplayIdempotent(t *testing.T) {
	c, l, root, stage, _, after := crashed(t, "rename", 3)
	if _, err := os.Lstat(filepath.Join(stage, "3.deleted")); err != nil {
		t.Fatalf("the deleted target is not in the stage: %v", err)
	}
	c.fault = crashAt("renames", 0)
	if err := c.Recover(l); !errors.Is(err, errCrash) {
		t.Fatalf("Recover = %v, want the crash", err)
	}
	c.fault = nil
	for range 2 {
		wantRecovered(t, c, l, root, after)
	}
}

// requireRefusal fails the test unless err is a *RecoveryRefusal.
func requireRefusal(t *testing.T, err error) {
	t.Helper()
	if refusalOf(t, err) == nil {
		t.Fatalf("Recover = %v, want *RecoveryRefusal", err)
	}
}

// refusalOf returns the *RecoveryRefusal that err is, and fails the test when it is none.
func refusalOf(t *testing.T, err error) *RecoveryRefusal {
	t.Helper()
	var refusal *RecoveryRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("Recover = %v, want *RecoveryRefusal", err)
	}
	return refusal
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// After a partial apply, an edit of a renamed target is refused naming the stage files of both recorded states;
// copying one of them over the target resolves it.
func TestRefusalAfterPartialApplyResolvable(t *testing.T) {
	c, l, root, stage, _, after := crashed(t, "rename", 1)
	writeFile(t, root, "a.md", "human\n")
	refusal := refusalOf(t, c.Recover(l))
	if refusal.Path != "a.md" || refusal.Pre != hashOf("old a\n") || refusal.Post != hashOf("new a\n") {
		t.Fatalf("refusal = %+v", refusal)
	}
	for _, name := range []string{hashOf("old a\n"), hashOf("new a\n")} {
		if !strings.Contains(refusal.Error(), filepath.Join(stage, name)) {
			t.Fatalf("message %q does not name the stage file %s", refusal, name)
		}
	}
	copyFile(t, filepath.Join(stage, hashOf("new a\n")), filepath.Join(root, "a.md"))
	wantRecovered(t, c, l, root, after)
}

func TestRefusalResolvedByRestoringPre(t *testing.T) {
	c, l, root, stage, _, after := crashed(t, "prepare", 0)
	writeFile(t, root, "a.md", "human\n")
	requireRefusal(t, c.Recover(l))
	copyFile(t, filepath.Join(stage, hashOf("old a\n")), filepath.Join(root, "a.md"))
	wantRecovered(t, c, l, root, after)
}

func TestRefusalResolvedByRestoringPost(t *testing.T) {
	c, l, root, stage, _, after := crashed(t, "prepare", 0)
	writeFile(t, root, "a.md", "human\n")
	requireRefusal(t, c.Recover(l))
	copyFile(t, filepath.Join(stage, hashOf("new a\n")), filepath.Join(root, "a.md"))
	wantRecovered(t, c, l, root, after)
}

// Until the target is resolved, every open refuses the same way and changes nothing.
func TestRefusalPersistsUntilResolved(t *testing.T) {
	c, l, root, _, _, _ := crashed(t, "prepare", 0)
	writeFile(t, root, "a.md", "human\n")
	atEdit := snapshot(t, root)
	first := refusalOf(t, c.Recover(l)).Error()
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		c2, _ := newCorpus(t, root)
		l2 := acquire(t, c2, fmt.Sprintf("s%d", i+2))
		if got := refusalOf(t, c2.Recover(l2)).Error(); got != first {
			t.Fatalf("refusal %d = %q, want %q", i, got, first)
		}
		if err := l2.Release(); err != nil {
			t.Fatal(err)
		}
	}
	if got := snapshot(t, root); !reflect.DeepEqual(got, atEdit) {
		t.Fatalf("corpus changed: %v", got)
	}
}

// A create whose folder is absent creates its folders from the corpus root, in place or (as on Windows) in the stage
// and moved into place.
func TestCommitCreatesTableFolder(t *testing.T) {
	for _, staged := range []bool{false, true} {
		t.Run(fmt.Sprintf("staged=%v", staged), func(t *testing.T) {
			root := t.TempDir()
			c, _ := newCorpus(t, root)
			c.stageFolders = staged
			l := acquire(t, c, "s1")
			changes := []Change{{Path: "tbl/k1.md", Data: []byte("k1\n")}, {Path: "x/y/z.md", Data: []byte("z\n")}}
			if err := c.Commit(l, changes); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"tbl/k1.md": "k1\n", "x/y/z.md": "z\n"}
			if got := snapshot(t, root); !reflect.DeepEqual(got, want) {
				t.Fatalf("corpus = %v", got)
			}
			if s := stages(t, c); len(s) != 0 {
				t.Fatalf("stages = %v", s)
			}
		})
	}
}

// A crash after a folder is made and before the file's rename: the replay finds the folder (or its staged form) and
// completes.
func TestCrashAfterFolderBeforeRename(t *testing.T) {
	for _, tc := range []struct {
		staged bool
		step   string
	}{{false, "folder"}, {true, "folder"}, {true, "folder-staged"}} {
		t.Run(fmt.Sprintf("staged=%v %s", tc.staged, tc.step), func(t *testing.T) {
			root := t.TempDir()
			c, _ := newCorpus(t, root)
			c.stageFolders = tc.staged
			l := acquire(t, c, "s1")
			c.fault = crashAt(tc.step, 0)
			if err := c.Commit(l, []Change{{Path: "x/y/z.md", Data: []byte("z\n")}}); !errors.Is(err, errCrash) {
				t.Fatalf("Commit = %v, want the crash", err)
			}
			c.fault = nil
			if _, err := os.Lstat(filepath.Join(root, "x", "y", "z.md")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("z.md before the replay: %v", err)
			}
			if err := c.Recover(l); err != nil {
				t.Fatal(err)
			}
			if got := snapshot(t, root); !reflect.DeepEqual(got, map[string]string{"x/y/z.md": "z\n"}) {
				t.Fatalf("corpus = %v", got)
			}
		})
	}
}

// A folder is created following no symlink: one on the way refuses the commit, and one planted before a replay
// refuses the replay; nothing is written through it.
func TestCreateFolderRefusesSymlink(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		root := t.TempDir()
		elsewhere := t.TempDir()
		symlink(t, elsewhere, filepath.Join(root, "linkdir"))
		c, _ := newCorpus(t, root)
		l := acquire(t, c, "s1")
		if err := c.Commit(l, []Change{{Path: "linkdir/new/x.md", Data: []byte("x\n")}}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Commit = %v, want a symlink refusal", err)
		}
		entries, readErr := os.ReadDir(elsewhere)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Fatalf("written through the link: %v", entries)
		}
	})
	t.Run("replay", func(t *testing.T) {
		root := t.TempDir()
		elsewhere := t.TempDir()
		c, _ := newCorpus(t, root)
		l := acquire(t, c, "s1")
		c.fault = crashAt("prepare", 0)
		if err := c.Commit(l, []Change{{Path: "tbl/k1.md", Data: []byte("k1\n")}}); !errors.Is(err, errCrash) {
			t.Fatalf("Commit = %v", err)
		}
		c.fault = nil
		symlink(t, elsewhere, filepath.Join(root, "tbl"))
		if err := c.Recover(l); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Recover = %v, want a symlink refusal", err)
		}
		entries, readErr := os.ReadDir(elsewhere)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Fatalf("written through the link: %v", entries)
		}
	})
}
