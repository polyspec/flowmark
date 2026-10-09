package corpus

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ValidateSelected validates the documents a documents configuration selects: the `include`
// globs minus the `exclude` globs, both relative to `root` with `**` matching any number of
// path segments.
func ValidateSelected(root string, include, exclude []string) ([]Finding, error) {
	documents, err := selectedDocuments(root, include, exclude)
	if err != nil {
		return nil, err
	}
	return validateDocuments(documents)
}

// selectedDocuments reads the selected markdown files as path-text pairs.
func selectedDocuments(root string, include, exclude []string) ([]documentText, error) {
	var documents []documentText
	walkedLast = false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if path == root {
			return nil
		}
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
		relative = filepath.ToSlash(relative)
		if !matchesAny(include, relative) || matchesAny(exclude, relative) {
			return nil
		}
		text, err := readFile(path)
		if err != nil {
			return err
		}
		walkedLast = true
		documents = append(documents, documentText{path: relative, text: text})
		return nil
	})
	return documents, err
}

// matchesAny names whether any glob matches the path. A `**` segment matches any number of
// segments; every other segment is a filepath.Match pattern.
func matchesAny(globs []string, path string) bool {
	for _, glob := range globs {
		if matchGlob(strings.Split(glob, "/"), strings.Split(path, "/")) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		for skip := 0; skip <= len(path); skip++ {
			if matchGlob(pattern[1:], path[skip:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	ok, err := filepath.Match(pattern[0], path[0])
	if err != nil || !ok {
		return false
	}
	return matchGlob(pattern[1:], path[1:])
}

// documentText is one selected document: its path relative to the root and its text.
type documentText struct {
	path string
	text string
}

// readFile reads one file as text.
func readFile(path string) (string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(text), nil
}

// walkedLast records whether the last selection saw at least one document; a test probe.
var walkedLast = false

// sawDocuments reports whether the last selection found a document.
func sawDocuments() bool { return walkedLast }
