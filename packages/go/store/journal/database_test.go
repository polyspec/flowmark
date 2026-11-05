package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// databaseFiles are the names of the files of the database of token: its holder record, the database, and the files
// SQLite keeps beside it.
func databaseFiles(token string) []string {
	return []string{token + ".holder", token + ".db", token + ".db-wal", token + ".db-shm", token + ".db-journal"}
}

// TestRecoverRemovesEndedDatabases pins the recovery of the databases of the openings of a corpus (spec 40 SQL
// statements): the files of a process of this host that has ended are removed with its holder record, and those of a
// running process, of a process of another host, of a holder record that does not parse, and any other name, stay.
func TestRecoverRemovesEndedDatabases(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	gone := selfRecord(t, c, "sql")
	gone.PID = endedPID(t)
	other := selfRecord(t, c, "sql")
	other.Host = "another-machine"
	plant := func(token, holder string) {
		for _, name := range databaseFiles(token)[1:] {
			writeFile(t, root, DatabaseDir+"/"+name, "rows")
		}
		writeFile(t, root, DatabaseDir+"/"+token+".holder", holder)
	}
	cases := []struct {
		name   string
		token  string
		holder string
		kept   bool
	}{
		{"an ended process of this host", "00000000000000000000000000000001", string(gone.record()), false},
		{"a running process", "00000000000000000000000000000002", string(selfRecord(t, c, "sql").record()), true},
		{"a process of another host", "00000000000000000000000000000003", string(other.record()), true},
		{"a holder record that does not parse", "00000000000000000000000000000004", "{\"pid\": ", true},
	}
	for _, tc := range cases {
		plant(tc.token, tc.holder)
	}
	writeFile(t, root, DatabaseDir+"/notes.txt", "kept")
	l := acquire(t, c, "s1")
	defer func() {
		if err := l.Release(); err != nil {
			t.Fatal(err)
		}
	}()
	if err := c.Recover(l); err != nil {
		t.Fatal(err)
	}
	exists := func(name string) bool {
		_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(DatabaseDir), name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		return err == nil
	}
	for _, tc := range cases {
		for _, name := range databaseFiles(tc.token) {
			if exists(name) != tc.kept {
				t.Errorf("%s: %s exists %v, want %v", tc.name, name, !tc.kept, tc.kept)
			}
		}
	}
	if !exists("notes.txt") {
		t.Error("the recovery removed a file of another name")
	}
}

// TestRecoverRefusesLinkedDatabases pins that the recovery follows no symbolic link under the databases' folder, nor
// the folder itself when it is one.
func TestRecoverRefusesLinkedDatabases(t *testing.T) {
	cases := map[string]func(t *testing.T, root string){
		"a linked holder record": func(t *testing.T, root string) {
			writeFile(t, root, "elsewhere.holder", "{}")
			if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(DatabaseDir)), 0o700); err != nil {
				t.Fatal(err)
			}
			symlink(t, filepath.Join(root, "elsewhere.holder"), filepath.Join(root, filepath.FromSlash(DatabaseDir), "00000000000000000000000000000001.holder"))
		},
		"a linked folder": func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, "elsewhere"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, ".flowmark"), 0o755); err != nil {
				t.Fatal(err)
			}
			symlink(t, filepath.Join(root, "elsewhere"), filepath.Join(root, filepath.FromSlash(DatabaseDir)))
		},
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			plant(t, root)
			c, _ := newCorpus(t, root)
			l := acquire(t, c, "s1")
			defer func() {
				if err := l.Release(); err != nil {
					t.Fatal(err)
				}
			}()
			if err := c.Recover(l); err == nil || !strings.Contains(err.Error(), "symbolic link") {
				t.Fatalf("Recover = %v, want a symlink refusal", err)
			}
		})
	}
}

// TestNewDatabase pins the place of a new database: the folder is its owner's alone, the holder record of this process
// is written under a new token before the database exists, and Remove takes the database, the files beside it and the
// record away.
func TestNewDatabase(t *testing.T) {
	root := t.TempDir()
	c, _ := newCorpus(t, root)
	d, err := c.NewDatabase()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, filepath.FromSlash(DatabaseDir))
	if err := checkPrivate(dir); err != nil {
		t.Fatalf("the folder made for the databases: %v", err)
	}
	if filepath.Dir(d.Path) != dir || filepath.Base(d.Path) != d.token+".db" {
		t.Fatalf("the database is %s, want %s.db in %s", d.Path, d.token, dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, d.token+".holder"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := parseRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	if h.PID != os.Getpid() || h.Session != "sql" {
		t.Fatalf("the holder record names %s, want this process's sql", h)
	}
	if _, err := os.Lstat(d.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the database exists before SQLite opens it: %v", err)
	}
	e, err := c.NewDatabase()
	if err != nil {
		t.Fatal(err)
	}
	if e.Path == d.Path {
		t.Fatal("two databases of one process share their file")
	}
	for _, name := range databaseFiles(d.token)[1:] {
		writeFile(t, root, DatabaseDir+"/"+name, "rows")
	}
	if err := d.Remove(); err != nil {
		t.Fatal(err)
	}
	for _, name := range databaseFiles(d.token) {
		if _, err := os.Lstat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s after Remove: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, e.token+".holder")); err != nil {
		t.Fatalf("the other database's record after Remove: %v", err)
	}
}

// TestNewDatabaseRefuses pins the refusals of the place of a database: a folder that others may enter, a folder that is
// a link or a file, and a file of the database that exists before it is made, a link included.
func TestNewDatabaseRefuses(t *testing.T) {
	cases := []struct {
		name  string
		plant func(t *testing.T, root string)
		want  string
	}{
		{"a folder others may enter", func(t *testing.T, root string) {
			dir := filepath.Join(root, filepath.FromSlash(DatabaseDir))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}, "its owner's alone"},
		{"a linked folder", func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, "elsewhere"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, ".flowmark"), 0o755); err != nil {
				t.Fatal(err)
			}
			symlink(t, filepath.Join(root, "elsewhere"), filepath.Join(root, filepath.FromSlash(DatabaseDir)))
		}, "symbolic link"},
		{"a file for the folder", func(t *testing.T, root string) {
			writeFile(t, root, DatabaseDir, "no folder")
		}, "not a folder"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.plant(t, root)
			c, _ := newCorpus(t, root)
			if _, err := c.NewDatabase(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewDatabase = %v, want a refusal holding %q", err, tc.want)
			}
		})
	}
	const token = "00000000000000000000000000000009"
	for _, name := range databaseFiles(token) {
		for _, linked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s planted, linked %v", name, linked), func(t *testing.T) {
				root := t.TempDir()
				c, _ := newCorpus(t, root)
				d, err := c.NewDatabase()
				if err != nil {
					t.Fatal(err)
				}
				if err := d.Remove(); err != nil {
					t.Fatal(err)
				}
				p := filepath.Join(root, filepath.FromSlash(DatabaseDir), name)
				want := "exists before"
				if linked {
					writeFile(t, root, "elsewhere", "rows")
					symlink(t, filepath.Join(root, "elsewhere"), p)
					want = "symbolic link"
				} else {
					writeFile(t, root, DatabaseDir+"/"+name, "rows")
				}
				if err := c.checkAbsent(token); err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("checkAbsent = %v, want a refusal holding %q", err, want)
				}
			})
		}
	}
}
