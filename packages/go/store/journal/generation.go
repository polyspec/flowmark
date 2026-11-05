package journal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
)

// GenerationPath is the declared place of the commit generation (spec 40 The commit generation): the number of the
// commits of the corpus, and the paths of each of the last Generations commits. A commit writes it at its commit
// record, under the lock, and it is replaced whole by a rename, so a reader never sees a part of it.
const GenerationPath = ".flowmark/generation"

// Generations is the number of commits whose paths the generation keeps; a corpus read at an older generation learns
// no paths from it and reads the whole tree again.
const Generations = 64

// Generation is the content of the generation file: N is the number of the last commit, 0 for a corpus that no commit
// of this protocol wrote, and Commits the paths of the last Generations commits, oldest first, the last of number N.
type Generation struct {
	N       uint64           `json:"generation"`
	Commits []GenerationCommit `json:"commits"`
}

// GenerationCommit is one commit of a Generation: its number and the corpus paths it wrote or deleted, sorted.
type GenerationCommit struct {
	N     uint64   `json:"generation"`
	Paths []string `json:"paths"`
}

// commitRecord is the content of a stage's commit record: the generation the commit takes. A record written before
// the generation existed holds none, and the recovery that collects it gives it the next one.
type commitRecord struct {
	Generation uint64 `json:"generation,omitempty"`
}

// Since returns the paths the commits after generation n wrote, sorted and without repeats, and true; it returns false
// when a commit after n is no longer kept, or n is after the generation, so the caller reads the whole tree again.
func (g Generation) Since(n uint64) ([]string, bool) {
	if n > g.N {
		return nil, false
	}
	if n == g.N {
		return []string{}, true
	}
	if len(g.Commits) == 0 || g.Commits[0].N > n+1 {
		return nil, false
	}
	var out []string
	for _, c := range g.Commits {
		if c.N > n {
			out = append(out, c.Paths...)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), true
}

// next returns the generation that follows g with the commit of number n and paths, keeping the last Generations
// commits.
func (g Generation) next(n uint64, paths []string) Generation {
	sorted := slices.Clone(paths)
	slices.Sort(sorted)
	commits := append(slices.Clone(g.Commits), GenerationCommit{N: n, Paths: slices.Compact(sorted)})
	if len(commits) > Generations {
		commits = commits[len(commits)-Generations:]
	}
	return Generation{N: n, Commits: commits}
}

// Generation reads the commit generation of the corpus; an absent file is generation 0 with no commit. It takes no
// lock: the file is replaced whole, so a read sees one generation or the next.
func (c *Corpus) Generation() (Generation, error) {
	info, err := c.lstat(GenerationPath)
	if err != nil {
		return Generation{}, fmt.Errorf("journal: %w", err)
	}
	if info == nil {
		return Generation{Commits: []GenerationCommit{}}, nil
	}
	if !info.Mode().IsRegular() {
		return Generation{}, fmt.Errorf("journal: %s is not a regular file", c.abs(GenerationPath))
	}
	data, _, err := readShared(c.abs(GenerationPath))
	if errors.Is(err, fs.ErrNotExist) {
		return Generation{Commits: []GenerationCommit{}}, nil
	}
	if err != nil {
		return Generation{}, fmt.Errorf("journal: %w", err)
	}
	return parseGeneration(c.abs(GenerationPath), data)
}

// parseGeneration reads and checks the generation file at p: one object, its commits numbered in order up to N, each
// path a clean corpus path.
func parseGeneration(p string, data []byte) (Generation, error) {
	var g Generation
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&g); err != nil {
		return Generation{}, fmt.Errorf("journal: %s holds no generation: %w", p, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Generation{}, fmt.Errorf("journal: %s holds more than one generation", p)
	}
	if g.Commits == nil || len(g.Commits) > Generations || (g.N == 0) != (len(g.Commits) == 0) {
		return Generation{}, fmt.Errorf("journal: %s holds %d commits at generation %d; a generation after 0 holds 1 to %d", p, len(g.Commits), g.N, Generations)
	}
	for i, cm := range g.Commits {
		if cm.N != g.N-uint64(len(g.Commits)-1-i) || cm.N == 0 {
			return Generation{}, fmt.Errorf("journal: %s holds the commit %d out of the order up to %d", p, cm.N, g.N)
		}
		for _, rel := range cm.Paths {
			if err := checkTarget(rel); err != nil {
				return Generation{}, fmt.Errorf("journal: %s: commit %d: %w", p, cm.N, err)
			}
		}
	}
	return g, nil
}

// commitGeneration returns the generation the commit of a stage takes: the one its commit record holds, or the next
// after the corpus's when the stage has none yet or a record without one.
func (c *Corpus) commitGeneration(stage string) (uint64, error) {
	ok, err := regularExists(filepath.Join(stage, commitName))
	if err != nil {
		return 0, err
	}
	if ok {
		rec, err := readCommit(stage)
		if err != nil {
			return 0, err
		}
		if rec.Generation != 0 {
			return rec.Generation, nil
		}
	}
	g, err := c.Generation()
	if err != nil {
		return 0, err
	}
	return g.N + 1, nil
}

// readCommit reads and checks the commit record of a stage.
func readCommit(stage string) (commitRecord, error) {
	p := filepath.Join(stage, commitName)
	data, _, err := readShared(p)
	if err != nil {
		return commitRecord{}, fmt.Errorf("journal: %w", err)
	}
	var rec commitRecord
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rec); err != nil {
		return commitRecord{}, fmt.Errorf("journal: %s holds no commit record: %w", p, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return commitRecord{}, fmt.Errorf("journal: %s holds more than one commit record", p)
	}
	return rec, nil
}

// publishGeneration makes n, with the paths of its renames, the generation of the corpus when it is after the one
// written; written again for the same commit it changes nothing, so a recovery that collects a stage after its
// generation was written leaves it as it is.
func (c *Corpus) publishGeneration(n uint64, renames []rename) error {
	g, err := c.Generation()
	if err != nil {
		return err
	}
	if g.N >= n {
		return nil
	}
	if g.N+1 != n {
		// The generation file was removed or replaced by hand after the commit record was written: the commits between
		// are not known, so the generation keeps this commit alone and a corpus read before it reads the whole tree.
		g = Generation{N: n - 1}
	}
	paths := make([]string, len(renames))
	for i, r := range renames {
		paths[i] = r.Path
	}
	data, err := json.Marshal(g.next(n, paths))
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if err := writeDurable(c.abs(".flowmark"), "generation", append(data, '\n')); err != nil {
		return err
	}
	c.obs.Observe(Event{Kind: Generated, Path: c.abs(GenerationPath), Detail: strconv.FormatUint(n, 10)})
	return nil
}
