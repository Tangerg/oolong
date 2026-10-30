package ptytest_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

func TestInlineFinishPreservesAcceptedContributionsAndDeliveredRows(t *testing.T) {
	canvas := grid.NewInline(4, 2)
	screen, err := ptytest.NewScreen(ptytest.Size{Cols: 4, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	first := captureInlineRows(t, 4, inlineRow("ab"))
	canvas.Frame().Text(0, 0, ">", grid.Style{})
	flushInlineToScreen(t, canvas, screen, first)
	second := captureInlineRows(t, 4, inlineRow("cd"))
	canvas.Frame().Text(0, 0, ">", grid.Style{})
	candidate, err := canvas.Prepare(second)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	func() {
		defer func() {
			if recover() == nil {
				t.Error("finish overtook pending frame")
			}
		}()
		_ = canvas.Finish(&output)
	}()
	if output.Len() != 0 {
		t.Fatal("refused finish emitted bytes")
	}
	candidate.Abort()
	flushInlineToScreen(t, canvas, screen, second)
	if err := canvas.Finish(&output); err != nil {
		t.Fatal(err)
	}
	if err := screen.Apply(output.Bytes()); err != nil {
		t.Fatal(err)
	}
	canvas.Frame().Text(0, 0, ">", grid.Style{})
	flushInlineToScreen(t, canvas, screen)
	if got, want := screen.Rows(), []string{"ab  ", "cd  ", ">   ", ">   ", "    "}; !slices.Equal(got, want) {
		t.Fatalf("finished rows=%q want=%q", got, want)
	}
}
