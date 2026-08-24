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
	generation  uint64
}

type frameStamp struct {
	transaction *transaction
	generation  uint64
}

func (f Frame) stamp() frameStamp {
	if !f.active() {
		return frameStamp{}
	}
	return frameStamp{transaction: f.transaction, generation: f.generation}
}

// Sub returns a child frame over r, whose coordinates begin at zero.
func (f Frame) Sub(r image.Rectangle) Frame {
	return Frame{View: f.View.Sub(r), transaction: f.transaction, generation: f.generation}
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
// Snapshot commits T by ordinary Go assignment; it does not clone or synchronize
// data reachable through pointers, slices, maps or interfaces inside T. Presentation
// data behind such references must therefore be independently owned or treated as
// immutable by its producers and consumers. A reference deliberately used as a live
// identity or behavior, such as a Widget, keeps that reference's normal semantics.
//
// One Snapshot may be staged once in a root frame. Sharing it between siblings is an
// ownership error and panics instead of making the sibling drawn last win. Refine a
// staged rich-model value through the value returned by its Stage operation rather
// than staging the same owner again.
//
// The zero value contains the zero T and is ready to stage. A Snapshot must not be
// copied after first use: its pending value is enlisted with exactly one transaction.
type Snapshot[T any] struct {
	noCopy noCopy

	current T
	pending T
	staged  *transaction
}

// Value returns the last completely drawn value by ordinary Go assignment. See
// [Snapshot] for the ownership rule when T contains references.
func (s *Snapshot[T]) Value() T {
	if s == nil {
		var zero T
		return zero
	}
	return s.current
}

// Stage prepares value for publication with frame's complete root draw. It must be
// called at most once for this Snapshot in one frame. See [Snapshot] for the ownership
// rule when value contains references.
func (s *Snapshot[T]) Stage(frame Frame, value T) {
	if s == nil {
		return
	}
	frame.enlist(s, &s.staged)
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
	states     []stagedState
	active     bool
	generation uint64
}

func (f Frame) active() bool {
	return f.transaction != nil && f.transaction.active && f.generation == f.transaction.generation
}

// enlist gives one piece of presentation state its sole position in this root frame.
// One registration per frame is what makes sibling order irrelevant: two components
// cannot race to make the last pending value win. A state object still owned by a
// different active root is the same ownership error under another spelling.
func (f Frame) enlist(state stagedState, staged **transaction) {
	if !f.active() {
		panic("headless: presentation state staged outside its Root.Draw frame")
	}
	if *staged != nil {
		if *staged == f.transaction {
			panic("headless: presentation state staged twice in one frame")
		}
		panic("headless: presentation state staged by two roots")
	}
	*staged = f.transaction
	f.transaction.states = append(f.transaction.states, state)
}

// owns reports whether a short-lived layout still belongs to the active frame that
// created it. It is used for refinements of one staged value, never to register a
// second producer.
func (f Frame) owns(staged *transaction) bool {
	return f.active() && staged == f.transaction
}

func (t *transaction) begin() {
	if t.active {
		panic("headless: Root.Draw is not reentrant")
	}
	t.states = t.states[:0]
	t.generation++
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
// The zero Root draws nothing and declines input. A Root must not be copied after
// first use: its transaction, committed tree and held gesture are one frame owner.
type Root struct {
	noCopy noCopy

	of Widget

	presentation Snapshot[Widget]
	tx           transaction
	// held is the presented root that accepted a press. Root is itself an ownership
	// boundary when Of is replaced, so it must not hand the rest of that gesture to
	// the replacement.
	held Interactive
}

// NewRoot wraps a live widget tree in its presentation transaction.
func NewRoot(of Widget) *Root {
	r := &Root{}
	r.SetContent(of)
	return r
}

// Content returns the widget tree used to build the next frame. Input continues to
// target the last completely drawn tree until another Draw commits the replacement.
func (r *Root) Content() Widget {
	if r == nil {
		return nil
	}
	return r.of
}

// SetContent replaces the widget tree used to build the next frame. An accepted
// pointer gesture remains owned by its original target through release; replacing a
// root cannot hand half a gesture to a different tree.
func (r *Root) SetContent(of Widget) {
	if r == nil {
		return
	}
	r.of = of
}

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
	frame := Frame{View: view, transaction: &r.tx, generation: r.tx.generation}
	r.presentation.Stage(frame, r.of)
	if r.of != nil {
		r.of.Draw(frame)
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
