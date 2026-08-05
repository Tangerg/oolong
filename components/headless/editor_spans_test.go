package headless_test

import (
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
