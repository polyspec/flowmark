package main

import (
	"strings"
	"testing"
	"time"
)

// TestCreated pins the time of a seeded post: inside its month, in whole seconds, rising with the post, never after
// now, and the same spans for the same arguments.
func TestCreated(t *testing.T) {
	const perMonth = 100
	finished := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		month int
		now   time.Time
	}{
		{"a finished month", 3, finished},
		{"the month that is not finished", 10, time.Date(2026, time.October, 12, 5, 6, 7, 800000000, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var prev time.Time
			for i := 0; i < perMonth; i++ {
				text := created(2026, c.month, i, perMonth, c.now)
				if !strings.HasSuffix(text, ".000000") {
					t.Fatalf("post %d: %q has a fraction of a second", i, text)
				}
				at, err := time.Parse("2006-01-02 15:04:05.000000", text)
				if err != nil {
					t.Fatalf("post %d: %q: %v", i, text, err)
				}
				if int(at.Month()) != c.month || at.Year() != 2026 {
					t.Fatalf("post %d: %q is not in month %d", i, text, c.month)
				}
				if at.After(c.now) {
					t.Fatalf("post %d: %q is after now %v", i, text, c.now)
				}
				if i > 0 && at.Before(prev) {
					t.Fatalf("post %d: %q is before the post before it", i, text)
				}
				prev = at
				if again := created(2026, c.month, i, perMonth, c.now); again != text {
					t.Fatalf("post %d: %q then %q", i, text, again)
				}
			}
		})
	}
}
