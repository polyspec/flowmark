package ops

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
)

const actionsDBS = `dbspec 1 flowmark

table owners {
  id i64 identity
  name varchar(16)
  primary key (id)
}

table pets {
  id i64 identity
  owner i64
  name varchar(16)
  primary key (id)
  index ix_pets_owner (owner)
  foreign key fk_pets_owner (owner) references owners (id) on delete cascade on update restrict
}

table toys {
  id i64 identity
  pet i64
  primary key (id)
  index ix_toys_pet (pet)
  foreign key fk_toys_pet (pet) references pets (id) on delete cascade on update restrict
}

table notes {
  id i64 identity
  owner i64 null
  primary key (id)
  index ix_notes_owner (owner)
  foreign key fk_notes_owner (owner) references owners (id) on delete set_null on update restrict
}

table bills {
  id i64 identity
  owner i64
  primary key (id)
  index ix_bills_owner (owner)
  foreign key fk_bills_owner (owner) references owners (id) on delete restrict on update restrict
}

table ledger {
  id i64 identity
  amount i32
  primary key (id)
  settings {
    immutable
  }
}
`

func actionsCorpus(t *testing.T) *Corpus {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{"dbspec/flowmark.dbs": actionsDBS})
	c := openCorpus(t, root)
	for _, step := range [][2]string{
		{"owners", `{"name":"a"}`}, {"owners", `{"name":"b"}`},
		{"pets", `{"owner":1,"name":"p"}`}, {"pets", `{"owner":2,"name":"q"}`},
		{"toys", `{"pet":1}`}, {"toys", `{"pet":2}`},
		{"notes", `{"owner":1}`}, {"notes", `{"owner":2}`},
	} {
		if _, err := run(t, c, "row.insert", `{"table":"`+step[0]+`","values":`+step[1]+`}`); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func holds(t *testing.T, c *Corpus, table string, want ...string) {
	t.Helper()
	got, err := run(t, c, "rows", `{"table":"`+table+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Fatalf("%s holds %s, want %s", table, got, w)
		}
	}
}

func lacks(t *testing.T, c *Corpus, table string, not ...string) {
	t.Helper()
	got, err := run(t, c, "rows", `{"table":"`+table+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range not {
		if strings.Contains(got, n) {
			t.Fatalf("%s holds %s, which has %s", table, got, n)
		}
	}
}

// TestReferentialActions holds the accepting and rejecting cases of the delete actions (spec 50 Referential actions).
func TestReferentialActions(t *testing.T) {
	t.Run("cascade takes the rows at every level and set_null clears", func(t *testing.T) {
		c := actionsCorpus(t)
		if got, err := run(t, c, "row.delete", `{"table":"owners","key":1}`); err != nil || !strings.Contains(got, `"affected":1`) {
			t.Fatalf("got %s, %v", got, err)
		}
		lacks(t, c, "owners", `"name":"a"`)
		lacks(t, c, "pets", `"name":"p"`)
		holds(t, c, "pets", `"name":"q"`)
		lacks(t, c, "toys", `"pet":1`)
		holds(t, c, "toys", `"pet":2`)
		holds(t, c, "notes", `"id":1,"owner":null`, `"id":2,"owner":2`)
	})
	t.Run("restrict refuses and leaves every row", func(t *testing.T) {
		c := actionsCorpus(t)
		if _, err := run(t, c, "row.insert", `{"table":"bills","values":{"owner":1}}`); err != nil {
			t.Fatal(err)
		}
		if got, err := run(t, c, "row.delete", `{"table":"owners","key":1}`); err == nil || !strings.Contains(err.Error(), "FOREIGN_KEY") {
			t.Fatalf("got %s, %v", got, err)
		}
		holds(t, c, "owners", `"name":"a"`)
		holds(t, c, "pets", `"name":"p"`)
		holds(t, c, "notes", `"id":1,"owner":1`)
	})
	t.Run("restrict below a cascade refuses the whole delete", func(t *testing.T) {
		c := actionsCorpus(t)
		if _, err := run(t, c, "row.insert", `{"table":"bills","values":{"owner":2}}`); err != nil {
			t.Fatal(err)
		}
		if got, err := run(t, c, "row.delete", `{"table":"owners","key":2}`); err == nil {
			t.Fatalf("got %s", got)
		}
		holds(t, c, "toys", `"pet":2`)
	})
}

// TestImmutable holds the accepting and rejecting cases of an immutable table (spec 50 Immutable).
func TestImmutable(t *testing.T) {
	c := actionsCorpus(t)
	if got, err := run(t, c, "row.insert", `{"table":"ledger","values":{"amount":1}}`); err != nil {
		t.Fatalf("an insert is accepted: %s, %v", got, err)
	}
	for _, f := range []struct{ op, members string }{
		{"row.update", `{"table":"ledger","key":1,"values":{"amount":2}}`},
		{"row.delete", `{"table":"ledger","key":1}`},
	} {
		got, err := run(t, c, f.op, f.members)
		if err == nil || !strings.Contains(err.Error(), "CONSTRAINT") || !strings.Contains(err.Error(), "table ledger is immutable") {
			t.Fatalf("%s: got %s, %v", f.op, got, err)
		}
	}
	holds(t, c, "ledger", `"amount":1`)
}

// TestDefaultStorage holds that a table with no store setting is a table in one document of its own (spec 30).
func TestDefaultStorage(t *testing.T) {
	c := actionsCorpus(t)
	holds(t, c, "owners", `"name":"a"`)
	lacks(t, c, "owners", `"name":"z"`)
}
