package irexec

import (
	"slices"
	"testing"
)

// TestNextPrefixedKeys pins the keys a key_prefix allocates (spec 20 "Keys"): after the largest decimal suffix without a
// leading zero among the keys of the prefix, one after another, whatever else the keys hold.
func TestNextPrefixedKeys(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keys  []string
		count int
		want  []string
	}{
		{"no keys", nil, 3, []string{"T1", "T2", "T3"}},
		{"after the largest", []string{"T2", "T10", "T9"}, 2, []string{"T11", "T12"}},
		{"a leading zero is no number", []string{"T3", "T099"}, 1, []string{"T4"}},
		{"a suffix that is not decimal is no number", []string{"T3", "T7a", "T-9", "T"}, 1, []string{"T4"}},
		{"another prefix is no number", []string{"T3", "N50", "t40"}, 1, []string{"T4"}},
		{"zero alone is a number", []string{"T0"}, 1, []string{"T1"}},
		{"beyond 64 bits", []string{"T18446744073709551616"}, 2, []string{"T18446744073709551617", "T18446744073709551618"}},
		{"none asked", []string{"T3"}, 0, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextPrefixedKeys("T", tc.keys, tc.count); !slices.Equal(got, tc.want) {
				t.Fatalf("NextPrefixedKeys(T, %v, %d) = %v, want %v", tc.keys, tc.count, got, tc.want)
			}
			if tc.count > 0 {
				if got := NextPrefixed("T", tc.keys); got != tc.want[0] {
					t.Fatalf("NextPrefixed(T, %v) = %s, want %s", tc.keys, got, tc.want[0])
				}
			}
		})
	}
}
