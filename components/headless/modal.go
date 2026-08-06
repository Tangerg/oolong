package headless

import (
	"image"

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

// Stack is an interface with layers floating over it, and the answer to which of
// them the keyboard belongs to.
//
// Only the top layer sees input; with nothing on it, the interface underneath has
// its input back. That is the whole of the focus model between layers, and it is
// enough for what a streaming interface actually does — a dialog over a composer, a
// palette over both. Within a layer, or within the interface underneath, a
// [Container] is what decides.
//
// The zero Stack is empty and ready.
type Stack struct {
	// Base is the interface the layers float over. It is drawn first and has the
	// input whenever no layer does.
	//
	// It is here rather than left to the caller because owning it is what lets the
	// stack say who has the keyboard. A caller that drew the interface itself and
	// then drew the stack over it would leave a field underneath still believing it
	// was being typed into — still drawing a cursor, into the frame's one cursor,
	// under a dialog that has one of its own.
	Base Widget

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

	layers []Modal
	// areas is where each layer was last drawn, in the coordinates of the view the
	// stack was drawn into. A hit test happens between frames, so it has to ask
	// about the frame that is on screen rather than the one being built.
	areas []image.Rectangle

	// holder is whatever was last told it has the keyboard, and settled says
	// anything has been told at all. Until then every one of them believes it does —
	// see [Focusable].
	holder  Widget
	settled bool
	// blurred says the stack itself has been told it does not have the keyboard,
	// which is what a stack inside something larger is told.
	blurred bool
	// pending is how far into a multi-chord binding the keys typed so far have got.
	pending keymap.Pending
}

// Push puts a layer on top, and gives it the keyboard.
func (s *Stack) Push(m Modal) {
	if m == nil {
		return
	}
	s.layers = append(s.layers, m)
	s.areas = append(s.areas, image.Rectangle{})
	s.settle()
}

// Pop removes the top layer and reports whether there was one. The keyboard goes
// back to whatever was underneath.
func (s *Stack) Pop() bool {
	n := len(s.layers)
	if n == 0 {
		return false
	}
	top := s.layers[n-1]
	s.layers = s.layers[:n-1]
	s.areas = s.areas[:n-1]
	s.settle()
	if closer, ok := top.(Closer); ok {
		closer.Closed()
	}
	return true
}

// Clear pops every layer, from the top down, so each is told in the order it
// would have been dismissed.
func (s *Stack) Clear() {
	for s.Pop() {
	}
}

// Top is the layer with the input, or nil when there is none.
func (s *Stack) Top() Modal {
	if len(s.layers) == 0 {
		return nil
	}
	return s.layers[len(s.layers)-1]
}

// Depth is how many layers there are.
func (s *Stack) Depth() int { return len(s.layers) }

// Empty reports whether the interface underneath has its input back.
func (s *Stack) Empty() bool { return len(s.layers) == 0 }

// Area is where the top layer was last drawn, and whether there is one.
func (s *Stack) Area() (image.Rectangle, bool) {
	if len(s.areas) == 0 {
		return image.Rectangle{}, false
	}
	return s.areas[len(s.areas)-1], true
}

// Handle gives the event to the top layer, and reports whether the stack dealt
// with it.
//
// An empty stack consumes nothing, so an interface can offer it every event and
// carry on when it is not interested. A stack with anything in it consumes every
// key, because a key reaching what a modal is covering is a keystroke going
// somewhere the user cannot see.
func (s *Stack) Handle(ev input.Event) bool {
	s.settle()
	top := s.Top()
	if top == nil {
		if handler, ok := s.Base.(Interactive); ok {
			return handler.Handle(ev)
		}
		return false
	}
	area, _ := s.Area()

	if mouse, ok := ev.(input.Mouse); ok {
		if !mouse.Pos.In(area) {
			return s.outside(mouse)
		}
		// The layer is drawn into a view whose origin is its own, so it reasons in
		// its own coordinates and the position has to arrive in them too.
		local := mouse
		local.Pos = mouse.Pos.Sub(area.Min)
		top.Handle(local)
		return true
	}

	if top.Handle(ev) {
		return true
	}
	if key, ok := ev.(input.Key); ok {
		if action, mine := s.keys().Lookup(key, &s.pending); mine && action != "" {
			s.Do(action)
		}
	}
	// Consumed either way: what is underneath is covered, and a key that fell
	// through to it would act somewhere the user is not looking.
	return true
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
func (s *Stack) outside(mouse input.Mouse) bool {
	if mouse.Action == input.MouseDown && !s.KeepOnClickOutside && !sticky(s.Top()) {
		s.Pop()
		return true
	}
	// A wheel or a move outside the layer belongs to whatever is under it: a modal
	// does not stop the transcript behind it from scrolling.
	return mouse.Action == input.MouseUp
}

// Draw paints the interface and then the layers from the bottom up, each into the
// space it asked for, and records where they went.
func (s *Stack) Draw(v grid.View) {
	s.settle()
	if s.Base != nil {
		s.Base.Draw(v)
	}
	space := v.Bounds().Size()
	for i, m := range s.layers {
		if backdrop, ok := m.(Backdrop); ok {
			backdrop.Backdrop(v)
		}
		area := m.Place(space).In(space)
		s.areas[i] = area
		m.Draw(v.Sub(area))
	}
}

// Focus takes the keyboard for the whole stack, or gives it up, and passes the news
// to whichever of the layers or the interface underneath currently holds it. A stack
// is a widget, so one can sit inside a [Container] like anything else.
func (s *Stack) Focus(has bool) {
	s.blurred = !has
	s.settled = false
	s.settle()
}

// settle makes sure the keyboard is where the layers say it should be, and that
// whatever had it before has been told it no longer does.
func (s *Stack) settle() {
	want := Widget(nil)
	if top := s.Top(); top != nil {
		want = top
	} else if s.Base != nil {
		want = s.Base
	}
	if s.settled && want == s.holder {
		return
	}
	from := s.holder
	s.holder, s.settled = want, true
	if from != nil && from != want {
		tell(from, false)
	}
	// A layer that is no longer on top is covered by one that is, which is the same
	// thing as not having the keyboard.
	for _, m := range s.layers {
		if m != want && m != from {
			tell(m, false)
		}
	}
	if s.Base != nil && s.Base != want && s.Base != from {
		tell(s.Base, false)
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
