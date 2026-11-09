package statement_test

import (
	"fmt"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/cost"
)

// bulkSizes are the rows that one statement of TestCostBulkInsert inserts.
var bulkSizes = []int{100, 1000, 4000}

// TestCostBulkInsert pins that one statement inserting many rows into a document costs what it touches (plan, Ruling: an
// operation costs what it touches; T60): each row it appends joins the walk of its document without the rows before it
// being walked again, so the rows walked grow by the same number for each further inserted row at every size of
// bulkSizes, where walking the document again for each row made that number grow with the rows of the statement.
func TestCostBulkInsert(t *testing.T) {
	walked := map[int]int64{}
	for _, n := range bulkSizes {
		s, _ := openCost(t, 1000)
		warm(t, s)
		before := s.Cost().Snapshot()
		run(t, s, fmt.Sprintf("WITH RECURSIVE g(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM g WHERE i < %d) "+
			"INSERT INTO notes (id, title, written) SELECT 'B' || i, 'bulk ' || i, '2030-01-01' FROM g", n))
		walked[n] = s.Cost().Snapshot().Since(before).Get(cost.RowsWalked)
		if got := cells(run(t, s, "SELECT COUNT(*) FROM notes WHERE written = '2030-01-01'")); got != fmt.Sprintf("[[%d]]", n) {
			t.Fatalf("after an insert of %d rows the document holds %s", n, got)
		}
	}
	slope := func(a, b int) float64 { return float64(walked[b]-walked[a]) / float64(b-a) }
	first := slope(bulkSizes[0], bulkSizes[1])
	for i := 2; i < len(bulkSizes); i++ {
		if got := slope(bulkSizes[i-1], bulkSizes[i]); got != first {
			t.Errorf("each further inserted row walks %v rows from %d to %d rows and %v from %d to %d rows: %v",
				got, bulkSizes[i-1], bulkSizes[i], first, bulkSizes[0], bulkSizes[1], walked)
		}
	}
}
