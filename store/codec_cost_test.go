package store

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// The codec's cost of spec 40 (Cost model): the parse and the render of a document file of the items table of the
// document fixture, per megabyte of the file. Each row is a list item with its fields and a detail section whose body
// holds paragraphs with inline markdown — emphasis, a code span, a link — and an evidence section, the shape of a
// project's item. Beside ns/op, B/op and allocs/op, each benchmark reports the throughput (MB/s), the bytes allocated
// per byte of the file (alloc-x) and the allocations per kilobyte of it (allocs/KB); TestCodecAllocationBudget holds
// the allocation budget, which, unlike time, a machine's load does not change.

// codecAllocBudget is spec 40's budget of the bytes the parse, and the render, of a file allocate per byte of it.
const codecAllocBudget = 64

// TestCodecAllocationBudget requires the parse and the render of a document of one and of four megabytes each to
// allocate at most codecAllocBudget bytes per byte of the file, so a cost that grows faster than the file shows.
func TestCodecAllocationBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("renders and parses megabytes")
	}
	cat := loadDocumentFixture(t)
	lk := documentLinker()
	for _, mb := range []int{1, 4} {
		rows, data := benchDocument(t, mb)
		parse := allocated(func() { ParseDocument(cat, "items", "items.flowmark.md", data, lk) })
		render := allocated(func() { RenderDocument(cat, "items", docPath("items"), rows, lk) })
		for name, n := range map[string]uint64{"parse": parse, "render": render} {
			if x := float64(n) / float64(len(data)); x > codecAllocBudget {
				t.Errorf("the %s of %d MB allocates %.1f bytes per byte; the budget is %d", name, mb, x, codecAllocBudget)
			}
		}
	}
}

// allocated returns the bytes that run allocates.
func allocated(run func()) uint64 {
	before := memStats()
	run()
	return memStats().TotalAlloc - before.TotalAlloc
}

// benchMBs are the sizes of the benchmarked document, in megabytes.
var benchMBs = []int{1, 5}

// benchRows returns the rows of an items document of about mb megabytes once rendered.
func benchRows(mb int) []*Row {
	var rows []*Row
	size := 0
	for i := 1; size < mb<<20; i++ {
		var body strings.Builder
		for p := 0; p < 3; p++ {
			fmt.Fprintf(&body, "Paragraph %d of item %d: the parser reads the *list rows* from the left and the `fields` from the right, as [the spec](docs/spec.md) states; each value keeps its _canonical_ form.\n", p, i)
			if p < 2 {
				body.WriteString("\n")
			}
		}
		evidence := fmt.Sprintf("Ran the golden tests for item %d.\n", i)
		r := &Row{
			Table: "items",
			Key:   fmt.Sprintf("T%d", i),
			Title: fmt.Sprintf("Benchmark item %d of the codec", i),
			Body:  body.String(),
			Scalars: map[string]*string{
				"state":    sp("waiting"),
				"position": sp(fmt.Sprint(i)),
				"done":     sp("false"),
				"estimate": sp("12.50"),
			},
			Sections: map[string]*string{"evidence": &evidence},
		}
		rows = append(rows, r)
		size += len(r.Body) + len(evidence) + 120
	}
	return rows
}

// benchDocument returns the rows of benchRows(mb) and their rendered document.
func benchDocument(b testing.TB, mb int) ([]*Row, []byte) {
	b.Helper()
	cat := loadDocumentFixture(b)
	rows := benchRows(mb)
	out, rf, err := RenderDocument(cat, "items", docPath("items"), rows, documentLinker())
	if err != nil || len(rf) > 0 {
		b.Fatalf("render: %v, %d refusals, first %+v", err, len(rf), rf[:min(len(rf), 1)])
	}
	back, diags, err := ParseDocument(cat, "items", "items.flowmark.md", out, documentLinker())
	if err != nil || len(diags) > 0 || len(back) != len(rows) {
		b.Fatalf("parse: %v, %d diagnostics, first %+v, %d rows of %d", err, len(diags), diags[:min(len(diags), 1)], len(back), len(rows))
	}
	return back, out
}

// reportPerByte reports the bytes allocated per byte of the document and the allocations per kilobyte of it, from the
// totals of the benchmark's run.
func reportPerByte(b *testing.B, size int, run func()) {
	b.Helper()
	b.SetBytes(int64(size))
	b.ReportAllocs()
	var allocs, bytes uint64
	before := memStats()
	for b.Loop() {
		run()
	}
	after := memStats()
	allocs, bytes = after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc
	n := float64(b.N)
	b.ReportMetric(float64(bytes)/n/float64(size), "alloc-x")
	b.ReportMetric(float64(allocs)/n/(float64(size)/1024), "allocs/KB")
}

// memStats returns the allocator's statistics.
func memStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// BenchmarkParseDocument is the parse of a document file: its rows read, checked and located.
func BenchmarkParseDocument(b *testing.B) {
	for _, mb := range benchMBs {
		b.Run(fmt.Sprintf("MB=%d", mb), func(b *testing.B) {
			_, data := benchDocument(b, mb)
			cat := loadDocumentFixture(b)
			lk := documentLinker()
			reportPerByte(b, len(data), func() {
				if _, diags, err := ParseDocument(cat, "items", "items.flowmark.md", data, lk); err != nil || len(diags) > 0 {
					b.Fatal(err, diags)
				}
			})
		})
	}
}

// BenchmarkRenderDocument is the render of a document file from its rows: written and checked.
func BenchmarkRenderDocument(b *testing.B) {
	for _, mb := range benchMBs {
		b.Run(fmt.Sprintf("MB=%d", mb), func(b *testing.B) {
			rows, data := benchDocument(b, mb)
			cat := loadDocumentFixture(b)
			lk := documentLinker()
			reportPerByte(b, len(data), func() {
				if _, rf, err := RenderDocument(cat, "items", docPath("items"), rows, lk); err != nil || len(rf) > 0 {
					b.Fatal(err, rf)
				}
			})
		})
	}
}
