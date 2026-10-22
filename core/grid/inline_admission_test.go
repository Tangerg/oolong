package grid_test

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

func TestInlineRefusesPublicationAfterDestinationFailure(t *testing.T) {
	i := grid.NewInline(10, 2)
	i.Frame().Text(0, 0, "initial", grid.Style{})
	if err := i.Flush(failing{}); !errors.Is(err, errNope) {
		t.Fatal(err)
	}
	called := false
	draw := func(grid.View) { called = true }
	if err := i.Print(content(1, draw)); !errors.Is(err, errNope) {
		t.Fatalf("Print refusal = %v", err)
	}
	if err := i.Append(func(view grid.View) bool { draw(view); return false }); !errors.Is(err, errNope) {
		t.Fatalf("Append refusal = %v", err)
	}
	if called {
		t.Fatal("failed publication called another content producer")
	}
}

func TestInlineAppendSettlesAllRowsTogether(t *testing.T) {
	for _, failure := range []string{"unsupported", "no progress", "panic", "Goexit"} {
		t.Run(failure, func(t *testing.T) {
			canvas := grid.NewInline(30, 2)
			if err := canvas.Append(appendLines("kept")); err != nil {
				t.Fatal(err)
			}
			var result error
			var panicked any
			returned, calls := false, 0
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() { panicked = recover() }()
				result = canvas.Append(func(view grid.View) bool {
					calls++
					if calls == 1 {
						view.Text(0, 0, "unaccepted", grid.Style{})
						return true
					}
					switch failure {
					case "unsupported":
						view.Paint(grid.Area(0, 0, 1, 1), 1, publicationCallbackPainter{callback: func() {
							t.Error("a refused painter ran")
						}})
					case "panic":
						panic("interrupted")
					case "Goexit":
						runtime.Goexit()
					}
					return true
				})
				returned = true
			}()
			<-done
			switch failure {
			case "unsupported":
				if !returned || !errors.Is(result, errors.ErrUnsupported) {
					t.Fatalf("unsupported append = %v, returned=%v", result, returned)
				}
			case "no progress":
				if !returned || !errors.Is(result, io.ErrNoProgress) {
					t.Fatalf("empty continuation = %v, returned=%v", result, returned)
				}
			case "panic":
				if returned || panicked != "interrupted" {
					t.Fatalf("panic = %v, returned=%v", panicked, returned)
				}
			case "Goexit":
				if returned || panicked != nil {
					t.Fatalf("Goexit = %v, returned=%v", panicked, returned)
				}
			}
			if column, open := canvas.Tail(); calls != 2 || column != 4 || !open {
				t.Fatalf("unfinished append changed the open row: calls=%d column=%d open=%v", calls, column, open)
			}
			if err := canvas.Append(appendLines("after")); err != nil {
				t.Fatal(err)
			}
			output := inline(t, canvas, grid.Cursor{}, nil)
			if !strings.Contains(output, "kept") || !strings.Contains(output, "after") || strings.Contains(output, "unaccepted") {
				t.Fatalf("unfinished append changed accepted output: %q", output)
			}
		})
	}
}

func TestInlineRefusesNativeRegionsWithoutPublishingPartialCells(t *testing.T) {
	for _, operation := range []string{"Print", "Append"} {
		t.Run(operation, func(t *testing.T) {
			inlineCanvas := grid.NewInline(20, 2)
			if err := inlineCanvas.Append(appendLines("before")); err != nil {
				t.Fatal(err)
			}
			draw := func(view grid.View) {
				view.Text(0, 0, "refused", grid.Style{})
				view.Paint(grid.Area(0, 0, 1, 1), 1, publicationCallbackPainter{callback: func() {
					t.Error("refused scrollback content invoked its native painter")
				}})
			}
			var err error
			if operation == "Print" {
				err = inlineCanvas.Print(content(1, draw))
			} else {
				err = inlineCanvas.Append(func(view grid.View) bool { draw(view); return false })
			}
			if !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("native scrollback admission = %v", err)
			}
			if column, open := inlineCanvas.Tail(); column != 6 || !open {
				t.Errorf("refused content changed the open row: column=%d open=%v", column, open)
			}
			if err := inlineCanvas.Append(appendLines("after")); err != nil {
				t.Fatal(err)
			}
			output := inline(t, inlineCanvas, grid.Cursor{}, nil)
			if !strings.Contains(output, "before") || !strings.Contains(output, "after") || strings.Contains(output, "refused") {
				t.Fatalf("refused content changed accepted output: %q", output)
			}
		})
	}
}
