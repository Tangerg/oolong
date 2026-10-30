package base_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestEditorSelectionWaitsForItsPressedButton(t *testing.T) {
	for _, release := range []struct {
		name    string
		button  input.Button
		action  input.MouseAction
		handled bool
	}{
		{name: "left", button: input.ButtonLeft, action: input.MouseUp, handled: true},
		{name: "unspecified", button: input.ButtonNone, action: input.MouseUp, handled: true},
		{name: "cancel", button: input.ButtonNone, action: input.MouseCancel},
	} {
		t.Run(release.name, func(t *testing.T) {
			var editor base.Editor
			editor.SetText("abcdef")
			root := newTestRoot(&editor)
			root.Draw(grid.NewSurface(10, 1).View())
			root.Handle(input.Mouse{Pos: image.Pt(1, 0), Action: input.MouseDown, Button: input.ButtonLeft})
			root.Handle(input.Mouse{Pos: image.Pt(3, 0), Action: input.MouseDrag, Button: input.ButtonLeft})
			if editor.Selected() != "bc" {
				t.Fatalf("initial drag selected %q, want bc", editor.Selected())
			}
			if root.Handle(input.Mouse{Pos: image.Pt(3, 0), Action: input.MouseUp, Button: input.ButtonRight}) {
				t.Error("another button's release was consumed by the editor")
			}
			if !root.Handle(input.Mouse{Pos: image.Pt(5, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) || editor.Selected() != "bcde" {
				t.Fatalf("another button's release ended the editor drag: selected %q", editor.Selected())
			}
			if handled := root.Handle(input.Mouse{Pos: image.Pt(-1, 0), Action: release.action, Button: release.button}); handled != release.handled {
				t.Fatalf("ending gesture returned %v, want %v", handled, release.handled)
			}
			if root.Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) || editor.Selected() != "bcde" {
				t.Fatal("a released editor drag resumed or changed its selection")
			}
		})
	}
}
