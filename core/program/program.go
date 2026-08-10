// Package program runs a terminal interface.
//
// It owns the terminal, the frame schedule, and the one goroutine that is allowed to
// touch the interface's state. It knows nothing about what the interface is for: what
// it drives is a [Component], which draws itself and answers input, and everything a
// component needs from the program it asks for through a [Runtime].
//
// # The concurrency model, in full
//
// One goroutine draws and handles input. Anything that happens elsewhere — a request
// finishing, a file changing, a timer firing — reaches the interface through a
// [Dispatcher] obtained from [Runtime.Dispatcher], and runs there. That is the whole of
// it, and it is why state reached only from that goroutine needs no internal lock.
//
// The program parks when there is nothing to do. It wakes for input, for posted work,
// and for the frame writer settling output — never on a clock that runs regardless. A
// component that wants a clock starts one with [Runtime.After] or [Runtime.Every], and
// an interface with nothing scheduled costs nothing.
//
// # The two places an interface can be
//
// A program either takes a screen of its own, which it gives back on the way out, or
// draws in the terminal's own screen as a block with the session's output above it.
// The second is what [Config.Inline] asks for, and it is the difference between a
// program the user enters and leaves and one that is part of their session: what an
// inline interface has finished with is printed with [InlineRuntime.Print] and belongs to
// the terminal from then on — scrollable, selectable, and still there afterwards.
package program

import (
	"errors"
	"fmt"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// ErrFrameTimeout means a frame writer did not account for its pending frames
// before display ownership had to change. The program refuses the transition: a
// late frame would otherwise be written into the next owner's output.
var ErrFrameTimeout = errors.New("program: frame writer did not drain")

// ErrInvalidFrameSequence means a host accepted a non-empty frame without assigning
// it a usable position in its progress watermark. Continuing would allow a later
// frame to overtake output the presenter still owns, so publication stops instead.
var ErrInvalidFrameSequence = errors.New("program: invalid frame writer sequence")

// ErrInvalidSize means a host reported geometry that cannot safely back a program
// surface. Hosts are transport boundaries and their dimensions may come from an
// untrusted peer, so invalid input is an error rather than a grid allocation or
// panic.
var ErrInvalidSize = errors.New("program: invalid host size")

// MaxCells is the largest host-controlled program surface. A screen owns both a
// front and back cell store, so a bound belongs at the host edge before either is
// allocated. The limit admits terminals far larger than ordinary displays while
// keeping one resize from becoming an open-ended memory request.
const MaxCells = 1 << 18

// ValidateSize reports whether width and height describe a safe non-empty program
// surface. Host adapters can call it before acquiring transport resources; Run
// applies it to both the opening size and every later resize.
func ValidateSize(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: %dx%d", ErrInvalidSize, width, height)
	}
	if width > MaxCells/height {
		return fmt.Errorf("%w: %dx%d exceeds %d cells", ErrInvalidSize,
			width, height, MaxCells)
	}
	return nil
}

// Component is an interface a program can run: it draws itself into the space it is
// given, and says whether it wants an event.
//
// It is handed a view that is already positioned and clipped, so its coordinates are
// its own. An event it does not consume is dropped by the program — a component is the
// root of its own tree and there is nobody above it to pass one on to.
type Component interface {
	Draw(view grid.View)
	Handle(event input.Event) bool
}

// Dispatcher is a copyable, concurrency-safe handle into a running program.
// Its zero value drops work. It deliberately exposes no owner-only operation.
type Dispatcher struct{ tasks *taskQueue }

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

// InlineRuntime is a [Runtime] that can publish completed output into terminal
// scrollback. It is only constructed for [Config.Inline], and its zero value is
// inert.
type InlineRuntime struct{ *Runtime }

// inlineCanvas is the publication surface when this is a live inline runtime.
// Keeping the embedded-runtime and mode checks together makes every publishing
// operation share the same zero-value semantics.
func (r *InlineRuntime) inlineCanvas() *grid.Inline {
	if r == nil {
		return nil
	}
	p := r.owner()
	if p == nil {
		return nil
	}
	return p.inline
}
