package irexec

import (
	"slices"
	"strconv"
	"testing"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/flowmark/packages/go/internal/corpustest"
)

// TestSequenceWrites pins the AUTOINCREMENT sequence a write keeps (sequences.go), as SQLite keeps it in
// sqlite_sequence: an insert raises it to each key it gives or meets, a meeting of a row on a unique key included; a
// delete leaves it as it is, so a delete of the row of the largest key names the sequence in dbspec/sequences.json,
// and a sequence the file names below the largest key stays there; the file names a table only while its sequence is
// not its largest key. The next key is one more than the larger of the sequence and the largest key; past the largest
// int64 there is none, as SQLite fails it. The tags of the test corpus hold the keys 1 and 2, and the label go is 1.
func TestSequenceWrites(t *testing.T) {
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
	upsert := func(f *fixture) []byte {
		return f.msg("insert", "tags", map[string]any{"set": sets("label", "kind"), "on_duplicate": []any{set("kind", 2)}}, "go", "topic", "area")
	}
	cases := []struct {
		name string
		// counters is the file of the sequences before the steps, "" for none.
		counters string
		steps    func(f *fixture)
		// want is the file of the sequences after the steps, "" for none, and next the key of the insert after them.
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
		{"a sequence below the largest key", "{\"tags\": 1}", func(f *fixture) {
			f.commit(f.ok(del(f, 1)))
		}, "{\"tags\": 1}", 3},
		{"a delete of the largest key down to a sequence below it", "{\"tags\": 1}", func(f *fixture) {
			f.commit(f.ok(del(f, 2)))
		}, "", 2},
		{"an insert after a sequence below the largest key", "{\"tags\": 1}", func(f *fixture) {
			f.commit(f.want(insert(f), `{"affected":1,"keys":[3]}`))
		}, "", 4},
		{"a sequence past the largest key", "{\"tags\": 5}", func(f *fixture) {
			f.commit(f.ok(del(f, 2)))
		}, "{\"tags\": 5}", 6},
		{"a key written by hand past the sequence", "{\"tags\": 5}", func(f *fixture) {
			f.commit(f.want(load(f, 9, "nine"), `{"affected":1,"keys":[9]}`))
		}, "", 10},
		{"a key written by hand below the sequence", "{\"tags\": 5}", func(f *fixture) {
			f.commit(f.want(load(f, 4, "four"), `{"affected":1,"keys":[4]}`))
		}, "{\"tags\": 5}", 6},
		{"no row of sqlite_sequence", "{\"tags\": null}", func(f *fixture) {
			f.commit(f.ok(del(f, 2)))
		}, "{\"tags\": null}", 2},
		{"a key written by hand below the largest after no row", "{\"tags\": null}", func(f *fixture) {
			f.commit(f.ok(del(f, 1)))
			f.commit(f.want(load(f, 1, "one"), `{"affected":1,"keys":[1]}`))
		}, "{\n  \"tags\": 1\n}\n", 3},
		{"a zero sequence", "{\"tags\": 0}", func(f *fixture) {
			f.commit(f.ok(del(f, 2)))
			f.commit(f.ok(del(f, 1)))
		}, "{\"tags\": 0}", 1},
		{"a negative sequence", "{\"tags\": -5}", func(f *fixture) {}, "{\"tags\": -5}", 3},
		{"an insert that meets a row on a unique key takes the key it would give", "", func(f *fixture) {
			f.commit(f.want(upsert(f), `{"affected":1,"keys":[1]}`))
		}, "{\n  \"tags\": 3\n}\n", 4},
		{"an insert that meets a row after a sequence below the largest key", "{\"tags\": 1}", func(f *fixture) {
			f.commit(f.want(upsert(f), `{"affected":1,"keys":[1]}`))
		}, "{\n  \"tags\": 3\n}\n", 4},
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
				t.Fatalf("the sequences are %q, want %q", got, tc.want)
			}
			f.want(insert(f), `{"affected":1,"keys":[`+strconv.Itoa(tc.next)+`]}`)
		})
	}
	t.Run("no key past the largest int64", func(t *testing.T) {
		f := baseFixture(t)
		corpustest.WriteFiles(t, f.root, map[string]string{catalog.SequencesFile: "{\"tags\": 9223372036854775807}"})
		f.reopen()
		f.fails(insert(f), "DRIVER")
		f.fails(upsert(f), "DRIVER")
		f.commit(f.want(load(f, 3, "three"), `{"affected":1,"keys":[3]}`))
	})
}

// TestSetSequences pins the rows of sqlite_sequence a statement of SQL leaves: each is the sequence of its table, a
// lower value, zero, a negative value and a deleted row included, in a post-image whose target is the sequences' file
// and whose corpus differs at it; the sequences the corpus already holds ask for no write; a table that keeps no
// sequence is refused. The corpus set from is unchanged. The tags of the test corpus hold the keys 1 and 2.
func TestSetSequences(t *testing.T) {
	cases := []struct {
		name string
		held map[string]int64
		// want is the file after the set, "" for none, and counter the row of sqlite_sequence of tags.
		want    string
		counter int64
		row     bool
		next    int64
	}{
		{"past the largest key", map[string]int64{"tags": 7}, "{\n  \"tags\": 7\n}\n", 7, true, 7},
		{"below the largest key", map[string]int64{"tags": 1}, "{\n  \"tags\": 1\n}\n", 1, true, 2},
		{"zero", map[string]int64{"tags": 0}, "{\n  \"tags\": 0\n}\n", 0, true, 2},
		{"negative", map[string]int64{"tags": -3}, "{\n  \"tags\": -3\n}\n", -3, true, 2},
		{"no row", nil, "{\n  \"tags\": null\n}\n", 0, false, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := baseFixture(t)
			res, err := f.c.SetSequences(heldOf(f.c, tc.held))
			if err != nil || res.Post == nil {
				t.Fatalf("a set to %v: %+v, %v", tc.held, res, err)
			}
			if tgs := res.Post.Targets; len(tgs) != 1 || tgs[0].Path != catalog.SequencesFile || tgs[0].Before != nil || string(tgs[0].After) != tc.want {
				t.Fatalf("the targets of the set are %+v", tgs)
			}
			if n, row := res.Post.Next.Counter("tags"); n != tc.counter || row != tc.row {
				t.Fatalf("the set corpus has the sequence %d, %v", n, row)
			}
			if got := res.Post.Next.Sequence("tags"); got != tc.next {
				t.Fatalf("the set corpus gives keys past %d", got)
			}
			if n, row := f.c.Counter("tags"); n != 2 || !row {
				t.Fatalf("the corpus set from has the sequence %d, %v", n, row)
			}
			if got := f.c.Differs(res.Post.Next); !slices.Equal(got, []string{catalog.SequencesFile}) {
				t.Fatalf("the set corpus differs at %v", got)
			}
			f.commit(res)
			if got := string(f.c.Data(catalog.SequencesFile)); got != tc.want {
				t.Fatalf("the committed sequences are %q", got)
			}
		})
	}
	f := baseFixture(t)
	res, err := f.c.SetSequences(heldOf(f.c, map[string]int64{"tags": 2}))
	if err != nil || res.Post != nil {
		t.Fatalf("a set to the largest key, the sequence the rows imply: %+v, %v", res, err)
	}
	for _, table := range []string{"items", "TAGS", "nosuch"} {
		if _, err := f.c.SetSequences(map[string]int64{table: 3}); err == nil {
			t.Fatalf("a set of %s, which keeps no sequence, is not refused", table)
		}
	}
}

// heldOf is the rows of sqlite_sequence of the corpus c with the sequence of tags of tags, none when tags names none.
func heldOf(c *Corpus, tags map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for _, tb := range c.Catalog().Tables {
		if n, row := c.Counter(tb.Name); tb.Autoincrement && tb.Name != "tags" && row {
			out[tb.Name] = n
		}
	}
	if n, ok := tags["tags"]; ok {
		out["tags"] = n
	}
	return out
}
