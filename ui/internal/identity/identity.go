// Package identity compares open interface values without imposing comparability on
// implementations supplied by component users.
package identity

import "reflect"

// Same reports whether a and b demonstrably name the same object.
//
// Interface equality is not an identity primitive: it panics when a caller's
// concrete component contains a slice, map, or function. Comparable values can
// answer directly. Maps have stable runtime identity despite not being comparable as
// interface values. Everything else is conservatively different; an ownership
// boundary may repeat a focus transition, but it must never retain a gesture or a
// lease merely because two values happen to have equal-looking storage.
func Same(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	left, right := reflect.ValueOf(a), reflect.ValueOf(b)
	if left.Type() != right.Type() {
		return false
	}
	if left.Comparable() {
		return left.Interface() == right.Interface()
	}
	if left.Kind() == reflect.Map {
		return left.Pointer() == right.Pointer()
	}
	return false
}
