package irexec

import (
	"github.com/polyspec/flowmark/internal/corpustest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/catalog"
)

// relationsDBS returns the catalog of the relations golden: the corpus of complex foreign keys, whose every foreign
// key is one column that references a one-column primary key (spec 30 Foreign keys) and whose shapes are a chain five
// tables deep, a self reference, a two-directional pair, a table five others reference, a link into a block row and
// every storage kind.
func relationsDBS(t testing.TB) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(moduleRoot(t), "store", "testdata", "relations", "dbspec", "flowmark.dbs"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestRelationsCatalogRules pins what the catalog of complex foreign keys refuses: a composite primary key and a
// composite foreign key are schema errors, for a foreign key is a link to a row and a row has a link only through its
// one-column key.
func TestRelationsCatalogRules(t *testing.T) {
	base := relationsDBS(t)
	compositePK := strings.Replace(base,
		"  id varchar(8)\n  name varchar(40)\n  primary key (id)\n  settings {\n    key_prefix 'T'",
		"  id varchar(8)\n  name varchar(40)\n  seat varchar(4)\n  primary key (id, seat)\n  settings {\n    key_prefix 'T'", 1)
	compositeFK := strings.Replace(base,
		"  foreign key fk_teams_tenant (tenant) references tenants (id) on delete restrict on update restrict",
		"  foreign key fk_teams_pair (tenant, id) references tenants (id) on delete restrict on update restrict", 1)
	for _, c := range []struct {
		name string
		dbs  string
		want string
	}{
		{"30#corpus-rules-on-dbspec-constructs: a composite primary key", compositePK, "single-column primary key"},
		{"30#corpus-rules-on-dbspec-constructs: a composite foreign key", compositeFK, "2 columns and 1 referenced"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.dbs == base {
				t.Fatal("the edit found no anchor in the golden")
			}
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "dbspec"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "dbspec", "flowmark.dbs"), []byte(c.dbs), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := corpustest.WriteSetFile(filepath.Join(root, "dbspec")); err != nil {
				t.Fatal(err)
			}
			_, diags := catalog.Load(root)
			found := ""
			for _, d := range diags {
				if strings.Contains(d.Rule+":"+d.Message, c.want) {
					found = d.Message
				}
			}
			if found == "" {
				t.Fatalf("diagnostics %+v; want one naming %q", diags, c.want)
			}
		})
	}
}

// TestRelationsChain pins the chain five tables deep: every table of the chain inserts through the executor, in the
// storage kind its table declares, a join reads across the chain and the relation of a relation assembles.
func TestRelationsChain(t *testing.T) {
	f := goldenFixture(t, "relations").on(t)
	insert := func(table string, cols []string, params ...any) {
		t.Helper()
		f.commit(f.ok(f.msg("insert", table, map[string]any{"set": sets(cols...)}, params...)))
	}
	insert("tenants", []string{"id", "name"}, "T1", "폴리스펙")
	insert("teams", []string{"id", "tenant", "name"}, "G1", "T1", "엔진팀")
	insert("members", []string{"id", "team", "name"}, "M1", "G1", "맥스")
	insert("posts", []string{"id", "author", "team", "title"}, "P1", "M1", "G1", "첫 글")
	insert("comments", []string{"post", "author", "body"}, "P1", "M1", "첫 댓글")

	f.sub(t, "60#surfaces: a join reads across the chain", func(f *fixture) {
		posts := map[string]any{"rel": "post", "kind": "inner", "left": "post", "right": "id",
			"query": map[string]any{"entity": "posts", "columns": map[string]any{"mode": "none", "add": []string{"title"}}}}
		f.want(f.msg("all", "comments", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"body"}}, "joins": []any{posts}}),
			`{"rows":[{"id":1,"post":{"id":"P1","author":"M1","category":null,"team":"G1","title":"첫 글"},"author":"M1","body":"첫 댓글"}]}`)
	})

	f.sub(t, "60#surfaces: a relation of a relation assembles", func(f *fixture) {
		f.commit(f.ok(f.msg("insert", "reactions", map[string]any{"set": sets("id", "comment", "kind")}, "R1", 1, "like")))
		reactions := map[string]any{"rel": "reactions", "kind": "many", "keys": []any{map[string]any{"left": "id", "right": "comment"}},
			"query": map[string]any{"entity": "reactions", "columns": map[string]any{"mode": "none", "add": []string{"kind"}}}}
		comments := map[string]any{"rel": "comments", "kind": "many", "keys": []any{map[string]any{"left": "id", "right": "post"}},
			"query": map[string]any{"entity": "comments", "columns": map[string]any{"mode": "none", "add": []string{"body"}},
				"relations": []any{reactions}}}
		f.want(f.msg("all", "posts", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"title"}},
			"relations": []any{comments}}),
			`{"rows":[{"id":"P1","author":"M1","category":null,"team":"G1","title":"첫 글","comments":[{"id":1,"post":"P1","author":"M1","body":"첫 댓글","reactions":[{"id":"R1","comment":1,"kind":"like"}]}]}]}`)
	})
}

// TestRelationsConstraints pins the writes and the refusals of the complex foreign keys: a move between parents keeps
// the reference, a dangling reference is refused, a delete a live reference holds is refused — a self reference, a
// chain edge and a two-directional pair each — and a pair that references each other goes in one sequence of writes.
func TestRelationsConstraints(t *testing.T) {
	seeded := func(t *testing.T) *fixture {
		t.Helper()
		f := goldenFixture(t, "relations").on(t)
		for _, c := range []struct {
			table string
			cols  []string
			vals  []any
		}{
			{"tenants", []string{"id", "name"}, []any{"T1", "폴리스펙"}},
			{"teams", []string{"id", "tenant", "name"}, []any{"G1", "T1", "엔진팀"}},
			{"members", []string{"id", "team", "name"}, []any{"M1", "G1", "맥스"}},
			{"members", []string{"id", "team", "name"}, []any{"M2", "G1", "제인"}},
			{"categories", []string{"id", "name"}, []any{"C1", "공지"}},
			{"categories", []string{"id", "parent", "name"}, []any{"C2", "C1", "출시"}},
			{"posts", []string{"id", "author", "category", "team", "title"}, []any{"P1", "M1", "C2", "G1", "첫 글"}},
			{"comments", []string{"post", "author", "body"}, []any{"P1", "M2", "첫 댓글"}},
		} {
			f.commit(f.ok(f.msg("insert", c.table, map[string]any{"set": sets(c.cols...)}, c.vals...)))
		}
		return f
	}
	t.Run("60#surfaces: a move between parents keeps the reference", func(t *testing.T) {
		f := seeded(t)
		// The post moves to author M2 and to the parent category C1.
		f.commit(f.ok(f.msg("update", "posts", map[string]any{
			"set":   []any{set("author", 0), set("category", 1)},
			"where": where(pred("id", "eq", p(2))),
		}, "M2", "C1", "P1")))
		f.want(f.msg("all", "posts", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"category"}}}),
			`{"rows":[{"id":"P1","author":"M2","category":"C1","team":"G1"}]}`)
	})
	t.Run("10#sections-and-their-values: a dangling reference is refused", func(t *testing.T) {
		f := seeded(t)
		err := f.fails(f.msg("insert", "posts", map[string]any{"set": sets("id", "author", "team", "title")}, "P9", "M9", "G1", "빈 글"), "FOREIGN_KEY")
		wantFaults(t, err, []dupFault{{"posts", "P9", -1, "author", "FOREIGN_KEY", "no members row"}})
	})
	t.Run("50#keys-and-references: a delete a live self reference holds is refused", func(t *testing.T) {
		f := seeded(t)
		err := f.fails(f.msg("delete", "categories", map[string]any{"where": where(pred("id", "eq", p(0)))}, "C1"), "FOREIGN_KEY")
		wantFaults(t, err, []dupFault{{"categories", "C1", -1, "", "FOREIGN_KEY", "referenced by categories C2"}})
	})
	t.Run("50#keys-and-references: a delete a live chain edge holds is refused", func(t *testing.T) {
		f := seeded(t)
		err := f.fails(f.msg("delete", "members", map[string]any{"where": where(pred("id", "eq", p(0)))}, "M1"), "FOREIGN_KEY")
		wantFaults(t, err, []dupFault{{"members", "M1", -1, "", "FOREIGN_KEY", "posts P1"}})
	})
	t.Run("60#surfaces: a two-directional pair goes together", func(t *testing.T) {
		f := seeded(t)
		// The pair: M2 pins P1 and P1's author is M1 — P1 points at a member and a member points back at P1.
		f.commit(f.ok(f.msg("update", "members", map[string]any{
			"set":   []any{set("pinned_post", 0)},
			"where": where(pred("id", "eq", p(1))),
		}, "P1", "M2")))
		// Deleting the post alone is refused — the member holds its pin — and deleting the member alone is
		// refused too — the post holds its author; the pin clears first, then the three rows go in one order.
		f.fails(f.msg("delete", "posts", map[string]any{"where": where(pred("id", "eq", p(0)))}, "P1"), "FOREIGN_KEY")
		f.fails(f.msg("delete", "members", map[string]any{"where": where(pred("id", "eq", p(0)))}, "M2"), "FOREIGN_KEY")
		for _, m := range [][]byte{
			f.msg("update", "members", map[string]any{"set": []any{map[string]any{"column": "pinned_post", "null": true}}, "where": where(pred("id", "eq", p(0)))}, "M2"),
			f.msg("delete", "comments", map[string]any{"where": where(pred("post", "eq", p(0)))}, "P1"),
			f.msg("delete", "posts", map[string]any{"where": where(pred("id", "eq", p(0)))}, "P1"),
			f.msg("delete", "members", map[string]any{"where": where(pred("id", "eq", p(0)))}, "M2"),
		} {
			f.commit(f.ok(m))
		}
		f.want(f.msg("count", "posts", nil), `{"count":0}`)
	})
}
