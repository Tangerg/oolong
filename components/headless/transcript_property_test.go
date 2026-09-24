package headless_test

import (
	"math/rand/v2"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

// A transcript is the one numbering a scroll offset, the ends of a selection, a
// search match and a pinned prompt all speak. They only mean the same thing to each
// other while the numbering answers the same way whichever question is asked of it,
// and the questions are asked by four different owners — so the answers are checked
// against each other rather than against what any one of them expects.

func blockCount(n int) headless.BlockID {
	if n < 0 {
		panic("negative block count")
	}
	return headless.BlockID(n)
}

// assertOneNumbering holds the transcript's three views of its own rows to each
// other: what each block says its extent is, what each row says it belongs to, and
// how tall the whole thing is.
func assertOneNumbering(t *testing.T, tr *headless.Transcript) {
	t.Helper()
	first, total := tr.FirstBlock(), 0
	at := tr.StartRow()
	for i := range tr.Len() {
		id := first + blockCount(i)
		top, height, ok := tr.Extent(id)
		if !ok {
			t.Fatalf("block %d of %d has no extent", i, tr.Len())
		}
		if top != at {
			t.Fatalf("block %d starts at row %d, and the block before it ended at %d", i, top, at)
		}
		for offset := range height {
			row := top + offset
			gotID, gotOffset, ok := tr.At(row)
			if !ok || gotID != id || gotOffset != offset {
				t.Fatalf("row %d belongs to block %d offset %d, but block %d says it is its row %d",
					row, gotID, gotOffset, id, offset)
			}
		}
		at, total = at+height, total+height
	}
	if at != tr.EndRow() {
		t.Fatalf("the blocks end at row %d and the transcript ends at %d", at, tr.EndRow())
	}
	if total != tr.Height() {
		t.Fatalf("the blocks are %d rows and the transcript is %d", total, tr.Height())
	}
	if _, _, ok := tr.At(tr.EndRow()); ok {
		t.Fatalf("row %d is past the end and belongs to a block anyway", tr.EndRow())
	}
	// Every row of every block, so the answer is every block with rows in it. A
	// block of no rows is touched by nothing and is outside it at either end.
	if tr.Height() > 0 {
		wantFirst, wantLast := first, first+blockCount(tr.Len())
		for wantFirst < wantLast {
			if _, height, _ := tr.Extent(wantFirst); height > 0 {
				break
			}
			wantFirst++
		}
		for ; wantLast > wantFirst; wantLast-- {
			if _, height, _ := tr.Extent(wantLast - 1); height > 0 {
				break
			}
		}
		gotFirst, gotLast := tr.Visible(tr.StartRow(), tr.Height())
		if gotFirst != wantFirst || gotLast != wantLast {
			t.Fatalf("every row shows blocks [%d,%d), want [%d,%d)",
				gotFirst, gotLast, wantFirst, wantLast)
		}
	}
}

func TestATranscriptAnswersEveryQuestionFromOneNumbering(t *testing.T) {
	// Retention and a change of width are the two operations that move the numbering
	// rather than extend it, so a run of this that reached neither would be checking
	// the easy half.
	retained, rewidened := 0, 0
	for run := range 60 {
		random := rand.New(rand.NewPCG(uint64(run), 0x7472616e73)) //nolint:gosec // reproducible fixtures
		tr := &headless.Transcript{}
		width := 1 + random.IntN(40)
		stageTranscript(tr, width)
		var live []*block

		for step := range 30 {
			switch random.IntN(8) {
			case 0, 1, 2:
				b := &block{name: "b", lines: random.IntN(4)}
				live = append(live, b)
				tr.Append(b)
			case 3:
				if len(live) > 0 {
					// A streaming answer growing: the last block gets taller.
					last := live[len(live)-1]
					last.lines++
					tr.Changed(tr.FirstBlock() + blockCount(len(live)-1))
				}
			case 4:
				if len(live) > 0 {
					tr.Finish(tr.FirstBlock())
				}
			case 5:
				// Retention hands finished blocks to the terminal's own output.
				released := tr.Commit(func(headless.Block, int) bool { return true })
				live = live[min(released, len(live)):]
				retained += released
			case 6:
				if next := 1 + random.IntN(40); next != width {
					width, rewidened = next, rewidened+1
				}
			default:
			}
			stageTranscript(tr, width)
			if got := tr.Len(); got != len(live) {
				t.Fatalf("run %d step %d: the transcript holds %d blocks and the test %d",
					run, step, got, len(live))
			}
			assertOneNumbering(t, tr)
		}
	}
	if retained == 0 || rewidened == 0 {
		t.Fatalf("released %d blocks and changed width %d times", retained, rewidened)
	}
}
