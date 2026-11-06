package irexec

import (
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
	"github.com/polyspec/flowmark/packages/go/store"
)

// TestPostImage pins that a write's post-image shares every document it does not change with the corpus it read, and
// copies the rows of the documents it changes only (plan, Ruling: an operation costs what it touches). That the corpus
// read stays as it was is checked by every request of the tests (fixture.run).
func TestPostImage(t *testing.T) {
	for _, tc := range []struct {
		name, kind, entity string
		body               map[string]any
		params             []any
	}{
		{"an update of a document row", "update", "people", map[string]any{"set": sets("name"), "where": where(pred("id", "eq", p(1)))}, []any{"Renamed", "P2"}},
		{"an update of a files row", "update", "notes", map[string]any{"set": sets("title"), "where": where(pred("id", "eq", p(1)))}, []any{"Renamed", "N1"}},
		{"a delete of a files row", "delete", "notes", map[string]any{"where": where(pred("id", "eq", p(0)))}, []any{"N2"}},
		{"an insert of a files row", "insert", "notes", map[string]any{"set": sets("id", "title", "body")}, []any{"N9", "Ninth", "Body."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := catalogFixture(t, baseDocs()).on(t)
			k := &cost.Counts{}
			f.c.cost = k
			res := f.ok(f.msg(tc.kind, tc.entity, tc.body, tc.params...))
			targets := map[string]bool{}
			copied := 0
			for _, tg := range res.Post.Targets {
				targets[tg.Path] = true
				if old := f.c.file(tg.Path); old != nil {
					walk := 0
					for _, r := range old.rows {
						walk += countRows(r)
					}
					copied += walk
				}
			}
			if len(targets) == 0 {
				t.Fatal("the write changed no document")
			}
			next := res.Post.Next
			for p, old := range f.c.files.All() {
				if shared := next.file(p) == old; shared == targets[p] {
					t.Errorf("%s: the post-image shares it %v, and the write changes it %v", p, shared, targets[p])
				}
			}
			if got := k.Snapshot().Get(cost.RowsCopied); got > int64(copied) {
				t.Errorf("the write copies %d rows; the documents it changes hold %d", got, copied)
			}
		})
	}
}

// countRows returns the number of rows of r: r and its block rows, recursively.
func countRows(r *store.Row) int {
	n := 1
	for _, name := range r.BlockNames() {
		for _, b := range r.Blocks[name] {
			n += countRows(b)
		}
	}
	return n
}
