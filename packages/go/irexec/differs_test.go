package irexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/store"
)

// TestCorpusData pins the bytes and the differences a corpus reports: the write
// path reads them to check that a target is unchanged and to name a change.
func TestCorpusData(t *testing.T) {
	f := baseFixture(t)
	if got := string(f.c.Data("tags.flowmark.md")); got != baseDocs()["tags.flowmark.md"] {
		t.Fatalf("Data(tags) = %q", got)
	}
	if got := f.c.Data("missing.flowmark.md"); got != nil {
		t.Fatalf("Data(missing) = %q, want nil", got)
	}
	same, err := Open(f.cat, store.DirFS(f.root), nil)
	if err != nil {
		t.Fatal(err)
	}
	if d := f.c.Differs(same); len(d) != 0 {
		t.Fatalf("an unchanged corpus differs at %v", d)
	}
	write := func(p, text string) {
		if err := os.WriteFile(filepath.Join(f.root, filepath.FromSlash(p)), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("tags.flowmark.md", baseDocs()["tags.flowmark.md"]+"| 3 | web | area |\n")
	if err := os.Remove(filepath.Join(f.root, "notes", "N3.flowmark.md")); err != nil {
		t.Fatal(err)
	}
	write("notes/N4.flowmark.md", lines("# Fourth", "- position: 3"))
	changed, err := Open(f.cat, store.DirFS(f.root), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(f.c.Differs(changed), ","), "notes/N3.flowmark.md,notes/N4.flowmark.md,tags.flowmark.md"; got != want {
		t.Fatalf("Differs = %s, want %s", got, want)
	}
	// A walk entry that makes a table unavailable is a difference too, at its place.
	if err := os.Symlink("../tags.flowmark.md", filepath.Join(f.root, "notes", "N5.flowmark.md")); err != nil {
		t.Fatalf("the platform refuses a symbolic link, which the walk test needs: %v", err)
	}
	walked, err := Open(f.cat, store.DirFS(f.root), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(changed.Differs(walked), ","); got != "notes/N5.flowmark.md" {
		t.Fatalf("Differs = %s, want the walk entry", got)
	}
}
