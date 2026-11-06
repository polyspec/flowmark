package statement_test

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/statement"
)

// TestStatementEndsTheTransaction pins a statement that ends the SQLite transaction itself, as RAISE(ROLLBACK) does
// (spec 40, SQL statements): its answer is the statement's error alone, the transaction's earlier writes are gone, the
// ROLLBACK that follows has nothing to end, and the next transaction writes the corpus. A RAISE(ABORT) keeps the
// transaction and its earlier writes.
func TestStatementEndsTheTransaction(t *testing.T) {
	for _, tc := range []struct {
		name, begin, raise string
		ends               bool
	}{
		{"RAISE(ROLLBACK) after BEGIN IMMEDIATE", "BEGIN IMMEDIATE", "ROLLBACK", true},
		{"RAISE(ROLLBACK) after a deferred BEGIN", "BEGIN", "ROLLBACK", true},
		{"RAISE(ABORT) after BEGIN IMMEDIATE", "BEGIN IMMEDIATE", "ABORT", false},
		{"RAISE(ABORT) after a deferred BEGIN", "BEGIN", "ABORT", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, root := open(t)
			run(t, s, "CREATE TABLE guarded (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
			run(t, s, "CREATE TRIGGER guarded_refuse BEFORE INSERT ON guarded WHEN NEW.name = 'refused' BEGIN SELECT RAISE("+tc.raise+", 'refused'); END")
			run(t, s, tc.begin)
			run(t, s, "INSERT INTO guarded (id, name) VALUES (1, 'kept')")
			_, err := statement.Run(s, "INSERT INTO guarded (id, name) VALUES (2, 'refused')", nil)
			if err == nil {
				t.Fatal("the refused insert answered no error")
			}
			if want := "constraint failed: refused (1811) (SQLITE_CONSTRAINT_TRIGGER)"; !strings.HasSuffix(err.Error(), want) || strings.Contains(err.Error(), "\n") {
				t.Fatalf("the refused insert answered %q, want the statement's error alone, ending %q", err, want)
			}
			want := `[[1]]`
			if tc.ends {
				want = `[[0]]`
			}
			if got := cells(run(t, s, "SELECT COUNT(*) FROM guarded")); got != want {
				t.Fatalf("after the refused insert the table holds %s rows, want %s", got, want)
			}
			run(t, s, "ROLLBACK")
			run(t, s, "BEGIN IMMEDIATE")
			run(t, s, "INSERT INTO guarded (id, name) VALUES (3, 'next')")
			run(t, s, "COMMIT")
			if got := cells(run(t, s, "SELECT id FROM guarded ORDER BY id")); got != `[[3]]` {
				t.Fatalf("the next transaction left rows %s, want [[3]]", got)
			}
			if got := file(t, root, "guarded.flowmark.md"); !strings.Contains(got, "next") || strings.Contains(got, "kept") {
				t.Fatalf("the corpus holds:\n%s", got)
			}
		})
	}
}
