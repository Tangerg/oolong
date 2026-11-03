package base

import (
	"image"
	"slices"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

// List owns one-row items, selection and scrolling. Moving the cursor keeps it
// visible; Row supplies appearance. The zero value is ready. A List must not be
// copied after first use. Methods panic on a nil *List.
type List[T any] struct {
	changes  subscriptions[struct{}]
	element  scene.Element
	geometry eventGeometry
	noCopy   noCopy

	// items are private so replacing them cannot bypass selection clamping or leave
	// committed routing totals describing a collection the list no longer owns.
	items   []T
	itemsID *byte
	// Indices distinguish equal items for row appearance. Callbacks run during drawing
	// and must be observationally pure.
	Row func(v grid.View, at int, item T, selected bool)
	// Nil uses [DefaultListKeys].
	Keys *keymap.Map
	// Wrap enables movement across the ends; the default stops at each end.
	Wrap bool

	revision        uint64
	contentRevision uint64
	selected        int
	scroll          Scroll
	// presentation is the committed window and first visible row. Both are routing
	// geometry: a page and a press must refer to the same complete frame.
	presentation scene.Snapshot[listPresentation]
	pointer      Pointer
	matcher      keymap.Matcher
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
	requireMutable(l.element)
	next := l.clampIndex(i)
	if next != l.selected {
		l.selected = next
		l.revision++
		l.reveal(next)
		l.notifyChange()
	}
}

// Move shifts the selection by n items, wrapping only if asked to.
func (l *List[T]) Move(n int) {
	if len(l.items) == 0 {
		return
	}
	l.Select(moveIndex(l.selected, n, len(l.items), l.Wrap))
}

// SetItems copies the replacement collection and clamps selection by index. Pointer
// routing waits for its next complete frame so old geometry cannot select new items.
func (l *List[T]) SetItems(items []T) {
	requireMutable(l.element)
	l.matcher.Reset(func() {
		l.replaceItems(items, l.selected)
		l.notifyChange()
	})
}

// Install rows, cursor and routing identity without calling observers. A composed
// control can complete its own state before publishing this one list change.
func (l *List[T]) replaceItems(items []T, selected int) {
	requireMutable(l.element)
	l.revision++
	l.contentRevision++
	l.pointer.Left()
	l.items = own(l.items, items)
	l.itemsID = new(byte)
	next := l.clampIndex(selected)
	if len(l.items) == 0 {
		l.scroll.toTop()
	} else if next != l.selected {
		l.reveal(next)
	}
	l.selected = next
}

// Items returns a copy of the items in list order.
func (l *List[T]) Items() []T {
	return slices.Clone(l.items)
}

func (l *List[T]) Len() int {
	return len(l.items)
}

func (l *List[T]) At(index int) (T, bool) {
	items := l.items
	if index >= 0 && index < len(items) {
		return items[index], true
	}
	var zero T
	return zero, false
}

// Hit returns the item index at point in the list's local coordinates, using the
// visible area and scroll position of its last complete frame. Clipped points and
// blank rows miss. Before the first draw, or after SetItems until a replacement
// frame commits, no point hits. Hit changes neither selection nor scrolling.
func (l *List[T]) Hit(point image.Point) (int, bool) {
	presented := l.geometry.read(&l.presentation)
	if presented.itemsID != l.itemsID || !point.In(presented.visible) {
		return 0, false
	}
	at := presented.first + point.Y
	if at >= presented.total {
		return 0, false
	}
	return at, true
}

// Handle answers keys, the wheel and a press, reporting whether it consumed the event.
func (l *List[T]) Handle(ev input.Event) bool {
	requireMutable(l.element)
	if mouse, ok := ev.(input.Mouse); ok {
		return l.mouse(mouse, l.Select)
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := l.matcher.Handle(l.keys(), key, l.Do)
	return handled
}

// Do handles list actions and reports whether the action was accepted.
func (l *List[T]) Do(action keymap.Action) bool {
	return l.navigate(action, l.Select)
}

func (l *List[T]) navigate(action keymap.Action, choose func(int)) bool {
	before := l.Selected()
	at, known := l.target(action, before)
	if !known {
		return false
	}
	choose(at)
	if l.Selected() == before {
		// Explicit navigation requests visibility even at a boundary.
		if l.reveal(before) {
			l.element.Invalidate()
		}
	}
	return true
}

func (l *List[T]) target(action keymap.Action, from int) (int, bool) {
	page := max(l.geometry.read(&l.presentation).window-1, 1)
	var delta int
	switch action {
	case SelectPrev:
		delta = -1
	case SelectNext:
		delta = 1
	case SelectPageUp:
		delta = -page
	case SelectPageDown:
		delta = page
	case SelectFirst:
		return l.clampIndex(0), true
	case SelectLast:
		return l.clampIndex(len(l.items) - 1), true
	default:
		return 0, false
	}
	if len(l.items) == 0 {
		return 0, true
	}
	return moveIndex(from, delta, len(l.items), l.Wrap), true
}

func (l *List[T]) mouse(ev input.Mouse, choose func(int)) bool {
	switch ev.Action {
	case input.MouseCancel:
		l.pointer.Left()
		return false
	case input.WheelUp, input.WheelDown:
		if !ev.Pos.In(l.geometry.read(&l.presentation).visible) {
			return false
		}
		return l.scroll.Handle(ev, nil)
	case input.MouseDown:
		l.pointer.Left()
		if ev.Button != input.ButtonLeft {
			return false
		}
		if _, hit := l.Hit(ev.Pos); !hit {
			return false
		}
	case input.MouseDrag:
		if !ev.Drags(input.ButtonLeft) {
			return false
		}
	case input.MouseUp:
		return l.pointer.Handle(ev)
	default:
		return false
	}
	if !l.pointer.Handle(ev) {
		return false
	}
	at, ok := l.Hit(ev.Pos)
	if !ok {
		return false
	}
	choose(at)
	return true
}

func (l *List[T]) keys() *keymap.Map {
	if l.Keys != nil {
		return l.Keys
	}
	return listKeys()
}

func (l *List[T]) HeightForWidth(int) int { return len(l.items) }

// Losing focus cancels an unfinished gesture or key sequence.
func (l *List[T]) focusChanged(has bool) {
	if !has {
		l.pointer.Left()
		l.matcher.Clear()
	}
}

func (l *List[T]) Focused() bool { return l.element.Focused() }

func (l *List[T]) Scroll() *Scroll { return &l.scroll }

func (l *List[T]) Draw(v scene.Frame) {
	l.DrawRows(v, l.Row)
}

// DrawRows paints visible items with a frame-local renderer while List retains
// selection, scrolling and pointer routing. Callbacks must be observationally pure.
// It panics unless the list is mounted in the frame's scene outside read-only projection.
func (l *List[T]) DrawRows(v scene.Frame, draw func(grid.View, int, T, bool)) {
	requireProjection(l.element, v)
	width, height := v.Size()
	total := len(l.items)
	scroll := l.scroll.Stage(v, total, height)
	selected := l.Selected()
	first := scroll.Offset()
	visible := v.Visible()
	l.presentation.Stage(v, listPresentation{
		visible: visible, window: height, first: first, total: total, itemsID: l.itemsID,
	})
	l.pointer.Stage(v, v.Bounds())
	if draw == nil || visible.Empty() {
		return
	}
	end := min(visible.Max.Y, total-first)
	for y := visible.Min.Y; y < end; y++ {
		index := first + y
		row := v.Sub(grid.Area(0, y, width, 1)).View
		draw(row, index, l.items[index], index == selected)
	}
}

func (l *List[T]) reveal(at int) bool {
	if len(l.items) == 0 {
		return false
	}
	return l.scroll.revealContent(len(l.items), at, at)
}

type listPresentation struct {
	itemsID              *byte
	visible              image.Rectangle
	window, first, total int
}

func (l *List[T]) clampIndex(i int) int {
	if len(l.items) == 0 {
		return 0
	}
	return min(max(i, 0), len(l.items)-1)
}

// Subscribe observes completed collection or selection changes on the UI owner
// sequence. The returned function removes the observer and may be called twice.
// It panics for a nil callback.
func (l *List[T]) Subscribe(callback func()) func() {
	if callback == nil {
		panic("base: nil list subscriber")
	}
	return l.changes.add(func(struct{}) { callback() })
}

func (l *List[T]) notifyChange() {
	l.changes.publish(struct{}{}, l.element.Invalidate)
}
