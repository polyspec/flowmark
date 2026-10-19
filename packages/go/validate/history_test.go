package validate

import (
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"os"
	"path/filepath"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// documentHistoryCatalog is a state table whose history table is stored as a
// document table (spec 30 The history table): its rows are found through the
// history foreign key, not in a block of the state row.
const documentHistoryCatalog = `dbspec 1 flowmark

table tickets {
  id varchar(16)
  state varchar(16)
  title varchar(64)
  primary key (id)
  settings {
    store document table
    title title
    state_machine state initial open
    state_machine state open -> doing
    state_machine state doing -> closed
    state_machine state terminal closed
    state_machine state history ticket_history row ticket from from_state to to_state at at
  }
}

table ticket_history {
  id i64 identity
  ticket varchar(16)
  from_state varchar(16)
  to_state varchar(16)
  at datetime(6)
  primary key (id)
  index ix_ticket_history_ticket (ticket)
  foreign key fk_ticket_history (ticket) references tickets (id) on delete restrict on update restrict
  settings {
    store document table
  }
}
`

// historyCorpus writes a corpus of the document-history catalog holding files, and
// returns its catalog and root.
func historyCorpus(t *testing.T, files map[string]string) (*catalog.Catalog, string) {
	t.Helper()
	root := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("dbspec/flowmark.dbs", documentHistoryCatalog)
	for rel, text := range files {
		write(rel, text)
	}
	if err := corpustest.WriteSetFile(filepath.Join(root, "dbspec")); err != nil {
		t.Fatal(err)
	}
	cat, diags := catalog.Load(root)
	if len(diags) > 0 {
		t.Fatalf("catalog: %+v", diags)
	}
	return cat, root
}

// ticketRows returns the tickets document with rows "key state".
func ticketRows(rows ...[2]string) string {
	s := "# tickets\n| id | state | title |\n| --- | --- | --- |\n"
	for _, r := range rows {
		s += "| " + r[0] + " | " + r[1] + " | Ticket " + r[0] + " |\n"
	}
	return s
}

// historyRows returns the ticket_history document with rows "id ticket from to".
func historyRows(rows ...[4]string) string {
	s := "# ticket_history\n| id | ticket | from_state | to_state | at |\n| --- | --- | --- | --- | --- |\n"
	for _, r := range rows {
		s += "| " + r[0] + " | [" + r[1] + "](tickets.flowmark.md) | " + r[2] + " | " + r[3] + " | 2026-10-01 08:00:00.000000 |\n"
	}
	return s
}

// TestDocumentTableHistory pins history.gap, history.chain and state.transition for a history table
// stored as a document table: a state row's history rows are the rows of the
// history table whose foreign key names it, in corpus order.
func TestDocumentTableHistory(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{name: "50#a-state-changed-outside-the-engine: a state where its document-table history ends has no gap",
			files: map[string]string{
				"tickets.flowmark.md":        ticketRows([2]string{"K1", "closed"}, [2]string{"K2", "open"}),
				"ticket_history.flowmark.md": historyRows([4]string{"1", "K1", "open", "doing"}, [4]string{"2", "K1", "doing", "closed"}),
			}},
		{name: "50#a-state-changed-outside-the-engine: a state that differs from the last to of its document-table history",
			files: map[string]string{
				"tickets.flowmark.md":        ticketRows([2]string{"K1", "closed"}),
				"ticket_history.flowmark.md": historyRows([4]string{"1", "K1", "open", "doing"}),
			},
			want: []string{"tickets.flowmark.md:4 history.gap"}},
		{name: "50#a-state-changed-outside-the-engine: a non-initial state without document-table history rows",
			files: map[string]string{
				"tickets.flowmark.md":        ticketRows([2]string{"K1", "doing"}, [2]string{"K2", "doing"}),
				"ticket_history.flowmark.md": historyRows([4]string{"1", "K2", "open", "doing"}),
			},
			want: []string{"tickets.flowmark.md:4 history.gap"}},
		{name: "50#a-state-changed-outside-the-engine: a document-table history row whose from differs from the previous to",
			files: map[string]string{
				"tickets.flowmark.md":        ticketRows([2]string{"K1", "closed"}),
				"ticket_history.flowmark.md": historyRows([4]string{"1", "K1", "open", "doing"}, [4]string{"2", "K1", "open", "closed"}),
			},
			want: []string{
				"ticket_history.flowmark.md:5 history.chain",
				"ticket_history.flowmark.md:5 state.transition ~ from open to closed",
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, root := historyCorpus(t, tc.files)
			diags, err := Corpus(cat, root)
			if err != nil {
				t.Fatal(err)
			}
			check(t, diags, tc.want)
		})
	}
}
