// Tests of the checklist state machine types and the superseded spawn.
package app

import "testing"

func TestSupersededID(t *testing.T) {
	if id := SupersededID("T8.0.14", []string{"T8.0.14"}); id != "T8.0.14-1" {
		t.Fatalf("first spawn: %q", id)
	}
	if id := SupersededID("T8.0.14", []string{"T8.0.14", "T8.0.14-1"}); id != "T8.0.14-2" {
		t.Fatalf("second spawn: %q", id)
	}
}
