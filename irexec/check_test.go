package irexec

import (
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
	if err := os.WriteFile(filepath.Join(dir, "dbspec", "gauges.dbs"), []byte(dbs), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
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
		err := f.fails(setMode(f, "G1", "manual"), "CONSTRAINT")
		if err.Diagnostic == nil || err.Diagnostic.Rule != "type" || err.Diagnostic.Line != 4 {
			t.Fatalf("error %+v, want the type diagnostic of G1's level", err)
		}
	})
	t.Run("50#checks: a check false over readable values names the check", func(t *testing.T) {
		f := gauges(t)
		err := f.fails(f.msg("insert", "gauges", map[string]any{"set": sets("id", "level", "mode")}, "G3", -1, "auto"), "CONSTRAINT")
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
