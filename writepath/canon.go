package writepath

import (
	"context"
	"slices"

	"github.com/polyspec/flowmark/catalog"
	"github.com/polyspec/flowmark/irexec"
)

// CanonResult is what flowmark canon did: the files it rewrote, sorted, and, with no file named, the files it left as
// they are, each with the diagnostic that keeps it from being a target.
type CanonResult struct {
	Rewritten []string
	Reported  []irexec.CanonReport
}

// Canon runs flowmark canon (spec 40 The canon operation): it rewrites the named files, each a corpus path, or with
// none named every corpus document it can, to canonical form, as its own transaction under the lock. Its targets are
// exempt from the commit refusal of a file with a format.* diagnostic: every file it rewrites is one of them. A named
// file it cannot rewrite refuses the whole command, and nothing is written.
func (s *Session) Canon(ctx context.Context, files []string) (*CanonResult, error) {
	named := slices.Clone(files)
	// The result of the last computation: Rewrite runs it again on the corpus read again when its commit meets a change.
	var res *CanonResult
	compute := func(c *irexec.Corpus) ([]Rewrite, error) {
		targets, reports, err := c.Canon(named)
		if err != nil {
			return nil, err
		}
		r := &CanonResult{Rewritten: []string{}, Reported: reports}
		rws := make([]Rewrite, len(targets))
		for i, tg := range targets {
			rws[i] = Rewrite{Path: tg.Path, After: tg.After}
			r.Rewritten = append(r.Rewritten, tg.Path)
		}
		res = r
		return rws, nil
	}
	if _, err := s.Rewrite(ctx, compute, canonTargets); err != nil {
		return nil, err
	}
	return res, nil
}

// canonTargets is the exemption of canon: every file it rewrites is its target.
func canonTargets(string, catalog.Diagnostic) bool { return true }
