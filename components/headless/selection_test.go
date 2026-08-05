package headless_test

import (
	"image"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// lines is a block whose rows are given verbatim, so a selection test can state the
// text it is dragging over.
type lines struct{ rows []headless.Row }

func (l *lines) Measure(int) int         { return len(l.rows) }
func (l *lines) Rows(int) []headless.Row { return l.rows }
func (l *lines) Draw(v grid.View) {
	for y, r := range l.rows {
		v.Text(0, y, r.Text, grid.Style{})
	}
}

// text is a transcript of one block with the given rows.
func transcriptOf(rows ...headless.Row) *headless.Transcript {
	var tr headless.Transcript
	tr.Resize(80)
	tr.Append(&lines{rows: rows})
	return &tr
}

func plainRows(texts ...string) []headless.Row {
	out := make([]headless.Row, len(texts))
	for i, s := range texts {
		out[i] = headless.Row{Text: s}
	}
	return out
}

func TestSelectionOrdersItsEndsWithoutMovingTheAnchor(t *testing.T) {
	// A drag upwards is a real thing, and the anchor has to stay where the user put
	// it: sorting on the way in would turn the selection inside out as the pointer
	// crossed its own start.
	var s headless.Selection
	s.Begin(headless.Point{Row: 5, Col: 3})
	s.Extend(headless.Point{Row: 2, Col: 1})

	start, end := s.Range()
	if start != (headless.Point{Row: 2, Col: 1}) || end != (headless.Point{Row: 5, Col: 3}) {
		t.Errorf("range = %+v..%+v, want 2:1..5:3", start, end)
	}

	// And back down past the anchor again.
	s.Extend(headless.Point{Row: 9, Col: 0})
	start, end = s.Range()
	if start != (headless.Point{Row: 5, Col: 3}) || end != (headless.Point{Row: 9, Col: 0}) {
		t.Errorf("range = %+v..%+v, want 5:3..9:0", start, end)
	}
}

func TestSelectionIgnoresAPointerWithNoButtonDown(t *testing.T) {
	var s headless.Selection
	s.Extend(headless.Point{Row: 4, Col: 4})
	if s.Active() {
		t.Error("moving over the text selected something")
	}

	s.Begin(headless.Point{Row: 1, Col: 0})
	s.Extend(headless.Point{Row: 1, Col: 5})
	s.Done()
	if s.Dragging() {
		t.Error("still dragging after the button came up")
	}
	// The selection stays after the drag, and stops following the pointer.
	s.Extend(headless.Point{Row: 8, Col: 8})
	if _, end := s.Range(); end != (headless.Point{Row: 1, Col: 5}) {
		t.Errorf("the far end moved to %+v after the drag ended", end)
	}
}

func TestSelectionCoversWhatWasDraggedOver(t *testing.T) {
	var s headless.Selection
	s.Begin(headless.Point{Row: 1, Col: 2})
	s.Extend(headless.Point{Row: 3, Col: 4})

	for _, tc := range []struct {
		name     string
		row, col int
		want     bool
	}{
		{"the row above", 0, 5, false},
		{"before the start on its own row", 1, 1, false},
		{"the first selected cell", 1, 2, true},
		{"past the start on its own row", 1, 40, true},
		{"a whole row in the middle", 2, 0, true},
		{"the last selected cell", 3, 4, true},
		{"past the end on its own row", 3, 5, false},
		{"the row below", 4, 0, false},
	} {
		if got := s.Covers(tc.row, tc.col); got != tc.want {
			t.Errorf("%s (%d,%d) = %v, want %v", tc.name, tc.row, tc.col, got, tc.want)
		}
	}

	s.Clear()
	if s.Covers(2, 0) {
		t.Error("a cleared selection still covers a cell")
	}
	if !s.Empty() {
		t.Error("a cleared selection is not empty")
	}
}

func TestSelectionCopiesWhatItCovers(t *testing.T) {
	tr := transcriptOf(plainRows("first line", "second line", "third line")...)

	for _, tc := range []struct {
		name     string
		from, to headless.Point
		want     string
	}{
		{
			name: "part of one row",
			from: headless.Point{Row: 0, Col: 0}, to: headless.Point{Row: 0, Col: 4},
			want: "first",
		},
		{
			name: "the end of one row",
			from: headless.Point{Row: 1, Col: 7}, to: headless.Point{Row: 1, Col: 99},
			want: "line",
		},
		{
			name: "across rows",
			from: headless.Point{Row: 0, Col: 6}, to: headless.Point{Row: 1, Col: 5},
			want: "line\nsecond",
		},
		{
			name: "everything",
			from: headless.Point{Row: 0, Col: 0}, to: headless.Point{Row: 2, Col: 99},
			want: "first line\nsecond line\nthird line",
		},
		{
			name: "one character",
			from: headless.Point{Row: 0, Col: 0}, to: headless.Point{Row: 0, Col: 0},
			want: "f",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s headless.Selection
			s.Begin(tc.from)
			s.Extend(tc.to)
			if got := s.Text(tr); got != tc.want {
				t.Errorf("copied %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSelectionRejoinsWhatTheWidthBroke is the difference between a copy that pastes
// as a paragraph and one that pastes as a column of fragments hard-wrapped to
// whatever the window happened to be.
func TestSelectionRejoinsWhatTheWidthBroke(t *testing.T) {
	// One logical line the wrap broke between words, so the break ate a space, and
	// then a word too long for the row, where it ate nothing.
	tr := transcriptOf(
		headless.Row{Text: "the quick"},
		headless.Row{Text: "brown", Joined: true, Gap: " "},
		headless.Row{Text: "supercalifragi", Joined: true, Gap: " "},
		headless.Row{Text: "listic", Joined: true},
		headless.Row{Text: "a new paragraph"},
	)

	var s headless.Selection
	s.Begin(headless.Point{Row: 0, Col: 0})
	s.Extend(headless.Point{Row: 4, Col: 99})

	want := "the quick brown supercalifragilistic\na new paragraph"
	if got := s.Text(tr); got != want {
		t.Errorf("copied %q,\n    want %q", got, want)
	}
}

// TestSelectionTakesAWideCharacterOnlyWhenItIsWhollyInside. Half a character cannot
// be put on a clipboard, and taking one the user only touched the edge of is the
// error that gets noticed, because it appears at the ends of every copy.
func TestSelectionTakesAWideCharacterOnlyWhenItIsWhollyInside(t *testing.T) {
	// Columns:  0 1  2 3  4 5  6
	//           中    文    字
	tr := transcriptOf(plainRows("中文字")...)

	for _, tc := range []struct {
		name     string
		from, to int
		want     string
	}{
		{"all three", 0, 5, "中文字"},
		{"the first, exactly", 0, 1, "中"},
		{"the first, one column short", 0, 0, ""},
		{"starting on the second half of the first", 1, 3, "文"},
		{"the middle one", 2, 3, "文"},
		{"ending on the first half of the last", 2, 4, "文"},
		{"the last two", 2, 5, "文字"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s headless.Selection
			s.Begin(headless.Point{Row: 0, Col: tc.from})
			s.Extend(headless.Point{Row: 0, Col: tc.to})
			if got := s.Text(tr); got != tc.want {
				t.Errorf("columns %d..%d copied %q, want %q", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestSelectionCopiesRowsOfBlocksItCannotRead(t *testing.T) {
	// A block that cannot be copied still occupies its rows, so the copy has as many
	// lines as the user dragged over.
	var tr headless.Transcript
	tr.Resize(80)
	tr.Append(&lines{rows: plainRows("before")})
	tr.Append(&plain{lines: 2})
	tr.Append(&lines{rows: plainRows("after")})

	var s headless.Selection
	s.Begin(headless.Point{Row: 0, Col: 0})
	s.Extend(headless.Point{Row: 3, Col: 99})
	if got, want := s.Text(&tr), "before\n\n\nafter"; got != want {
		t.Errorf("copied %q, want %q", got, want)
	}
}

func TestSelectionCopiesNothingWhenThereIsNothingToCopy(t *testing.T) {
	tr := transcriptOf(plainRows("something")...)

	var s headless.Selection
	if got := s.Text(tr); got != "" {
		t.Errorf("an inactive selection copied %q", got)
	}

	s.Begin(headless.Point{Row: 0, Col: 0})
	if got := s.Text(nil); got != "" {
		t.Errorf("a selection over no transcript copied %q", got)
	}

	// Entirely past the end of the text.
	s.Begin(headless.Point{Row: 40, Col: 0})
	s.Extend(headless.Point{Row: 50, Col: 0})
	if got := s.Text(tr); got != "" {
		t.Errorf("a selection past the end copied %q", got)
	}
}

func TestSelectionTrimsThePaddingARowWasDrawnWith(t *testing.T) {
	tr := transcriptOf(plainRows("short      ", "also short   ")...)

	var s headless.Selection
	s.Begin(headless.Point{Row: 0, Col: 0})
	s.Extend(headless.Point{Row: 1, Col: 99})
	if got, want := s.Text(tr), "short\nalso short"; got != want {
		t.Errorf("copied %q, want %q", got, want)
	}
}

func TestClicksCountAGesture(t *testing.T) {
	// A terminal reports two presses and never a double-click, so whether they are one
	// gesture is a question about when they arrived.
	var c headless.Clicks
	base := time.Unix(0, 0)
	at := image.Pt(10, 4)

	if got := c.Press(input.Mouse{Pos: at, At: base}); got != 1 {
		t.Errorf("the first press is %d, want 1", got)
	}
	if got := c.Press(input.Mouse{Pos: at, At: base.Add(100 * time.Millisecond)}); got != 2 {
		t.Errorf("a press soon after is %d, want 2", got)
	}
	if got := c.Press(input.Mouse{Pos: at, At: base.Add(200 * time.Millisecond)}); got != 3 {
		t.Errorf("a third is %d, want 3", got)
	}
	// Long after, the run starts again.
	if got := c.Press(input.Mouse{Pos: at, At: base.Add(2 * time.Second)}); got != 1 {
		t.Errorf("a press long after is %d, want 1", got)
	}
}

func TestClicksBreakWhenThePointerMoves(t *testing.T) {
	var c headless.Clicks
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(10, 4), At: base})
	if got := c.Press(input.Mouse{Pos: image.Pt(30, 4), At: base.Add(50 * time.Millisecond)}); got != 1 {
		t.Errorf("a press elsewhere is %d, want 1", got)
	}
}

// TestClicksAllowAHandThatDrifts. A gesture that broke because the pointer moved one
// column would feel like the double-click had failed.
func TestClicksAllowAHandThatDrifts(t *testing.T) {
	var c headless.Clicks
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(10, 4), At: base})
	if got := c.Press(input.Mouse{Pos: image.Pt(11, 5), At: base.Add(50 * time.Millisecond)}); got != 2 {
		t.Errorf("a press one cell away is %d, want 2", got)
	}
}

func TestClicksTakeTheirOwnInterval(t *testing.T) {
	c := headless.Clicks{Within: 10 * time.Millisecond}
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(0, 0), At: base})
	if got := c.Press(input.Mouse{Pos: image.Pt(0, 0), At: base.Add(50 * time.Millisecond)}); got != 1 {
		t.Errorf("a press past the interval is %d, want 1", got)
	}
}

func TestClicksReset(t *testing.T) {
	var c headless.Clicks
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(0, 0), At: base})
	c.Reset()
	if got := c.Press(input.Mouse{Pos: image.Pt(0, 0), At: base.Add(time.Millisecond)}); got != 1 {
		t.Errorf("the press after a reset is %d, want 1", got)
	}
}

func TestSelectWord(t *testing.T) {
	tr := transcriptOf(plainRows("the quick brown fox")...)
	var s headless.Selection

	if !s.SelectWord(tr, headless.Point{Row: 0, Col: 5}) {
		t.Fatal("no word at column 5")
	}
	if got := s.Text(tr); got != "quick" {
		t.Errorf("selected %q, want %q", got, "quick")
	}
	// And it is a finished selection rather than a drag still in progress.
	if s.Dragging() {
		t.Error("a double-click left a drag in progress")
	}
}

// TestSelectWordTakesACJKRun, which is the refinement that matters for text written
// without spaces.
func TestSelectWordTakesACJKRun(t *testing.T) {
	tr := transcriptOf(plainRows("见 中文词组 here")...)
	var s headless.Selection
	// Column 4 is inside the CJK run: "见" is two columns, then a space.
	if !s.SelectWord(tr, headless.Point{Row: 0, Col: 4}) {
		t.Fatal("no word there")
	}
	if got := s.Text(tr); got != "中文词组" {
		t.Errorf("selected %q, want the whole run", got)
	}
}

func TestSelectWordFindsNothingInTheMargin(t *testing.T) {
	tr := transcriptOf(plainRows("word")...)
	var s headless.Selection
	for _, col := range []int{99, -1} {
		if s.SelectWord(tr, headless.Point{Row: 0, Col: col}) {
			t.Errorf("column %d selected %q", col, s.Text(tr))
		}
	}
	if s.SelectWord(nil, headless.Point{}) {
		t.Error("a selection over no transcript found a word")
	}
	if s.SelectWord(tr, headless.Point{Row: 9}) {
		t.Error("a row that is not there had a word on it")
	}
}

func TestSelectWordSelectsNothingOnASpace(t *testing.T) {
	tr := transcriptOf(plainRows("a b")...)
	var s headless.Selection
	if s.SelectWord(tr, headless.Point{Row: 0, Col: 1}) {
		t.Errorf("a double-click on a space selected %q", s.Text(tr))
	}
}

// TestSelectLineTakesTheRowAndNotTheLine. What a triple-click selects is what the
// reader sees as a line; the logical line behind it holds text they cannot point at.
func TestSelectLineTakesTheRowAndNotTheLine(t *testing.T) {
	tr := transcriptOf(
		headless.Row{Text: "the quick"},
		headless.Row{Text: "brown fox", Joined: true, Gap: " "},
	)
	var s headless.Selection
	if !s.SelectLine(tr, headless.Point{Row: 1, Col: 3}) {
		t.Fatal("no line there")
	}
	if got := s.Text(tr); got != "brown fox" {
		t.Errorf("selected %q, want just the row", got)
	}
}

func TestSelectLineOfNothing(t *testing.T) {
	tr := transcriptOf(headless.Row{Text: ""})
	var s headless.Selection
	if s.SelectLine(tr, headless.Point{Row: 0}) {
		t.Error("an empty row was selected")
	}
	if s.SelectLine(nil, headless.Point{}) {
		t.Error("a selection over no transcript found a line")
	}
}

func TestClicksBreakWhenThePointerMovesUpAndLeft(t *testing.T) {
	// The other sign, which is the branch a single direction of drift never reaches.
	var c headless.Clicks
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(10, 4), At: base})
	if got := c.Press(input.Mouse{Pos: image.Pt(4, 1), At: base.Add(50 * time.Millisecond)}); got != 1 {
		t.Errorf("a press up and to the left is %d, want 1", got)
	}
}
