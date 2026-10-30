package base

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestTranscriptInputRejectsPresentationReentryBeforeChangingGesture(t *testing.T) {
	transcript, err := NewTranscript(TranscriptConfig{})
	if err != nil {
		t.Fatal(err)
	}
	transcript.clicked, transcript.dragged = 9, true
	if paintErr := transcript.Element().SetPaint(func(scene.Frame) {
		defer func() {
			if recover() == nil {
				t.Error("paint changed interaction state")
			}
		}()
		transcript.Handle(input.Mouse{Action: input.MouseCancel})
	}); paintErr != nil {
		t.Fatal(paintErr)
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: transcript.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	candidate, err := root.Prepare(grid.NewSurface(20, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	candidate.Abort()
	if transcript.clicked != 9 || !transcript.dragged {
		t.Fatal("phase guard ran after gesture mutation")
	}
}
