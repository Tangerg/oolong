package headless_test

import (
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
)

type measuredTranscriptBlock struct {
	rows    int
	measure func()
}

func (b *measuredTranscriptBlock) HeightForWidth(int) int {
	if b.measure != nil {
		b.measure()
	}
	return b.rows
}

func (*measuredTranscriptBlock) Draw(grid.View) {}

func TestTranscriptRemeasurementPublishesOneCompleteRowSpace(t *testing.T) {
	for _, exit := range []string{"panic", "Goexit"} {
		t.Run(exit, func(t *testing.T) {
			var transcript headless.Transcript
			stageTranscript(&transcript, 20)
			first := &measuredTranscriptBlock{rows: 1}
			second := &measuredTranscriptBlock{rows: 1}
			firstID := transcript.Append(first)
			secondID := transcript.Append(second)
			first.rows = 3
			second.measure = func() {
				if exit == "Goexit" {
					runtime.Goexit()
				}
				panic("measurement interrupted")
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if got := recover(); (exit == "panic" && got != "measurement interrupted") || (exit == "Goexit" && got != nil) {
						t.Errorf("measurement exit = %v", got)
					}
				}()
				transcript.Changed(firstID)
				t.Error("interrupted measurement returned")
			}()
			<-done
			if top, height, ok := transcript.Extent(firstID); !ok || top != 0 || height != 1 {
				t.Errorf("failed measurement changed the first extent: %d %d %v", top, height, ok)
			}
			if top, height, ok := transcript.Extent(secondID); !ok || top != 1 || height != 1 || transcript.Height() != 2 {
				t.Errorf("failed measurement changed the complete row space: %d %d %v total=%d", top, height, ok, transcript.Height())
			}
			second.measure = nil
			transcript.Changed(firstID)
			if top, height, ok := transcript.Extent(secondID); !ok || top != 3 || height != 1 || transcript.Height() != 4 {
				t.Fatalf("replacement measurement did not settle: %d %d %v total=%d", top, height, ok, transcript.Height())
			}
		})
	}
}

func TestRecoveredTranscriptMeasurementCannotCommitAnEmptyLayout(t *testing.T) {
	var transcript headless.Transcript
	stageTranscript(&transcript, 20)
	block := &measuredTranscriptBlock{rows: 2}
	id := transcript.Append(block)
	block.measure = func() { panic("measurement interrupted") }
	root := headless.NewRoot(transcriptFrameAction(func(frame headless.Frame) {
		defer func() {
			if got := recover(); got != "measurement interrupted" {
				t.Errorf("measurement exit = %v", got)
			}
		}()
		transcript.Stage(frame, 10)
	}))
	root.Draw(grid.NewSurface(10, 2).View())
	if transcript.Width() != 20 || transcript.Height() != 2 || transcript.Len() != 1 || transcript.Block(id) != block {
		t.Fatal("recovering a failed measurement committed an empty transcript")
	}
	block.measure = nil
	stageTranscript(&transcript, 10)
	if transcript.Width() != 10 || transcript.Height() != 2 {
		t.Fatal("failed measurement prevented later valid placement")
	}
}
