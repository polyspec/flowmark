package main

import (
	"bytes"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const canonDBS = `dbspec 1 canon

table tags {
  id i64 identity
  label varchar(16)
  primary key (id)
  settings {
    store document table
  }
}

table notes {
  id varchar(16)
  title varchar(64)
  primary key (id)
  settings {
    store files
    title title
  }
}
`

// canonCorpus writes a corpus with a noncanonical tags document and an unreadable note, and returns its root.
func canonCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for p, text := range map[string]string{
		"dbspec/canon.dbs":     canonDBS,
		"tags.flowmark.md":     "# tags\r\n| id | label |\r\n| --- | --- |\r\n| 1 | go |\r\n",
		"notes/N1.flowmark.md": "# A note\xff\n",
	} {
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

// TestCanonCommand pins the CLI face of flowmark canon (spec 40 The canon operation): it rewrites the corpus and
// prints each file rewritten and each file left; a named file canon cannot rewrite fails it, and nothing is written.
func TestCanonCommand(t *testing.T) {
	canonical := "# tags\n| id | label |\n| --- | --- |\n| 1 | go |\n"
	t.Run("40#the-canon-operation: the corpus is rewritten and the unreadable file left", func(t *testing.T) {
		root := canonCorpus(t)
		var out, errOut bytes.Buffer
		if code := canonTo(&out, &errOut, []string{"--lock-wait-ms", "0", root}); code != 0 {
			t.Fatalf("code %d: %s", code, errOut.String())
		}
		want := "[canon] rewrote tags.flowmark.md\n" +
			"[canon] left notes/N1.flowmark.md: notes/N1.flowmark.md:1:9 format.encoding: the file is not UTF-8\n" +
			"[canon] 1 rewritten, 1 left\n"
		if out.String() != want {
			t.Fatalf("out:\n%s\nwant:\n%s", out.String(), want)
		}
		if b, _ := os.ReadFile(filepath.Join(root, "tags.flowmark.md")); string(b) != canonical {
			t.Fatalf("tags:\n%q", b)
		}
	})
	t.Run("40#the-canon-operation: a named file canon cannot rewrite fails the command", func(t *testing.T) {
		root := canonCorpus(t)
		var out, errOut bytes.Buffer
		if code := canonTo(&out, &errOut, []string{root, "tags.flowmark.md", filepath.FromSlash("notes/N1.flowmark.md")}); code != 1 {
			t.Fatalf("code %d, want 1", code)
		}
		if !strings.Contains(errOut.String(), "CONSTRAINT") || !strings.Contains(errOut.String(), "notes/N1.flowmark.md") {
			t.Fatalf("stderr: %s", errOut.String())
		}
		if b, _ := os.ReadFile(filepath.Join(root, "tags.flowmark.md")); string(b) == canonical {
			t.Fatal("tags was rewritten")
		}
	})
	t.Run("40#the-canon-operation: a command line without a corpus is a usage error", func(t *testing.T) {
		var out, errOut bytes.Buffer
		if code := canonTo(&out, &errOut, nil); code != 2 {
			t.Fatalf("code %d, want 2", code)
		}
	})
}
