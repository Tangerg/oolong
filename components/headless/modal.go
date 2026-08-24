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
type Closer interface {
	Modal
	Closed()
}

// Backdrop is a modal that wants to touch the space it is covering before it is
// drawn into its own corner of it — dimming what is behind, usually.
//
// It exists because a layer is handed a view of the area it asked for and nothing
// else, which is what stops it drawing outside its own box. That is the right
// default and it makes dimming impossible, so a layer that means to reach further
// has to say so, and gets the whole space exactly once.
//
// Nothing in this package decides what a backdrop looks like. It calls back.
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

// Stack is an interface with layers floating over it, and the answer to which of
// them the keyboard belongs to.
//
// The top layer owns keyboard input and pointer input inside its area; with nothing on
// it, the interface underneath has its input back. Wheel and move reports outside a
// layer follow the visible stack downward, while a press outside is consumed as the
// dismissal gesture. A layer that accepts a press captures its drag and release. That
// is the whole focus and pointer model between layers. Within a layer, or within the
// interface underneath, a [Container] is what decides.
//
// The zero Stack is empty and ready. A Stack must not be copied after first use: its
// layer identities, focus, pointer capture and committed geometry are one mutable
// owner.
type Stack struct {
	noCopy noCopy

	// base is private because replacing the focused root has to release the old one.
	// SetBase is the ownership transition; an exported field could bypass it.
	base Widget

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

	// holder is whatever was last told it has the keyboard, and settled says
	// anything has been told at all. Until then every one of them believes it does —
	// see [Focusable].
	holder     Widget
	holderID   LayerID
	holderBase bool
	focusState
	// matcher owns how far into a multi-chord binding the keys have got.
	matcher keymap.Matcher
	// held is the layer that accepted a pointer press. Drag and release stay with it
	// even after the pointer leaves its rectangle, matching capture inside containers.
	held LayerID
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
	if s.settled && s.holderID != 0 {
		// A layer owns the keyboard. The new base starts in the state every lone
		// widget assumes — focused — so it must hear the one real transition to its
		// covered state; the layer above it has not changed owners at all.
		tell(s.base, false)
		return
	}
	s.settled = false
	s.settle()
}

// Push puts a layer on top, gives it the keyboard, and returns its stable handle.
func (s *Stack) Push(m Modal) LayerID {
	if m == nil {
		return 0
	}
	id, ok := s.layerIDs.next()
	if !ok {
		panic("headless: stack exhausted layer identities")
	}
	layerID := LayerID(id)
	s.layers = append(s.layers, stackLayer{id: layerID, modal: m})
	s.settle()
	return layerID
}

// Pop removes the top layer and reports whether there was one. The keyboard goes
// back to whatever was underneath.
func (s *Stack) Pop() bool {
	n := len(s.layers)
	if n == 0 {
		return false
	}
	return s.remove(n - 1)
}

// Remove takes the insertion named by id out of the stack and reports whether it was
// present. A controller uses this when its layer closes while another is above it;
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
// would have been dismissed.
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
	if len(presented.layers) == 0 {
		if handler, ok := presented.base.(Interactive); ok {
			return handler.Handle(ev)
		}
		return false
	}
	top := presented.layers[len(presented.layers)-1]

	if mouse, ok := ev.(input.Mouse); ok {
		return s.mouse(presented, mouse)
	}

	if top.modal.Handle(ev) {
		return true
	}
	if key, ok := ev.(input.Key); ok {
		s.matcher.Handle(s.keys(), key, func(action keymap.Action) bool {
			if action != Close {
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

// mouse routes pointer input through modal capture and the visible layer geometry.
func (s *Stack) mouse(presented stackPresentation, mouse input.Mouse) bool {
	if mouse.Action == input.MouseDown {
		// The input protocol carries one pointer gesture. A new press supersedes an
		// incomplete old one, including when its new target declines the press.
		s.held = 0
	}
	if s.held != 0 && (mouse.Action == input.MouseDrag || mouse.Action == input.MouseUp) {
		held, found := presented.placed(s.held)
		if mouse.Action == input.MouseUp {
			s.held = 0
		}
		if !found {
			// The owner of the gesture is gone. Do not hand its remainder to whatever
			// replaced it and accidentally begin a different interaction halfway through.
			return true
		}
		s.deliver(held, mouse)
		return true
	}

	top := len(presented.layers) - 1
	if top < 0 {
		return s.deliverBase(presented.base, mouse)
	}
	placed := presented.layers[top]
	if !mouse.Pos.In(placed.area) {
		switch mouse.Action {
		case input.WheelUp, input.WheelDown, input.MouseMove:
			return s.below(presented, top, mouse)
		default:
			return s.outside(mouse, placed)
		}
	}

	handled := s.deliver(placed, mouse)
	if mouse.Action == input.MouseDown && handled {
		s.held = placed.id
	}
	// A pointer event inside a modal never activates what the modal covers, even when
	// its content had no behavior for this particular event.
	return true
}

func (s *Stack) below(presented stackPresentation, before int, mouse input.Mouse) bool {
	for i := before - 1; i >= 0; i-- {
		placed := presented.layers[i]
		if mouse.Pos.In(placed.area) && s.deliver(placed, mouse) {
			return true
		}
	}
	return s.deliverBase(presented.base, mouse)
}

func (s *Stack) deliver(placed layerPlacement, mouse input.Mouse) bool {
	local := mouse
	local.Pos = mouse.Pos.Sub(placed.area.Min)
	return placed.modal.Handle(local)
}

func (s *Stack) deliverBase(base Widget, mouse input.Mouse) bool {
	handler, ok := base.(Interactive)
	return ok && handler.Handle(mouse)
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

// outside deals with a mouse event beyond the top layer.
func (s *Stack) outside(mouse input.Mouse, top layerPlacement) bool {
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
	if s.held == layer.id {
		s.held = 0
	}
	copy(s.layers[at:], s.layers[at+1:])
	s.layers[len(s.layers)-1] = stackLayer{}
	s.layers = s.layers[:len(s.layers)-1]
	s.layers = trim(s.layers)
	s.settle()
	if closer, ok := layer.modal.(Closer); ok {
		closer.Closed()
	}
	return true
}

// Draw paints the interface and then the layers from the bottom up, each into the
// space it asked for, and records where they went.
func (s *Stack) Draw(v Frame) {
	base := s.base
	layers := slices.Clone(s.layers)
	space := v.Bounds().Size()
	presented := stackPresentation{base: base, layers: make([]layerPlacement, len(layers))}
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
	base   Widget
	layers []layerPlacement
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
	if !has {
		s.matcher.Clear()
	}
	s.change(has, s.settle, &s.holder)
}

// settle makes sure the keyboard is where the layers say it should be, and that
// whatever had it before has been told it no longer does.
func (s *Stack) settle() {
	want := Widget(nil)
	wantID := LayerID(0)
	wantBase := false
	if top, ok := s.top(); ok {
		want, wantID = top.modal, top.id
	} else if s.base != nil {
		want, wantBase = s.base, true
	}
	if s.settled && wantID == s.holderID && wantBase == s.holderBase {
		return
	}
	from := s.holder
	s.holder, s.holderID, s.holderBase, s.settled = want, wantID, wantBase, true
	if from != nil {
		tell(from, false)
	}
	// A layer that is no longer on top is covered by one that is, which is the same
	// thing as not having the keyboard.
	for _, layer := range s.layers {
		if layer.id != wantID {
			tell(layer.modal, false)
		}
	}
	if s.base != nil && !wantBase {
		tell(s.base, false)
	}
	tell(want, !s.blurred)
}

// keys is the map to read through, standing in the default for a caller who set none.
func (s *Stack) keys() *keymap.Map {
	if s.Keys != nil {
		return s.Keys
	}
	return stackKeys()
}

// sticky reports whether a layer currently refuses to be dismissed.
func sticky(m Modal) bool {
	insistent, ok := m.(Insistent)
	return ok && insistent.Insists()
}
