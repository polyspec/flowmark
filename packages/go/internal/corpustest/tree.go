package corpustest

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNow is the request clock of every request of the tests.
const TestNow = "2026-10-11 09:00:00.123456"

// CaseSensitiveEnv names the folder, on a case-sensitive file system, where a test writes a corpus whose file names
// differ only in case; Kit provisions it for make test-go with CASE_SENSITIVE_ENV. It is read only when the test's
// temporary folder folds case, and must then be set and absolute.
const CaseSensitiveEnv = "FLOWMARK_CASE_SENSITIVE_DIR"

// Lines joins lines with line feeds and a final one.
func Lines(ls ...string) string { return strings.Join(ls, "\n") + "\n" }

// ModuleRoot returns the root of the module: the nearest ancestor of the test's working directory that holds go.mod.
// The go tool declares that working directory to be the package directory, and go.mod declares the module root, so
// another package's testdata is found from a declared root rather than from where the test's package stands.
func ModuleRoot(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	for dir := wd; ; {
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && fi.Mode().IsRegular() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("module root: no ancestor of %s holds go.mod", wd)
		}
		dir = parent
	}
}

// GoldenDir returns the directory of a golden corpus of the store package, <module root>/store/testdata/<name>.
func GoldenDir(t testing.TB, name string) string {
	t.Helper()
	return filepath.Join(ModuleRoot(t), "store", "testdata", filepath.FromSlash(name))
}

// CopyTree copies the regular files of src into dst.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// WriteFiles writes files (slash-separated corpus path to text) under root, creating their folders.
func WriteFiles(t testing.TB, root string, files map[string]string) {
	t.Helper()
	for p, text := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// WriteTree writes files under root as WriteFiles does, and then the set file of root/dbspec (WriteSetFile).
func WriteTree(t testing.TB, root string, files map[string]string) {
	t.Helper()
	WriteFiles(t, root, files)
	if err := WriteSetFile(filepath.Join(root, "dbspec")); err != nil {
		t.Fatal(err)
	}
}

// NewTree writes files under a fresh temporary root as WriteTree does and returns the root.
func NewTree(t testing.TB, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	WriteTree(t, root, files)
	return root
}

// FoldsCase reports whether the file system of dir folds case: a file written as CaseProbe is found as caseprobe.
func FoldsCase(t testing.TB, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(probe); err != nil {
			t.Error(err)
		}
	}()
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

// CaseSensitiveRoot returns an empty folder on a case-sensitive file system: the test's temporary folder when its file
// system keeps case, and otherwise a new folder under the one CaseSensitiveEnv declares. It fails the test when neither
// keeps case.
func CaseSensitiveRoot(t testing.TB) string {
	t.Helper()
	root := t.TempDir()
	if !FoldsCase(t, root) {
		return root
	}
	dir := os.Getenv(CaseSensitiveEnv)
	if dir == "" {
		t.Fatalf("the file system of %s folds case, so two names that differ only in case cannot be written; set %s to an absolute folder on a case-sensitive file system", root, CaseSensitiveEnv)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("%s=%s is not an absolute path", CaseSensitiveEnv, dir)
	}
	root, err := os.MkdirTemp(dir, "flowmark-case-")
	if err != nil {
		t.Fatalf("%s=%s: %v", CaseSensitiveEnv, dir, err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	if FoldsCase(t, root) {
		t.Fatalf("%s=%s folds case", CaseSensitiveEnv, dir)
	}
	return root
}

// Message builds a request message of the executor: the request of kind on entity with the fields of body, the
// manifest hash hash, the parameters params and the request clock now.
func Message(t testing.TB, hash, now, kind, entity string, body map[string]any, params ...any) []byte {
	t.Helper()
	req := map[string]any{}
	for k, v := range body {
		req[k] = v
	}
	req["ir_version"] = 1
	req["manifest_hash"] = hash
	req["kind"] = kind
	req["entity"] = entity
	req["n_params"] = len(params)
	if params == nil {
		params = []any{}
	}
	b, err := json.Marshal(map[string]any{"ir_version": 1, "manifest_hash": hash, "request": req, "params": params, "now": now})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// WhereID is the where of the row whose id is parameter i.
func WhereID(i int) map[string]any {
	return map[string]any{"items": []any{map[string]any{"pred": map[string]any{"column": "id", "op": "eq", "p": i}}}}
}

// PeopleDBS is the catalog of one people document table.
const PeopleDBS = `dbspec 1 people

table people {
  id varchar(32)
  name varchar(64)
  primary key (id)
  settings {
    store document table
    title name
  }
}
`

// PeopleRoot writes a corpus of PeopleDBS with one row, P1 Ada, under a fresh root and returns the root.
func PeopleRoot(t testing.TB) string {
	t.Helper()
	return NewTree(t, map[string]string{
		"dbspec/people.dbs":  PeopleDBS,
		"people.flowmark.md": "# people\n| id | name |\n| --- | --- |\n| P1 | Ada |\n",
	})
}

// EqualLines fails the test unless the lines got are the lines want.
func EqualLines(t testing.TB, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("answers:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Opened fails the test on err, the error of opening c, and closes c when the test ends, failing it when the close fails.
func Opened(t testing.TB, c io.Closer, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}

// WantExit runs the child process cmd and requires it to exit with code, the exit of a crash that a test provokes.
func WantExit(t testing.TB, cmd *exec.Cmd, code int) {
	t.Helper()
	out, err := cmd.Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != code {
		t.Fatalf("child = %v %q, want the exit %d of the crash", err, out, code)
	}
}
