package kit_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestTranscriptSelectionWaitsForItsPressedButton(t *testing.T) {
	for _, release := range []struct {
		name   string
		button input.Button
	}{
		{name: "left", button: input.ButtonLeft},
		{name: "unspecified", button: input.ButtonNone},
	} {
		t.Run(release.name, func(t *testing.T) {
			content := session(t, 10, []string{"abcdef"})
			var selection headless.Selection
			transcript := &kit.Transcript{Content: content, Selection: &selection}
			root := headless.NewRoot(transcript)
			root.Draw(grid.NewSurface(10, 1).View())
			root.Handle(input.Mouse{Pos: image.Pt(1, 0), Action: input.MouseDown, Button: input.ButtonLeft})
			root.Handle(input.Mouse{Pos: image.Pt(3, 0), Action: input.MouseDrag, Button: input.ButtonLeft})
			if selection.Text(content) != "bcd" {
				t.Fatalf("initial drag selected %q, want bcd", selection.Text(content))
			}
			if root.Handle(input.Mouse{Pos: image.Pt(3, 0), Action: input.MouseUp, Button: input.ButtonRight}) {
				t.Error("another button's release was consumed by the transcript")
			}
			if !selection.Dragging() || !root.Handle(input.Mouse{Pos: image.Pt(4, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) || selection.Text(content) != "bcde" {
				t.Fatalf("another button's release ended the transcript drag: selected %q", selection.Text(content))
			}
			if !root.Handle(input.Mouse{Pos: image.Pt(-1, 0), Action: input.MouseUp, Button: release.button}) {
				t.Fatal("the matching release outside the transcript was declined")
			}
			if selection.Dragging() || root.Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) || selection.Text(content) != "bcde" {
				t.Fatal("a released transcript drag resumed or changed its selection")
			}
		})
	}
}
