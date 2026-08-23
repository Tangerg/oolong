package headless_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/text"
)

// block is a test block of a stated height, which can be changed to stand in for a
// streaming answer growing.
//
// Its height follows the width the way real content does — the taller it gets, the
// narrower the space — so that a resize has something to change.
type block struct {
	name    string
	lines   int
	measure int // how many times it was measured
}

func (b *block) Measure(width int) int {
	b.measure++
	if width <= 0 {
		return 0
	}
	// One row per line, plus one more for every line that does not fit the width.
	rows := b.lines
	if width < 10 {
		rows *= 2
	}
	return rows
}

func (b *block) Draw(v grid.View) {
	for y := range b.lines {
		v.Text(0, y, fmt.Sprintf("%s:%d", b.name, y), grid.Style{})
	}
}

// Rows makes the block copyable, which is what a selection needs.
func (b *block) Rows(width int) []text.Row {
	rows := b.Measure(width)
	out := make([]text.Row, rows)
	for y := range rows {
		out[y] = text.Row{Text: fmt.Sprintf("%s:%d", b.name, y)}
	}
	return out
}

// plain is a block that cannot be copied, so a selection across one has to survive
// it without losing count of the rows.
type plain struct{ lines int }

func (p *plain) Measure(int) int  { return p.lines }
func (p *plain) Draw(v grid.View) { v.Text(0, 0, "plain", grid.Style{}) }

func TestTranscriptStacksBlocksInOneCoordinateSpace(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 40)
	a, b, c := &block{name: "a", lines: 3}, &block{name: "b", lines: 1}, &block{name: "c", lines: 4}
	tr.Append(a)
	tr.Append(b)
	tr.Append(c)

	if got := tr.Height(); got != 8 {
		t.Fatalf("total rows = %d, want 8", got)
	}
	for i, want := range []struct{ top, height int }{{0, 3}, {3, 1}, {4, 4}} {
		top, height, ok := tr.Extent(headless.BlockID(i))
		if !ok {
			t.Fatalf("block %d has no extent", i)
		}
		if top != want.top || height != want.height {
			t.Errorf("block %d covers [%d,+%d), want [%d,+%d)", i, top, height, want.top, want.height)
		}
	}
}

func TestTranscriptRowCoordinatesCannotWrap(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	var tr headless.Transcript
	stageTranscript(&tr, 40)
	id := tr.Append(&plain{lines: maxInt})

	if got := tr.Height(); got != maxInt {
		t.Fatalf("height = %d, want %d", got, maxInt)
	}
	if got := tr.EndRow(); got != maxInt {
		t.Fatalf("end row = %d, want %d", got, maxInt)
	}
	if top, height, ok := tr.Extent(id); !ok || top != 0 || height != maxInt {
		t.Fatalf("extent = (%d, %d, %v), want (0, %d, true)", top, height, ok, maxInt)
	}
}

func TestTranscriptFindsTheBlockAtARow(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 40)
	for _, lines := range []int{3, 1, 4} {
		tr.Append(&block{name: "x", lines: lines})
	}

	for _, tc := range []struct {
		row, offset int
		block       headless.BlockID
		ok          bool
	}{
		{row: 0, block: 0, offset: 0, ok: true},
		{row: 2, block: 0, offset: 2, ok: true},
		{row: 3, block: 1, offset: 0, ok: true},
		{row: 4, block: 2, offset: 0, ok: true},
		{row: 7, block: 2, offset: 3, ok: true},
		{row: 8, ok: false},
		{row: -1, ok: false},
	} {
		i, offset, ok := tr.At(tc.row)
		if ok != tc.ok {
			t.Errorf("row %d: ok = %v, want %v", tc.row, ok, tc.ok)
			continue
		}
		if ok && (i != tc.block || offset != tc.offset) {
			t.Errorf("row %d is block %d offset %d, want block %d offset %d",
				tc.row, i, offset, tc.block, tc.offset)
		}
	}
}

// TestTranscriptSkipsBlocksWithNoHeight, because a bisection over tops can land on
// any of a run that shares one, and only one of them owns the row.
func TestTranscriptSkipsBlocksWithNoHeight(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 40)
	tr.Append(&block{name: "first", lines: 2})
	tr.Append(&block{name: "empty", lines: 0})
	tr.Append(&block{name: "also empty", lines: 0})
	tr.Append(&block{name: "last", lines: 2})

	i, offset, ok := tr.At(2)
	if !ok || i != 3 || offset != 0 {
		t.Errorf("row 2 is block %d offset %d (%v), want block 3 offset 0", i, offset, ok)
	}
}

// TestTranscriptOnlyMeasuresWhatMoved is the cost claim. A streaming answer grows one
// token at a time, and re-measuring the whole session for each one is what this
// structure exists to avoid.
func TestTranscriptOnlyMeasuresWhatMoved(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 40)
	var blocks []*block
	var lastID headless.BlockID
	for i := range 50 {
		b := &block{name: fmt.Sprintf("b%d", i), lines: 2}
		blocks = append(blocks, b)
		lastID = tr.Append(b)
	}
	for _, b := range blocks {
		b.measure = 0
	}

	// The last block grows, as a streaming answer does.
	last := blocks[len(blocks)-1]
	last.lines = 5
	tr.Changed(lastID)

	if got := last.measure; got != 1 {
		t.Errorf("the changed block was measured %d times, want 1", got)
	}
	for i, b := range blocks[:len(blocks)-1] {
		if b.measure != 0 {
			t.Errorf("block %d was measured %d times for a change after it", i, b.measure)
		}
	}
	if got := tr.Height(); got != 49*2+5 {
		t.Errorf("total rows = %d, want %d", got, 49*2+5)
	}
}

func TestTranscriptReflowMeasuresEverythingOnce(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 40)
	var blocks []*block
	for i := range 5 {
		b := &block{name: fmt.Sprintf("b%d", i), lines: 2}
		blocks = append(blocks, b)
		tr.Append(b)
	}
	for _, b := range blocks {
		b.measure = 0
	}

	if got := stageTranscript(&tr, 8).Height(); got != 5*4 {
		t.Errorf("rows at the narrower width = %d, want %d", got, 5*4)
	}
	for i, b := range blocks {
		if b.measure != 1 {
			t.Errorf("block %d was measured %d times for one resize", i, b.measure)
		}
	}

	// And the same width again costs nothing, which is what makes it safe to call
	// once a frame.
	for _, b := range blocks {
		b.measure = 0
	}
	stageTranscript(&tr, 8)
	for i, b := range blocks {
		if b.measure != 0 {
			t.Errorf("block %d was measured again for a width it already had", i)
		}
	}
}

func TestTranscriptDrawsOnlyWhatTheWindowTouches(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	for i := range 6 {
		tr.Append(&block{name: fmt.Sprintf("b%d", i), lines: 2})
	}

	// A window of four rows starting at row 5: the second half of b2, b3, b4, and
	// the first half of... row 8 is b4's second row, so b2 b3 b4 only.
	first, last := tr.Visible(5, 4)
	if first != 2 || last != 5 {
		t.Errorf("blocks touching rows 5..8 are [%d,%d), want [2,5)", first, last)
	}
}

// TestTranscriptDrawsAcrossItsOwnEdges: a block the window cuts into is drawn whole
// into a view that starts above the space available, and the rows outside are
// discarded — which is what makes a block not need to know it is partly visible.
func TestTranscriptDrawsAcrossItsOwnEdges(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "a", lines: 4})
	tr.Append(&block{name: "b", lines: 4})

	// Three rows, starting two into the first block.
	s := grid.NewSurface(20, 3)
	drawTranscript(s.View(), 2, &tr)

	want := []string{"a:2", "a:3", "b:0"}
	for y, text := range want {
		if got := rowText(s.View(), y); got != text {
			t.Errorf("row %d = %q, want %q", y, got, text)
		}
	}
}

func TestTranscriptDrawsNothingIntoNoSpace(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "a", lines: 2})

	// A view with nowhere to draw is handed to the blocks not at all, rather than
	// each of them being left to notice. The zero view is the one a container gives
	// a child it had no room for.
	drawTranscript(grid.NewSurface(0, 0).View(), 0, &tr)
	drawTranscript(grid.View{}, 0, &tr)

	// And the transcript itself is unchanged by having been asked.
	if got := tr.Height(); got != 2 {
		t.Errorf("rows = %d after drawing into nothing, want 2", got)
	}
}

func TestTranscriptTextIsTheRowsAndNothingElse(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "a", lines: 2})
	tr.Append(&block{name: "b", lines: 2})

	got := rowTexts(tr.Rows(1, 2))
	want := []string{"a:1", "b:0"}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestTranscriptTextKeepsTheRowsOfBlocksItCannotCopy, because a selection dragged
// across one has to come out with as many lines as the user dragged over.
func TestTranscriptTextKeepsTheRowsOfBlocksItCannotCopy(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "a", lines: 1})
	tr.Append(&plain{lines: 2})
	tr.Append(&block{name: "b", lines: 1})

	got := rowTexts(tr.Rows(0, 4))
	want := []string{"a:0", "", "", "b:0"}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTranscriptTextClampsToWhatExists(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "a", lines: 2})

	if got := tr.Rows(0, 10); len(got) != 2 {
		t.Errorf("asking for ten rows of a two-row transcript gave %d: %q", len(got), rowTexts(got))
	}
	if got := tr.Rows(5, 2); got != nil {
		t.Errorf("asking past the end gave %q", rowTexts(got))
	}
	if got := tr.Rows(0, 0); got != nil {
		t.Errorf("asking for no rows gave %q", rowTexts(got))
	}
}

func TestTranscriptWithoutAWidthMeasuresNothing(t *testing.T) {
	// The zero value is a transcript that has not been told how wide the space is,
	// and a height measured against no width is a number invented for it.
	var tr headless.Transcript
	tr.Append(&block{name: "a", lines: 3})
	if got := tr.Height(); got != 0 {
		t.Errorf("rows = %d before any width was set, want 0", got)
	}
	if got := stageTranscript(&tr, 20).Height(); got != 3 {
		t.Errorf("rows = %d once a width was set, want 3", got)
	}
}

func TestTranscriptIgnoresWhatItCannotHold(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(nil)
	if got := tr.Len(); got != 0 {
		t.Errorf("a nil block was appended anyway: length %d", got)
	}
	if b := tr.Block(0); b != nil {
		t.Errorf("Block(0) of an empty transcript = %v", b)
	}
	if b := tr.Last(); b != nil {
		t.Errorf("Last of an empty transcript = %v", b)
	}
	if _, _, ok := tr.Extent(0); ok {
		t.Error("Extent(0) of an empty transcript reported a range")
	}
	// Out of range on either side changes nothing rather than panicking: a caller
	// holding an index from an earlier frame has no way to know it is stale.
	tr.Changed(^headless.BlockID(0))
	tr.Changed(5)
}

// rowText reads one row of a surface back as a string, trimmed of the blanks that
// padding leaves.
func rowText(v grid.View, y int) string {
	var b strings.Builder
	width, _ := v.Size()
	for x := range width {
		if c := cellAt(v, x, y); c.Content != "" {
			b.WriteString(c.Content)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

func TestTranscriptEdges(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "a", lines: 2})

	if got := tr.Width(); got != 20 {
		t.Errorf("Width = %d, want 20", got)
	}
	if got := tr.Block(0); got == nil {
		t.Error("Block(0) of a transcript with one block is nil")
	}
	if got := tr.Last(); got == nil {
		t.Error("Last of a transcript with one block is nil")
	}

	// Windows that touch nothing.
	for _, tc := range []struct{ from, rows int }{
		{from: 5, rows: 3},  // past the end
		{from: 0, rows: 0},  // no height
		{from: 0, rows: -1}, // less than none
	} {
		first, last := tr.Visible(tc.from, tc.rows)
		if first != last {
			t.Errorf("rows [%d,+%d) touch blocks [%d,%d), want none", tc.from, tc.rows, first, last)
		}
	}

	// Drawing a window that starts past everything writes nothing rather than
	// reaching for a block that is not there.
	s := grid.NewSurface(20, 2)
	drawTranscript(s.View(), 99, &tr)
	if got := rowText(s.View(), 0); got != "" {
		t.Errorf("a window past the end drew %q", got)
	}

	// The request is an absolute half-open interval. One wholly before row zero is
	// empty; one crossing it contributes only the part that exists.
	if got := tr.Rows(-4, 2); got != nil {
		t.Errorf("Rows(-4, 2) = %q, want no live rows", rowTexts(got))
	}
	if got := tr.Rows(-1, 2); len(got) != 1 || got[0].Text != "a:0" {
		t.Errorf("Rows(-1, 2) = %q, want only row zero", rowTexts(got))
	}
	first, last := tr.Visible(-4, 2)
	if first != last {
		t.Errorf("Visible(-4, 2) = [%d,%d), want no blocks", first, last)
	}
	first, last = tr.Visible(-1, 2)
	if first != tr.FirstBlock() || last != tr.FirstBlock()+1 {
		t.Errorf("Visible(-1, 2) = [%d,%d), want the first block", first, last)
	}
}

func TestTranscriptOfNothingAtAll(t *testing.T) {
	var tr headless.Transcript
	if got := tr.Height(); got != 0 {
		t.Errorf("rows = %d", got)
	}
	if _, _, ok := tr.At(0); ok {
		t.Error("row 0 of an empty transcript belongs to a block")
	}
	first, last := tr.Visible(0, 10)
	if first != 0 || last != 0 {
		t.Errorf("Visible = [%d,%d), want [0,0)", first, last)
	}
	drawTranscript(grid.NewSurface(10, 2).View(), 0, &tr)
	if got := tr.Rows(0, 4); got != nil {
		t.Errorf("Rows = %q, want nothing", rowTexts(got))
	}
}

func TestTranscriptFindsRowsAcrossManyBlocks(t *testing.T) {
	// Enough blocks that the bisection has to go both ways, and every row checked
	// against the block it must belong to.
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	const blocks, lines = 9, 3
	for i := range blocks {
		tr.Append(&block{name: fmt.Sprintf("b%d", i), lines: lines})
	}
	for row := range blocks * lines {
		i, offset, ok := tr.At(row)
		if !ok {
			t.Fatalf("row %d belongs to nothing", row)
		}
		if want := headless.BlockID(row / lines); i != want {
			t.Errorf("row %d is block %d, want %d", row, i, want)
		}
		if want := row % lines; offset != want {
			t.Errorf("row %d is offset %d, want %d", row, offset, want)
		}
	}
}

// TestTranscriptStepsOverEmptyBlocksWhileDrawing: a block of no height occupies no
// rows, so it is neither drawn nor counted, and the rows after it do not shift.
func TestTranscriptStepsOverEmptyBlocksWhileDrawing(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "a", lines: 1})
	tr.Append(&block{name: "gone", lines: 0})
	tr.Append(&block{name: "b", lines: 1})

	s := grid.NewSurface(20, 2)
	drawTranscript(s.View(), 0, &tr)
	for y, want := range []string{"a:0", "b:0"} {
		if got := rowText(s.View(), y); got != want {
			t.Errorf("row %d = %q, want %q", y, got, want)
		}
	}
	if got := rowTexts(tr.Rows(0, 2)); len(got) != 2 || got[0] != "a:0" || got[1] != "b:0" {
		t.Errorf("Rows = %q, want [a:0 b:0]", got)
	}
}

// short is a block that says it is taller than the text it can produce, which is what
// a block whose content changed between measuring and copying looks like.
type short struct{ rows int }

func (s *short) Measure(int) int     { return s.rows }
func (s *short) Draw(v grid.View)    { v.Text(0, 0, "short", grid.Style{}) }
func (s *short) Rows(int) []text.Row { return []text.Row{{Text: "only one"}} }

func TestTranscriptTextSurvivesABlockThatSaysTooLittle(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&short{rows: 3})

	got := rowTexts(tr.Rows(0, 3))
	want := []string{"only one", "", ""}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// rowTexts is what a row list says, so a test can state the whole of it at once.
func rowTexts(rows []text.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Text
	}
	return out
}

func TestScrollRevealMovesAsLittleAsItCan(t *testing.T) {
	// A reader stepping through search results wants the text around them to stay put
	// where it already fits. Centring every result loses the context that made it
	// worth finding.
	for _, tc := range []struct {
		name       string
		offset     int
		row        int
		wantOffset int
	}{
		{name: "already visible", offset: 10, row: 12, wantOffset: 10},
		{name: "the first visible row", offset: 10, row: 10, wantOffset: 10},
		{name: "the last visible row", offset: 10, row: 14, wantOffset: 10},
		{name: "just above", offset: 10, row: 9, wantOffset: 9},
		{name: "far above", offset: 10, row: 2, wantOffset: 2},
		{name: "just below", offset: 10, row: 15, wantOffset: 11},
		{name: "far below", offset: 10, row: 40, wantOffset: 36},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s headless.Scroll
			stageScroll(&s, 100, 5)
			s.By(tc.offset)
			s.Reveal(tc.row, tc.row)
			if got := s.Offset(); got != tc.wantOffset {
				t.Errorf("offset = %d, want %d", got, tc.wantOffset)
			}
		})
	}
}

func TestScrollRevealStopsFollowing(t *testing.T) {
	// A row was asked for, and following the end would scroll away from it at once.
	var s headless.Scroll
	s.ToBottom()
	stageScroll(&s, 100, 5)
	if !s.AtBottom() {
		t.Fatal("not following to begin with")
	}
	s.Reveal(3, 3)
	if s.AtBottom() {
		t.Error("still following after a row was asked for")
	}
	if got := s.Offset(); got != 3 {
		t.Errorf("offset = %d, want 3", got)
	}
}

func TestScrollRevealShowsWhereReadingBegins(t *testing.T) {
	// A match taller than the window: its start wins, or the reader is shown the end
	// of something and left to scroll back for what it was part of.
	var s headless.Scroll
	stageScroll(&s, 100, 3)
	s.Reveal(20, 40)
	if got := s.Offset(); got != 20 {
		t.Errorf("offset = %d, want the start of the range", got)
	}

	// And a range that fits is brought in whole.
	s.Reveal(60, 62)
	if got := s.Offset(); got != 60 {
		t.Errorf("offset = %d, want 60", got)
	}
}

func TestScrollRevealBackwardsRangeIsTheSameRange(t *testing.T) {
	var s headless.Scroll
	stageScroll(&s, 100, 5)
	s.Reveal(40, 20)
	if got := s.Offset(); got != 20 {
		t.Errorf("offset = %d, want 20", got)
	}
}

func TestScrollRevealWithNoWindowDoesNothing(t *testing.T) {
	var s headless.Scroll
	s.Reveal(50, 50)
	if got := s.Offset(); got != 0 {
		t.Errorf("offset = %d before any layout", got)
	}
}

// TestScrollTakesTheTerminalsWordForWhatANotchIs. The same code used to scroll three
// times as far on Apple Terminal as on iTerm2, because it moved a fixed number of rows
// per report and they send a different number of reports.
func TestScrollTakesTheTerminalsWordForWhatANotchIs(t *testing.T) {
	keys := headless.DefaultScrollKeys()
	notch := func(w input.Wheel) int {
		var s headless.Scroll
		stageScroll(&s, 1000, 10)
		s.ToTop()
		s.Wheel(w)
		for range w.Reports {
			s.Handle(input.Mouse{Action: input.WheelDown}, keys)
		}
		return s.Offset()
	}

	// One notch is three rows on both, however many reports it took to say so.
	if got := notch(input.Wheel{Reports: 3, Rows: 3}); got != 3 {
		t.Errorf("a notch on a terminal that sends three reports moved %d rows, want 3", got)
	}
	if got := notch(input.Wheel{Reports: 1, Rows: 3}); got != 3 {
		t.Errorf("a notch on a terminal that sends one report moved %d rows, want 3", got)
	}
}

// printer records what was handed to the terminal, and can refuse.
type printer struct {
	got    []string
	refuse bool
}

func (p *printer) print(b headless.Block, rows int) bool {
	if p.refuse {
		return false
	}
	name := "?"
	if bl, ok := b.(*block); ok {
		name = bl.name
	}
	p.got = append(p.got, fmt.Sprintf("%s:%d", name, rows))
	return true
}

// TestOnlyTheLeadingRunIsCommitted. Text printed into a terminal goes after what is
// already there, so a block that finished while an earlier one is still being written
// has to wait — giving it over first would put the answer above the question.
func TestOnlyTheLeadingRunIsCommitted(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	for i := range 4 {
		tr.Append(&block{name: fmt.Sprintf("b%d", i), lines: 2})
	}
	// The first, and the third, but not the second.
	tr.Finish(0)
	tr.Finish(2)

	var p printer
	if got := tr.Commit(p.print); got != 1 {
		t.Fatalf("committed %d blocks, want only the leading one", got)
	}
	if len(p.got) != 1 || p.got[0] != "b0:2" {
		t.Errorf("gave the terminal %v", p.got)
	}

	// Once the second finishes, both it and the third go, in order.
	tr.Finish(1)
	p.got = nil
	if got := tr.Commit(p.print); got != 2 {
		t.Fatalf("committed %d, want the two now leading", got)
	}
	if len(p.got) != 2 || p.got[0] != "b1:2" || p.got[1] != "b2:2" {
		t.Errorf("gave the terminal %v, want b1 then b2", p.got)
	}
}

// TestABlockGoesOnce, whatever happens to it afterwards. The terminal owns it and the
// program cannot take it back.
func TestABlockGoesOnce(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	b := &block{name: "only", lines: 2}
	tr.Append(b)
	tr.Finish(0)

	var p printer
	tr.Commit(p.print)
	// It changes anyway, which a caller should not do and which must not print again.
	b.lines = 9
	tr.Changed(0)
	if got := tr.Commit(p.print); got != 0 {
		t.Errorf("committed %d blocks the second time", got)
	}
	if len(p.got) != 1 {
		t.Errorf("gave the terminal %v", p.got)
	}
}

func TestARefusalStopsTheRun(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	for i := range 3 {
		id := tr.Append(&block{name: fmt.Sprintf("b%d", i), lines: 1})
		tr.Finish(id)
	}

	p := printer{refuse: true}
	if got := tr.Commit(p.print); got != 0 {
		t.Fatalf("committed %d despite a refusal", got)
	}
	// And everything is still there to try again.
	p.refuse = false
	if got := tr.Commit(p.print); got != 3 {
		t.Errorf("committed %d on the retry, want all three", got)
	}
}

// TestACommittedBlockIsNoLongerDrawn, because it is the terminal's now and drawing it
// again would show it twice.
func TestACommittedBlockIsNoLongerDrawn(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "gone", lines: 2})
	tr.Append(&block{name: "here", lines: 2})
	tr.Finish(0)

	var p printer
	tr.Commit(p.print)

	s := grid.NewSurface(20, 4)
	drawTranscript(s.View(), tr.StartRow(), &tr)
	for y, want := range []string{"here:0", "here:1"} {
		if got := rowText(s.View(), y); got != want {
			t.Errorf("row %d = %q, want %q", y, got, want)
		}
	}
}

func TestStartRowIsWhereWhatIsLeftBegins(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "a", lines: 3})
	tr.Append(&block{name: "b", lines: 2})

	if got := tr.StartRow(); got != 0 {
		t.Errorf("nothing committed, and it begins at %d", got)
	}
	tr.Finish(0)
	var p printer
	tr.Commit(p.print)
	if got := tr.StartRow(); got != 3 {
		t.Errorf("begins at %d, want past the three rows given away", got)
	}
	if got := tr.FirstBlock(); got != 1 {
		t.Errorf("first live block = %d, want 1", got)
	}
	if got := tr.Len(); got != 1 {
		t.Errorf("live blocks = %d, want 1", got)
	}
	if got := tr.Height(); got != 2 {
		t.Errorf("live rows = %d, want 2", got)
	}
}

func TestFinishIgnoresWhatIsNotThere(t *testing.T) {
	var tr headless.Transcript
	stageTranscript(&tr, 20)
	tr.Append(&block{name: "a", lines: 1})
	tr.Finish(^headless.BlockID(0))
	tr.Finish(9)
	if tr.Finished(^headless.BlockID(0)) || tr.Finished(9) || tr.Finished(0) {
		t.Error("something was finished that should not be")
	}
	tr.Finish(0)
	if !tr.Finished(0) {
		t.Error("the block was not finished")
	}
}
