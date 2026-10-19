package examples

import (
	"strings"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/ops"
)

// The board corpora: the comments of a post as a block list inside the post's file, and as one file per comment whose
// field list links to its post.
var boardCorpora = []struct {
	root    string
	storage catalog.StorageKind
}{
	{"board/block", catalog.StorageBlock},
	{"board/files", catalog.StorageFiles},
}

// boardRows reads every table of each board corpus and fails when the variants differ.
func boardRows(t *testing.T, step string, cs []*ops.Corpus) {
	t.Helper()
	var first map[string]string
	for i, c := range cs {
		rows := map[string]string{}
		for _, tb := range c.Catalog().Tables {
			rows[tb.Name] = run(t, c, "rows", `{"table":"`+tb.Name+`"}`)
		}
		if i == 0 {
			first = rows
			continue
		}
		equalTables(t, "the board after "+step, boardCorpora[0].root, boardCorpora[i].root, first, rows)
	}
}

// TestBoardVariants: both board corpora store comments as their variant says, are valid and canonical, read back the
// same rows, and stay equal and canonical through creating a post, commenting on it, replying, editing and deleting,
// each run through the engine on both.
func TestBoardVariants(t *testing.T) {
	var roots []string
	var cs []*ops.Corpus
	for _, v := range boardCorpora {
		sound(t, copyCorpus(t, v.root))
		root := copyCorpus(t, v.root)
		c := open(t, root)
		comments, ok := c.Catalog().Table("comments")
		if !ok {
			t.Fatalf("%s declares no table comments", v.root)
		}
		if comments.Storage.Kind != v.storage {
			t.Errorf("%s stores comments as %s, want %s", v.root, comments.Storage.Kind, v.storage)
		}
		tables(t, c, v.root)
		roots = append(roots, root)
		cs = append(cs, c)
	}
	boardRows(t, "opening", cs)
	steps := []struct{ name, op, members string }{
		{"creating a post", "row.insert", `{"table":"posts","values":{"title":"Release notes","body":"What changed this week.","author":"U2","created":"2026-10-10 09:00:00.000000"}}`},
		{"commenting", "row.insert", `{"table":"comments","values":{"post":"P3","body":"The list is long.","author":"U1","created":"2026-10-10 09:10:00.000000"}}`},
		{"a second comment", "row.insert", `{"table":"comments","values":{"post":"P3","body":"Thanks for writing it.","author":"U3","created":"2026-10-10 09:20:00.000000"}}`},
		{"replying", "row.insert", `{"table":"comments","values":{"post":"P3","parent":"C5","body":"It is shorter now.","author":"U2","created":"2026-10-10 09:30:00.000000"}}`},
		{"editing the post", "row.update", `{"table":"posts","key":"P3","values":{"title":"Release notes, week 41","body":"What changed this week.\n\nAnd what comes next."}}`},
		{"editing a comment", "row.update", `{"table":"comments","key":"C6","values":{"body":"Thank you for writing it."}}`},
		{"deleting the reply", "row.delete", `{"table":"comments","key":"C7"}`},
		{"deleting a comment", "row.delete", `{"table":"comments","key":"C5"}`},
		{"deleting the last comment", "row.delete", `{"table":"comments","key":"C6"}`},
		{"deleting the post", "row.delete", `{"table":"posts","key":"P3"}`},
	}
	for _, s := range steps {
		for _, c := range cs {
			run(t, c, s.op, s.members)
		}
		boardRows(t, s.name, cs)
		if s.name == "editing a comment" {
			for i, root := range roots {
				boardPage(t, boardCorpora[i].storage, root)
				sound(t, root)
			}
		}
	}
	for _, root := range roots {
		sound(t, root)
	}
}

// boardPage checks that a board corpus reads as a board page: in the block variant the post's file holds its comments
// and the reply; in the files variant each comment's file links to its post.
func boardPage(t *testing.T, kind catalog.StorageKind, root string) {
	t.Helper()
	files := snapshot(t, root)
	post := files["posts/2026-10/P3.flowmark.md"]
	if !strings.HasPrefix(post, "# Release notes, week 41\n") {
		t.Errorf("%s: the post P3 does not open with its title:\n%s", root, post)
	}
	switch kind {
	case catalog.StorageBlock:
		for _, text := range []string{"The list is long.", "Thank you for writing it.", "It is shorter now."} {
			if !strings.Contains(post, text) {
				t.Errorf("%s: the post's file lacks the comment %q:\n%s", root, text, post)
			}
		}
	case catalog.StorageFiles:
		for _, key := range []string{"C5", "C6", "C7"} {
			if c := files["comments/"+key+".flowmark.md"]; !strings.Contains(c, "../posts/2026-10/P3.flowmark.md") {
				t.Errorf("%s: the comment %s does not link to its post:\n%s", root, key, c)
			}
		}
	}
}
