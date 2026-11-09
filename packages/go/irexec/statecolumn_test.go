package irexec

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
)

// stateCatalog declares a state machine on two tables in the grammar of orm 0.0.26, which the engine reads and applies
// none of (spec 50 State columns): tasks, a list whose checkbox lines give the glyph of each value, with a terminal, a
// required column, a history and a limit; tickets, a table whose state column has a foreign key to a table of states;
// and cards, a table whose state column a check constrains.
const stateCatalog = `dbspec 1 flowmark

table states {
  id varchar(16)
  primary key (id)
  settings {
    store document table
  }
}

table tasks {
  id varchar(16)
  state varchar(16) default 'open'
  title varchar(64)
  note text null
  primary key (id)
  settings {
    store document list
    title title
    markdown note
    checkbox state open ' '
    checkbox state doing '~'
    checkbox state closed 'x'
    state_machine state initial open
    state_machine state open -> doing
    state_machine state doing -> closed require (note)
    state_machine state terminal closed
    state_machine state history task_history row task from from_state to to_state at at
    state_machine state limit doing 1
  }
}

table task_history {
  id i64 identity
  task varchar(16)
  from_state varchar(16)
  to_state varchar(16)
  at datetime(6)
  note text null
  primary key (id)
  index ix_task_history_task (task)
  foreign key fk_task_history_task (task) references tasks (id) on delete restrict on update restrict
  settings {
    store document table
    markdown note
  }
}

table tickets {
  id varchar(16)
  state varchar(16)
  title varchar(64)
  primary key (id)
  index ix_tickets_state (state)
  foreign key fk_tickets_state (state) references states (id) on delete restrict on update restrict
  settings {
    store document table
    title title
    state_machine state initial open
    state_machine state open -> closed
    state_machine state terminal closed
    state_machine state history ticket_history row ticket from from_state to to_state at at
  }
}

table cards {
  id varchar(16)
  state varchar(16)
  title varchar(64)
  primary key (id)
  check ck_cards_state (state in ('open', 'closed'))
  settings {
    store document table
    title title
  }
}
` + "\n" + corpustest.TicketHistoryTable

// stateDocs is the corpus of stateCatalog: T1 open, T2 doing with one history row, T3 closed, a ticket open with no
// history row, a card open, and the states open, closed and parked.
func stateDocs() map[string]string {
	return map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["flowmark.dbs"]}` + "\n",
		"dbspec/flowmark.dbs":    stateCatalog,
		"states.flowmark.md": corpustest.Lines(
			"# states",
			"| id |",
			"| --- |",
			"| closed |",
			"| open |",
			"| parked |",
		),
		"tasks.flowmark.md": corpustest.Lines(
			"# tasks",
			"- [ ] T1 First",
			"- [~] T2 Second",
			"- [x] T3 Third",
		),
		"task_history.flowmark.md": corpustest.Lines(
			"# task_history",
			"| id | task | from_state | to_state | at |",
			"| --- | --- | --- | --- | --- |",
			"| 1 | [T2](tasks.flowmark.md) | open | doing | 2026-10-01 08:00:00.000000 |",
		),
		"tickets.flowmark.md": corpustest.Lines(
			"# tickets",
			"| id | state | title |",
			"| --- | --- | --- |",
			"| K1 | [open](states.flowmark.md) | Ticket |",
		),
		"cards.flowmark.md": corpustest.Lines(
			"# cards",
			"| id | state | title |",
			"| --- | --- | --- |",
			"| C1 | open | Card |",
		),
	}
}

// stateFixture is a corpus of stateCatalog.
func stateFixture(t *testing.T) *fixture {
	t.Helper()
	src := t.TempDir()
	corpustest.WriteFiles(t, src, stateDocs())
	return newFixture(t, src, nil)
}

// TestStateColumn pins a state column (spec 50 State columns): the engine applies none of a state machine, so a write
// that a machine would refuse is accepted and writes no history row, while the values of the column are those its type
// and the schema's constraints allow, as on every database, and a list shows a value only through its checkbox line.
func TestStateColumn(t *testing.T) {
	byID := func(f *fixture, entity string, cols []any, id any, params ...any) []byte {
		params = append(params, id)
		return f.msg("update", entity, map[string]any{"set": cols, "where": where(pred("id", "eq", p(len(params)-1)))}, params...)
	}
	accepts := []struct {
		name string
		msg  func(f *fixture) []byte
		file string // the file of the written row
		want string // a line the committed file holds
	}{
		{"50#state-columns: an insert in a state that is not initial",
			func(f *fixture) []byte {
				return f.msg("insert", "tasks", map[string]any{"set": sets("id", "title", "state")}, "T4", "Fourth", "doing")
			},
			"tasks.flowmark.md", "- [~] T4 Fourth\n"},
		{"50#state-columns: an update that no transition declares",
			func(f *fixture) []byte { return byID(f, "tasks", sets("state"), "T1", "closed") },
			"tasks.flowmark.md", "- [x] T1 First\n"},
		{"50#state-columns: an update that leaves a terminal state",
			func(f *fixture) []byte { return byID(f, "tasks", sets("state"), "T3", "open") },
			"tasks.flowmark.md", "- [ ] T3 Third\n"},
		{"50#state-columns: an update without the column a transition requires",
			func(f *fixture) []byte { return byID(f, "tasks", sets("state"), "T2", "closed") },
			"tasks.flowmark.md", "- [x] T2 Second\n"},
		{"50#state-columns: an update of a row whose state no history row records",
			func(f *fixture) []byte { return byID(f, "tickets", sets("state"), "K1", "closed") },
			"tickets.flowmark.md", "| K1 | [closed](states.flowmark.md) | Ticket |\n"},
		{"50#state-columns: an update past a limit",
			func(f *fixture) []byte { return byID(f, "tasks", sets("state"), "T1", "doing") },
			"tasks.flowmark.md", "- [~] T1 First\n"},
		{"50#state-columns: a value no machine line names that the foreign key allows",
			func(f *fixture) []byte { return byID(f, "tickets", sets("state"), "K1", "parked") },
			"tickets.flowmark.md", "| K1 | [parked](states.flowmark.md) | Ticket |\n"},
	}
	for _, c := range accepts {
		t.Run(c.name, func(t *testing.T) {
			f := stateFixture(t)
			history := f.file("task_history.flowmark.md")
			res := f.ok(c.msg(f))
			f.commit(res)
			if got := f.file(c.file); !strings.Contains(got, c.want) {
				t.Fatalf("%s:\n%s\nwant the line %q", c.file, got, c.want)
			}
			if got := f.file("task_history.flowmark.md"); got != history {
				t.Fatalf("the write wrote a history row:\n%s", got)
			}
			for _, tg := range res.Post.Targets {
				if strings.HasSuffix(tg.Path, "_history.flowmark.md") {
					t.Fatalf("the write targets %s", tg.Path)
				}
			}
		})
	}

	f := stateFixture(t)
	rejects := []struct {
		name string
		msg  []byte
		code string
		want []dupFault
	}{
		{"50#state-columns: a list value that no checkbox line names", byID(f, "tasks", sets("state"), "T1", "lost"), "CONSTRAINT",
			[]dupFault{{"tasks", "T1", -1, "state", "CONSTRAINT", "no checkbox"}}},
		{"50#state-columns: an insert of a list value that no checkbox line names", f.msg("insert", "tasks", map[string]any{"set": sets("id", "title", "state")}, "T4", "Fourth", "lost"), "CONSTRAINT",
			[]dupFault{{"tasks", "T4", -1, "state", "CONSTRAINT", "no checkbox"}}},
		{"50#state-columns: a value the foreign key to the states refuses", byID(f, "tickets", sets("state"), "K1", "nowhere"), "FOREIGN_KEY",
			[]dupFault{{"tickets", "K1", -1, "state", "FOREIGN_KEY", "nowhere"}}},
		{"50#state-columns: a value the check refuses", byID(f, "cards", sets("state"), "C1", "parked"), "CONSTRAINT",
			[]dupFault{{"cards", "C1", -1, "", "CONSTRAINT", "ck_cards_state"}}},
		{"50#state-columns: a value longer than the column", byID(f, "tasks", sets("state"), "T1", "a-state-longer-than-16"), "CODEC_ENCODE", nil},
	}
	for _, c := range rejects {
		f.sub(t, c.name, func(f *fixture) {
			err := f.failure(c.msg, c.code)
			if c.want != nil {
				wantFaults(f.t, err, c.want)
			}
		})
	}
}
