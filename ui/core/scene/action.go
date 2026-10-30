package scene

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ActionID names a semantic operation independently of its input binding.
type ActionID string

// Standard action identifiers let different appearances expose the same operations.
const (
	ActionActivate  ActionID = "activate"
	ActionSubmit    ActionID = "submit"
	ActionDismiss   ActionID = "dismiss"
	ActionSetValue  ActionID = "set-value"
	ActionFill      ActionID = "fill"
	ActionSelect    ActionID = "select"
	ActionExpand    ActionID = "expand"
	ActionCollapse  ActionID = "collapse"
	ActionIncrement ActionID = "increment"
	ActionDecrement ActionID = "decrement"
	ActionUndo      ActionID = "undo"
	ActionRedo      ActionID = "redo"
)

// ActionState is the owner-reported outcome of an operation.
type ActionState uint8

// Action outcomes come from the owner; consumption and revision guesses are insufficient.
const (
	Unchanged ActionState = iota
	Applied
	Started
)

// ActionResult carries a typed value and an explicit change or start outcome.
type ActionResult[T any] struct {
	State ActionState
	Value T
}

// Availability is a pure precondition projection. Zero allows the operation;
// execution still checks it immediately before calling its owner.
type Availability struct {
	Disabled bool
	Reason   string
}

// ActionConfig supplies one operation and its pure availability projection.
type ActionConfig[P, R any] struct {
	ID           ActionID
	Label        string
	Availability func(P) Availability
	Run          func(context.Context, P) (ActionResult[R], error)
}

// Action preserves compile-time parameter and result types for ordinary callers.
type Action[P, R any] struct{ config ActionConfig[P, R] }

// NewAction creates a typed operation. It panics for an empty identifier or nil handler.
func NewAction[P, R any](cfg ActionConfig[P, R]) *Action[P, R] {
	if cfg.ID == "" || cfg.Run == nil {
		panic("scene: action requires an identifier and handler")
	}
	cfg.ID = ActionID(strings.Clone(string(cfg.ID)))
	cfg.Label = strings.Clone(cfg.Label)
	return &Action[P, R]{config: cfg}
}

// Availability projects whether these parameters can currently execute without changing state.
func (a *Action[P, R]) Availability(params P) Availability {
	if a == nil {
		return Availability{Disabled: true, Reason: "missing action"}
	}
	if a.config.Availability != nil {
		return a.config.Availability(params)
	}
	return Availability{}
}

// Invoke checks context and current availability before calling the owner handler.
func (a *Action[P, R]) Invoke(ctx context.Context, params P) (ActionResult[R], error) {
	var zero ActionResult[R]
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if available := a.Availability(params); available.Disabled {
		return zero, &UnavailableError{Action: a.ID(), Reason: available.Reason}
	}
	return a.run(ctx, params)
}

func (a *Action[P, R]) run(ctx context.Context, params P) (ActionResult[R], error) {
	var zero ActionResult[R]
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	result, err := a.config.Run(ctx, params)
	if err == nil && result.State > Started {
		return zero, fmt.Errorf("%w: action returned an unknown state", ErrInvalid)
	}
	return result, err
}

// ID returns this operation's stable semantic name.
func (a *Action[P, R]) ID() ActionID {
	if a == nil {
		return ""
	}
	return a.config.ID
}

// ActionBinding is the single heterogeneous boundary used by discovery and AX.
// Ordinary application calls retain Action[P,R] and its compile-time types.
type ActionBinding struct {
	id           ActionID
	label        string
	parameter    reflect.Type
	result       reflect.Type
	availability func(any) (Availability, error)
	invoke       func(context.Context, any) (Invocation, error)
}

// Invocation is the erased result used only at the heterogeneous automation boundary.
type Invocation struct {
	State ActionState
	Value any
}

// Action errors distinguish missing operations from invalid parameter types.
var (
	ErrActionNotFound = errors.New("scene: action not found")
	ErrArgument       = errors.New("scene: action parameter has the wrong type")
)

// UnavailableError reports why a current target or operation cannot execute.
type UnavailableError struct {
	Action ActionID
	Reason string
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("scene: %s unavailable: %s", e.Action, e.Reason)
}
func (e *UnavailableError) Unwrap() error { return ErrUnavailable }

// Binding erases an action once for heterogeneous scene discovery while retaining exact runtime types.
func (a *Action[P, R]) Binding() ActionBinding {
	if a == nil {
		return ActionBinding{}
	}
	return ActionBinding{
		id: a.config.ID, label: a.config.Label,
		parameter: reflect.TypeFor[P](), result: reflect.TypeFor[R](),
		availability: func(value any) (Availability, error) {
			params, err := actionParameter[P](value)
			if err != nil {
				return Availability{}, err
			}
			return a.Availability(params), nil
		},
		invoke: func(ctx context.Context, value any) (Invocation, error) {
			params, err := actionParameter[P](value)
			if err != nil {
				return Invocation{}, err
			}
			result, err := a.run(ctx, params)
			return Invocation{State: result.State, Value: result.Value}, err
		},
	}
}

func actionParameter[T any](value any) (T, error) {
	var zero T
	if value == nil {
		typ := reflect.TypeFor[T]()
		if typ.Kind() == reflect.Struct && typ.NumField() == 0 {
			return zero, nil
		}
	}
	if typed, ok := value.(T); ok {
		return typed, nil
	}
	return zero, fmt.Errorf("%w: expected %s, got %T", ErrArgument, reflect.TypeFor[T](), value)
}

// ID returns the semantic operation name exposed by this binding.
func (a ActionBinding) ID() ActionID { return a.id }

// Invoke resolves an exact current attachment, validates scope and calls its typed operation.
func (r *Root) Invoke(ctx context.Context, target Target, action ActionID, params any) (Invocation, error) {
	if err := r.mutable(); err != nil {
		return Invocation{}, err
	}
	if err := ctx.Err(); err != nil {
		return Invocation{}, err
	}
	n := r.resolve(target)
	if n == nil {
		return Invocation{}, ErrStale
	}
	if !r.allowed(n) {
		return Invocation{}, ErrUnavailable
	}
	for _, binding := range n.config.Actions {
		if binding.id != action {
			continue
		}
		var queryErr error
		availability := r.project(func() Availability {
			available, err := binding.availability(params)
			queryErr = err
			return available
		})
		if queryErr != nil {
			return Invocation{}, queryErr
		}
		if availability.Disabled {
			return Invocation{}, &UnavailableError{Action: action, Reason: availability.Reason}
		}
		before := r.modelRevision
		result, err := binding.invoke(ctx, params)
		if err == nil && result.State != Unchanged && before == r.modelRevision {
			r.Changed()
		}
		return result, err
	}
	return Invocation{}, ErrActionNotFound
}
