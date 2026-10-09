// Package corpus validates a corpus directory: strict parse, unique keys and resolved
// references (spec 50-constraints).
package corpus

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/polyspec/flowmark/codec"
)

// Finding is one corpus-level violation with the coordinates of its place.
type Finding struct {
	File    string
	Line    int
	Column  int
	Rule    string
	Message string
}

// The task-ID token and its boundary rules (spec 10-canonical-format): a reference is a
// token in prose, not inside a word, not before a digit after a dot.
var token = regexp.MustCompile(`T[0-9]+(?:[.-][0-9]+)*`)

// Validate walks the markdown documents of `root` (skipping hidden entries and .flowmark)
// and returns the findings of the strict parse, the key uniqueness and the references.
func Validate(root string) ([]Finding, error) {
	findings := []Finding{}
	docIDs := map[string]string{}
	itemIDs := map[string]string{}
	itemAt := map[string]string{}
	type document struct {
		path string
		text string
	}
	documents := []document{}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".md") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		documents = append(documents, document{path: filepath.ToSlash(relative), text: string(text)})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Pass 1: the strict parse and the keys.
	for _, doc := range documents {
		parsed, diagnostics := codec.Parse(doc.text)
		for _, diagnostic := range diagnostics {
			findings = append(findings, Finding{File: doc.path, Line: diagnostic.Line, Column: diagnostic.Column, Rule: diagnostic.Rule, Message: diagnostic.Message})
		}
		if parsed.DocID != "" {
			if first, ok := docIDs[parsed.DocID]; ok {
				findings = append(findings, Finding{File: doc.path, Line: 1, Rule: "key.duplicate",
					Message: fmt.Sprintf("the doc-id %q appears again; the first is in %s", parsed.DocID, first)})
			} else {
				docIDs[parsed.DocID] = doc.path
			}
		}
		for _, item := range parsed.Items {
			if first, ok := itemIDs[item.ID]; ok {
				findings = append(findings, Finding{File: doc.path, Line: item.Line, Rule: "key.duplicate",
					Message: fmt.Sprintf("the item %q appears again; the first is in %s at line %s", item.ID, first, itemAt[item.ID])})
			} else {
				itemIDs[item.ID] = doc.path
				itemAt[item.ID] = fmt.Sprint(item.Line)
			}
		}
	}

	// Pass 2: the references — every task-ID token in prose resolves to an item.
	for _, doc := range documents {
		for _, line := range codec.Prose(doc.text) {
			for _, match := range token.FindAllStringIndex(line.Text, -1) {
				start, end := match[0], match[1]
				if !boundariesHold(line.Text, start, end) {
					continue
				}
				id := line.Text[start:end]
				if _, ok := itemIDs[id]; !ok {
					findings = append(findings, Finding{File: doc.path, Line: line.Number, Column: start + 1, Rule: "reference.dangling",
						Message: fmt.Sprintf("the task %q resolves to no item", id)})
				}
			}
		}
	}
	return findings, nil
}

// boundariesHold checks the token edges: not inside a word, not after a dot, not before a
// word character, a hyphen or a dotted digit.
func boundariesHold(text string, start, end int) bool {
	if start > 0 {
		prev := text[start-1]
		if prev == '.' || prev == '_' || isWordByte(prev) {
			return false
		}
	}
	if end < len(text) {
		next := text[end]
		if next == '-' || next == '_' || isWordByte(next) {
			return false
		}
		if next == '.' && end+1 < len(text) && isWordByte(text[end+1]) {
			return false
		}
	}
	return true
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
