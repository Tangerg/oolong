package headless

// Tests of what this package keeps to itself.
//
// The undo history's bound is a promise about memory in a long-lived process, not
// about behaviour a caller can see: an editor that kept every keystroke for a
// session that runs all day is a leak with a friendly name. Nothing outside can
// ask how many steps are held, so this asks from inside.

import (
	"testing"
)

func TestEditorUndoHistoryIsBounded(t *testing.T) {
	// An unbounded history in a long-lived process is a leak with a friendly name.
	e := NewEditor()
	for i := range maxUndo + 50 {
		e.Insert("x")
		e.MoveLeft()
		_ = i
	}
	if len(e.undo) > maxUndo {
		t.Fatalf("history holds %d steps, want at most %d", len(e.undo), maxUndo)
	}
}
