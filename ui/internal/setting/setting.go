// Package setting owns the transition every retained controller and skin value
// follows, so headless behaviour and its appearance cannot disagree about when a
// changed value invalidates.
package setting

// Owner is the element whose presentation a setting changes. Taking the
// capability rather than the element keeps this package beneath the scene.
type Owner interface {
	CheckMutation() error
	Invalidate()
}

// Assign panics when owner refuses mutation, ignores an unchanged value, and
// otherwise stores value and invalidates owner once.
func Assign[T comparable](owner Owner, field *T, value T) {
	if err := owner.CheckMutation(); err != nil {
		panic(err)
	}
	if *field == value {
		return
	}
	*field = value
	owner.Invalidate()
}

// Replace is Assign for a value that cannot be compared, such as a callback or
// an interface: every replacement invalidates owner.
func Replace[T any](owner Owner, field *T, value T) {
	if err := owner.CheckMutation(); err != nil {
		panic(err)
	}
	*field = value
	owner.Invalidate()
}
