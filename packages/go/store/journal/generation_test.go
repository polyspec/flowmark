package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixtureGeneration is the generation after the fixture's commit, numbered n, is the last of n commits of it.
func fixtureGeneration(n uint64) Generation {
	return Generation{N: n, Commits: []GenerationCommit{{N: n, Paths: []string{"a.md", "c.md", "d.md", "sub/b.md"}}}}
}

// wantGeneration requires the generation of c to be want.
func wantGeneration(t *testing.T, c *Corpus, want Generation) {
	t.Helper()
	got, err := c.Generation()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generation = %+v, want %+v", got, want)
	}
}

// generationOf returns a generation whose last commit is n and which keeps the commits from first, each of which wrote
// the path p<number>.md.
func generationOf(first, n uint64) Generation {
	g := Generation{N: n, Commits: []GenerationCommit{}}
	for i := first; i <= n && i > 0; i++ {
		g.Commits = append(g.Commits, GenerationCommit{N: i, Paths: []string{fmt.Sprintf("p%d.md", i)}})
	}
	return g
}

// TestGenerationSince pins the paths a corpus read at a generation learns (spec 40 The commit generation): the paths
// of every commit after it, sorted and without repeats, or none known when a commit after it is no longer kept.
func TestGenerationSince(t *testing.T) {
	repeated := Generation{N: 3, Commits: []GenerationCommit{{N: 2, Paths: []string{"b.md", "a.md"}}, {N: 3, Paths: []string{"a.md", "c.md"}}}}
	for _, tc := range []struct {
		name  string
		g     Generation
		since uint64
		want  []string
		ok    bool
	}{
		{"no commit yet", generationOf(1, 0), 0, []string{}, true},
		{"read at the generation", generationOf(1, 5), 5, []string{}, true},
		{"one commit after", generationOf(1, 5), 4, []string{"p5.md"}, true},
		{"every kept commit", generationOf(1, 5), 0, []string{"p1.md", "p2.md", "p3.md", "p4.md", "p5.md"}, true},
		{"the oldest kept commit is the next", generationOf(10, 20), 9, []string{"p10.md", "p11.md", "p12.md", "p13.md", "p14.md", "p15.md", "p16.md", "p17.md", "p18.md", "p19.md", "p20.md"}, true},
		{"a commit after it is no longer kept", generationOf(10, 20), 8, nil, false},
		{"read at a generation after the corpus's", generationOf(1, 5), 6, nil, false},
		{"a path two commits wrote is named once", repeated, 1, []string{"a.md", "b.md", "c.md"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.g.Since(tc.since)
			if ok != tc.ok || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Since(%d) = %v, %v; want %v, %v", tc.since, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestGenerationFile pins the form of the generation file: one object whose commits are numbered in order up to its
// generation, at most Generations of them, each path a clean corpus path; anything else is refused naming the file.
func TestGenerationFile(t *testing.T) {
	valid := func(g Generation) string {
		b, err := json.Marshal(g)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, tc := range []struct {
		name, text, refusal string
	}{
		{"no commit", valid(generationOf(1, 0)), ""},
		{"one commit", valid(generationOf(1, 1)), ""},
		{"the kept commits", valid(generationOf(37, 100)), ""},
		{"a deleted path and a path in a folder", `{"generation": 2, "commits": [{"generation": 2, "paths": ["a/b.md", "c.md"]}]}`, ""},
		{"no object", ``, "holds no generation"},
		{"not JSON", `generation 1`, "holds no generation"},
		{"two objects", valid(generationOf(1, 1)) + valid(generationOf(1, 1)), "more than one generation"},
		{"an unknown member", `{"generation": 0, "commits": [], "next": 1}`, "holds no generation"},
		{"no commits member", `{"generation": 0}`, "holds 0 commits"},
		{"a generation without its commit", `{"generation": 3, "commits": []}`, "holds 0 commits at generation 3"},
		{"a commit at generation 0", `{"generation": 0, "commits": [{"generation": 0, "paths": []}]}`, "holds 1 commits at generation 0"},
		{"more commits than kept", valid(generationOf(1, Generations+1)), fmt.Sprintf("holds %d commits", Generations+1)},
		{"a commit missing between", `{"generation": 3, "commits": [{"generation": 1, "paths": []}, {"generation": 3, "paths": []}]}`, "out of the order"},
		{"commits out of order", `{"generation": 2, "commits": [{"generation": 2, "paths": []}, {"generation": 1, "paths": []}]}`, "out of the order"},
		{"the last commit is not the generation", `{"generation": 2, "commits": [{"generation": 1, "paths": []}]}`, "out of the order"},
		{"a path outside the corpus", `{"generation": 1, "commits": [{"generation": 1, "paths": ["../a.md"]}]}`, "not a clean path"},
		{"an absolute path", `{"generation": 1, "commits": [{"generation": 1, "paths": ["/a.md"]}]}`, "not a clean path"},
		{"a backslash", `{"generation": 1, "commits": [{"generation": 1, "paths": ["a\\b.md"]}]}`, "backslash"},
		{"the journal's own folder", `{"generation": 1, "commits": [{"generation": 1, "paths": [".flowmark/lock"]}]}`, ".flowmark"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, GenerationPath, tc.text)
			c, _ := newCorpus(t, root)
			_, err := c.Generation()
			if tc.refusal == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.refusal) || !strings.Contains(err.Error(), filepath.Join(root, ".flowmark", "generation")) {
				t.Fatalf("Generation = %v, want a refusal naming %q and the file", err, tc.refusal)
			}
		})
	}
	t.Run("a folder in its place", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".flowmark", "generation"), 0o755); err != nil {
			t.Fatal(err)
		}
		c, _ := newCorpus(t, root)
		if _, err := c.Generation(); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("Generation = %v, want not a regular file", err)
		}
	})
	t.Run("a symbolic link in its place", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "elsewhere", valid(generationOf(1, 1)))
		if err := os.MkdirAll(filepath.Join(root, ".flowmark"), 0o755); err != nil {
			t.Fatal(err)
		}
		symlink(t, filepath.Join(root, "elsewhere"), filepath.Join(root, ".flowmark", "generation"))
		c, _ := newCorpus(t, root)
		if _, err := c.Generation(); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Generation = %v, want the link refused", err)
		}
	})
}

// TestGeneration pins the commit generation (spec 40 The commit generation): each commit takes the next number at its
// prepare record and writes it with its paths after its renames, before its stage is collected, the last Generations commits are kept,
// and another handle of the corpus learns it from the file alone.
func TestGeneration(t *testing.T) {
	t.Run("a corpus no commit wrote is generation 0", func(t *testing.T) {
		c, _ := newCorpus(t, t.TempDir())
		wantGeneration(t, c, generationOf(1, 0))
	})
	t.Run("each commit takes the next generation with its paths", func(t *testing.T) {
		root, changes, _, after := fixture(t)
		c, rec := newCorpus(t, root)
		l := acquire(t, c, "s1")
		wantCommitted(t, c, l, changes, root, after)
		wantGeneration(t, c, fixtureGeneration(1))
		got := rec.waitFor(t, 1, kindIs(Generated))
		if got[0].Path != c.abs(GenerationPath) || got[0].Detail != "1" {
			t.Fatalf("generation event = %+v", got[0])
		}
		if err := c.Commit(l, []Change{{Path: "a.md", Data: []byte("newer a\n")}}); err != nil {
			t.Fatal(err)
		}
		want := fixtureGeneration(1)
		want.N = 2
		want.Commits = append(want.Commits, GenerationCommit{N: 2, Paths: []string{"a.md"}})
		wantGeneration(t, c, want)
	})
	t.Run("another handle learns a commit from the generation", func(t *testing.T) {
		root, changes, _, _ := fixture(t)
		writer, _ := newCorpus(t, root)
		reader, _ := newCorpus(t, root)
		before, err := reader.Generation()
		if err != nil {
			t.Fatal(err)
		}
		l := acquire(t, writer, "s1")
		if err := writer.Commit(l, changes); err != nil {
			t.Fatal(err)
		}
		g, err := reader.Generation()
		if err != nil {
			t.Fatal(err)
		}
		if paths, ok := g.Since(before.N); !ok || !reflect.DeepEqual(paths, []string{"a.md", "c.md", "d.md", "sub/b.md"}) {
			t.Fatalf("the reader learns %v, %v", paths, ok)
		}
	})
	t.Run("the last Generations commits are kept", func(t *testing.T) {
		// The generation holds a full window of Generations commits, as after that many commits; each commit after it
		// drops the oldest.
		root := t.TempDir()
		full, err := json.Marshal(generationOf(1, Generations))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, GenerationPath, string(full))
		c, _ := newCorpus(t, root)
		l := acquire(t, c, "s1")
		for i := Generations + 1; i <= Generations+2; i++ {
			if err := c.Commit(l, []Change{{Path: fmt.Sprintf("p%d.md", i), Data: []byte("x\n")}}); err != nil {
				t.Fatal(err)
			}
			wantGeneration(t, c, generationOf(uint64(i-Generations+1), uint64(i)))
		}
	})
	t.Run("an unreadable generation refuses a commit before anything is staged", func(t *testing.T) {
		root, changes, before, _ := fixture(t)
		writeFile(t, root, GenerationPath, "damaged")
		c, _ := newCorpus(t, root)
		l := acquire(t, c, "s1")
		var ce *CommitError
		if err := c.Commit(l, changes); !errors.As(err, &ce) || ce.Prepared || !strings.Contains(err.Error(), "holds no generation") {
			t.Fatalf("Commit = %v, want the generation refused before the prepare record", err)
		}
		if got := snapshot(t, root); !reflect.DeepEqual(got, before) {
			t.Fatalf("corpus = %v", got)
		}
		if s := stages(t, c); len(s) != 0 {
			t.Fatalf("stages = %v", s)
		}
	})
	t.Run("a generation file removed by hand keeps the commit alone", func(t *testing.T) {
		root, changes, _, _ := fixture(t)
		c, _ := newCorpus(t, root)
		l := acquire(t, c, "s1")
		for i := 1; i <= 3; i++ {
			if err := c.Commit(l, []Change{{Path: fmt.Sprintf("p%d.md", i), Data: []byte("x\n")}}); err != nil {
				t.Fatal(err)
			}
		}
		c.fault = crashAt("renames", 0)
		if err := c.Commit(l, changes); !errors.Is(err, errCrash) {
			t.Fatalf("Commit = %v, want the crash", err)
		}
		c.fault = nil
		if err := os.Remove(filepath.Join(root, ".flowmark", "generation")); err != nil {
			t.Fatal(err)
		}
		if err := c.Recover(l); err != nil {
			t.Fatal(err)
		}
		wantGeneration(t, c, fixtureGeneration(4))
		if _, ok := fixtureGeneration(4).Since(2); ok {
			t.Fatal("a corpus read at generation 2 learns paths across the removed commits")
		}
	})
	t.Run("a stage whose collection was interrupted is collected", func(t *testing.T) {
		c, l, root, _, _, after := crashed(t, "collect", 0)
		if s := stages(t, c); len(s) != 1 {
			t.Fatalf("stages = %v, want the prepare record kept", s)
		}
		wantRecovered(t, c, l, root, after)
		wantGeneration(t, c, fixtureGeneration(1))
		if s := stages(t, c); len(s) != 0 {
			t.Fatalf("stages = %v", s)
		}
	})
	t.Run("a stage whose generation is written is collected without its targets judged", func(t *testing.T) {
		// The renames are done once the generation names the commit, so an edit made after it is the corpus's and no
		// change that landed while the commit was interrupted.
		c, l, root, _, _, after := crashed(t, "generation", 0)
		writeFile(t, root, "a.md", "edited after the commit\n")
		after["a.md"] = "edited after the commit\n"
		wantRecovered(t, c, l, root, after)
		wantGeneration(t, c, fixtureGeneration(1))
	})
}
