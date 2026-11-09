package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
)

const auditSet = `dbspec 1 audited

table audit {
  seq i64 identity
  actor varchar(32)
  primary key (seq)
}

table item {
  seq i64 identity
  title varchar(32)
  audit_seq i64
  deleted_at datetime(6) null
  primary key (seq)
  index ix_item_audit (audit_seq)
  foreign key fk_item_audit (audit_seq) references audit (seq) on delete restrict on update restrict
  settings {
    soft_delete deleted_at
    audit into item_history column audit_seq references audit action change previous previous_audit_seq
  }
}

table item_history {
  history_id i64 identity
  change varchar(8)
  previous_audit_seq i64 null
  seq i64
  title varchar(32)
  audit_seq i64
  deleted_at datetime(6) null
  primary key (history_id)
  index ix_item_history_row (seq)
}
`

// TestAuditRows holds the rows a write of an audited table writes into its history (spec 50 Audit), each with its own key,
// which raises the sequence of the history table as SQLite's AUTOINCREMENT does.
func TestAuditRows(t *testing.T) {
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{"dbspec/audited.dbs": auditSet})
	c := openCorpus(t, root)
	if got, err := run(t, c, "row.insert", `{"table":"audit","values":{"actor":"a"}}`); err != nil {
		t.Fatalf("the audit record: %s, %v", got, err)
	}
	if got, err := run(t, c, "row.insert", `{"table":"item","values":{"title":"x"}}`); err == nil || !strings.Contains(err.Error(), "needs an audit") {
		t.Fatalf("a write without an audit record: %s, %v", got, err)
	}
	s, err := c.Session("audit-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	send := func(kind string, body map[string]any, params ...json.RawMessage) {
		t.Helper()
		m := message{hash: c.Catalog().ManifestHash, table: "item", params: params}
		b := m.request(kind, body)
		var wire map[string]any
		if err := json.Unmarshal(b, &wire); err != nil {
			t.Fatal(err)
		}
		wire["audit"] = 1
		b, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Exec(context.Background(), b); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	insert := map[string]any{"set": []map[string]any{{"column": "title", "p": 0}}}
	send("insert", insert, json.RawMessage(`"first"`))
	send("insert", insert, json.RawMessage(`"second"`))
	data, err := os.ReadFile(filepath.Join(root, "item_history.flowmark.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "first") != 1 || strings.Count(string(data), "second") != 1 {
		t.Fatalf("the history holds:\n%s", data)
	}
	got, err := run(t, c, "rows", `{"table":"item_history"}`)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data), string(got))
	for _, want := range []string{`"history_id":1`, `"history_id":2`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("no %s in %s", want, got)
		}
	}
	// Each history row takes the next key of its AUTOINCREMENT table and raises its sequence to it, as an audit trigger's
	// insert does in SQLite, so every sequence is the largest key of its table and the corpus names none.
	if data, err := os.ReadFile(filepath.Join(root, "dbspec", "sequences.json")); !os.IsNotExist(err) {
		t.Fatalf("the corpus names the sequences %q, %v", data, err)
	}
}
