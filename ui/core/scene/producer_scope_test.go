package scene_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestInterruptedPaintRevokesItsCellsBeforeTheOwnerResumes(t *testing.T) {
	for _, mode := range []string{"panic", "exit"} {
		t.Run(mode, func(t *testing.T) {
			var retained grid.View
			r := root(t, scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
				retained = frame.View
				if mode == "exit" {
					runtime.Goexit()
				}
				panic("paint stopped")
			}}))
			surface := grid.NewSurface(1, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					got := recover()
					if mode == "panic" && got != "paint stopped" || mode == "exit" && got != nil {
						t.Errorf("paint exit: %v", got)
					}
				}()
				_, _ = r.Prepare(surface.View())
				t.Error("interrupted paint returned")
			}()
			<-done
			func() {
				defer func() {
					err, _ := recover().(error)
					if !errors.Is(err, scene.ErrInvalid) {
						t.Errorf("expired writer: %v", err)
					}
				}()
				retained.Text(0, 0, "X", grid.Style{})
			}()
			if r.Progress().LayoutRevision != 0 || surface.Rows()[0] != "" {
				t.Fatal("aborted paint altered the settled output")
			}
		})
	}
}
