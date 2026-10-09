// Tests of the aggregate kinds: sum, avg, group_count and paginate over the rows.
package executor

import (
	"os"
	"path/filepath"
	"testing"
)

const numeric = "<!-- doc-id: board -->\n# Board\n\n## Work\n\n- [ ] T1 5\n- [ ] T2 7\n- [ ] T3 9\n"

func numericCorpus(t *testing.T) *Corpus {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "board.md"), []byte(numeric), 0o644); err != nil {
		t.Fatal(err)
	}
	corpus, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return corpus
}

func TestSumAndAvg(t *testing.T) {
	corpus := numericCorpus(t)
	sum, err := corpus.Sum("items", "title", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if sum != 21 {
		t.Fatalf("sum: %d", sum)
	}
	average, err := corpus.Avg("items", "title", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if average != 7 {
		t.Fatalf("avg: %d", average)
	}
}

func TestGroupCount(t *testing.T) {
	corpus := load(t)
	groups, err := corpus.GroupCount("items", "state", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 || groups["[o]"] != 1 || groups["[ ]"] != 1 || groups["[~]"] != 1 {
		t.Fatalf("groups: %v", groups)
	}
}

func TestPaginate(t *testing.T) {
	corpus := load(t)
	page, total, err := corpus.Paginate("items", Query{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(page) != 2 || page[0]["id"] != "T1.1" {
		t.Fatalf("page: %v, total: %d", page, total)
	}
}
