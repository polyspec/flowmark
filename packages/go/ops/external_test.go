package ops

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
)

// TestExternalDocument holds the accepting and rejecting cases of an external document of the set (spec 30 External documents).
func TestExternalDocument(t *testing.T) {
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["owned.dbs"], "external": ["shared.dbs"]}`,
		"dbspec/owned.dbs": `dbspec 1 owned

use shared { notes }

table readers {
  id i64 identity
  note i64
  primary key (id)
  index ix_readers_note (note)
  foreign key fk_readers_note (note) references notes (id) on delete restrict on update restrict
}
`,
		"dbspec/shared.dbs": `dbspec 1 shared

table notes {
  id i64 identity
  title varchar(16)
  primary key (id)
}

table unused {
  id i64 identity
  primary key (id)
}
`,
	})
	c := openCorpus(t, root)
	if got, err := run(t, c, "row.insert", `{"table":"notes","values":{"title":"x"}}`); err == nil || !strings.Contains(err.Error(), "owned by another document set") {
		t.Fatalf("a write of an external table: %s, %v", got, err)
	}
	if got, err := run(t, c, "rows.load", `{"table":"notes","rows":[{"id":1,"title":"x"}]}`); err != nil {
		t.Fatalf("a load of an external table: %s, %v", got, err)
	}
	holds(t, c, "notes", `"title":"x"`)
	if got, err := run(t, c, "row.insert", `{"table":"readers","values":{"note":1}}`); err != nil {
		t.Fatalf("a reference to an external row: %s, %v", got, err)
	}
	if got, err := run(t, c, "row.insert", `{"table":"readers","values":{"note":2}}`); err == nil || !strings.Contains(err.Error(), "FOREIGN_KEY") {
		t.Fatalf("a dangling reference to an external table: %s, %v", got, err)
	}
	if got, err := run(t, c, "rows", `{"table":"unused"}`); err == nil {
		t.Fatalf("a table the set does not use: %s", got)
	}
}
