package cost_test

import (
	"sync"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
)

// TestCounters holds the counts: each counter adds on its own, a nil Counts counts nothing, Since subtracts counter by
// counter, and adds from many goroutines are all kept.
func TestCounters(t *testing.T) {
	var k cost.Counts
	before := k.Snapshot()
	for i, c := range cost.Counters() {
		k.Add(c, i+1)
	}
	got := k.Snapshot().Since(before)
	for i, c := range cost.Counters() {
		if got.Get(c) != int64(i+1) {
			t.Errorf("%s = %d, want %d", c, got.Get(c), i+1)
		}
	}
	var none *cost.Counts
	none.Add(cost.FilesRead, 5)
	if s := none.Snapshot(); s != (cost.Snapshot{}) {
		t.Errorf("a nil Counts counted %v", s)
	}
	for _, tc := range []struct {
		name string
		add  int
	}{{"zero", 0}, {"one", 1}, {"many", 1000}} {
		t.Run(tc.name, func(t *testing.T) {
			var k cost.Counts
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() { k.Add(cost.RowsCopied, tc.add) })
			}
			wg.Wait()
			if got := k.Snapshot().Get(cost.RowsCopied); got != int64(8*tc.add) {
				t.Errorf("rows-copied = %d, want %d", got, 8*tc.add)
			}
		})
	}
	seen := map[string]bool{}
	for _, c := range cost.Counters() {
		if c.String() == "" || seen[c.String()] {
			t.Errorf("the counter %d has the name %q, empty or taken", c, c.String())
		}
		seen[c.String()] = true
	}
}
