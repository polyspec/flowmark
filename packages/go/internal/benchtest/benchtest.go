// Package benchtest measures the operations of the engine's benchmarks: the CPU time of each operation, which a
// machine's load changes less than its wall time, and the 95th percentile of its wall time (spec 40 Cost model).
package benchtest

import (
	"slices"
	"testing"
	"time"
)

// Loop runs op b.N times and reports the 95th percentile of its wall time and the CPU time per operation.
func Loop(b *testing.B, op func(i int)) {
	b.Helper()
	start := ProcessCPU(b)
	var walls []time.Duration
	i := 0
	for b.Loop() {
		i++
		t0 := time.Now()
		op(i)
		walls = append(walls, time.Since(t0))
	}
	b.ReportMetric(float64((ProcessCPU(b)-start)/time.Microsecond)/1000/float64(b.N), "cpu-ms/op")
	slices.Sort(walls)
	b.ReportMetric(float64(walls[(len(walls)*95+99)/100-1]/time.Microsecond)/1000, "p95-ms")
}
