package base_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestTranscriptStageRequiresItsOwnMountedSceneBeforeMeasurement(t *testing.T) {
	for _, attachment := range []string{"detached", "another scene"} {
		t.Run(attachment, func(t *testing.T) {
			model := transcriptModel(t, base.TranscriptConfig{})
			measurements := 0
			transcriptAdd(t, model, &measuredTranscriptBlock{rows: 1, measure: func() { measurements++ }})
			var owner *scene.Root
			want := scene.ErrDetached
			if attachment == "another scene" {
				owner = transcriptScene(t, model)
				want = scene.ErrInvalid
			}
			foreign, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { model.Stage(frame, 20) }})})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(foreign.Close)
			func() {
				defer func() {
					got, _ := recover().(error)
					if !errors.Is(got, want) {
						t.Errorf("invalid producer panic=%v, want %v", got, want)
					}
				}()
				_, _ = foreign.Prepare(grid.NewSurface(20, 1).View())
			}()
			if measurements != 0 || model.Width() != 0 || model.Height() != 0 {
				t.Fatal("invalid producer read content or changed geometry before refusal")
			}
			if owner == nil {
				owner = transcriptScene(t, model)
			}
			transcriptCommit(t, owner, 20, 1, 1)
			if measurements != 1 || model.Width() != 20 || model.Height() != 1 {
				t.Fatal("refused producer prevented its owner's valid frame")
			}
		})
	}
}

func TestTranscriptPlacementRequiresOneLiveFrameOwner(t *testing.T) {
	for _, test := range []string{"zero frame", "expired frame", "same width twice", "different widths twice", "two roots"} {
		t.Run(test, func(t *testing.T) {
			transcript := transcriptModel(t, base.TranscriptConfig{})
			var saved scene.Frame
			run := func(frame scene.Frame) { saved = frame; transcript.Stage(frame, 20) }
			element := transcript.Element()
			if err := element.SetPaint(func(frame scene.Frame) { run(frame) }); err != nil {
				t.Fatal(err)
			}
			root, err := scene.NewRoot(scene.RootConfig{Element: element})
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
