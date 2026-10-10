package store

import (
	"path"
	"strings"
)

// RelLink returns the link from the corpus file fromFile to the corpus file
// toFile: the relative path with "/" separators, followed by "#anchor" when
// anchor is not empty. A link inside the file itself is "#anchor" alone, or the
// file's name when there is no anchor.
func RelLink(fromFile, toFile, anchor string) (string, error) {
	var rel string
	if fromFile == toFile {
		rel = path.Base(toFile)
		if anchor != "" {
			return "#" + anchor, nil
		}
		return rel, nil
	}
	rel = slashRel(path.Dir(fromFile), toFile)
	if anchor != "" {
		rel += "#" + anchor
	}
	return rel, nil
}

// slashRel returns the path of target relative to the folder dir, both clean
// slash-separated corpus paths with no ".." element.
func slashRel(dir, target string) string {
	var dirs []string
	if dir != "." {
		dirs = strings.Split(dir, "/")
	}
	targets := strings.Split(target, "/")
	common := 0
	for common < len(dirs) && common < len(targets)-1 && dirs[common] == targets[common] {
		common++
	}
	ups := strings.Repeat("../", len(dirs)-common)
	return ups + strings.Join(targets[common:], "/")
}

// DetailAnchor returns the anchor of the detail section of a row:
// <table>-row-<lowercased key>.
func DetailAnchor(table, key string) string {
	return table + "-row-" + strings.ToLower(key)
}
