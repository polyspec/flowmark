package ops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"github.com/polyspec/flowmark/packages/go/statement"
)

// The checklist of this repository is a corpus (T61, docs/plans/flowmark/plan.md "T61 — the checklist as a corpus").
const checklistCorpus = "../../../docs/plans/flowmark/checklist"

// checklistTree is the catalog of the checklist corpus with the rows of its items file, or with none when rows is false.
func checklistTree(t *testing.T, rows bool) map[string]string {
	t.Helper()
	tree := map[string]string{}
	names := []string{"dbspec/checklist.dbs", "dbspec/dbspec-set.json"}
	if rows {
		names = append(names, "items.flowmark.md")
	}
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(checklistCorpus, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		tree[name] = string(b)
	}
	return tree
}

// checklistRow is the values of a row of items in the state state, with the cause and the retry in both languages.
func checklistRow(id, state string, cause, causeKo, retry, retryKo any) string {
	v := map[string]any{
		"id": id, "task": "a task", "task_ko": "작업", "deliverables": "-", "deliverables_ko": "-",
		"verification": "-", "verification_ko": "-", "position": 1, "state": state,
		"cause": cause, "cause_ko": causeKo, "retry": retry, "retry_ko": retryKo,
	}
	b, err := json.Marshal(map[string]any{"table": "items", "values": v})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// TestChecklistCatalog checks each check of the checklist catalog with the rows it accepts and the rows it refuses.
func TestChecklistCatalog(t *testing.T) {
	cases := []struct {
		name, state                   string
		cause, causeKo, retry, retryKo any
		refusedBy                     string
	}{
		{name: "waiting holds no cause or retry", state: "waiting"},
		{name: "active", state: "active"},
		{name: "verifying", state: "verifying"},
		{name: "done", state: "done"},
		{name: "bypassed holds a cause and a retry in both languages", state: "bypassed", cause: "c", causeKo: "원", retry: "r", retryKo: "재"},
		{name: "impossible holds a cause in both languages and no retry", state: "impossible", cause: "c", causeKo: "원"},
		{name: "a state that is no state of the checklist", state: "closed", refusedBy: "ck_items_state"},
		{name: "the marker is no state name", state: "o", refusedBy: "ck_items_state"},
		{name: "bypassed without a retry", state: "bypassed", cause: "c", causeKo: "원", refusedBy: "ck_items_retry"},
		{name: "bypassed without a Korean cause", state: "bypassed", cause: "c", retry: "r", retryKo: "재", refusedBy: "ck_items_cause"},
		{name: "impossible without a cause", state: "impossible", refusedBy: "ck_items_cause"},
		{name: "impossible with a retry", state: "impossible", cause: "c", causeKo: "원", retry: "r", retryKo: "재", refusedBy: "ck_items_retry"},
		{name: "done with a cause", state: "done", cause: "c", causeKo: "원", refusedBy: "ck_items_cause"},
		{name: "waiting with a retry", state: "waiting", retry: "r", retryKo: "재", refusedBy: "ck_items_retry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			corpustest.WriteTree(t, root, checklistTree(t, false))
			c := openCorpus(t, root)
			row := checklistRow("T1", tc.state, tc.cause, tc.causeKo, tc.retry, tc.retryKo)
			if tc.refusedBy == "" {
				ok(t, c, "row.insert", row)
				return
			}
			fails(t, c, "row.insert", row, "CONSTRAINT", tc.refusedBy)
			if got := ok(t, c, "rows", `{"table":"items"}`); got != `{"rows":[]}` {
				t.Fatalf("a refused insert wrote %s", got)
			}
		})
	}
}

// TestChecklistByState reads the corpus of this repository's checklist and checks that the board, a SQL query by state,
// answers each row once: the rows of the states together are the rows of the table, each key once.
func TestChecklistByState(t *testing.T) {
	root := t.TempDir()
	corpustest.WriteTree(t, root, checklistTree(t, true))
	c := openCorpus(t, root)
	var all struct {
		Rows []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(ok(t, c, "rows", `{"table":"items"}`)), &all); err != nil {
		t.Fatal(err)
	}
	if len(all.Rows) == 0 {
		t.Fatal("the checklist corpus holds no row")
	}
	s, err := c.Session("board")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	seen := map[string]string{}
	for _, state := range []string{"waiting", "active", "verifying", "done", "bypassed", "impossible"} {
		a, err := statement.Run(s, "SELECT id FROM items WHERE state = ?", []json.RawMessage{json.RawMessage(strconv.Quote(state))})
		if err != nil {
			t.Fatalf("the board query of %s: %v", state, err)
		}
		for _, row := range a.Rows {
			id, isText := row[0].(string)
			if !isText {
				t.Fatalf("the board query of %s answered the id %v, not a text", state, row[0])
			}
			if prior, twice := seen[id]; twice {
				t.Fatalf("%s is answered by %s and by %s", id, prior, state)
			}
			seen[id] = state
		}
	}
	if len(seen) != len(all.Rows) {
		var missing []string
		for _, r := range all.Rows {
			if _, found := seen[r.ID]; !found {
				missing = append(missing, r.ID+"="+r.State)
			}
		}
		t.Fatalf("the states answer %d of %d rows; unanswered: %s", len(seen), len(all.Rows), strings.Join(missing, ", "))
	}
}
