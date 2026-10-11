package irexec

import "testing"

// TestParamSlots pins the check of a parameter against the type of its slot (spec 60 The wire), made before any row is read:
// the answer is CODEC_ENCODE, and the same request with a value of the slot's type is run.
func TestParamSlots(t *testing.T) {
	f := baseFixture(t)
	insertItem := func(title, due any) []byte {
		return f.msg("insert", "items", map[string]any{"set": sets("title", "body", "due")}, title, "", due)
	}
	cases := []struct {
		name string
		bad  []byte
		good []byte
	}{
		{"60#the-wire: U+0000 in a varchar", insertItem("a\u0000b", nil), insertItem("ab", nil)},
		{"60#the-wire: a date past 9999", insertItem("x", "10000-01-01"), insertItem("x", "9999-12-31")},
		{"60#the-wire: a date before 0001", insertItem("x", "0000-12-31"), insertItem("x", "0001-01-01")},
		{"60#the-wire: a date that does not exist", insertItem("x", "2026-02-30"), insertItem("x", "2026-02-28")},
		{"60#the-wire: a contains value that is no text",
			f.msg("all", "people", map[string]any{"where": where(pred("email", "contains", p(0)))}, 5),
			f.msg("all", "people", map[string]any{"where": where(pred("email", "contains", p(0)))}, "5")},
		{"60#the-wire: an interval that is no integer",
			f.msg("all", "people", map[string]any{"where": where(pred("joined", "gt", map[string]any{"value": map[string]any{"name": "days_ago", "ps": []int{0}}}))}, "ten"),
			f.msg("all", "people", map[string]any{"where": where(pred("joined", "gt", map[string]any{"value": map[string]any{"name": "days_ago", "ps": []int{0}}}))}, 10)},
	}
	for _, c := range cases {
		f.sub(t, c.name, func(f *fixture) {
			f.fails(c.bad, "CODEC_ENCODE")
			f.ok(c.good)
		})
	}
	f.sub(t, "60#the-wire: NULL is a value of a nullable column and of a comparison", func(f *fixture) {
		f.ok(f.msg("insert", "people", map[string]any{"set": sets("name", "age")}, "Lin", nil))
		f.ok(f.msg("all", "people", map[string]any{"where": where(pred("age", "eq", p(0)))}, nil))
	})
	f.sub(t, "60#the-wire: NULL assigned to a non-null column", func(f *fixture) {
		f.fails(f.msg("insert", "people", map[string]any{"set": sets("name")}, nil), "CODEC_ENCODE")
		f.fails(f.msg("update", "people", map[string]any{"set": sets("name"), "where": where(pred("id", "eq", p(1)))}, nil, "P2"), "CODEC_ENCODE")
		f.fails(f.msg("insert", "tickets", map[string]any{"set": sets("id", "state", "title"), "on_duplicate": []any{set("title", 3)}}, "K9", "open", "x", nil), "CODEC_ENCODE")
	})
	f.sub(t, "60#the-wire: a request that is checked before any row is read", func(f *fixture) {
		// A request over an empty corpus match still fails: the answer does not depend on the rows.
		f.fails(f.msg("all", "items", map[string]any{"where": where(pred("id", "eq", p(0)), pred("state", "eq", p(1)))}, "no-such-id", 5), "CODEC_ENCODE")
	})
}
