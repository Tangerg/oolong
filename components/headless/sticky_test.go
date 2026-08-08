package headless_test

import (
	"fmt"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

// pinnable builds a transcript of alternating prompts and answers, and the sticky
// that pins the prompts.
func pinnable(t *testing.T, pairs int) (*headless.Transcript, *headless.Sticky) {
	t.Helper()
	// A short prompt and an answer long enough to scroll it off.
	const prompt, answer = 2, 10
	var tr headless.Transcript
	tr.Resize(40)
	s := &headless.Sticky{Gap: 1}
	for i := range pairs {
		id := tr.Append(&block{name: fmt.Sprintf("prompt%d", i), lines: prompt})
		s.Add(id)
		tr.Append(&block{name: fmt.Sprintf("answer%d", i), lines: answer})
	}
	return &tr, s
}

// TestNothingIsPinnedWhileItIsStillOnScreen. A header repeating something visible two
// rows below is noise, and the moment it stops being visible is exactly the moment it
// starts being worth showing.
func TestNothingIsPinnedWhileItIsStillOnScreen(t *testing.T) {
	tr, s := pinnable(t, 2)
	if _, ok := s.At(tr.Layout(), 0, 8); ok {
		t.Error("a prompt at the top of the view was pinned as well")
	}
	if _, ok := s.At(tr.Layout(), 2, 8); !ok {
		t.Error("a prompt scrolled off the top was not pinned")
	}
}

func TestPinnedFootprintSaturatesInsteadOfTurningNegative(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	var transcript headless.Transcript
	transcript.Resize(40)
	id := transcript.Append(&block{lines: maxInt})
	sticky := headless.Sticky{Gap: maxInt}
	sticky.Add(id)
	pinned, ok := sticky.At(transcript.Layout(), 1, 1)
	if !ok || pinned.Rows != maxInt {
		t.Fatalf("pinned = %+v, %t; want saturated footprint", pinned, ok)
	}
}

func TestThePinnedBlockIsTheOneAbove(t *testing.T) {
	tr, s := pinnable(t, 3)
	// Blocks: prompt0 at 0, answer0 at 2, prompt1 at 12, answer1 at 14, prompt2 at 24.
	for _, tc := range []struct {
		from int
		want headless.BlockID
	}{
		{from: 3, want: 0},
		{from: 11, want: 0},
		{from: 13, want: 2},
		{from: 25, want: 4},
	} {
		p, ok := s.At(tr.Layout(), tc.from, 8)
		if !ok {
			t.Errorf("from row %d nothing was pinned", tc.from)
			continue
		}
		if p.Block != tc.want {
			t.Errorf("from row %d block %d is pinned, want %d", tc.from, p.Block, tc.want)
		}
	}
}

// TestTheNextPromptPushesTheHeaderOff, so the change reads as one thing replacing
// another rather than two things flickering.
func TestTheNextPromptPushesTheHeaderOff(t *testing.T) {
	tr, s := pinnable(t, 2)
	// prompt1 begins at row 12, and the header's footprint is two rows plus a gap.

	full, ok := s.At(tr.Layout(), 4, 8)
	if !ok || full.ClipTop != 0 || full.Rows != 3 {
		t.Fatalf("well clear of the next prompt: %+v", full)
	}
	if full.Fade != 1 {
		t.Errorf("a header sitting still has fade %v, want 1", full.Fade)
	}

	// Close enough that the next prompt is inside the footprint.
	pushed, ok := s.At(tr.Layout(), 10, 8)
	if !ok {
		t.Fatal("nothing pinned while being pushed")
	}
	if pushed.ClipTop == 0 {
		t.Errorf("the header is not being clipped: %+v", pushed)
	}
	if pushed.Fade >= 1 {
		t.Errorf("a header being pushed has fade %v, want less than one", pushed.Fade)
	}
	if pushed.Visible() >= full.Visible() {
		t.Errorf("being pushed showed %d rows, want fewer than %d", pushed.Visible(), full.Visible())
	}
}

func TestTheHeaderGoesWhenItIsFullyPushed(t *testing.T) {
	tr, s := pinnable(t, 2)
	// Row 12 is where prompt1 begins, so at that point it is on screen itself.
	if _, ok := s.At(tr.Layout(), 12, 8); ok {
		t.Error("the old header survived the new prompt arriving")
	}
}

// TestATallPromptCollapses, because a header pinned in full eats the view it was meant
// to give context to.
func TestATallPromptCollapses(t *testing.T) {
	tr := &headless.Transcript{}
	tr.Resize(40)
	s := &headless.Sticky{MinHeight: 2, Gap: 1}
	s.SetBlocks([]headless.BlockID{0})
	tr.Append(&block{name: "tall", lines: 6})
	tr.Append(&block{name: "answer", lines: 30})

	heights := make([]int, 0, 6)
	for from := 1; from <= 6; from++ {
		p, ok := s.At(tr.Layout(), from, 10)
		if !ok {
			t.Fatalf("nothing pinned from row %d", from)
		}
		heights = append(heights, p.Height)
	}
	if heights[0] <= heights[len(heights)-1] {
		t.Errorf("heights %v, want them shrinking as the prompt scrolls away", heights)
	}
	if got := heights[len(heights)-1]; got != 2 {
		t.Errorf("it collapsed to %d, want the floor of 2", got)
	}
}

func TestAPromptThatDoesNotCollapseKeepsItsHeight(t *testing.T) {
	tr := &headless.Transcript{}
	tr.Resize(40)
	s := &headless.Sticky{}
	s.SetBlocks([]headless.BlockID{0})
	tr.Append(&block{name: "tall", lines: 6})
	tr.Append(&block{name: "answer", lines: 30})

	p, ok := s.At(tr.Layout(), 5, 10)
	if !ok {
		t.Fatal("nothing pinned")
	}
	if p.Height != 6 {
		t.Errorf("height = %d, want the whole prompt", p.Height)
	}
}

func TestStickyAnswersNothingWhenThereIsNothingToAnswer(t *testing.T) {
	tr, s := pinnable(t, 1)
	for _, tc := range []struct {
		name string
		tr   *headless.Transcript
		s    *headless.Sticky
		from int
		rows int
	}{
		{name: "no transcript", tr: nil, s: s, from: 4, rows: 8},
		{name: "no rows", tr: tr, s: s, from: 4, rows: 0},
		{name: "nothing pinnable", tr: tr, s: &headless.Sticky{}, from: 4, rows: 8},
		{name: "above everything", tr: tr, s: s, from: -5, rows: 8},
		{name: "past everything", tr: tr, s: s, from: 500, rows: 8},
	} {
		if _, ok := tc.s.At(tc.tr.Layout(), tc.from, tc.rows); ok {
			t.Errorf("%s: something was pinned", tc.name)
		}
	}
}

func TestStickyIgnoresBlocksThatAreNotThere(t *testing.T) {
	tr := &headless.Transcript{}
	tr.Resize(40)
	tr.Append(&block{name: "only", lines: 2})
	tr.Append(&block{name: "answer", lines: 20})
	s := &headless.Sticky{}
	s.SetBlocks([]headless.BlockID{0, 99})

	p, ok := s.At(tr.Layout(), 5, 8)
	if !ok || p.Block != 0 {
		t.Errorf("pinned %+v (%v), want block 0", p, ok)
	}
}

func TestAPinnedBlockOfNoHeightIsNotPinned(t *testing.T) {
	tr := &headless.Transcript{}
	tr.Resize(40)
	tr.Append(&block{name: "first", lines: 2})
	tr.Append(&block{name: "nothing", lines: 0})
	tr.Append(&block{name: "answer", lines: 20})
	s := &headless.Sticky{}
	s.SetBlocks([]headless.BlockID{1})

	if _, ok := s.At(tr.Layout(), 6, 8); ok {
		t.Error("a block with no rows was pinned")
	}
}

func TestStickyOwnsItsBlockIdentities(t *testing.T) {
	given := []headless.BlockID{1, 2, 3}
	var s headless.Sticky
	s.SetBlocks(given)

	given[1] = 99
	s.DiscardBefore(2)
	if got := s.Blocks(); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("blocks after discard = %v, want [2 3]", got)
	}
	if given[0] != 1 {
		t.Fatalf("discard cleared caller-owned input: %v", given)
	}

	got := s.Blocks()
	got[0] = 88
	if kept := s.Blocks(); kept[0] != 2 {
		t.Fatalf("mutating snapshot changed sticky state: %v", kept)
	}
}
