package statement_test

import (
	"fmt"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/benchtest"
)

// benchSizes are the rows of the corpora the benchmarks measure, the sizes spec 40 states the times of.
var benchSizes = []int{1000, 10000, 100000}

// BenchmarkStatement measures each point operation of costOps, in their order, on one corpus of each size of
// benchSizes, after the statements that fill its database and build its index (spec 40 Cost model): the CPU time of an
// operation and the 95th percentile of its wall time. It runs a fixed number of times (`-benchtime 20x`), since the
// deletes take a row of their sibling group each.
func BenchmarkStatement(b *testing.B) {
	for _, rows := range benchSizes {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			s, root := openCost(b, rows)
			warm(b, s)
			for _, op := range costOps {
				b.Run(op.name, func(b *testing.B) {
					benchtest.Loop(b, func(i int) { op.do(b, s, root, i-1) })
				})
			}
		})
	}
}
