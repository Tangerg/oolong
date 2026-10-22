package ptytest_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/ptytest"
)

func TestInlineFinishPreservesPendingContinuationAndDeliveredRows(t *testing.T) {
	canvas := grid.NewInline(4, 2)
	screen, err := ptytest.NewScreen(ptytest.Size{Cols: 4, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := canvas.Append(func(view grid.View) bool { inlineRow("ab").Draw(view); return false }); err != nil {
		t.Fatal(err)
	}
	canvas.Frame().Text(0, 0, ">", grid.Style{})
	flushInlineToScreen(t, canvas, screen)
	if err := canvas.Append(func(view grid.View) bool { inlineRow("cd").Draw(view); return false }); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := canvas.Finish(&output); err == nil || output.Len() != 0 {
		t.Fatalf("finish with unflushed continuation = %v, %q", err, output.String())
	}
	canvas.Frame().Text(0, 0, ">", grid.Style{})
	flushInlineToScreen(t, canvas, screen)
	if err := canvas.Finish(&output); err != nil {
		t.Fatal(err)
	}
	if err := screen.Apply(output.Bytes()); err != nil {
		t.Fatal(err)
	}
	canvas.Frame().Text(0, 0, ">", grid.Style{})
	flushInlineToScreen(t, canvas, screen)
	if got, want := screen.Rows(), []string{"abcd", ">   ", ">   ", "    ", "    "}; !slices.Equal(got, want) {
		t.Fatalf("rows after finish and resume = %q, want %q", got, want)
	}
}
