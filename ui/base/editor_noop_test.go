package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

// Backspace at the start of the document is a handled no-op. It used to leave
// the typing run open, so the next character joined the edits before it and one
// Undo restored text the user had already deleted.
func TestBackspaceAtTheStartClosesTheTypingRun(t *testing.T) {
	editor := &base.Editor{}
	editor.SetText("hello")
	editor.SetCursor(0, 2)
	editor.InsertRune('x')
	for range 4 {
		editor.DeleteBack()
	}
	editor.InsertRune('y')
	if got := editor.Text(); got != "yllo" {
		t.Fatalf("text = %q", got)
	}
	editor.Undo()
	if got := editor.Text(); got != "llo" {
		t.Fatalf("undo restored %q, want only the character typed after the no-op removed", got)
	}
}
