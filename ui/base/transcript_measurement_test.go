package base_test

import (
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
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
			transcript := transcriptModel(t, base.TranscriptConfig{})
			first, second := &measuredTranscriptBlock{rows: 1}, &measuredTranscriptBlock{rows: 1}
			a, b := transcriptAdd(t, transcript, first), transcriptAdd(t, transcript, second)
			root := transcriptScene(t, transcript)
			transcriptCommit(t, root, 20, 4, 1)
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
					got := recover()
					if (exit == "panic" && got != "measurement interrupted") || (exit == "Goexit" && got != nil) {
						t.Errorf("exit=%v", got)
					}
				}()
				_, _ = root.Prepare(grid.NewSurface(10, 4).View())
				t.Error("interrupted measurement returned")
			}()
			<-done
			if top, rows, ok := transcript.Extent(a.ID()); !ok || top != 0 || rows != 1 {
				t.Fatalf("first extent=%d,%d,%v", top, rows, ok)
			}
			if top, rows, ok := transcript.Extent(b.ID()); !ok || top != 1 || rows != 1 || transcript.Height() != 2 {
				t.Fatalf("second extent=%d,%d,%v", top, rows, ok)
			}
			second.measure = nil
			transcriptCommit(t, root, 10, 4, 2)
			if top, rows, ok := transcript.Extent(b.ID()); !ok || top != 3 || rows != 1 || transcript.Height() != 4 {
				t.Fatalf("settled extent=%d,%d,%v", top, rows, ok)
			}
		})
	}
}

func TestRecoveredTranscriptMeasurementCannotCommitAnEmptyLayout(t *testing.T) {
	transcript := transcriptModel(t, base.TranscriptConfig{})
	block := &measuredTranscriptBlock{rows: 2}
	item := transcriptAdd(t, transcript, block)
	width := 20
	root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		defer func() {
			if got := recover(); got != nil && got != "measurement interrupted" {
				panic(got)
			}
		}()
		transcript.Stage(frame, width)
	}})})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	transcriptCommit(t, root, width, 2, 1)
	block.measure = func() { panic("measurement interrupted") }
	width = 10
	transcriptCommit(t, root, width, 2, 2)
	if transcript.Width() != 20 || transcript.Height() != 2 || transcript.Block(item.ID()) != block {
		t.Fatal("recovered measurement committed an empty layout")
	}
	block.measure = nil
	transcriptCommit(t, root, width, 2, 3)
	if transcript.Width() != 10 || transcript.Height() != 2 {
		t.Fatal("failed measurement prevented later placement")
	}
}
