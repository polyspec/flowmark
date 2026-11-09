package validate

import (
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// documentHistoryCatalog is a state table and a history table of it, both stored as document tables: ordinary tables,
// since the engine holds no state machine (spec 50 State columns).
const documentHistoryCatalog = `dbspec 1 flowmark

table tickets {
  id varchar(16)
  state varchar(16)
  title varchar(64)
  primary key (id)
  settings {
    store document table
    title title
  }
}
` + "\n" + corpustest.TicketHistoryTable

// historyCorpus writes a corpus of the document-history catalog holding files, and
// returns its catalog and root.
func historyCorpus(t *testing.T, files map[string]string) (*catalog.Catalog, string) {
	t.Helper()
	root := t.TempDir()
	corpustest.WriteFiles(t, root, map[string]string{"dbspec/flowmark.dbs": documentHistoryCatalog})
	corpustest.WriteTree(t, root, files)
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

// TestDocumentTableHistory pins that a state table and its history table, stored as document tables, hold ordinary
// rows (spec 50 State columns): no gap, chain or transition of the machine is judged.
func TestDocumentTableHistory(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{name: "50#state-columns: a state where its document-table history ends is no fault",
			files: map[string]string{
				"tickets.flowmark.md":        ticketRows([2]string{"K1", "closed"}, [2]string{"K2", "open"}),
				"ticket_history.flowmark.md": historyRows([4]string{"1", "K1", "open", "doing"}, [4]string{"2", "K1", "doing", "closed"}),
			}},
		{name: "50#state-columns: a state that differs from the last to of its document-table history is no fault",
			files: map[string]string{
				"tickets.flowmark.md":        ticketRows([2]string{"K1", "closed"}),
				"ticket_history.flowmark.md": historyRows([4]string{"1", "K1", "open", "doing"}),
			}},
		{name: "50#state-columns: a non-initial state without document-table history rows is no fault",
			files: map[string]string{
				"tickets.flowmark.md":        ticketRows([2]string{"K1", "doing"}, [2]string{"K2", "doing"}),
				"ticket_history.flowmark.md": historyRows([4]string{"1", "K2", "open", "doing"}),
			}},
		{name: "50#state-columns: a document-table history row whose from differs from the previous to is no fault",
			files: map[string]string{
				"tickets.flowmark.md":        ticketRows([2]string{"K1", "closed"}),
				"ticket_history.flowmark.md": historyRows([4]string{"1", "K1", "open", "doing"}, [4]string{"2", "K1", "open", "closed"}),
			}},
		{name: "50#state-columns: any state its type allows is no fault in table storage",
			files: map[string]string{
				"tickets.flowmark.md": ticketRows([2]string{"K1", "parked"}),
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, root := historyCorpus(t, tc.files)
			checkCorpus(t, cat, root, tc.want)
		})
	}
}
