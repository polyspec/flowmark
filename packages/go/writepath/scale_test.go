package writepath

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store/journal"
)

// TestScaleEvent pins the event of a write that carries a file past the scale envelope (spec 40 The scale
// envelope): the write succeeds, and the journal publishes scale.exceeded naming the file, its bytes and the limit.
// A write within the envelope publishes none.
func TestScaleEvent(t *testing.T) {
	old := catalog.Envelope
	t.Cleanup(func() { catalog.Envelope = old })
	catalog.Envelope = catalog.EnvelopeLimits{FileBytes: 400, FolderFiles: old.FolderFiles, CorpusFiles: old.CorpusFiles}
	scaleEvents := func(f *fixture) []journal.Event {
		var out []journal.Event
		f.rec.mu.Lock()
		defer f.rec.mu.Unlock()
		for _, e := range f.rec.events {
			if e.Kind == journal.ScaleExceeded {
				out = append(out, e)
			}
		}
		return out
	}
	t.Run("40#the-scale-envelope: a write within the envelope publishes nothing", func(t *testing.T) {
		f := newFixture(t, shardDBS(t), nil)
		want(t, f.session("scale"), f.msg("insert", "events", map[string]any{"set": sets(0, "title", "at")}, "Small", "2026-10-11 09:30:00.000000"), `{"affected":1,"keys":["E1"]}`)
		if got := scaleEvents(f); len(got) != 0 {
			t.Fatalf("events %+v", got)
		}
	})
	t.Run("40#the-scale-envelope: a write of a file past the file size succeeds and publishes scale.exceeded", func(t *testing.T) {
		f := newFixture(t, shardDBS(t), nil)
		s := f.session("scale")
		for i := 0; i < 12; i++ {
			want(t, s, f.msg("insert", "notes", map[string]any{"set": sets(0, "title", "written")}, strings.Repeat("N", 20), "2026-03-04"), fmt.Sprintf(`{"affected":1,"keys":["N%d"]}`, i+1))
		}
		got := scaleEvents(f)
		if len(got) == 0 {
			t.Fatal("no scale.exceeded event for a document past the file size")
		}
		last := got[len(got)-1]
		if !strings.HasSuffix(filepath.ToSlash(last.Path), "notes/2026.flowmark.md") || !strings.Contains(last.Detail, "400") {
			t.Fatalf("event %+v; want the document and the limit of 400 bytes", last)
		}
		if f.file("notes/2026.flowmark.md") == "" {
			t.Fatal("the write that carried the document past the envelope was not written")
		}
	})
}
