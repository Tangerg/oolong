package headless

import (
	"github.com/Tangerg/oolong/core/grid"
)

type transcriptTestFrame struct {
	transcript *Transcript
	width      int
}

func (f *transcriptTestFrame) Draw(frame Frame) {
	f.transcript.Stage(frame, f.width)
}

func stageTranscriptForTest(transcript *Transcript, width int) {
	frame := &transcriptTestFrame{transcript: transcript, width: width}
	NewRoot(frame).Draw(grid.NewSurface(max(width, 1), 1).View())
}

type testWidget func(Frame)

func (draw testWidget) Draw(frame Frame) { draw(frame) }

func stageScrollForTest(scroll *Scroll, total, window int) {
	widget := testWidget(func(frame Frame) {
		scroll.Stage(frame, total, window)
	})
	NewRoot(widget).Draw(grid.NewSurface(1, max(window, 1)).View())
}
