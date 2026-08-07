package headless

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// Frame is the drawing space of one headless component frame.
//
// The embedded grid view is already positioned and clipped. Sub and Subs preserve
// the presentation transaction while deriving child views; code drawing a passive
// [Block] can pass View to it directly.
//
// A Frame is created by [Root]. Components must not construct one: the transaction is
// what prevents routing geometry from becoming visible before the complete root frame
// has been built.
type Frame struct {
	grid.View
	transaction *transaction
}

// Sub returns a child frame over r, whose coordinates begin at zero.
func (f Frame) Sub(r image.Rectangle) Frame {
	return Frame{View: f.View.Sub(r), transaction: f.transaction}
}

// Subs returns child frames over rects, preserving their order.
func (f Frame) Subs(rects []image.Rectangle) []Frame {
	out := make([]Frame, len(rects))
	for i, rect := range rects {
		out[i] = f.Sub(rect)
	}
	return out
}

// Snapshot holds derived presentation state committed with a complete [Root] frame.
//
// Value is the geometry or other recomputable presentation fact used by Handle.
// Stage makes a replacement visible only when the complete root Draw returns. A
// panic or another aborted draw releases the pending value and leaves Value unchanged.
// Snapshot is not application state: using it for semantic values would make Draw
// advance meaning and violate the ownership model.
//
// The zero value contains the zero T and is ready to stage.
type Snapshot[T any] struct {
	current T
	pending T
	staged  *transaction
}

// Value returns the last completely drawn value.
func (s *Snapshot[T]) Value() T {
	if s == nil {
		var zero T
		return zero
	}
	return s.current
}

// Stage prepares value for publication with frame's complete root draw.
func (s *Snapshot[T]) Stage(frame Frame, value T) {
	if s == nil {
		return
	}
	if frame.transaction == nil || !frame.transaction.active {
		panic("headless: presentation state staged outside Root.Draw")
	}
	if s.staged != frame.transaction {
		if s.staged != nil {
			panic("headless: presentation state staged by two roots")
		}
		s.staged = frame.transaction
		frame.transaction.states = append(frame.transaction.states, s)
	}
	s.pending = value
}

func (s *Snapshot[T]) commit(tx *transaction) {
	if s.staged != tx {
		return
	}
	s.current = s.pending
	var zero T
	s.pending = zero
	s.staged = nil
}

func (s *Snapshot[T]) abort(tx *transaction) {
	if s.staged != tx {
		return
	}
	var zero T
	s.pending = zero
	s.staged = nil
}

type stagedState interface {
	commit(tx *transaction)
	abort(tx *transaction)
}

type transaction struct {
	states []stagedState
	active bool
}

func (t *transaction) begin() {
	if t.active {
		panic("headless: Root.Draw is not reentrant")
	}
	t.states = t.states[:0]
	t.active = true
}

func (t *transaction) commit() {
	for _, state := range t.states {
		state.commit(t)
	}
	clear(t.states)
	t.states = t.states[:0]
	t.active = false
}

func (t *transaction) abort() {
	for _, state := range t.states {
		state.abort(t)
	}
	clear(t.states)
	t.states = t.states[:0]
	t.active = false
}

// Root is the composition boundary between a headless widget tree and a program.
//
// Draw stages every nested Snapshot and publishes them together only after the whole
// tree returns. Handle therefore routes against the last complete logical frame, never
// a mixture of children from a frame still being built. Root depends only on grid and
// input; program sees it structurally as its consumer-defined Component.
//
// The zero Root draws nothing and declines input.
type Root struct {
	Of Widget

	presentation Snapshot[Widget]
	tx           transaction
	// held is the presented root that accepted a press. Root is itself an ownership
	// boundary when Of is replaced, so it must not hand the rest of that gesture to
	// the replacement.
	held Interactive
}

// NewRoot wraps a live widget tree in its presentation transaction.
func NewRoot(of Widget) *Root { return &Root{Of: of} }

// Draw builds and atomically commits one logical component frame.
func (r *Root) Draw(view grid.View) {
	if r == nil {
		return
	}
	r.tx.begin()
	defer func() {
		if r.tx.active {
			r.tx.abort()
		}
	}()
	frame := Frame{View: view, transaction: &r.tx}
	r.presentation.Stage(frame, r.Of)
	if r.Of != nil {
		r.Of.Draw(frame)
	}
	r.tx.commit()
}

// Handle offers input to the last completely drawn tree. A root replaced before its
// next frame remains the input target the user can see. A root that accepted a pointer
// press also receives that gesture's drag and release even if it is replaced meanwhile.
func (r *Root) Handle(event input.Event) bool {
	if r == nil {
		return false
	}
	if mouse, ok := event.(input.Mouse); ok {
		return r.mouse(mouse)
	}
	interactive, ok := r.presentation.Value().(Interactive)
	return ok && interactive.Handle(event)
}

func (r *Root) mouse(event input.Mouse) bool {
	if r.held != nil && (event.Action == input.MouseDrag || event.Action == input.MouseUp) {
		target := r.held
		if event.Action == input.MouseUp {
			r.held = nil
		}
		return target.Handle(event)
	}
	if event.Action == input.MouseDown {
		// A new press supersedes an incomplete old gesture, whether or not the new
		// target accepts it.
		r.held = nil
	}
	target, ok := r.presentation.Value().(Interactive)
	if !ok || !target.Handle(event) {
		return false
	}
	if event.Action == input.MouseDown {
		r.held = target
	}
	return true
}
