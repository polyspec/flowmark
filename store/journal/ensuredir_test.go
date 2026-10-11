package journal

import (
	"fmt"
	"sync"
	"testing"
)

// TestEnsureDirRace pins that creating the lock folder is safe when several processes of one corpus do it at once
// (spec 40 The lock): the folder is made once, and every caller finds it. A caller that sees the folder absent and
// then finds it made by another has lost a race it must not report, as the folder is there.
func TestEnsureDirRace(t *testing.T) {
	const callers = 16
	for round := 0; round < 40; round++ {
		root := t.TempDir()
		c, _ := newCorpus(t, root)
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, callers)
		for i := range errs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs[i] = c.ensureDir(".flowmark")
			}()
		}
		close(start)
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("40#the-lock: round %d, caller %d: %v", round, i, fmt.Sprint(err))
			}
		}
	}
}
