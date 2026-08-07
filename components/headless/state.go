package headless

// ownedValue is local storage unless an accessor gives ownership to a caller.
//
// It stays private because ownership is part of each controller's construction, not a
// second state API applications should pass around. The two controller constructors
// make the distinction visible; operations use this one path in either mode and never
// keep a shadow copy of controlled state.
type ownedValue[T any] struct {
	local   T
	binding Accessor[T]
}

func localValue[T any](initial T) ownedValue[T] { return ownedValue[T]{local: initial} }

func controlledValue[T any](binding Accessor[T]) ownedValue[T] {
	if binding == nil {
		panic("headless: nil controlled-state accessor")
	}
	return ownedValue[T]{binding: binding}
}

func (v *ownedValue[T]) get() T {
	if v.binding != nil {
		return v.binding.Get()
	}
	return v.local
}

func (v *ownedValue[T]) set(value T) {
	if v.binding != nil {
		v.binding.Set(value)
		return
	}
	v.local = value
}
