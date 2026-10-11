package irexec

import (
	"bytes"
	"encoding/json"
	"sort"
	"sync"
	"testing"
)

// errorCodes are the codes of orm docs/errors.yaml.
var errorCodes = map[string]bool{
	"VERSION_MISMATCH": true, "SCHEMA_HASH_MISMATCH": true, "SCHEMA_INVALID": true, "DIALECT_UNKNOWN": true,
	"IR_INVALID": true, "CAPABILITY_UNSUPPORTED": true, "ENTITY_UNKNOWN": true, "COLUMN_UNKNOWN": true,
	"RELATION_UNKNOWN": true, "INDEX_UNKNOWN": true, "COLUMN_ALIAS_CONFLICT": true, "OPERATOR_UNKNOWN": true,
	"FUNCTION_UNKNOWN": true, "OPERATOR_NOT_ALLOWED": true, "OR_AT_GROUP_START": true, "EMPTY_IN": true,
	"ENTITY_NOT_JOINED": true, "LIMIT_IN_RELATION": true, "NO_ROWS": true, "COLUMN_UNSELECTED": true,
	"OPTIMISTIC_LOCK": true, "WRITE_CONFLICT": true, "CODEC_DECODE": true, "CODEC_ENCODE": true,
	"CODEC_UNSUPPORTED": true, "CONFIG": true, "CANCELED": true, "ROLLBACK": true, "FAULT": true,
	"SUBSCRIBER": true, "SCHEMA_DIFFERS": true, "INTERNAL": true, "LOCK_NOT_AVAILABLE": true, "DEADLOCK": true,
	"DUPLICATE_KEY": true, "FOREIGN_KEY": true, "CONSTRAINT": true, "READ_ONLY": true, "CONNECTION_LOST": true,
	"DRIVER": true,
}

// fuzzCorpora are the corpora the fuzzer runs requests on: the store goldens, then
// the executor's base corpus.
var (
	fuzzMu       sync.Mutex
	fuzzFixtures = map[int]*fixture{}
)

func fuzzFixture(t testing.TB, i int) *fixture {
	t.Helper()
	fuzzMu.Lock()
	defer fuzzMu.Unlock()
	if f, ok := fuzzFixtures[i]; ok {
		return f.on(t)
	}
	var f *fixture
	if i < len(goldenRoots) {
		f = goldenFixture(t, goldenRoots[i])
	} else {
		f = baseFixture(t)
	}
	fuzzFixtures[i] = f
	return f.on(t)
}

// seedMessages returns request messages over every table of the fixture: each read
// kind, and a delete and an update by the key of the table's first row.
func seedMessages(f *fixture) [][]byte {
	var out [][]byte
	names := make([]string, 0, len(f.e.Model().Entities))
	for n := range f.e.Model().Entities {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		ent := f.e.Model().Entities[n]
		key := ent.PK[0]
		out = append(out,
			f.msg("all", n, nil),
			f.msg("count", n, map[string]any{"group_by": []string{key}}),
			f.msg("group_count", n, map[string]any{"group_by": []string{key}}),
			f.msg("paginate", n, map[string]any{"limit": map[string]any{"offset": 0, "count": 1}, "order": []any{map[string]any{"column": key, "desc": true}}}),
		)
		for _, x := range f.c.recs[ent.Table] {
			v, err := f.c.value(x, key)
			if err != nil {
				break
			}
			param := any(v.text)
			if isInteger(columnOf(x.tb, key).Type.Kind) {
				param = json.Number(v.text)
			}
			out = append(out,
				f.msg("one", n, map[string]any{"where": where(pred(key, "eq", p(0)))}, param),
				f.msg("delete", n, map[string]any{"where": where(pred(key, "eq", p(0)))}, param),
			)
			for _, fl := range ent.Fields {
				if !fl.PrimaryKey && !fl.Identity && fl.Type == "varchar" {
					out = append(out, f.msg("update", n, map[string]any{"set": sets(fl.Name), "where": where(pred(key, "eq", p(1)))}, "x", param))
					break
				}
			}
			break
		}
	}
	return out
}

// FuzzExecute decodes and runs fuzzed request messages on the golden corpora: the
// executor must not panic, must answer or fail with one code of orm errors.yaml and
// never INTERNAL, must leave the corpus it read unchanged, and must answer the same
// message the same way twice. The seeds are messages over every table of each
// golden corpus.
func FuzzExecute(f *testing.F) {
	for i := 0; i <= len(goldenRoots); i++ {
		fx := fuzzFixture(f, i)
		for _, m := range seedMessages(fx) {
			f.Add(uint8(i), m)
		}
	}
	f.Fuzz(func(t *testing.T, i uint8, msg []byte) {
		fx := fuzzFixture(t, int(i)%(len(goldenRoots)+1))
		first := outcome(t, fx, msg)
		if second := outcome(t, fx, msg); !bytes.Equal(first, second) && !bytes.Contains(msg, []byte(`"random"`)) {
			t.Fatalf("one message answered two ways:\n%s\n%s", first, second)
		}
	})
}

// outcome runs msg and returns its answer, or its error code, as bytes.
func outcome(t *testing.T, fx *fixture, msg []byte) []byte {
	res, err := fx.e.Execute(fx.c, msg)
	if err != nil {
		e, ok := err.(*Error)
		if !ok {
			t.Fatalf("an error that is not an *Error: %v", err)
		}
		if !errorCodes[e.Code] || e.Code == "INTERNAL" {
			t.Fatalf("message %q failed with %v", msg, e)
		}
		return []byte(e.Code)
	}
	b, merr := json.Marshal(res.Answer)
	if merr != nil {
		t.Fatalf("the answer does not marshal: %v", merr)
	}
	if res.Post != nil {
		seen := map[string]bool{}
		for _, tg := range res.Post.Targets {
			if seen[tg.Path] || (tg.Before == nil && tg.After == nil) {
				t.Fatalf("a target %q repeated or empty", tg.Path)
			}
			seen[tg.Path] = true
			b = append(b, []byte(tg.Path)...)
			b = append(b, tg.After...)
		}
	}
	return b
}
