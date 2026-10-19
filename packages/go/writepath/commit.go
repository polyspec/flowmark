package writepath

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/irexec"
	"github.com/polyspec/flowmark/packages/go/store/journal"
)

// commit runs the commit protocol of spec 40 for the transaction t, whose lock is held:
//
//  1. The corpus is read again. When it is not the corpus the transaction read, every target must hold the bytes it
//     was read with; otherwise WRITE_CONFLICT (CORPUS_CONFLICT) names each changed target and the first line at which
//     it changed.
//  2. The constraint checks run on the corpus re-read with the post-image applied: the transaction's writes run
//     again, in order, on the re-read corpus — the executor's checks are the checks of a write — and a write they
//     refuse fails the commit with its error. A write that answers otherwise, or a post-image that differs, means the
//     writes depend on the change that landed: WRITE_CONFLICT names each changed file. When the re-read corpus is the
//     one the transaction read, its writes were checked on it already.
//  3. The journal stages and applies the post-image (steps 3 to 8).
//
// Nothing is written when the transaction changes no byte. A journal commit that stops is recovered at once, under the
// same lock: before its prepare record it wrote nothing, and its partial stage is discarded; from the prepare record on
// the commit is decided, and recovery completes it, so the commit succeeds — or, when that recovery fails too, the
// commit stays decided and completes at the next acquisition of the lock, which the failure says. It reports whether
// the corpus was written.
func (c *Corpus) commit(t *tx) (bool, error) {
	net := netTargets(t.writes)
	if len(net) == 0 {
		return false, nil
	}
	// The targets are read again from the disk under the lock, whatever the notifications said: a hand edit whose
	// notification has not arrived is still found (step 1). The commit's own writes are dropped too, on every way out.
	paths := make([]string, len(net))
	for i, tg := range net {
		paths[i] = tg.Path
	}
	c.tree.Forget(paths...)
	defer c.tree.Forget(paths...)
	now, err := c.read()
	if err != nil {
		return false, err
	}
	if changed := t.base.Differs(now); len(changed) > 0 {
		var conflicts []Conflict
		for _, tg := range net {
			if cur := now.Data(tg.Path); !sameBytes(cur, tg.Before) {
				conflicts = append(conflicts, Conflict{File: tg.Path, Line: firstDiffLine(tg.Before, cur)})
			}
		}
		if len(conflicts) > 0 {
			return false, conflictError(conflicts, "changed since the transaction read it, and the commit never overwrites a change it did not read")
		}
		diverged, err := c.recheck(t, now)
		if err != nil {
			return false, err
		}
		if diverged {
			conflicts := make([]Conflict, len(changed))
			for i, p := range changed {
				conflicts[i] = Conflict{File: p, Line: firstDiffLine(t.base.Data(p), now.Data(p))}
			}
			return false, conflictError(conflicts, "changed since the transaction read it, and the transaction's writes give another result on it")
		}
	}
	changes := make([]journal.Change, len(net))
	for i, tg := range net {
		changes[i] = journal.Change{Path: tg.Path, Data: tg.After, Delete: tg.After == nil}
	}
	return c.journalCommit(t.lock, changes)
}

// journalCommit writes changes through the journal under the held lock l (steps 3 to 8) and reports whether the
// corpus was written. A journal commit that stops is recovered at once, under the same lock: before its prepare record
// it wrote nothing, and its partial stage is discarded; from the prepare record on the commit is decided, and recovery
// completes it — or, when that recovery fails too, the commit completes at the next acquisition of the lock, which the
// failure says.
func (c *Corpus) journalCommit(l *journal.Lock, changes []journal.Change) (bool, error) {
	err := c.j.Commit(l, changes)
	if err == nil {
		c.publishScale(changes)
		return true, nil
	}
	var ce *journal.CommitError
	prepared := errors.As(err, &ce) && ce.Prepared
	rerr := c.j.Recover(l)
	switch {
	case prepared && rerr == nil:
		c.publishScale(changes)
		return true, nil
	case prepared:
		return false, &Error{Code: codeDriver, Face: codeDriver, Err: errors.Join(err, rerr),
			Msg: fmt.Sprintf("the commit stopped after its prepare record: %v; recovering it under the lock failed: %v; the commit is prepared and decided, and it completes at the next acquisition of the lock — do not repeat it", err, rerr)}
	case rerr != nil:
		return false, &Error{Code: codeDriver, Face: codeDriver, Err: errors.Join(err, rerr),
			Msg: fmt.Sprintf("the commit stopped before its prepare record, so nothing was written: %v; discarding its partial stage failed: %v, and the next acquisition of the lock discards it", err, rerr)}
	}
	return false, &Error{Code: codeDriver, Face: codeDriver, Err: err,
		Msg: fmt.Sprintf("the commit stopped before its prepare record, so nothing was written, and its partial stage is discarded: %v", err)}
}

// recheck runs the writes of t again, in order, on the re-read corpus now (step 2). A write they refuse fails the
// commit with its error. It reports whether the writes diverged: a write answered otherwise than it did — the client
// holds the first answer — or the net post-image differs from the transaction's.
func (c *Corpus) recheck(t *tx, now *irexec.Corpus) (bool, error) {
	cur := now
	again := make([]*write, len(t.writes))
	for i, w := range t.writes {
		if cur == nil {
			return false, newError(codeInternal, "the write %d of the transaction follows a rewrite, which is its own transaction", i)
		}
		res, err := w.run(cur)
		if err != nil {
			return false, err
		}
		if res.Post == nil {
			return false, newError(codeInternal, "the write %d of the transaction ran again as a read", i)
		}
		if err := c.refuseRewrite(res.Post.Targets, w.exempt); err != nil {
			return false, err
		}
		if !reflect.DeepEqual(res.Answer, w.answer) {
			return true, nil
		}
		again[i] = &write{targets: res.Post.Targets}
		cur = res.Post.Next
	}
	return !sameNet(netTargets(t.writes), netTargets(again)), nil
}

// sameNet reports whether two net changes write the same files with the same bytes.
func sameNet(a, b []irexec.Target) bool {
	return slices.EqualFunc(a, b, func(x, y irexec.Target) bool { return x.Path == y.Path && sameBytes(x.After, y.After) })
}

// netTargets returns the net change of the writes, sorted by path: for each file, the bytes before the first write
// that changed it and after the last; a file the writes leave as it was is no target.
func netTargets(writes []*write) []irexec.Target {
	byPath := map[string]*irexec.Target{}
	var paths []string
	for _, w := range writes {
		for _, tg := range w.targets {
			if n, ok := byPath[tg.Path]; ok {
				n.After = tg.After
				continue
			}
			n := tg
			byPath[tg.Path] = &n
			paths = append(paths, tg.Path)
		}
	}
	var out []irexec.Target
	for _, p := range paths {
		if n := byPath[p]; !sameBytes(n.Before, n.After) {
			out = append(out, *n)
		}
	}
	slices.SortFunc(out, func(a, b irexec.Target) int { return strings.Compare(a.Path, b.Path) })
	return out
}

// sameBytes reports whether two file states are equal: both absent, or both present with equal bytes.
func sameBytes(a, b []byte) bool {
	return (a == nil) == (b == nil) && bytes.Equal(a, b)
}

// firstDiffLine returns the 1-based line at which the file state b first differs from a; a file absent in either is
// line 1.
func firstDiffLine(a, b []byte) int {
	if a == nil || b == nil {
		return 1
	}
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return 1 + bytes.Count(a[:i], []byte("\n"))
}

// conflictError is WRITE_CONFLICT (CORPUS_CONFLICT) naming each file and line.
func conflictError(conflicts []Conflict, why string) *Error {
	at := make([]string, len(conflicts))
	for i, cf := range conflicts {
		at[i] = fmt.Sprintf("%s:%d", cf.File, cf.Line)
	}
	return &Error{Code: codeWriteConflict, Face: faceCorpusConflict, Conflicts: conflicts,
		Msg: fmt.Sprintf("%s %s; nothing was written: read the corpus again and make the change again", strings.Join(at, ", "), why)}
}

// isConflict reports whether err is WRITE_CONFLICT.
func isConflict(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == codeWriteConflict
}

// refuseRewrite refuses a write that must rewrite a file with a format.* diagnostic (spec 40 The commit protocol,
// step 2): CONSTRAINT naming the file and the diagnostic, and flowmark canon when it is format.noncanonical. A target
// the write removes is never refused, since removing a file rewrites none of its bytes; exempt passes the diagnostics
// the operation is exempt for.
func (c *Corpus) refuseRewrite(targets []irexec.Target, exempt Exempt) error {
	for _, tg := range targets {
		if tg.After == nil {
			continue
		}
		for _, d := range tg.Diagnostics {
			if !strings.HasPrefix(d.Rule, "format.") || (exempt != nil && exempt(tg.Path, d)) {
				continue
			}
			fix := "repair the file by editing it, then write again"
			if d.Rule == "format.noncanonical" {
				fix = fmt.Sprintf("run `flowmark canon %s %s` to make it canonical, then write again", c.root, tg.Path)
			}
			return &Error{Code: codeConstraint, Face: codeConstraint, Diagnostic: &d,
				Msg: fmt.Sprintf("the write must rewrite %s, which has %s at %d:%d: %s; a write never rewrites bytes the user did not write: %s", tg.Path, d.Rule, d.Line, d.Column, d.Message, fix)}
		}
	}
	return nil
}

// publishScale publishes ScaleExceeded for each file the commit wrote past the file size of the scale envelope (spec
// 40 The scale envelope): the write is written all the same, and the event says which file grew past it.
func (c *Corpus) publishScale(changes []journal.Change) {
	limit := catalog.Envelope.FileBytes
	for _, ch := range changes {
		if !ch.Delete && len(ch.Data) > limit {
			c.j.Publish(journal.Event{Kind: journal.ScaleExceeded, Path: c.j.Abs(ch.Path), Detail: fmt.Sprintf("%d bytes, past the %d bytes of the scale envelope", len(ch.Data), limit)})
		}
	}
}
