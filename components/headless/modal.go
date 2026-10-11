package headless

import (
	"image"
	"slices"

	"github.com/Tangerg/oolong/components/internal/identity"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Modal is a layer that floats over an interface and takes its input while it is
// on top.
//
// It is a [Widget] that also says where it wants to go and answers events. Where
// it goes is a [layout.Placement] rather than a rectangle, so the same modal is
// placed correctly whatever it is floating over and does not have to be told the
// size of the screen.
type Modal interface {
	Widget

	// Handle answers an event, reporting whether it was consumed. Mouse positions
	// are in the modal's own coordinates: the stack has already translated them,
	// and events outside the modal never arrive here.
	//
	// Consumed, not closed. A modal that wants to close itself is built with a
	// callback by whoever pushed it, the same way a completion is — the stack does
	// not have to guess what an unconsumed key meant.
	Handle(ev input.Event) bool

	// Place says where the modal goes in the space it floats over.
	Place(space image.Point) layout.Placement
}

// Insistent is a modal that the escape key does not close.
//
// It is for the layer that has to be answered rather than dismissed — a
// confirmation, a required choice. Without it the way to make one would be to
// consume the escape key and do nothing, which reads at the call site as a bug.
type Insistent interface {
	Modal
	// Insists reports whether the modal currently refuses to be dismissed. It is a
	// method rather than a marker so a layer can stop insisting once it has what
	// it needs.
	Insists() bool
}

// Closer is a modal that wants to know when it has been popped, whether that was
// its own doing or the stack's.
// Closed runs once after removal, before focus is transferred. It may reopen the
// modal; the old removal will not close the new insertion.
type Closer interface {
	Modal
	Closed()
}

// CloseRequester owns whether a request to remove a modal is accepted.
// The stack removes the layer only after RequestClose returns true.
type CloseRequester interface{ RequestClose() bool }

// Backdrop receives the full covered view before modal drawing. Ordinary modal
// views stay clipped to their own bounds, so dimming must request this wider capability.
type Backdrop interface {
	Modal
	// Backdrop is given the whole space the stack is drawn into, before this
	// layer's own drawing and after everything below it.
	Backdrop(v grid.View)
}

// LayerID is one insertion into a [Stack].
//
// A handle, rather than Modal interface equality, makes duplicate insertions
// unambiguous and lets every valid Modal implementation be removed even when its
// concrete value is not comparable. The zero ID names no layer.
type LayerID uint64

type stackLayer struct {
	id    LayerID
	modal Modal
}

// Stack owns modal membership, focus and pointer routing. Only the committed top
// focus holder receives keys. Outside wheel and move events descend through layers;
// an outside press is consumed for dismissal. Accepted presses capture drag and
// release, including outside bounds; foreign-button releases preserve capture.
// The zero value is ready. A Stack must not be copied after first use.
type Stack struct {
	noCopy noCopy

	// base is private because replacing the focused root has to release the old one.
	// SetBase is the ownership transition; an exported field could bypass it.
	base   Widget
	baseID *byte

	// Keys say which keystrokes produce which of the actions a stack answers to, which
	// is the one that closes the top layer. Nil reads through [DefaultStackKeys].
	//
	// A map rather than one field for the one key, because the same layer appears in
	// interfaces where escape means "back" and interfaces where it means "close", and
	// which it is here is the program's to say. A layer that must be answered rather
	// than dismissed implements [Insistent].
	Keys *keymap.Map
	// KeepOnClickOutside stops a press outside the top layer from popping it.
	// Off by default, because a click on what a modal is covering means the user
	// is finished with the modal, and every interface they already use agrees.
	KeepOnClickOutside bool

	layers   []stackLayer
	layerIDs identitySequence
	// presentation is the base and layers of the last complete root frame. Pairing
	// identity with geometry keeps input on what is visible if the semantic stack is
	// changed while another frame is being prepared.
	presentation Snapshot[stackPresentation]

	// The holder and its membership are published before focus callbacks. Initial
	// settlement also blurs all other attachments — see [Focusable].
	holder     Widget
	holderID   LayerID
	holderBase bool
	focusState
	matcher      keymap.Matcher
	matcherLayer LayerID
	// capture owns the recipient of one pointer gesture, whether a base or layer.
	capture pointerCapture[stackPointerTarget]
	hover   stackPointerTarget
}

// NewStack constructs a stack over base. The zero Stack has no base and is ready.
func NewStack(base Widget) *Stack {
	s := &Stack{}
	s.SetBase(base)
	return s
}

// Base returns the interface the layers float over.
func (s *Stack) Base() Widget {
	if s == nil {
		return nil
	}
	return s.base
}

// SetBase replaces the interface beneath the layers and settles keyboard ownership.
func (s *Stack) SetBase(base Widget) {
	if s == nil {
		return
	}
	if identity.Same(s.base, base) {
		return
	}
	s.base = base
	s.baseID = new(byte)
	if s.state.Settled() && s.holderID != 0 {
		// A layer owns the keyboard. The new base starts in the state every lone
		// widget assumes — focused — so it must hear the one real transition to its
		// covered state; the layer above it has not changed owners at all.
		if !identity.Same(s.base, s.holder) {
			tell(s.base, false)
		}
		return
	}
	s.state.Unsettle()
	s.settle()
}

// Push returns a stable handle and focuses the new layer. Nil returns zero.
// Controller-owned DialogContent must use Dialog.Show or Sync; direct pushes, including
// embedded wrappers, panic. Handles are never reused; exhaustion panics before a stale
// handle could identify a different layer.
func (s *Stack) Push(m Modal) LayerID {
	if controlled, ok := m.(interface{ rejectIndependentInsertion() }); ok {
		controlled.rejectIndependentInsertion()
	}
	id := s.insert(m)
	if id != 0 {
		s.settle()
	}
	return id
}

// insert commits membership before a controller can receive focus callbacks.
func (s *Stack) insert(m Modal) LayerID {
	if m == nil {
		return 0
	}
	id, ok := s.layerIDs.next()
	if !ok {
		panic("headless: stack exhausted layer identities")
	}
	layerID := LayerID(id)
	s.layers = append(s.layers, stackLayer{id: layerID, modal: m})
	return layerID
}

// Pop requests removal of the top layer and reports whether it was removed.
// A CloseRequester can reject removal. On success the keyboard goes back to
// whatever was underneath.
func (s *Stack) Pop() bool {
	n := len(s.layers)
	if n == 0 {
		return false
	}
	return s.remove(n - 1)
}

// Remove requests removal of the insertion named by id and reports whether it was
// removed. A CloseRequester can reject removal. A controller uses this when its layer closes while another is above it;
// popping would dismiss a different control.
func (s *Stack) Remove(id LayerID) bool {
	if id == 0 {
		return false
	}
	for i := range slices.Backward(s.layers) {
		if s.layers[i].id == id {
			return s.remove(i)
		}
	}
	return false
}

// Contains reports whether id is currently in the stack.
func (s *Stack) Contains(id LayerID) bool {
	return id != 0 && slices.ContainsFunc(s.layers, func(layer stackLayer) bool {
		return layer.id == id
	})
}

// Clear pops every layer, from the top down, so each is told in the order it
// would have been dismissed. It stops if a CloseRequester rejects removal.
func (s *Stack) Clear() {
	for s.Pop() {
	}
}

// Top is the layer with the input, or nil when there is none.
func (s *Stack) Top() Modal {
	top, ok := s.top()
	if !ok {
		return nil
	}
	return top.modal
}

func (s *Stack) top() (stackLayer, bool) {
	if len(s.layers) == 0 {
		return stackLayer{}, false
	}
	return s.layers[len(s.layers)-1], true
}

// Depth is how many layers there are.
func (s *Stack) Depth() int { return len(s.layers) }

// Area is where the top layer was last drawn, and whether there is one.
func (s *Stack) Area() (image.Rectangle, bool) {
	presented := s.presentation.Value()
	if len(presented.layers) == 0 {
		return image.Rectangle{}, false
	}
	return presented.layers[len(presented.layers)-1].area, true
}

// Handle gives the event to the top layer, and reports whether the stack dealt
// with it.
//
// An empty stack consumes nothing, so an interface can offer it every event and
// carry on when it is not interested. A stack with anything in it consumes every
// key, because a key reaching what a modal is covering is a keystroke going
// somewhere the user cannot see.
func (s *Stack) Handle(ev input.Event) bool {
	presented := s.presentation.Value()
	if mouse, ok := ev.(input.Mouse); ok {
		return s.mouse(presented, mouse)
	}
	s.settle()
	top, layered := s.top()
	if top.id != s.matcherLayer {
		if !s.matcher.Reset(func() { s.matcherLayer = top.id }) {
			return true
		}
	}
	if s.holderID != top.id || !s.keyboardPresented(presented) {
		return true
	}
	if !layered {
		if handler, ok := s.holder.(Interactive); ok {
			return s.state.Forward(func() bool { return handler.Handle(ev) })
		}
		return false
	}
	if s.state.Forward(func() bool { return top.modal.Handle(ev) }) {
		return true
	}
	if key, ok := ev.(input.Key); ok {
		s.matcher.Handle(s.keys(), key, func(action keymap.Action) bool {
			if action != Close || s.holderID != top.id {
				return false
			}
			if !sticky(top.modal) {
				s.dismiss(top.id)
			}
			return true
		})
	}
	// Consumed either way: what is underneath is covered, and a key that fell
	// through to it would act somewhere the user is not looking.
	return true
}

func (s *Stack) keyboardPresented(presented stackPresentation) bool {
	if len(presented.layers) > 0 {
		return presented.layers[len(presented.layers)-1].id == s.holderID
	}
	return s.holderID == 0 && presented.baseID == s.baseID
}

func (s *Stack) mouse(presented stackPresentation, mouse input.Mouse) bool {
	held, turn := s.capture.begin(mouse)
	if !s.capture.current(turn) || mouse.Action == input.MouseCancel {
		return false
	}
	if mouse.Action == input.MouseLeave {
		s.leave(mouse)
		return false
	}
	if handled, captured := s.continueGesture(presented, mouse, held, turn); captured {
		return handled
	}
	if !mouse.Pos.In(presented.visible) {
		s.leave(mouse)
		return false
	}
	return s.route(presented, mouse, turn)
}

func (s *Stack) continueGesture(presented stackPresentation, mouse input.Mouse, owner *stackPointerTarget, turn uint64) (handled, captured bool) {
	if owner == nil || (mouse.Action != input.MouseDrag && mouse.Action != input.MouseUp) {
		return false, false
	}
	held, found := s.placedTarget(presented, *owner)
	if !found {
		// The owner of the gesture is gone. Do not hand its remainder to whatever
		// replaced it and accidentally begin a different interaction halfway through.
		s.leave(mouse)
		return owner.id != 0, true
	}
	handled = s.deliver(held, mouse, turn)
	return handled || held.id != 0, true
}

func (s *Stack) route(presented stackPresentation, mouse input.Mouse, turn uint64) bool {
	top := len(presented.layers) - 1
	if top < 0 {
		return s.deliverBase(presented, mouse, turn)
	}
	placed := presented.layers[top]
	if !mouse.Pos.In(placed.area) {
		switch mouse.Action {
		case input.WheelUp, input.WheelDown, input.MouseMove:
			return s.below(presented, top, mouse, turn)
		default:
			return s.outside(mouse, placed, turn)
		}
	}
	s.deliver(placed.pointerTarget(), mouse, turn)
	// A pointer event inside a modal never activates what the modal covers, even when
	// its content had no behavior for this particular event.
	return true
}

func (s *Stack) below(presented stackPresentation, before int, mouse input.Mouse, turn uint64) bool {
	for i := before - 1; i >= 0; i-- {
		placed := presented.layers[i]
		if mouse.Pos.In(placed.area) && s.deliver(placed.pointerTarget(), mouse, turn) {
			return true
		}
	}
	return s.deliverBase(presented, mouse, turn)
}

func (s *Stack) deliver(placed stackPointerTarget, mouse input.Mouse, turn uint64) bool {
	if !s.capture.current(turn) || !s.pointerAttached(placed) {
		return false
	}
	s.hoverTo(placed, mouse)
	if !s.capture.current(turn) || s.hover.id != placed.id || s.hover.baseID != placed.baseID ||
		!s.pointerAttached(placed) {
		return false
	}
	if mouse.Action == input.MouseDown {
		return s.capture.offer(turn, placed, func() bool { return placed.deliverPointer(mouse) })
	}
	return placed.deliverPointer(mouse)
}

func (s *Stack) pointerAttached(target stackPointerTarget) bool {
	if target.id != 0 {
		return s.Contains(target.id)
	}
	return target.baseID != nil && target.baseID == s.presentation.Value().baseID
}

func (s *Stack) deliverBase(presented stackPresentation, mouse input.Mouse, turn uint64) bool {
	if !s.capture.current(turn) {
		return false
	}
	child, ok := presented.base.(Interactive)
	if !ok || !mouse.Pos.In(presented.visible) {
		s.leave(mouse)
		return false
	}
	return s.deliver(stackPointerTarget{child: child, baseID: presented.baseID}, mouse, turn)
}

func (s *Stack) leave(mouse input.Mouse) {
	s.hoverTo(stackPointerTarget{}, mouse)
}

func (s *Stack) hoverTo(to stackPointerTarget, mouse input.Mouse) {
	previous := s.hover
	s.hover = to
	if previous.id == to.id && previous.baseID == to.baseID {
		return
	}
	if previous.child != nil {
		mouse.Action = input.MouseLeave
		mouse.Pos = mouse.Pos.Sub(previous.area.Min)
		previous.child.Handle(mouse)
	}
}

// Do runs one of the stack's actions by name, reporting whether it was one a stack
// knows. See [Doer].
func (s *Stack) Do(action keymap.Action) bool {
	if action != Close {
		return false
	}
	if top := s.Top(); top != nil && !sticky(top) {
		s.Pop()
	}
	return true
}

func (s *Stack) outside(mouse input.Mouse, top layerPlacement, turn uint64) bool {
	s.leave(mouse)
	if !s.capture.current(turn) {
		return true
	}
	if mouse.Action == input.MouseDown {
		if !s.KeepOnClickOutside && !sticky(top.modal) {
			s.dismiss(top.id)
		}
		// The press either dismissed the modal or was explicitly kept from doing so.
		// In neither case may it activate something behind the modal as well.
		return true
	}
	return mouse.Action == input.MouseUp || mouse.Action == input.MouseDrag
}

// dismiss removes layer only while it is still the semantic top. An event routed
// against an older presented frame must never close a newer layer it did not show.
func (s *Stack) dismiss(id LayerID) {
	top, ok := s.top()
	if !ok || id == 0 || top.id != id {
		return
	}
	s.Pop()
}

func (s *Stack) remove(at int) bool {
	if at < 0 || at >= len(s.layers) {
		return false
	}
	layer := s.layers[at]
	if owner, ok := layer.modal.(CloseRequester); ok && !owner.RequestClose() {
		return false
	}
	// A synchronous owner notification may already remove or reorder this layer.
	at = slices.IndexFunc(s.layers, func(current stackLayer) bool { return current.id == layer.id })
	if at < 0 {
		return true
	}
	copy(s.layers[at:], s.layers[at+1:])
	s.layers[len(s.layers)-1] = stackLayer{}
	s.layers = s.layers[:len(s.layers)-1]
	s.layers = trim(s.layers)
	if closer, ok := layer.modal.(Closer); ok {
		closer.Closed()
	}
	s.settle()
	return true
}

// Draw paints the interface and then the layers from the bottom up, each into the
// space it asked for, and records where they went.
func (s *Stack) Draw(v Frame) {
	base := s.base
	layers := slices.Clone(s.layers)
	space := v.Bounds().Size()
	presented := stackPresentation{base: base, baseID: s.baseID, visible: v.Visible(), layers: make([]layerPlacement, len(layers))}
	for i, layer := range layers {
		presented.layers[i] = layerPlacement{
			id: layer.id, modal: layer.modal, area: layer.modal.Place(space).In(space),
		}
	}
	s.presentation.Stage(v, presented)

	if base != nil {
		base.Draw(v)
	}
	for _, placed := range presented.layers {
		if backdrop, ok := placed.modal.(Backdrop); ok {
			backdrop.Backdrop(v.View)
		}
		placed.modal.Draw(v.Sub(placed.area))
	}
}

type stackPresentation struct {
	base    Widget
	baseID  *byte
	visible image.Rectangle
	layers  []layerPlacement
}

type stackPointerTarget struct {
	id     LayerID
	baseID *byte
	child  Interactive
	area   image.Rectangle
}

func (p stackPointerTarget) deliverPointer(event input.Mouse) bool {
	event.Pos = event.Pos.Sub(p.area.Min)
	return p.child.Handle(event)
}

func (p layerPlacement) pointerTarget() stackPointerTarget {
	return stackPointerTarget{id: p.id, child: p.modal, area: p.area}
}

func (s *Stack) placedTarget(presented stackPresentation, owner stackPointerTarget) (stackPointerTarget, bool) {
	if owner.id == 0 {
		child, ok := presented.base.(Interactive)
		return stackPointerTarget{baseID: presented.baseID, child: child}, ok && owner.baseID == presented.baseID && !presented.visible.Empty()
	}
	placed, found := presented.placed(owner.id)
	return placed.pointerTarget(), found && s.Contains(owner.id)
}

func (p stackPresentation) placed(id LayerID) (layerPlacement, bool) {
	for _, placed := range p.layers {
		if placed.id == id {
			return placed, true
		}
	}
	return layerPlacement{}, false
}

type layerPlacement struct {
	id    LayerID
	modal Modal
	area  image.Rectangle
}

// Focus takes the keyboard for the whole stack, or gives it up, and passes the news
// to whichever of the layers or the interface underneath currently holds it. A stack
// is a widget, so one can sit inside a [Container] like anything else.
func (s *Stack) Focus(has bool) {
	s.change(has, s.settle, &s.holder, &s.matcher)
}

func (s *Stack) settle() {
	want := Widget(nil)
	wantID := LayerID(0)
	wantBase := false
	if top, ok := s.top(); ok {
		want, wantID = top.modal, top.id
	} else if s.base != nil {
		want, wantBase = s.base, true
	}
	if s.state.Settled() && wantID == s.holderID && wantBase == s.holderBase {
		return
	}
	from := s.holder
	s.holder, s.holderID, s.holderBase = want, wantID, wantBase
	revision := s.begin()
	s.matcher.Clear()
	if s.superseded(revision) {
		return
	}
	// A notification can synchronously change membership and settle a new owner.
	// The superseded transfer must not issue any more focus notifications.
	kept := identity.Same(from, want)
	if from != nil && !kept {
		tell(from, false)
		if s.superseded(revision) {
			return
		}
	}
	if !s.blurCovered(revision) {
		return
	}
	if !kept {
		tell(want, s.state.Focused())
	}
}

func (s *Stack) blurCovered(revision uint64) bool {
	// A layer that is no longer on top is covered by one that is, which is the same
	// thing as not having the keyboard.
	for _, layer := range s.layers {
		if layer.id != s.holderID && !identity.Same(layer.modal, s.holder) {
			tell(layer.modal, false)
			if s.superseded(revision) {
				return false
			}
		}
	}
	if s.base != nil && !s.holderBase && !identity.Same(s.base, s.holder) {
		tell(s.base, false)
	}
	return !s.superseded(revision)
}

func (s *Stack) keys() *keymap.Map {
	if s.Keys != nil {
		return s.Keys
	}
	return stackKeys()
}

func sticky(m Modal) bool {
	insistent, ok := m.(Insistent)
	return ok && insistent.Insists()
}
