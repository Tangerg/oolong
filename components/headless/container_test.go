package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

// A container is the piece that lets an interface be more than one thing. What is
// asserted here is the two routings — a key goes where the keyboard is, a mouse event
// goes where the pointer is — and that a widget which never hears about any of it
// still works on its own.

// field stands in for anything typed into: it records what it was told, in the
// coordinates it was told it in.
type field struct {
	name string
	// takes decides whether it claims what it is given, so a test can put a widget
	// that wants an event beside one that does not.
	takes bool

	keys    []input.Key
	mice    []input.Mouse
	focused bool
	// told counts how often it was told where it stands, so a test can check that
	// the answer is pushed on change and not every frame.
	told int
	area image.Rectangle
}

func (f *field) Draw(v headless.Frame) {
	w, h := v.Size()
	f.area = grid.Rect(0, 0, w, h)
	v.Text(0, 0, f.name, grid.Style{})
}

func (f *field) Handle(ev input.Event) bool {
	switch ev := ev.(type) {
	case input.Key:
		f.keys = append(f.keys, ev)
	case input.Mouse:
		f.mice = append(f.mice, ev)
	}
	return f.takes
}

func (f *field) Focus(has bool) {
	f.focused = has
	f.told++
}

// lastMouse is the last pointer event this field was given.
func (f *field) lastMouse(t *testing.T) input.Mouse {
	t.Helper()
	if len(f.mice) == 0 {
		t.Fatalf("%s was given no pointer event", f.name)
	}
	return f.mice[len(f.mice)-1]
}

// press is a left button press at a point.
func pressAt(x, y int) input.Mouse {
	return input.Mouse{Pos: image.Pt(x, y), Action: input.MouseDown, Button: input.ButtonLeft}
}

// drawn lays a container out, which is what gives a hit test something to answer
// against: a click is about a frame that has already been drawn.
func drawn(c *headless.Container, h int) {
	s := grid.NewSurface(10, h)
	headless.NewRoot(c).Draw(s.View())
}

func TestAKeyGoesToWhicheverChildHasTheKeyboard(t *testing.T) {
	first, second := &field{name: "first", takes: true}, &field{name: "second", takes: true}
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(1), Of: first},
		headless.Item{Size: layout.Fixed(1), Of: second},
	)

	if !c.Handle(input.Key{Code: input.Character, Rune: 'a'}) {
		t.Fatal("the key was not taken by anyone")
	}
	if len(first.keys) != 1 {
		t.Errorf("the first field got %d keys, want the one that was typed", len(first.keys))
	}
	if len(second.keys) != 0 {
		t.Errorf("the second field got %d keys, and it does not have the keyboard", len(second.keys))
	}

	c.Give(second)
	c.Handle(input.Key{Code: input.Character, Rune: 'b'})
	if len(second.keys) != 1 {
		t.Errorf("after being given the keyboard the second field got %d keys", len(second.keys))
	}
	if len(first.keys) != 1 {
		t.Errorf("the first field went on getting keys after losing the keyboard")
	}
}

// TestEveryChildIsToldWhereItStands, and only when it changes. A widget that has to
// be asked every frame is a widget that cannot draw itself from what it knows.
func TestEveryChildIsToldWhereItStands(t *testing.T) {
	first, second := &field{name: "first"}, &field{name: "second"}
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(1), Of: first},
		headless.Item{Size: layout.Fixed(1), Of: second},
	)
	drawn(c, 2)

	if !first.focused || second.focused {
		t.Fatalf("focus is first=%v second=%v, want it on the first child", first.focused, second.focused)
	}
	// The second was told it does not have the keyboard, rather than left believing
	// it does — which is what it believes until somebody says otherwise.
	if second.told != 1 {
		t.Errorf("the second child was told %d times, want once", second.told)
	}

	told := first.told
	drawn(c, 2)
	drawn(c, 2)
	if first.told != told {
		t.Errorf("a child was told again on a frame where nothing changed")
	}
}

func TestTabMovesTheKeyboardOnlyAfterTheFieldDeclinesIt(t *testing.T) {
	greedy, plain := &field{name: "greedy", takes: true}, &field{name: "plain"}
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(1), Of: greedy},
		headless.Item{Size: layout.Fixed(1), Of: plain},
	)

	// A field that means something by tab keeps it.
	c.Handle(input.Key{Code: input.Tab})
	if c.Focused() != headless.Widget(greedy) {
		t.Fatal("tab moved the keyboard away from a field that consumed it")
	}

	greedy.takes = false
	c.Handle(input.Key{Code: input.Tab})
	if c.Focused() != headless.Widget(plain) {
		t.Fatal("tab did not move the keyboard on when the field declined it")
	}
	c.Handle(input.Key{Code: input.Tab, Mods: input.Shift})
	if c.Focused() != headless.Widget(greedy) {
		t.Fatal("shift+tab did not move the keyboard back")
	}
}

// TestAKeyNobodyWantedIsDeclined, so a container inside a container hands back what
// it cannot use instead of swallowing it.
func TestAKeyNobodyWantedIsDeclined(t *testing.T) {
	inner := &field{name: "inner"}
	outer := headless.Rows(headless.Item{Of: inner})
	if outer.Handle(input.Key{Code: input.Character, Rune: 'x'}) {
		t.Fatal("a key nobody wanted was reported as handled")
	}
}

func TestAPointerEventGoesToWhateverItIsOverInThatChildsOwnCoordinates(t *testing.T) {
	top, bottom := &field{name: "top", takes: true}, &field{name: "bottom", takes: true}
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(2), Of: top},
		headless.Item{Size: layout.Fixed(2), Of: bottom},
	)
	drawn(c, 4)

	if !c.Handle(pressAt(3, 3)) {
		t.Fatal("a press inside the container was not taken")
	}
	if len(top.mice) != 0 {
		t.Errorf("the press reached the wrong child")
	}
	// Row 3 of the container is row 1 of a child that starts at row 2. A widget
	// reasons in its own coordinates, exactly as it draws in them.
	if got := bottom.lastMouse(t).Pos; got != image.Pt(3, 1) {
		t.Errorf("the press arrived at %v, want it translated into the child's own box", got)
	}
}

// TestAPressMovesTheKeyboardToWhatWasPressed, whether or not the press itself was
// wanted: clicking a pane is how a user says they mean that one.
func TestAPressMovesTheKeyboardToWhatWasPressed(t *testing.T) {
	top, bottom := &field{name: "top"}, &field{name: "bottom"}
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(2), Of: top},
		headless.Item{Size: layout.Fixed(2), Of: bottom},
	)
	drawn(c, 4)

	c.Handle(pressAt(0, 3))
	if c.Focused() != headless.Widget(bottom) {
		t.Fatal("pressing a pane did not give it the keyboard")
	}
	if !bottom.focused || top.focused {
		t.Errorf("focus is top=%v bottom=%v", top.focused, bottom.focused)
	}
}

// TestADragStaysWithWhateverTookThePress. Routing by position alone would hand the
// rest of the gesture to whatever the pointer wandered over, which is what makes a
// selection stop extending the moment it leaves the pane it started in.
func TestADragStaysWithWhateverTookThePress(t *testing.T) {
	top, bottom := &field{name: "top", takes: true}, &field{name: "bottom", takes: true}
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(2), Of: top},
		headless.Item{Size: layout.Fixed(2), Of: bottom},
	)
	drawn(c, 4)

	c.Handle(pressAt(0, 0))
	c.Handle(input.Mouse{Pos: image.Pt(0, 3), Action: input.MouseDrag})
	c.Handle(input.Mouse{Pos: image.Pt(0, 3), Action: input.MouseUp})
	if len(bottom.mice) != 0 {
		t.Fatalf("the child the pointer wandered over got %d events", len(bottom.mice))
	}
	if len(top.mice) != 3 {
		t.Fatalf("the child that took the press got %d events, want all three", len(top.mice))
	}
	// And the position is still in that child's coordinates, even though the pointer
	// is no longer inside it.
	if got := top.mice[1].Pos; got != image.Pt(0, 3) {
		t.Errorf("the drag arrived at %v", got)
	}

	// The release ends the capture: the next press goes wherever it lands.
	c.Handle(pressAt(0, 3))
	if len(bottom.mice) != 1 {
		t.Errorf("after the release the next press did not go where it landed")
	}
}

func TestANewPressEndsAnIncompleteContainerGesture(t *testing.T) {
	first := &field{name: "first", takes: true}
	second := &field{name: "second"}
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(2), Of: first},
		headless.Item{Size: layout.Fixed(2), Of: second},
	)
	drawn(c, 4)

	c.Handle(pressAt(0, 0))
	if c.Handle(pressAt(0, 3)) {
		t.Fatal("the second child accepted the replacement press")
	}
	// Outside every child: only stale capture could send this back to the first.
	c.Handle(input.Mouse{Pos: image.Pt(0, 8), Action: input.MouseDrag})
	if got := len(first.mice); got != 1 {
		t.Fatalf("the old gesture owner received %d events after a new press, want one", got)
	}
}

func TestRemovingThePresentedOwnerEndsContainerCapture(t *testing.T) {
	child := &field{name: "child", takes: true}
	c := headless.Rows(headless.Item{Size: layout.Fixed(2), Of: child})
	drawn(c, 2)
	c.Handle(pressAt(0, 0))

	c.Set()
	drawn(c, 2)
	c.Handle(input.Mouse{Pos: image.Pt(0, 8), Action: input.MouseDrag})
	c.Set(headless.Item{Size: layout.Fixed(2), Of: child})
	drawn(c, 2)
	c.Handle(input.Mouse{Pos: image.Pt(0, 8), Action: input.MouseDrag})
	if got := len(child.mice); got != 1 {
		t.Fatalf("a removed and reinserted child resumed an old gesture with %d events", got)
	}
}

// TestAPointerEventOutsideEveryChildIsDeclined, because a container that claimed the
// whole region would stop anything above it from ever seeing the pointer.
func TestAPointerEventOutsideEveryChildIsDeclined(t *testing.T) {
	only := &field{name: "only", takes: true}
	c := headless.Rows(headless.Item{Size: layout.Fixed(1), Of: only})
	drawn(c, 4)

	if c.Handle(pressAt(0, 3)) {
		t.Fatal("a press below every child was taken anyway")
	}
}

// TestTheKeyboardIsHeldByIdentityAndNotByPosition, so a container whose items are
// rebuilt between frames — a status row that comes and goes — does not silently move
// the keyboard to whatever took the old index.
func TestTheKeyboardIsHeldByIdentityAndNotByPosition(t *testing.T) {
	status, composer := &field{name: "status"}, &field{name: "composer"}
	c := headless.Rows(headless.Item{Size: layout.Fixed(1), Of: composer})
	drawn(c, 2)

	c.Set(
		headless.Item{Size: layout.Fixed(1), Of: status},
		headless.Item{Size: layout.Fixed(1), Of: composer},
	)
	drawn(c, 2)
	if c.Focused() != headless.Widget(composer) {
		t.Fatal("inserting a child above moved the keyboard")
	}

	// And a child that is taken away gives it up rather than holding it from
	// nowhere.
	c.Set(headless.Item{Size: layout.Fixed(1), Of: status})
	drawn(c, 2)
	if c.Focused() != headless.Widget(status) {
		t.Fatal("the keyboard stayed with a child that is no longer there")
	}
	if composer.focused {
		t.Error("a child that was taken away still believes it has the keyboard")
	}
}

func TestContainerOwnsItsChildListAndReturnsSnapshots(t *testing.T) {
	one, two := &field{name: "one"}, &field{name: "two"}
	items := []headless.Item{{Size: layout.Fixed(1), Of: one}}
	container := headless.Rows(items...)
	items[0].Of = two
	if got := container.Items()[0].Of; got != headless.Widget(one) {
		t.Fatal("caller mutation replaced the container's child")
	}

	snapshot := container.Items()
	snapshot[0].Of = two
	if got := container.Items()[0].Of; got != headless.Widget(one) {
		t.Fatal("snapshot mutation replaced the container's child")
	}
}

// TestAContainerInsideAContainerPassesTheAnswerDown. A container is a widget, which
// is what makes more than one row of panes possible at all — and what makes it
// possible to get two cursors if the news does not travel.
func TestAContainerInsideAContainerPassesTheAnswerDown(t *testing.T) {
	left, right := &field{name: "left"}, &field{name: "right"}
	inner := headless.Rows(headless.Item{Size: layout.Fixed(1), Of: right})
	outer := headless.Columns(
		headless.Item{Size: layout.Fixed(5), Of: left},
		headless.Item{Size: layout.Fixed(5), Of: inner},
	)
	drawn(outer, 1)

	if !left.focused {
		t.Fatal("the outer container did not give the keyboard to its first child")
	}
	if right.focused {
		t.Fatal("a child of an unfocused container believes it has the keyboard")
	}

	outer.Give(inner)
	if !right.focused {
		t.Fatal("giving the inner container the keyboard did not reach the widget in it")
	}
	if left.focused {
		t.Error("the first child kept the keyboard")
	}
}

// TestALoneWidgetKeepsTheKeyboardWithNobodyToGiveIt. Most interfaces start as one
// field and nothing else, and one that drew no cursor until it was told it could
// would look broken from the first frame.
func TestALoneWidgetKeepsTheKeyboardWithNobodyToGiveIt(t *testing.T) {
	var editor headless.Editor
	editor.SetText("hi")
	s := grid.NewScreen(10, 1)
	headless.NewRoot(&editor).Draw(s.Frame())
	if !s.Cursor().Visible {
		t.Fatal("a field that is the whole interface drew no cursor")
	}

	editor.Focus(false)
	s.Frame()
	headless.NewRoot(&editor).Draw(s.Frame())
	if s.Cursor().Visible {
		t.Fatal("a field told it does not have the keyboard drew a cursor anyway")
	}
}

// TestOnlyTheFocusedFieldPlacesTheCursor is the bug the whole arrangement is for. A
// frame has one cursor, so two fields both asking for it is not two cursors: it is
// one, wherever the last of them happened to draw.
func TestOnlyTheFocusedFieldPlacesTheCursor(t *testing.T) {
	first, second := &headless.Editor{}, &headless.Editor{}
	first.SetText("one")
	second.SetText("two")
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(1), Of: first},
		headless.Item{Size: layout.Fixed(1), Of: second},
	)

	s := grid.NewScreen(10, 2)
	root := headless.NewRoot(c)
	root.Draw(s.Frame())
	if got := s.Cursor(); !got.Visible || got.Pos.Y != 0 {
		t.Fatalf("the cursor is %+v, want it in the first field", got)
	}

	c.Give(second)
	root.Draw(s.Frame())
	if got := s.Cursor(); !got.Visible || got.Pos.Y != 1 {
		t.Fatalf("the cursor is %+v, want it in the field with the keyboard", got)
	}
}

// layer is a modal that records where it stands, for the stack's half of the same
// question.
type layer struct {
	field
	where layout.Placement
}

func (l *layer) Place(image.Point) layout.Placement { return l.where }

// TestALayerTakesTheKeyboardFromWhatItCovers, and gives it back. Without it the
// interface underneath goes on drawing a cursor into the frame's one cursor, under a
// dialog that has one of its own.
func TestALayerTakesTheKeyboardFromWhatItCovers(t *testing.T) {
	base := &field{name: "base"}
	over := &layer{field: field{name: "over"}, where: layout.Placement{Width: 4, Height: 1}}
	s := &headless.Stack{Base: base}
	s.Focus(true)

	headless.NewRoot(s).Draw(grid.NewSurface(10, 4).View())
	if !base.focused {
		t.Fatal("the interface under an empty stack does not have the keyboard")
	}

	s.Push(over)
	if base.focused {
		t.Error("the interface under a layer still believes it is being typed into")
	}
	if !over.focused {
		t.Error("the layer that was pushed was not given the keyboard")
	}

	s.Pop()
	if !base.focused {
		t.Error("the keyboard did not come back when the layer closed")
	}
	if over.focused {
		t.Error("a layer that was popped still believes it has the keyboard")
	}
}

// TestAContainerAsksItsChildrenHowBigTheyWant, so a column of widgets can itself go
// in a slot that grows to fit what is in it.
func TestAContainerAsksItsChildrenHowBigTheyWant(t *testing.T) {
	var editor headless.Editor
	editor.SetText("one\ntwo\nthree")
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(1), Of: &field{name: "header"}},
		headless.Item{Size: layout.Measured(0, 0), Of: &editor},
	)
	if got := c.Measure(20); got != 4 {
		t.Errorf("the container wants %d rows, want the header and three lines", got)
	}
}

// TestASlotOfNoRowsGetsNoRows. A sizing means in a container exactly what it means
// in a layout, zero value included — because [layout.Fixed] of nothing is that same
// zero value, and a child that grew back to its natural size when a caller asked for
// none of it would appear on exactly the frames it was meant to be absent from.
func TestASlotOfNoRowsGetsNoRows(t *testing.T) {
	status, body := &field{name: "status"}, &field{name: "body"}
	c := headless.Rows(
		headless.Item{Size: layout.Fixed(0), Of: status},
		headless.Item{Size: layout.Flex(1), Of: body},
	)
	s := grid.NewSurface(10, 3)
	headless.NewRoot(c).Draw(s.View())

	if got := status.area.Dy(); got != 0 {
		t.Errorf("a slot asked for no rows got %d", got)
	}
	if got := body.area.Dy(); got != 3 {
		t.Errorf("the rest of the region went to %d rows, want all three", got)
	}
}
