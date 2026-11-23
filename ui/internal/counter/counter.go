// Package counter owns the rule every identity and revision in this module
// follows: a successor is refused rather than wrapped, because a wrapped number
// can name state that still exists. What exhaustion means stays with each owner.
package counter

// Next is v's successor, or false when v is the largest value of its type.
func Next[T ~uint32 | ~uint64](v T) (T, bool) {
	if v == ^T(0) {
		return v, false
	}
	return v + 1, true
}
