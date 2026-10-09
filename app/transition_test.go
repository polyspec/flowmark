// Tests of the checklist state machine: the transition table of spec 100-checklist.
package app

import "testing"

func TestTransitionTable(t *testing.T) {
	cases := []struct {
		from, to string
		ok       bool
		requires []string
	}{
		{"waiting", "doing", true, nil},
		{"doing", "done", true, []string{"evidence"}},
		{"doing", "bypassed", true, []string{"cause", "retry"}},
		{"bypassed", "doing", true, nil},
		{"waiting", "impossible", true, []string{"cause"}},
		{"done", "doing", false, nil},
		{"done", "impossible", false, nil},
		{"waiting", "done", false, nil},
	}
	for _, testCase := range cases {
		transition, ok := CanTransition(testCase.from, testCase.to)
		if ok != testCase.ok {
			t.Errorf("%s -> %s: ok = %v", testCase.from, testCase.to, ok)
			continue
		}
		if ok && len(transition.Requires) != len(testCase.requires) {
			t.Errorf("%s -> %s: requires %v, want %v", testCase.from, testCase.to, transition.Requires, testCase.requires)
		}
	}
}

func TestStateGlyphs(t *testing.T) {
	for glyph, want := range map[string]string{"[o]": "done", "[ ]": "waiting", "[~]": "doing", "[!]": "bypassed", "[-]": "impossible"} {
		state, ok := StateOf(glyph)
		if !ok || state != want {
			t.Fatalf("glyph %s: %q, %v", glyph, state, ok)
		}
	}
	if _, ok := StateOf("[x]"); ok {
		t.Fatal("an unknown glyph maps to no state")
	}
}

func TestSupersededID(t *testing.T) {
	if id := SupersededID("T8.0.14", []string{"T8.0.14"}); id != "T8.0.14-1" {
		t.Fatalf("first spawn: %q", id)
	}
	if id := SupersededID("T8.0.14", []string{"T8.0.14", "T8.0.14-1"}); id != "T8.0.14-2" {
		t.Fatalf("second spawn: %q", id)
	}
}
