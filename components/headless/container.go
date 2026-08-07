package headless

import (
	"image"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Focusable is a widget that can hold the keyboard.
//
// # Why a widget has to be told
//
// A keystroke has one destination and a frame has one cursor, so with more than one
// field on screen something has to decide which of them the typing is for. Deciding
// it by letting an event fall through until somebody claims it works while the
// widgets are in a line and nothing else: two editors both claim every key, and both
// place the terminal's cursor, and the one that draws last wins.
//
// So the answer is pushed rather than pulled. A [Container] tells the widget that has
// the keyboard, and tells the ones that do not, and a widget draws itself
// accordingly — a cursor, a lit border, a highlighted row.
//
// # Why the zero value has the keyboard
//
// A widget that has never been told anything assumes it has the keyboard. That is
// what makes a single field work when it is the whole interface, with no container
// above it to say so — which is how most interfaces start, and how every one of this
// library's examples began. A container tells every child where it stands as soon as
// it has one, so nothing is ever left guessing once there is a choice to make.
//
// Answering input is not the same as wanting the keyboard, which is why this is its
// own interface. A transcript answers the wheel and a drag; it is not somewhere the
// user types, and it has no business in the ring that tab walks.
type Focusable interface {
	Interactive
	// Focus is told true when this widget takes the keyboard and false when it loses
	// it. It is called when the answer changes rather than every frame.
	//
	// It may be told the same thing twice — every child of a container is told where
	// it stands as soon as there is a container to say so, whether or not it had
	// supposed otherwise. A widget that does something on losing the keyboard, such
	// as validating what was typed, has to check that it had it.
	Focus(has bool)
}

// Item is one child of a [Container]: what goes there, and how much room it gets.
type Item struct {
	// Size is how much of the divided axis this child takes. It means exactly what
	// it means in [layout.Slot], including the zero value, which asks for nothing —
	// deliberately, because [layout.Fixed] of zero is that same zero value, and a
	// container that read it as "however much you want" would put back a row a
	// caller had just asked to have none of.
	//
	// A child as big as its content wants to be is [layout.Measured].
	Size layout.Sizing
	// Of is the child. A child that can answer how big it wants to be — anything
	// implementing [Sized] — is asked when Size says the slot is measured.
	Of Widget
}

// Container arranges widgets in a region and decides which of them an event is for.
//
// It is the piece that was missing while every interface here was a single widget
// with everything hand-wired underneath it. A caller that had two things on screen
// laid them out itself, forwarded events itself, and worked out for itself which of
// them a click had landed on — and the answer to the last one is only knowable while
// a frame is being drawn, so it had to be remembered by hand as well.
//
// # The two routings
//
// A key goes to the widget that has the keyboard. A mouse event goes to the widget it
// is over. They are different questions with different answers, and treating them as
// one is what makes an interface where clicking a pane does not let you type in it,
// or where the wheel scrolls whatever was last typed into.
//
// A press is captured: everything until the release goes to whichever child took it,
// wherever the pointer wanders. Without that a selection stops extending the moment
// the drag leaves the pane it started in, which is not what any interface does.
//
// # What it does not do
//
// It does not draw. There is no border, no gap, no highlight for the focused child:
// those are appearance, they belong a layer up, and a container that had an opinion
// about them would be one nobody could dress differently.
//
// The zero Container is an empty column, ready to have items appended.
type Container struct {
	// Axis is which way the children are arranged. The zero value stacks them down
	// the region.
	Axis layout.Axis
	// Items are the children, in the order they are arranged and in the order the
	// keyboard walks them. It may be rebuilt between frames: focus is held by
	// identity, so a child that is still there keeps it and one that is gone gives it
	// up to the first child that will take it.
	//
	// Held by identity means held as a pointer. A child value containing a function
	// cannot be compared at all, and comparing one is a panic rather than a false.
	Items []Item
	// Gap is how many blank rows or columns go between one child and the next. Zero
	// puts them against each other.
	//
	// It is the layout's — see [layout.Flow] — rather than a blank child inserted
	// between every pair, which is what spacing used to be and which put things in
	// the ring that are not children: a hole nothing can focus, nothing can be
	// clicked in, and every index has to be corrected for.
	Gap int
	// Keys say which keystrokes move the keyboard along the ring. Nil reads through
	// [DefaultContainerKeys], which is tab and shift+tab.
	//
	// They are tried only after the focused child has declined the event, so a widget
	// that means something by tab — a completion, a field with columns in it — keeps
	// it.
	Keys *keymap.Map

	// focused is the child with the keyboard, held by identity rather than by index
	// so that rebuilding the items does not silently move it.
	focused Widget
	// settled says whether the children have been told where they stand. Until they
	// have, every one of them believes it has the keyboard — see [Focusable].
	settled bool
	// blurred says this container has been told it does not have the keyboard, so
	// the child it would focus must not be told that it does.
	blurred bool

	// presentation is the child identities and areas from the last complete root
	// frame. Identity belongs in the snapshot with geometry: if Items is reordered
	// before the next draw, input still goes to what the user can actually see.
	presentation Snapshot[[]childPlacement]
	// held is the child a press was given to, kept by identity for the same reason
	// the focus is. Everything up to the release goes to it.
	held Widget
	// slots is rebuilt every frame from the items and kept to save the allocation.
	slots []layout.Slot
	// pending is how far into a multi-chord binding the keys typed so far have got.
	pending keymap.Pending
}

// Rows is a container that stacks its children down the region.
func Rows(items ...Item) *Container {
	c := &Container{Axis: layout.Down}
	c.Set(items...)
	return c
}

// Columns is a container that puts its children side by side.
func Columns(items ...Item) *Container {
	c := &Container{Axis: layout.Across}
	c.Set(items...)
	return c
}

// Set replaces the children, preserving focus by identity where possible.
func (c *Container) Set(items ...Item) {
	c.Items = append(c.Items[:0], items...)
	c.settle()
}

// Add appends a child and returns the container, so a tree can be built in one
// expression.
func (c *Container) Add(items ...Item) *Container {
	c.Items = append(c.Items, items...)
	c.settle()
	return c
}

// Focused is the child with the keyboard, or nil when no child will take it.
func (c *Container) Focused() Widget {
	c.settle()
	return c.focused
}

// Give hands the keyboard to a child, reporting whether it took it. A child that is
// not in this container, or that does not want the keyboard, does not get it.
func (c *Container) Give(w Widget) bool {
	if _, ok := w.(Focusable); !ok {
		return false
	}
	for _, item := range c.Items {
		if item.Of == w {
			c.settle()
			c.move(w)
			return true
		}
	}
	return false
}

// FocusNext moves the keyboard to the next child that will take it, wrapping round.
// It reports whether anything moved, which is false when no child takes the keyboard
// or only one does.
func (c *Container) FocusNext() bool { return c.step(1) }

// FocusPrev moves the keyboard to the previous child that will take it.
func (c *Container) FocusPrev() bool { return c.step(-1) }

// Focus takes the keyboard for this container, or gives it up, and passes the news
// to the child that holds it. A container is a widget like any other, so a container
// inside a container is how an interface gets more than one row of panes.
func (c *Container) Focus(has bool) {
	c.blurred = !has
	c.settle()
	tell(c.focused, has)
}

// Draw arranges the children and draws each into the room it got.
func (c *Container) Draw(v Frame) {
	items := append([]Item(nil), c.Items...)
	rects := c.flow().Rects(v.Bounds().Size(), c.arrangeItems(items))
	placed := make([]childPlacement, len(items))
	for i, item := range items {
		placed[i] = childPlacement{child: item.Of, area: rects[i]}
	}
	c.presentation.Stage(v, placed)
	for _, child := range placed {
		if child.child != nil {
			child.child.Draw(v.Sub(child.area))
		}
	}
}

// Measure is how much of the divided axis the children want altogether, which is
// what a container inside a measured slot answers with.
func (c *Container) Measure(across int) int {
	return c.flow().Wanted(across, c.arrange())
}

// flow is how this container divides its region: its axis, and the room it leaves
// between one child and the next.
func (c *Container) flow() layout.Flow {
	return layout.Flow{Axis: c.Axis, Gap: c.Gap}
}

// Handle gives the event to whichever child it is for.
//
// A key goes to the child with the keyboard, and the ring is walked only if that
// child did not want it. Anything nobody wanted is declined, so a container inside a
// container passes what it cannot use back up rather than swallowing it.
func (c *Container) Handle(ev input.Event) bool {
	c.settle()
	if mouse, ok := ev.(input.Mouse); ok {
		return c.mouse(mouse)
	}
	if handler, ok := c.focused.(Interactive); ok && handler.Handle(ev) {
		return true
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	action, mine := c.keys().Lookup(key, &c.pending)
	switch {
	case !mine:
		return false
	case action == "":
		return true // the start of a binding more than one chord long
	}
	return c.Do(action)
}

// Do runs one of the container's actions by name, reporting whether it was one a
// container knows and whether it changed anything. See [Doer].
func (c *Container) Do(action keymap.Action) bool {
	switch action {
	case FocusNext:
		return c.FocusNext()
	case FocusPrev:
		return c.FocusPrev()
	}
	return false
}

// mouse routes a pointer event by where it is, and by who took the press.
func (c *Container) mouse(ev input.Mouse) bool {
	if c.held != nil {
		switch ev.Action {
		case input.MouseDrag, input.MouseUp:
			held, found := c.placed(c.held)
			if ev.Action == input.MouseUp {
				c.held = nil
			}
			if !found {
				// The child that took the press is no longer here. The gesture has
				// nowhere to go, which is not the same as it belonging to whatever
				// took its place.
				return false
			}
			return c.deliver(held, ev)
		default:
		}
	}
	at, found := c.at(ev.Pos)
	if !found {
		return false
	}
	if ev.Action != input.MouseDown {
		return c.deliver(at, ev)
	}
	// A press moves the keyboard whether or not the child does anything with the
	// press itself: clicking a pane is how a user says they mean that one.
	c.Give(at.child)
	if !c.deliver(at, ev) {
		return false
	}
	c.held = at.child
	return true
}

// deliver hands an event to a child, in the child's own coordinates.
//
// The translation is the container's job for the same reason the clipping is: a
// widget is handed a view whose origin is its own and reasons in it, so a position
// that arrived in anybody else's coordinates would be a position it cannot use.
func (c *Container) deliver(to childPlacement, ev input.Mouse) bool {
	handler, ok := to.child.(Interactive)
	if !ok {
		return false
	}
	local := ev
	local.Pos = ev.Pos.Sub(to.area.Min)
	return handler.Handle(local)
}

// at is the child a point is over, or -1.
func (c *Container) at(p image.Point) (childPlacement, bool) {
	for _, child := range c.presentation.Value() {
		if p.In(child.area) {
			return child, true
		}
	}
	return childPlacement{}, false
}

func (c *Container) placed(w Widget) (childPlacement, bool) {
	for _, child := range c.presentation.Value() {
		if child.child == w {
			return child, true
		}
	}
	return childPlacement{}, false
}

// arrange rebuilds the slots from the items, asking each child that can measure
// itself to do so.
func (c *Container) arrange() []layout.Slot {
	return c.arrangeItems(c.Items)
}

func (c *Container) arrangeItems(items []Item) []layout.Slot {
	c.slots = c.slots[:0]
	for _, item := range items {
		slot := layout.Slot{Size: item.Size}
		if measurer, ok := item.Of.(layout.Measurer); ok {
			slot.Of = measurer
		}
		c.slots = append(c.slots, slot)
	}
	return c.slots
}

type childPlacement struct {
	child Widget
	area  image.Rectangle
}

// settle makes sure the keyboard is somewhere it can be, and that every child has
// been told where it stands.
//
// It runs before anything else this container does, because the items can be rebuilt
// between frames and the child that had the keyboard may no longer be there.
func (c *Container) settle() {
	want := c.focused
	if !c.has(want) {
		want = c.first()
	}
	c.move(want)
	c.settled = true
}

// move puts the keyboard on a child and takes it off everything else.
//
// Every other child is told, not just the one that had it: until a container says
// otherwise each of them believes it has the keyboard, which is what makes a single
// widget work with no container above it.
func (c *Container) move(to Widget) {
	if c.settled && to == c.focused {
		return
	}
	from := c.focused
	c.focused = to
	// The one that had it is told first and by name, because it may be the reason
	// the keyboard moved at all: a child taken out of the items is no longer in the
	// loop below, and would otherwise go on believing it has the keyboard.
	if from != nil && from != to {
		tell(from, false)
	}
	for _, item := range c.Items {
		if item.Of != to && item.Of != from {
			tell(item.Of, false)
		}
	}
	tell(to, !c.blurred)
}

// step moves the keyboard along the ring by one, in the given direction.
func (c *Container) step(by int) bool {
	c.settle()
	n := len(c.Items)
	if n == 0 {
		return false
	}
	from := c.indexOf(c.focused)
	for offset := 1; offset <= n; offset++ {
		i := ((from+by*offset)%n + n) % n
		if w, ok := c.Items[i].Of.(Focusable); ok {
			if Widget(w) == c.focused {
				return false
			}
			c.move(w)
			return true
		}
	}
	return false
}

// first is the earliest child that will take the keyboard, or nil.
func (c *Container) first() Widget {
	for _, item := range c.Items {
		if w, ok := item.Of.(Focusable); ok {
			return w
		}
	}
	return nil
}

// has reports whether a widget is one of this container's children.
func (c *Container) has(w Widget) bool { return w != nil && c.indexOf(w) >= 0 }

// indexOf is where a widget sits among the children, or -1.
func (c *Container) indexOf(w Widget) int {
	if w == nil {
		return -1
	}
	for i, item := range c.Items {
		if item.Of == w {
			return i
		}
	}
	return -1
}

// keys is the map to read through, standing in the default for a caller who set none.
func (c *Container) keys() *keymap.Map {
	if c.Keys != nil {
		return c.Keys
	}
	return containerKeys()
}

// tell says whether a widget has the keyboard, if it is the kind that wants to know.
func tell(w Widget, has bool) {
	if focusable, ok := w.(Focusable); ok {
		focusable.Focus(has)
	}
}
