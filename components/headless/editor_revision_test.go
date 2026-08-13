package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
)

func TestEditorRevisionDistinguishesHandlingFromChanging(t *testing.T) {
	editor := editorWith("draft")
	before := editor.Revision()

	if !editor.Handle(input.Key{Code: input.Backspace}) {
		t.Fatal("backspace at the beginning was not handled")
	}
	if got := editor.Revision(); got != before {
		t.Fatalf("handled no-op advanced revision from %d to %d", before, got)
	}
	if !editor.Do(headless.DeleteBack) {
		t.Fatal("known no-op action was not handled")
	}
	if got := editor.Revision(); got != before {
		t.Fatalf("known no-op action advanced revision from %d to %d", before, got)
	}

	// Text is authoritative even when the physical key was not Character. This is a
	// real terminal event shape and the reason consumers must observe the editor
	// rather than infer a change from Code.
	if !editor.Handle(input.Key{Code: input.F1, Text: "x"}) {
		t.Fatal("key-provided text was not handled")
	}
	if got := editor.Revision(); got != before+1 {
		t.Fatalf("text insertion advanced revision to %d, want %d", got, before+1)
	}
}

func TestEveryEditorContentOperationAdvancesRevisionOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(*headless.Editor)
		change func(*headless.Editor)
	}{
		{"set text", nil, func(e *headless.Editor) { e.SetText("text") }},
		{"clear", func(e *headless.Editor) { e.SetText("text") }, func(e *headless.Editor) { e.Clear() }},
		{"insert", nil, func(e *headless.Editor) { e.Insert("text") }},
		{"selection replacement", func(e *headless.Editor) { e.SetText("old"); e.SelectAll() }, func(e *headless.Editor) { e.Insert("new") }},
		{"replace", func(e *headless.Editor) { e.SetText("old") }, func(e *headless.Editor) { e.Replace(0, 3, "new") }},
		{"rune", nil, func(e *headless.Editor) { e.InsertRune('x') }},
		{"newline", nil, func(e *headless.Editor) { e.Newline() }},
		{"backspace", func(e *headless.Editor) { e.SetText("x") }, func(e *headless.Editor) { e.DeleteBack() }},
		{"forward delete", func(e *headless.Editor) { e.SetText("x"); e.SetCursor(0, 0) }, func(e *headless.Editor) { e.DeleteForward() }},
		{"word delete", func(e *headless.Editor) { e.SetText("a word") }, func(e *headless.Editor) { e.DeleteWordBack() }},
		{"kill to end", func(e *headless.Editor) { e.SetText("text"); e.SetCursor(0, 0) }, func(e *headless.Editor) { e.KillToEnd() }},
		{"kill to start", func(e *headless.Editor) { e.SetText("text") }, func(e *headless.Editor) { e.KillToStart() }},
		{"selection delete", func(e *headless.Editor) { e.SetText("text"); e.SelectAll() }, func(e *headless.Editor) { e.DeleteSelection() }},
		{"paste event", nil, func(e *headless.Editor) { e.Handle(input.Paste{Text: "text"}) }},
		{"cut", func(e *headless.Editor) {
			e.Clipboard = &clip{}
			e.SetText("text")
			e.SelectAll()
		}, func(e *headless.Editor) { e.Cut() }},
		{"yank", func(e *headless.Editor) {
			e.SetText("text")
			e.KillToStart()
		}, func(e *headless.Editor) { e.Yank() }},
		{"yank pop", func(e *headless.Editor) {
			e.SetText("one")
			e.KillToStart()
			e.SetText("two")
			e.KillToStart()
			e.Yank()
		}, func(e *headless.Editor) { e.YankPop() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := &headless.Editor{}
			if tc.setup != nil {
				tc.setup(editor)
			}
			before := editor.Revision()
			tc.change(editor)
			if got := editor.Revision(); got != before+1 {
				t.Fatalf("revision = %d, want %d", got, before+1)
			}
		})
	}
}

func TestEditorRevisionIncludesElementsAndHistory(t *testing.T) {
	editor := &headless.Editor{}

	before := editor.Revision()
	element := editor.InsertElement(fileChip, "@main.go")
	if got := editor.Revision(); got != before+1 {
		t.Fatalf("element insertion revision = %d, want %d", got, before+1)
	}

	before = editor.Revision()
	if !editor.RemoveElement(element.ID) {
		t.Fatal("element was not removed")
	}
	if got := editor.Revision(); got != before+1 {
		t.Fatalf("element removal revision = %d, want %d", got, before+1)
	}

	before = editor.Revision()
	editor.Undo()
	if got := editor.Revision(); got != before+1 {
		t.Fatalf("undo revision = %d, want %d", got, before+1)
	}
	before = editor.Revision()
	editor.Redo()
	if got := editor.Revision(); got != before+1 {
		t.Fatalf("redo revision = %d, want %d", got, before+1)
	}
}

func TestEditorRevisionIgnoresNonContentOperationsAndNoOps(t *testing.T) {
	for _, tc := range []struct {
		name string
		does func(*headless.Editor)
	}{
		{"empty insert", func(e *headless.Editor) { e.Insert("") }},
		{"empty clear", func(e *headless.Editor) { e.Clear() }},
		{"same text", func(e *headless.Editor) { e.SetText("") }},
		{"empty replace", func(e *headless.Editor) { e.Replace(0, 0, "") }},
		{"control character", func(e *headless.Editor) { e.InsertRune('\x1b') }},
		{"single-line newline", func(e *headless.Editor) { e.Newline() }},
		{"backspace", func(e *headless.Editor) { e.DeleteBack() }},
		{"forward delete", func(e *headless.Editor) { e.DeleteForward() }},
		{"word delete", func(e *headless.Editor) { e.DeleteWordBack() }},
		{"kill to end", func(e *headless.Editor) { e.KillToEnd() }},
		{"kill to start", func(e *headless.Editor) { e.KillToStart() }},
		{"undo", func(e *headless.Editor) { e.Undo() }},
		{"redo", func(e *headless.Editor) { e.Redo() }},
		{"unknown element", func(e *headless.Editor) { e.RemoveElement(99) }},
		{"cursor", func(e *headless.Editor) { e.SetCursor(3, 8) }},
		{"movement", func(e *headless.Editor) { e.MoveRight() }},
		{"selection", func(e *headless.Editor) { e.SelectAll() }},
		{"focus", func(e *headless.Editor) { e.Focus(false) }},
		{"copy", func(e *headless.Editor) { e.Clipboard = &clip{}; e.Copy() }},
		{"paste request", func(e *headless.Editor) { e.Clipboard = &clip{}; e.Paste() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := &headless.Editor{SingleLine: true}
			before := editor.Revision()
			tc.does(editor)
			if got := editor.Revision(); got != before {
				t.Fatalf("revision = %d, want %d", got, before)
			}
		})
	}
}

func TestEditorRevisionIgnoresInteractionAndPresentationState(t *testing.T) {
	clipboard := &clip{}
	editor := &headless.Editor{Clipboard: clipboard}
	editor.SetText("first\nsecond\nthird")
	before := editor.Revision()

	editor.SetCursor(0, 0)
	editor.Anchor()
	editor.MoveRight()
	editor.Scroll().By(1)
	editor.Focus(false)
	if !editor.Copy() {
		t.Fatal("selected text was not copied")
	}
	if clipboard.held != "f" {
		t.Fatalf("clipboard holds %q, want f", clipboard.held)
	}
	if got := editor.Revision(); got != before {
		t.Fatalf("interaction state advanced revision from %d to %d", before, got)
	}
}

func TestReplacingVisibleTextStillReportsElementChanges(t *testing.T) {
	editor := &headless.Editor{}
	editor.InsertElement(fileChip, "@main.go")

	before := editor.Revision()
	editor.SetText(editor.Text())
	if got := editor.Revision(); got != before+1 {
		t.Fatalf("dropping element identity advanced revision to %d, want %d", got, before+1)
	}
	if got := editor.Elements(); len(got) != 0 {
		t.Fatalf("SetText retained elements %+v", got)
	}
}

func TestReplacingASelectionWithTheSameTextDoesNotAdvanceRevision(t *testing.T) {
	editor := editorWith("same")
	editor.SelectAll()
	before := editor.Revision()

	editor.Insert("same")
	if got := editor.Revision(); got != before {
		t.Fatalf("equivalent replacement advanced revision from %d to %d", before, got)
	}
	if selected := editor.Selected(); selected != "" {
		t.Fatalf("replacement left %q selected", selected)
	}
}

func TestEquivalentTypingLeavesTheNextEditUndoable(t *testing.T) {
	editor := editorWith("hello")
	editor.Anchor()
	editor.MoveRight()
	before := editor.Revision()

	editor.InsertRune('h')
	if got := editor.Revision(); got != before {
		t.Fatalf("equivalent typing advanced revision from %d to %d", before, got)
	}
	editor.InsertRune('X')
	editor.Undo()
	if got := editor.Text(); got != "hello" {
		t.Fatalf("undo after equivalent typing restored %q, want hello", got)
	}
}

func TestAHandledSingleLineNewlineClosesTheTypingRun(t *testing.T) {
	var editor headless.Editor
	editor.SingleLine = true
	editor.Handle(input.Key{Code: input.Character, Rune: 'x'})
	if !editor.Do(headless.InsertNewline) {
		t.Fatal("the known newline action was not handled")
	}
	editor.Handle(input.Key{Code: input.Character, Rune: 'y'})

	editor.Undo()
	if got := editor.Text(); got != "x" {
		t.Fatalf("undo after a handled newline restored %q, want the preceding typing run", got)
	}
}

func TestSettingTheSamePlainTextDoesNotAdvanceRevisionOrAddHistory(t *testing.T) {
	editor := editorWith("same")
	before := editor.Revision()

	editor.SetText("same")
	if got := editor.Revision(); got != before {
		t.Fatalf("equivalent SetText advanced revision from %d to %d", before, got)
	}
	editor.Undo()
	if got := editor.Text(); got != "" {
		t.Fatalf("no-op SetText inserted an undo step before %q", got)
	}
}

func TestEmptyReplacementInsideAnElementIsANoOp(t *testing.T) {
	editor := &headless.Editor{}
	element := editor.InsertElement(fileChip, "@main.go")
	before := editor.Revision()

	editor.Replace(3, 3, "")
	if got := editor.Revision(); got != before {
		t.Fatalf("empty replacement advanced revision from %d to %d", before, got)
	}
	got := editor.Elements()
	if len(got) != 1 || got[0].ID != element.ID {
		t.Fatalf("empty replacement disturbed element: %+v", got)
	}
}

func TestRevisionIsMonotonicAcrossCoalescedTypingAndHistory(t *testing.T) {
	editor := &headless.Editor{}
	for _, r := range "abc" {
		editor.InsertRune(r)
	}
	if got := editor.Revision(); got != 3 {
		t.Fatalf("three content changes produced revision %d", got)
	}

	editor.Undo()
	if got := editor.Revision(); got != 4 {
		t.Fatalf("undo restored an old revision: got %d, want 4", got)
	}
	if got := editor.Text(); got != "" {
		t.Fatalf("coalesced undo left %q", got)
	}

	editor.Redo()
	if got := editor.Revision(); got != 5 {
		t.Fatalf("redo restored an old revision: got %d, want 5", got)
	}
}
