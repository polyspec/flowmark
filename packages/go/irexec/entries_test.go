package irexec

import (
	"fmt"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
)

// TestIndexEntries pins that the index costs the rows it enters, not the rows that share their keys (spec 40 Cost
// model): n notes, each a file, that reference one item and share one sibling group write the same entries to the
// index per note when it is built at 200 and at 2,000 notes, and an update of one note writes the same entries at
// both sizes.
func TestIndexEntries(t *testing.T) {
	check := checkFresh
	checkFresh = nil
	defer func() { checkFresh = check }()
	type entries struct{ build, write int64 }
	measure := func(t *testing.T, n int) entries {
		docs := baseDocs()
		for i := 1; i <= n; i++ {
			docs[fmt.Sprintf("notes/B%d.flowmark.md", i)] = lines(
				fmt.Sprintf("# Bench note %d", i),
				"- item: [T1](../items.flowmark.md#items-row-t1)",
				fmt.Sprintf("- position: %d", i),
				"",
				fmt.Sprintf("The body of the bench note %d.", i),
			)
		}
		f := catalogFixture(t, docs).on(t)
		f.c.cost = &cost.Counts{}
		if f.c.lookupOne(keyIndex("items", "T1")) == nil {
			t.Fatal("the corpus holds no item T1")
		}
		built := f.c.cost.Snapshot()
		f.ok(f.msg("update", "notes", map[string]any{"set": []any{map[string]any{"column": "weight", "p": 0}}, "where": map[string]any{"items": []any{pred("id", "eq", p(1))}}}, 1.5, "N3"))
		return entries{built.Get(cost.IndexEntries), f.c.cost.Snapshot().Since(built).Get(cost.IndexEntries)}
	}
	base := measure(t, 0)
	small, large := measure(t, 200), measure(t, 2000)
	t.Logf("index entries: built %d, %d and %d at 0, 200 and 2,000 notes; an update of one note %d and %d at 200 and 2,000", base.build, small.build, large.build, small.write, large.write)
	if per, want := (large.build-base.build)/2000, (small.build-base.build)/200; per != want || (large.build-base.build)%2000 != 0 {
		t.Errorf("the index is built with %d entries at 2,000 notes and %d at 200 beside %d without them: %d per note, %d at 200",
			large.build, small.build, base.build, per, want)
	}
	if large.write != small.write {
		t.Errorf("an update of one note writes %d entries at 2,000 notes and %d at 200", large.write, small.write)
	}
}
