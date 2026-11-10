package program

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

func TestRepaintReplyMutationUsesTheNormalFrameSchedule(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			value := "before"
			element := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { frame.Text(0, 0, value, grid.Style{}) }})
			root, rootErr := scene.NewRoot(scene.RootConfig{Element: element})
			if rootErr != nil {
				t.Fatal(rootErr)
			}
			writer := &admissionWriter{changes: make(chan struct{}, 1)}
			p := &program{root: root, canvas: grid.NewScreen(10, 1), writer: writer, tasks: newTaskQueue(), frameInterval: time.Second}
			t.Cleanup(p.stop)
			if err := p.answerRepaint(&repaintRequest{reply: func(_, _ int, repaintErr error) {
				if repaintErr != nil {
					t.Fatal(repaintErr)
				}
				if mutate {
					value = "after"
					element.Invalidate()
				}
			}}); err != nil {
				t.Fatal(err)
			}
			if err := p.draw(); err != nil || len(writer.frames) != 1 {
				t.Fatalf("reply bypassed the frame interval: frames=%d err=%v", len(writer.frames), err)
			}
			time.Sleep(time.Second)
			if err := p.draw(); err != nil {
				t.Fatal(err)
			}
			wantFrames, wantText := 1, "before    "
			if mutate {
				wantFrames, wantText = 2, "after     "
			}
			shown, screenErr := ptytest.NewScreen(ptytest.Size{Cols: 10, Rows: 1})
			if screenErr != nil {
				t.Fatal(screenErr)
			}
			for _, frame := range writer.frames {
				if err := shown.Apply(frame); err != nil {
					t.Fatal(err)
				}
			}
			if err := shown.Flush(); err != nil {
				t.Fatal(err)
			}
			if len(writer.frames) != wantFrames || shown.Rows()[0] != wantText {
				t.Fatalf("mutate=%t: frames=%d text=%q", mutate, len(writer.frames), shown.Rows()[0])
			}
		})
	}
}
