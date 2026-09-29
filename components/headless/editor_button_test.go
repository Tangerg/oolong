package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestEditorSelectionWaitsForItsPressedButton(t *testing.T) {
	for _, release := range []struct {
		name   string
		button input.Button
	}{
		{name: "left", button: input.ButtonLeft},
		{name: "unspecified", button: input.ButtonNone},
	} {
		t.Run(release.name, func(t *testing.T) {
			var editor headless.Editor
			editor.SetText("abcdef")
			root := headless.NewRoot(&editor)
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
			if !root.Handle(input.Mouse{Pos: image.Pt(-1, 0), Action: input.MouseUp, Button: release.button}) {
				t.Fatal("the matching release outside the editor was declined")
			}
			if root.Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) || editor.Selected() != "bcde" {
				t.Fatal("a released editor drag resumed or changed its selection")
			}
		})
	}
}
