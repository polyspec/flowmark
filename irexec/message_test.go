package irexec

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMessage pins the request message of the protocol (orm protocol section 5,
// spec 60 The wire): {ir_version, manifest_hash, request, params, now}, the request
// validated with ir.Validate, params of n_params values in the runtime value model
// JSON, and now in the datetime wire form.
func TestMessage(t *testing.T) {
	f := baseFixture(t)
	hash := f.e.Model().ManifestHash
	request := func(edit func(m map[string]any)) map[string]any {
		m := map[string]any{"ir_version": 1, "manifest_hash": hash, "kind": "count", "entity": "tags", "n_params": 0}
		if edit != nil {
			edit(m)
		}
		return m
	}
	message := func(edit func(m map[string]any)) []byte {
		m := map[string]any{"ir_version": 1, "manifest_hash": hash, "request": request(nil), "params": []any{}, "now": testNow}
		if edit != nil {
			edit(m)
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	f.sub(t, "60#the-wire: a well-formed message is answered", func(f *fixture) {
		f.want(message(nil), `{"count":2}`)
	})
	cases := []struct {
		name string
		msg  []byte
		code string
	}{
		{"60#the-wire: a message of another IR version", message(func(m map[string]any) { m["ir_version"] = 2 }), "VERSION_MISMATCH"},
		{"60#the-wire: a request of another IR version", message(func(m map[string]any) { m["request"] = request(func(r map[string]any) { r["ir_version"] = 2 }) }), "VERSION_MISMATCH"},
		{"60#the-wire: a message of another manifest", message(func(m map[string]any) { m["manifest_hash"] = "sha256:" + strings.Repeat("0", 64) }), "SCHEMA_HASH_MISMATCH"},
		{"60#the-wire: a request of another manifest", message(func(m map[string]any) {
			m["request"] = request(func(r map[string]any) { r["manifest_hash"] = "sha256:" + strings.Repeat("0", 64) })
		}), "SCHEMA_HASH_MISMATCH"},
		{"60#the-wire: a message field the protocol does not name", message(func(m map[string]any) { m["sql"] = "SELECT 1" }), "IR_INVALID"},
		{"60#the-wire: a request field the protocol does not name", message(func(m map[string]any) { m["request"] = request(func(r map[string]any) { r["sql"] = "x" }) }), "IR_INVALID"},
		{"60#the-wire: a message without a request", message(func(m map[string]any) { delete(m, "request") }), "IR_INVALID"},
		{"60#the-wire: params of another length than n_params", message(func(m map[string]any) { m["params"] = []any{1} }), "IR_INVALID"},
		{"60#the-wire: now without its six fraction digits", message(func(m map[string]any) { m["now"] = "2026-10-11 09:00:00" }), "IR_INVALID"},
		{"60#the-wire: now that is no datetime", message(func(m map[string]any) { m["now"] = "today" }), "IR_INVALID"},
		{"60#the-wire: an unknown kind", message(func(m map[string]any) { m["request"] = request(func(r map[string]any) { r["kind"] = "merge" }) }), "IR_INVALID"},
		{"60#the-wire: a parameter index out of range", message(func(m map[string]any) {
			m["request"] = request(func(r map[string]any) { r["where"] = where(pred("id", "eq", p(3))) })
		}), "IR_INVALID"},
		{"60#the-wire: two objects", append(message(nil), []byte(" {}")...), "IR_INVALID"},
		{"60#the-wire: no JSON", []byte("not json"), "IR_INVALID"},
	}
	for _, c := range cases {
		f.sub(t, c.name, func(f *fixture) { f.fails(c.msg, c.code) })
	}
}

// TestTimeFormToday pins the time form the executor reads and answers until orm T91
// fixes the time wire form HH:MM:SS.ffffff (spec 60 The wire; T12.4 rulings 5 and
// 6): a time(p) parameter and answer are the readable form with exactly p digits,
// so the six-digit wire form of a time(0) or time(3) value is refused today.
func TestTimeFormToday(t *testing.T) {
	t.Run("60#the-wire: a time is read and answered with its p digits until orm T91", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.want(f.msg("insert", "stamps", map[string]any{"set": sets("label", "opens", "closes")}, "one", "10:00:00", "10:00:00.100"), `{"affected":1,"keys":[1]}`))
		f.want(f.msg("all", "stamps", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"opens", "closes"}}, "where": where(pred("opens", "eq", p(0)))}, "10:00:00"),
			`{"rows":[{"id":1,"opens":"10:00:00","closes":"10:00:00.100"}]}`)
	})
	f := baseFixture(t)
	for _, c := range []struct{ col, v string }{{"opens", "10:00:00.000000"}, {"closes", "10:00:00.100000"}} {
		f.sub(t, "60#the-wire: the six-digit time form waits on orm T91: "+c.col, func(f *fixture) {
			err := f.fails(f.msg("insert", "stamps", map[string]any{"set": sets("label", c.col)}, "one", c.v), "CONSTRAINT")
			wantFaults(f.t, err, []dupFault{{"stamps", "1", -1, c.col, "CONSTRAINT", c.col}})
		})
	}
}

// TestParams pins the parameters of the runtime value model JSON: a decimal is a
// string, bytes are base64, and a datetime is YYYY-MM-DD HH:MM:SS.ffffff in UTC, its
// digits past the column's precision zeros.
func TestParams(t *testing.T) {
	t.Run("60#the-wire: a datetime of six digits into datetime(0) with zeros past the precision", func(t *testing.T) {
		f := fresh(t)
		f.commit(f.ok(f.msg("insert", "people", map[string]any{"set": sets("name", "joined")}, "Lin", "2026-10-09 07:30:00.000000")))
		f.want(f.msg("all", "people", map[string]any{"columns": map[string]any{"mode": "none", "add": []string{"joined"}}, "where": where(pred("id", "eq", p(0)))}, "P4"),
			`{"rows":[{"id":"P4","joined":"2026-10-09 07:30:00.000000"}]}`)
	})
	f := baseFixture(t)
	cases := []struct {
		name  string
		param any
		col   string
	}{
		{"60#the-wire: a datetime with a fraction past the column's precision", "2026-10-09 07:30:00.500000", "joined"},
		{"60#the-wire: a datetime not in the wire form", "2026-10-09T07:30:00Z", "joined"},
		{"60#the-wire: a decimal as a number", 1.5, "score"},
		{"60#the-wire: a decimal with more digits than its scale", "1.505", "score"},
		{"60#the-wire: an integer with a fraction", 1.5, "age"},
		{"60#the-wire: an integer past its type", 3000000000, "age"},
		{"60#the-wire: a bool as text", "true", "active"},
	}
	for _, c := range cases {
		f.sub(t, c.name, func(f *fixture) {
			f.fails(f.msg("insert", "people", map[string]any{"set": sets("name", c.col)}, "Lin", c.param), "CODEC_ENCODE")
		})
	}
}
