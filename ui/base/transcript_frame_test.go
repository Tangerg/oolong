package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestTranscriptPlacementRequiresOneLiveFrameOwner(t *testing.T) {
	for _, test := range []string{"zero frame", "expired frame", "same width twice", "different widths twice", "two roots"} {
		t.Run(test, func(t *testing.T) {
			transcript := transcriptModel(t, base.TranscriptConfig{})
			var saved scene.Frame
			run := func(frame scene.Frame) { saved = frame; transcript.Stage(frame, 20) }
			root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { run(frame) }})})
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			transcriptCommit(t, root, 20, 1, 1)
			old := saved
			run = func(frame scene.Frame) {
				switch test {
				case "zero frame":
					transcript.Stage(scene.Frame{}, 10)
				case "expired frame":
					transcript.Stage(old, 10)
				case "same width twice":
					transcript.Stage(frame, 10)
					transcript.Stage(frame, 10)
				case "different widths twice":
					transcript.Stage(frame, 20)
					transcript.Stage(frame, 10)
				case "two roots":
					transcript.Stage(frame, 20)
					other, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{Paint: func(other scene.Frame) { transcript.Stage(other, 10) }})})
					if err != nil {
						panic(err)
					}
					defer other.Close()
					_, _ = other.Prepare(grid.NewSurface(10, 1).View())
				}
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Error("accepted invalid frame owner")
					}
				}()
				_, _ = root.Prepare(grid.NewSurface(10, 1).View())
			}()
			if transcript.Width() != 20 {
				t.Fatal("rejected frame advanced width")
			}
			run = func(frame scene.Frame) { transcript.Stage(frame, 10) }
			transcriptCommit(t, root, 10, 1, 2)
			if transcript.Width() != 10 {
				t.Fatal("aborted frame retained ownership")
			}
		})
	}
}
