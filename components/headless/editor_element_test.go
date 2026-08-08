package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
)

const fileChip headless.ElementKind = 1

func TestElementIsInsertedWithASeparator(t *testing.T) {
	e := editorWith("")
	el := e.InsertElement(fileChip, "@main.go")

	if got, want := e.Text(), "@main.go "; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := el.Text(e); got != "@main.go" {
		t.Errorf("the element's own text is %q", got)
	}
	// The separator is ordinary text and not part of the element: it is there to be
	// typed after and to be deleted.
	if _, inside := e.ElementAt(0, len("@main.go")); inside {
		t.Error("the separator is inside the element")
	}
}

// TestTheCursorStepsOverAnElement. It is one thing on screen, and a cursor inside it
// has no position a reader could account for.
func TestTheCursorStepsOverAnElement(t *testing.T) {
	e := editorWith("")
	e.InsertElement(fileChip, "@main.go")
	e.Insert("and more")
	e.SetCursor(0, 0)

	e.Handle(input.Key{Code: input.Right})
	if _, col := e.Cursor(); col != len("@main.go") {
		t.Errorf("moving right landed at column %d, want past the element", col)
	}
	e.Handle(input.Key{Code: input.Left})
	if _, col := e.Cursor(); col != 0 {
		t.Errorf("moving back landed at column %d, want the near side", col)
	}
}

// TestBackspaceTakesAWholeElement, because a fragment of one still looks like the
// thing and no longer is.
func TestBackspaceTakesAWholeElement(t *testing.T) {
	e := editorWith("")
	e.InsertElement(fileChip, "@main.go")
	// Past the separator, then back over it and over the element.
	e.Handle(input.Key{Code: input.Backspace})
	e.Handle(input.Key{Code: input.Backspace})

	if got := e.Text(); got != "" {
		t.Errorf("text = %q, want the element gone whole", got)
	}
	if got := e.Elements(); len(got) != 0 {
		t.Errorf("the editor still holds %+v", got)
	}
}

func TestDeleteForwardTakesAWholeElement(t *testing.T) {
	e := editorWith("")
	e.InsertElement(fileChip, "@main.go")
	e.SetCursor(0, 0)
	e.Handle(input.Key{Code: input.Delete})

	if got, want := e.Text(), " "; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := e.Elements(); len(got) != 0 {
		t.Errorf("the editor still holds %+v", got)
	}
}

// TestEditingAroundAnElementMovesIt. Its identity is what a program keys its own
// record by, so it has to survive the text around it changing.
func TestEditingAroundAnElementMovesIt(t *testing.T) {
	e := editorWith("")
	el := e.InsertElement(fileChip, "@main.go")
	e.SetCursor(0, 0)
	e.Insert("look at ")

	got := e.Elements()
	if len(got) != 1 {
		t.Fatalf("the editor holds %+v, want the one element", got)
	}
	if got[0].ID != el.ID {
		t.Errorf("the element came back with identity %d, want %d", got[0].ID, el.ID)
	}
	if got[0].Start != len("look at ") {
		t.Errorf("the element is at %d, want it moved along by the text before it", got[0].Start)
	}
	if text := got[0].Text(e); text != "@main.go" {
		t.Errorf("the element now covers %q", text)
	}
}

// TestAnElementCoversTheSameWordsAfterEveryKindOfEdit. There is one rule for moving
// a mark over a change — see [text.Shift] — and the way to know the editor is using
// it once and correctly is to state the outcome as "the same words", which no amount
// of arithmetic on line and column numbers can be quietly wrong about.
func TestAnElementCoversTheSameWordsAfterEveryKindOfEdit(t *testing.T) {
	for _, tc := range []struct {
		name string
		does func(e *headless.Editor)
	}{
		{"typing in front of it", func(e *headless.Editor) {
			e.SetCursor(0, 0)
			e.Insert("see ")
		}},
		{"a backspace in front of it", func(e *headless.Editor) {
			e.SetCursor(0, 4)
			e.DeleteBack()
		}},
		{"a whole paragraph pasted above", func(e *headless.Editor) {
			e.SetCursor(0, 0)
			e.Insert("one\ntwo\nthree\n")
		}},
		{"the line below joined onto it", func(e *headless.Editor) {
			e.SetCursor(0, 0)
			e.MoveLineEnd()
			e.DeleteForward()
		}},
		{"typing after it", func(e *headless.Editor) {
			e.SetCursor(0, 0)
			e.MoveLineEnd()
			e.Insert(" please")
		}},
	} {
		e := editorWith("look ")
		e.SetCursor(0, len("look "))
		el := e.InsertElement(fileChip, "@main.go")
		e.Insert("\ntail")
		tc.does(e)

		got := e.Elements()
		if len(got) != 1 {
			t.Errorf("%s: the editor holds %+v, want the one element", tc.name, got)
			continue
		}
		if got[0].ID != el.ID {
			t.Errorf("%s: the element came back as %d, want %d", tc.name, got[0].ID, el.ID)
		}
		if body := got[0].Text(e); body != "@main.go" {
			t.Errorf("%s: the element now covers %q", tc.name, body)
		}
	}
}

func TestElementsSurviveALineBreakAboveThem(t *testing.T) {
	e := editorWith("")
	el := e.InsertElement(fileChip, "@main.go")
	e.SetCursor(0, 0)
	e.Insert("first\n")

	got := e.Elements()
	if len(got) != 1 {
		t.Fatalf("the editor holds %+v", got)
	}
	if got[0].Line != 1 || got[0].Start != 0 {
		t.Errorf("the element is at line %d column %d, want 1:0", got[0].Line, got[0].Start)
	}
	if got[0].ID != el.ID {
		t.Errorf("identity changed to %d", got[0].ID)
	}
}

// TestTypingIntoAnElementDropsIt. Text typed into the middle of one names something
// else, and a chip that still looks like a file and points at half of one is worse
// than no chip.
func TestTypingIntoAnElementDropsIt(t *testing.T) {
	e := editorWith("")
	e.InsertElement(fileChip, "@main.go")
	// SetCursor snaps out of an element, so the text goes in through the API that
	// does not.
	e.Replace(3, 3, "XX")

	if got := e.Elements(); len(got) != 0 {
		t.Errorf("the editor still holds %+v after being typed into", got)
	}
	if got, want := e.Text(), "@maXXin.go "; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestRemoveElement(t *testing.T) {
	e := editorWith("")
	first := e.InsertElement(fileChip, "@one.go")
	e.InsertElement(fileChip, "@two.go")

	if !e.RemoveElement(first.ID) {
		t.Fatal("removing an element that is there reported false")
	}
	if got, want := e.Text(), "@two.go "; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := e.Elements(); len(got) != 1 {
		t.Fatalf("the editor holds %+v, want one", got)
	}
	if e.RemoveElement(9999) {
		t.Error("removing an element that is not there reported true")
	}
}

// TestUndoGivesBackTheSameElement, not one that looks like it. Two chips with the
// same words are two different things, and a program keying its own record by
// identity would be given back the wrong record.
func TestUndoGivesBackTheSameElement(t *testing.T) {
	e := editorWith("")
	el := e.InsertElement(fileChip, "@main.go")
	e.RemoveElement(el.ID)
	if got := e.Elements(); len(got) != 0 {
		t.Fatalf("the element survived being removed: %+v", got)
	}

	e.Undo()
	got := e.Elements()
	if len(got) != 1 {
		t.Fatalf("undo gave back %+v, want the element", got)
	}
	if got[0].ID != el.ID {
		t.Errorf("undo gave back identity %d, want %d", got[0].ID, el.ID)
	}
}

func TestElementsAreReportedInOrder(t *testing.T) {
	e := editorWith("")
	e.InsertElement(fileChip, "@b.go")
	e.SetCursor(0, 0)
	e.InsertElement(fileChip, "@a.go")

	got := e.Elements()
	if len(got) != 2 {
		t.Fatalf("the editor holds %+v", got)
	}
	if got[0].Text(e) != "@a.go" || got[1].Text(e) != "@b.go" {
		t.Errorf("in the order %q then %q, want a then b", got[0].Text(e), got[1].Text(e))
	}
}

func TestElementsAreACopy(t *testing.T) {
	e := editorWith("")
	e.InsertElement(fileChip, "@main.go")
	got := e.Elements()
	got[0].Start = 99

	if again := e.Elements(); again[0].Start != 0 {
		t.Errorf("writing to the returned slice moved the element to %d", again[0].Start)
	}
}

func TestInsertingNoElementDoesNothing(t *testing.T) {
	e := editorWith("text")
	if el := e.InsertElement(fileChip, ""); el.ID != 0 {
		t.Errorf("an element of no text was given identity %d", el.ID)
	}
	if got := e.Text(); got != "text" {
		t.Errorf("text = %q", got)
	}
}

func TestElementTextOfSomethingThatIsNotThere(t *testing.T) {
	e := editorWith("short")
	for _, el := range []headless.Element{
		{Line: 9, Start: 0, End: 2},
		{Line: 0, Start: -1, End: 2},
		{Line: 0, Start: 0, End: 99},
		{Line: 0, Start: 3, End: 3},
	} {
		if got := el.Text(e); got != "" {
			t.Errorf("%+v reported the text %q", el, got)
		}
	}
}

// TestElementsSurviveEveryWayOfMoving. Selecting is movement with shift, so each of
// these has to leave the element where it is and the identity as it was.
func TestElementsSurviveEveryWayOfMoving(t *testing.T) {
	e := editorWith("")
	el := e.InsertElement(fileChip, "@main.go")
	e.Insert("tail\nsecond line")

	for _, key := range []input.Key{
		{Code: input.Up},
		{Code: input.Down},
		{Code: input.Left},
		{Code: input.Right},
		{Code: input.Character, Rune: 'b', Mods: input.Alt},
		{Code: input.Character, Rune: 'f', Mods: input.Alt},
		{Code: input.Character, Rune: 'a', Mods: input.Ctrl},
		{Code: input.Character, Rune: 'e', Mods: input.Ctrl},
		shift(input.Left), shift(input.Right), shift(input.Up), shift(input.Down),
	} {
		e.Handle(key)
	}
	got := e.Elements()
	if len(got) != 1 || got[0].ID != el.ID {
		t.Fatalf("after moving about, the editor holds %+v", got)
	}
	if text := got[0].Text(e); text != "@main.go" {
		t.Errorf("the element covers %q", text)
	}
}

func TestKillingALineTakesTheElementsOnIt(t *testing.T) {
	e := editorWith("")
	e.InsertElement(fileChip, "@main.go")
	e.SetCursor(0, 0)
	e.KillToEnd()

	if got := e.Elements(); len(got) != 0 {
		t.Errorf("the editor still holds %+v after the line was killed", got)
	}
}

func TestDeletingAWordTakesAnAtomicElementWhole(t *testing.T) {
	e := editorWith("")
	e.InsertElement(fileChip, "@main.go")
	e.DeleteWordBack()

	if got := e.Text(); got != "" {
		t.Errorf("text = %q, want the whole element gone", got)
	}
	if got := e.Elements(); len(got) != 0 {
		t.Errorf("the editor still holds %+v after the element was deleted", got)
	}
}

func TestJoiningLinesKeepsAnElementBelow(t *testing.T) {
	e := editorWith("first\n")
	e.SetCursor(1, 0)
	el := e.InsertElement(fileChip, "@main.go")

	// Backspace at the start of the second line joins it onto the first.
	e.SetCursor(1, 0)
	e.Handle(input.Key{Code: input.Backspace})

	got := e.Elements()
	if len(got) != 1 || got[0].ID != el.ID {
		t.Fatalf("the editor holds %+v", got)
	}
	if got[0].Line != 0 || got[0].Start != len("first") {
		t.Errorf("the element is at %d:%d, want 0:%d", got[0].Line, got[0].Start, len("first"))
	}
}

// TestTheCursorCanSitOnEitherSideOfAnElement. Both ends are places a cursor may be;
// only what is between them is not. A cursor that skipped the near side could never
// type in front of an element.
func TestTheCursorCanSitOnEitherSideOfAnElement(t *testing.T) {
	e := editorWith("")
	e.Insert("say ")
	e.InsertElement(fileChip, "@main.go")
	e.SetCursor(0, 0)

	// Right along "say " lands on the element's near side, not past it.
	for range 4 {
		e.Handle(input.Key{Code: input.Right})
	}
	if _, col := e.Cursor(); col != len("say ") {
		t.Fatalf("the cursor is at column %d, want the near side of the element", col)
	}
	e.Insert("X")
	if got, want := e.Text(), "say X@main.go "; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := e.Elements(); len(got) != 1 {
		t.Errorf("typing in front of the element disturbed it: %+v", got)
	}
}
