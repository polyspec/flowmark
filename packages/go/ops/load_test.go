package ops

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
)

const loadDBS = `dbspec 1 flowmark

table tags {
  id i64 identity
  label varchar(16)
  note text null
  primary key (id)
  settings {
    store document table
  }
}
`

func loadCorpus(t *testing.T) *Corpus {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{"dbspec/flowmark.dbs": loadDBS})
	return openCorpus(t, root)
}

// TestRowsLoad holds the accepting and rejecting cases of rows.load (spec 40 The faces, orm PRT-26).
func TestRowsLoad(t *testing.T) {
	t.Run("rows keep their keys and the identity continues after the largest", func(t *testing.T) {
		c := loadCorpus(t)
		got, err := run(t, c, "rows.load", `{"table":"tags","rows":[{"id":7,"label":"a"},{"id":43,"label":"b"},{"id":9,"label":"c","note":"n"}]}`)
		if err != nil || got != `{"affected":3}` {
			t.Fatalf("got %s, %v", got, err)
		}
		rows, err := run(t, c, "rows", `{"table":"tags"}`)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"id":7`, `"id":43`, `"id":9`, `"note":"n"`} {
			if !strings.Contains(rows, want) {
				t.Fatalf("rows %s lack %s", rows, want)
			}
		}
		if got, err := run(t, c, "row.insert", `{"table":"tags","values":{"label":"d"}}`); err != nil || !strings.Contains(got, `"keys":[44]`) {
			t.Fatalf("got %s, %v", got, err)
		}
	})
	t.Run("a row without the identity column takes the next key", func(t *testing.T) {
		c := loadCorpus(t)
		if got, err := run(t, c, "rows.load", `{"table":"tags","rows":[{"id":7,"label":"a"},{"label":"b"}]}`); err != nil || got != `{"affected":2}` {
			t.Fatalf("got %s, %v", got, err)
		}
		if got, err := run(t, c, "row.get", `{"table":"tags","key":8}`); err != nil || !strings.Contains(got, `"label":"b"`) {
			t.Fatalf("got %s, %v", got, err)
		}
	})
	t.Run("an empty list writes nothing", func(t *testing.T) {
		if got, err := run(t, loadCorpus(t), "rows.load", `{"table":"tags","rows":[]}`); err != nil || got != `{"affected":0}` {
			t.Fatalf("got %s, %v", got, err)
		}
	})
	for _, f := range []struct{ name, members, code, text string }{
		{"a row that assigns no column", `{"table":"tags","rows":[{}]}`, codeConfig, "assigns no column"},
		{"a member that is no array", `{"table":"tags","rows":{}}`, codeConfig, "not an array"},
		{"a key that exists", `{"table":"tags","rows":[{"id":1,"label":"a"},{"id":1,"label":"b"}]}`, "DUPLICATE_KEY", "1"},
		{"a value not of its type", `{"table":"tags","rows":[{"id":"x","label":"a"}]}`, "CONSTRAINT", "not a value of id"},
	} {
		t.Run(f.name, func(t *testing.T) { fails(t, loadCorpus(t), "rows.load", f.members, f.code, f.text) })
	}
}
