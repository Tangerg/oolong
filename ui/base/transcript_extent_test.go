package base_test

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestTranscriptUsesItsLastRepresentableRow(t *testing.T) {
	transcript := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, transcript, &measuredTranscriptBlock{rows: math.MaxInt - 1})
	last := transcriptAdd(t, transcript, &measuredTranscriptBlock{rows: 1})
	root := transcriptScene(t, transcript)
	transcriptCommit(t, root, 20, 1, 1)
	if transcript.Height() != math.MaxInt || transcript.EndRow() != math.MaxInt {
		t.Fatal("last representable row refused")
	}
	if id, offset, ok := transcript.At(math.MaxInt - 1); !ok || id != last.ID() || offset != 0 {
		t.Fatalf("last row=%d,%d,%v", id, offset, ok)
	}
	empty := transcriptAdd(t, transcript, &measuredTranscriptBlock{})
	transcriptCommit(t, root, 20, 1, 2)
	if top, rows, ok := transcript.Extent(empty.ID()); !ok || top != math.MaxInt || rows != 0 {
		t.Fatalf("empty extent=%d,%d,%v", top, rows, ok)
	}
}
