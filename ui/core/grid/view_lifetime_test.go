package grid_test

import (
	"bytes"
	"io"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func viewMutations(view grid.View) map[string]func() {
	return map[string]func(){
		"Text":        func() { view.Text(0, 0, "changed", grid.Style{}) },
		"Fill":        func() { view.Fill(view.Bounds(), grid.Style{}) },
		"MergeStyle":  func() { view.MergeStyle(0, 0, grid.Style{Attr: grid.Bold}) },
		"Link":        func() { view.Link(0, 0, 1, "https://example.test") },
		"Blend":       func() { view.Blend(view.Bounds(), grid.RGBColor(1, 2, 3), 0.5) },
		"Fade":        func() { view.Fade(view.Bounds(), 0.5) },
		"PlaceCursor": func() { view.PlaceCursor(0, 0, grid.CursorStyle{}) },
		"Paint":       func() { view.Paint(grid.Area(0, 0, 1, 1), 1, publicationCallbackPainter{callback: func() {}}) },
	}
}

func TestCanvasRetainedViewsCannotMutateDuringPublication(t *testing.T) {
	for _, mode := range []string{"screen", "inline"} {
		for _, source := range []string{"writer", "painter"} {
			for name := range viewMutations(grid.View{}) {
				t.Run(mode+"/"+source+"/"+name, func(t *testing.T) {
					var canvas regionCanvas = grid.NewScreen(10, 2)
					if mode == "inline" {
						canvas = grid.NewInline(10, 2)
					}
					view := canvas.Frame().Sub(grid.Area(0, 0, 10, 2))
					view.Text(0, 0, "before", grid.Style{})
					callback := func() { rejectsPublicationReentry(t, viewMutations(view)[name]) }
					writer := io.Discard
					if source == "painter" {
						view.Paint(grid.Area(7, 0, 1, 1), 1, publicationCallbackPainter{callback: callback})
					} else {
						writer = publicationCallbackWriter{callback: callback}
					}
					if err := flushCanvas(canvas, writer); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

func TestCanvasFrameViewsEndAtOwnershipTransfer(t *testing.T) {
	for _, mode := range []string{"screen", "inline"} {
		for _, end := range []string{"Frame", "Resize", "Flush", "Finish"} {
			if mode == "screen" && end == "Finish" {
				continue
			}
			t.Run(mode+"/"+end, func(t *testing.T) {
				var canvas interface {
					regionCanvas
					Resize(w, h int)
				} = grid.NewScreen(10, 2)
				if mode == "inline" {
					canvas = grid.NewInline(10, 2)
				}
				view := canvas.Frame()
				view.Text(0, 0, "before", grid.Style{})
				switch end {
				case "Frame":
					canvas.Frame()
				case "Resize":
					canvas.Resize(12, 3)
				case "Flush":
					if err := flushCanvas(canvas, io.Discard); err != nil {
						t.Fatal(err)
					}
				case "Finish":
					if err := canvas.(*grid.Inline).Finish(io.Discard); err != nil {
						t.Fatal(err)
					}
				}
				for _, mutate := range viewMutations(view) {
					rejectsPublicationReentry(t, mutate)
				}
				canvas.Frame().Text(0, 0, "after", grid.Style{})
				var out bytes.Buffer
				if err := flushCanvas(canvas, &out); err != nil || out.Len() == 0 {
					t.Fatalf("replacement frame failed: %v %q", err, out.String())
				}
			})
		}
	}
}

func TestCaptureDrawingViewsEndWithTheirOperation(t *testing.T) {
	for _, exit := range []string{"return", "panic", "Goexit"} {
		t.Run(exit, func(t *testing.T) {
			var retained grid.View
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if got := recover(); (exit == "panic" && got != "drawing interrupted") || (exit != "panic" && got != nil) {
						t.Errorf("drawing exit=%v", got)
					}
				}()
				_, err := grid.Capture(grid.CaptureConfig{Width: 10, Content: content(1, func(view grid.View) {
					retained = view.Sub(view.Bounds())
					view.Text(0, 0, "drawn", grid.Style{})
					if exit == "panic" {
						panic("drawing interrupted")
					}
					if exit == "Goexit" {
						runtime.Goexit()
					}
				})})
				if err != nil || exit != "return" {
					t.Errorf("capture returned after %s: %v", exit, err)
				}
			}()
			<-done
			for _, mutate := range viewMutations(retained) {
				rejectsPublicationReentry(t, mutate)
			}
		})
	}
}

func TestCaptureCannotReuseAnEarlierCapturesView(t *testing.T) {
	var previous grid.View
	if _, err := grid.Capture(grid.CaptureConfig{Width: 10, Content: content(1, func(view grid.View) { previous = view; view.Text(0, 0, "first", grid.Style{}) })}); err != nil {
		t.Fatal(err)
	}
	if _, err := grid.Capture(grid.CaptureConfig{Width: 10, Content: content(1, func(view grid.View) {
		rejectsPublicationReentry(t, func() { previous.Text(0, 0, "stale", grid.Style{}) })
		view.Text(0, 0, "current", grid.Style{})
	})}); err != nil {
		t.Fatal(err)
	}
}
