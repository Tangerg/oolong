package grid_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestInlineFinishCannotOvertakeAnUnsettledFrame(t *testing.T) {
	canvas := grid.NewInline(20, 2)
	inline(t, canvas, grid.Cursor{}, lines("live"))
	frozen := captureInline(t, canvas, content(1, lines("accepted")))
	canvas.Frame().Text(0, 0, "live", grid.Style{})
	candidate, err := canvas.Prepare(frozen)
	if err != nil {
		t.Fatal(err)
	}
	rejectsPublicationReentry(t, func() { _, _ = canvas.PrepareFinish() })
	candidate.Abort()
	var output bytes.Buffer
	if err := canvas.Flush(&output, frozen); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "accepted") {
		t.Fatal("refused finish lost caller-owned contribution")
	}
	output.Reset()
	if err := canvas.Finish(&output); err != nil || output.Len() == 0 {
		t.Fatalf("finish=%v output=%q", err, output.String())
	}
}

func TestInlineAbortedFinishLeavesItsAnchorAndCleanupAvailable(t *testing.T) {
	canvas := grid.NewInline(20, 3)
	inline(t, canvas, grid.Cursor{Visible: true}, lines("one", "two"))
	candidate, err := canvas.PrepareFinish()
	if err != nil {
		t.Fatal(err)
	}
	original := candidate.Bytes()
	candidate.Abort()
	retry, err := canvas.PrepareFinish()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retry.Bytes(), original) {
		t.Fatal("aborted finish advanced anchor")
	}
	retry.Commit()
	last, err := canvas.PrepareFinish()
	if err != nil {
		t.Fatal(err)
	}
	if last.Len() != 0 {
		t.Fatal("committed finish emitted cleanup twice")
	}
	last.Commit()
}
