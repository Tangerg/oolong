package kit_test

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/text"
)

// said is a block of plain rows, standing in for anything a session prints.
type said struct{ rows []string }

func (s *said) Measure(int) int { return len(s.rows) }

func (s *said) Draw(v grid.View) {
	for y, row := range s.rows {
		v.Text(0, y, row, grid.Style{})
	}
}

func (s *said) Rows(int) []text.Row {
	out := make([]text.Row, len(s.rows))
	for i, row := range s.rows {
		out[i] = text.Row{Text: row}
	}
	return out
}

func session(t *testing.T, width int, blocks ...[]string) *headless.Transcript {
	t.Helper()
	tr := &headless.Transcript{}
	tr.Resize(width)
	for _, rows := range blocks {
		tr.Append(&said{rows: rows})
	}
	return tr
}

// styles is the style of every cell on a row, so a test can say exactly what was
// picked out.
func styles(v grid.View, y, width int) []grid.Style {
	out := make([]grid.Style, width)
	for x := range width {
		out[x] = cellAt(v, x, y).Style
	}
	return out
}

func drawTranscript(view grid.View, transcript *kit.Transcript) {
	headless.NewRoot(transcript).Draw(view)
}

func TestTranscriptDrawsTheWindow(t *testing.T) {
	tr := session(t, 20, []string{"one", "two", "three", "four"})
	var sc headless.Scroll
	s := grid.NewSurface(20, 2)

	drawTranscript(s.View(), &kit.Transcript{Content: tr, Scroll: &sc})
	if got := rowOf(s.View(), 0, 20); !strings.HasPrefix(got, "one") {
		t.Errorf("the first row is %q", got)
	}
	if got := rowOf(s.View(), 1, 20); !strings.HasPrefix(got, "two") {
		t.Errorf("the second row is %q", got)
	}
}

func TestTranscriptRoutesWheelToItsScroll(t *testing.T) {
	tr := session(t, 20, []string{"one", "two", "three", "four"})
	var sc headless.Scroll
	s := grid.NewSurface(20, 2)
	view := &kit.Transcript{Content: tr, Scroll: &sc}
	drawTranscript(s.View(), view)

	if !view.Handle(input.Mouse{Action: input.WheelDown}) {
		t.Fatal("the transcript declined its own wheel event")
	}
	drawTranscript(s.View(), view)
	if got := rowOf(s.View(), 0, 20); !strings.HasPrefix(got, "two") {
		t.Errorf("the first row after scrolling is %q, want two", got)
	}
}

// TestSelectionIsLaidOverWhatWasDrawn, not drawn into it. What is selected is a
// property of the cells and not of the content, so a block never has to be told.
func TestSelectionIsLaidOverWhatWasDrawn(t *testing.T) {
	tr := session(t, 20, []string{"hello world"})
	var sel headless.Selection
	sel.Begin(headless.Point{Row: 0, Col: 0})
	sel.Extend(headless.Point{Row: 0, Col: 4})

	th := kit.Dark()
	s := grid.NewSurface(20, 1)
	drawTranscript(s.View(), &kit.Transcript{Content: tr, Selection: &sel, Theme: th})

	got := styles(s.View(), 0, 20)
	for x := range 5 {
		if got[x] != th.Selection {
			t.Errorf("column %d is %+v, want the selection style", x, got[x])
		}
	}
	if got[5] == th.Selection {
		t.Error("the column after the selection is styled as selected")
	}
	// And the text is untouched.
	if row := rowOf(s.View(), 0, 20); !strings.HasPrefix(row, "hello world") {
		t.Errorf("the text became %q", row)
	}
}

func TestMatchesArePickedOutAndTheCurrentOneDiffers(t *testing.T) {
	tr := session(t, 20, []string{"cat and cat"})
	th := kit.Dark()
	s := grid.NewSurface(20, 1)

	drawTranscript(s.View(), &kit.Transcript{
		Content: tr,
		Matches: []headless.Match{
			{Row: 0, Spans: []headless.Span{{Col: 0, Width: 3}}},
			{Row: 0, Spans: []headless.Span{{Col: 8, Width: 3}}},
		},
		Current: 1,
		Theme:   th,
	})

	got := styles(s.View(), 0, 20)
	if got[0] != th.Selection {
		t.Errorf("the first match is %+v, want the match style", got[0])
	}
	if got[8] != th.Accent {
		t.Errorf("the current match is %+v, want the accent", got[8])
	}
	if got[4] == th.Selection || got[4] == th.Accent {
		t.Errorf("the text between the matches is picked out: %+v", got[4])
	}
}

// TestAPinnedHeaderTakesRoomFromTheBody, and the body scrolls on from where the
// header left off rather than repeating the rows the header is showing.
func TestAPinnedHeaderTakesRoomFromTheBody(t *testing.T) {
	tr := session(t, 20,
		[]string{"prompt"},
		[]string{"a1", "a2", "a3", "a4", "a5", "a6"},
	)
	var sc headless.Scroll
	sticky := &headless.Sticky{Blocks: []headless.BlockID{0}, Gap: 1}
	s := grid.NewSurface(20, 4)

	view := kit.Transcript{Content: tr, Scroll: &sc, Sticky: sticky, Glyphs: kit.Glyphs{Horizontal: "-"}}
	// Scrolled two rows down, so the prompt is off the top.
	sc.Layout(tr.Height(), 4)
	sc.By(2)
	drawTranscript(s.View(), &view)

	if got := rowOf(s.View(), 0, 20); !strings.HasPrefix(got, "prompt") {
		t.Errorf("the pinned row is %q, want the prompt", got)
	}
	if got := rowOf(s.View(), 1, 20); !strings.HasPrefix(got, "-") {
		t.Errorf("the divider row is %q", got)
	}
	// Two rows of body, showing what the scroll is at — the header takes room from
	// the top rather than hiding content behind itself.
	if got := rowOf(s.View(), 2, 20); !strings.HasPrefix(got, "a2") {
		t.Errorf("the first body row is %q, want a2", got)
	}
}

// TestAPinnedHeaderDissolvesAsTheNextOnePushesItOff. The fade is worked out where
// the scrolling is and has to reach the cells, or a header slides out at full
// strength and disappears at the edge — which reads as a glitch rather than as one
// thing giving way to another.
func TestAPinnedHeaderDissolvesAsTheNextOnePushesItOff(t *testing.T) {
	draw := func(scrollBy int) grid.Style {
		t.Helper()
		tr := session(t, 20,
			[]string{"first", "prompt"},
			[]string{"a1", "a2", "a3"},
			[]string{"second", "prompt"},
			[]string{"b1", "b2", "b3"},
		)
		var sc headless.Scroll
		sticky := &headless.Sticky{Blocks: []headless.BlockID{0, 2}}
		s := grid.NewSurface(20, 4)
		s.SetGround(grid.Ground{
			FG: grid.RGBColor(0xFF, 0xFF, 0xFF),
			BG: grid.RGBColor(0x00, 0x00, 0x00),
		})
		sc.Layout(tr.Height(), 4)
		sc.By(scrollBy)
		drawTranscript(s.View(), &kit.Transcript{Content: tr, Scroll: &sc, Sticky: sticky})
		return cellAt(s, 0, 0).Style
	}

	// Two rows down the first header is pinned and sitting still; four rows down the
	// second block is coming up under it and pushing it off.
	settled, pushed := draw(2), draw(4)

	// A header sitting still is left exactly as it drew itself, which is what keeps
	// an interface that is not moving silent on the wire.
	if !settled.FG.Default() {
		t.Errorf("a header sitting still was recoloured to %+v", settled.FG.RGB())
	}
	// One being pushed off has moved toward what it is drawn on — and only part of
	// the way, because a header that jumped straight to the background would blink
	// out rather than give way.
	if pushed.FG.Default() {
		t.Fatal("a header being pushed off was not faded at all")
	}
	if got := pushed.FG.RGB(); got.R == 0xFF || got.R == 0x00 {
		t.Errorf("a header being pushed off is %+v, want it part way from the text to the background", got)
	}
}

func TestTranscriptDrawsNothingWithoutContent(t *testing.T) {
	s := grid.NewSurface(10, 2)
	drawTranscript(s.View(), &kit.Transcript{})
	drawTranscript(grid.View{}, &kit.Transcript{Content: session(t, 10, []string{"x"})})
	if got := rowOf(s.View(), 0, 10); strings.TrimSpace(got) != "" {
		t.Errorf("drew %q", got)
	}
}

// TestTheCurrentMatchIsToldApartFromTheRest. Stepping through matches has to be
// visible without the others disappearing, so one theme role picks out a match and
// another picks out the one being stepped to.
func TestTheCurrentMatchIsToldApartFromTheRest(t *testing.T) {
	th := kit.Dark()
	if th.Selection == th.Accent {
		t.Fatal("the two roles a transcript tells matches apart with are the same colour")
	}
}

// TestFollowingTheEndStaysAtTheEndWithAHeaderAbove. Laying the scroll out against the
// full height would leave the last rows unreachable, which a session that follows its
// own output notices immediately.
func TestFollowingTheEndStaysAtTheEndWithAHeaderAbove(t *testing.T) {
	tr := session(t, 20,
		[]string{"prompt"},
		[]string{"a1", "a2", "a3", "a4", "a5", "a6"},
	)
	var sc headless.Scroll
	sc.ToBottom()
	sticky := &headless.Sticky{Blocks: []headless.BlockID{0}, Gap: 1}
	s := grid.NewSurface(20, 4)

	drawTranscript(s.View(), &kit.Transcript{Content: tr, Scroll: &sc, Sticky: sticky, Glyphs: kit.Glyphs{Horizontal: "-"}})

	// Two rows of body, and the last of them has to be the last row of content.
	if got := rowOf(s.View(), 3, 20); !strings.HasPrefix(got, "a6") {
		t.Errorf("the last row is %q, want the end of the content", got)
	}
}

// recordingPrinter stands in for an inline loop.
type recordingPrinter struct{ rows []int }

func (p *recordingPrinter) PrintRows(rows int, draw func(grid.View)) {
	p.rows = append(p.rows, rows)
	draw(grid.NewSurface(20, rows).View())
}

func TestCommittingGivesTheFinishedBlocksToThePrinter(t *testing.T) {
	tr := session(t, 20, []string{"one", "two"}, []string{"three"})
	tr.Finish(0)

	var p recordingPrinter
	view := kit.Transcript{Content: tr}
	if got := view.Commit(&p); got != 1 {
		t.Fatalf("committed %d, want the one finished block", got)
	}
	if len(p.rows) != 1 || p.rows[0] != 2 {
		t.Errorf("printed %v, want one block of two rows", p.rows)
	}
}

func TestCommitNTransfersOnlyTheExcessStablePrefix(t *testing.T) {
	tr := session(t, 20, []string{"one"}, []string{"two"}, []string{"three"})
	for id := range headless.BlockID(3) {
		tr.Finish(id)
	}

	var printer recordingPrinter
	view := kit.Transcript{Content: tr}
	if got := view.CommitN(&printer, 1); got != 1 {
		t.Fatalf("committed %d blocks, want one", got)
	}
	if tr.Len() != 2 || tr.FirstBlock() != 1 || len(printer.rows) != 1 {
		t.Fatalf("live=%d first=%d printed=%v", tr.Len(), tr.FirstBlock(), printer.rows)
	}
	if got := view.CommitN(&printer, 0); got != 0 || tr.Len() != 2 {
		t.Fatal("a zero limit transferred retained blocks")
	}
}

func TestCommittingRebasesTheLiveComponentState(t *testing.T) {
	tr := session(t, 20, []string{"old", "rows"}, []string{"live", "tail"})
	tr.Finish(0)

	var scroll headless.Scroll
	scroll.Layout(tr.Height(), 2)
	scroll.By(1)
	var selection headless.Selection
	selection.Begin(headless.Point{Row: 1, Col: 2})
	selection.Extend(headless.Point{Row: 3, Col: 3})
	sticky := &headless.Sticky{Blocks: []headless.BlockID{0, 1}}
	view := kit.Transcript{
		Content:   tr,
		Scroll:    &scroll,
		Selection: &selection,
		Sticky:    sticky,
	}

	var printer recordingPrinter
	if got := view.Commit(&printer); got != 1 {
		t.Fatalf("committed %d blocks, want one", got)
	}
	if tr.StartRow() != 2 || tr.Height() != 2 {
		t.Fatalf("live range is [%d,%d), want [2,4)", tr.StartRow(), tr.EndRow())
	}
	if got := scroll.Offset(); got != 0 {
		t.Errorf("scroll offset is %d after its prefix left, want 0", got)
	}
	if got := sticky.Blocks; len(got) != 1 || got[0] != 1 {
		t.Errorf("sticky blocks are %v, want only live block 1", got)
	}
	start, end := selection.Range()
	if start != (headless.Point{Row: 2}) || end != (headless.Point{Row: 3, Col: 3}) {
		t.Errorf("selection is %v..%v, want live rows 2..3", start, end)
	}

	surface := grid.NewSurface(20, 2)
	drawTranscript(surface.View(), &view)
	if got := rowOf(surface.View(), 0, 20); !strings.HasPrefix(got, "live") {
		t.Errorf("first live row is %q, want live", got)
	}
}

func TestCommittingNothingWhenThereIsNothingToCommitTo(t *testing.T) {
	tr := session(t, 20, []string{"one"})
	tr.Finish(0)
	if got := (&kit.Transcript{Content: tr}).Commit(nil); got != 0 {
		t.Errorf("committed %d to nobody", got)
	}
	if got := (&kit.Transcript{}).Commit(&recordingPrinter{}); got != 0 {
		t.Errorf("a view of nothing committed %d", got)
	}
}

// TestSteppingToAMatchScrollsToIt. Search returns the row a match begins on and the
// scroll can be told to bring a row in; nothing joined them, so a match could be found
// and not reached.
func TestSteppingToAMatchScrollsToIt(t *testing.T) {
	rows := make([]string, 60)
	for i := range rows {
		rows[i] = "line"
	}
	tr := session(t, 20, rows)
	var sc headless.Scroll
	s := grid.NewSurface(20, 5)

	view := kit.Transcript{
		Content: tr,
		Scroll:  &sc,
		Matches: []headless.Match{
			{Row: 2, Spans: []headless.Span{{Col: 0, Width: 4}}},
			{Row: 40, Spans: []headless.Span{{Col: 0, Width: 4}}},
		},
		Current: 1,
	}
	drawTranscript(s.View(), &view)

	if got := sc.Offset(); got > 40 || got+5 <= 40 {
		t.Errorf("the window starts at %d, which does not show row 40", got)
	}
}

// TestSteppingToAMatchShowsTheWholeOfIt, because a match that crosses a break the width
// made covers several rows and half of one is half of what was asked for.
func TestSteppingToAMatchShowsTheWholeOfIt(t *testing.T) {
	rows := make([]string, 60)
	for i := range rows {
		rows[i] = "line"
	}
	tr := session(t, 20, rows)
	var sc headless.Scroll
	s := grid.NewSurface(20, 5)

	drawTranscript(s.View(), &kit.Transcript{
		Content: tr,
		Scroll:  &sc,
		Matches: []headless.Match{{Row: 30, Spans: []headless.Span{
			{Col: 0, Width: 4}, {Col: 0, Width: 4}, {Col: 0, Width: 4},
		}}},
	})

	if got := sc.Offset(); got > 30 || got+5 < 33 {
		t.Errorf("the window starts at %d, which does not show rows 30 to 32", got)
	}
}

func TestNoCurrentMatchScrollsNowhere(t *testing.T) {
	tr := session(t, 20, []string{"a", "b", "c"})
	var sc headless.Scroll
	sc.ToBottom()
	s := grid.NewSurface(20, 2)

	for _, view := range []kit.Transcript{
		{Content: tr, Scroll: &sc},
		{Content: tr, Scroll: &sc, Matches: []headless.Match{{Row: 0}}, Current: 9},
		{Content: tr, Scroll: &sc, Matches: []headless.Match{{Row: 0}}, Current: -1},
		// A match with no columns to show is not somewhere to go.
		{Content: tr, Scroll: &sc, Matches: []headless.Match{{Row: 0}}},
	} {
		drawTranscript(s.View(), &view)
		if !sc.AtBottom() {
			t.Errorf("%+v moved the view off the end", view.Matches)
			sc.ToBottom()
		}
	}
}

// press is a left button going down on the top row of the drawn window.
func press(x int, at time.Time) input.Mouse {
	return input.Mouse{Pos: image.Pt(x, 0), Action: input.MouseDown, Button: input.ButtonLeft, At: at}
}

// TestSelectingWithTheMouse is the seam this closes: a selection, a click counter, the
// word rule, the scroll offset and the translation from a point on screen into a row
// were five pieces a caller had to wire together.
func TestSelectingWithTheMouse(t *testing.T) {
	tr := session(t, 30, []string{"the quick brown", "fox jumps over"})
	var sel headless.Selection
	view := kit.Transcript{Content: tr, Selection: &sel}
	base := time.Unix(0, 0)
	drawTranscript(grid.NewSurface(30, 2).View(), &view)

	// A drag.
	view.Handle(press(4, base))
	view.Handle(input.Mouse{Pos: image.Pt(8, 0), Action: input.MouseDrag})
	view.Handle(input.Mouse{Pos: image.Pt(8, 0), Action: input.MouseUp})
	if got := sel.Text(tr); got != "quick" {
		t.Errorf("the drag selected %q, want %q", got, "quick")
	}

	// A double-click takes the word.
	sel.Clear()
	sel.Clicks.Reset()
	view.Handle(press(11, base))
	view.Handle(press(11, base.Add(80*time.Millisecond)))
	if got := sel.Text(tr); got != "brown" {
		t.Errorf("the double-click selected %q, want %q", got, "brown")
	}

	// A third takes the row.
	view.Handle(press(11, base.Add(160*time.Millisecond)))
	if got := sel.Text(tr); got != "the quick brown" {
		t.Errorf("the triple-click selected %q, want the row", got)
	}
}

// TestSelectingAccountsForTheScroll, because a point on screen means nothing without
// knowing how far the transcript has been scrolled.
func TestSelectingAccountsForTheScroll(t *testing.T) {
	tr := session(t, 30, []string{"first", "second", "third", "fourth"})
	var sc headless.Scroll
	var sel headless.Selection
	s := grid.NewSurface(30, 2)
	view := kit.Transcript{Content: tr, Scroll: &sc, Selection: &sel}
	drawTranscript(s.View(), &view)
	sc.By(2)
	drawTranscript(s.View(), &view)

	// The top row on screen is now "third".
	view.Handle(press(0, time.Unix(0, 0)))
	view.Handle(input.Mouse{Pos: image.Pt(4, 0), Action: input.MouseDrag})
	if got := sel.Text(tr); got != "third" {
		t.Errorf("selected %q, want the row that is on screen", got)
	}
}

func TestASelectionGestureSettlesOutsideTheVisibleTranscript(t *testing.T) {
	tr := session(t, 6, []string{"abcdef", "ghijkl"})
	var sel headless.Selection
	view := kit.Transcript{Content: tr, Selection: &sel}
	drawTranscript(grid.NewSurface(6, 2).View(), &view)

	if !view.Handle(press(1, time.Unix(0, 0))) {
		t.Fatal("the selection did not accept its press")
	}
	if !view.Handle(input.Mouse{Pos: image.Pt(20, 20), Action: input.MouseDrag}) {
		t.Fatal("the selection dropped a drag beyond the visible rows")
	}
	start, end := sel.Range()
	if start != (headless.Point{Row: 0, Col: 1}) || end != (headless.Point{Row: 1, Col: 5}) {
		t.Fatalf("selection = %v..%v, want (0,1)..(1,5)", start, end)
	}
	if !view.Handle(input.Mouse{Pos: image.Pt(20, 20), Action: input.MouseUp}) {
		t.Fatal("the selection dropped the release beyond the visible rows")
	}
	if sel.Dragging() {
		t.Fatal("the selection still believes its released gesture is active")
	}
	if view.Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDrag}) {
		t.Fatal("an unowned drag was consumed after release")
	}
}

func TestASelectionReleaseReturnsToTheSelectionThatAcceptedItsPress(t *testing.T) {
	tr := session(t, 10, []string{"first", "second"})
	first, second := &headless.Selection{}, &headless.Selection{}
	view := kit.Transcript{Content: tr, Selection: first}
	root := headless.NewRoot(&view)
	root.Draw(grid.NewSurface(10, 2).View())

	view.Handle(press(1, time.Unix(0, 0)))
	view.Selection = second
	// Publish both the replacement and collapsed geometry. The release still belongs
	// to the selection that took the press; there need not be a row left to hit-test.
	root.Draw(grid.NewSurface(0, 0).View())
	if !view.Handle(input.Mouse{Pos: image.Pt(20, 20), Action: input.MouseUp}) {
		t.Fatal("the collapsed transcript dropped the release")
	}
	if first.Dragging() {
		t.Fatal("the original selection was left dragging")
	}
	if second.Active() || second.Dragging() {
		t.Fatal("the replacement inherited another selection's gesture")
	}
}

func TestTheTranscriptLeavesAloneWhatIsNotItsToAnswer(t *testing.T) {
	tr := session(t, 30, []string{"a"})
	var sel headless.Selection
	for _, tc := range []struct {
		name string
		view kit.Transcript
		ev   input.Mouse
	}{
		{name: "no content", view: kit.Transcript{Selection: &sel}, ev: press(0, time.Time{})},
		{name: "no selection", view: kit.Transcript{Content: tr}, ev: press(0, time.Time{})},
		{
			name: "the wrong button",
			view: kit.Transcript{Content: tr, Selection: &sel},
			ev:   input.Mouse{Action: input.MouseDown, Button: input.ButtonRight},
		},
		{
			name: "the wheel",
			view: kit.Transcript{Content: tr, Selection: &sel},
			ev:   input.Mouse{Action: input.WheelDown},
		},
	} {
		if tc.view.Handle(tc.ev) {
			t.Errorf("%s: it was consumed", tc.name)
		}
	}
}

// TestADoubleClickInTheMarginStartsASelection, because there is no word there to take
// and a press is still a press.
func TestADoubleClickInTheMarginStartsASelection(t *testing.T) {
	tr := session(t, 30, []string{"ab"})
	var sel headless.Selection
	view := kit.Transcript{Content: tr, Selection: &sel}
	base := time.Unix(0, 0)
	drawTranscript(grid.NewSurface(30, 1).View(), &view)

	view.Handle(press(20, base))
	view.Handle(press(20, base.Add(80*time.Millisecond)))
	if !sel.Dragging() {
		t.Error("a double-click past the text did not start a selection")
	}
}
