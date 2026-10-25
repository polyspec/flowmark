package writepath

import (
	"context"
	"errors"
	"fmt"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store/journal"
)

// errCrash is the error the fault seam returns where a crash would stop the commit.
var errCrash = errors.New("simulated crash")

// crashAt returns a fault seam that fails at one step of the journal.
func crashAt(step string, index int) func(string, int) error {
	return func(s string, i int) error {
		if s == step && i == index {
			return errCrash
		}
		return nil
	}
}

// crashOnce returns a fault seam that fails at one step of the journal the first time it is reached, as a crash of
// one commit that the recovery after it does not meet again.
func crashOnce(step string, index int) func(string, int) error {
	fired := false
	return func(s string, i int) error {
		if s == step && i == index && !fired {
			fired = true
			return errCrash
		}
		return nil
	}
}

// TestDecidedCommitRecoveryFails pins a commit stopped after its prepare record whose recovery under the same lock
// fails too: DRIVER saying the commit is prepared, completes at the next acquisition and must not be repeated; the
// next acquisition completes it.
func TestDecidedCommitRecoveryFails(t *testing.T) {
	f := baseFixture(t)
	s := f.session("server")
	must(t, s.Begin(context.Background(), false))
	for _, m := range crashTx(f) {
		if _, err := s.Exec(context.Background(), m); err != nil {
			t.Fatal(err)
		}
	}
	f.j.SetFault(crashAt("renames", 0))
	err := s.Commit()
	fails(t, err, "DRIVER", "prepared", "next acquisition", "do not repeat")
	f.j.SetFault(nil)
	f.rec.reset()
	want(t, f.session("cli"), f.countTags(), `{"count":3}`)
	f.rec.wantKinds(t, journal.LockAcquired, journal.Applied, journal.Collected, journal.Replayed, journal.LockReleased)
	if got := snapshot(t, f.root); !reflect.DeepEqual(got, afterCrashTx(t)) {
		t.Fatalf("corpus = %v", got)
	}
	f.clean()
}

// crashPoint is a step of the commit with the state recovery must give: before the prepare record the stage is
// discarded (the old state); from it on it is completed (the new state).
type crashPoint struct {
	step  string
	index int
	new   bool
	event journal.Kind
}

// crashPoints are the steps of the crash transaction of crashTx, whose three targets are notes/N3.flowmark.md (a
// whole-file delete), people.flowmark.md and tags.flowmark.md.
var crashPoints = []crashPoint{
	{"post-images", 0, false, journal.Discarded},
	{"pre-images", 0, false, journal.Discarded},
	{"prepare", 0, true, journal.Replayed},
	{"rename", 0, true, journal.Replayed},
	{"rename", 1, true, journal.Replayed},
	{"rename", 2, true, journal.Replayed},
	{"renames", 0, true, journal.Replayed},
	{"commit", 0, true, journal.Replayed},
	{"collect", 0, true, journal.Replayed},
}

// crashTx returns the messages of the crash transaction: a tag inserted, a person inserted, and the note N3 deleted
// with its whole file.
func crashTx(f *fixture) [][]byte {
	return [][]byte{
		f.insertTag("web", "area"),
		f.msg("insert", "people", map[string]any{"set": sets(0, "name")}, "Lin"),
		f.msg("delete", "notes", map[string]any{"where": corpustest.WhereID(0)}, "N3"),
	}
}

// snapshot returns every corpus file outside .flowmark/ and dbspec/ by slash path.
func snapshot(t testing.TB, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == ".flowmark" || rel == "dbspec" {
				return filepath.SkipDir
			}
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// afterCrashTx returns the corpus that the crash transaction commits, by running it on a copy without a fault.
func afterCrashTx(t *testing.T) map[string]string {
	t.Helper()
	f := baseFixture(t)
	s := f.session("reference")
	must(t, s.Begin(context.Background(), false))
	for _, m := range crashTx(f) {
		if _, err := s.Exec(context.Background(), m); err != nil {
			t.Fatal(err)
		}
	}
	must(t, s.Commit())
	return snapshot(t, f.root)
}

// TestRecoveryOnAcquisition pins a commit stopped at each step of the journal (spec 40 The commit protocol): before its
// prepare record it writes nothing and fails DRIVER, its partial stage discarded; from the prepare record on it is
// decided, and recovery under the same lock completes it, so the commit succeeds. Never a mix.
func TestRecoveryOnAcquisition(t *testing.T) {
	after := afterCrashTx(t)
	for _, cp := range crashPoints {
		t.Run(fmt.Sprintf("40#opening-a-corpus: %s#%d", cp.step, cp.index), func(t *testing.T) {
			f := baseFixture(t)
			before := snapshot(t, f.root)
			s := f.session("server")
			must(t, s.Begin(context.Background(), false))
			for _, m := range crashTx(f) {
				if _, err := s.Exec(context.Background(), m); err != nil {
					t.Fatal(err)
				}
			}
			f.j.SetFault(crashOnce(cp.step, cp.index))
			err := s.Commit()
			if cp.new {
				// From the prepare record on the commit is decided: recovery under the same lock completes it.
				if err != nil {
					t.Fatalf("Commit = %v, want the commit completed under its lock", err)
				}
			} else {
				fails(t, err, "DRIVER", "simulated crash", "nothing was written")
				if !errors.Is(err, errCrash) {
					t.Fatalf("Commit = %v, want the crash as its cause", err)
				}
			}
			f.j.SetFault(nil)
			kinds := f.rec.kinds()
			if len(kinds) < 3 || kinds[0] != journal.LockAcquired || kinds[len(kinds)-2] != cp.event || kinds[len(kinds)-1] != journal.LockReleased {
				t.Fatalf("events = %v, want the lock, then %s, then the release", kinds, cp.event)
			}
			f.rec.reset()
			want(t, f.session("cli"), f.countTags(), map[bool]string{false: `{"count":2}`, true: `{"count":3}`}[cp.new])
			f.rec.wantKinds(t, opened...)
			wantSnap := before
			if cp.new {
				wantSnap = after
			}
			if got := snapshot(t, f.root); !reflect.DeepEqual(got, wantSnap) {
				t.Fatalf("corpus = %v, want %v", got, wantSnap)
			}
			f.clean()
		})
	}
}

// TestOpenRecovers pins recovery when a corpus is opened (spec 40 Opening a corpus): Open runs it under the lock and
// publishes its events; a target changed while the commit was interrupted refuses opening and every request, as
// DRIVER naming the target, until it holds one of its recorded states.
func TestOpenRecovers(t *testing.T) {
	stopped := func(t *testing.T) *fixture {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(context.Background(), false))
		for _, m := range crashTx(f) {
			if _, err := s.Exec(context.Background(), m); err != nil {
				t.Fatal(err)
			}
		}
		// A fault that recovery meets again leaves the decided commit's stage for the next acquisition.
		f.j.SetFault(crashAt("renames", 0))
		if err := s.Commit(); !errors.Is(err, errCrash) {
			t.Fatalf("Commit = %v", err)
		}
		return f
	}
	open := func(t *testing.T, root string) (*Corpus, *recorder, error) {
		cat, diags := catalog.Load(root)
		if len(diags) > 0 {
			t.Fatal(diags)
		}
		rec := &recorder{}
		j, err := journal.New(root, rec)
		if err != nil {
			t.Fatal(err)
		}
		c, err := Open(context.Background(), cat, j, 0)
		if err == nil {
			t.Cleanup(func() {
				if err := c.Close(); err != nil {
					t.Errorf("Close: %v", err)
				}
			})
		}
		return c, rec, err
	}
	t.Run("40#opening-a-corpus: opening completes a prepared stage", func(t *testing.T) {
		f := stopped(t)
		_, rec, err := open(t, f.root)
		if err != nil {
			t.Fatal(err)
		}
		rec.wantKinds(t, journal.LockAcquired, journal.Applied, journal.Collected, journal.Replayed, journal.LockReleased)
		if got := snapshot(t, f.root); !reflect.DeepEqual(got, afterCrashTx(t)) {
			t.Fatalf("corpus = %v", got)
		}
	})
	t.Run("40#opening-a-corpus: opening a corpus without a stage", func(t *testing.T) {
		f := baseFixture(t)
		_, rec, err := open(t, f.root)
		if err != nil {
			t.Fatal(err)
		}
		rec.wantKinds(t, opened...)
	})
	t.Run("40#opening-a-corpus: a target changed during the interruption refuses until resolved", func(t *testing.T) {
		f := stopped(t)
		f.j.SetFault(nil)
		edited := baseDocs()["tags.flowmark.md"] + "| 9 | hand | area |\n"
		f.write("tags.flowmark.md", edited)
		_, _, err := open(t, f.root)
		fails(t, err, "DRIVER", "tags.flowmark.md", "matches neither recorded state")
		var refusal *journal.RecoveryRefusal
		if !errors.As(err, &refusal) || refusal.Path != "tags.flowmark.md" {
			t.Fatalf("Open = %v, want the recovery refusal of tags.flowmark.md as its cause", err)
		}
		_, err = f.session("cli").Exec(context.Background(), f.countTags())
		fails(t, err, "DRIVER", "tags.flowmark.md")
		if f.file("tags.flowmark.md") != edited {
			t.Fatal("the edit was overwritten")
		}
		// The person restores the recorded state before the commit, and opening completes the commit.
		f.write("tags.flowmark.md", baseDocs()["tags.flowmark.md"])
		if _, _, err := open(t, f.root); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(f.file("tags.flowmark.md"), "| 3 | web | area |\n") {
			t.Fatalf("tags:\n%s", f.file("tags.flowmark.md"))
		}
	})
	t.Run("40#the-commit-protocol: a create whose folder is missing, stopped after the folder", func(t *testing.T) {
		f := newFixture(t, executorCatalog(t), nil)
		s := f.session("cli")
		// The recovery after the stop finds the folder in place and completes the create.
		f.j.SetFault(crashAt("folder", 0))
		want(t, s, f.msg("insert", "notes", map[string]any{"set": sets(0, "id", "title", "body")}, "N1", "First", ""), `{"affected":1,"keys":["N1"]}`)
		f.j.SetFault(nil)
		kinds := f.rec.kinds()
		if kinds[len(kinds)-2] != journal.Replayed {
			t.Fatalf("events = %v, want the replay of the stopped commit", kinds)
		}
		if f.file("notes/N1.flowmark.md") == "" {
			t.Fatal("the create did not land")
		}
		f.clean()
	})
}
