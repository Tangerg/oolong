package headless

// Accessor is caller-owned readable and writable state.
//
// Fields use one because a form is collecting into an application value. Controlled
// controllers use the same small contract so their operations update the caller's
// single source of truth instead of maintaining a private shadow copy.
//
// Reading an accessor is pure but cannot announce that its owner wrote a new value.
// Fields reconcile at their next semantic operation and project the current value in
// Draw. Controllers whose reconciliation also moves focus or modal stack membership
// expose Sync, keeping those transitions explicit and outside presentation.
//
// Value must be a pure read because presentation may call it. Set is synchronous: when
// it returns, Value reports the state the owner accepted. Set may also persist,
// validate or publish that transition, which is why components avoid redundant calls.
//
// [Bind] is the usual case: a variable of the caller's own. A setting or record field
// can implement the same contract, which is why this is an interface and not a pointer.
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
type ownedValue[T comparable] struct {
	state   valueState[T]
	binding Accessor[T]
}

func newOwnedValue[T comparable](initial T, binding Accessor[T]) ownedValue[T] {
	return ownedValue[T]{state: valueState[T]{local: initial}, binding: binding}
}

func (v *ownedValue[T]) get() T {
	return v.state.get(v.binding)
}

// set is the one transition for a controlled scalar. Accessors may persist, publish
// or validate a write, so assigning the value they already hold is not harmless just
// because Bind would make it look like an ordinary variable assignment.
func (v *ownedValue[T]) set(value T) bool {
	return v.state.set(v.binding, value)
}

// valueState is the single scalar-ownership behavior. Constructed controllers carry
// their accessor inside ownedValue; zero-value form fields keep configuration public
// and supply it to these methods. In either shape, a binding is the value and local
// storage is dormant rather than a synchronized shadow copy.
type valueState[T comparable] struct{ local T }

func (v *valueState[T]) get(binding Accessor[T]) T {
	if binding != nil {
		return binding.Value()
	}
	return v.local
}

func (v *valueState[T]) set(binding Accessor[T], value T) bool {
	if v.get(binding) == value {
		return false
	}
	if binding != nil {
		binding.Set(value)
		return true
	}
	v.local = value
	return true
}
