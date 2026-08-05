package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// selected reads back which cells of a surface carry a style, which is how a test
// says what the user can see.
func selected(v grid.View, w, h int, style grid.Style) [][2]int {
	var out [][2]int
	for y := range h {
		for x := range w {
			if c := v.CellAt(x, y); c != nil && c.Style == style {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

// TestASelectionCanBeSeen is the hole this closes. The behaviour worked, the copy was
// right, and the screen showed nothing.
func TestASelectionCanBeSeen(t *testing.T) {
	mark := grid.Style{Attr: grid.Reverse}
	e := editorWith("hello world")
	e.SelectionStyle = mark
	for range 5 {
		e.Handle(shift(input.Right))
	}

	s := grid.NewSurface(20, 2)
	e.Draw(s.View())

	got := selected(s.View(), 20, 2, mark)
	if len(got) != 5 {
		t.Fatalf("%d cells are marked, want the five selected: %v", len(got), got)
	}
	for i, cell := range got {
		if cell != [2]int{i, 0} {
			t.Errorf("marked cell %d is %v, want {%d 0}", i, cell, i)
		}
	}
}

func TestNoSelectionMarksNothing(t *testing.T) {
	mark := grid.Style{Attr: grid.Reverse}
	e := editorWith("hello world")
	e.SelectionStyle = mark

	s := grid.NewSurface(20, 2)
	e.Draw(s.View())
	if got := selected(s.View(), 20, 2, mark); len(got) != 0 {
		t.Errorf("%v is marked with nothing selected", got)
	}
}

func TestAFieldWithoutASelectionStyleDrawsNoSelection(t *testing.T) {
	// The zero value draws none, which is what this had before there was one.
	e := editorWith("hello world")
	e.SelectAll()
	s := grid.NewSurface(20, 2)
	e.Draw(s.View())
	if got := selected(s.View(), 20, 2, grid.Style{Attr: grid.Reverse}); len(got) != 0 {
		t.Errorf("%v is marked without being asked", got)
	}
}

// TestSpansFollowTheWrap. A range in a wrapped field is not a rectangle: it starts
// part way along a row, covers whole rows, and ends part way along another — and
// where the rows begin is decided by the width.
func TestSpansFollowTheWrap(t *testing.T) {
	const line = "the quick brown fox"
	e := editorWith(line)
	// At width 10 the first row is "the quick " — the break takes the space with the
	// row it ends, so the row is ten columns wide and the second begins at "brown".
	const secondRow = len("the quick ")
	spans := e.Spans(headless.Caret{Line: 0, Col: 4}, headless.Caret{Line: 0, Col: 15}, 10)
	if len(spans) != 2 {
		t.Fatalf("the range covers %d rows, want 2: %+v", len(spans), spans)
	}
	// From "q" to the end of the first row.
	if want := (headless.RowSpan{Row: 0, Col: 4, Width: secondRow - 4}); spans[0] != want {
		t.Errorf("the first row's span is %+v, want %+v", spans[0], want)
	}
	// And from the start of the second to where the range ends.
	if want := (headless.RowSpan{Row: 1, Col: 0, Width: 15 - secondRow}); spans[1] != want {
		t.Errorf("the second row's span is %+v, want %+v", spans[1], want)
	}
}

func TestSpansAcrossLines(t *testing.T) {
	e := editorWith("first\nsecond\nthird")
	spans := e.Spans(headless.Caret{Line: 0, Col: 2}, headless.Caret{Line: 2, Col: 3}, 20)
	if len(spans) != 3 {
		t.Fatalf("got %+v, want three rows", spans)
	}
	if spans[0].Col != 2 || spans[0].Width != 3 {
		t.Errorf("the first row's span is %+v, want columns 2..5", spans[0])
	}
	if spans[2].Col != 0 || spans[2].Width != 3 {
		t.Errorf("the last row's span is %+v, want columns 0..3", spans[2])
	}
}

// TestSpansAreInColumnsAndNotBytes, because a wide character occupies two columns and
// a highlight is painted in columns.
func TestSpansAreInColumnsAndNotBytes(t *testing.T) {
	e := editorWith("中文ab")
	spans := e.Spans(headless.Caret{Line: 0, Col: 0}, headless.Caret{Line: 0, Col: len("中文")}, 20)
	if len(spans) != 1 {
		t.Fatalf("got %+v", spans)
	}
	if spans[0].Width != 4 {
		t.Errorf("two wide characters cover %d columns, want 4", spans[0].Width)
	}
}

func TestSpansOfABackwardsRangeAreTheSameRange(t *testing.T) {
	e := editorWith("hello")
	forward := e.Spans(headless.Caret{Line: 0, Col: 1}, headless.Caret{Line: 0, Col: 4}, 20)
	back := e.Spans(headless.Caret{Line: 0, Col: 4}, headless.Caret{Line: 0, Col: 1}, 20)
	if len(forward) != 1 || len(back) != 1 || forward[0] != back[0] {
		t.Errorf("forward %+v, backwards %+v", forward, back)
	}
}

func TestSpansOfNothing(t *testing.T) {
	e := editorWith("hello")
	for _, tc := range []struct {
		name     string
		from, to headless.Caret
		width    int
	}{
		{name: "no width", from: headless.Caret{}, to: headless.Caret{Line: 0, Col: 3}, width: 0},
		{name: "an empty range", from: headless.Caret{Line: 0, Col: 2}, to: headless.Caret{Line: 0, Col: 2}, width: 20},
	} {
		if got := e.Spans(tc.from, tc.to, tc.width); len(got) != 0 {
			t.Errorf("%s covered %+v", tc.name, got)
		}
	}
	if got := e.SelectionSpans(20); len(got) != 0 {
		t.Errorf("a field with nothing selected covered %+v", got)
	}
}

// TestTheSelectionAndTheCursorAgree is why the spans are read from the same rows the
// cursor is placed from. Two wraps disagree about where the text is exactly when the
// text is interesting.
func TestTheSelectionAndTheCursorAgree(t *testing.T) {
	mark := grid.Style{Attr: grid.Reverse}
	e := editorWith("the quick brown fox jumps over")
	e.SelectionStyle = mark
	e.SetCursor(0, 0)
	for range 25 {
		e.Handle(shift(input.Right))
	}

	// A screen rather than a bare surface, because the cursor is only observable
	// through one: that is the object that carries it to the terminal.
	screen := grid.NewScreen(10, 6)
	frame := screen.Frame()
	e.Draw(frame)

	// The cursor is where the selection ends, so the cell it sits on must be the one
	// after the last marked cell on its row.
	cursor := screen.Cursor()
	if !cursor.Visible {
		t.Fatal("no cursor was placed")
	}
	marked := selected(frame, 10, 6, mark)
	if len(marked) == 0 {
		t.Fatal("nothing was marked")
	}
	last := marked[len(marked)-1]
	if last[1] != cursor.Pos.Y {
		t.Errorf("the selection ends on row %d and the cursor is on row %d", last[1], cursor.Pos.Y)
	}
	if last[0]+1 != cursor.Pos.X {
		t.Errorf("the selection ends at column %d and the cursor is at %d", last[0], cursor.Pos.X)
	}
}

func click(x, y int) input.Mouse {
	return input.Mouse{Pos: image.Pt(x, y), Action: input.MouseDown, Button: input.ButtonLeft}
}

// drag is a pointer moving with the button held, on the first row.
func drag(x int) input.Mouse {
	return input.Mouse{Pos: image.Pt(x, 0), Action: input.MouseDrag, Button: input.ButtonLeft}
}

// TestAFieldCanBeClickedInto. Until this, a text field could be typed into and not
// clicked into, which is the first thing anybody tries.
func TestAFieldCanBeClickedInto(t *testing.T) {
	e := editorWith("hello world")
	if !e.HandleMouse(click(6, 0), 20) {
		t.Fatal("the field ignored a click")
	}
	if line, col := e.Cursor(); line != 0 || col != 6 {
		t.Errorf("the cursor is at %d:%d, want 0:6", line, col)
	}
}

func TestClickingLandsOnAClusterBoundary(t *testing.T) {
	// Column three is the second half of the second wide character, and half of one is
	// not a place a cursor can be.
	e := editorWith("中文ab")
	e.HandleMouse(click(3, 0), 20)
	_, col := e.Cursor()
	if col != len("中") {
		t.Errorf("the cursor is at byte %d, want the boundary at %d", col, len("中"))
	}
}

func TestClickingAWrappedRow(t *testing.T) {
	e := editorWith("the quick brown fox jumps")
	// At width 10 the second row begins at "brown".
	e.HandleMouse(click(0, 1), 10)
	_, col := e.Cursor()
	if got := e.Text()[col:]; got != "brown fox jumps" {
		t.Errorf("the cursor landed before %q", got)
	}
}

func TestClickingBelowTheTextMeansTheEnd(t *testing.T) {
	// What a click past the last line means in every editor there is.
	e := editorWith("one\ntwo")
	e.HandleMouse(click(0, 8), 20)
	line, col := e.Cursor()
	if line != 1 || col != len("two") {
		t.Errorf("the cursor is at %d:%d, want the end", line, col)
	}
}

func TestDraggingSelects(t *testing.T) {
	e := editorWith("hello world")
	e.HandleMouse(click(0, 0), 20)
	e.HandleMouse(drag(5), 20)
	if got := e.Selected(); got != "hello" {
		t.Errorf("dragging selected %q, want %q", got, "hello")
	}
	e.HandleMouse(input.Mouse{Pos: image.Pt(5, 0), Action: input.MouseUp}, 20)
	if got := e.Selected(); got != "hello" {
		t.Errorf("the selection did not survive the button coming up: %q", got)
	}
}

// TestAClickThatDidNotMoveSelectsNothing, because an anchor still on the cursor is a
// click and not a selection.
func TestAClickThatDidNotMoveSelectsNothing(t *testing.T) {
	e := editorWith("hello world")
	e.HandleMouse(click(3, 0), 20)
	e.HandleMouse(input.Mouse{Pos: image.Pt(3, 0), Action: input.MouseUp}, 20)
	if got := e.Selected(); got != "" {
		t.Errorf("a click selected %q", got)
	}
}

func TestAClickReplacesTheSelectionBefore(t *testing.T) {
	e := editorWith("hello world")
	e.SelectAll()
	e.HandleMouse(click(2, 0), 20)
	if got := e.Selected(); got != "" {
		t.Errorf("the old selection survived a click: %q", got)
	}
}

func TestTheFieldIgnoresWhatIsNotItsToAnswer(t *testing.T) {
	e := editorWith("hello")
	for _, ev := range []input.Mouse{
		{Pos: image.Pt(1, 0), Action: input.MouseDown, Button: input.ButtonRight},
		{Pos: image.Pt(1, 0), Action: input.MouseMove},
		{Pos: image.Pt(1, 0), Action: input.WheelDown},
		// A drag with nothing started by a press of its own.
		drag(3),
	} {
		if e.HandleMouse(ev, 20) {
			t.Errorf("the field consumed %+v", ev)
		}
	}
	if e.HandleMouse(click(1, 0), 0) {
		t.Error("the field answered a click in a box of no width")
	}
	if e.HandleMouse(click(-1, -1), 20) {
		t.Error("the field answered a click outside it")
	}
}

// TestClickingStepsOverAnElement, the same rule moving does: a chip is one thing, and
// a cursor inside it has no position a reader could account for.
func TestClickingStepsOverAnElement(t *testing.T) {
	e := editorWith("")
	e.InsertElement(fileChip, "@main.go")
	e.HandleMouse(click(3, 0), 40)
	_, col := e.Cursor()
	if col != len("@main.go") {
		t.Errorf("a click inside the element put the cursor at %d, want past it", col)
	}
}

// TestClickingAgreesWithWhereTheTextWasDrawn is the property all three walks share.
// A click, a cursor and a selection read the same rows, or they disagree exactly when
// the text is interesting.
func TestClickingAgreesWithWhereTheTextWasDrawn(t *testing.T) {
	const width = 12
	e := editorWith("中文 mixed 文字 with wide ones")
	screen := grid.NewScreen(width, 8)

	// Only the rows the text actually has: below them a click means the end, which is
	// a different promise and has its own test.
	for y := range e.Measure(width) {
		for x := range width {
			e.HandleMouse(click(x, y), width)
			frame := screen.Frame()
			e.Draw(frame)
			cursor := screen.Cursor()
			if !cursor.Visible {
				continue
			}
			// The cursor lands on the row that was clicked and no further right than
			// the click — or at the start of the row below, which is the same
			// position in the text: where a row was broken by the width, the offset
			// after its last character and the offset before the next row's first
			// are one offset with two places on screen, and this editor draws it in
			// the second. A click past the end of a wrapped row therefore shows the
			// caret at the start of the next.
			onward := cursor.Pos.Y == y+1 && cursor.Pos.X == 0
			if onward {
				continue
			}
			if cursor.Pos.Y != y {
				t.Fatalf("a click at (%d,%d) put the cursor at (%d,%d)", x, y, cursor.Pos.X, cursor.Pos.Y)
			}
			if cursor.Pos.X > x {
				t.Fatalf("a click at (%d,%d) put the cursor at column %d", x, y, cursor.Pos.X)
			}
		}
	}
}

func TestClickingAnEmptyField(t *testing.T) {
	// An empty field still has a row, because a blank line in a composer is a blank
	// line on screen. A click anywhere in it lands on the only position there is.
	e := headless.NewEditor()
	at, ok := e.At(4, 5, 20)
	if !ok {
		t.Fatal("a click in an empty field found nothing")
	}
	if at != (headless.Caret{}) {
		t.Errorf("it landed at %+v, want the start", at)
	}
}

func TestSpansOfARangeOutsideTheText(t *testing.T) {
	e := editorWith("one\ntwo")
	// A range whose lines are not there covers nothing rather than indexing past the
	// end.
	if got := e.Spans(headless.Caret{Line: 5}, headless.Caret{Line: 9}, 20); len(got) != 0 {
		t.Errorf("covered %+v", got)
	}
}

func TestDraggingUpwardsSelects(t *testing.T) {
	// The other direction, which exercises the ordering the anchor is kept for.
	e := editorWith("hello world")
	e.HandleMouse(click(11, 0), 20)
	e.HandleMouse(drag(6), 20)
	if got := e.Selected(); got != "world" {
		t.Errorf("dragging back selected %q, want %q", got, "world")
	}
}

func TestDraggingOffTheFieldChangesNothing(t *testing.T) {
	// A drag that leaves the box entirely has no position to move to, and moving the
	// far end to a guess would select something nobody dragged over.
	e := editorWith("hello world")
	e.HandleMouse(click(0, 0), 20)
	e.HandleMouse(drag(2), 20)
	before := e.Selected()
	if e.HandleMouse(drag(-5), 20) {
		t.Error("a drag outside the field was consumed")
	}
	if got := e.Selected(); got != before {
		t.Errorf("the selection became %q, want it left at %q", got, before)
	}
}
