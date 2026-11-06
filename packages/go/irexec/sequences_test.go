package irexec

import (
	"slices"
	"strconv"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
)

// TestSequenceRaise pins the AUTOINCREMENT counter a write keeps (sequences.go): a write that lowers the sequence of an
// AUTOINCREMENT table — a delete or a key change of the row of its largest key — raises its counter, the commit writes
// it to dbspec/sequences.json, and the next key the table gives is one more than its sequence; a write that leaves the
// sequence as it is changes no counter. The tags of the test corpus hold the keys 1 and 2.
func TestSequenceRaise(t *testing.T) {
	label := 0
	insert := func(f *fixture) []byte {
		label++
		return f.msg("insert", "tags", map[string]any{"set": sets("label", "kind")}, "label"+string(rune('a'+label)), "topic")
	}
	// load inserts a tag of the key id, as a load of orm (PRT-26) gives a row its identity.
	load := func(f *fixture, id int, label string) []byte {
		return f.msg("insert", "tags", map[string]any{"set": sets("id", "label", "kind"), "load": true}, id, label, "topic")
	}
	del := func(f *fixture, id int) []byte {
		return f.msg("delete", "tags", map[string]any{"where": where(pred("id", "eq", p(0)))}, id)
	}
	cases := []struct {
		name string
		// counters is the file of the counters before the steps, "" for none.
		counters string
		steps    func(f *fixture)
		// want is the file of the counters after the steps, "" for none, and next the key of the insert after them.
		want string
		next int
	}{
		{"a delete of the largest key", "", func(f *fixture) {
			f.commit(f.want(insert(f), `{"affected":1,"keys":[3]}`))
			f.commit(f.ok(del(f, 3)))
		}, "{\n  \"tags\": 3\n}\n", 4},
		{"a delete of a key below the largest", "", func(f *fixture) {
			f.commit(f.ok(del(f, 1)))
		}, "", 3},
		{"a key loaded past the largest, then deleted", "", func(f *fixture) {
			f.commit(f.want(load(f, 10, "ten"), `{"affected":1,"keys":[10]}`))
			f.commit(f.ok(del(f, 10)))
		}, "{\n  \"tags\": 10\n}\n", 11},
		{"every row deleted", "", func(f *fixture) {
			f.commit(f.ok(del(f, 2)))
			f.commit(f.ok(del(f, 1)))
		}, "{\n  \"tags\": 2\n}\n", 3},
		{"a counter below the largest key", "{\"tags\": 1}", func(f *fixture) {
			f.commit(f.ok(del(f, 1)))
		}, "{\"tags\": 1}", 3},
		{"a counter past the largest key", "{\"tags\": 5}", func(f *fixture) {
			f.commit(f.ok(del(f, 2)))
		}, "{\"tags\": 5}", 6},
		{"a key written by hand past the counter", "{\"tags\": 5}", func(f *fixture) {
			f.commit(f.want(load(f, 9, "nine"), `{"affected":1,"keys":[9]}`))
		}, "{\"tags\": 5}", 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := baseFixture(t)
			if tc.counters != "" {
				corpustest.WriteFiles(t, f.root, map[string]string{catalog.SequencesFile: tc.counters})
				f.reopen()
			}
			tc.steps(f)
			if got := string(f.c.Data(catalog.SequencesFile)); got != tc.want {
				t.Fatalf("the counters are %q, want %q", got, tc.want)
			}
			f.want(insert(f), `{"affected":1,"keys":[`+strconv.Itoa(tc.next)+`]}`)
		})
	}
}

// TestRaiseSequences pins the raise a statement of SQL asks for: a value past the sequence of its table is the new
// counter, in a post-image whose target is the counters' file and whose corpus differs at it; a value at or below the
// sequence asks for no write; a table that keeps no counter is refused. The corpus raised from is unchanged.
func TestRaiseSequences(t *testing.T) {
	f := baseFixture(t)
	for _, n := range []int64{1, 2} {
		res, err := f.c.RaiseSequences(map[string]int64{"tags": n})
		if err != nil || res.Post != nil {
			t.Fatalf("a raise to %d, at or below the sequence 2: %+v, %v", n, res, err)
		}
	}
	res, err := f.c.RaiseSequences(map[string]int64{"tags": 7})
	if err != nil || res.Post == nil {
		t.Fatalf("a raise to 7: %+v, %v", res, err)
	}
	want := "{\n  \"tags\": 7\n}\n"
	if tgs := res.Post.Targets; len(tgs) != 1 || tgs[0].Path != catalog.SequencesFile || tgs[0].Before != nil || string(tgs[0].After) != want {
		t.Fatalf("the targets of the raise are %+v", tgs)
	}
	if got := res.Post.Next.Sequence("tags"); got != 7 {
		t.Fatalf("the raised corpus has the sequence %d", got)
	}
	if got := f.c.Sequence("tags"); got != 2 {
		t.Fatalf("the corpus raised from has the sequence %d", got)
	}
	if got := f.c.Differs(res.Post.Next); !slices.Equal(got, []string{catalog.SequencesFile}) {
		t.Fatalf("the raised corpus differs at %v", got)
	}
	f.commit(res)
	if got := string(f.c.Data(catalog.SequencesFile)); got != want {
		t.Fatalf("the committed counters are %q", got)
	}
	if _, err := f.c.RaiseSequences(map[string]int64{"items": 3}); err == nil {
		t.Fatal("a raise of a table that keeps no counter is not refused")
	}
}
