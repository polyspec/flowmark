// Package pmap is a persistent ordered map: a change returns a new map that shares every node it did not change with
// the old one, so a corpus read, a write's post-image and the next read cost the keys they change, not the keys they
// hold (plan, Ruling: an operation costs what it touches).
//
// The map is a treap: a binary search tree on the keys that is a heap on priorities, so its expected depth is O(log n)
// whatever the order of the changes. A key's priority is its hash under a seed the process draws at random, so the
// shape of a map is a function of its keys alone: two maps of the same keys have the same shape, however they were
// built, Diff compares them along the same paths, and no order of keys a caller chooses makes the tree deep. A change copies the nodes on the paths it splits and joins; the old map
// is never changed, so maps are safe to share between goroutines that only read them.
package pmap

import (
	"cmp"
	"hash/maphash"
	"iter"
)

// seed is the hash seed of the priorities, one for the process.
var seed = maphash.MakeSeed()

// Map is a persistent ordered map from K to V. The zero Map is empty and ready to use.
type Map[K cmp.Ordered, V any] struct {
	root *node[K, V]
	n    int
}

type node[K cmp.Ordered, V any] struct {
	key         K
	val         V
	prio        uint64
	left, right *node[K, V]
}

// Len returns the number of keys of m.
func (m Map[K, V]) Len() int { return m.n }

// Get returns the value of k and whether m holds k.
func (m Map[K, V]) Get(k K) (V, bool) {
	for t := m.root; t != nil; {
		switch c := cmp.Compare(k, t.key); {
		case c < 0:
			t = t.left
		case c > 0:
			t = t.right
		default:
			return t.val, true
		}
	}
	var zero V
	return zero, false
}

// Set returns m with k bound to v.
func (m Map[K, V]) Set(k K, v V) Map[K, V] {
	l, e, r := split(m.root, k)
	n := m.n
	if e == nil {
		n++
	}
	prio := maphash.Comparable(seed, k)
	return Map[K, V]{root: join(join(l, &node[K, V]{key: k, val: v, prio: prio}), r), n: n}
}

// Delete returns m without k; m itself when it does not hold k.
func (m Map[K, V]) Delete(k K) Map[K, V] {
	if _, ok := m.Get(k); !ok {
		return m
	}
	l, _, r := split(m.root, k)
	return Map[K, V]{root: join(l, r), n: m.n - 1}
}

// split returns the keys of t before k, the node of k (nil when t holds none), and the keys after k, copying the nodes
// on the path to k.
func split[K cmp.Ordered, V any](t *node[K, V], k K) (l, e, r *node[K, V]) {
	if t == nil {
		return nil, nil, nil
	}
	switch c := cmp.Compare(k, t.key); {
	case c < 0:
		ll, le, lr := split(t.left, k)
		cp := *t
		cp.left = lr
		return ll, le, &cp
	case c > 0:
		rl, re, rr := split(t.right, k)
		cp := *t
		cp.right = rl
		return &cp, re, rr
	default:
		return t.left, t, t.right
	}
}

// join returns the tree of the keys of l and then of r, every key of l before every key of r, copying the nodes on
// the spines it joins.
func join[K cmp.Ordered, V any](l, r *node[K, V]) *node[K, V] {
	switch {
	case l == nil:
		return r
	case r == nil:
		return l
	case l.prio >= r.prio:
		cp := *l
		cp.right = join(l.right, r)
		return &cp
	default:
		cp := *r
		cp.left = join(l, r.left)
		return &cp
	}
}

// All yields every key of m with its value, in ascending order of the keys.
func (m Map[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) { ascend(m.root, nil, yield) }
}

// From yields every key of m from k on, k included, with its value, in ascending order.
func (m Map[K, V]) From(k K) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) { ascend(m.root, &k, yield) }
}

// ascend yields the keys of t from *from on (every key when from is nil) in ascending order, and reports whether
// yield asked for more.
func ascend[K cmp.Ordered, V any](t *node[K, V], from *K, yield func(K, V) bool) bool {
	for t != nil {
		if from != nil && cmp.Compare(t.key, *from) < 0 {
			t = t.right
			continue
		}
		if !ascend(t.left, from, yield) || !yield(t.key, t.val) {
			return false
		}
		// Every key of the right subtree is after t's, so none is before from.
		from, t = nil, t.right
	}
	return true
}

// Before yields every key of m before k, k excluded, with its value, in descending order of the keys.
func (m Map[K, V]) Before(k K) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) { descend(m.root, k, yield) }
}

// descend yields the keys of t before *before in descending order, and reports whether yield asked for more.
func descend[K cmp.Ordered, V any](t *node[K, V], before K, yield func(K, V) bool) bool {
	for t != nil {
		if cmp.Compare(t.key, before) >= 0 {
			t = t.left
			continue
		}
		// The right subtree holds keys after t's, some of them perhaps not before the bound; the left one holds none.
		if !descend(t.right, before, yield) || !yield(t.key, t.val) {
			return false
		}
		return descendAll(t.left, yield)
	}
	return true
}

// descendAll yields every key of t in descending order, and reports whether yield asked for more.
func descendAll[K cmp.Ordered, V any](t *node[K, V], yield func(K, V) bool) bool {
	for t != nil {
		if !descendAll(t.right, yield) || !yield(t.key, t.val) {
			return false
		}
		t = t.left
	}
	return true
}

// Diff calls f with each key that a and b do not hold alike, in ascending order: a key one of them holds and the
// other does not, or a key whose values same reports unequal. A subtree the two maps share is skipped whole, so maps
// that one was changed from the other differ in the keys of the changes, and Diff costs those keys, not the keys the
// maps hold. It returns the number of keys it compared.
func Diff[K cmp.Ordered, V any](a, b Map[K, V], same func(x, y V) bool, f func(k K)) int {
	return diff(a.root, b.root, same, f)
}

func diff[K cmp.Ordered, V any](a, b *node[K, V], same func(x, y V) bool, f func(k K)) int {
	switch {
	case a == b:
		return 0
	case a == nil:
		n := 0
		ascend(b, nil, func(k K, _ V) bool { n++; f(k); return true })
		return n
	case b == nil:
		n := 0
		ascend(a, nil, func(k K, _ V) bool { n++; f(k); return true })
		return n
	}
	// Splitting b at a's key keeps every subtree of b off the path to the key, so what b shares with a's subtrees
	// stays shared.
	l, e, r := split(b, a.key)
	n := diff(a.left, l, same, f) + 1
	if e == nil || !same(a.val, e.val) {
		f(a.key)
	}
	return n + diff(a.right, r, same, f)
}
