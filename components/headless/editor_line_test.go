package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestAFieldThatHoldsOneLineNeverGetsASecond(t *testing.T) {
	// Every way text arrives goes through the same rule, because a rule kept in four
	// places out of five is a rule with a way round it. A break becomes a space: it is
	// what the text meant, and dropping it would join two words.
	e := &headless.Editor{SingleLine: true}
	e.SetText("one\ntwo")
	if got := e.Text(); got != "one two" {
		t.Fatalf("after SetText = %q", got)
	}
	e.Handle(input.Paste{Text: "\nthree\nfour"})
	if got := e.Text(); got != "one two three four" {
		t.Fatalf("after a paste = %q", got)
	}
	e.Newline()
	if got := e.Text(); got != "one two three four" {
		t.Fatalf("after Newline = %q", got)
	}
	if got := e.Measure(4); got != 1 {
		t.Fatalf("measured %d rows", got)
	}
}

func TestAFieldThatHoldsOneLineSlidesSidewaysToKeepTheCursorInView(t *testing.T) {
	e := &headless.Editor{SingleLine: true}
	e.SetText("abcdefghij")

	// The cursor is at the end, so the window shows the end of the text with room for
	// the cursor to sit past the last character.
	rows := paint(5, 1, e.Draw)
	equalRows(t, rows, []string{"ghij."})

	e.Do(headless.MoveLineStart)
	rows = paint(5, 1, e.Draw)
	equalRows(t, rows, []string{"abcde"})
}

func TestAFieldThatHoldsOneLineComesBackWhenItsTextGetsShorter(t *testing.T) {
	// Otherwise a deletion leaves the field showing its end with blank columns to the
	// right of text that would have fitted.
	e := &headless.Editor{SingleLine: true}
	e.SetText("abcdefghij")
	paint(5, 1, e.Draw)
	for range 8 {
		e.DeleteBack()
	}
	rows := paint(5, 1, e.Draw)
	equalRows(t, rows, []string{"ab..."})
}

func TestAMaskedFieldShowsNothingItHolds(t *testing.T) {
	e := &headless.Editor{Mask: "•"}
	e.SetText("secret")
	rows := paint(8, 1, e.Draw)
	equalRows(t, rows, []string{"••••••.."})
	if got := e.Text(); got != "secret" {
		t.Fatalf("the field itself = %q, want what was typed", got)
	}
}

func TestAMaskedFieldPutsTheCursorWhereTheMaskIs(t *testing.T) {
	// The mask is one cluster per cluster and a different count of bytes, so every
	// position has to be translated. A cursor placed from the text's own offsets would
	// sit wherever the two happened to agree.
	e := &headless.Editor{Mask: "••"}
	e.SetText("ab")
	e.SetCursor(0, 1)

	// A screen rather than a bare surface, because the cursor is only observable
	// through one.
	screen := grid.NewScreen(8, 1)
	e.Draw(screen.Frame())
	if cursor := screen.Cursor(); !cursor.Visible || cursor.Pos.X != 2 {
		t.Fatalf("cursor at %+v, want it two masked columns along", cursor)
	}
}

func TestAClickInAMaskedFieldLandsOnTheClusterItIsUnder(t *testing.T) {
	e := &headless.Editor{Mask: "••"}
	e.SetText("abc")
	paint(10, 1, e.Draw)

	e.HandleMouse(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(4, 0)}, 10)
	if _, col := e.Cursor(); col != 2 {
		t.Fatalf("cursor at byte %d, want the third cluster", col)
	}
}

func TestAMaskedFieldHoldsOneLineWhetherOrNotItWasAsked(t *testing.T) {
	// A secret is one value, and where to break a line nobody can read is not a
	// question worth an answer.
	e := &headless.Editor{Mask: "*"}
	e.SetText("a\nb")
	if got := e.Text(); got != "a b" {
		t.Fatalf("= %q", got)
	}
}
