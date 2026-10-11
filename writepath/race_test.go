package writepath

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestConcurrentSessions pins the sessions of one process sharing a corpus (spec 40 The lock): single requests and
// transactions with savepoints of several sessions run at once, each waiting for the lock, and a transaction closed
// from another goroutine — as a server closes an idle or disconnected session — rolls back. Run under go test -race.
func TestConcurrentSessions(t *testing.T) {
	f := baseFixture(t)
	f.c.wait = patience
	const writers, each, txs = 4, 5, 3
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, writers*each+txs*8+4)
	for w := 0; w < writers; w++ {
		s := f.session(fmt.Sprintf("w%d", w))
		msgs := make([][]byte, each)
		for i := range msgs {
			msgs[i] = f.insertTag(fmt.Sprintf("w%d-%d", w, i), "area")
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, m := range msgs {
				if _, err := s.Exec(ctx, m); err != nil {
					errs <- err
				}
			}
		}()
	}
	tx := f.session("tx")
	kept, dropped := make([][]byte, txs), make([][]byte, txs)
	for i := range kept {
		kept[i] = f.insertTag(fmt.Sprintf("kept%d", i), "topic")
		dropped[i] = f.insertTag(fmt.Sprintf("dropped%d", i), "topic")
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < txs; i++ {
			for _, step := range []func() error{
				func() error { return tx.Begin(ctx, false) },
				func() error { _, err := tx.Exec(ctx, kept[i]); return err },
				func() error { return tx.Savepoint("s") },
				func() error { _, err := tx.Exec(ctx, dropped[i]); return err },
				func() error { return tx.RollbackTo("s") },
				tx.Commit,
			} {
				if err := step(); err != nil {
					errs <- err
					return
				}
			}
		}
	}()
	idle, err := f.c.Session("idle")
	if err != nil {
		t.Fatal(err)
	}
	if err := idle.Begin(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := idle.Exec(ctx, f.insertTag("idle", "topic")); err != nil {
		t.Fatal(err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := idle.Close(); err != nil {
			errs <- err
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	want(t, f.session("check"), f.countTags(), fmt.Sprintf(`{"count":%d}`, 2+writers*each+txs))
	tags := f.file("tags.flowmark.md")
	if strings.Contains(tags, "dropped") || strings.Contains(tags, "| idle |") {
		t.Fatalf("a rolled-back write landed:\n%s", tags)
	}
	f.clean()
}
