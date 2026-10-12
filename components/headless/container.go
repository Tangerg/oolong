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

// Focusable participates in explicit keyboard ownership. A Container assigns focus
// so two input consumers cannot claim one key or cursor. A standalone widget assumes
// focus until told otherwise; pointer handling alone does not imply ring membership.
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
	// position. Name a child when it may move: focus follows its logical slot.
	// Pointer gestures follow the same concrete child in that slot. Replacing a
	// child cancels its gesture; non-comparable value widgets are replacements
	// on every Set, so use a pointer when retaining their identity matters.
	// Non-empty keys must be unique within one container.
	Key string
	// Size is how much of the divided axis this child takes. It means exactly what
	// it means in [layout.Slot], including the zero value, which asks for nothing —
	// [layout.Fixed] of zero also allocates nothing, but explicitly names a fixed
	// policy; [layout.Sizing.IsZero] distinguishes it from omission.
	//
	// A child as big as its content wants to be is [layout.Measured].
	Size layout.Sizing
	Of   Widget
}

// Container owns child layout, keyboard focus and pointer routing. Accepted presses
// capture drag and release outside the child; foreign-button releases preserve capture.
// Removed recipients receive cancellation before hover leave.
// Appearance remains with callers. The zero value is an empty column. A Container
// must not be copied after first use.
type Container struct {
	noCopy noCopy

	// Axis is which way the children are arranged. The zero value stacks them down
	// the region.
	Axis layout.Axis
	// items are the children, in arrangement and keyboard order. They are private so
	// replacing them cannot bypass focus settlement or leave pointer capture owned by
	// a child that is no longer present.
	//
	items []containerItem
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
	// holder is published before focus callbacks. Keeping it beside the index lets
	// Set release a removed child without comparing interface values.
	holder Widget
	focusState

	// presentation is the child identities and areas from the last complete root
	// frame. Identity belongs in the snapshot with geometry: if Items is reordered
	// before the next draw, input still goes to what the user can actually see.
	presentation Snapshot[[]childPlacement]
	// capture is the exact presented child a press was given to. Everything up to the
	// release goes back to it; a later frame may update its geometry when its identity
	// is still demonstrably the same.
	capture pointerCapture[childPlacement]
	hover   childPlacement
	slots   []layout.Slot
	matcher keymap.Matcher
}

// NewContainer constructs a container that arranges its children along axis.
// [layout.Down] stacks rows and [layout.Across] places columns side by side.
//
// Repeating a non-empty [Item.Key] is a programmer error and panics, here and in
// [Container.Set] and [Container.Add]. A key exists so focus and an in-progress
// pointer gesture follow a child that moves; two children answering to one key would
// send them to whichever was found first, which changes as the children are reordered.
func NewContainer(axis layout.Axis, items ...Item) *Container {
	c := &Container{Axis: axis}
	c.Set(items...)
	return c
}

// Set replaces the children. Focus follows a non-empty [Item.Key], or the old position
// for an unnamed item. A repeated key is a programmer error and panics, as described
// on [NewContainer].
func (c *Container) Set(items ...Item) {
	checkItemKeys(items)
	key := c.focusKey()
	at := c.focused
	next := make([]containerItem, len(items))
	for i, item := range items {
		next[i].Item = item
		next[i].Key = strings.Clone(item.Key)
		old := i
		if item.Key != "" {
			old = c.indexOfKey(item.Key)
		}
		if old >= 0 && old < len(c.items) && c.items[old].Key == item.Key && identity.Same(c.items[old].Of, item.Of) {
			next[i].identity = c.items[old].identity
		} else {
			next[i].identity = new(byte)
		}
	}
	c.items = next
	if key != "" {
		at = c.indexOfKey(key)
	} else if len(c.items) > 0 {
		at = min(max(at, 0), len(c.items)-1)
	}
	c.focused = at
	c.state.Unsettle()
	c.settle()
}

// Add appends a child and returns the container, so a tree can be built in one
// expression. A key already used by a child that is staying is a programmer error and
// panics, as described on [NewContainer].
func (c *Container) Add(items ...Item) *Container {
	checkItemKeys(append(c.Items(), items...))
	for _, item := range items {
		item.Key = strings.Clone(item.Key)
		c.items = append(c.items, containerItem{Item: item, identity: new(byte)})
	}
	c.state.Unsettle()
	c.settle()
	return c
}

// Items returns a copy of the children in arrangement and keyboard order.
func (c *Container) Items() []Item {
	if c == nil {
		return nil
	}
	items := make([]Item, len(c.items))
	for i, child := range c.items {
		items[i] = child.Item
	}
	return items
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

// FocusIndex hands the keyboard to the child at index, reporting whether it took it. An
// index that does not exist, or a child that does not want the keyboard, is declined.
// Addressing the item rather than comparing Widget interface values keeps this API
// valid for every implementation the interface permits.
func (c *Container) FocusIndex(index int) bool {
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
	c.change(has, c.settle, &c.holder, &c.matcher)
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
			identity: item.identity, child: item.Of, area: rects[i], visible: rects[i].Intersect(v.Visible()),
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

// HeightForWidth reports the container height after allocating its children at width.
func (c *Container) HeightForWidth(width int) int { return c.Measure(layout.Down, width) }

// WidthForHeight reports the container width after allocating its children at height.
func (c *Container) WidthForHeight(height int) int { return c.Measure(layout.Across, height) }

// Measure computes an intrinsic extent along the explicitly requested axis.
// Along the container's flow it sums child requests; across its flow it allocates
// the supplied extent first, then takes the largest child cross-axis request.
func (c *Container) Measure(axis layout.Axis, across int) int {
	if axis == c.Axis {
		return c.measureWith(across, c.flow())
	}
	sizes := c.flow().Divide(across, 0, c.arrange())
	wanted := 0
	for i, item := range c.items {
		wanted = max(wanted, measureWidget(item.Of, axis, sizes[i]))
	}
	return wanted
}

func measureWidget(widget Widget, axis layout.Axis, across int) int {
	if measurer, ok := widget.(layout.Measurer); ok {
		return measurer.Measure(axis, across)
	}
	if axis == layout.Down {
		if sized, ok := widget.(Sized); ok {
			return sized.HeightForWidth(across)
		}
	} else if sized, ok := widget.(interface{ WidthForHeight(height int) int }); ok {
		return sized.WidthForHeight(across)
	}
	return 0
}

func (c *Container) measureWith(across int, flow layout.Flow) int {
	return flow.Wanted(across, c.arrange())
}

func (c *Container) flow() layout.Flow {
	return layout.Flow{Axis: c.Axis, Gap: c.Gap}
}

// Handle gives the event to whichever child it is for.
//
// A key goes to the child with the keyboard, and the ring is walked only if that
// child did not want it. Anything nobody wanted is declined, so a container inside a
// container passes what it cannot use back up rather than swallowing it.
func (c *Container) Handle(ev input.Event) bool {
	if c.handleChild(ev) {
		return true
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := c.matcher.Handle(c.keys(), key, c.Do)
	return handled
}

func (c *Container) handleChild(ev input.Event) bool {
	c.settle()
	if mouse, ok := ev.(input.Mouse); ok {
		return c.mouse(mouse)
	}
	if handler, ok := c.holder.(Interactive); ok {
		return c.state.Forward(func() bool { return handler.Handle(ev) })
	}
	return false
}

// Do runs one of the container's actions by name, reporting whether the action
// was accepted. See [Doer].
func (c *Container) Do(action keymap.Action) bool {
	switch action {
	case FocusNext:
		return c.FocusNext()
	case FocusPrev:
		return c.FocusPrev()
	}
	return false
}

func (c *Container) mouse(ev input.Mouse) bool {
	held, turn := c.capture.begin(ev)
	if !c.capture.current(turn) {
		return true
	}
	if ev.Action == input.MouseCancel {
		return false
	}
	if ev.Action == input.MouseLeave {
		c.leave(ev)
		return false
	}
	if held.continues(ev) {
		current, found := c.placed(held.target)
		if held.cancelled || !found || c.currentIndex(current) < 0 {
			if c.capture.cancel(turn, held, ev) {
				c.leave(ev)
			}
			return !c.capture.current(turn)
		}
		return c.deliver(current, ev, turn)
	}
	at, found := c.at(ev.Pos)
	if !found || c.currentIndex(at) < 0 {
		c.leave(ev)
		return false
	}
	if ev.Action != input.MouseDown {
		return c.deliver(at, ev, turn)
	}
	_, revision := c.presentation.borrow()
	// A press moves the keyboard whether or not the child does anything with the
	// press itself: clicking a pane is how a user says they mean that one.
	c.FocusIndex(c.currentIndex(at))
	if !c.capture.current(turn) || !c.presentation.unchanged(revision) {
		return true
	}
	if c.currentIndex(at) < 0 {
		c.leave(ev)
		return false
	}
	return c.deliver(at, ev, turn)
}

// Children use local clipped coordinates; translate input with the same geometry
// used to draw them.
func (c *Container) deliver(to childPlacement, ev input.Mouse, turn uint64) bool {
	_, revision := c.presentation.borrow()
	c.hoverTo(to, ev)
	if !c.capture.current(turn) || !c.presentation.unchanged(revision) || c.hover.identity != to.identity || c.currentIndex(to) < 0 {
		return true
	}
	handled := c.capture.deliver(turn, to, ev)
	return handled || !c.capture.current(turn) || !c.presentation.unchanged(revision) || c.currentIndex(to) < 0
}

func (to childPlacement) deliverPointer(ev input.Mouse) bool {
	handler, ok := to.child.(Interactive)
	if !ok {
		return false
	}
	local := ev
	local.Pos = ev.Pos.Sub(to.area.Min)
	return handler.Handle(local)
}

func (c *Container) leave(ev input.Mouse) {
	c.hoverTo(childPlacement{}, ev)
}

func (c *Container) hoverTo(to childPlacement, ev input.Mouse) {
	previous := c.hover
	c.hover = to
	if previous.identity == to.identity {
		return
	}
	if handler, ok := previous.child.(Interactive); ok {
		ev.Action = input.MouseLeave
		ev.Pos = ev.Pos.Sub(previous.area.Min)
		handler.Handle(ev)
	}
}

func (c *Container) at(p image.Point) (childPlacement, bool) {
	for _, child := range c.presentation.Value() {
		if p.In(child.visible) {
			return child, true
		}
	}
	return childPlacement{}, false
}

func (c *Container) placed(want childPlacement) (childPlacement, bool) {
	for _, child := range c.presentation.Value() {
		if child.identity == want.identity {
			return child, true
		}
	}
	return childPlacement{}, false
}

func (c *Container) arrange() []layout.Slot {
	return c.arrangeItems(c.items)
}

func (c *Container) arrangeItems(items []containerItem) []layout.Slot {
	clear(c.slots)
	c.slots = c.slots[:0]
	for _, item := range items {
		slot := layout.Slot{
			Size: item.Size,
			Of:   layout.MeasureFunc(func(axis layout.Axis, across int) int { return measureWidget(item.Of, axis, across) }),
		}
		c.slots = append(c.slots, slot)
	}
	c.slots = trim(c.slots)
	return c.slots
}

// containerItem owns a child and its attachment identity together. A replacement
// cannot update one without constructing the other.
type containerItem struct {
	Item
	identity *byte
}

type childPlacement struct {
	identity *byte
	child    Widget
	area     image.Rectangle
	visible  image.Rectangle
}

// currentIndex resolves a presented attachment, never an index from another frame.
// Removed and replaced children are inert until their replacements are drawn.
func (c *Container) currentIndex(p childPlacement) int {
	return slices.IndexFunc(c.items, func(item containerItem) bool { return item.identity == p.identity })
}

// Children can be replaced between frames; settle focus before any operation can
// use a removed focus holder.
func (c *Container) settle() {
	want := c.focused
	if !c.focusable(want) {
		want = c.first()
	}
	c.move(want)
}

// Notify every child because standalone widgets assume focus until explicitly blurred.
func (c *Container) move(to int) {
	if !c.focusable(to) {
		to = -1
	}
	if c.state.Settled() && to == c.focused {
		return
	}
	from := c.holder
	next := c.widgetAt(to)
	kept := identity.Same(from, next)
	c.focused = to
	c.holder = next
	turn := c.begin()
	// The one that had it is told first and by name, because it may be the reason
	// the keyboard moved at all: a child taken out of the items is no longer in the
	// loop below, and would otherwise go on believing it has the keyboard.
	if from != nil && !kept {
		tell(from, false)
		if c.superseded(turn) {
			return
		}
	}
	for i, item := range c.items {
		if i != to && !identity.Same(item.Of, next) {
			tell(item.Of, false)
			if c.superseded(turn) {
				return
			}
		}
	}
	if !kept {
		tell(c.holder, c.state.Focused())
	}
}

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

func (c *Container) keys() *keymap.Map {
	if c.Keys != nil {
		return c.Keys
	}
	return containerKeys()
}

func tell(w Widget, has bool) {
	if focusable, ok := w.(Focusable); ok {
		focusable.Focus(has)
	}
}
