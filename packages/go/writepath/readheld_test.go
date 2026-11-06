package writepath

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store"
)

// fromDisk is the corpus read from the disk as it now is, which a read of the corpus kept in memory must equal.
func (f *fixture) fromDisk() *irexec.Corpus {
	f.t.Helper()
	c, err := irexec.Open(f.c.cat, store.DirFS(f.root))
	if err != nil {
		f.t.Fatal(err)
	}
	return c
}

// committedIs requires the corpus Committed returns to s to be want.
func committedIs(t *testing.T, s *Session, want *irexec.Corpus, step string) {
	t.Helper()
	committedAs(t, s, func() *irexec.Corpus { return want }, step)
}

// recoveredIs requires the corpus Committed returns to s to be the one the disk holds after it, which its recovery may
// have changed.
func recoveredIs(t *testing.T, s *Session, f *fixture, step string) {
	t.Helper()
	committedAs(t, s, f.fromDisk, step)
}

// committedAs requires the corpus Committed returns to s to be the corpus want returns after it.
func committedAs(t *testing.T, s *Session, want func() *irexec.Corpus, step string) {
	t.Helper()
	got, err := s.Committed(context.Background())
	if err != nil {
		t.Fatalf("%s: %v", step, err)
	}
	if d := got.Differs(want()); len(d) != 0 {
		t.Fatalf("%s: the read differs at %v", step, d)
	}
}

// heldReader is a corpus written by one fixture and read by a second over its root, a session of the reader, and the
// corpus of its first read.
func heldReader(t *testing.T) (writer, reader *fixture, rs *Session, before *irexec.Corpus) {
	t.Helper()
	writer = baseFixture(t)
	reader = openFixture(t, writer.root)
	rs = reader.session("reader")
	before = reader.fromDisk()
	committedIs(t, rs, before, "the first read")
	return writer, reader, rs, before
}

// TestReadHeld pins the read of a corpus whose lock another holds (spec 40 Reads): without the lock it reads what that
// holder's commits wrote, unless a prepare record is present or the generation moves during the read, when it answers
// the corpus last read and the next read builds the corpus again from the tree, reading from the disk only what changed; a corpus whose own session holds the lock answers the
// corpus last read, since no other process can commit.
func TestReadHeld(t *testing.T) {
	ctx := context.Background()
	update := func(t *testing.T, s *Session, f *fixture, title string) {
		t.Helper()
		m := f.msg("update", "notes", map[string]any{"set": sets(0, "title"), "where": corpustest.WhereID(1)}, title, "N3")
		if _, err := s.Exec(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("40#sql-statements: a read while another holds the lock reads that holder's commit without it", func(t *testing.T) {
		writer := baseFixture(t)
		reader := openFixture(t, writer.root)
		rs := reader.session("reader")
		committedIs(t, rs, reader.fromDisk(), "the first read")
		ws := writer.session("writer")
		update(t, ws, writer, "Committed before the hold")
		holder := writer.begun(t, "holder")
		defer func() { must(t, holder.Rollback()) }()
		reader.rec.reset()
		committedIs(t, rs, reader.fromDisk(), "the read while the writer holds the lock")
		reader.wantRead("notes/N3.flowmark.md")
		if got := reader.c.Generation(); got != 1 {
			t.Fatalf("the read without the lock is at generation %d, want 1", got)
		}
		committedIs(t, rs, reader.fromDisk(), "the read after it, nothing changed")
		reader.wantRead()
	})
	t.Run("40#sql-statements: a prepare record present answers the corpus last read", func(t *testing.T) {
		writer, reader, rs, before := heldReader(t)
		update(t, writer.session("writer"), writer, "Committed under a stage")
		holder := writer.begun(t, "holder")
		stage := filepath.Join(writer.root, ".flowmark", "tx", "0123456789abcdef0123456789abcdef.prepare")
		must(t, os.MkdirAll(filepath.Dir(stage), 0o755))
		must(t, os.WriteFile(stage, []byte("{}\n"), 0o644))
		committedIs(t, rs, before, "the read while a prepare record is present")
		if got := reader.c.Generation(); got != 0 {
			t.Fatalf("the read that was not kept is at generation %d, want 0", got)
		}
		must(t, os.Remove(stage))
		committedIs(t, rs, reader.fromDisk(), "the read once the stage is gone")
		must(t, holder.Rollback())
	})
	t.Run("40#sql-statements: a commit during the read answers the corpus last read, and the next read builds it again", func(t *testing.T) {
		writer, reader, rs, before := heldReader(t)
		holder := writer.begun(t, "holder")
		update(t, holder, writer, "Committed during the read")
		// A file written again with its own bytes is a notification that changes nothing, so the read is one without the
		// lock, during which the holder commits.
		writer.write("notes/N2.flowmark.md", writer.file("notes/N2.flowmark.md"))
		committed := false
		reader.c.freeHook = func() {
			reader.c.freeHook = nil
			must(t, holder.Commit())
			committed = true
		}
		committedIs(t, rs, before, "the read during which the holder commits")
		if !committed {
			t.Fatal("the read did not read without the lock")
		}
		reader.rec.reset()
		committedIs(t, rs, reader.fromDisk(), "the read after the commit")
		reader.wantRead("notes/N3.flowmark.md")
	})
	t.Run("40#sql-statements: a session of the corpus holding the lock answers the corpus last read", func(t *testing.T) {
		f := baseFixture(t)
		rs := f.session("reader")
		before := f.fromDisk()
		committedIs(t, rs, before, "the first read")
		holder := f.begun(t, "holder")
		f.write("notes/N3.flowmark.md", lines("# Hand edit", "- position: 2"))
		committedIs(t, rs, before, "the read while a session of the corpus holds the lock")
		must(t, holder.Rollback())
		committedIs(t, rs, f.fromDisk(), "the read once the lock is free")
	})
	t.Run("40#sql-statements: a corpus not read yet reports the lock held", func(t *testing.T) {
		f := baseFixture(t)
		f.c.last.Store(nil)
		holder := openFixture(t, f.root).begun(t, "holder")
		defer func() { must(t, holder.Rollback()) }()
		if _, err := f.session("reader").Committed(ctx); err == nil {
			t.Fatal("a corpus not read yet answered while another holds the lock")
		}
	})
}

// TestReadRecoversPrepared pins a read of a corpus whose commit stopped at each step of the journal, its recovery
// under the same lock stopped too (spec 40 Reads): a stage left under a free lock makes the read take the lock, whose
// recovery completes a decided commit and discards one before its prepare record, and the read answers the corpus the
// recovery left, never a mix.
func TestReadRecoversPrepared(t *testing.T) {
	eachCrashPoint(t, func(t *testing.T, cp crashPoint, f *fixture, before, after map[string]string) {
		reader := openFixture(t, f.root)
		rs := reader.session("reader")
		committedIs(t, rs, reader.fromDisk(), "the first read")
		s := f.crashSession(t, "server")
		f.j.SetFault(crashAt(cp.step, cp.index))
		// The commit completes when the recovery under its lock does; otherwise the crash is its cause.
		if err := s.Commit(); err != nil && !errors.Is(err, errCrash) {
			t.Fatalf("Commit = %v, want the crash as its cause", err)
		}
		f.j.SetFault(nil)
		reader.rec.reset()
		recoveredIs(t, rs, reader, "the read after the stopped commit")
		if staged, err := reader.j.Staged(); err != nil || staged {
			t.Fatalf("after the read the corpus is staged %v, %v; want no stage", staged, err)
		}
		wantCrashed(t, f, cp.new, before, after)
	})
}
