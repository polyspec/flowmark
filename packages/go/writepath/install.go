package writepath

import (
	"bytes"
	"context"
	"slices"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/store"
	"github.com/polyspec/flowmark/packages/go/store/journal"
)

// gitAttributes is the corpus path of the git attributes file, and attributesLine the line flowmark install keeps in
// it, so a git checkout keeps LF on every platform (spec 10 Encoding and line structure).
const (
	gitAttributes  = ".gitattributes"
	attributesLine = "*.flowmark.md text eol=lf"
)

// InstallResult is what flowmark install did: the documents and folders it created, sorted, and whether it added the
// line to .gitattributes.
type InstallResult struct {
	Created    []string
	Attributes bool
}

// Install runs flowmark install (spec 30 Install, spec 40 The faces) as its own transaction under the lock: in one
// journal commit, the file <table>.flowmark.md of each document table that has none, holding the empty table, and the
// line of .gitattributes when no line of it is that line; then the folder of each files table that has none. Another
// kind of entry at a table's place, or a .gitattributes that is no regular file, refuses it as CONFIG before anything
// is written. An install that finds everything in place writes nothing.
func (s *Session) Install(ctx context.Context) (_ *InstallResult, err error) {
	ctx, leave, err := s.enter(ctx)
	if err != nil {
		return nil, err
	}
	defer leave()
	if s.tx != nil {
		return nil, newError(codeConfig, "flowmark install is its own transaction; commit or roll back the open transaction first")
	}
	l, err := s.c.acquire(ctx, s.name)
	if err != nil {
		return nil, err
	}
	written := false
	defer func() { err = releaseAfter(l, written, err) }()
	res, changes, folders, err := s.c.installPlan(l)
	if err != nil {
		return nil, err
	}
	if len(changes) > 0 {
		if written, err = s.c.journalCommit(l, changes); err != nil {
			return nil, err
		}
	}
	for _, f := range folders {
		if err := s.c.j.MakeFolder(l, f); err != nil {
			return nil, driverError("creating the folder "+f, err)
		}
		written = true
	}
	return res, nil
}

// installPlan reads, under the held lock l, what stands at each place install fills, and returns its result, the
// files to commit and the folders to create after them.
func (c *Corpus) installPlan(l *journal.Lock) (*InstallResult, []journal.Change, []string, error) {
	res := &InstallResult{Created: []string{}}
	var changes []journal.Change
	var folders []string
	for _, tb := range c.cat.Tables {
		var place string
		var want journal.EntryKind
		switch tb.Storage.Kind {
		case catalog.StorageFiles:
			place, want = tb.Name, journal.EntryFolder
		case catalog.StorageDocument:
			place, want = tb.Name+".flowmark.md", journal.EntryFile
			if tb.Storage.Shard != nil {
				// A sharded document table stands in the folder of its periods (spec 20 Time shards).
				place, want = tb.Name, journal.EntryFolder
			}
		default:
			continue // an empty block is not written
		}
		kind, err := c.j.Entry(place)
		if err != nil {
			return nil, nil, nil, driverError("reading the place of the table "+tb.Name, err)
		}
		switch {
		case kind == want:
			continue
		case kind != journal.EntryAbsent:
			return nil, nil, nil, newError(codeConfig, "%s, the place of the table %s, holds another kind of entry; move it aside and install again", place, tb.Name)
		case want == journal.EntryFolder:
			folders = append(folders, place)
		default:
			empty, refusals, err := store.RenderDocument(c.cat, tb.Name, place, nil, nil)
			if err != nil || len(refusals) > 0 {
				return nil, nil, nil, newError(codeInternal, "the empty table %s does not render: %v %v", tb.Name, err, refusals)
			}
			changes = append(changes, journal.Change{Path: place, Data: empty})
		}
		res.Created = append(res.Created, place)
	}
	kind, err := c.j.Entry(gitAttributes)
	if err != nil {
		return nil, nil, nil, driverError("reading "+gitAttributes, err)
	}
	if kind != journal.EntryAbsent && kind != journal.EntryFile {
		return nil, nil, nil, newError(codeConfig, "%s is no regular file; replace it with a file and install again", gitAttributes)
	}
	data, err := c.j.ReadFile(l, gitAttributes)
	if err != nil {
		return nil, nil, nil, driverError("reading "+gitAttributes, err)
	}
	if !hasLine(data, attributesLine) {
		next := slices.Clone(data)
		if len(next) > 0 && next[len(next)-1] != '\n' {
			next = append(next, '\n')
		}
		next = append(next, attributesLine+"\n"...)
		changes = append(changes, journal.Change{Path: gitAttributes, Data: next})
		res.Attributes = true
	}
	slices.Sort(res.Created)
	return res, changes, folders, nil
}

// hasLine reports whether a line of data, its line ending aside, is line.
func hasLine(data []byte, line string) bool {
	for _, l := range bytes.Split(data, []byte("\n")) {
		if string(bytes.TrimSuffix(l, []byte("\r"))) == line {
			return true
		}
	}
	return false
}
