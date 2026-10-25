package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var errCrash = errors.New("simulated crash")

// The fixture: an update, a create in an existing folder, a create with the same content (one shared blob) and a
// delete — four renames in this order.
func fixture(t *testing.T) (root string, changes []Change, before, after map[string]string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, root, "a.md", "old a\n")
	writeFile(t, root, "c.md", "old c\n")
	writeFile(t, root, "sub/keep.md", "keep\n")
	changes = fixtureChanges()
	before = map[string]string{"a.md": "old a\n", "c.md": "old c\n", "sub/keep.md": "keep\n"}
	after = map[string]string{"a.md": "new a\n", "sub/b.md": "same\n", "d.md": "same\n", "sub/keep.md": "keep\n"}
	return root, changes, before, after
}

func hashOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestCommitAppliesAndCollects(t *testing.T) {
	root, changes, _, after := fixture(t)
	c, rec := newCorpus(t, root)
	l := acquire(t, c, "s1")
	wantCommitted(t, c, l, changes, root, after)
	if s := stages(t, c); len(s) != 0 {
		t.Fatalf("stages left: %v", s)
	}
	wantKinds(t, rec, LockAcquired, Prepared, Applied, Collected)
}

// Re-running the same commit is idempotent: the second run finds the post-image and leaves it.
func TestCommitRerunIsIdempotent(t *testing.T) {
	root, changes, _, after := fixture(t)
	c, _ := newCorpus(t, root)
	l := acquire(t, c, "s1")
	if err := c.Commit(l, changes[:3]); err != nil {
		t.Fatal(err)
	}
	if err := c.Commit(l, changes[:3]); err != nil {
		t.Fatal(err)
	}
	after["c.md"] = "old c\n"
	if got := snapshot(t, root); !reflect.DeepEqual(got, after) {
		t.Fatalf("corpus = %v, want %v", got, after)
	}
}

// The journal is written only under the lock of its own corpus.
func TestCommitRequiresHeldLock(t *testing.T) {
	root, changes, before, _ := fixture(t)
	c, _ := newCorpus(t, root)
	l := acquire(t, c, "s1")
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if err := c.Commit(l, changes); err == nil {
		t.Fatal("Commit accepted a released lock")
	}
	if err := c.Recover(l); err == nil {
		t.Fatal("Recover accepted a released lock")
	}
	other, _ := newCorpus(t, t.TempDir())
	ol := acquire(t, other, "s2")
	if err := c.Commit(ol, changes); err == nil {
		t.Fatal("Commit accepted the lock of another corpus")
	}
	if err := c.Commit(nil, changes); err == nil {
		t.Fatal("Commit accepted no lock")
	}
	if got := snapshot(t, root); !reflect.DeepEqual(got, before) {
		t.Fatalf("corpus changed: %v", got)
	}
}

func TestCommitRejectsChanges(t *testing.T) {
	cases := map[string][]Change{
		"empty path":         {{Path: "", Data: []byte("x")}},
		"parent path":        {{Path: "../x.md", Data: []byte("x")}},
		"absolute path":      {{Path: "/x.md", Data: []byte("x")}},
		"unclean path":       {{Path: "sub/./x.md", Data: []byte("x")}},
		"backslash path":     {{Path: `sub\x.md`, Data: []byte("x")}},
		"journal path":       {{Path: ".flowmark/x.md", Data: []byte("x")}},
		"duplicate path":     {{Path: "a.md", Data: []byte("1")}, {Path: "a.md", Data: []byte("2")}},
		"delete absent":      {{Path: "zz.md", Delete: true}},
		"delete with data":   {{Path: "a.md", Delete: true, Data: []byte("x")}},
		"file as folder":     {{Path: "a.md/x.md", Data: []byte("x")}},
		"directory target":   {{Path: "sub", Data: []byte("x")}},
		"symlink target":     {{Path: "link.md", Data: []byte("x")}},
		"symlink folder":     {{Path: "linkdir/x.md", Data: []byte("x")}},
		"one bad among good": {{Path: "a.md", Data: []byte("x")}, {Path: "../x.md", Data: []byte("x")}},
	}
	for name, changes := range cases {
		t.Run(name, func(t *testing.T) {
			root, _, _, _ := fixture(t)
			switch name {
			case "symlink target":
				symlink(t, filepath.Join(root, "a.md"), filepath.Join(root, "link.md"))
			case "symlink folder":
				symlink(t, filepath.Join(root, "sub"), filepath.Join(root, "linkdir"))
			}
			before := snapshot(t, root)
			c, rec := newCorpus(t, root)
			l := acquire(t, c, "s1")
			if err := c.Commit(l, changes); err == nil {
				t.Fatal("Commit accepted")
			}
			if got := snapshot(t, root); !reflect.DeepEqual(got, before) {
				t.Fatalf("corpus changed: %v", got)
			}
			if s := stages(t, c); len(s) != 0 {
				t.Fatalf("stages left: %v", s)
			}
			wantKinds(t, rec, LockAcquired)
		})
	}
}

// Recovery first: a commit refuses while a stage is left from a crash, and succeeds after recovery.
func TestCommitRefusesLeftoverStage(t *testing.T) {
	root, changes, _, after := fixture(t)
	c, _ := newCorpus(t, root)
	l := acquire(t, c, "s1")
	c.fault = crashAt("post-images", 0)
	if err := c.Commit(l, changes); !errors.Is(err, errCrash) {
		t.Fatalf("Commit = %v, want the crash", err)
	}
	c.fault = nil
	if err := c.Commit(l, changes); err == nil || !strings.Contains(err.Error(), "recover") {
		t.Fatalf("Commit = %v, want a recovery-first refusal", err)
	}
	if err := c.Recover(l); err != nil {
		t.Fatal(err)
	}
	wantCommitted(t, c, l, changes, root, after)
}

func crashAt(step string, index int) func(string, int) error {
	return func(s string, i int) error {
		if s == step && i == index {
			return errCrash
		}
		return nil
	}
}

// crashPoints lists every fault point of a commit with the state recovery must give: before the prepare record the
// stage is discarded (the old state); from it on the stage is completed (the new state).
var crashPoints = []struct {
	step  string
	index int
	new   bool
	event Kind
}{
	{"post-images", 0, false, Discarded},
	{"pre-images", 0, false, Discarded},
	{"prepare", 0, true, Replayed},
	{"rename", 0, true, Replayed},
	{"rename", 1, true, Replayed},
	{"rename", 2, true, Replayed},
	{"rename", 3, true, Replayed},
	{"renames", 0, true, Replayed},
	{"commit", 0, true, Replayed},
	{"collect", 0, true, Replayed},
}

// A crash at every step of the commit, then recovery: the corpus is the old state or the new one, never a mix.
func TestCrashAtEveryStep(t *testing.T) {
	for _, p := range crashPoints {
		t.Run(fmt.Sprintf("%s#%d", p.step, p.index), func(t *testing.T) {
			root, changes, before, after := fixture(t)
			c, _ := newCorpus(t, root)
			l := acquire(t, c, "s1")
			c.fault = crashAt(p.step, p.index)
			if err := c.Commit(l, changes); !errors.Is(err, errCrash) {
				t.Fatalf("Commit = %v, want the crash", err)
			}
			// A crash between two renames leaves a mixed corpus, which only the replay repairs.
			if p.step == "rename" && p.index < len(changes)-1 {
				if got := snapshot(t, root); reflect.DeepEqual(got, before) || reflect.DeepEqual(got, after) {
					t.Fatalf("corpus at the crash = %v, want a mix", got)
				}
			}
			if err := l.Release(); err != nil {
				t.Fatal(err)
			}
			// A fresh process opens the corpus.
			c2, rec := newCorpus(t, root)
			l2 := acquire(t, c2, "s2")
			want := before
			if p.new {
				want = after
			}
			wantRecovered(t, c2, l2, root, want)
			if s := stages(t, c2); len(s) != 0 {
				t.Fatalf("stages left: %v", s)
			}
			kinds := rec.kinds()
			if kinds[len(kinds)-1] != p.event {
				t.Fatalf("events = %v, want the last %s", kinds, p.event)
			}
			// Recovery is idempotent: a second run finds nothing and changes nothing.
			rec.reset()
			if err := c2.Recover(l2); err != nil {
				t.Fatal(err)
			}
			wantKinds(t, rec)
			if got := snapshot(t, root); !reflect.DeepEqual(got, want) {
				t.Fatalf("corpus after a second recovery = %v", got)
			}
		})
	}
}

// A crash during the replay itself: the next replay completes the stage.
func TestCrashDuringReplay(t *testing.T) {
	for _, p := range []struct {
		step  string
		index int
	}{{"rename", 0}, {"rename", 2}, {"renames", 0}, {"commit", 0}, {"collect", 0}} {
		t.Run(fmt.Sprintf("%s#%d", p.step, p.index), func(t *testing.T) {
			c, l, root, _, _, after := crashed(t, "prepare", 0)
			c.fault = crashAt(p.step, p.index)
			if err := c.Recover(l); !errors.Is(err, errCrash) {
				t.Fatalf("Recover = %v, want the crash", err)
			}
			c.fault = nil
			wantRecovered(t, c, l, root, after)
		})
	}
}

// A target whose hash matches neither recorded hash is refused and reported, never overwritten; nothing of the stage
// is applied, and the stage stays until the target is resolved.
func TestRecoverRefusesHumanEdit(t *testing.T) {
	for _, p := range []struct {
		step  string
		index int
	}{{"prepare", 0}, {"rename", 0}} {
		t.Run(fmt.Sprintf("%s#%d", p.step, p.index), func(t *testing.T) {
			root, changes, _, after := fixture(t)
			c, rec := newCorpus(t, root)
			l := acquire(t, c, "s1")
			c.fault = crashAt(p.step, p.index)
			if err := c.Commit(l, changes); !errors.Is(err, errCrash) {
				t.Fatalf("Commit = %v", err)
			}
			c.fault = nil
			writeFile(t, root, "d.md", "human\n")
			atCrash := snapshot(t, root)
			rec.reset()
			err := c.Recover(l)
			var refusal *RecoveryRefusal
			if !errors.As(err, &refusal) {
				t.Fatalf("Recover = %v, want *RecoveryRefusal", err)
			}
			if refusal.Path != "d.md" || refusal.Pre != "" || refusal.Post != hashOf("same\n") || refusal.Current != hashOf("human\n") {
				t.Fatalf("refusal = %+v", refusal)
			}
			stage := filepath.Join(c.txDir(), stages(t, c)[0])
			for _, part := range []string{"d.md", hashOf("same\n"), "absent", filepath.Join(stage, hashOf("same\n"))} {
				if !strings.Contains(err.Error(), part) {
					t.Fatalf("message %q does not name %q", err, part)
				}
			}
			if got := snapshot(t, root); !reflect.DeepEqual(got, atCrash) {
				t.Fatalf("corpus changed by a refused recovery: %v", got)
			}
			if s := stages(t, c); len(s) != 1 {
				t.Fatalf("stages = %v, want the stage kept", s)
			}
			wantKinds(t, rec)
			// The owner resolves the target to a recorded state; recovery then completes.
			if err := os.Remove(filepath.Join(root, "d.md")); err != nil {
				t.Fatal(err)
			}
			wantRecovered(t, c, l, root, after)
		})
	}
}

// A rename source whose bytes do not match its hash is never renamed into the corpus, nor is a copy written again
// from a blob that does not match.
func TestRecoverRefusesCorruptBlob(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage func(t *testing.T, stage string)
	}{
		{"copy", func(t *testing.T, stage string) {
			if err := os.WriteFile(filepath.Join(stage, "0."+hashOf("new a\n")), []byte("rot\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"blob behind a missing copy", func(t *testing.T, stage string) {
			if err := os.Remove(filepath.Join(stage, "0."+hashOf("new a\n"))); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stage, hashOf("new a\n")), []byte("rot\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, l, root, _, before, _ := crashed(t, "prepare", 0)
			c.fault = nil
			tc.damage(t, filepath.Join(c.txDir(), stages(t, c)[0]))
			err := c.Recover(l)
			if err == nil || !strings.Contains(err.Error(), hashOf("new a\n")) {
				t.Fatalf("Recover = %v, want a corrupt-blob error", err)
			}
			if got := snapshot(t, root); !reflect.DeepEqual(got, before) {
				t.Fatalf("corpus = %v, want %v", got, before)
			}
		})
	}
}

// A torn prepare record (only its partial file) is no prepare record: the stage is discarded.
func TestRecoverDiscardsTornPrepare(t *testing.T) {
	root, changes, before, _ := fixture(t)
	c, rec := newCorpus(t, root)
	l := acquire(t, c, "s1")
	c.fault = crashAt("pre-images", 0)
	if err := c.Commit(l, changes); !errors.Is(err, errCrash) {
		t.Fatalf("Commit = %v", err)
	}
	c.fault = nil
	s := stages(t, c)
	if err := os.WriteFile(filepath.Join(c.txDir(), s[0], "prepare.part"), []byte(`{"renames":[{"pa`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec.reset()
	if err := c.Recover(l); err != nil {
		t.Fatal(err)
	}
	wantKinds(t, rec, Discarded)
	if got := snapshot(t, root); !reflect.DeepEqual(got, before) {
		t.Fatalf("corpus = %v", got)
	}
}

// A prepare record that does not parse is an error and never an absent record.
func TestRecoverRejectsMalformedPrepare(t *testing.T) {
	for _, text := range []string{"garbage", `{"renames":[{"path":"../x","pre":"","post":""}]}`, `{"renames":[{"path":"a.md","pre":"zz","post":""}]}`, `{"renames":[],"extra":1}`} {
		t.Run(text, func(t *testing.T) {
			root, _, before, _ := fixture(t)
			c, _ := newCorpus(t, root)
			l := acquire(t, c, "s1")
			stage := filepath.Join(c.txDir(), stageName)
			if err := os.MkdirAll(stage, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stage, "prepare"), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := c.Recover(l); err == nil {
				t.Fatal("Recover accepted")
			}
			if got := snapshot(t, root); !reflect.DeepEqual(got, before) {
				t.Fatalf("corpus = %v", got)
			}
			if s := stages(t, c); len(s) != 1 {
				t.Fatalf("stages = %v", s)
			}
		})
	}
}

// Recovery reads only stage folders: any other entry, and a symlink where the journal must be, are refused.
func TestRecoverRejectsForeignEntries(t *testing.T) {
	t.Run("file in tx", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, ".flowmark/tx/stray", "x")
		c, _ := newCorpus(t, root)
		l := acquire(t, c, "s1")
		if err := c.Recover(l); err == nil {
			t.Fatal("Recover accepted a file in tx")
		}
	})
	t.Run("symlink tx", func(t *testing.T) {
		root := t.TempDir()
		elsewhere := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".flowmark"), 0o755); err != nil {
			t.Fatal(err)
		}
		symlink(t, elsewhere, filepath.Join(root, ".flowmark", "tx"))
		c, l := refusesLinkedStage(t, root)
		writeFile(t, root, "a.md", "x\n")
		if err := c.Commit(l, []Change{{Path: "a.md", Data: []byte("y\n")}}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Commit = %v, want a symlink refusal", err)
		}
	})
	t.Run("symlink stage", func(t *testing.T) {
		root := t.TempDir()
		elsewhere := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".flowmark", "tx"), 0o755); err != nil {
			t.Fatal(err)
		}
		symlink(t, elsewhere, filepath.Join(root, ".flowmark", "tx", stageName))
		refusesLinkedStage(t, root)
	})
}

// Opening a corpus without a journal finds nothing and publishes nothing.
func TestRecoverWithoutJournal(t *testing.T) {
	root := t.TempDir()
	c, rec := newCorpus(t, root)
	l := acquire(t, c, "s1")
	rec.reset()
	if err := c.Recover(l); err != nil {
		t.Fatal(err)
	}
	wantKinds(t, rec)
}
