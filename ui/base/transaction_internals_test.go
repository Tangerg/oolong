package base

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type transcriptTestFrame struct {
	transcript *Transcript
	width      int
}

func (f *transcriptTestFrame) Draw(frame scene.Frame) {
	f.transcript.Stage(frame, f.width)
}

func stageTranscriptForTest(transcript *Transcript, width int) {
	frame := &transcriptTestFrame{transcript: transcript, width: width}
	newTestRoot(frame).Draw(grid.NewSurface(max(width, 1), 1).View())
}
