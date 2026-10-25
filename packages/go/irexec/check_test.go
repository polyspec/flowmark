package irexec

import (
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"os"
	"path/filepath"
	"testing"
)

// gaugesCatalog writes a catalog whose check reads a column that a row may hold a
// value not of its type in, and returns its corpus root.
func gaugesCatalog(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dbspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	dbs := lines(
		"dbspec 1 gauges",
		"",
		"table gauges {",
		"  id varchar(8)",
		"  level i32 null",
		"  mode varchar(8)",
		"  primary key (id)",
		"  check ck_gauges_level (level >= 0 or mode = 'free')",
		"  settings {",
		"    store document table",
		"  }",
		"}",
	)
	corpustest.WriteTree(t, dir, map[string]string{"dbspec/gauges.dbs": dbs})
	return dir
}

// viewsCatalog writes a catalog with the view table and a table of codec columns,
// one of whose stages hold the host stage hex, and returns its corpus root.
func viewsCatalog(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dbspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	dbs := lines(
		"dbspec 1 flowmark",
		"",
		"table views {",
		"  id varchar(8)",
		"  title varchar(255)",
		"  kind varchar(16)",
		"  entity varchar(63)",
		"  filter text",
		"  sort text",
		"  group_by varchar(63) null",
		"  primary key (id)",
		"  settings {",
		"    store document table",
		"    title title",
		"    codec filter ordered_json",
		"    codec sort ordered_json",
		"  }",
		"}",
		"",
		"table memos {",
		"  id varchar(8)",
		"  title varchar(255)",
		"  body text",
		"  rank i16 null",
		"  doc text null",
		"  hexed varchar(255) null",
		"  primary key (id)",
		"  settings {",
		"    store document table",
		"    title title",
		"    body body",
		"    markdown body",
		"    codec doc ordered_json",
		"    codec hexed ordered_json hex",
		"  }",
		"}",
	)
	corpustest.WriteTree(t, dir, map[string]string{"dbspec/flowmark.dbs": dbs})
	return dir
}

// TestViewAndCodecJudgement pins the commit's judgement of the view and codec
// rules (spec 50 Codec values, Views): the validator's judgement runs over each
// row the write writes, a view over the whole row and a codec over the values the
// write assigns, and a fault is CONSTRAINT at its column naming its rule. A stage
// list with a host stage waits on orm T74, as the validator does.
func TestViewAndCodecJudgement(t *testing.T) {
	const noFilter = `{"where":{"items":[]},"params":[]}`
	clean := func(t *testing.T) *fixture { return newFixture(t, viewsCatalog(t), nil).on(t) }
	// stored holds the view V1 of an entity the catalog does not declare and the
	// view V3 whose filter its codec cannot decode.
	stored := func(t *testing.T) *fixture {
		return newFixture(t, viewsCatalog(t), map[string]string{
			"views.flowmark.md": lines(
				"# views",
				"| id | title | kind | entity | filter | sort | group_by |",
				"| --- | --- | --- | --- | --- | --- | --- |",
				"| V1 | Ghosts | list | ghosts | "+noFilter+" | [] |  |",
				`| V3 | Broken | list | memos | {"where": | [] |  |`,
			),
		}).on(t)
	}
	insertView := func(f *fixture, entity, filter, sort string) []byte {
		return f.msg("insert", "views", map[string]any{"set": sets("id", "title", "kind", "entity", "filter", "sort")}, "V2", "Memos", "list", entity, filter, sort)
	}
	insertMemo := func(f *fixture, col, value string) []byte {
		return f.msg("insert", "memos", map[string]any{"set": sets("id", "title", "body", col)}, "N2", "Two", "Text.", value)
	}
	t.Run("50#views: a valid view is written", func(t *testing.T) {
		f := clean(t)
		f.commit(f.want(insertView(f, "memos", `{"where":{"items":[{"pred":{"column":"rank","op":"gt","p":0}}]},"params":[1]}`, `[{"column":"title"}]`), `{"affected":1,"keys":["V2"]}`))
	})
	t.Run("50#views: a view of an entity the catalog does not declare is refused at entity", func(t *testing.T) {
		f := clean(t)
		err := f.failure(insertView(f, "ghosts", noFilter, `[]`), "CONSTRAINT")
		wantFaults(t, err, []dupFault{{"views", "V2", -1, "entity", "CONSTRAINT", "view: the entity ghosts"}})
	})
	t.Run("50#views: a faulty filter and a faulty sort are two faults", func(t *testing.T) {
		f := clean(t)
		err := f.failure(insertView(f, "memos", `{"where":{"items":[{"pred":{"column":"rank","op":"gt","p":0}}]},"params":[1,2]}`, `[{"column":"color"}]`), "CONSTRAINT")
		wantFaults(t, err, []dupFault{
			{"views", "V2", -1, "filter", "CONSTRAINT", "view: the filter holds param 1"},
			{"views", "V2", -1, "sort", "CONSTRAINT", "view: the sort"},
		})
	})
	t.Run("50#views: a write that leaves a stored view invalid is refused", func(t *testing.T) {
		f := stored(t)
		err := f.failure(f.msg("update", "views", map[string]any{"set": sets("title"), "where": where(pred("id", "eq", p(1)))}, "Spooks", "V1"), "CONSTRAINT")
		wantFaults(t, err, []dupFault{{"views", "V1", -1, "entity", "CONSTRAINT", "view: the entity ghosts"}})
	})
	t.Run("50#views: a filter its codec cannot decode is codec alone", func(t *testing.T) {
		f := clean(t)
		err := f.failure(insertView(f, "memos", `{"where":`, `[]`), "CONSTRAINT")
		wantFaults(t, err, []dupFault{{"views", "V2", -1, "filter", "CONSTRAINT", "codec: filter holds a value its codec"}})
	})
	t.Run("50#codec-values: a value its stages decode is written", func(t *testing.T) {
		f := clean(t)
		f.commit(f.want(insertMemo(f, "doc", `{"a":[1,2]}`), `{"affected":1,"keys":["N2"]}`))
	})
	t.Run("50#codec-values: a value its stages cannot decode is refused at its column", func(t *testing.T) {
		f := clean(t)
		err := f.failure(insertMemo(f, "doc", `{"a":`), "CONSTRAINT")
		wantFaults(t, err, []dupFault{{"memos", "N2", -1, "doc", "CONSTRAINT", "codec: doc holds a value its codec"}})
	})
	t.Run("50#codec-values: a write that does not assign an undecodable value leaves it", func(t *testing.T) {
		f := stored(t)
		f.want(f.msg("update", "views", map[string]any{"set": sets("title"), "where": where(pred("id", "eq", p(1)))}, "Mended", "V3"), `{"affected":1,"keys":[]}`)
	})
	t.Run("50#codec-values: a stage list with the host stage hex waits on orm T74", func(t *testing.T) {
		f := clean(t)
		f.commit(f.want(insertMemo(f, "hexed", "zz"), `{"affected":1,"keys":["N2"]}`))
	})
}

// TestCheckJudgement pins the commit's judgement of a check (spec 50 Checks): the
// validator's evaluator judges it over the written row, a value not of its column
// type evaluated once true and once false. The check passes when it is not false
// whatever that value is, fails naming the check when it is false whatever it is,
// and fails naming the type diagnostic when it needs the value.
func TestCheckJudgement(t *testing.T) {
	gauges := func(t *testing.T) *fixture {
		return newFixture(t, gaugesCatalog(t), map[string]string{"gauges.flowmark.md": lines(
			"# gauges",
			"| id | level | mode |",
			"| --- | --- | --- |",
			"| G1 | high | auto |",
			"| G2 | 3 | auto |",
		)}).on(t)
	}
	setMode := func(f *fixture, id, mode string) []byte {
		return f.msg("update", "gauges", map[string]any{"set": sets("mode"), "where": where(pred("id", "eq", p(1)))}, mode, id)
	}
	t.Run("50#checks: a check true whatever the value not of its type is passes", func(t *testing.T) {
		f := gauges(t)
		f.want(setMode(f, "G1", "free"), `{"affected":1,"keys":[]}`)
	})
	t.Run("50#checks: a check that needs the value not of its type names its type diagnostic", func(t *testing.T) {
		f := gauges(t)
		err := f.failure(setMode(f, "G1", "manual"), "CONSTRAINT")
		if err.Diagnostic == nil || err.Diagnostic.Rule != "type" || err.Diagnostic.Line != 4 {
			t.Fatalf("error %+v, want the type diagnostic of G1's level", err)
		}
	})
	t.Run("50#checks: a check false over readable values names the check", func(t *testing.T) {
		f := gauges(t)
		err := f.failure(f.msg("insert", "gauges", map[string]any{"set": sets("id", "level", "mode")}, "G3", -1, "auto"), "CONSTRAINT")
		wantFaults(t, err, []dupFault{{"gauges", "G3", -1, "", "CONSTRAINT", "ck_gauges_level"}})
	})
	t.Run("50#checks: a check true over readable values passes", func(t *testing.T) {
		f := newFixture(t, gaugesCatalog(t), map[string]string{"gauges.flowmark.md": lines(
			"# gauges",
			"| id | level | mode |",
			"| --- | --- | --- |",
			"| G2 | 3 | auto |",
		)}).on(t)
		f.commit(f.want(setMode(f, "G2", "manual"), `{"affected":1,"keys":[]}`))
	})
}
