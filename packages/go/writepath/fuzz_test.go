package writepath

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// tagRow is a row of tags in the model of FuzzSession.
type tagRow struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// tagModel is the oracle of FuzzSession: the committed rows of tags, and, inside a transaction, its rows, whether it
// is read-only, and its savepoint stack, each with the rows it was set at.
type tagModel struct {
	committed []tagRow
	inTx      bool
	readOnly  bool
	cur       []tagRow
	marks     []tagMark
}

// tagMark is a savepoint of the model: its name and the rows it was set at.
type tagMark struct {
	name string
	rows []tagRow
}

// visible returns the rows a request of the session reads.
func (m *tagModel) visible() []tagRow {
	if m.inTx {
		return m.cur
	}
	return m.committed
}

// set gives the session's rows the value rows.
func (m *tagModel) set(rows []tagRow) {
	if m.inTx {
		m.cur = rows
	} else {
		m.committed = rows
	}
}

// insert models the insert of a tag: a label held by another row is DUPLICATE_KEY, which the uniques report before
// the check; a kind outside the check is CONSTRAINT; the key is max + 1 over the rows.
func (m *tagModel) insert(label, kind string) string {
	if m.inTx && m.readOnly {
		return "READ_ONLY"
	}
	rows := m.visible()
	if slices.ContainsFunc(rows, func(r tagRow) bool { return r.Label == label }) {
		return "DUPLICATE_KEY"
	}
	if kind != "topic" && kind != "area" {
		return "CONSTRAINT"
	}
	id := 0
	for _, r := range rows {
		id = max(id, r.ID)
	}
	m.set(append(slices.Clone(rows), tagRow{ID: id + 1, Label: label, Kind: kind}))
	return ""
}

func (m *tagModel) delete(id int) string {
	if m.inTx && m.readOnly {
		return "READ_ONLY"
	}
	m.set(slices.DeleteFunc(slices.Clone(m.visible()), func(r tagRow) bool { return r.ID == id }))
	return ""
}

func (m *tagModel) update(id int, kind string) string {
	if m.inTx && m.readOnly {
		return "READ_ONLY"
	}
	rows := slices.Clone(m.visible())
	i := slices.IndexFunc(rows, func(r tagRow) bool { return r.ID == id })
	if i < 0 {
		return ""
	}
	if kind != "topic" && kind != "area" {
		return "CONSTRAINT"
	}
	rows[i].Kind = kind
	m.set(rows)
	return ""
}

func (m *tagModel) find(name string) int {
	for i := len(m.marks) - 1; i >= 0; i-- {
		if m.marks[i].name == name {
			return i
		}
	}
	return -1
}

// FuzzSession runs a sequence of write requests and control messages, read from the fuzzer's bytes as pairs of an
// operation and its argument, through one session, against a model of the tags rows with the transaction and its
// savepoint stack. Every operation fails with the code the model gives, or succeeds, and the rows the session then
// reads are the model's; at the end the corpus validates clean, with no stage and no lock left.
func FuzzSession(f *testing.F) {
	for _, seed := range [][]byte{
		{0, 1, 9, 0},
		{2, 1, 0, 2, 5, 0, 0, 3, 6, 0, 3, 0},
		{2, 1, 0, 2, 4, 0},
		{2, 0, 0, 2, 3, 0},
		{0, 2, 0, 2, 1, 1, 8, 3},
		{2, 1, 5, 0, 5, 1, 7, 0, 6, 1, 0, 4, 3, 0},
		{3, 0, 6, 0, 2, 1, 2, 1, 4, 0},
		{2, 1, 5, 0, 0, 1, 5, 1, 7, 0, 6, 1, 3, 0},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 32 {
			ops = ops[:32]
		}
		fx := baseFixture(t)
		s := fx.session("fuzz")
		ctx := context.Background()
		m := &tagModel{committed: []tagRow{{1, "go", "topic"}, {2, "ops", "area"}}}
		kinds := []string{"topic", "area", "other"}
		for i := 0; i+1 < len(ops); i += 2 {
			op, arg := ops[i]%10, int(ops[i+1])
			var err error
			var code string
			switch op {
			case 0:
				label, kind := fmt.Sprintf("l%d", arg%8), kinds[arg%3]
				code = m.insert(label, kind)
				_, err = s.Exec(ctx, fx.insertTag(label, kind))
			case 1:
				code = m.delete(arg%6 + 1)
				_, err = s.Exec(ctx, fx.msg("delete", "tags", map[string]any{"where": corpustest.WhereID(0)}, arg%6+1))
			case 2:
				if m.inTx {
					code = "CONFIG"
				} else {
					m.inTx, m.readOnly, m.cur, m.marks = true, arg%4 == 0, slices.Clone(m.committed), nil
				}
				err = s.Begin(ctx, arg%4 == 0)
			case 3:
				if m.inTx {
					m.committed, m.inTx = m.cur, false
				} else {
					code = "CONFIG"
				}
				err = s.Commit()
			case 4:
				if m.inTx {
					m.inTx = false
				} else {
					code = "CONFIG"
				}
				err = s.Rollback()
			case 5:
				name := fmt.Sprintf("s%d", arg%3)
				if m.inTx {
					m.marks = slices.DeleteFunc(m.marks, func(k tagMark) bool { return k.name == name })
					m.marks = append(m.marks, tagMark{name, m.cur})
				} else {
					code = "CONFIG"
				}
				err = s.Savepoint(name)
			case 6:
				name := fmt.Sprintf("s%d", arg%3)
				if j := m.find(name); !m.inTx || j < 0 {
					code = "CONFIG"
				} else {
					m.cur, m.marks = m.marks[j].rows, m.marks[:j+1]
				}
				err = s.RollbackTo(name)
			case 7:
				name := fmt.Sprintf("s%d", arg%3)
				if j := m.find(name); !m.inTx || j < 0 {
					code = "CONFIG"
				} else {
					m.marks = m.marks[:j]
				}
				err = s.Release(name)
			case 8:
				id, kind := arg%6+1, kinds[arg%3]
				code = m.update(id, kind)
				_, err = s.Exec(ctx, fx.msg("update", "tags", map[string]any{"set": sets(0, "kind"), "where": corpustest.WhereID(1)}, kind, id))
			case 9:
			}
			if got := codeOf(err); got != code {
				t.Fatalf("op %d(%d) failed with %v; the model says %q", op, arg, err, code)
			}
			a, err := s.Exec(ctx, fx.msg("all", "tags", nil))
			if err != nil {
				t.Fatalf("reading the tags: %v", err)
			}
			b, err := json.Marshal(a)
			if err != nil {
				t.Fatal(err)
			}
			var got struct{ Rows []tagRow }
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if want := m.visible(); !slices.Equal(got.Rows, want) && (len(got.Rows) != 0 || len(want) != 0) {
				t.Fatalf("after op %d(%d) the session reads %+v; the model holds %+v", op, arg, got.Rows, want)
			}
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		fx.clean()
		if _, err := os.Lstat(filepath.Join(fx.root, ".flowmark", "lock")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("the lock is left: %v", err)
		}
	})
}
