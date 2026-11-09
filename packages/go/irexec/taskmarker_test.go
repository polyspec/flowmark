package irexec

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
)

// markerCatalog declares the task markers of spec 30 Markdown settings and the state columns of spec 50: tasks, a list
// whose varchar(1) state a check constrains; chores, a list whose nullable varchar(1) mark takes any character; todos, a
// list whose nullable bool done is the GFM checkbox; tickets, a table whose state has a foreign key to a table of
// states; and cards, a table whose state a check constrains. The engine holds no machine, so no write writes a
// history row.
const markerCatalog = `dbspec 1 flowmark

table states {
  id varchar(16)
  primary key (id)
  settings {
    store document table
  }
}

table tasks {
  id varchar(16)
  state varchar(1) default ' '
  title varchar(64)
  primary key (id)
  check ck_tasks_state (state in (' ', '~', 'x'))
  settings {
    store document list
    title title
    task_marker state
  }
}

table chores {
  id varchar(16)
  mark varchar(1) null
  title varchar(64)
  primary key (id)
  settings {
    store document list
    title title
    task_marker mark
  }
}

table todos {
  id varchar(16)
  done bool null
  title varchar(64)
  primary key (id)
  settings {
    store document list
    title title
    task_marker done
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

// markerDocs is the corpus of markerCatalog: the tasks T1 to T3 in each state, the chores C1 with a mark and C2
// without, the todos D1 unchecked, D2 checked and D3 without a box, a ticket open, a card open, and the states open,
// closed and parked.
func markerDocs() map[string]string {
	return map[string]string{
		"dbspec/dbspec-set.json": `{"schema": 1, "documents": ["flowmark.dbs"]}` + "\n",
		"dbspec/flowmark.dbs":    markerCatalog,
		"states.flowmark.md": corpustest.Lines(
			"# states",
			"| id |",
			"| --- |",
			"| closed |",
			"| open |",
			"| parked |",
		),
		"tasks.flowmark.md":  corpustest.Lines("# tasks", "- [ ] T1 First", "- [~] T2 Second", "- [x] T3 Third"),
		"chores.flowmark.md": corpustest.Lines("# chores", "- [?] C1 Ask", "- C2 Plain"),
		"todos.flowmark.md":  corpustest.Lines("# todos", "- [ ] D1 Open", "- [x] D2 Closed", "- D3 None"),
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

// markerFixture is a corpus of markerCatalog.
func markerFixture(t *testing.T) *fixture {
	t.Helper()
	src := t.TempDir()
	corpustest.WriteFiles(t, src, markerDocs())
	return newFixture(t, src, nil)
}

// TestTaskMarker pins the writes of a task marker column and of a state column (spec 30 Markdown settings, spec 50
// State columns): a list row writes the value between the brackets of its task marker — a varchar(1) value as itself,
// the empty text as [], a bool as [ ] or [x], NULL as no box — and the values are those the column's type and the
// schema's constraints allow, as on every database; no write writes a history row.
func TestTaskMarker(t *testing.T) {
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
		{"30#markdown-settings: an insert writes its varchar(1) value in the box",
			func(f *fixture) []byte {
				return f.msg("insert", "tasks", map[string]any{"set": sets("id", "title", "state")}, "T4", "Fourth", "~")
			},
			"tasks.flowmark.md", "- [~] T4 Fourth\n"},
		{"30#markdown-settings: an insert without the column writes its default",
			func(f *fixture) []byte { return f.msg("insert", "tasks", map[string]any{"set": sets("id", "title")}, "T4", "Fourth") },
			"tasks.flowmark.md", "- [ ] T4 Fourth\n"},
		{"50#state-columns: an update to any value the check allows, in any order",
			func(f *fixture) []byte { return byID(f, "tasks", sets("state"), "T3", " ") },
			"tasks.flowmark.md", "- [ ] T3 Third\n"},
		{"30#markdown-settings: any one character of a varchar(1) marker",
			func(f *fixture) []byte { return byID(f, "chores", sets("mark"), "C2", "!") },
			"chores.flowmark.md", "- [!] C2 Plain\n"},
		{"30#markdown-settings: the empty text of a varchar(1) marker is []",
			func(f *fixture) []byte { return byID(f, "chores", sets("mark"), "C1", "") },
			"chores.flowmark.md", "- [] C1 Ask\n"},
		{"30#markdown-settings: a NULL marker is a row without a box",
			func(f *fixture) []byte { return byID(f, "chores", sets("mark"), "C1", nil) },
			"chores.flowmark.md", "- C1 Ask\n"},
		{"30#markdown-settings: a bool marker true is the checked box",
			func(f *fixture) []byte { return byID(f, "todos", sets("done"), "D1", true) },
			"todos.flowmark.md", "- [x] D1 Open\n"},
		{"30#markdown-settings: a bool marker false is the unchecked box",
			func(f *fixture) []byte { return byID(f, "todos", sets("done"), "D3", false) },
			"todos.flowmark.md", "- [ ] D3 None\n"},
		{"30#markdown-settings: a NULL bool marker is a row without a box",
			func(f *fixture) []byte { return byID(f, "todos", sets("done"), "D2", nil) },
			"todos.flowmark.md", "- D2 Closed\n"},
		{"50#state-columns: a value the foreign key allows",
			func(f *fixture) []byte { return byID(f, "tickets", sets("state"), "K1", "parked") },
			"tickets.flowmark.md", "| K1 | [parked](states.flowmark.md) | Ticket |\n"},
	}
	for _, c := range accepts {
		t.Run(c.name, func(t *testing.T) {
			f := markerFixture(t)
			res := f.ok(c.msg(f))
			wantTargets(t, res, c.file)
			f.commit(res)
			if got := f.file(c.file); !strings.Contains(got, c.want) {
				t.Fatalf("%s:\n%s\nwant the line %q", c.file, got, c.want)
			}
		})
	}
	t.Run("30#markdown-settings: a read answers the value in the box", func(t *testing.T) {
		f := markerFixture(t)
		f.want(f.msg("all", "chores", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"mark"}}}), `{"rows":[{"id":"C1","mark":"?"},{"id":"C2","mark":null}]}`)
		f.want(f.msg("all", "todos", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"done"}}}), `{"rows":[{"id":"D1","done":false},{"id":"D2","done":true},{"id":"D3","done":null}]}`)
	})

	f := markerFixture(t)
	rejects := []struct {
		name string
		msg  []byte
		code string
		want []dupFault
	}{
		{"50#state-columns: a value the check refuses", byID(f, "tasks", sets("state"), "T1", "o"), "CONSTRAINT",
			[]dupFault{{"tasks", "T1", -1, "", "CONSTRAINT", "ck_tasks_state"}}},
		{"30#markdown-settings: a value longer than the varchar(1) of the marker", byID(f, "chores", sets("mark"), "C1", "ab"), "CODEC_ENCODE", nil},
		{"30#markdown-settings: a marker value that would break its line", byID(f, "chores", sets("mark"), "C1", "\n"), "CONSTRAINT",
			[]dupFault{{"chores", "C1", -1, "mark", "CONSTRAINT", "line break"}}},
		{"30#markdown-settings: a value of a non-null marker set to NULL", byID(f, "tasks", sets("state"), "T1", nil), "CODEC_ENCODE", nil},
		{"50#state-columns: a value the foreign key to the states refuses", byID(f, "tickets", sets("state"), "K1", "nowhere"), "FOREIGN_KEY",
			[]dupFault{{"tickets", "K1", -1, "state", "FOREIGN_KEY", "nowhere"}}},
		{"50#state-columns: a value the check of a table refuses", byID(f, "cards", sets("state"), "C1", "parked"), "CONSTRAINT",
			[]dupFault{{"cards", "C1", -1, "", "CONSTRAINT", "ck_cards_state"}}},
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
