// Package sync is the sync engine: md→db export as a derived SQL dump and db→md
// materialization with the provenance header (spec 70-sync). The export holds no state of
// its own — it rebuilds from the corpus on every run.
package sync

import (
	"fmt"
	"strings"

	"github.com/polyspec/flowmark/executor"
)

// ExportSQL writes the corpus of `root` as one deterministic SQL dump: the documents and the
// items as INSERT statements, ordered by their primary keys.
func ExportSQL(root string) (string, error) {
	corpus, err := executor.Open(root)
	if err != nil {
		return "", err
	}
	documents, err := corpus.All("documents", executor.Query{})
	if err != nil {
		return "", err
	}
	items, err := corpus.All("items", executor.Query{})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, row := range documents {
		fmt.Fprintf(&out, "INSERT INTO documents (doc_id, title) VALUES ('%s', '%s');\n", sql(row["doc_id"]), sql(row["title"]))
	}
	for _, row := range items {
		fmt.Fprintf(&out, "INSERT INTO items (id, state, title, doc_id) VALUES ('%s', '%s', '%s', '%s');\n",
			sql(row["id"]), sql(row["state"]), sql(row["title"]), sql(row["doc_id"]))
	}
	return out.String(), nil
}

// sql quotes one value for the dump.
func sql(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
