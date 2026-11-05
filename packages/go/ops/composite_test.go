package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/validate"
)

const compositeDBS = `dbspec 1 flowmark

table accounts {
  tenant i64
  account i64
  name varchar(16)
  primary key (tenant, account)
}

table members {
  tenant i64
  account i64
  role varchar(16)
  primary key (tenant, account, role)
  foreign key fk_members_account (tenant, account) references accounts (tenant, account) on delete restrict on update restrict
}

table grants {
  id i64 identity
  tenant i64
  account i64
  primary key (id)
  index ix_grants_account (tenant, account)
  foreign key fk_grants_account (tenant, account) references accounts (tenant, account) on delete cascade on update restrict
}
`

func compositeCorpus(t *testing.T) (*Corpus, string) {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteTree(t, root, map[string]string{"dbspec/flowmark.dbs": compositeDBS})
	return openCorpus(t, root), root
}

func compositeInsert(t *testing.T, c *Corpus, table, values string) {
	t.Helper()
	if got, err := run(t, c, "row.insert", `{"table":"`+table+`","values":`+values+`}`); err != nil {
		t.Fatalf("%s %s: %s, %v", table, values, got, err)
	}
}

// TestCompositeKey holds the accepting and rejecting cases of a composite primary and foreign key (spec 30 Composite keys).
func TestCompositeKey(t *testing.T) {
	t.Run("a row is found by its whole key and stored in one key cell", func(t *testing.T) {
		c, root := compositeCorpus(t)
		compositeInsert(t, c, "accounts", `{"tenant":900,"account":1,"name":"a"}`)
		compositeInsert(t, c, "accounts", `{"tenant":901,"account":1,"name":"c"}`)
		compositeInsert(t, c, "accounts", `{"tenant":900,"account":2,"name":"b"}`)
		holds(t, c, "accounts", `"tenant":900`, `"account":2`, `"name":"c"`)
		got, err := run(t, c, "row.get", `{"table":"accounts","key":"901.1"}`)
		if err != nil || !strings.Contains(got, `"name":"c"`) || strings.Contains(got, `"name":"a"`) {
			t.Fatalf("got %s, %v", got, err)
		}
		b, err := os.ReadFile(filepath.Join(root, "accounts.flowmark.md"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		for _, want := range []string{"| tenant.account | name |", "| 900.1 | a |", "| 900.2 | b |", "| 901.1 | c |"} {
			if !strings.Contains(text, want) {
				t.Fatalf("the file lacks %q:\n%s", want, text)
			}
		}
	})
	t.Run("the same whole key is refused and a shared part is not", func(t *testing.T) {
		c, _ := compositeCorpus(t)
		compositeInsert(t, c, "accounts", `{"tenant":900,"account":1,"name":"a"}`)
		compositeInsert(t, c, "accounts", `{"tenant":901,"account":1,"name":"b"}`)
		got, err := run(t, c, "row.insert", `{"table":"accounts","values":{"tenant":900,"account":1,"name":"x"}}`)
		if err == nil || !strings.Contains(err.Error(), "DUPLICATE_KEY") {
			t.Fatalf("got %s, %v", got, err)
		}
	})
	t.Run("values whose text joins alike are different keys", func(t *testing.T) {
		c, _ := compositeCorpus(t)
		compositeInsert(t, c, "accounts", `{"tenant":990004,"account":123,"name":"a"}`)
		compositeInsert(t, c, "accounts", `{"tenant":9900041,"account":23,"name":"b"}`)
		holds(t, c, "accounts", `"name":"a"`, `"name":"b"`)
	})
	t.Run("a row without a whole key is refused", func(t *testing.T) {
		c, _ := compositeCorpus(t)
		if got, err := run(t, c, "row.insert", `{"table":"accounts","values":{"tenant":900,"name":"x"}}`); err == nil {
			t.Fatalf("got %s", got)
		}
	})
	t.Run("a foreign key names the whole key", func(t *testing.T) {
		c, _ := compositeCorpus(t)
		compositeInsert(t, c, "accounts", `{"tenant":900,"account":1,"name":"a"}`)
		compositeInsert(t, c, "members", `{"tenant":900,"account":1,"role":"owner"}`)
		got, err := run(t, c, "row.insert", `{"table":"members","values":{"tenant":900,"account":2,"role":"owner"}}`)
		if err == nil || !strings.Contains(err.Error(), "FOREIGN_KEY") {
			t.Fatalf("got %s, %v", got, err)
		}
		if got, err := run(t, c, "row.delete", `{"table":"accounts","key":"900.1"}`); err == nil || !strings.Contains(err.Error(), "FOREIGN_KEY") {
			t.Fatalf("got %s, %v", got, err)
		}
	})
	t.Run("a cascade follows a composite foreign key", func(t *testing.T) {
		c, _ := compositeCorpus(t)
		compositeInsert(t, c, "accounts", `{"tenant":900,"account":1,"name":"a"}`)
		compositeInsert(t, c, "accounts", `{"tenant":900,"account":2,"name":"b"}`)
		compositeInsert(t, c, "grants", `{"tenant":900,"account":1}`)
		compositeInsert(t, c, "grants", `{"tenant":900,"account":2}`)
		if got, err := run(t, c, "row.delete", `{"table":"accounts","key":"900.1"}`); err != nil {
			t.Fatalf("got %s, %v", got, err)
		}
		lacks(t, c, "grants", `"account":1`)
		holds(t, c, "grants", `"account":2`)
	})
	t.Run("the validator reports a composite foreign key that names no row", func(t *testing.T) {
		_, root := compositeCorpus(t)
		corpustest.WriteFiles(t, root, map[string]string{
			"members.flowmark.md": "# members\n| tenant.account.role | \n| --- | \n| 900.1.owner | \n",
		})
		cat, loadDiags := catalog.Load(root)
		if cat == nil {
			t.Fatal(loadDiags)
		}
		diags, err := validate.Corpus(cat, root)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range diags {
			found = found || (d.Rule == "reference.dangling" && strings.Contains(d.Message, "900.1"))
		}
		if !found {
			t.Fatalf("diagnostics %+v", diags)
		}
	})
}
