// Package corpustest writes the parts of a test corpus that every package's tests share.
package corpustest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WriteSetFile writes dbspec/dbspec-set.json into the dbspec directory dir, listing every .dbs file of dir in name
// order, unless the directory already holds a set file: a test of the set file's own rules writes the one it needs.
// A corpus without dbspec/, or a dbspec/ without a .dbs file, gets no set file, so the catalog reports it as it does
// for any corpus.
func WriteSetFile(dir string) error {
	setFile := filepath.Join(dir, "dbspec-set.json")
	if _, err := os.Lstat(setFile); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var documents []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".dbs") {
			documents = append(documents, e.Name())
		}
	}
	if len(documents) == 0 {
		return nil
	}
	sort.Strings(documents)
	text, err := json.Marshal(map[string]any{"schema": 1, "documents": documents})
	if err != nil {
		return err
	}
	return os.WriteFile(setFile, append(text, '\n'), 0o644)
}
