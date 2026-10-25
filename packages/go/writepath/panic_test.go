package writepath

import (
	"context"
	"testing"

	"github.com/polyspec/flowmark/packages/go/irexec"
)

// mustPanic runs fn and requires it to panic.
func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	fn()
}

// panicAt returns a fault seam that panics at one step of the journal.
func panicAt(step string) func(string, int) error {
	return func(s string, _ int) error {
		if s == step {
			panic("panic at " + step)
		}
		return nil
	}
}

// TestPanicReleasesLock pins that a panic inside an operation never leaks the corpus lock: the operation that panics
// releases it on the way out, so the next session takes it without waiting.
func TestPanicReleasesLock(t *testing.T) {
	ctx := context.Background()
	t.Run("40#the-lock: a panic in a single operation", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("cli")
		mustPanic(t, func() {
			if _, err := s.Rewrite(ctx, func(*irexec.Corpus) ([]Rewrite, error) { panic("compute") }, nil); err != nil {
				t.Errorf("Rewrite returned %v; want its panic", err)
			}
		})
		want(t, f.session("other"), f.countTags(), `{"count":2}`)
	})
	t.Run("40#the-lock: a panic in a commit", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(ctx, false))
		want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[3]}`)
		f.j.SetFault(panicAt("prepare"))
		mustPanic(t, func() {
			if err := s.Commit(); err != nil {
				t.Errorf("Commit returned %v; want its panic", err)
			}
		})
		f.j.SetFault(nil)
		// The prepare record was durable: the commit is decided, and the next acquisition completes it.
		want(t, f.session("other"), f.countTags(), `{"count":3}`)
		f.clean()
	})
	t.Run("40#the-lock: a panic in the recovery of an acquisition", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("server")
		must(t, s.Begin(ctx, false))
		want(t, s, f.insertTag("web", "area"), `{"affected":1,"keys":[3]}`)
		f.j.SetFault(crashAt("renames", 0))
		if err := s.Commit(); err == nil {
			t.Fatal("the commit succeeded; want its stage left by the fault")
		}
		f.j.SetFault(panicAt("renames"))
		mustPanic(t, func() {
			if err := f.session("begin").Begin(ctx, false); err != nil {
				t.Errorf("Begin returned %v; want its panic", err)
			}
		})
		f.j.SetFault(nil)
		want(t, f.session("other"), f.countTags(), `{"count":3}`)
		f.clean()
	})
}
