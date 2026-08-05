package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
)

// said is a block of plain rows, standing in for anything a session prints.
type said struct{ rows []string }

func (s *said) Measure(int) int { return len(s.rows) }

func (s *said) Draw(v grid.View) {
	for y, row := range s.rows {
		v.Text(0, y, row, grid.Style{})
	}
}

func (s *said) Rows(int) []headless.Row {
	out := make([]headless.Row, len(s.rows))
	for i, row := range s.rows {
		out[i] = headless.Row{Text: row}
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
		if c := v.CellAt(x, y); c != nil {
			out[x] = c.Style
		}
	}
	return out
}

func TestTranscriptDrawsTheWindow(t *testing.T) {
	tr := session(t, 20, []string{"one", "two", "three", "four"})
	var sc headless.Scroll
	s := grid.NewSurface(20, 2)

	kit.Transcript{Content: tr, Scroll: &sc}.Draw(s.View())
	if got := rowOf(s.View(), 0, 20); !strings.HasPrefix(got, "one") {
		t.Errorf("the first row is %q", got)
	}
	if got := rowOf(s.View(), 1, 20); !strings.HasPrefix(got, "two") {
		t.Errorf("the second row is %q", got)
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
	kit.Transcript{Content: tr, Selection: &sel, SelectionStyle: th.Selection}.Draw(s.View())

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

	kit.Transcript{
		Content: tr,
		Matches: []headless.Match{
			{Row: 0, Spans: []headless.Span{{Col: 0, Width: 3}}},
			{Row: 0, Spans: []headless.Span{{Col: 8, Width: 3}}},
		},
		Current:      1,
		MatchStyle:   th.Selection,
		CurrentStyle: th.Accent,
	}.Draw(s.View())

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
	sticky := &headless.Sticky{Blocks: []int{0}, Gap: 1}
	s := grid.NewSurface(20, 4)

	view := kit.Transcript{Content: tr, Scroll: &sc, Sticky: sticky, Divider: "-"}
	// Scrolled two rows down, so the prompt is off the top.
	sc.Layout(tr.Height(), 4)
	sc.By(2)
	view.Draw(s.View())

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

func TestTranscriptDrawsNothingWithoutContent(t *testing.T) {
	s := grid.NewSurface(10, 2)
	kit.Transcript{}.Draw(s.View())
	kit.Transcript{Content: session(t, 10, []string{"x"})}.Draw(grid.View{})
	if got := rowOf(s.View(), 0, 10); strings.TrimSpace(got) != "" {
		t.Errorf("drew %q", got)
	}
}

func TestDressedFillsInALook(t *testing.T) {
	th, g := kit.Dark(), kit.Unicode()
	got := kit.Dressed(th, g)
	if got.SelectionStyle != th.Selection {
		t.Error("the selection was not dressed")
	}
	if got.Divider != g.Horizontal {
		t.Errorf("the divider is %q, want the glyph set's rule", got.Divider)
	}
	if got.CurrentStyle == got.MatchStyle {
		t.Error("the current match looks like every other one")
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
	sticky := &headless.Sticky{Blocks: []int{0}, Gap: 1}
	s := grid.NewSurface(20, 4)

	kit.Transcript{Content: tr, Scroll: &sc, Sticky: sticky, Divider: "-"}.Draw(s.View())

	// Two rows of body, and the last of them has to be the last row of content.
	if got := rowOf(s.View(), 3, 20); !strings.HasPrefix(got, "a6") {
		t.Errorf("the last row is %q, want the end of the content", got)
	}
}
