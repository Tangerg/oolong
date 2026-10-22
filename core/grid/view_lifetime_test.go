package grid_test

import (
	"bytes"
	"io"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
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
		for _, source := range []string{"writer", "painter", "Print", "Append"} {
			if mode == "screen" && (source == "Print" || source == "Append") {
				continue
			}
			for name := range viewMutations(grid.View{}) {
				t.Run(mode+"/"+source+"/"+name, func(t *testing.T) {
					var canvas regionCanvas = grid.NewScreen(10, 2)
					if mode == "inline" {
						canvas = grid.NewInline(10, 2)
					}
					view := canvas.Frame().Sub(grid.Area(0, 0, 10, 2))
					view.Text(0, 0, "before", grid.Style{})
					callback := func() { rejectsPublicationReentry(t, viewMutations(view)[name]) }
					switch source {
					case "writer":
						if err := canvas.Flush(publicationCallbackWriter{callback: callback}); err != nil {
							t.Fatal(err)
						}
					case "painter":
						view.Paint(grid.Area(7, 0, 1, 1), 1, publicationCallbackPainter{callback: callback})
						if err := canvas.Flush(io.Discard); err != nil {
							t.Fatal(err)
						}
					case "Print":
						if err := canvas.(*grid.Inline).Print(content(1, func(v grid.View) {
							callback()
							v.Text(0, 0, "printed", grid.Style{})
						})); err != nil {
							t.Fatal(err)
						}
						view.Text(0, 0, "allowed", grid.Style{})
					case "Append":
						if err := canvas.(*grid.Inline).Append(func(v grid.View) bool {
							callback()
							v.Text(0, 0, "appended", grid.Style{})
							return false
						}); err != nil {
							t.Fatal(err)
						}
						view.Text(0, 0, "allowed", grid.Style{})
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
					if err := canvas.Flush(io.Discard); err != nil {
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
				if err := canvas.Flush(&out); err != nil || out.Len() == 0 {
					t.Fatalf("replacement frame failed: %v %q", err, out.String())
				}
			})
		}
	}
}

func TestInlineDrawingViewsEndWithTheirOperation(t *testing.T) {
	for _, operation := range []string{"Print", "Append"} {
		for _, exit := range []string{"return", "panic", "Goexit"} {
			t.Run(operation+"/"+exit, func(t *testing.T) {
				canvas := grid.NewInline(10, 2)
				var retained grid.View
				draw := func(view grid.View) {
					retained = view.Sub(view.Bounds())
					view.Text(0, 0, "drawn", grid.Style{})
					if exit == "panic" {
						panic("drawing interrupted")
					}
					if exit == "Goexit" {
						runtime.Goexit()
					}
				}
				done := make(chan struct{})
				go func() {
					defer close(done)
					defer func() {
						if got := recover(); (exit == "panic" && got != "drawing interrupted") || (exit != "panic" && got != nil) {
							t.Errorf("drawing exit = %v", got)
						}
					}()
					var err error
					if operation == "Print" {
						err = canvas.Print(content(1, draw))
					} else {
						err = canvas.Append(func(view grid.View) bool { draw(view); return false })
					}
					if err != nil || exit != "return" {
						t.Errorf("publication returned after %s: %v", exit, err)
					}
				}()
				<-done
				for _, mutate := range viewMutations(retained) {
					rejectsPublicationReentry(t, mutate)
				}
			})
		}
	}
}

func TestInlineAppendCannotReuseAPreviousRowsView(t *testing.T) {
	canvas := grid.NewInline(10, 2)
	var previous grid.View
	rows := 0
	if err := canvas.Append(func(view grid.View) bool {
		rows++
		if rows == 2 {
			rejectsPublicationReentry(t, func() { previous.Text(0, 0, "stale", grid.Style{}) })
		}
		previous = view
		view.Text(0, 0, "current", grid.Style{})
		return rows < 2
	}); err != nil {
		t.Fatal(err)
	}
}
