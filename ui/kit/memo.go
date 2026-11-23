package kit

// memo holds one value derived from a comparable key, so measurement and drawing
// at the same layout share one derivation. The zero memo holds nothing;
// discarding a memo is assigning the zero value, which a copy never shares.
type memo[K comparable, V any] struct {
	key   K
	value V
	valid bool
}

// get returns the value for key, deriving it only when the key changed. The
// result points into the memo and is valid until the next get or reset.
func (m *memo[K, V]) get(key K, derive func() V) *V {
	if !m.valid || m.key != key {
		m.key, m.value, m.valid = key, derive(), true
	}
	return &m.value
}
