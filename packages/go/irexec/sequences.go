package irexec

import (
	"bytes"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/polyspec/flowmark/packages/go/catalog"
)

// The AUTOINCREMENT sequences of a corpus are part of it, in catalog.SequencesFile: the sequence of a table is the one
// SQLite keeps in sqlite_sequence, the row of the table there or none, and the file names the tables whose sequence is
// not the one their rows imply — their largest key, or no row when they hold none (plan, A refresh of the changed rows).
// A write changes a sequence as SQLite changes it: an insert raises it to each key it gives or meets, a delete or a
// change of a key leaves it as it is, and an explicit write of sqlite_sequence by a statement of SQL sets it, a lower
// value included (SetSequences). The key an insert gives is one more than the larger of the sequence and the largest
// key, so no key is given twice, a key written by hand included, as SQLite's AUTOINCREMENT gives none. The commit
// writes the file with the write.

// sequences are the sequences of a corpus that its file names and the bytes of the file they were read from or are
// written as, nil when the corpus has no file; a nil sequence is a table of no row of sqlite_sequence. A corpus never
// changes the sequences it shares.
type sequences struct {
	data     []byte
	counters map[string]*int64
}

// SetSequenceFile gives the corpus c, which a read is computing and no other reads, the sequences of the bytes data of
// catalog.SequencesFile, nil when the corpus has no such file.
func (c *Corpus) SetSequenceFile(data []byte) error {
	counters, err := catalog.ParseSequences(data)
	if err != nil {
		return err
	}
	c.seq = &sequences{data: slices.Clone(data), counters: counters}
	return nil
}

// sequenceData returns the bytes of the sequences' file of c, nil when it has none.
func (c *Corpus) sequenceData() []byte {
	if c.seq == nil {
		return nil
	}
	return c.seq.data
}

// largestKey returns the largest positive key of the table, 0 when it holds none.
func (c *Corpus) largestKey(table string) int64 {
	tb, ok := c.cat.Table(table)
	if !ok || tb.Key == "" {
		return 0
	}
	d := c.maxNumber(table, tb.Key)
	if d == "" {
		return 0
	}
	// The index holds the canonical digits of a positive int64 (indexKeys).
	m, err := strconv.ParseInt(d, 10, 64)
	if err != nil {
		panic("irexec: the number index holds " + d + ", which is no int64")
	}
	return m
}

// Counter returns the sequence SQLite keeps in sqlite_sequence for the AUTOINCREMENT table: its value and true for a
// row, false for none. A table the file names has the sequence the file gives it; another has its largest key, or no
// row when it holds none.
func (c *Corpus) Counter(table string) (int64, bool) {
	if c.seq != nil {
		if n, named := c.seq.counters[table]; named {
			if n == nil {
				return 0, false
			}
			return *n, true
		}
	}
	m := c.largestKey(table)
	return m, m > 0
}

// Sequence returns the number past which the AUTOINCREMENT table gives keys: the larger of its sequence and its largest
// key, 0 when both are below 1. The key the table gives next is one more, as SQLite's AUTOINCREMENT gives it.
func (c *Corpus) Sequence(table string) int64 {
	n, _ := c.Counter(table)
	return max(n, c.largestKey(table), 0)
}

// sequenceOf is the sequence of one table, as Corpus.Counter returns it.
type sequenceOf struct {
	n    int64
	held bool
}

// raise raises the sequence to the key k, as an insert of SQLite that gives or meets k does: a table of no row of
// sqlite_sequence takes k, or 0 when k is lower, as SQLite starts from 0.
func (s *sequenceOf) raise(k int64) {
	if !s.held {
		s.n, s.held = 0, true
	}
	s.n = max(s.n, k)
}

// insertSequence returns the sequence of the AUTOINCREMENT table tb while the write w runs, which each row the write
// gives or meets in it raises.
func (w *writer) insertSequence(tb *catalog.Table) *sequenceOf {
	if s := w.seqs[tb.Name]; s != nil {
		return s
	}
	n, held := w.c.Counter(tb.Name)
	s := &sequenceOf{n: n, held: held}
	if w.seqs == nil {
		w.seqs = map[string]*sequenceOf{}
	}
	w.seqs[tb.Name] = s
	return s
}

// attempted raises the sequence of the AUTOINCREMENT table tb to the key k of a row the write w gives or meets in it;
// a key of no integer, which a fault of the write refuses, raises nothing.
func (w *writer) attempted(tb *catalog.Table, k cell) {
	if !tb.Autoincrement || k.null {
		return
	}
	if n, err := strconv.ParseInt(k.text, 10, 64); err == nil {
		w.insertSequence(tb).raise(n)
	}
}

// writtenSequences returns the target of the sequences' file of the next corpus of the write w, nil when its bytes
// stay as they are: a table the write inserted into takes the sequence the write raised, and every other
// AUTOINCREMENT table keeps the sequence it had, which its rows of the next corpus may no longer imply.
func (w *writer) writtenSequences() *Target {
	held := map[string]int64{}
	for _, tb := range w.c.cat.Tables {
		if !tb.Autoincrement {
			continue
		}
		s := sequenceOf{}
		if raised := w.seqs[tb.Name]; raised != nil {
			s = *raised
		} else {
			s.n, s.held = w.c.Counter(tb.Name)
		}
		if s.held {
			held[tb.Name] = s.n
		}
	}
	return w.next.setSequences(w.c, held)
}

// setSequences gives the corpus c, read or written from before, the rows held of sqlite_sequence, one sequence for
// each AUTOINCREMENT table that has a row there, and returns the target of the sequences' file, nil when the file
// names the same sequences, whose bytes then stay as they are. The file names a table whose sequence is not the one its
// rows in c imply; it keeps the names of the tables that keep no sequence as they are.
func (c *Corpus) setSequences(before *Corpus, held map[string]int64) *Target {
	var was map[string]*int64
	if c.seq != nil {
		was = c.seq.counters
	}
	counters := maps.Clone(was)
	if counters == nil {
		counters = map[string]*int64{}
	}
	for _, tb := range c.cat.Tables {
		if !tb.Autoincrement {
			continue
		}
		m := c.largestKey(tb.Name)
		n, ok := held[tb.Name]
		switch {
		case ok && m > 0 && n == m, !ok && m == 0:
			delete(counters, tb.Name)
		case ok:
			counters[tb.Name] = &n
		default:
			counters[tb.Name] = nil
		}
	}
	if maps.EqualFunc(counters, was, func(a, b *int64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }) && sameSequenceData(c, before) {
		return nil
	}
	data := catalog.RenderSequences(counters)
	prev := before.sequenceData()
	if bytes.Equal(data, prev) && (data == nil) == (prev == nil) {
		return nil
	}
	c.seq = &sequences{data: data, counters: counters}
	return &Target{Path: catalog.SequencesFile, Before: prev, After: data}
}

// sameSequenceData reports whether the corpora a and b hold the same bytes of the sequences' file, or both none.
func sameSequenceData(a, b *Corpus) bool {
	x, y := a.sequenceData(), b.sequenceData()
	return bytes.Equal(x, y) && (x == nil) == (y == nil)
}

// SetSequences gives the corpus the rows held of sqlite_sequence that a statement of SQL left in its database, one
// sequence for each AUTOINCREMENT table that has a row there, as SQLite keeps them: a key the database gave past the
// sequence of the corpus, as a key a statement inserted and deleted again, and an explicit write of sqlite_sequence, a
// lower value and a deleted row included. A table of held that keeps no sequence is refused. The result has no
// post-image when the sequences' file stays as it is.
func (c *Corpus) SetSequences(held map[string]int64) (*Result, error) {
	for table := range held {
		if tb, ok := c.cat.Table(table); !ok || !tb.Autoincrement || tb.Name != table {
			return nil, errf(codeInternal, "the sequence of %s, which is no AUTOINCREMENT table", table)
		}
	}
	next := c.fork()
	tg := next.setSequences(c, held)
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
