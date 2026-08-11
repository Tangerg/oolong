package headless

import (
	"slices"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

// List is a vertical list of one-row items with a selection.
//
// It is generic over the item so a list of sessions and a list of files are the same
// widget with different rows, and so nothing here has to know what an item is. The
// row is drawn by a function the caller supplies: a list that formatted its own
// items would be a list that had opinions about them.
//
// Selection and scrolling are separate concerns that have to agree: moving the
// selection past the edge of the window scrolls to keep it visible, because a
// selection the user cannot see is a selection they will act on by mistake.
type List[T any] struct {
	// items are private so replacing them cannot bypass selection clamping or leave
	// committed routing totals describing a collection the list no longer owns.
	items []T
	// Row draws one item. at is where it sits among the items and selected says
	// whether it is the one under the cursor, which the caller renders however it
	// likes — a list does not know what selected looks like in its surroundings.
	//
	// The index is there because a row is often about more than the item: a number
	// down the left, a mark for what has been chosen, a colour that alternates. Only
	// the list knows it, and a caller finding it again by comparing items would be
	// guessing whenever two of them were alike.
	Row func(v grid.View, at int, item T, selected bool)
	// Keys say which keystrokes produce which of the actions the list answers to —
	// see [List.Do]. Nil reads through [DefaultListKeys].
	Keys *keymap.Map
	// Wrap moves the selection from the last item to the first and back. Off by
	// default: in a long list, wrapping loses the user's place.
	Wrap bool

	selected int
	blurred  bool
	scroll   Scroll
	// presentation is the committed window and first visible row. Both are routing
	// geometry: a page and a press must refer to the same complete frame.
	presentation Snapshot[listPresentation]
	// matcher owns how far into a multi-chord binding the keys have got.
	matcher keymap.Matcher
}

// Selected is the index under the cursor, or -1 for an empty list.
func (l *List[T]) Selected() int {
	if len(l.items) == 0 {
		return -1
	}
	return l.clampIndex(l.selected)
}

// Current is the item under the cursor, and whether there was one.
func (l *List[T]) Current() (T, bool) {
	i := l.Selected()
	if i < 0 {
		var zero T
		return zero, false
	}
	return l.items[i], true
}

// Select moves the cursor to an index, clamped to the list.
func (l *List[T]) Select(i int) {
	l.selected = l.clampIndex(i)
	l.reveal()
}

// Move shifts the selection by n items, wrapping only if asked to.
func (l *List[T]) Move(n int) {
	if len(l.items) == 0 {
		return
	}
	l.Select(moveIndex(l.selected, n, len(l.items), l.Wrap))
}

// SetItems replaces the contents, keeping the selection on the same index where
// that still exists. The list copies the slice; the caller may reuse or change its
// input after this returns.
//
// Keeping the index rather than the item: a list that is refreshed while the user is
// reading it should not jump, and following an item by identity would need this
// widget to know how to compare items, which is knowledge it has no business
// holding.
func (l *List[T]) SetItems(items []T) {
	l.items = own(l.items, items)
	l.selected = l.clampIndex(l.selected)
}

// Items returns a copy of the items in list order.
func (l *List[T]) Items() []T {
	if l == nil {
		return nil
	}
	return slices.Clone(l.items)
}

// Len reports how many items the list owns.
func (l *List[T]) Len() int {
	if l == nil {
		return 0
	}
	return len(l.items)
}

// At returns the item at index and whether it exists.
func (l *List[T]) At(index int) (T, bool) {
	if l != nil && index >= 0 && index < len(l.items) {
		return l.items[index], true
	}
	var zero T
	return zero, false
}

// Handle answers keys, the wheel and a press, reporting whether it consumed the event.
func (l *List[T]) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		return l.mouse(mouse)
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := l.matcher.Handle(l.keys(), key, l.Do)
	return handled
}

// Do runs one of the list's actions by name, reporting whether it was one this list
// knows. See [Doer].
func (l *List[T]) Do(action keymap.Action) bool {
	page := max(l.presentation.Value().window-1, 1)
	switch action {
	case SelectPrev:
		l.Move(-1)
	case SelectNext:
		l.Move(1)
	case SelectPageUp:
		l.Move(-page)
	case SelectPageDown:
		l.Move(page)
	case SelectFirst:
		l.Select(0)
	case SelectLast:
		l.Select(len(l.items) - 1)
	default:
		return false
	}
	return true
}

// mouse scrolls on the wheel and moves the selection to whatever was pressed.
//
// A row a reader pressed is the row they mean, and a drag carries the selection with
// it, which is what a list does everywhere. The position is in the list's own box —
// whoever drew it is responsible for that, which for anything inside a [Container] is
// the container.
func (l *List[T]) mouse(ev input.Mouse) bool {
	switch ev.Action {
	case input.WheelUp:
		l.scroll.By(l.scroll.wheel.Rows(ev.At, -1))
		return true
	case input.WheelDown:
		l.scroll.By(l.scroll.wheel.Rows(ev.At, 1))
		return true
	case input.MouseDown:
		if ev.Button != input.ButtonLeft {
			return false
		}
		return l.reach(ev.Pos.Y)
	case input.MouseDrag:
		return l.reach(ev.Pos.Y)
	default:
		return false
	}
}

// reach moves the selection to the row at a height in the box, when there is one there.
//
// Against the window the last frame drew, because a press arrives between two frames
// and is about the one on screen.
func (l *List[T]) reach(y int) bool {
	presented := l.presentation.Value()
	if presented.window <= 0 || y < 0 || y >= presented.window {
		return false
	}
	at := presented.first + y
	if at >= presented.total || at >= len(l.items) {
		return false
	}
	l.Select(at)
	return true
}

// keys is the map to read through, standing in the default for a caller who set none.
// A list is a struct a caller fills in, so its zero value has to work: one that quietly
// ignored the arrow keys would look finished and not be.
func (l *List[T]) keys() *keymap.Map {
	if l.Keys != nil {
		return l.Keys
	}
	return listKeys()
}

// Measure is one row per item, which is what a container needs to decide whether the
// list can have all the room it wants.
func (l *List[T]) Measure(int) int { return len(l.items) }

// Focus takes the keyboard, or gives it up.
//
// A list draws no differently for it: what a selection looks like in a list nobody
// is typing at is a matter of taste, and taste is what this layer refuses. It is here
// because a container hands the keyboard to its children by asking for this, so a
// list without it could not be one of them — and because [List.Focused] is how a row
// asks, which is where the answer belongs.
func (l *List[T]) Focus(has bool) {
	if !has {
		l.matcher.Clear()
	}
	l.blurred = !has
}

// Focused reports whether this list has the keyboard.
func (l *List[T]) Focused() bool { return !l.blurred }

// Scroll exposes the position, for a scrollbar drawn beside the list.
func (l *List[T]) Scroll() *Scroll { return &l.scroll }

// Draw paints the visible items.
func (l *List[T]) Draw(v Frame) {
	l.DrawRows(v, l.Row)
}

// DrawRows paints the visible items with row.
//
// Supplying the renderer for this frame lets an appearance component reuse
// geometry it computed once at the frame's width. The list still owns selection,
// scrolling and committed pointer routing; [List.Draw] is this method with Row.
func (l *List[T]) DrawRows(v Frame, draw func(grid.View, int, T, bool)) {
	l.drawRows(v, l.Selected(), draw)
}

// drawRows projects a selection supplied by an owning controller without changing
// this list's cursor. Select uses it to show a caller-owned initial value before the
// first semantic operation initializes the cursor.
func (l *List[T]) drawRows(v Frame, selected int, draw func(grid.View, int, T, bool)) {
	width, height := v.Size()
	total := len(l.items)
	scroll := l.scroll.Stage(v, total, height)
	if selected >= 0 {
		scroll.Reveal(selected)
	}
	first := scroll.Offset()
	l.presentation.Stage(v, listPresentation{window: height, first: first, total: total})
	if draw == nil {
		return
	}
	for y := range height {
		index := first + y
		if index >= total {
			break
		}
		row := v.Sub(grid.Rect(0, y, width, 1)).View
		draw(row, index, l.items[index], index == selected)
	}
}

// reveal scrolls the least amount that brings the selection into the window.
func (l *List[T]) reveal() {
	window := l.presentation.Value().window
	if window <= 0 || len(l.items) == 0 {
		return
	}
	l.scroll.layout(len(l.items), window)
	first := l.scroll.Offset()
	switch last := first + window - 1; {
	case l.selected < first:
		l.scroll.By(l.selected - first)
	case l.selected > last:
		l.scroll.By(l.selected - last)
	}
}

type listPresentation struct {
	window, first, total int
}

func (l *List[T]) clampIndex(i int) int {
	if len(l.items) == 0 {
		return 0
	}
	return min(max(i, 0), len(l.items)-1)
}
