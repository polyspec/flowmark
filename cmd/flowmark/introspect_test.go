package main

import (
	"bytes"
	"encoding/json"
	"github.com/polyspec/flowmark/internal/corpustest"
	"os"
	"path/filepath"
	"testing"
)

// introspectCorpus writes canonDBS and the given data files under a fresh root and returns it.
func introspectCorpus(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["dbspec/canon.dbs"] = canonDBS
	for p, text := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := corpustest.WriteSetFile(filepath.Join(root, "dbspec")); err != nil {
		t.Fatal(err)
	}
	return root
}

// introspectAnswer is the JSON object flowmark introspect prints.
type introspectAnswer struct {
	Catalog *struct {
		ManifestHash string `json:"manifest_hash"`
		Documents    []struct {
			File string `json:"file"`
			Name string `json:"name"`
			Text string `json:"text"`
		} `json:"documents"`
	} `json:"catalog"`
	Diagnostics []struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Column  int    `json:"column"`
		Rule    string `json:"rule"`
		Message string `json:"message"`
	} `json:"diagnostics"`
}

// runIntrospect runs the command and decodes its standard output, requiring exactly one JSON value there.
func runIntrospect(t *testing.T, args []string) (int, introspectAnswer, map[string]json.RawMessage, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := introspectTo(&out, &errOut, args)
	var ans introspectAnswer
	var raw map[string]json.RawMessage
	if code != 2 && out.Len() > 0 {
		dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
		if err := dec.Decode(&raw); err != nil {
			t.Fatalf("standard output is not one JSON object: %v\n%s", err, out.String())
		}
		if dec.More() {
			t.Fatalf("standard output holds more than one JSON value:\n%s", out.String())
		}
		if err := json.Unmarshal(out.Bytes(), &ans); err != nil {
			t.Fatal(err)
		}
	}
	return code, ans, raw, errOut.String()
}

// TestIntrospectCommand pins the CLI face of introspection (spec 30 Introspection).
func TestIntrospectCommand(t *testing.T) {
	t.Run("30#introspection: a valid corpus prints its catalog and exits 0", func(t *testing.T) {
		root := introspectCorpus(t, map[string]string{"tags.flowmark.md": "# tags\n| id | label |\n| --- | --- |\n| 1 | go |\n"})
		code, ans, raw, errText := runIntrospect(t, []string{root})
		if code != 0 || errText != "" {
			t.Fatalf("exit %d, stderr %q", code, errText)
		}
		if ans.Catalog == nil || ans.Catalog.ManifestHash == "" || len(ans.Catalog.Documents) != 1 {
			t.Fatalf("catalog = %+v", ans.Catalog)
		}
		d := ans.Catalog.Documents[0]
		if d.File != "dbspec/canon.dbs" || d.Name != "canon" || d.Text != canonDBS {
			t.Fatalf("document = %+v", d)
		}
		if string(raw["diagnostics"]) != "[]" {
			t.Fatalf("diagnostics = %s, want []", raw["diagnostics"])
		}
	})
	t.Run("30#introspection: a failing document prints the catalog with its diagnostic and exits 1", func(t *testing.T) {
		root := introspectCorpus(t, map[string]string{"notes/N1.flowmark.md": "# A note\xff\n"})
		code, ans, _, errText := runIntrospect(t, []string{root})
		if code != 1 || errText != "" {
			t.Fatalf("exit %d, stderr %q", code, errText)
		}
		if ans.Catalog == nil || len(ans.Catalog.Documents) != 1 {
			t.Fatalf("catalog = %+v", ans.Catalog)
		}
		if len(ans.Diagnostics) != 1 || ans.Diagnostics[0].File != "notes/N1.flowmark.md" || ans.Diagnostics[0].Rule != "format.encoding" || ans.Diagnostics[0].Line != 1 {
			t.Fatalf("diagnostics = %+v", ans.Diagnostics)
		}
	})
	t.Run("30#introspection: a corpus without a catalog prints a null catalog and exits 1", func(t *testing.T) {
		code, ans, raw, _ := runIntrospect(t, []string{t.TempDir()})
		if code != 1 {
			t.Fatalf("exit %d", code)
		}
		if string(raw["catalog"]) != "null" {
			t.Fatalf("catalog = %s, want null", raw["catalog"])
		}
		if len(ans.Diagnostics) != 1 || ans.Diagnostics[0].File != "dbspec" || ans.Diagnostics[0].Rule != "catalog" || ans.Diagnostics[0].Message != "no dbspec/ directory" {
			t.Fatalf("diagnostics = %+v", ans.Diagnostics)
		}
	})
	t.Run("30#introspection: a wrong command line exits 2", func(t *testing.T) {
		for _, args := range [][]string{nil, {"a", "b"}} {
			var out, errOut bytes.Buffer
			if code := introspectTo(&out, &errOut, args); code != 2 || out.Len() != 0 || errOut.Len() == 0 {
				t.Fatalf("%q: exit %d, stdout %q, stderr %q", args, code, out.String(), errOut.String())
			}
		}
	})
}
