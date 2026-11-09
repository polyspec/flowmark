package irexec

import (
	"fmt"
	"slices"
	"testing"
)

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

// TestOrderValues pins the order column (spec 20 Order): any integer of its type, answered as written, the siblings
// standing by ascending value, those that share a value in key order and those whose value is not of its type last; a
// write of a value rewrites the written row's file alone.
func TestOrderValues(t *testing.T) {
	cases := []struct {
		name string
		docs map[string]string
		want string
	}{
		{"20#order: siblings stand by ascending value", map[string]string{
			"notes/N1.flowmark.md": lines("# First note", "- position: 30"),
			"notes/N3.flowmark.md": lines("# Second root", "- position: -4"),
		}, `{"rows":[{"id":"N3","parent":null,"position":-4},{"id":"N1","parent":null,"position":30},{"id":"N2","parent":"N1","position":1}]}`},
		{"20#order: siblings that share a value stand in key order", map[string]string{
			"notes/N1.flowmark.md": lines("# First note", "- position: 2"),
		}, `{"rows":[{"id":"N1","parent":null,"position":2},{"id":"N2","parent":"N1","position":1},{"id":"N3","parent":null,"position":2}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			docs := baseDocs()
			for p, text := range tc.docs {
				docs[p] = text
			}
			f := catalogFixture(t, docs).on(t)
			f.want(notePositions(f), tc.want)
		})
	}
	t.Run("20#order: a sibling whose value is not of its type stands last", func(t *testing.T) {
		docs := baseDocs()
		docs["notes/N0.flowmark.md"] = lines("# Unreadable", "- position: x")
		f := catalogFixture(t, docs).on(t)
		f.want(f.msg("all", "notes", map[string]any{"columns": map[string]any{"mode": "none", "remove": []string{"item", "parent"}}}),
			`{"rows":[{"id":"N1"},{"id":"N2"},{"id":"N3"},{"id":"N0"}]}`)
	})
	t.Run("20#order: an insert before every sibling writes its own file alone", func(t *testing.T) {
		f := fresh(t)
		res := f.want(f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "position")}, "N4", "Fourth", "", 0), `{"affected":1,"keys":["N4"]}`)
		wantTargets(t, res, "notes/N4.flowmark.md")
		f.commit(res)
		f.want(notePositions(f),
			`{"rows":[{"id":"N4","parent":null,"position":0},{"id":"N1","parent":null,"position":1},{"id":"N2","parent":"N1","position":1},{"id":"N3","parent":null,"position":2}]}`)
	})
	t.Run("20#order: a parent change writes the moved file alone", func(t *testing.T) {
		f := fresh(t)
		res := f.ok(f.msg("update", "notes", map[string]any{"set": sets("parent"), "where": where(pred("id", "eq", p(1)))}, "N1", "N3"))
		wantTargets(t, res, "notes/N3.flowmark.md")
		f.commit(res)
		if got := f.file("notes/N3.flowmark.md"); got != lines("# Second root", "- parent: [N1](N1.flowmark.md)", "- position: 2") {
			t.Fatalf("N3:\n%s", got)
		}
	})
	t.Run("20#order: a delete leaves the siblings' values", func(t *testing.T) {
		f := fresh(t)
		res := f.ok(f.msg("delete", "notes", map[string]any{"where": where(pred("id", "eq", p(0)))}, "N2"))
		wantTargets(t, res, "notes/N2.flowmark.md")
	})
}

// BenchmarkFilesOrderInsertFront measures an insert before 1,000 siblings of a files table: the write renders the
// inserted row's file alone (files/op), since no sibling's value changes.
func BenchmarkFilesOrderInsertFront(b *testing.B) {
	const n = 1000
	docs := baseDocs()
	delete(docs, "notes/N1.flowmark.md")
	delete(docs, "notes/N2.flowmark.md")
	delete(docs, "notes/N3.flowmark.md")
	for i := 1; i <= n; i++ {
		docs[fmt.Sprintf("notes/R%04d.flowmark.md", i)] = lines(fmt.Sprintf("# Root %d", i), fmt.Sprintf("- position: %d", i))
	}
	f := catalogFixture(b, docs).on(b)
	m := f.msg("insert", "notes", map[string]any{"set": sets("id", "title", "body", "position")}, "X1", "Front", "", 0)
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
