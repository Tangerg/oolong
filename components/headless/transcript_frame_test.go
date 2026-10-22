package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
)

type transcriptFrameAction func(headless.Frame)

func (f transcriptFrameAction) Draw(frame headless.Frame) { f(frame) }

func TestTranscriptPlacementRequiresOneLiveFrameOwner(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*headless.Transcript, grid.View)
	}{
		{"zero frame", func(content *headless.Transcript, _ grid.View) {
			content.Stage(headless.Frame{}, 20)
		}},
		{"expired frame", func(content *headless.Transcript, view grid.View) {
			var saved headless.Frame
			headless.NewRoot(transcriptFrameAction(func(frame headless.Frame) { saved = frame })).Draw(view)
			content.Stage(saved, 20)
		}},
		{"old frame during later draw", func(content *headless.Transcript, view grid.View) {
			var saved headless.Frame
			root := headless.NewRoot(transcriptFrameAction(func(frame headless.Frame) { saved = frame }))
			root.Draw(view)
			root.SetContent(transcriptFrameAction(func(headless.Frame) { content.Stage(saved, 20) }))
			root.Draw(view)
		}},
		{"same width twice", func(content *headless.Transcript, view grid.View) {
			headless.NewRoot(transcriptFrameAction(func(frame headless.Frame) {
				content.Stage(frame, 20)
				content.Stage(frame, 20)
			})).Draw(view)
		}},
		{"unchanged then changed width", func(content *headless.Transcript, view grid.View) {
			headless.NewRoot(transcriptFrameAction(func(frame headless.Frame) {
				content.Stage(frame, 20)
				content.Stage(frame, 10)
			})).Draw(view)
		}},
		{"two roots", func(content *headless.Transcript, view grid.View) {
			other := headless.NewRoot(transcriptFrameAction(func(frame headless.Frame) { content.Stage(frame, 10) }))
			headless.NewRoot(transcriptFrameAction(func(frame headless.Frame) {
				content.Stage(frame, 20)
				other.Draw(view)
			})).Draw(view)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var content headless.Transcript
			stageTranscript(&content, 20)
			func() {
				defer func() {
					if recover() == nil {
						t.Error("transcript accepted placement without one live frame owner")
					}
				}()
				test.run(&content, grid.NewSurface(20, 1).View())
			}()
			if content.Width() != 20 {
				t.Fatalf("rejected frame changed committed width to %d", content.Width())
			}
			stageTranscript(&content, 10)
			if content.Width() != 10 {
				t.Fatal("rejected frame retained ownership after abort")
			}
		})
	}
}
