package store

import (
	"bytes"
	"errors"
	"sync"

	"github.com/polyspec/flowmark/catalog"
)

// Memo keeps the parse of each corpus document across the reads of a long-lived corpus, so a read parses only the
// documents whose parse can differ from the last one. A parse is a function of the catalog, the table, the corpus
// path, the bytes and the link targets its Linker answered; the memo records each answer, and a document is parsed
// again when its bytes differ or when the Linker now answers one of its questions otherwise — a row whose detail
// section appeared, a key a second row now shares, a dangling link whose row now exists. The rows it returns are
// shared between reads and never changed: a write changes copies of them. A nil *Memo parses every time.
//
// Each read is one generation: Begin starts it, and End drops the documents it did not parse or reuse, so a removed
// document is not kept.
type Memo struct {
	mu   sync.Mutex
	cat  *catalog.Catalog
	gen  uint64
	docs map[string]*memoDoc
}

// memoDoc is the parse of one document: its inputs, the answers its Linker gave, and its results.
type memoDoc struct {
	table   string
	data    []byte
	answers []linkAnswer
	gen     uint64

	row   *Row
	rows  []*Row
	diags []Diagnostic
	err   error
}

// linkAnswer is a question a parse asked its Linker and the answer it got.
type linkAnswer struct {
	from, table, key string
	target           string
	err              string
	shared           bool
}

// NewMemo returns an empty memo.
func NewMemo() *Memo { return &Memo{docs: map[string]*memoDoc{}} }

// Begin starts the generation of a read with the catalog cat; a catalog other than the last one drops every parse.
func (m *Memo) Begin(cat *catalog.Catalog) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cat != cat {
		m.cat = cat
		m.docs = map[string]*memoDoc{}
	}
	m.gen++
}

// End drops the parses the generation did not use.
func (m *Memo) End() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for p, d := range m.docs {
		if d.gen != m.gen {
			delete(m.docs, p)
		}
	}
}

// recorder is a Linker that records each answer of lk.
type recorder struct {
	lk      Linker
	answers []linkAnswer
}

func (r *recorder) Target(fromFile, table, key string) (string, error) {
	target, err := r.lk.Target(fromFile, table, key)
	r.answers = append(r.answers, answerOf(fromFile, table, key, target, err))
	return target, err
}

func answerOf(from, table, key, target string, err error) linkAnswer {
	a := linkAnswer{from: from, table: table, key: key, target: target}
	if err != nil {
		a.err = err.Error()
		a.shared = errors.Is(err, ErrKeyShared)
	}
	return a
}

// reusable returns the parse of rel when its inputs are the same and lk answers each of its questions as it did.
func (m *Memo) reusable(table, rel string, data []byte, lk Linker) *memoDoc {
	d := m.docs[rel]
	if d == nil || d.table != table || !bytes.Equal(d.data, data) {
		return nil
	}
	for _, a := range d.answers {
		target, err := lk.Target(a.from, a.table, a.key)
		if answerOf(a.from, a.table, a.key, target, err) != a {
			return nil
		}
	}
	d.gen = m.gen
	return d
}

// ParseFile is ParseFile through the memo.
func (m *Memo) ParseFile(cat *catalog.Catalog, table, rel string, data []byte, lk Linker) (*Row, []Diagnostic) {
	if m == nil {
		return ParseFile(cat, table, rel, data, lk)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if d := m.reusable(table, rel, data, lk); d != nil {
		return d.row, d.diags
	}
	r := &recorder{lk: lk}
	row, diags := ParseFile(cat, table, rel, data, r)
	m.docs[rel] = &memoDoc{table: table, data: data, answers: r.answers, gen: m.gen, row: row, diags: diags}
	return row, diags
}

// ParseDocument is ParseDocument through the memo.
func (m *Memo) ParseDocument(cat *catalog.Catalog, table, rel string, data []byte, lk Linker) ([]*Row, []Diagnostic, error) {
	if m == nil {
		return ParseDocument(cat, table, rel, data, lk)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if d := m.reusable(table, rel, data, lk); d != nil {
		return d.rows, d.diags, d.err
	}
	r := &recorder{lk: lk}
	rows, diags, err := ParseDocument(cat, table, rel, data, r)
	m.docs[rel] = &memoDoc{table: table, data: data, answers: r.answers, gen: m.gen, rows: rows, diags: diags, err: err}
	return rows, diags, err
}
