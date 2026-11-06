package grid_test

import (
	"bytes"
	"errors"
	"image"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/term"
)

func TestInlineRefusesPublicationAfterDestinationFailure(t *testing.T) {
	canvas := grid.NewInline(10, 2)
	canvas.Frame().Text(0, 0, "initial", grid.Style{})
	if err := canvas.Flush(failing{}); !errors.Is(err, errNope) {
		t.Fatal(err)
	}
	frozen := captureInline(t, canvas, content(1, lines("later")))
	candidate, err := canvas.Prepare(frozen)
	if candidate != nil || !errors.Is(err, errNope) {
		t.Fatalf("failed session accepted candidate=%v err=%v", candidate, err)
	}
}

func TestCaptureSettlesAllRowsTogether(t *testing.T) {
	for _, failure := range []string{"unsupported", "panic", "Goexit"} {
		t.Run(failure, func(t *testing.T) {
			canvas := grid.NewInline(30, 2)
			before := captureInline(t, canvas, content(1, lines("kept")))
			var result grid.Frozen
			var err error
			var panicked any
			returned := false
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() { panicked = recover() }()
				result, err = grid.Capture(grid.CaptureConfig{Width: 30, Content: content(2, func(view grid.View) {
					view.Text(0, 0, "unaccepted", grid.Style{})
					view.Text(0, 1, "partial", grid.Style{})
					switch failure {
					case "unsupported":
						view.Paint(grid.Area(0, 1, 1, 1), 1, publicationCallbackPainter{callback: func() { t.Error("refused native painter ran") }})
					case "panic":
						panic("interrupted")
					case "Goexit":
						runtime.Goexit()
					}
				})})
				returned = true
			}()
			<-done
			if result.RowCount() != 0 {
				t.Fatal("failed capture returned partial cells")
			}
			switch failure {
			case "unsupported":
				if !returned || !errors.Is(err, errors.ErrUnsupported) {
					t.Fatalf("unsupported=%v returned=%v", err, returned)
				}
			case "panic":
				if returned || panicked != "interrupted" {
					t.Fatalf("panic=%v returned=%v", panicked, returned)
				}
			case "Goexit":
				if returned || panicked != nil {
					t.Fatalf("Goexit=%v returned=%v", panicked, returned)
				}
			}
			after := captureInline(t, canvas, content(1, lines("after")))
			output := inline(t, canvas, grid.Cursor{}, nil, before, after)
			if !strings.Contains(output, "kept") || !strings.Contains(output, "after") || strings.Contains(output, "unaccepted") {
				t.Fatalf("failed capture changed accepted output: %q", output)
			}
		})
	}
}

func TestInlineCandidateAbortPreservesBothAnchorAndLiveBaseline(t *testing.T) {
	canvas := grid.NewInline(20, 3)
	inline(t, canvas, grid.Cursor{}, lines("prompt"))
	frozen := captureInline(t, canvas, content(1, lines("published")))
	canvas.Frame().Text(0, 0, "prompt", grid.Style{})
	candidate, err := canvas.Prepare(frozen)
	if err != nil {
		t.Fatal(err)
	}
	original := candidate.Bytes()
	mutated := candidate.Bytes()
	mutated[0] = 'x'
	if !bytes.Equal(candidate.Bytes(), original) {
		t.Fatal("candidate leaked writer payload")
	}
	rejectsPublicationReentry(t, func() { canvas.Frame() })
	rejectsPublicationReentry(t, func() { canvas.Resize(10, 3) })
	candidate.Abort()
	retry, err := canvas.Prepare(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retry.Bytes(), original) {
		t.Fatal("aborted candidate advanced inline anchor or diff baseline")
	}
	retry.Commit()
	if output := inline(t, canvas, grid.Cursor{}, lines("prompt")); output != "" {
		t.Fatalf("settled frame republished content: %q", output)
	}
}

func TestInlineUnrepresentableWidthRefusesBeforeOutput(t *testing.T) {
	canvas := grid.NewInline(4, 2)
	frozen := captureInline(t, canvas, content(1, lines("abcd")))
	canvas.Resize(0, 2)
	var output bytes.Buffer
	if err := canvas.Flush(&output, frozen); err == nil || output.Len() != 0 {
		t.Fatalf("zero width=%v output=%q", err, output.String())
	}
	canvas.Resize(2, 2)
	if err := canvas.Flush(&output, frozen); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "ab") || !strings.Contains(output.String(), "cd") {
		t.Fatal("refused projection lost immutable content")
	}
}

func TestSparseInlineGroupCannotAdvanceOnWriterBudgetRefusal(t *testing.T) {
	canvas := grid.NewInline(100, 40)
	inline(t, canvas, grid.Cursor{}, lines("accepted"))
	canvas.Frame().Text(99, 39, "X", grid.Style{})
	candidate, err := canvas.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Abort()
	writer := term.NewWriter(term.WriterConfig{
		Destination: io.Discard, MaxFrameBytes: candidate.Len() - 1,
	})
	t.Cleanup(func() {
		if closeErr := writer.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	if sequence, queueErr := writer.Queue(candidate.Bytes()); sequence != 0 || !errors.Is(queueErr, term.ErrPayloadTooLarge) {
		t.Fatalf("oversized group admission = (%d, %v)", sequence, queueErr)
	}
	candidate.Abort()
	if got := inline(t, canvas, grid.Cursor{}, lines("accepted")); got != "" {
		t.Fatalf("refused staging changed the accepted anchor or cells: %q", got)
	}
}

func TestInvalidInlineViewportRefusesFramesAndFinishWithoutMovingTheAnchor(t *testing.T) {
	for _, size := range []image.Point{image.Pt(0, 2), image.Pt(4, 0)} {
		t.Run(size.String(), func(t *testing.T) {
			canvas := grid.NewInline(4, 2)
			inline(t, canvas, grid.Cursor{}, lines("old", "tail"))
			canvas.Resize(size.X, size.Y)
			var output bytes.Buffer
			if err := canvas.Flush(&output); !errors.Is(err, grid.ErrInvalidTerminalSize) || output.Len() != 0 {
				t.Fatalf("invalid frame = %v, output %q", err, output.String())
			}
			if err := canvas.Finish(&output); !errors.Is(err, grid.ErrInvalidTerminalSize) || output.Len() != 0 {
				t.Fatalf("invalid finish = %v, output %q", err, output.String())
			}
			canvas.Resize(4, 2)
			got := inline(t, canvas, grid.Cursor{}, lines("new"))
			control := grid.NewInline(4, 2)
			inline(t, control, grid.Cursor{}, lines("old", "tail"))
			control.Invalidate()
			want := inline(t, control, grid.Cursor{}, lines("new"))
			if got != want {
				t.Fatalf("invalid geometry advanced anchor: %q, want %q", got, want)
			}
		})
	}
}
