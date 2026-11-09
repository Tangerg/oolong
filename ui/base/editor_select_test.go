package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// clip stands in for a system clipboard, and can refuse the way a terminal does.
type clip struct {
	held    string
	refuse  bool
	asked   int
	receive func(string, error)
}

func (c *clip) Copy(text string) bool {
	if c.refuse {
		return false
	}
	c.held = text
	return true
}

func (c *clip) Read(ctx context.Context, receive func(string, error)) error {
	if c.refuse {
		return errors.ErrUnsupported
	}
	c.asked++
	c.receive = func(text string, err error) {
		if ctx.Err() == nil {
			receive(text, err)
		}
	}
	return nil
}

// shift is the same key with shift held, which is what a terminal sends.
func shift(code input.Code) input.Key {
	return input.Key{Code: code, Mods: input.Shift}
}

func editorWith(text string) *base.Editor {
	e := &base.Editor{}
	e.SetText(text)
	e.SetCursor(0, 0)
	return e
}

func TestEditorDefaultBindingsExtendSelection(t *testing.T) {
	e := editorWith("hello world")
	for range 5 {
		e.Handle(shift(input.Right))
	}
	if got := e.Selected(); got != "hello" {
		t.Errorf("selected %q, want %q", got, "hello")
	}

	// And carries on from where it is rather than starting again.
	e.Handle(shift(input.Right))
	if got := e.Selected(); got != "hello " {
		t.Errorf("extended to %q, want %q", got, "hello ")
	}
}

func TestEditorHonorsExplicitShiftBinding(t *testing.T) {
	keys := base.DefaultEditorKeys()
	keys.Bind(base.DeleteForward, input.Shift.With(input.Left))
	editor := base.NewEditor(base.EditorConfig{Initial: "abcd"})
	editor.SetKeys(keys)
	editor.SetCursor(0, 2)
	if !editor.Handle(shift(input.Left)) || editor.Text() != "abd" {
		t.Fatalf("explicit shifted deletion produced %q, want abd", editor.Text())
	}
}

func TestEditorShiftPrefixReachesMatcher(t *testing.T) {
	keys := base.DefaultEditorKeys()
	keys.Bind(base.SelectAll, input.Shift.With(input.Left), input.Ctrl.Rune('a'))
	editor := base.NewEditor(base.EditorConfig{Initial: "abcd"})
	editor.SetKeys(keys)
	editor.SetCursor(0, 2)
	editor.Handle(shift(input.Left))
	if _, offset := editor.Cursor(); offset != 2 || editor.Selected() != "" {
		t.Fatal("unfinished shifted sequence changed the selection")
	}
	editor.Handle(input.Key{Code: input.Character, Rune: 'a', Mods: input.Ctrl})
	if got := editor.Selected(); got != "abcd" {
		t.Fatalf("shifted sequence selected %q, want abcd", got)
	}
}

func TestEditorSelectionExtensionBelongsToTheAction(t *testing.T) {
	keys := base.DefaultEditorKeys()
	keys.Unbind(input.Shift.With(input.Left))
	keys.Bind(base.ExtendLeft, input.Ctrl.Rune('b'))
	editor := base.NewEditor(base.EditorConfig{Initial: "abcd"})
	editor.SetKeys(keys)
	editor.SetCursor(0, 2)
	if editor.Handle(shift(input.Left)) {
		t.Fatal("unbound shifted movement was consumed")
	}
	if _, offset := editor.Cursor(); offset != 2 || editor.Selected() != "" {
		t.Fatal("unbound shifted movement changed the selection")
	}
	if !editor.Handle(input.Key{Rune: 'b', Mods: input.Ctrl}) || editor.Selected() != "b" {
		t.Fatal("an extension bound without Shift did not select")
	}
	if !editor.Do(base.ExtendLeft) || editor.Selected() != "ab" {
		t.Fatal("the same semantic action did not continue the selection")
	}
}

func TestEditorSelectsBackwards(t *testing.T) {
	e := editorWith("hello world")
	e.SetCursor(0, 5)
	for range 5 {
		e.Handle(shift(input.Left))
	}
	if got := e.Selected(); got != "hello" {
		t.Errorf("selected %q, want %q", got, "hello")
	}
}

func TestEditorSelectsAcrossLines(t *testing.T) {
	e := editorWith("first\nsecond\nthird")
	e.SetCursor(0, 2)
	e.Handle(shift(input.Down))
	e.Handle(shift(input.Down))
	if got, want := e.Selected(), "rst\nsecond\nth"; got != want {
		t.Errorf("selected %q, want %q", got, want)
	}
}

func TestMovingWithoutShiftLetsTheSelectionGo(t *testing.T) {
	e := editorWith("hello world")
	e.Handle(shift(input.Right))
	e.Handle(shift(input.Right))
	if e.Selected() == "" {
		t.Fatal("nothing was selected to begin with")
	}
	e.Handle(input.Key{Code: input.Right})
	if got := e.Selected(); got != "" {
		t.Errorf("still selected %q after an arrow key", got)
	}
}

func TestReplacementDropsASelectionWhetherOrNotTextChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*base.Editor)
	}{
		{"clear", func(e *base.Editor) { e.Clear() }},
		{"different set text", func(e *base.Editor) { e.SetText("hi") }},
		{"equivalent set text", func(e *base.Editor) { e.SetText("hello world") }},
		{"different replace", func(e *base.Editor) { e.Replace(0, 5, "HELLO") }},
		{"equivalent replace", func(e *base.Editor) { e.Replace(0, 5, "hello") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := editorWith("hello world")
			editor.SelectAll()
			tc.change(editor)
			if selected := editor.Selected(); selected != "" {
				t.Fatalf("content replacement left %q selected", selected)
			}
		})
	}
}

func TestSelectingNothingIsNotASelection(t *testing.T) {
	// A shift-arrow pressed and taken back leaves the anchor on the cursor, and
	// copying that would put an empty string on the clipboard.
	e := editorWith("hello")
	e.Handle(shift(input.Right))
	e.Handle(shift(input.Left))
	if _, _, ok := e.Selection(); ok {
		t.Errorf("an anchor on the cursor was reported as a selection of %q", e.Selected())
	}
}

func TestEditorSelectAll(t *testing.T) {
	e := editorWith("first\nsecond")
	e.Handle(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	if got, want := e.Selected(), "first\nsecond"; got != want {
		t.Errorf("selected %q, want %q", got, want)
	}
}

// TestSelectAllDoesNotTakeTheStartOfTheLine, because Ctrl+A is where readline has
// always put it and a text field that moved it would be wrong everywhere at once.
func TestSelectAllDoesNotTakeTheStartOfTheLine(t *testing.T) {
	e := editorWith("hello")
	e.SetCursor(0, 5)
	e.Handle(input.Key{Code: input.Character, Rune: 'a', Mods: input.Ctrl})
	if _, col := e.Cursor(); col != 0 {
		t.Errorf("the cursor is at column %d, want the start of the line", col)
	}
	if got := e.Selected(); got != "" {
		t.Errorf("it selected %q as well", got)
	}
}

func TestTypingReplacesASelection(t *testing.T) {
	e := editorWith("hello world")
	for range 5 {
		e.Handle(shift(input.Right))
	}
	e.Handle(input.Key{Code: input.Character, Rune: 'b'})
	if got, want := e.Text(), "b world"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := e.Selected(); got != "" {
		t.Errorf("the selection survived being typed over: %q", got)
	}
}

func TestNewlineReplacesASelection(t *testing.T) {
	e := editorWith("hello world")
	for range 5 {
		e.Handle(shift(input.Right))
	}
	e.Newline()
	if got, want := e.Text(), "\n world"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := e.Selected(); got != "" {
		t.Errorf("the selection survived being replaced by a newline: %q", got)
	}
}

// TestTypingOverASelectionIsOneStepToUndo, because a user who selected a word and
// typed another did one thing.
func TestTypingOverASelectionIsOneStepToUndo(t *testing.T) {
	e := editorWith("hello world")
	for range 5 {
		e.Handle(shift(input.Right))
	}
	e.Handle(input.Key{Code: input.Character, Rune: 'b'})
	e.Undo()
	if got, want := e.Text(), "hello world"; got != want {
		t.Errorf("one undo gave %q, want %q", got, want)
	}
}

func TestDeletingWithASelectionTakesTheSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  input.Key
	}{
		{"backspace", input.Key{Code: input.Backspace}},
		{"delete", input.Key{Code: input.Delete}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := editorWith("hello world")
			e.SetCursor(0, 6)
			for range 5 {
				e.Handle(shift(input.Right))
			}
			e.Handle(tc.key)
			if got, want := e.Text(), "hello "; got != want {
				t.Errorf("text = %q, want %q", got, want)
			}
		})
	}
}

func TestEditorCopiesToTheClipboard(t *testing.T) {
	c := &clip{}
	e := editorWith("hello world")
	e.SetClipboard(c)
	for range 5 {
		e.Handle(shift(input.Right))
	}
	e.Handle(input.Key{Code: input.Character, Rune: 'c', Mods: input.Alt})

	if c.held != "hello" {
		t.Errorf("the clipboard holds %q, want %q", c.held, "hello")
	}
	if got := e.Text(); got != "hello world" {
		t.Errorf("copying changed the text to %q", got)
	}
}

func TestEditorCutsToTheClipboard(t *testing.T) {
	c := &clip{}
	e := editorWith("hello world")
	e.SetClipboard(c)
	for range 5 {
		e.Handle(shift(input.Right))
	}
	e.Handle(input.Key{Code: input.Character, Rune: 'x', Mods: input.Alt})

	if c.held != "hello" {
		t.Errorf("the clipboard holds %q", c.held)
	}
	if got, want := e.Text(), " world"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// TestCutKeepsTheTextWhenTheClipboardRefusesIt. A terminal is free to refuse, and a
// cut that emptied the field into a refusal would lose the text with nothing to
// paste back.
func TestCutKeepsTheTextWhenTheClipboardRefusesIt(t *testing.T) {
	c := &clip{refuse: true}
	e := editorWith("hello world")
	e.SetClipboard(c)
	for range 5 {
		e.Handle(shift(input.Right))
	}
	if e.Cut() {
		t.Error("a refused cut reported success")
	}
	if got, want := e.Text(), "hello world"; got != want {
		t.Errorf("text = %q, want it untouched", got)
	}
}

func TestEditorAsksTheClipboardForAPaste(t *testing.T) {
	c := &clip{}
	e := editorWith("")
	e.SetClipboard(c)
	e.Handle(input.Key{Code: input.Character, Rune: 'v', Mods: input.Alt})
	if c.asked != 1 {
		t.Fatalf("the clipboard was asked %d times, want 1", c.asked)
	}
	c.receive("from elsewhere", nil)
	if got := e.Text(); got != "from elsewhere" {
		t.Errorf("text = %q", got)
	}
}

func TestEditorWithoutAClipboardDoesNothingQuietly(t *testing.T) {
	e := editorWith("hello")
	e.SelectAll()
	if e.Copy() {
		t.Error("a copy with nowhere to put it reported success")
	}
	if e.Cut() {
		t.Error("a cut with nowhere to put it reported success")
	}
	if err := e.Paste(t.Context(), func(error) {}); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("paste without a clipboard = %v", err)
	}
	if got, want := e.Text(), "hello"; got != want {
		t.Errorf("text = %q, want it untouched", got)
	}
}

func TestPastingReplacesASelection(t *testing.T) {
	e := editorWith("hello world")
	for range 5 {
		e.Handle(shift(input.Right))
	}
	e.Handle(input.Paste{Text: "goodbye"})
	if got, want := e.Text(), "goodbye world"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestShiftWithSomethingThatIsNotAMovementStillTypes(t *testing.T) {
	e := editorWith("")
	e.Handle(input.Key{Code: input.Character, Rune: 'A', Mods: input.Shift})
	if got := e.Text(); got != "A" {
		t.Errorf("text = %q, want %q", got, "A")
	}
	if _, _, ok := e.Selection(); ok {
		t.Error("typing a capital letter started a selection")
	}
}

func TestMaskedEditorCannotExportTheSelectionFromAnyView(t *testing.T) {
	session := base.NewEditSession(base.EditSessionConfig{Initial: "secret"})
	owner := base.NewEditor(base.EditorConfig{Session: session})
	view := base.NewEditor(base.EditorConfig{Session: session})
	clipboard := &clip{held: "unchanged"}
	view.SetClipboard(clipboard)
	owner.SetMask("*")
	view.SelectAll()
	if view.Copy() || view.Cut() || clipboard.held != "unchanged" || view.Text() != "secret" {
		t.Fatal("masked selection escaped to clipboard or was deleted")
	}
	owner.SetMask("")
	if !view.Copy() || clipboard.held != "secret" {
		t.Fatal("visible selection failed to copy")
	}
}

func TestCopyDuringObservationNeverInvokesClipboard(t *testing.T) {
	editor := base.NewEditor(base.EditorConfig{Initial: "private"})
	clipboard := &clip{}
	editor.SetClipboard(clipboard)
	editor.SelectAll()
	defer func() {
		if err, _ := recover().(error); !errors.Is(err, scene.ErrBusy) || clipboard.held != "" {
			t.Fatal("clipboard export was not refused during observation")
		}
	}()
	editor.Element().ReadOnly(func() { editor.Copy() })
}
