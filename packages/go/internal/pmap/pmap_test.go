package pmap

import (
	"encoding/binary"
	"maps"
	"slices"
	"testing"
)

// entries returns the keys and values m yields through seq, in order.
func entries(seq func(func(int, string) bool)) ([]int, []string) {
	var ks []int
	var vs []string
	for k, v := range seq {
		ks = append(ks, k)
		vs = append(vs, v)
	}
	return ks, vs
}

// wantModel requires m to hold exactly the keys and values of model, in ascending order, and every range of m to
// agree with model.
func wantModel(t *testing.T, m Map[int, string], model map[int]string) {
	t.Helper()
	if m.Len() != len(model) {
		t.Fatalf("Len = %d, want %d", m.Len(), len(model))
	}
	keys := slices.Sorted(maps.Keys(model))
	got, vals := entries(m.All())
	if !slices.Equal(got, keys) {
		t.Fatalf("All yields %v, want %v", got, keys)
	}
	for i, k := range keys {
		if vals[i] != model[k] {
			t.Fatalf("All yields %d=%q, want %q", k, vals[i], model[k])
		}
		if v, ok := m.Get(k); !ok || v != model[k] {
			t.Fatalf("Get(%d) = %q, %v; want %q", k, v, ok, model[k])
		}
	}
	for _, probe := range []int{-1, 0, 1, 2, 3, 7, 50, 99, 100, 1000} {
		if _, ok := m.Get(probe); ok != hasKey(model, probe) {
			t.Fatalf("Get(%d) reports %v", probe, ok)
		}
		var from, before []int
		for _, k := range keys {
			if k >= probe {
				from = append(from, k)
			}
		}
		for i := len(keys) - 1; i >= 0; i-- {
			if keys[i] < probe {
				before = append(before, keys[i])
			}
		}
		if got, _ := entries(m.From(probe)); !slices.Equal(got, from) {
			t.Fatalf("From(%d) yields %v, want %v", probe, got, from)
		}
		if got, _ := entries(m.Before(probe)); !slices.Equal(got, before) {
			t.Fatalf("Before(%d) yields %v, want %v", probe, got, before)
		}
	}
}

func hasKey(m map[int]string, k int) bool {
	_, ok := m[k]
	return ok
}

// TestMap pins the map against Go's map: every key set, replaced and deleted, read back in order and by every range,
// and an old map unchanged by the changes made from it.
func TestMap(t *testing.T) {
	t.Run("the zero map is empty", func(t *testing.T) {
		var m Map[int, string]
		wantModel(t, m, map[int]string{})
		if m.Delete(3).Len() != 0 {
			t.Fatal("deleting from the empty map holds a key")
		}
	})
	for _, tc := range []struct {
		name string
		keys []int
	}{
		{"one key", []int{5}},
		{"ascending keys", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{"descending keys", []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}},
		{"keys out of order with repeats", []int{50, 3, 99, 3, 0, 50, 7, 100, 2, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m Map[int, string]
			model := map[int]string{}
			for i, k := range tc.keys {
				v := string(rune('a' + i))
				m = m.Set(k, v)
				model[k] = v
				wantModel(t, m, model)
			}
			for _, k := range tc.keys {
				m = m.Delete(k)
				delete(model, k)
				wantModel(t, m, model)
			}
		})
	}
	t.Run("a change leaves the map it was made from as it was", func(t *testing.T) {
		var m Map[int, string]
		for k := range 100 {
			m = m.Set(k, "old")
		}
		n := m.Set(50, "new").Delete(7).Set(1000, "added")
		model := map[int]string{}
		for k := range 100 {
			model[k] = "old"
		}
		wantModel(t, m, model)
		model[50], model[1000] = "new", "added"
		delete(model, 7)
		wantModel(t, n, model)
	})
	t.Run("deleting a key the map does not hold returns the map", func(t *testing.T) {
		m := Map[int, string]{}.Set(1, "a")
		if n := m.Delete(2); n.root != m.root || n.Len() != 1 {
			t.Fatal("the map changed")
		}
	})
	t.Run("a range stops when its loop does", func(t *testing.T) {
		var m Map[int, string]
		for k := range 10 {
			m = m.Set(k, "v")
		}
		for name, seq := range map[string]func(func(int, string) bool){"All": m.All(), "From": m.From(3), "Before": m.Before(8)} {
			n := 0
			for range seq {
				n++
				if n == 2 {
					break
				}
			}
			if n != 2 {
				t.Fatalf("%s yielded %d keys before the break", name, n)
			}
		}
	})
	t.Run("the depth stays logarithmic over ascending keys", func(t *testing.T) {
		var m Map[int, string]
		const n = 100000
		for k := range n {
			m = m.Set(k, "")
		}
		if d := depth(m.root); d > 80 {
			t.Fatalf("depth %d at %d ascending keys", d, n)
		}
	})
}

// depth returns the number of nodes on the longest path from t down.
func depth(t *node[int, string]) int {
	if t == nil {
		return 0
	}
	return 1 + max(depth(t.left), depth(t.right))
}

// FuzzMap applies the changes the input encodes — each two bytes a key, set when the first is even and deleted when it
// is odd — to the map and to Go's map, and requires them equal after every change, with every older map unchanged.
func FuzzMap(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 5, 2, 5, 1, 5})
	f.Add([]byte{0, 1, 0, 2, 0, 3, 1, 2, 0, 2, 1, 1, 1, 3})
	f.Fuzz(func(t *testing.T, ops []byte) {
		var m Map[int, string]
		model := map[int]string{}
		var olds []Map[int, string]
		var oldModels []map[int]string
		for i := 0; i+1 < len(ops); i += 2 {
			k := int(binary.BigEndian.Uint16([]byte{0, ops[i+1]})) % 64
			olds, oldModels = append(olds, m), append(oldModels, maps.Clone(model))
			if ops[i]%2 == 0 {
				v := string(rune('a' + i%26))
				m, model[k] = m.Set(k, v), v
			} else {
				m = m.Delete(k)
				delete(model, k)
			}
			wantModel(t, m, model)
		}
		for i := range olds {
			wantModel(t, olds[i], oldModels[i])
			wantDiff(t, olds[i], m, oldModels[i], model)
		}
	})
}

// modelDiff returns the keys a and b do not hold alike, ascending.
func modelDiff(a, b map[int]string) []int {
	var out []int
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			out = append(out, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func sameString(x, y string) bool { return x == y }

// wantDiff requires Diff of a and b, both ways, to yield the keys their models do not hold alike, in ascending order.
func wantDiff(t *testing.T, a, b Map[int, string], ma, mb map[int]string) {
	t.Helper()
	for _, c := range [][2]Map[int, string]{{a, b}, {b, a}} {
		var got []int
		Diff(c[0], c[1], sameString, func(k int) { got = append(got, k) })
		if want := modelDiff(ma, mb); !slices.Equal(got, want) {
			t.Fatalf("Diff yields %v, want %v", got, want)
		}
	}
}

// TestDiff pins Diff against Go's maps, and that two maps one was changed from the other are compared at the keys of
// the changes, whatever the keys they hold.
func TestDiff(t *testing.T) {
	build := func(keys ...int) (Map[int, string], map[int]string) {
		var m Map[int, string]
		model := map[int]string{}
		for _, k := range keys {
			m, model[k] = m.Set(k, "v"), "v"
		}
		return m, model
	}
	for _, tc := range []struct {
		name string
		a, b []int
	}{
		{"two empty maps", nil, nil},
		{"an empty map and another", nil, []int{1, 2, 3}},
		{"disjoint keys", []int{1, 3, 5}, []int{2, 4, 6}},
		{"the same keys built in other orders", []int{1, 2, 3, 4, 5}, []int{5, 4, 3, 2, 1}},
		{"overlapping keys", []int{1, 2, 3, 4}, []int{3, 4, 5, 6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, ma := build(tc.a...)
			b, mb := build(tc.b...)
			wantDiff(t, a, b, ma, mb)
		})
	}
	t.Run("a value that differs at the same key", func(t *testing.T) {
		a, ma := build(1, 2, 3)
		b, mb := a.Set(2, "w"), map[int]string{1: "v", 2: "w", 3: "v"}
		wantDiff(t, a, b, ma, mb)
	})
	t.Run("a change compares the keys of its paths, not every key", func(t *testing.T) {
		const n = 100000
		var a Map[int, string]
		for k := range n {
			a = a.Set(k, "v")
		}
		b := a.Set(n/2, "w").Delete(7).Set(n+1, "v")
		var got []int
		compared := Diff(a, b, sameString, func(k int) { got = append(got, k) })
		if !slices.Equal(got, []int{7, n / 2, n + 1}) {
			t.Fatalf("Diff yields %v", got)
		}
		t.Logf("Diff compares %d keys of %d for three changes", compared, n)
		if compared > 400 {
			t.Fatalf("Diff compares %d keys of %d for three changes", compared, n)
		}
		if Diff(a, a, sameString, func(int) { t.Fatal("a map differs from itself") }) != 0 {
			t.Fatal("a map compares keys with itself")
		}
	})
}
