package headless

import (
	"image"

	"github.com/Tangerg/oolong/components/internal/identity"
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
//
// Staging presentation state through a Frame no active [Root.Draw] owns — a zero value,
// or one kept past the frame it came from — is a programmer error and panics, as does
// staging one state object twice in a frame or from two roots. Each is two producers
// for a value only one can win, and the losing write would otherwise surface much
// later as a widget scrolled or focused somewhere nobody asked it to be.
type Frame struct {
	grid.View
	transaction *transaction
	generation  uint64
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
// Snapshot commits T by ordinary Go assignment and clones nothing reachable through
// pointers, slices, maps or interfaces inside it, so presentation data behind such a
// reference must be independently owned or immutable. A reference deliberately used as
// a live identity, such as a Widget, keeps its normal semantics.
//
// One Snapshot may be staged once in a root frame. Sharing it between siblings panics
// instead of making the sibling drawn last win; refine a staged value through the one
// its Stage operation returned.
//
// The zero value contains the zero T and is ready to stage. A Snapshot must not be
// copied after first use: its pending value is enlisted with exactly one transaction.
// A nil *Snapshot is a programmer error; methods panic.
type Snapshot[T any] struct {
	noCopy noCopy

	current T
	pending T
	staged  *transaction
	// Surviving attachment identities do not keep a geometry borrow valid across redraws.
	revision uint64
}

// Value returns the last completely drawn value by ordinary Go assignment. See
// [Snapshot] for the ownership rule when T contains references.
func (s *Snapshot[T]) Value() T {
	return s.current
}

func (s *Snapshot[T]) borrow() (T, uint64) { return s.current, s.revision }

func (s *Snapshot[T]) unchanged(revision uint64) bool { return s.revision == revision }

// Stage prepares value for publication with frame's complete root draw. It must be
// called at most once for this Snapshot in one frame. See [Snapshot] for the ownership
// rule when value contains references.
func (s *Snapshot[T]) Stage(frame Frame, value T) {
	frame.enlist(s, &s.staged)
	s.pending = value
}

func (s *Snapshot[T]) commit(tx *transaction) {
	if s.staged != tx {
		return
	}
	s.current = s.pending
	s.revision++
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
// tree returns. Handle enters the last complete tree and uses committed pointer
// geometry; keyboard destinations belong to that tree's focus controllers. Root depends on grid and
// input; program sees it structurally as its consumer-defined Component.
//
// The zero Root draws nothing and declines input. A Root must not be copied after
// first use: its transaction, committed tree and held gesture are one frame owner.
type Root struct {
	noCopy noCopy

	of Widget

	presentation Snapshot[rootPlacement]
	tx           transaction
	// capture is the presented root that accepted a press. Root is itself an ownership
	// boundary when Of is replaced, so it must not hand the rest of that gesture to
	// the replacement.
	capture pointerCapture[rootPlacement]
	hover   rootPlacement
}

type rootPlacement struct {
	child    Widget
	identity *byte
	visible  image.Rectangle
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
//
// It is not reentrant, and a widget that reaches it again while it is running — by
// drawing a nested Root, or by drawing from a callback this frame invoked — is a
// programmer error and panics. The commit is what makes routing geometry appear all at
// once; an inner Draw would publish a tree the outer one had not finished building, so
// [Root.Handle] would route the next event against a mixture of two frames.
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
	id := r.presentation.Value().identity
	if r.of == nil {
		id = nil
	} else if id == nil || !identity.Same(r.presentation.Value().child, r.of) {
		id = new(byte)
	}
	r.presentation.Stage(frame, rootPlacement{child: r.of, identity: id, visible: view.Visible()})
	if r.of != nil {
		r.of.Draw(frame)
	}
	r.tx.commit()
}

// Handle offers input to the last completely drawn tree. A root replaced before its
// next frame remains the input target the user can see. A root that accepted a pointer
// press also receives that gesture's drag and release even if it is replaced meanwhile.
// Capture ends on release of the pressed button or an unspecified [input.ButtonNone]
// release; a release naming another button does not end it.
// Reentrant pointer input or redraw consumes an interrupted route so it cannot fall
// through to replacement content.
func (r *Root) Handle(event input.Event) bool {
	if r == nil {
		return false
	}
	if mouse, ok := event.(input.Mouse); ok {
		return r.mouse(mouse)
	}
	presented := r.presentation.Value()
	interactive, ok := presented.child.(Interactive)
	if !ok {
		return false
	}
	handled := interactive.Handle(event)
	return handled || r.presentation.Value().identity != presented.identity
}

func (r *Root) mouse(event input.Mouse) bool {
	held, turn := r.capture.begin(event)
	if !r.capture.current(turn) {
		return true
	}
	if event.Action == input.MouseCancel {
		return false
	}
	if event.Action == input.MouseLeave {
		return r.leave(event, turn)
	}
	if held.continues(event) {
		target := held.target
		_, revision := r.presentation.borrow()
		r.hoverTo(target, event)
		if !r.capture.current(turn) || !r.presentation.unchanged(revision) || r.hover.identity != target.identity {
			return true
		}
		handled := r.capture.deliver(turn, target, event)
		return handled || !r.capture.current(turn) || !r.presentation.unchanged(revision)
	}
	return r.route(event, turn)
}

func (r *Root) route(event input.Mouse, turn uint64) bool {
	target, revision := r.presentation.borrow()
	_, ok := target.child.(Interactive)
	if !ok || !event.Pos.In(target.visible) {
		return r.leave(event, turn)
	}
	r.hoverTo(target, event)
	if !r.capture.current(turn) || !r.presentation.unchanged(revision) || r.hover.identity != target.identity {
		return true
	}
	handled := r.capture.deliver(turn, target, event)
	return handled || !r.capture.current(turn) || !r.presentation.unchanged(revision)
}

func (r *Root) leave(event input.Mouse, turn uint64) bool {
	_, revision := r.presentation.borrow()
	r.hoverTo(rootPlacement{}, event)
	return !r.capture.current(turn) || !r.presentation.unchanged(revision)
}

func (r *Root) hoverTo(to rootPlacement, event input.Mouse) {
	previous := r.hover
	r.hover = to
	if previous.identity == to.identity {
		return
	}
	if handler, ok := previous.child.(Interactive); ok {
		event.Action = input.MouseLeave
		handler.Handle(event)
	}
}

func (to rootPlacement) deliverPointer(event input.Mouse) bool {
	handler, ok := to.child.(Interactive)
	return ok && handler.Handle(event)
}
