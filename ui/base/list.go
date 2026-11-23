package base

import (
	"image"
	"slices"
	"strconv"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

// List owns one-row items, selection and scrolling. Moving the cursor keeps it
// visible; Row supplies appearance. The zero value is ready. A List must not be
// copied after first use.
// Replacing items panics before mutation if the collection revision is exhausted.
type List[T any] struct {
	rows[T]
	noCopy noCopy
}

// rows is what every list-shaped controller shares: owned items, the cursor,
// scrolling, pointer routing and navigation keys. It has no element of its own;
// the controller that embeds it supplies the one identity its scene sees, so no
// collaborator can construct a second identity for the same rows. A controller
// with its own replacement policy, such as Table's sort, shadows SetItems.
type rows[T any] struct {
	changes  subscriptions[struct{}]
	element  scene.Element
	geometry eventGeometry

	// items are private so replacing them cannot bypass selection clamping or leave
	// committed routing totals describing a collection the list no longer owns.
	items []T
	// Indices distinguish equal items for row appearance. Callbacks run during drawing
	// and must be observationally pure.
	rowPainter func(v grid.View, at int, item T, selected bool)
	// Nil uses [DefaultListKeys].
	keys keyBinding
	// Wrap enables movement across the ends; the default stops at each end.
	wrap     bool
	activate func(at int, item T)
	same     func(a, b T) bool
	// pressedRow is the row a captured left press began on. A drag to another row
	// clears it, so a press that moved the cursor cannot also activate.
	pressedRow int

	// Cursor movement revokes delayed row actions without changing row addresses.
	version         *byte
	contentRevision uint64
	selected        int
	scroll          Scroll
	// presentation is the committed window and first visible row. Both are routing
	// geometry: a page and a press must refer to the same complete frame.
	presentation scene.Snapshot[listPresentation]
	pointer      Pointer
	matcher      keymap.Matcher
}

type ListConfig[T any] struct {
	Items []T
	Row   func(grid.View, int, T, bool)
	Keys  *keymap.Map
	Wrap  bool
	// Activate opens or chooses an item: the [Activate] key on the item under the
	// cursor, or a click that presses and releases the same row. It runs on the UI
	// owner and may change the list. Nil leaves the list to selection alone, and
	// the activation key falls through to the containing control.
	Activate func(at int, item T)
	// Same reports whether two items are one item, so replacing the items keeps the
	// cursor on that item wherever it moved. Nil keeps the cursor's index. It must
	// be observationally pure; [Equal] compares comparable values.
	Same func(a, b T) bool
}

func NewList[T any](config ListConfig[T]) *List[T] {
	l := &List[T]{}
	l.configure(config)
	l.SetItems(config.Items)
	return l
}

// configure installs construction settings before the rows have an identity.
func (l *rows[T]) configure(config ListConfig[T]) {
	l.rowPainter, l.keys.current, l.wrap = config.Row, config.Keys, config.Wrap
	l.activate, l.same = config.Activate, config.Same
}

// adopt makes element the identity these rows mutate, scroll and bind keys under.
func (l *rows[T]) adopt(element scene.Element) {
	l.element = element
	l.scroll.element = element
	l.keys.attach(element, &l.matcher)
}

// Selected is the index under the cursor, or -1 for an empty list.
func (l *rows[T]) Selected() int {
	if len(l.items) == 0 {
		return -1
	}
	return l.clampIndex(l.selected)
}

// Current is the item under the cursor, and whether there was one.
func (l *rows[T]) Current() (T, bool) {
	i := l.Selected()
	if i < 0 {
		var zero T
		return zero, false
	}
	return l.items[i], true
}

// Select moves the cursor to an index, clamped to the list.
func (l *rows[T]) Select(i int) {
	requireMutable(l.element)
	next := l.clampIndex(i)
	if next != l.selected {
		l.selected = next
		l.version = new(byte)
		l.reveal(next)
		l.notifyChange()
	}
}

// Move shifts the selection by n items, wrapping only if asked to.
func (l *rows[T]) Move(n int) {
	if len(l.items) == 0 {
		return
	}
	l.Select(moveIndex(l.selected, n, len(l.items), l.wrap))
}

// SetItems copies the replacement collection and clamps selection by index. Pointer
// routing waits for its next complete frame so old geometry cannot select new items.
func (l *rows[T]) SetItems(items []T) {
	requireMutable(l.element)
	l.requireReplacement()
	at := l.followed(items, l.selected)
	l.matcher.Reset(func() {
		l.replaceItems(items, at)
		l.notifyChange()
	})
}

// followed is where the cursor belongs among items: on the item Same identifies
// with the one under the cursor now, or at fallback when nothing identifies it.
func (l *rows[T]) followed(items []T, fallback int) int {
	current, ok := l.Current()
	if !ok || l.same == nil {
		return fallback
	}
	at := fallback
	borrowed(l.element, func() {
		if i := slices.IndexFunc(items, func(item T) bool { return l.same(current, item) }); i >= 0 {
			at = i
		}
	})
	return at
}

// Install rows, cursor and routing identity without calling observers. A composed
// control can complete its own state before publishing this one list change.
func (l *rows[T]) replaceItems(items []T, selected int) {
	requireMutable(l.element)
	l.contentRevision = nextRevision(l.contentRevision)
	l.version = new(byte)
	l.pointer.Left()
	l.items = own(l.items, items)
	next := l.clampIndex(selected)
	if len(l.items) == 0 {
		l.scroll.toTop()
	} else if next != l.selected {
		l.reveal(next)
	}
	l.selected = next
}

func (l *rows[T]) requireReplacement() { _ = nextRevision(l.contentRevision) }

// Items returns a copy of the items in list order.
func (l *rows[T]) Items() []T {
	return slices.Clone(l.items)
}

func (l *rows[T]) Len() int {
	return len(l.items)
}

func (l *rows[T]) At(index int) (T, bool) {
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
func (l *rows[T]) Hit(point image.Point) (int, bool) {
	presented := l.geometry.read(&l.presentation)
	if presented.contentRevision != l.contentRevision || !point.In(presented.visible) {
		return 0, false
	}
	at := presented.first + point.Y
	if at >= presented.total {
		return 0, false
	}
	return at, true
}

// Handle answers keys, the wheel and a press, reporting whether it consumed the event.
func (l *rows[T]) Handle(ev input.Event) bool {
	requireMutable(l.element)
	if mouse, ok := ev.(input.Mouse); ok {
		return l.mouse(mouse, l.Select)
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := l.matcher.Handle(l.resolvedKeys(), key, keyboardReceipt(&l.element), l.Do)
	return handled
}

// Do handles list actions and reports whether the action was accepted. Activate is
// accepted only when the list has an activation and an item under the cursor.
func (l *rows[T]) Do(action keymap.Action) bool {
	if action == Activate {
		return l.activateAt(l.Selected())
	}
	return l.navigate(action, l.Select)
}

func (l *rows[T]) activateAt(at int) bool {
	if l.activate == nil || at < 0 || at >= len(l.items) {
		return false
	}
	l.activate(at, l.items[at])
	return true
}

func (l *rows[T]) navigate(action keymap.Action, choose func(int)) bool {
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

func (l *rows[T]) target(action keymap.Action, from int) (int, bool) {
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
	return moveIndex(from, delta, len(l.items), l.wrap), true
}

func (l *rows[T]) mouse(ev input.Mouse, choose func(int)) bool {
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
		at, hit := l.Hit(ev.Pos)
		if !hit {
			return false
		}
		l.pressedRow = at
	case input.MouseDrag:
		if !ev.Drags(input.ButtonLeft) {
			return false
		}
		if at, hit := l.Hit(ev.Pos); !hit || at != l.pressedRow {
			l.pressedRow = -1
		}
	case input.MouseUp:
		return l.release(ev)
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

// release settles a left press, activating the row it began and ended on.
func (l *rows[T]) release(ev input.Mouse) bool {
	if !l.pointer.Handle(ev) {
		return false
	}
	if l.pointer.Clicked(input.ButtonLeft) {
		if at, hit := l.Hit(ev.Pos); hit && at == l.pressedRow {
			l.activateAt(at)
		}
	}
	return true
}

func (l *rows[T]) resolvedKeys() *keymap.Map { return l.keys.resolve(listKeys) }

func (l *rows[T]) HeightForWidth(int) int { return len(l.items) }

// Losing focus cancels an unfinished gesture or key sequence.
func (l *rows[T]) focusChanged(has bool) {
	if !has {
		l.pointer.Left()
		l.matcher.Clear()
	}
}

func (l *rows[T]) Focused() bool { return l.element.Focused() }

func (l *rows[T]) Scroll() *Scroll { return &l.scroll }

func (l *rows[T]) Draw(v scene.Frame) {
	l.DrawRows(v, l.rowPainter)
}

// DrawRows paints visible items with a frame-local renderer while List retains
// selection, scrolling and pointer routing. Callbacks must be observationally pure.
// It panics unless the list is mounted in the frame's scene outside read-only projection.
func (l *rows[T]) DrawRows(v scene.Frame, draw func(grid.View, int, T, bool)) {
	requireProjection(l.element, v)
	width, height := v.Size()
	total := len(l.items)
	scroll := l.scroll.Stage(v, total, height)
	selected := l.Selected()
	first := scroll.Offset()
	visible := v.Visible()
	l.presentation.Stage(v, listPresentation{
		visible: visible, window: height, first: first, total: total, contentRevision: l.contentRevision,
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

func (l *rows[T]) reveal(at int) bool {
	if len(l.items) == 0 {
		return false
	}
	return l.scroll.revealContent(len(l.items), at, at)
}

type listPresentation struct {
	contentRevision      uint64
	visible              image.Rectangle
	window, first, total int
}

func (l *rows[T]) clampIndex(i int) int {
	if len(l.items) == 0 {
		return 0
	}
	return min(max(i, 0), len(l.items)-1)
}

// Subscribe observes completed collection or selection changes on the UI owner
// sequence. The returned function removes the observer and may be called twice.
// It panics for a nil callback.
func (l *rows[T]) Subscribe(callback func()) func() {
	if callback == nil {
		panic("base: nil list subscriber")
	}
	return l.changes.add(func(struct{}) { callback() })
}

func (l *rows[T]) notifyChange() {
	l.changes.publish(struct{}{}, l.element.Invalidate)
}

func (l *rows[T]) Row() func(grid.View, int, T, bool) { return l.rowPainter }

func (l *rows[T]) SetRow(value func(grid.View, int, T, bool)) {
	setting.Replace(l.element, &l.rowPainter, value)
}

func (l *rows[T]) Wrap() bool { return l.wrap }

func (l *rows[T]) SetWrap(value bool) {
	setting.Assign(l.element, &l.wrap, value)
}

func (l *rows[T]) Keys() *keymap.Map { return l.keys.current }

// KeysFor is what the effective key map binds to action, defaults included.
func (l *rows[T]) KeysFor(action keymap.Action) []input.Keys { return l.keys.bound(listKeys, action) }

func (l *rows[T]) SetKeys(keys *keymap.Map) {
	l.keys.set(l.element, &l.matcher, keys)
}

func (l *List[T]) Element() scene.Element {
	if l.element.IsZero() {
		l.element = scene.NewElement(scene.ElementConfig{
			Measure: layout.MeasureRows(l.HeightForWidth), Paint: l.Draw,
			Handle: func(e scene.Event) scene.EventResult { return partEvent(e, l.Handle, &l.geometry, &l.pointer.geometry) }, Focusable: func() bool { return true }, FocusChanged: l.focusChanged,
			Semantics: func() scene.Semantic {
				return scene.Semantic{Role: scene.RoleList, ContentRevision: l.contentRevision, Value: strconv.Itoa(l.Selected())}
			}, Actions: append(collectionActions(&l.rows, unchangedItem[T]), collectionActivateAction(&l.rows)),
		})
	}
	l.adopt(l.element)
	return l.element
}
