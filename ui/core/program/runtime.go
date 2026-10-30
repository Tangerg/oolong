package program

import (
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

// Scene is the attached scene for owner-side composition and semantic observation.
// Background callers enter through Dispatcher.Inspect or Invoke before using it.
func (r *Runtime) Scene() *scene.Root {
	if p := r.owner(); p != nil {
		return p.root
	}
	return nil
}
