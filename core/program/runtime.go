package program

import "time"

// Runtime is the program resource owned by the interface goroutine.
//
// It is concrete rather than a provider-defined interface: consumers that need
// only a subset declare that interface where they use it. Background work receives
// only [Runtime.Dispatcher], preserving ownership in the type system. Host features
// are grouped into the concrete [Environment], [Clipboard], [Session] and [Images]
// values rather than flattened into one capability catalogue. The zero value is
// inert; it is safe to embed in an object that has not been attached to a program.
type Runtime struct{ p *program }

// owner centralizes the inert zero-value contract for operations that need the
// live interface goroutine. It deliberately returns the concrete internal owner:
// callers stay inside this package, while capability consumers receive the narrow
// values exposed below.
func (r *Runtime) owner() *program {
	if r == nil {
		return nil
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

// Environment returns the host facts available to this runtime.
func (r *Runtime) Environment() Environment {
	return Environment{host: r.services()}
}

// Clipboard returns the runtime's clipboard capability.
func (r *Runtime) Clipboard() Clipboard {
	return Clipboard{host: r.services()}
}

// Session returns the terminal-session capability owned by this runtime.
func (r *Runtime) Session() Session { return Session{runtime: r} }

// Images returns the runtime's image capability.
func (r *Runtime) Images() Images {
	return Images{host: r.services()}
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
		p.tasks.post(nil)
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

// After schedules fn once on the interface goroutine after d. A non-positive delay
// makes fn ready for the next owner turn; it does not call fn inline. The returned
// stop function is concurrency-safe and idempotent. Stop prevents work that has not
// begun; when it races with the callback starting, either may win.
func (r *Runtime) After(d time.Duration, fn func()) (stop func()) {
	p := r.owner()
	if p == nil || fn == nil {
		return func() {}
	}
	lifetime := newClockLifetime()
	dispatch := r.Dispatcher()
	go func() {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
			if lifetime.cancelled.Load() {
				return
			}
			dispatch.Post(func() {
				if !lifetime.cancelled.Load() {
					fn()
				}
			})
		case <-lifetime.done:
		case <-p.tasks.done:
		}
	}()
	return lifetime.Stop
}

// Every schedules coalesced ticks on the interface goroutine. A non-positive
// interval or nil fn schedules nothing.
func (r *Runtime) Every(d time.Duration, fn func()) (stop func()) {
	p := r.owner()
	if p == nil || d <= 0 || fn == nil {
		return func() {}
	}
	lifetime := newClockLifetime()
	ticks := &coalescedTicks{lifetime: lifetime, dispatch: r.Dispatcher(), fn: fn}
	go func() {
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !ticks.post() {
					return
				}
			case <-lifetime.done:
				return
			case <-p.tasks.done:
				return
			}
		}
	}()
	return lifetime.Stop
}
