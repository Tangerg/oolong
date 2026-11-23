// Package known holds a value that may not have been learned yet. A value and the
// fact that it was learned live in one place, so nothing can read a value nobody
// supplied, or believe one was supplied after it was forgotten.
package known

// Value is either unknown, its zero value, or a learned value.
type Value[T any] struct {
	value T
	ok    bool
}

// Of is a learned value.
func Of[T any](value T) Value[T] { return Value[T]{value: value, ok: true} }

// Get reports the value and whether it was learned.
func (v Value[T]) Get() (T, bool) { return v.value, v.ok }

// Is reports whether v was learned and equals want.
func Is[T comparable](v Value[T], want T) bool {
	got, ok := v.Get()
	return ok && got == want
}
