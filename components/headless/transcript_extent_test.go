package headless_test

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func TestTranscriptRefusesUnrepresentableRowSpace(t *testing.T) {
	for _, operation := range []string{"append", "change", "stage", "initial stage", "append after publication"} {
		t.Run(operation, func(t *testing.T) {
			var transcript headless.Transcript
			if operation != "initial stage" {
				stageTranscript(&transcript, 20)
			}
			first := &measuredTranscriptBlock{rows: 1}
			firstID := transcript.Append(first)
			secondID := transcript.Append(&measuredTranscriptBlock{rows: 1})
			var apply func()
			switch operation {
			case "append":
				apply = func() { transcript.Append(&measuredTranscriptBlock{rows: math.MaxInt}) }
			case "change":
				first.rows = math.MaxInt
				apply = func() { transcript.Changed(firstID) }
			case "stage", "initial stage":
				first.rows = math.MaxInt
				apply = func() { stageTranscript(&transcript, 40) }
			case "append after publication":
				transcript.Finish(firstID)
				transcript.Finish(secondID)
				transcript.Commit(func(headless.Block, int) bool { return true })
				apply = func() { transcript.Append(&measuredTranscriptBlock{rows: math.MaxInt}) }
			}
			firstLive, count := transcript.FirstBlock(), transcript.Len()
			start, height, end, width := transcript.StartRow(), transcript.Height(), transcript.EndRow(), transcript.Width()
			func() {
				defer func() {
					if recover() == nil {
						t.Error("unrepresentable rows were accepted")
					}
				}()
				apply()
			}()
			if transcript.FirstBlock() != firstLive || transcript.Len() != count || transcript.StartRow() != start ||
				transcript.Height() != height || transcript.EndRow() != end || transcript.Width() != width {
				t.Fatal("refused row space changed the committed transcript")
			}
			if count > 0 {
				wantRows := 1
				if width == 0 {
					wantRows = 0
				}
				for wantTop, id := range []headless.BlockID{firstID, secondID} {
					if top, rows, ok := transcript.Extent(id); !ok || top != wantTop*wantRows || rows != wantRows {
						t.Fatalf("refusal changed block %d: %d, %d, %v", id, top, rows, ok)
					}
				}
			}
			first.rows = 1
			stageTranscript(&transcript, 20)
			end = transcript.EndRow()
			id := transcript.Append(&measuredTranscriptBlock{rows: 1})
			if top, rows, ok := transcript.Extent(id); !ok || top != end || rows != 1 {
				t.Fatalf("refusal prevented a later valid append: %d, %d, %v", top, rows, ok)
			}
		})
	}
}

func TestTranscriptUsesItsLastRepresentableRow(t *testing.T) {
	var transcript headless.Transcript
	stageTranscript(&transcript, 20)
	first := transcript.Append(&measuredTranscriptBlock{rows: math.MaxInt - 1})
	last := transcript.Append(&measuredTranscriptBlock{rows: 1})
	if transcript.Height() != math.MaxInt || transcript.EndRow() != math.MaxInt {
		t.Fatal("the last representable row was refused")
	}
	if id, offset, ok := transcript.At(math.MaxInt - 1); !ok || id != last || offset != 0 {
		t.Fatalf("last row = %d, %d, %v", id, offset, ok)
	}
	transcript.Finish(first)
	transcript.Finish(last)
	if count := transcript.Commit(func(headless.Block, int) bool { return true }); count != 2 {
		t.Fatalf("committed %d blocks, want both", count)
	}
	if transcript.StartRow() != math.MaxInt || transcript.Height() != 0 || transcript.EndRow() != math.MaxInt {
		t.Fatal("publication changed the exhausted row origin")
	}
	empty := transcript.Append(&measuredTranscriptBlock{})
	if top, rows, ok := transcript.Extent(empty); !ok || top != math.MaxInt || rows != 0 {
		t.Fatalf("empty block needs no new coordinate: %d, %d, %v", top, rows, ok)
	}
}
