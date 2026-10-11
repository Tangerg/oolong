// Package headless provides interactive behavior without prescribing appearance.
// Widgets belong to one event goroutine and use local, clipped coordinates.
// Measurement precedes drawing; [Sized] supplies height at a width. [Doer] separates
// widget actions from [keymap.Map] bindings so menus and keys can share behavior.
package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/text"
)

// Widget draws into a positioned, clipped view so child code cannot paint outside its allocation.
type Widget interface {
	Draw(frame Frame)
}

// Sized is a widget that reports its height at a width. HeightForWidth is asked
// before Draw and must agree with it. Horizontal intrinsic sizing is a separate
// optional WidthForHeight(height int) int capability.
type Sized interface {
	Widget
	HeightForWidth(width int) int
}

// Block is passive drawable content. It carries no routing or frame lifetime, so
// committed content can leave the interactive tree for transcripts or scrollback.
type Block interface {
	grid.Drawable
}

// RowGutter decorates visible visual rows. Width receives the logical line count
// before wrapping; [text.Row] identifies continuations. Text and input geometry
// remain owned by the content component.
type RowGutter interface {
	Width(lines int) int
	Draw(view grid.View, rows []text.Row)
}

// Static adapts a passive [Block] to a live [Widget]; only placement becomes interactive.
type Static struct{ Of Block }

// Draw draws the passive block into frame.
func (s Static) Draw(frame Frame) {
	if s.Of != nil {
		s.Of.Draw(frame.View)
	}
}

// HeightForWidth forwards the block's measurement.
func (s Static) HeightForWidth(across int) int {
	if s.Of == nil {
		return 0
	}
	return s.Of.HeightForWidth(across)
}

// Interactive is a widget that handles input.
type Interactive interface {
	Widget
	Handle(event input.Event) bool
}

// Doer accepts named actions independently of key bindings. False permits another
// handler to try; true indicates acceptance, including actions that change no state.
type Doer interface {
	Do(action keymap.Action) bool
}
