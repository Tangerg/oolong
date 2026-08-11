package headless

// Accessor is caller-owned readable and writable state.
//
// Fields use one because a form is collecting into an application value. Controlled
// controllers use the same small contract so their operations update the caller's
// single source of truth instead of maintaining a private shadow copy.
//
// [Bind] is the one every caller wants: a variable of their own. Anything else that can
// be read and written — a setting, a row of a record, a channel of one — is an accessor
// too, which is why this is an interface and not a pointer.
type Accessor[T any] interface {
	Value() T
	Set(v T)
}

// Bind is an accessor for a variable of the caller's own. A nil pointer is a
// programmer error and panics at construction rather than later in an unrelated
// control operation.
//
//	var name string
//	field := &headless.Text{Label: "Name", Value: headless.Bind(&name)}
func Bind[T any](p *T) Accessor[T] {
	if p == nil {
		panic("headless: nil binding pointer")
	}
	return bound[T]{p}
}

type bound[T any] struct{ at *T }

func (b bound[T]) Value() T { return *b.at }
func (b bound[T]) Set(v T)  { *b.at = v }

// ownedValue is local storage unless an accessor gives ownership to a caller.
//
// It stays private because ownership is part of each controller's configuration, not
// a second state API applications should pass around. One constructor accepts either
// mode; operations use this one path and never keep a shadow copy of controlled state.
type ownedValue[T any] struct {
	local   T
	binding Accessor[T]
}

func newOwnedValue[T any](initial T, binding Accessor[T]) ownedValue[T] {
	if binding != nil {
		return ownedValue[T]{binding: binding}
	}
	return ownedValue[T]{local: initial}
}

func (v *ownedValue[T]) get() T {
	if v.binding != nil {
		return v.binding.Value()
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
