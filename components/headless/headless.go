// Package headless is behaviour without appearance.
//
// Everything here holds the state an interactive thing needs and answers the input
// that changes it, and none of it decides what any of that looks like. A list knows
// which item is selected and what the arrow keys do; it does not know what a
// selected row looks like, and it draws one by calling back to whoever does. A
// scroll position knows it is following the end of a log; it has no scrollbar.
//
// That division is the whole point. Appearance is where every interface differs and
// behaviour is where they are all the same, so the part worth sharing is the part
// with no appearance in it. [github.com/Tangerg/oolong/components/kit] is one set of answers to
// what these should look like; it is a default rather than a destination, and an
// interface with its own ideas builds on this package and never imports it.
//
// # How a widget works
//
// A widget is a mutable object owned by one loop. It is asked to draw itself into
// the space it was given, and asked whether it wants an event. It does not return a
// new copy of itself, and it does not know where on the screen it is: the view it is
// handed is already positioned and already clipped, so a widget's coordinates are
// its own and it cannot draw outside its box.
//
// Measurement is separate from drawing because a container has to know how much its
// children want before it can decide where they go. A widget whose size along one
// axis follows from the other says so by implementing [Sized].
package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

// Widget draws itself into the space it is given.
//
// The view is already positioned and clipped. A widget that draws outside it is not
// a bug that shows on screen — the drawing is simply discarded — which is what makes
// the box a boundary rather than a convention.
type Widget interface {
	Draw(v grid.View)
}

// Sized is a widget whose size along the axis being divided follows from the room it
// has across the other: wrapped text, a list of variable-height rows, anything that
// reflows.
//
// It is [layout.Measurer] and nothing more, so a sized widget goes straight into a
// [layout.Slot]:
//
//	views := layout.Rows(v,
//		layout.Slot{Size: layout.Measured(0, 3), Of: header},
//		layout.Slot{Size: layout.Flex(1)},
//	)
//
// Measure is asked before Draw and must agree with it. A widget that reports one
// size and draws another gets clipped or leaves a gap, and both look like a layout
// bug somewhere else.
type Sized interface {
	Widget
	layout.Measurer
}

// Interactive is a widget that answers input.
//
// Handle reports whether the event was consumed. An unconsumed event carries on to
// whatever else might want it, which is how a key can mean one thing inside a text
// field and another outside it without either side knowing about the other.
//
// It is the same method set as [github.com/Tangerg/oolong/core/program.Component], so
// anything here runs as a program's root without an adapter. The two are stated
// separately because the loop must not depend on the widgets, and a widget must not
// depend on there being a loop.
type Interactive interface {
	Widget
	Handle(ev input.Event) bool
}

// Binding is a key and what it does.
//
// It pairs the two on purpose. A keystroke handled in one place and described in
// another drift apart, and the version the user reads is the one that is wrong.
type Binding struct {
	// Key is the keystroke, as the parser reports it, so the hint and the handler
	// are talking about the same thing.
	Key input.Key
	// Does is what it does, in as few words as fit.
	Does string
	// Hidden keeps a binding out of a hint row without making it any less real — for
	// a chord that works but that nobody needs told about.
	Hidden bool
}

// Matches reports whether ev is this binding's keystroke.
func (b Binding) Matches(ev input.Event) bool {
	key, ok := ev.(input.Key)
	return ok && key.Down() && key.Is(b.Key.Code, b.Key.Mods) && key.Rune == b.Key.Rune
}
