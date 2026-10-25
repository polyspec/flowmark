package writepath

import (
	"context"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/flowmark/packages/go/store/journal"
	"github.com/polyspec/flowmark/packages/go/validate"
)

// noteTitles returns the answer of a read of the id and title of every note.
func (f *fixture) noteTitles(s *Session) string {
	f.t.Helper()
	a, err := s.Exec(context.Background(), f.msg("all", "notes", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"id", "title"}}}))
	if err != nil {
		f.t.Fatal(err)
	}
	return answerJSON(f.t, a)
}

// wantRead requires the corpus documents read from the disk since the last reset to be want, and resets.
func (f *fixture) wantRead(want ...string) {
	f.t.Helper()
	if want == nil {
		want = []string{}
	}
	if got := f.rec.documentsRead(f.t, f.root); !reflect.DeepEqual(got, want) {
		f.t.Fatalf("documents read = %v, want %v", got, want)
	}
	f.rec.reset()
}

// TestTreeOverflow: a notification that reports lost events drops everything the tree holds, and the next operation
// reads the whole corpus again, so a change whose own notification was lost is answered (spec 40 Opening a corpus).
func TestTreeOverflow(t *testing.T) {
	f := baseFixture(t)
	s := f.session("cli")
	f.noteTitles(s)
	f.rec.reset()
	// The edit's notification is lost, and the queue reports an overflow in its place.
	f.c.tree.SetNotices(func(journal.Notices) journal.Notices { return journal.Notices{Overflow: true} })
	f.write("notes/N3.flowmark.md", lines("# Lost root", "- position: 2"))
	if got := f.noteTitles(s); !strings.Contains(got, `"Lost root"`) {
		t.Fatalf("after an overflow the edit is not answered: %s", got)
	}
	if got := f.rec.treeEvents(t, f.root, journal.CorpusOverflow); !reflect.DeepEqual(got, []string{"."}) {
		t.Fatalf("overflow events = %v, want one at the root", got)
	}
	f.wantRead("items.flowmark.md", "notes/N1.flowmark.md", "notes/N2.flowmark.md", "notes/N3.flowmark.md", "people.flowmark.md", "tags.flowmark.md")
	f.c.tree.SetNotices(nil)
	f.noteTitles(s)
	f.wantRead()
}

// TestTreeHandEditRacingCommit: a hand edit of a commit target whose notification has not arrived when the commit
// checks its targets is still found, since the commit reads its targets again under the lock (spec 40 The commit
// protocol, step 1).
func TestTreeHandEditRacingCommit(t *testing.T) {
	late := func(journal.Notices) journal.Notices { return journal.Notices{} }
	t.Run("40#the-commit-protocol: a transaction's commit refuses a target edited by hand", func(t *testing.T) {
		f := baseFixture(t)
		s := f.begun(t, "server")
		want(t, s, f.msg("update", "notes", map[string]any{"set": sets(0, "title"), "where": corpustest.WhereID(1)}, "Tx root", "N3"), `{"affected":1,"keys":[]}`)
		f.c.tree.SetNotices(late)
		hand := lines("# Hand root", "- position: 2")
		f.write("notes/N3.flowmark.md", hand)
		fails(t, s.Commit(), codeWriteConflict, "notes/N3.flowmark.md:1")
		if got := f.file("notes/N3.flowmark.md"); got != hand {
			t.Fatalf("the hand edit was overwritten: %q", got)
		}
	})
	t.Run("40#transactions: a single write retried on the hand edit writes over it", func(t *testing.T) {
		f := baseFixture(t)
		s := f.session("cli")
		f.c.tree.SetNotices(late)
		f.c.readHook = func(attempt int) {
			if attempt == 0 {
				f.write("notes/N3.flowmark.md", lines("# Second root", "- position: 5"))
			}
		}
		want(t, s, f.msg("update", "notes", map[string]any{"set": sets(0, "title"), "where": corpustest.WhereID(1)}, "Written root", "N3"), `{"affected":1,"keys":[]}`)
		if got, want := f.file("notes/N3.flowmark.md"), lines("# Written root", "- position: 5"); got != want {
			t.Fatalf("N3 = %q, want %q", got, want)
		}
	})
}

// TestTreeReadIsTheRead: the read of a long-lived corpus, through its tree and its memo of parses, is the read of the
// corpus from the disk after every change, a change that alters the parse of an unchanged file included: a row that
// loses its detail section makes the links to it in other files noncanonical (spec 40 Opening a corpus).
func TestTreeReadIsTheRead(t *testing.T) {
	f := baseFixture(t)
	same := func(step string) *validate.Snapshot {
		t.Helper()
		if err := f.c.tree.Refresh(); err != nil {
			t.Fatal(err)
		}
		got, err := validate.Read(f.cat, f.c.tree, f.c.memo)
		if err != nil {
			t.Fatal(err)
		}
		want, err := validate.Read(f.cat, store.DirFS(f.root), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: the long-lived read is\n%+v\nthe read from the disk is\n%+v", step, got.Diagnostics, want.Diagnostics)
		}
		return want
	}
	same("40#opening-a-corpus: the corpus as opened")
	items := baseDocs()["items.flowmark.md"]
	i := strings.Index(items, "## items row T1\n")
	j := strings.Index(items, "## items row T2\n")
	f.write("items.flowmark.md", items[:i]+items[j:])
	s := same("40#opening-a-corpus: a row loses its detail section, and the links to it in unchanged files change")
	if !slices.ContainsFunc(s.Diagnostics, func(d catalog.Diagnostic) bool {
		return d.File == "notes/N1.flowmark.md" && d.Rule == "format.noncanonical"
	}) {
		t.Fatalf("the link of the unchanged N1 did not become noncanonical: %+v", s.Diagnostics)
	}
	f.write("notes/N4.flowmark.md", lines("# Fourth", "- item: [T3](../items.flowmark.md)", "- position: 3"))
	same("40#opening-a-corpus: a new file links to a row")
	f.write("items.flowmark.md", items)
	same("40#opening-a-corpus: the detail section comes back")
	f.remove("notes/N4.flowmark.md")
	f.write("notes/N5.flowmark.md", lines("# Fifth", "- parent: [N9](N9.flowmark.md)", "- position: 4"))
	same("40#opening-a-corpus: a dangling link")
	f.write("notes/N9.flowmark.md", lines("# Ninth", "- position: 5"))
	same("40#opening-a-corpus: the dangling link's row appears")
}

// keptHeapBudget is spec 40's memory budget of the corpus kept in memory: the heap per row, its document's bytes, its
// two parses and its watch included, at one row a document — a files table, the most a row costs.
const keptHeapBudget = 8 << 10

// TestKeptHeapBudget: the corpus a long-lived corpus keeps in memory stays within its budget (spec 40 Cost model).
func TestKeptHeapBudget(t *testing.T) {
	const notes = 2000
	f := newFixture(t, executorCatalog(t), benchDocs(notes))
	per := keptHeap(t, f.root, len(benchDocs(notes)))
	t.Logf("40#cost-model: the corpus kept in memory holds %.0f bytes of heap per row", per)
	if per > keptHeapBudget {
		t.Fatalf("the corpus kept in memory holds %.0f bytes of heap per row, past its budget of %d", per, keptHeapBudget)
	}
}

// TestTreeReadsChangedFiles: an operation reads only the corpus files that changed since the last one, as the
// operating system's notifications name them, and never answers from a file's old bytes (spec 40 Opening a corpus).
func TestTreeReadsChangedFiles(t *testing.T) {
	f := baseFixture(t)
	s := f.session("cli")
	has := func(answer string, texts ...string) {
		t.Helper()
		for _, x := range texts {
			if !strings.Contains(answer, x) {
				t.Fatalf("answer %s holds no %s", answer, x)
			}
		}
	}

	t.Log("40#opening-a-corpus: an operation on an unchanged corpus reads no document")
	has(f.noteTitles(s), `"Second root"`)
	f.wantRead()

	t.Log("40#opening-a-corpus: a file edited in place is read again, alone")
	f.write("notes/N3.flowmark.md", lines("# Renamed root", "- position: 2"))
	has(f.noteTitles(s), `"Renamed root"`)
	f.wantRead("notes/N3.flowmark.md")

	t.Log("40#opening-a-corpus: a new file is read, alone")
	f.write("notes/N4.flowmark.md", lines("# Fourth", "- position: 3"))
	has(f.noteTitles(s), `"Fourth"`)
	f.wantRead("notes/N4.flowmark.md")

	t.Log("40#opening-a-corpus: a removed file is gone, and nothing is read")
	f.remove("notes/N4.flowmark.md")
	if got := f.noteTitles(s); strings.Contains(got, "Fourth") {
		t.Fatalf("the removed note is still answered: %s", got)
	}
	f.wantRead()

	t.Log("40#opening-a-corpus: a file replaced by a rename, as an editor saves it, is read again, alone")
	f.write("notes/.N3.swap", lines("# Saved root", "- position: 2"))
	if err := os.Rename(filepath.Join(f.root, "notes", ".N3.swap"), filepath.Join(f.root, "notes", "N3.flowmark.md")); err != nil {
		t.Fatal(err)
	}
	has(f.noteTitles(s), `"Saved root"`)
	f.wantRead("notes/N3.flowmark.md")

	t.Log("40#opening-a-corpus: a write reads its target again under the lock, and the next operation reads it again, alone")
	want(t, s, f.msg("update", "notes", map[string]any{"set": sets(0, "title"), "where": corpustest.WhereID(1)}, "Written root", "N3"), `{"affected":1,"keys":[]}`)
	has(f.noteTitles(s), `"Written root"`)
	if got := f.rec.documentsRead(t, f.root); len(got) == 0 || strings.Join(got, ",") != strings.Repeat("notes/N3.flowmark.md,", len(got)-1)+"notes/N3.flowmark.md" {
		t.Fatalf("documents read = %v, want notes/N3.flowmark.md alone", got)
	}
}
