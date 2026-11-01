package program

import (
	"context"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// Runtime is the program resource owned by the interface goroutine.
//
// It is concrete rather than a provider-defined interface: consumers that need
// only a subset declare that interface where they use it. Background work receives
// only [Runtime.Dispatcher], preserving ownership in the type system. Host features
// are grouped into the concrete [Environment], [Clipboard], [Session] and [Images]
// values rather than flattened into one capability catalogue. The zero value is
// inert; it is safe to embed in an object that has not been attached to a program.
// Run's completion or a failed builder ends owner operations and revokes retained
// Clipboard, Session and Images values. Environment snapshots remain readable.
type Runtime struct{ p *program }

// owner centralizes the inert zero-value contract for operations that need the
// live interface goroutine. It deliberately returns the concrete internal owner:
// callers stay inside this package, while capability consumers receive the narrow
// values exposed below.
func (r *Runtime) owner() *program {
	if r == nil {
		return nil
	}
	if r.p == nil {
		return nil
	}
	select {
	case <-r.p.tasks.done:
		return nil
	default:
	}
	return r.p
}

// services centralizes the zero Runtime contract for capability objects. Runtime
// operations that need the live owner still check it explicitly; a missing owner
// and a host with no optional services are equivalent only here.
func (r *Runtime) services() hostServices {
	p := r.owner()
	if p == nil {
		return hostServices{}
	}
	return p.host
}

// Environment returns the immutable startup facts, including after Run ends.
func (r *Runtime) Environment() Environment {
	if r == nil || r.p == nil {
		return Environment{}
	}
	return r.p.environment
}

// Clipboard returns the runtime's clipboard capability.
func (r *Runtime) Clipboard() Clipboard {
	return Clipboard{runtime: r}
}

// Session returns the terminal-session capability owned by this runtime.
func (r *Runtime) Session() Session { return Session{runtime: r} }

// Images returns the runtime's image capability.
func (r *Runtime) Images() Images {
	return Images{runtime: r}
}

// Dispatcher returns the concurrency-safe handle for background work.
func (r *Runtime) Dispatcher() Dispatcher {
	p := r.owner()
	if p == nil {
		return Dispatcher{}
	}
	return Dispatcher{tasks: p.tasks}
}

// Refresh requests a frame without changing component state.
func (r *Runtime) Refresh() {
	if p := r.owner(); p != nil {
		_ = p.tasks.post(nil)
	}
}

// Quit asks the program to stop.
func (r *Runtime) Quit() {
	p := r.owner()
	if p == nil {
		return
	}
	p.quit.Store(true)
	// Wake a parked loop so it can observe the transition. The signal carries no
	// task and coalesces with any wake-up already waiting.
	p.tasks.signal()
}

// Scene grants owner-side semantic access without transferring frame or transport
// authority. A handle obtained during Build begins observing after attachment.
// Background callers enter through Dispatcher.Dispatch before using it.
func (r *Runtime) Scene() Scene { return Scene{runtime: r} }

// Scene follows one runtime's attached UI. Its zero value and retained handles
// after Run report ErrStopped; a live builder without a root reports scene.ErrDetached.
type Scene struct{ runtime *Runtime }

func (s Scene) root() (*scene.Root, error) {
	p := s.runtime.owner()
	if p == nil {
		return nil, ErrStopped
	}
	if p.root == nil {
		return nil, scene.ErrDetached
	}
	return p.root, nil
}

// Observe reads bounded semantic evidence from the current attachment on the UI owner.
func (s Scene) Observe(query scene.Query) (scene.Observation, error) {
	root, err := s.root()
	if err != nil {
		return scene.Observation{}, err
	}
	return root.Observe(query)
}

// Invoke routes a semantic action to the same attachment and controller as human input.
func (s Scene) Invoke(ctx context.Context, target scene.Target, action scene.ActionID, params any) (scene.Invocation, error) {
	root, err := s.root()
	if err != nil {
		return scene.Invocation{}, err
	}
	return root.Invoke(ctx, target, action, params)
}

// Changes closes on the next attached scene transition. Before attachment it is
// nil; after the session stops, or on the zero value, it is already closed.
func (s Scene) Changes() <-chan struct{} {
	p := s.runtime.owner()
	if p == nil {
		return closedSignal
	}
	return p.root.Changes()
}
