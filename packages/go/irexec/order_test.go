package irexec

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// rankedDocs is the base corpus with the notes stored as ranks: the roots N1 and N3
// at 1000 and 2000, the child N2 at 1000 under N1.
func rankedDocs() map[string]string {
	docs := baseDocs()
	docs["notes/N1.flowmark.md"] = strings.Replace(docs["notes/N1.flowmark.md"], "- position: 1\n", "- position: 1000\n", 1)
	docs["notes/N2.flowmark.md"] = strings.Replace(docs["notes/N2.flowmark.md"], "- position: 1\n", "- position: 1000\n", 1)
	docs["notes/N3.flowmark.md"] = strings.Replace(docs["notes/N3.flowmark.md"], "- position: 2\n", "- position: 2000\n", 1)
	return docs
}

// targets returns the paths a write's post-image changes.
func targets(res *Result) []string {
	var out []string
	for _, tg := range res.Post.Targets {
		out = append(out, tg.Path)
	}
	return out
}

// wantTargets requires the post-image of res to change exactly paths.
func wantTargets(t testing.TB, res *Result, paths ...string) {
	t.Helper()
	if got := targets(res); !slices.Equal(got, paths) {
		t.Fatalf("targets %v; want %v", got, paths)
	}
}

// notePositions is the read of every note's parent and position in storage order.
func notePositions(f *fixture) []byte {
	return f.msg("all", "notes", map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"item"}, "add": []string{"position"}}})
}

// TestFilesOrderRank pins the stored form of order in files storage (spec 20 Order):
// the field holds a rank, a read fills the 1-based position, and a write rewrites
// the placed row's file alone while a free rank lies between its neighbours.
func TestFilesOrderRank(t *testing.T) {
	ranked := func(t *testing.T) *fixture { return catalogFixture(t, rankedDocs()).on(t) }
	byID := func(f *fixture, entity string, set []any, id string, vals ...any) []byte {
		return f.msg("update", entity, map[string]any{"set": set, "where": where(pred("id", "eq", p(len(vals))))}, append(vals, id)...)
	}
	t.Run("20#order: a read fills the position, never the rank", func(t *testing.T) {
		f := ranked(t)
		f.want(notePositions(f),
			`{"rows":[{"id":"N1","parent":null,"position":1},{"id":"N2","parent":"N1","position":1},{"id":"N3","parent":null,"position":2}]}`)
		f.want(f.msg("all", "notes", map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"item", "parent"}}, "where": where(pred("position", "eq", p(0)))}, 2),
			`{"rows":[{"id":"N3"}]}`)
	})
	t.Run("20#order: an insert at 1 writes its own file alone", func(t *testing.T) {
		f := ranked(t)
		res := f.want(f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "position")}, "N4", "Fourth", "", 1), `{"affected":1,"keys":["N4"]}`)
		wantTargets(t, res, "notes/N4.flowmark.md")
		f.commit(res)
		if got := f.file("notes/N4.flowmark.md"); got != lines("# Fourth", "- position: 500") {
			t.Fatalf("N4:\n%s", got)
		}
		f.want(notePositions(f),
			`{"rows":[{"id":"N4","parent":null,"position":1},{"id":"N1","parent":null,"position":2},{"id":"N2","parent":"N1","position":1},{"id":"N3","parent":null,"position":3}]}`)
	})
	t.Run("20#order: an insert without a position appends one spacing after the last rank", func(t *testing.T) {
		f := ranked(t)
		res := f.ok(f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body")}, "N4", "Fourth", ""))
		wantTargets(t, res, "notes/N4.flowmark.md")
		f.commit(res)
		if got := f.file("notes/N4.flowmark.md"); got != lines("# Fourth", "- position: 3000") {
			t.Fatalf("N4:\n%s", got)
		}
	})
	t.Run("20#order: a move into a gap writes the moved file alone", func(t *testing.T) {
		f := ranked(t)
		res := f.ok(byID(f, "notes", sets("position"), "N3", 1))
		wantTargets(t, res, "notes/N3.flowmark.md")
		f.commit(res)
		if got := f.file("notes/N3.flowmark.md"); got != lines("# Second root", "- position: 500") {
			t.Fatalf("N3:\n%s", got)
		}
		f.want(notePositions(f),
			`{"rows":[{"id":"N3","parent":null,"position":1},{"id":"N1","parent":null,"position":2},{"id":"N2","parent":"N1","position":1}]}`)
	})
	t.Run("20#order: a parent change writes the moved file alone", func(t *testing.T) {
		f := ranked(t)
		res := f.ok(byID(f, "notes", sets("parent"), "N3", "N1"))
		wantTargets(t, res, "notes/N3.flowmark.md")
		f.commit(res)
		if got := f.file("notes/N3.flowmark.md"); got != lines("# Second root", "- parent: [N1](N1.flowmark.md)", "- position: 2000") {
			t.Fatalf("N3:\n%s", got)
		}
		f.want(notePositions(f),
			`{"rows":[{"id":"N1","parent":null,"position":1},{"id":"N2","parent":"N1","position":1},{"id":"N3","parent":"N1","position":2}]}`)
	})
	t.Run("20#order: a delete leaves the siblings' ranks", func(t *testing.T) {
		docs := rankedDocs()
		docs["notes/N4.flowmark.md"] = lines("# Third root", "- position: 3000")
		f := catalogFixture(t, docs).on(t)
		res := f.ok(f.msg("delete", "notes", map[string]any{"where": where(pred("id", "eq", p(0)))}, "N3"))
		wantTargets(t, res, "notes/N3.flowmark.md")
		f.commit(res)
		f.want(notePositions(f),
			`{"rows":[{"id":"N1","parent":null,"position":1},{"id":"N2","parent":"N1","position":1},{"id":"N4","parent":null,"position":2}]}`)
	})
	t.Run("20#order: a place with no free rank renumbers the group", func(t *testing.T) {
		f := fresh(t) // the dense ranks 1 and 2: no rank lies between them
		res := f.ok(f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "position")}, "N4", "Fourth", "", 2))
		wantTargets(t, res, "notes/N1.flowmark.md", "notes/N3.flowmark.md", "notes/N4.flowmark.md")
		f.commit(res)
		for _, w := range []struct{ path, rank string }{{"notes/N1.flowmark.md", "1000"}, {"notes/N4.flowmark.md", "2000"}, {"notes/N3.flowmark.md", "3000"}} {
			if got := f.file(w.path); !strings.Contains(got, "- position: "+w.rank+"\n") {
				t.Fatalf("%s:\n%s", w.path, got)
			}
		}
		f.want(notePositions(f),
			`{"rows":[{"id":"N1","parent":null,"position":1},{"id":"N2","parent":"N1","position":1},{"id":"N4","parent":null,"position":2},{"id":"N3","parent":null,"position":3}]}`)
	})
	t.Run("20#order: a place that needs no unreadable rank is written and one that needs it is refused", func(t *testing.T) {
		docs := rankedDocs()
		docs["notes/N4.flowmark.md"] = lines("# Unreadable", "- position: x")
		f := catalogFixture(t, docs).on(t)
		ins := func(id string, k int) []byte {
			return f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "position")}, id, "New", "", k)
		}
		// N1 1000, N3 2000, N4 x: position 3 stands after N3 and before N4.
		res := f.ok(ins("N5", 3))
		wantTargets(t, res, "notes/N5.flowmark.md")
		if r := res.Post.Next.files["notes/N5.flowmark.md"].rows[0].Scalars["position"]; r == nil || *r != "3000" {
			t.Fatalf("N5 rank %v", r)
		}
		wantFaults(t, f.failure(ins("N5", 4), "CONSTRAINT"), nil)
	})
	t.Run("20#order: ranks shared by siblings stand in corpus order and a renumber parts them", func(t *testing.T) {
		docs := rankedDocs()
		docs["notes/N3.flowmark.md"] = lines("# Second root", "- position: 1000")
		f := catalogFixture(t, docs).on(t)
		f.want(notePositions(f),
			`{"rows":[{"id":"N1","parent":null,"position":1},{"id":"N2","parent":"N1","position":1},{"id":"N3","parent":null,"position":2}]}`)
		res := f.ok(f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "position")}, "N4", "Fourth", "", 2))
		wantTargets(t, res, "notes/N3.flowmark.md", "notes/N4.flowmark.md")
		f.commit(res)
		f.want(notePositions(f),
			`{"rows":[{"id":"N1","parent":null,"position":1},{"id":"N2","parent":"N1","position":1},{"id":"N4","parent":null,"position":2},{"id":"N3","parent":null,"position":3}]}`)
	})
}

// TestFilesOrderBound pins the bound of an order write in files storage: inserts at
// one place rewrite the placed row's file alone until no free rank remains there,
// and a renumber then rewrites the group once.
func TestFilesOrderBound(t *testing.T) {
	const n = 100
	docs := baseDocs()
	delete(docs, "notes/N1.flowmark.md")
	delete(docs, "notes/N2.flowmark.md")
	delete(docs, "notes/N3.flowmark.md")
	for i := 1; i <= n; i++ {
		docs[fmt.Sprintf("notes/R%03d.flowmark.md", i)] = lines(fmt.Sprintf("# Root %d", i), fmt.Sprintf("- position: %d", i*1000))
	}
	f := catalogFixture(t, docs).on(t)
	written := []int{}
	for i := 1; i <= 11; i++ {
		res := f.ok(f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "position")}, fmt.Sprintf("X%02d", i), "Front", "", 1))
		written = append(written, len(res.Post.Targets))
		f.commit(res)
	}
	// 1000 halves to 500, 250, 125, 62, 31, 15, 7, 3, 1: nine inserts write one
	// file each; the tenth finds no rank below 1 and renumbers the group of 110.
	want := []int{1, 1, 1, 1, 1, 1, 1, 1, 1, n + 10, 1}
	if !slices.Equal(written, want) {
		t.Fatalf("files written per insert %v; want %v", written, want)
	}
}

// BenchmarkFilesOrderInsertFront measures an insert at position 1 among 1,000
// siblings of a files table stored as ranks: the write renders the inserted row's
// file alone (files/op), where the dense form rendered all 1,001.
func BenchmarkFilesOrderInsertFront(b *testing.B) {
	const n = 1000
	docs := baseDocs()
	delete(docs, "notes/N1.flowmark.md")
	delete(docs, "notes/N2.flowmark.md")
	delete(docs, "notes/N3.flowmark.md")
	for i := 1; i <= n; i++ {
		docs[fmt.Sprintf("notes/R%04d.flowmark.md", i)] = lines(fmt.Sprintf("# Root %d", i), fmt.Sprintf("- position: %d", i*1000))
	}
	f := catalogFixture(b, docs).on(b)
	m := f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "position")}, "X1", "Front", "", 1)
	files := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := f.run(m)
		if err != nil {
			b.Fatal(err)
		}
		files += len(res.Post.Targets)
	}
	b.ReportMetric(float64(files)/float64(b.N), "files/op")
}
