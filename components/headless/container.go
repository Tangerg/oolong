package headless

import (
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/internal/identity"
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
	// Key is the child's stable identity across [Container.Set]. Empty uses its
	// position. Name a child when it may move: focus and an in-progress pointer
	// gesture then follow the part rather than whichever part took its old slot.
	// Non-empty keys must be unique within one container.
	Key string
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
// The zero Container is an empty column, ready to have items appended. A Container
// must not be copied after first use: children, focus, pointer capture and committed
// routing geometry are one mutable owner.
type Container struct {
	noCopy noCopy

	// Axis is which way the children are arranged. The zero value stacks them down
	// the region.
	Axis layout.Axis
	// items are the children, in arrangement and keyboard order. They are private so
	// replacing them cannot bypass focus settlement or leave pointer capture owned by
	// a child that is no longer present.
	//
	items []Item
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

	// focused is the semantic item index. A named item follows its key through Set;
	// an unnamed item follows its position, the same explicit identity rule used by
	// retained component trees elsewhere.
	focused int
	// holder is the concrete child last told it had the keyboard. Keeping it beside
	// the index lets Set release a removed child without comparing interface values.
	holder Widget
	focusState

	// presentation is the child identities and areas from the last complete root
	// frame. Identity belongs in the snapshot with geometry: if Items is reordered
	// before the next draw, input still goes to what the user can actually see.
	presentation Snapshot[[]childPlacement]
	// held is the exact presented child a press was given to. Everything up to the
	// release goes back to it; a later frame may update its geometry when its identity
	// is still demonstrably the same.
	held    childPlacement
	holding bool
	// slots is rebuilt every frame from the items and kept to save the allocation.
	slots []layout.Slot
	// matcher owns how far into a multi-chord binding the keys have got.
	matcher keymap.Matcher
}

// NewContainer constructs a container that arranges its children along axis.
// [layout.Down] stacks rows and [layout.Across] places columns side by side.
func NewContainer(axis layout.Axis, items ...Item) *Container {
	c := &Container{Axis: axis}
	c.Set(items...)
	return c
}

// Set replaces the children. Focus follows a non-empty [Item.Key], or the old position
// for an unnamed item.
func (c *Container) Set(items ...Item) {
	checkItemKeys(items)
	key := c.focusKey()
	at := c.focused
	c.items = own(c.items, items)
	for i := range c.items {
		c.items[i].Key = strings.Clone(c.items[i].Key)
	}
	if key != "" {
		at = c.indexOfKey(key)
	} else if len(c.items) > 0 {
		at = min(max(at, 0), len(c.items)-1)
	}
	c.focused = at
	c.settled = false
	c.settle()
}

// Add appends a child and returns the container, so a tree can be built in one
// expression.
func (c *Container) Add(items ...Item) *Container {
	checkItemKeys(append(slices.Clone(c.items), items...))
	c.items = append(c.items, items...)
	for i := len(c.items) - len(items); i < len(c.items); i++ {
		c.items[i].Key = strings.Clone(c.items[i].Key)
	}
	c.settled = false
	c.settle()
	return c
}

// Items returns a copy of the children in arrangement and keyboard order.
func (c *Container) Items() []Item {
	if c == nil {
		return nil
	}
	return slices.Clone(c.items)
}

// Len reports how many children the container owns.
func (c *Container) Len() int {
	if c == nil {
		return 0
	}
	return len(c.items)
}

// Focused is the child with the keyboard, or nil when no child will take it.
func (c *Container) Focused() Widget {
	c.settle()
	return c.widgetAt(c.focused)
}

// Give hands the keyboard to the child at index, reporting whether it took it. An
// index that does not exist, or a child that does not want the keyboard, is declined.
// Addressing the item rather than comparing Widget interface values keeps this API
// valid for every implementation the interface permits.
func (c *Container) Give(index int) bool {
	if !c.focusable(index) {
		return false
	}
	c.settle()
	c.move(index)
	return true
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
	if !has {
		c.matcher.Clear()
	}
	c.change(has, c.settle, &c.holder)
}

// Draw arranges the children and draws each into the room it got.
func (c *Container) Draw(v Frame) {
	c.drawWith(v, c.flow(), nil)
}

// drawWith projects the same settled children through flow and draw. It is private
// because the semantic child collection still has one owner; Form uses the seam to
// pass a frame-local Look without rewriting any child configuration during Draw.
func (c *Container) drawWith(v Frame, flow layout.Flow, draw func(Frame, Widget)) {
	items := slices.Clone(c.items)
	rects := flow.Rects(v.Bounds().Size(), c.arrangeItems(items))
	placed := make([]childPlacement, len(items))
	for i, item := range items {
		placed[i] = childPlacement{
			index: i, key: item.Key, child: item.Of, area: rects[i], frame: v.stamp(),
		}
	}
	c.presentation.Stage(v, placed)
	for _, child := range placed {
		if child.child == nil {
			continue
		}
		frame := v.Sub(child.area)
		if draw == nil {
			child.child.Draw(frame)
		} else {
			draw(frame, child.child)
		}
	}
}

// Measure is how much of the divided axis the children want altogether, which is
// what a container inside a measured slot answers with.
func (c *Container) Measure(across int) int {
	return c.measureWith(across, c.flow())
}

func (c *Container) measureWith(across int, flow layout.Flow) int {
	return flow.Wanted(across, c.arrange())
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
	if handler, ok := c.holder.(Interactive); ok && handler.Handle(ev) {
		return true
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := c.matcher.Handle(c.keys(), key, c.Do)
	return handled
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
	if ev.Action == input.MouseDown {
		// A new press begins a new gesture even if the terminal never reported the
		// previous release. Clear before routing so a declined press cannot leave the
		// old owner installed.
		c.held = childPlacement{}
		c.holding = false
	}
	if c.holding {
		switch ev.Action {
		case input.MouseDrag, input.MouseUp:
			owner := c.held
			current, found := c.placed(owner)
			if ev.Action == input.MouseUp {
				c.held = childPlacement{}
				c.holding = false
			}
			if !found || !current.sameOwner(owner) {
				c.held = childPlacement{}
				c.holding = false
				return false
			}
			return c.deliver(current, ev)
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
	c.Give(at.index)
	if !c.deliver(at, ev) {
		return false
	}
	c.held = at
	c.holding = true
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

func (c *Container) placed(want childPlacement) (childPlacement, bool) {
	for _, child := range c.presentation.Value() {
		if child.sameSlot(want) {
			return child, true
		}
	}
	return childPlacement{}, false
}

// arrange rebuilds the slots from the items, asking each child that can measure
// itself to do so.
func (c *Container) arrange() []layout.Slot {
	return c.arrangeItems(c.items)
}

func (c *Container) arrangeItems(items []Item) []layout.Slot {
	clear(c.slots)
	c.slots = c.slots[:0]
	for _, item := range items {
		slot := layout.Slot{Size: item.Size}
		if measurer, ok := item.Of.(layout.Measurer); ok {
			slot.Of = measurer
		}
		c.slots = append(c.slots, slot)
	}
	c.slots = trim(c.slots)
	return c.slots
}

type childPlacement struct {
	index int
	key   string
	child Widget
	area  image.Rectangle
	frame frameStamp
}

func (p childPlacement) sameOwner(other childPlacement) bool {
	if p.frame == other.frame {
		return true
	}
	if p.key != "" && p.key == other.key {
		return true
	}
	return identity.Same(p.child, other.child)
}

func (p childPlacement) sameSlot(other childPlacement) bool {
	if p.key != "" || other.key != "" {
		return p.key != "" && p.key == other.key
	}
	return p.index == other.index
}

// settle makes sure the keyboard is somewhere it can be, and that every child has
// been told where it stands.
//
// It runs before anything else this container does, because the items can be rebuilt
// between frames and the child that had the keyboard may no longer be there.
func (c *Container) settle() {
	want := c.focused
	if !c.focusable(want) {
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
func (c *Container) move(to int) {
	if !c.focusable(to) {
		to = -1
	}
	if c.settled && to == c.focused {
		return
	}
	from := c.holder
	next := c.widgetAt(to)
	kept := identity.Same(from, next)
	c.focused = to
	c.holder = next
	// The one that had it is told first and by name, because it may be the reason
	// the keyboard moved at all: a child taken out of the items is no longer in the
	// loop below, and would otherwise go on believing it has the keyboard.
	if from != nil && !kept {
		tell(from, false)
	}
	for i, item := range c.items {
		if i != to {
			tell(item.Of, false)
		}
	}
	if !kept {
		tell(c.holder, !c.blurred)
	}
}

// step moves the keyboard along the ring by one, in the given direction.
func (c *Container) step(by int) bool {
	c.settle()
	n := len(c.items)
	if n == 0 {
		return false
	}
	from := c.focused
	for offset := 1; offset <= n; offset++ {
		i := ((from+by*offset)%n + n) % n
		if _, ok := c.items[i].Of.(Focusable); ok {
			if i == c.focused {
				return false
			}
			c.move(i)
			return true
		}
	}
	return false
}

// first is the earliest child that will take the keyboard, or -1.
func (c *Container) first() int {
	for i, item := range c.items {
		if _, ok := item.Of.(Focusable); ok {
			return i
		}
	}
	return -1
}

func (c *Container) focusable(index int) bool {
	if index < 0 || index >= len(c.items) {
		return false
	}
	_, ok := c.items[index].Of.(Focusable)
	return ok
}

func (c *Container) widgetAt(index int) Widget {
	if index < 0 || index >= len(c.items) {
		return nil
	}
	return c.items[index].Of
}

func (c *Container) focusKey() string {
	if c.focused < 0 || c.focused >= len(c.items) {
		return ""
	}
	return c.items[c.focused].Key
}

func (c *Container) indexOfKey(key string) int {
	for i, item := range c.items {
		if item.Key == key {
			return i
		}
	}
	return -1
}

func checkItemKeys(items []Item) {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.Key == "" {
			continue
		}
		if _, exists := seen[item.Key]; exists {
			panic("headless: duplicate container item key")
		}
		seen[item.Key] = struct{}{}
	}
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
