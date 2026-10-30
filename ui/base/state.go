package base

// ownedValue is the sole mutable value of a scalar controller. Config only seeds it.
type ownedValue[T comparable] struct {
	local    T
	revision uint64
}

func newOwnedValue[T comparable](initial T) ownedValue[T] { return ownedValue[T]{local: initial} }
func (v *ownedValue[T]) get() T                           { return v.local }
func (v *ownedValue[T]) set(value T) bool {
	if v.local == value {
		return false
	}
	v.local = value
	v.revision++
	return true
}
