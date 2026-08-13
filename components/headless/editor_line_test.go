package headless_test

import (
	"fmt"
	"image"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/text"
)

func TestAFieldGivingUpFocusCancelsItsPartialBinding(t *testing.T) {
	var resolve func()
	cancelled := false
	keys := &keymap.Map{Resolve: func(_ time.Duration, fn func()) func() {
		resolve = fn
		return func() { cancelled = true }
	}}
	keys.Bind(headless.MoveLeft, input.Chord{Rune: 'g'})
	keys.Bind(headless.MoveRight, input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	e := &headless.Editor{}
	e.Keys = keys
	e.SetText("ab")

	if !e.Handle(input.Key{Rune: 'g'}) || resolve == nil {
		t.Fatal("the ambiguous first chord was not held")
	}
	e.Focus(false)
	if !cancelled {
		t.Fatal("blur did not cancel the binding resolver")
	}
	resolve()
	if line, col := e.Cursor(); line != 0 || col != 2 {
		t.Fatalf("late resolver moved cursor to %d:%d", line, col)
	}
}

func TestAFieldThatHoldsOneLineNeverGetsASecond(t *testing.T) {
	// Every way text arrives goes through the same rule, because a rule kept in four
	// places out of five is a rule with a way round it. A break becomes a space: it is
	// what the text meant, and dropping it would join two words.
	e := &headless.Editor{}
	e.SetSingleLine(true)
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
	e := &headless.Editor{}
	e.SetSingleLine(true)
	e.SetText("abcdefghij")

	// The cursor is at the end, so the window shows the end of the text with room for
	// the cursor to sit past the last character.
	rows := paintWidget(5, 1, e)
	equalRows(t, rows, []string{"ghij."})

	e.Do(headless.MoveLineStart)
	rows = paintWidget(5, 1, e)
	equalRows(t, rows, []string{"abcde"})
}

func TestAFieldThatHoldsOneLineComesBackWhenItsTextGetsShorter(t *testing.T) {
	// Otherwise a deletion leaves the field showing its end with blank columns to the
	// right of text that would have fitted.
	e := &headless.Editor{}
	e.SetSingleLine(true)
	e.SetText("abcdefghij")
	paintWidget(5, 1, e)
	for range 8 {
		e.DeleteBack()
	}
	rows := paintWidget(5, 1, e)
	equalRows(t, rows, []string{"ab..."})
}

func TestAMaskedFieldShowsNothingItHolds(t *testing.T) {
	e := &headless.Editor{}
	e.SetMask("•")
	e.SetText("secret")
	rows := paintWidget(8, 1, e)
	equalRows(t, rows, []string{"••••••.."})
	if got := e.Text(); got != "secret" {
		t.Fatalf("the field itself = %q, want what was typed", got)
	}
}

func TestAMaskedFieldPutsTheCursorWhereTheMaskIs(t *testing.T) {
	// The mask is one cluster per cluster and a different count of bytes, so every
	// position has to be translated. A cursor placed from the text's own offsets would
	// sit wherever the two happened to agree.
	e := &headless.Editor{}
	e.SetMask("••")
	e.SetText("ab")
	e.SetCursor(0, 1)

	// A screen rather than a bare surface, because the cursor is only observable
	// through one.
	screen := grid.NewScreen(8, 1)
	headless.NewRoot(e).Draw(screen.Frame())
	if cursor := screen.Cursor(); !cursor.Visible || cursor.Pos.X != 2 {
		t.Fatalf("cursor at %+v, want it two masked columns along", cursor)
	}
}

func TestAnEditorPublishesItsCursorStyleWithItsFrame(t *testing.T) {
	e := &headless.Editor{}
	e.CursorStyle = grid.CursorStyle{Shape: grid.CursorBar, Blink: true}

	screen := grid.NewScreen(8, 1)
	headless.NewRoot(e).Draw(screen.Frame())
	if got := screen.Cursor().Style; got != e.CursorStyle {
		t.Fatalf("cursor style = %+v, want %+v", got, e.CursorStyle)
	}
}

func TestAClickInAMaskedFieldLandsOnTheClusterItIsUnder(t *testing.T) {
	e := &headless.Editor{}
	e.SetMask("••")
	e.SetText("abc")
	paintWidget(10, 1, e)

	e.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(4, 0)})
	if _, col := e.Cursor(); col != 2 {
		t.Fatalf("cursor at byte %d, want the third cluster", col)
	}
}

func TestAMaskedFieldHoldsOneLineWhetherOrNotItWasAsked(t *testing.T) {
	// A secret is one value, and where to break a line nobody can read is not a
	// question worth an answer.
	e := &headless.Editor{}
	e.SetMask("*")
	e.SetText("a\nb")
	if got := e.Text(); got != "a b" {
		t.Fatalf("= %q", got)
	}
}

func TestEnablingOneLineModeSettlesExistingContent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		enable func(*headless.Editor)
	}{
		{"single-line", func(e *headless.Editor) { e.SetSingleLine(true) }},
		{"masked", func(e *headless.Editor) { e.SetMask("•") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := editorWith("first\nsecond")
			editor.SetCursor(1, len("second"))
			before := editor.Revision()
			tc.enable(editor)

			if got := editor.Text(); got != "first second" {
				t.Fatalf("one-line content = %q", got)
			}
			if got := editor.Revision(); got != before+1 {
				t.Fatalf("mode transition advanced revision to %d, want %d", got, before+1)
			}
			if line, col := editor.Cursor(); line != 0 || col != len("first second") {
				t.Fatalf("cursor = (%d,%d), want the same document position on line zero", line, col)
			}
			editor.Undo()
			if got := editor.Text(); got != "first second" {
				t.Fatalf("undo restored mode-invalid content %q", got)
			}
		})
	}
}

func TestOneLineModeCannotRestoreMultilineHistory(t *testing.T) {
	var editor headless.Editor
	editor.SetText("first\nsecond")
	editor.SetText("current")
	editor.SetSingleLine(true)

	editor.Undo()
	if got := editor.Text(); got != "current" {
		t.Fatalf("undo restored mode-invalid history %q", got)
	}
}

func TestDrawingTextDoesNotChangeItsEditorsHistory(t *testing.T) {
	field := &headless.Text{}
	editor := field.Editor()
	editor.SetText("one")
	editor.SetText("one\ntwo")
	editor.SetSingleLine(false)

	rows := paintWidget(20, 1, field)
	equalRows(t, rows, []string{"one two............."})
	editor.Undo()
	if got := editor.Text(); got != "one" {
		t.Fatalf("undo after Draw restored %q, want one", got)
	}
}

func TestEditorRejectsAMaskWithNoStableCellGeometry(t *testing.T) {
	for _, mask := range []string{"\t", "\n", "\x1b", "\u0301", "\xff"} {
		t.Run(fmt.Sprintf("%q", mask), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("SetMask(%q) did not panic", mask)
				}
			}()
			var editor headless.Editor
			editor.SetMask(mask)
		})
	}
}

func TestTextRejectsAnInvalidMaskBeforeDrawing(t *testing.T) {
	for _, mask := range []string{"\t", "\a", "\u200b"} {
		t.Run(fmt.Sprintf("%q", mask), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("Editor.SetMask(%q) did not reject invalid configuration", mask)
				}
			}()
			field := &headless.Text{}
			field.Editor().SetMask(mask)
		})
	}
}

func TestTextAcceptsAndDrawsAConfiguredMask(t *testing.T) {
	value := "secret"
	field := &headless.Text{Value: headless.Bind(&value)}
	field.Editor().SetMask("*")
	if got := field.Editor().Mask(); got != "*" {
		t.Fatalf("Mask() = %q, want *", got)
	}
	rows := paintWidget(8, 1, field)
	equalRows(t, rows, []string{"******.."})
}

func TestEveryTextEntryPathBuildsTheSameTerminalSafeDocument(t *testing.T) {
	const source = "a\r\nb\rc\x00d\x1be\tf\xff"
	const want = "a\nb\ncde\tf\ufffd"
	for _, tc := range []struct {
		name string
		put  func(*headless.Editor)
	}{
		{"set", func(e *headless.Editor) { e.SetText(source) }},
		{"insert", func(e *headless.Editor) { e.Insert(source) }},
		{"paste", func(e *headless.Editor) { e.Handle(input.Paste{Text: source}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var editor headless.Editor
			tc.put(&editor)
			if got := editor.Text(); got != want {
				t.Fatalf("stored %q, want %q", got, want)
			}
		})
	}
}

func TestReplaceCannotSplitAGraphemeCluster(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start, end int
		want       string
	}{
		{"insertion lands before the cluster", 2, 2, "aX中文b"},
		{"replacement covers touched clusters", 2, 6, "aXb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := editorWith("a中文b")
			editor.Replace(tc.start, tc.end, "X")
			if got := editor.Text(); got != tc.want {
				t.Fatalf("replacement = %q, want %q", got, tc.want)
			}
			if !utf8.ValidString(editor.Text()) {
				t.Fatalf("replacement left invalid UTF-8 %q", editor.Text())
			}
		})
	}
}

func TestVerticalMovementBeforeTheFirstFrameKeepsAClusterBoundary(t *testing.T) {
	editor := editorWith("中文\nabc")
	editor.SetCursor(1, 2)
	editor.MoveUp()
	line, col := editor.Cursor()
	if line != 0 || col != len("中") {
		t.Fatalf("cursor = (%d,%d), want column two after the first wide cluster", line, col)
	}
	editor.Insert("X")
	if got := editor.Text(); got != "中X文\nabc" || !utf8.ValidString(got) {
		t.Fatalf("insertion after vertical movement = %q", got)
	}
}

func TestInsertionSettlesAGraphemeThatReformsAcrossItsBoundary(t *testing.T) {
	editor := editorWith("a\u0605")
	editor.SetCursor(0, 1)
	editor.Insert("\u0605")

	line, col := editor.Cursor()
	if line != 0 || !clusterOffset(editor.Text(), col) {
		t.Fatalf("cursor (%d,%d) splits the reformed grapheme in %q", line, col, editor.Text())
	}
	editor.Insert("X")
	if got := editor.Text(); !utf8.ValidString(got) {
		t.Fatalf("the next insertion split UTF-8: %q", got)
	}
}

func clusterOffset(line string, at int) bool {
	if at == 0 || at == len(line) {
		return true
	}
	return text.NextCluster(line, text.PrevCluster(line, at)) == at
}
