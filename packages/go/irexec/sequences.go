package irexec

import (
	"bytes"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// The AUTOINCREMENT counters of a corpus are part of it, in catalog.SequencesFile: the counter of a table is the largest
// key it gave or held that none of its rows holds now, and the next key of the table is one more than its sequence, the
// larger of its counter and its largest key, so no key is given twice, a key written by hand included, as SQLite's
// AUTOINCREMENT gives none (plan, A refresh of the changed rows). A write that would lower a sequence — a delete or a
// key change of the row of its largest key — raises the counter to it, and the commit writes the file with the write.

// sequences are the counters of a corpus and the bytes of the file they were read from or are written as, nil when the
// corpus has no file. A corpus never changes the sequences it shares.
type sequences struct {
	data     []byte
	counters map[string]int64
}

// SetSequences gives the corpus c, which a read is computing and no other reads, the counters of the bytes data of
// catalog.SequencesFile, nil when the corpus has no such file.
func (c *Corpus) SetSequences(data []byte) error {
	counters, err := catalog.ParseSequences(data)
	if err != nil {
		return err
	}
	c.seq = &sequences{data: slices.Clone(data), counters: counters}
	return nil
}

// sequenceData returns the bytes of the counters' file of c, nil when it has none.
func (c *Corpus) sequenceData() []byte {
	if c.seq == nil {
		return nil
	}
	return c.seq.data
}

// Sequence returns the sequence of the AUTOINCREMENT table: the larger of its counter and its largest key, 0 when it has
// neither. The key the table gives next is one more.
func (c *Corpus) Sequence(table string) int64 {
	var n int64
	if c.seq != nil {
		n = c.seq.counters[table]
	}
	tb, ok := c.cat.Table(table)
	if !ok || tb.Key == "" {
		return n
	}
	if d := c.maxNumber(table, tb.Key); d != "" {
		// The index holds the canonical digits of a positive int64 (indexKeys).
		m, err := strconv.ParseInt(d, 10, 64)
		if err != nil {
			panic("irexec: the number index holds " + d + ", which is no int64")
		}
		n = max(n, m)
	}
	return n
}

// raiseSequences raises the counter of each AUTOINCREMENT table of the next corpus of the write w to its sequence in
// the corpus the write read when the write lowered it, and returns the target of the counters' file, nil when no
// counter rose.
func (w *writer) raiseSequences() *Target {
	raise := map[string]int64{}
	for _, tb := range w.c.cat.Tables {
		if !tb.Autoincrement {
			continue
		}
		if was := w.c.Sequence(tb.Name); w.next.Sequence(tb.Name) < was {
			raise[tb.Name] = was
		}
	}
	return w.next.raise(w.c, raise)
}

// raise sets the counters raise of the corpus c, read or written from before, and returns the target of the counters'
// file, nil when the bytes of the file stay as they are.
func (c *Corpus) raise(before *Corpus, raise map[string]int64) *Target {
	if len(raise) == 0 {
		return nil
	}
	counters := map[string]int64{}
	if c.seq != nil {
		maps.Copy(counters, c.seq.counters)
	}
	maps.Copy(counters, raise)
	data := catalog.RenderSequences(counters)
	prev := before.sequenceData()
	if bytes.Equal(data, prev) && (data == nil) == (prev == nil) {
		return nil
	}
	c.seq = &sequences{data: data, counters: counters}
	return &Target{Path: catalog.SequencesFile, Before: prev, After: data}
}

// RaiseSequences raises the counter of each AUTOINCREMENT table of raise whose sequence in c is lower to its value: a
// statement of SQL gives the corpus the counters its database gave keys past, as a key it inserted and deleted again.
// The result has no post-image when no counter rose.
func (c *Corpus) RaiseSequences(raise map[string]int64) (*Result, error) {
	rise := map[string]int64{}
	for table, n := range raise {
		tb, ok := c.cat.Table(table)
		if !ok || !tb.Autoincrement {
			return nil, errf(codeInternal, "the counter of %s, which is no AUTOINCREMENT table", table)
		}
		if n > c.Sequence(table) {
			rise[table] = n
		}
	}
	next := c.fork()
	tg := next.raise(c, rise)
	if tg == nil {
		return &Result{Answer: &Answer{}}, nil
	}
	return &Result{Answer: &Answer{}, Post: &PostImage{Targets: []Target{*tg}, Next: next}}, nil
}

// withTarget adds the target tg to targets, which are sorted by path, in its place.
func withTarget(targets []Target, tg *Target) []Target {
	if tg == nil {
		return targets
	}
	at, _ := slices.BinarySearchFunc(targets, tg.Path, func(t Target, p string) int { return strings.Compare(t.Path, p) })
	return slices.Insert(targets, at, *tg)
}
