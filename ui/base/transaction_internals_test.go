package base

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type transcriptTestFrame struct {
	transcript *Transcript
	width      int
}

func (f *transcriptTestFrame) Element() scene.Element {
	element := f.transcript.Element()
	if !element.Alive() {
		if err := element.SetPaint(f.Draw); err != nil {
			panic(err)
		}
	}
	return element
}

func (f *transcriptTestFrame) Draw(frame scene.Frame) {
	f.transcript.Stage(frame, f.width)
}

func stageTranscriptForTest(transcript *Transcript, width int) {
	frame := &transcriptTestFrame{transcript: transcript, width: width}
	root := newTestRoot(frame)
	defer root.root.Close()
	root.Draw(grid.NewSurface(max(width, 1), 1).View())
}
