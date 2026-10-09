// Tests of the conformance vectors: corpus before, request, expected rows (spec 90).
package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/polyspec/flowmark/executor"
)

type vector struct {
	Name   string            `json:"name"`
	Corpus map[string]string `json:"corpus"`
	Call   struct {
		Kind  string `json:"kind"`
		Table string `json:"table"`
		Query struct {
			Where []struct {
				Column string `json:"column"`
				Op     string `json:"op"`
				Value  string `json:"value"`
			} `json:"where"`
			Order string `json:"order"`
			Limit int    `json:"limit"`
		} `json:"query"`
	} `json:"call"`
	Expect struct {
		Rows  []map[string]string `json:"rows"`
		Count int                 `json:"count"`
		Page  []map[string]string `json:"page"`
		Total int                 `json:"total"`
	} `json:"expect"`
}

func TestVectors(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("vectors", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("vectors: %v (%d)", err, len(files))
	}
	for _, file := range files {
		text, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var test vector
		if err := json.Unmarshal(text, &test); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		t.Run(test.Name, func(t *testing.T) {
			root := t.TempDir()
			for name, text := range test.Corpus {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			corpus, err := executor.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			query := executor.Query{Order: test.Call.Query.Order, Limit: test.Call.Query.Limit}
			for _, condition := range test.Call.Query.Where {
				query.Where = append(query.Where, executor.Condition{Column: condition.Column, Op: condition.Op, Value: condition.Value})
			}
			switch test.Call.Kind {
			case "all":
				rows, err := corpus.All(test.Call.Table, query)
				if err != nil {
					t.Fatal(err)
				}
				compareRows(t, rows, test.Expect.Rows)
			case "count":
				count, err := corpus.Count(test.Call.Table, query)
				if err != nil {
					t.Fatal(err)
				}
				if count != test.Expect.Count {
					t.Fatalf("count: %d, want %d", count, test.Expect.Count)
				}
			case "paginate":
				page, total, err := corpus.Paginate(test.Call.Table, query)
				if err != nil {
					t.Fatal(err)
				}
				compareRows(t, page, test.Expect.Page)
				if total != test.Expect.Total {
					t.Fatalf("total: %d, want %d", total, test.Expect.Total)
				}
			default:
				t.Fatalf("the kind %q is not a vector kind", test.Call.Kind)
			}
		})
	}
}

// compareRows checks the expected projection of each row: the keys the expectation names.
func compareRows(t *testing.T, rows []executor.Row, expected []map[string]string) {
	t.Helper()
	if len(rows) != len(expected) {
		t.Fatalf("rows: %d, want %d", len(rows), len(expected))
	}
	for i, want := range expected {
		for column, value := range want {
			if rows[i][column] != value {
				t.Fatalf("row %d, column %s: %q, want %q", i, column, rows[i][column], value)
			}
		}
	}
}
