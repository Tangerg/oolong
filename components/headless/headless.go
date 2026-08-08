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
// with no appearance in it. An appearance layer supplies those decisions through
// callbacks and styles without this package knowing which one was chosen.
//
// # How a widget works
//
// A widget is a mutable object owned by one event goroutine. It is asked to draw itself into
// the space it was given, and asked whether it wants an event. It does not return a
// new copy of itself, and it does not know where on the screen it is: the view it is
// handed is already positioned and already clipped, so a widget's coordinates are
// its own and it cannot draw outside its box.
//
// Measurement is separate from drawing because a container has to know how much its
// children want before it can decide where they go. A widget whose size along one
// axis follows from the other says so by implementing [Sized].
//
// # How a key reaches a widget
//
// A widget names what it can do and answers to the name — see [Doer] — and an
// [keymap.Map] says which keystrokes produce which name. Nothing here owns a
// keystroke. That is what makes every key reboundable without replacing anything, what
// makes a binding several chords long expressible at all, and what lets the same action
// be reached from a menu or from a command typed by name.
//
// Each widget kind has a map of its own, because the same key means different things
// in different places: the down arrow moves a cursor in a field, a selection in a list
// and a window in a reader, and one table cannot say all three. Within a kind, one map
// can serve a whole interface — a widget answers the actions it knows and lets the rest
// past — which is how a program binds its own keys alongside a field's:
//
//	keys := headless.DefaultEditorKeys()
//	keys.Bind("send", input.Chord{Code: input.Enter})
package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Widget draws itself into the space it is given.
//
// The view is already positioned and clipped. A widget that draws outside it is not
// a bug that shows on screen — the drawing is simply discarded — which is what makes
// the box a boundary rather than a convention.
type Widget interface {
	Draw(frame Frame)
}

// Sized is a widget whose size along the axis being divided follows from the room it
// has across the other: wrapped text, a list of variable-height rows, anything that
// reflows.
//
// It is [layout.Measurer] and nothing more, so a sized widget goes straight into a
// [layout.Slot]:
//
//	rects := layout.Down.Rects(v.Bounds().Size(),
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

// Block is finished or deliberately retained drawable content.
//
// Unlike a live [Widget], a Block has no routing geometry and draws directly into a
// grid view. This is the shape accepted by transcripts and inline publication: once a
// block is committed, it can leave the active component tree without carrying a frame
// transaction or interaction lifecycle with it.
type Block interface {
	Draw(view grid.View)
	layout.Measurer
}

// RowGutter draws decoration beside visual text rows.
//
// Width is asked with the number of logical lines before the content wraps them.
// Draw receives only the rows visible in the current frame; [text.Row.Line] says
// which logical line each came from and Joined distinguishes its continuations.
// The row text is provided so a gutter can derive diagnostics from it, but the
// content owner still owns the text and input geometry.
//
// This is an appearance seam rather than an appearance decision. A line-number,
// breakpoint or diagnostic gutter can live in a higher package while the text
// component stays independent of all of them.
type RowGutter interface {
	Width(lines int) int
	Draw(view grid.View, rows []text.Row)
}

// Static adapts a passive [Block] into a measured live [Widget].
//
// It is the explicit bridge for a document or other finished value shown in a
// [Viewport]. The block remains passive; the active tree owns only its placement.
type Static struct{ Of Block }

// Draw draws the passive block into frame.
func (s Static) Draw(frame Frame) {
	if s.Of != nil {
		s.Of.Draw(frame.View)
	}
}

// Measure forwards the block's measurement.
func (s Static) Measure(across int) int {
	if s.Of == nil {
		return 0
	}
	return s.Of.Measure(across)
}

// Interactive is a widget that answers input.
//
// Any consumer of the same drawing and event method set can use an Interactive
// without an adapter, while neither side knows the other exists.
type Interactive interface {
	Widget
	Handle(event input.Event) bool
}

// Doer is a widget that can be asked for one of its actions by name.
//
// It is the other half of [keymap.Map]. A widget names what it can do and answers to
// the name; the map says which keystrokes produce which name. Neither knows the other,
// which is what lets a program rebind every key without touching a widget, and what
// lets the same action be reached from somewhere that is not the keyboard at all — a
// menu, a command typed by name, a test that presses nothing.
//
// Do reports whether the action was one this widget knows. An action it does not know
// is not an error: one keymap often drives a whole interface, and every widget reading
// through it answers what it recognises and lets the rest past.
type Doer interface {
	Do(action keymap.Action) bool
}
