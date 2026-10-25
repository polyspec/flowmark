package writepath

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// installOf runs flowmark install in a new session and fails the test on an error.
func (f *fixture) installOf() *InstallResult {
	f.t.Helper()
	r, err := f.session("install").Install(context.Background())
	if err != nil {
		f.t.Fatalf("Install = %v", err)
	}
	return r
}

// isFolder reports whether the corpus path p is a folder.
func (f *fixture) isFolder(p string) bool {
	f.t.Helper()
	fi, err := os.Lstat(filepath.Join(f.root, filepath.FromSlash(p)))
	return err == nil && fi.IsDir()
}

// TestInstall pins flowmark install (spec 30 Install, spec 40 The faces): the storage skeleton of the catalog and the
// .gitattributes line, as its own idempotent transaction.
func TestInstall(t *testing.T) {
	const line = "*.flowmark.md text eol=lf\n"
	t.Run("40#the-faces: an empty corpus gets every document, folder and the .gitattributes line", func(t *testing.T) {
		f := newFixture(t, cardsDBS, nil)
		r := f.installOf()
		if want := (&InstallResult{Created: []string{"cards", "pins.flowmark.md"}, Attributes: true}); !reflect.DeepEqual(r, want) {
			t.Fatalf("Install = %+v, want %+v", r, want)
		}
		if got := f.file("pins.flowmark.md"); got != "# pins\n" {
			t.Fatalf("pins = %q", got)
		}
		if !f.isFolder("cards") {
			t.Fatal("the folder of cards was not created")
		}
		if got := f.file(".gitattributes"); got != line {
			t.Fatalf(".gitattributes = %q", got)
		}
		f.clean()
	})
	t.Run("40#the-faces: an install that finds everything in place writes nothing", func(t *testing.T) {
		f := newFixture(t, cardsDBS, nil)
		f.installOf()
		f.rec.reset()
		r := f.installOf()
		if len(r.Created) != 0 || r.Attributes {
			t.Fatalf("second Install = %+v", r)
		}
		for _, k := range f.rec.kinds() {
			if k != "lock.acquired" && k != "lock.released" {
				t.Fatalf("the second install published %v", f.rec.kinds())
			}
		}
	})
	t.Run("30#install: .gitattributes keeps its other lines, and a last line without LF gets one", func(t *testing.T) {
		f := newFixture(t, cardsDBS, map[string]string{".gitattributes": "*.png binary"})
		f.installOf()
		if got := f.file(".gitattributes"); got != "*.png binary\n"+line {
			t.Fatalf(".gitattributes = %q", got)
		}
	})
	t.Run("30#install: a .gitattributes that holds the line is kept as it is", func(t *testing.T) {
		text := line + "*.png binary\n"
		f := newFixture(t, cardsDBS, map[string]string{".gitattributes": text})
		if r := f.installOf(); r.Attributes {
			t.Fatalf("Install = %+v", r)
		}
		if got := f.file(".gitattributes"); got != text {
			t.Fatalf(".gitattributes = %q", got)
		}
	})
	t.Run("30#install: an existing document keeps its rows", func(t *testing.T) {
		pins := lines("# pins", "", "| id | card |", "| --- | --- |", "| P1 | C1 |")
		f := newFixture(t, cardsDBS, map[string]string{"pins.flowmark.md": pins, "cards/C1.flowmark.md": "# One\n"})
		r := f.installOf()
		if len(r.Created) != 0 {
			t.Fatalf("Install = %+v", r)
		}
		if got := f.file("pins.flowmark.md"); got != pins {
			t.Fatalf("pins = %q", got)
		}
	})
	t.Run("40#the-faces: another kind of entry at a table's place refuses the install, and nothing is written", func(t *testing.T) {
		f := newFixture(t, cardsDBS, nil)
		if err := os.Mkdir(filepath.Join(f.root, "pins.flowmark.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := f.session("install").Install(context.Background())
		fails(t, err, "CONFIG", "pins.flowmark.md")
		if f.isFolder("cards") || f.file(".gitattributes") != "" {
			t.Fatal("the refused install wrote")
		}
	})
	t.Run("40#the-faces: a .gitattributes that is no regular file refuses the install", func(t *testing.T) {
		f := newFixture(t, cardsDBS, nil)
		if err := os.Mkdir(filepath.Join(f.root, ".gitattributes"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := f.session("install").Install(context.Background())
		fails(t, err, "CONFIG", ".gitattributes")
		if f.file("pins.flowmark.md") != "" {
			t.Fatal("the refused install wrote")
		}
	})
	t.Run("40#the-faces: install inside an open transaction is CONFIG", func(t *testing.T) {
		f := newFixture(t, cardsDBS, nil)
		s := f.begun(t, "tx")
		_, err := s.Install(context.Background())
		fails(t, err, "CONFIG", "its own transaction")
		if err := s.Rollback(); err != nil {
			t.Fatal(err)
		}
	})
}
