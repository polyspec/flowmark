package irexec

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repairNow is the engine clock of the repair cases.
var repairNow = time.Date(2026, 10, 11, 10, 0, 0, 123456000, time.UTC)

// repair runs the history repair of table and key against the fixture's corpus.
func (f *fixture) repair(table, key, from string) (*Result, *Error) {
	res, err := f.e.Repair(f.c, table, key, from, repairNow)
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			f.t.Fatalf("an error that is not an *Error: %v", err)
		}
		return nil, e
	}
	return res, nil
}

// itemsWith returns the base corpus with the items document replaced by text.
func itemsWith(text string) map[string]string {
	docs := baseDocs()
	docs["items.flowmark.md"] = text
	return docs
}

// jobsCatalog writes a catalog whose state machine has two initial states, and
// returns its corpus root.
func jobsCatalog(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dbspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	dbs := lines(
		"dbspec 1 jobs",
		"",
		"table jobs {",
		"  id varchar(16)",
		"  state varchar(16)",
		"  title varchar(64)",
		"  primary key (id)",
		"  settings {",
		"    store document table",
		"    title title",
		"    state_machine state initial queued",
		"    state_machine state initial held",
		"    state_machine state queued -> running",
		"    state_machine state held -> running",
		"    state_machine state running -> done",
		"    state_machine state terminal done",
		"    state_machine state history job_history row job from from_state to to_state at at",
		"  }",
		"}",
		"",
		"table job_history {",
		"  id i64 identity",
		"  job varchar(16)",
		"  from_state varchar(16)",
		"  to_state varchar(16)",
		"  at datetime(6)",
		"  primary key (id)",
		"  index ix_job_history_job (job)",
		"  foreign key fk_job_history (job) references jobs (id) on delete restrict on update restrict",
		"  settings {",
		"    store document table",
		"  }",
		"}",
	)
	if err := os.WriteFile(filepath.Join(dir, "dbspec", "jobs.dbs"), []byte(dbs), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestRepair pins the history repair (spec 50 A state changed outside the engine,
// Repair): one history row from the last to — for a row without history the one
// initial state, or --from — to the current state, at the engine clock, with the
// required columns copied from the row.
func TestRepair(t *testing.T) {
	t.Run("50#a-state-changed-outside-the-engine: a row without history goes from the one initial state, in its own block", func(t *testing.T) {
		f := catalogFixture(t, itemsWith(lines(
			"# items",
			"- [x] T1 Ship the parser owner:[P1](people.flowmark.md)",
			"  - [~] T2 Write the tests due:2026-10-20",
			"- [~] T3 Review the design",
			"- [-] T4 Drop the old path",
			"",
			"## items row T1",
			"",
			"Parser body.",
			"",
			"### evidence",
			"",
			"Golden tests pass.",
			"",
			"### item_history",
			"| id | from_state | to_state | at |",
			"| --- | --- | --- | --- |",
			"| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |",
			"| 2 | doing | done | 2026-10-01 09:00:00.000000 |",
			"",
			"#### item_history row 2",
			"",
			"##### evidence",
			"",
			"Golden tests pass.",
			"",
			"## items row T2",
			"",
			"### item_history",
			"| id | from_state | to_state | at |",
			"| --- | --- | --- | --- |",
			"| 3 | waiting | doing | 2026-10-02 08:00:00.000000 |",
			"",
			"## items row T4",
			"",
			"### cause",
			"",
			"Superseded.",
			"",
			"### item_history",
			"| id | from_state | to_state | at |",
			"| --- | --- | --- | --- |",
			"| 4 | waiting | dropped | 2026-10-03 08:00:00.000000 |",
			"",
			"#### item_history row 4",
			"",
			"##### cause",
			"",
			"Superseded.",
		)))
		res, err := f.repair("items", "T3", "")
		if err != nil {
			t.Fatal(err)
		}
		if got := answer(t, res); got != `{"affected":1,"keys":[5]}` {
			t.Fatalf("answer %s", got)
		}
		if len(res.Post.Targets) != 1 || res.Post.Targets[0].Path != "items.flowmark.md" {
			t.Fatalf("targets %+v", res.Post.Targets)
		}
		f.commit(res)
		if !strings.Contains(f.file("items.flowmark.md"), "## items row T3\n\n### item_history\n| id | from_state | to_state | at |\n| --- | --- | --- | --- |\n| 5 | waiting | doing | 2026-10-11 10:00:00.123456 |\n") {
			t.Fatalf("items:\n%s", f.file("items.flowmark.md"))
		}
	})
	t.Run("50#a-state-changed-outside-the-engine: a row with history goes from the to of its last history row, copying the required columns", func(t *testing.T) {
		docs := baseDocs()
		docs["items.flowmark.md"] = strings.Replace(strings.Replace(docs["items.flowmark.md"],
			"  - [~] T2 Write the tests due:2026-10-20", "  - [x] T2 Write the tests due:2026-10-20", 1),
			"## items row T2\n\n", "## items row T2\n\n### evidence\n\nReviewed.\n\n", 1)
		f := catalogFixture(t, docs)
		res, err := f.repair("items", "T2", "")
		if err != nil {
			t.Fatal(err)
		}
		f.commit(res)
		if !strings.Contains(f.file("items.flowmark.md"), "| 5 | doing | done | 2026-10-11 10:00:00.123456 |\n\n#### item_history row 5\n\n##### evidence\n\nReviewed.\n") {
			t.Fatalf("items:\n%s", f.file("items.flowmark.md"))
		}
	})
	t.Run("50#a-state-changed-outside-the-engine: a history document that is absent is created", func(t *testing.T) {
		f := catalogFixture(t, map[string]string{"tickets.flowmark.md": lines(
			"# tickets",
			"| id | state | title |",
			"| --- | --- | --- |",
			"| K1 | closed | Done by hand |",
		)})
		res, err := f.repair("tickets", "K1", "")
		if err != nil {
			t.Fatal(err)
		}
		if tg := res.Post.Targets; len(tg) != 1 || tg[0].Path != "ticket_history.flowmark.md" || tg[0].Before != nil {
			t.Fatalf("targets %+v", tg)
		}
		f.commit(res)
		if got := f.file("ticket_history.flowmark.md"); !strings.Contains(got, "| 1 | [K1](tickets.flowmark.md) | open | closed | 2026-10-11 10:00:00.123456 |\n") {
			t.Fatalf("ticket_history:\n%s", got)
		}
	})
	t.Run("50#a-state-changed-outside-the-engine: --from names one of several initial states", func(t *testing.T) {
		for _, from := range []string{"queued", "held"} {
			f := newFixture(t, jobsCatalog(t), map[string]string{"jobs.flowmark.md": lines(
				"# jobs",
				"| id | state | title |",
				"| --- | --- | --- |",
				"| J1 | running | First |",
			)})
			res, err := f.repair("jobs", "J1", from)
			if err != nil {
				t.Fatal(err)
			}
			f.commit(res)
			if got := f.file("job_history.flowmark.md"); !strings.Contains(got, "| 1 | [J1](jobs.flowmark.md) | "+from+" | running | 2026-10-11 10:00:00.123456 |\n") {
				t.Fatalf("job_history:\n%s", got)
			}
		}
	})
}

// TestRepairRejects pins the refusals of the history repair.
func TestRepairRejects(t *testing.T) {
	replace := func(old, new string) map[string]string {
		docs := baseDocs()
		docs["items.flowmark.md"] = strings.Replace(docs["items.flowmark.md"], old, new, 1)
		return docs
	}
	jobs := func(t *testing.T) *fixture {
		return newFixture(t, jobsCatalog(t), map[string]string{"jobs.flowmark.md": lines(
			"# jobs",
			"| id | state | title |",
			"| --- | --- | --- |",
			"| J1 | running | First |",
		)})
	}
	cases := []struct {
		name              string
		f                 func(t *testing.T) *fixture
		table, key, from  string
		code, column, has string
	}{
		{"50#a-state-changed-outside-the-engine: a pair that is not a declared transition", func(t *testing.T) *fixture {
			return catalogFixture(t, replace("- [ ] T3 Review the design", "- [x] T3 Review the design"))
		}, "items", "T3", "", "CONSTRAINT", "state", "waiting to done"},
		{"50#a-state-changed-outside-the-engine: a required column empty on the row", func(t *testing.T) *fixture {
			return catalogFixture(t, replace("  - [~] T2 Write the tests", "  - [x] T2 Write the tests"))
		}, "items", "T2", "", "CONSTRAINT", "evidence", "evidence"},
		{"50#a-state-changed-outside-the-engine: the current state's limit exceeded counting this row", func(t *testing.T) *fixture {
			return catalogFixture(t, replace("- [ ] T3 Review the design", "- [~] T3 Review the design\n- [~] T5 Another"))
		}, "items", "T3", "", "CONSTRAINT", "state", "limit 2"},
		{"50#a-state-changed-outside-the-engine: a row without a gap has nothing to repair", func(t *testing.T) *fixture { return baseFixture(t) }, "items", "T1", "", "CONSTRAINT", "state", "no history gap"},
		{"50#a-state-changed-outside-the-engine: a row in an initial state without history has no gap", func(t *testing.T) *fixture { return baseFixture(t) }, "items", "T3", "", "CONSTRAINT", "state", "no history gap"},
		{"50#a-state-changed-outside-the-engine: --from for a row with history rows", func(t *testing.T) *fixture {
			return catalogFixture(t, replace("  - [~] T2 Write the tests", "  - [x] T2 Write the tests"))
		}, "items", "T2", "waiting", "CONFIG", "", "last history row"},
		{"50#a-state-changed-outside-the-engine: --from that is not an initial state", jobs, "jobs", "J1", "running", "CONFIG", "", "not an initial state"},
		{"50#a-state-changed-outside-the-engine: no --from where the machine has several initial states", jobs, "jobs", "J1", "", "CONFIG", "", "--from"},
		{"50#a-state-changed-outside-the-engine: a key that names no row", func(t *testing.T) *fixture { return baseFixture(t) }, "items", "T9", "", "NO_ROWS", "", "T9"},
		{"50#a-state-changed-outside-the-engine: a table without a history", func(t *testing.T) *fixture { return baseFixture(t) }, "people", "P1", "", "CONFIG", "", "history"},
		{"10#diagnostics: an unavailable state table", func(t *testing.T) *fixture {
			return catalogFixture(t, map[string]string{"tickets.flowmark.md": "no heading\n"})
		}, "tickets", "K1", "", "CONSTRAINT", "", "unavailable"},
		{"10#diagnostics: an unavailable history table", func(t *testing.T) *fixture {
			return catalogFixture(t, map[string]string{
				"tickets.flowmark.md":        lines("# tickets", "| id | state | title |", "| --- | --- | --- |", "| K1 | closed | Done by hand |"),
				"ticket_history.flowmark.md": "no heading\n",
			})
		}, "tickets", "K1", "", "CONSTRAINT", "", "unavailable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.f(t)
			res, err := f.repair(c.table, c.key, c.from)
			if err == nil {
				t.Fatalf("repair answered %s; want %s", answer(t, res), c.code)
			}
			if err.Code != c.code || !strings.Contains(err.Msg, c.has) {
				t.Fatalf("repair failed with %v; want %s naming %q", err, c.code, c.has)
			}
			if c.column != "" && c.code == "CONSTRAINT" && err.Diagnostic == nil {
				if len(err.Faults) != 1 || err.Faults[0].Table != c.table || err.Faults[0].Key != c.key || err.Faults[0].Column != c.column {
					t.Fatalf("faults %+v; want one at %s %s column %s", err.Faults, c.table, c.key, c.column)
				}
			}
		})
	}
}
