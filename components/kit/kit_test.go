package kit

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// paint draws a widget into a surface of the given size and returns what it looks
// like, one string per row with a dot for a blank cell.
func paint(w, h int, draw func(grid.View)) []string {
	s := grid.NewSurface(w, h)
	draw(s.View())
	rows := make([]string, 0, h)
	for y := range h {
		var b strings.Builder
		for x := range w {
			c := s.CellAt(x, y)
			switch {
			case c.Width() == 0:
			case c.Content == "":
				b.WriteByte('.')
			default:
				b.WriteString(c.Content)
			}
		}
		rows = append(rows, b.String())
	}
	return rows
}

func equalRows(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("drawn:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBoxFramesAndReportsWhatIsLeft(t *testing.T) {
	box := Box{Border: Rounded, Padding: layout.Uniform(1)}
	rows := paint(8, 5, func(v grid.View) {
		inner := box.Draw(v)
		w, h := inner.Size()
		if w != 4 || h != 1 {
			t.Fatalf("inner = %dx%d, want 4x1", w, h)
		}
		inner.Text(0, 0, "abcd", grid.Style{})
	})
	equalRows(t, rows, []string{
		"╭──────╮",
		"│......│",
		"│.abcd.│",
		"│......│",
		"╰──────╯",
	})
}

func TestBoxOverheadMatchesWhatItDraws(t *testing.T) {
	// A box that reported one overhead and drew another would have its content
	// clipped, and the bug would look like it belonged to the content.
	for _, box := range []Box{
		{},
		{Border: Rounded},
		{Padding: layout.Uniform(2)},
		{Border: Square, Padding: layout.Symmetric(1, 2)},
	} {
		over := box.Overhead()
		s := grid.NewSurface(20, 10)
		inner := box.Inner(s.View())
		iw, ih := inner.Size()
		if iw != 20-over.W || ih != 10-over.H {
			t.Errorf("box %+v: inner %dx%d does not match overhead %+v", box, iw, ih, over)
		}
	}
}

func TestBoxTitleSitsInTheBorder(t *testing.T) {
	rows := paint(12, 2, func(v grid.View) {
		Box{Border: Rounded, Title: "Plan"}.Draw(v)
	})
	if !strings.Contains(rows[0], "Plan") {
		t.Fatalf("top border = %q, want the title in it", rows[0])
	}
	// A column of border survives on each side, or the line stops reading as a frame.
	if !strings.HasPrefix(rows[0], "╭─") || !strings.HasSuffix(rows[0], "─╮") {
		t.Fatalf("top border = %q, want border either side of the title", rows[0])
	}
}

func TestBoxSurvivesBeingSqueezed(_ *testing.T) {
	// A collapsing layout must look small, not corrupted. None of these may panic.
	for _, size := range [][2]int{{0, 0}, {1, 1}, {2, 1}, {1, 3}, {3, 2}} {
		paint(size[0], size[1], func(v grid.View) {
			Box{Border: Rounded, Title: "title", Footer: "footer", Padding: layout.Uniform(1)}.Draw(v)
		})
	}
}

func TestLabelTruncatesRatherThanWraps(t *testing.T) {
	// One row is all a label ever has. Folding would push whatever is below it off
	// the screen.
	rows := paint(6, 2, func(v grid.View) {
		Label{Text: "far too long", Ellipsis: "…"}.Draw(v)
	})
	equalRows(t, rows, []string{"far t…", "......"})
}

func TestLabelAlignment(t *testing.T) {
	for _, tc := range []struct {
		align layout.Align
		want  string
	}{
		{layout.Start, "ab........"},
		{layout.Center, "....ab...."},
		{layout.End, "........ab"},
	} {
		rows := paint(10, 1, func(v grid.View) {
			Label{Text: "ab", Align: tc.align}.Draw(v)
		})
		equalRows(t, rows, []string{tc.want})
	}
}

func TestParagraphHeightFollowsWidth(t *testing.T) {
	p := NewParagraph("one two three four", grid.Style{})
	if got := p.Measure(9); got != 3 {
		t.Fatalf("height at 9 = %d, want 3", got)
	}
	if got := p.Measure(4); got != 5 {
		t.Fatalf("height at 4 = %d, want 5", got)
	}
	// And what it reports is what it draws, or a container's layout is a guess.
	rows := paint(9, p.Measure(9), func(v grid.View) { p.Draw(v) })
	equalRows(t, rows, []string{"one two..", "three....", "four....."})
}

func TestParagraphKeepsNewlinesAsLineBreaks(t *testing.T) {
	p := NewParagraph("first\nsecond", grid.Style{})
	if got := p.Measure(20); got != 2 {
		t.Fatalf("height = %d, want a row per line", got)
	}
}

func TestParagraphIndentsEveryRow(t *testing.T) {
	p := NewParagraph("one two three", grid.Style{})
	p.Indent = 2
	rows := paint(7, 3, func(v grid.View) { p.Draw(v) })
	equalRows(t, rows, []string{"..one..", "..two..", "..three"})
}

func TestParagraphCapsItsHeight(t *testing.T) {
	p := NewParagraph("one two three four five", grid.Style{})
	p.MaxRows = 2
	if got := p.Measure(6); got != 2 {
		t.Fatalf("height = %d, want the cap", got)
	}
	rows := paint(6, 2, func(v grid.View) { p.Draw(v) })
	// The last surviving row usually fits, so truncating it would say nothing. It
	// still has to tell the reader there is more.
	if !strings.Contains(rows[1], "…") {
		t.Fatalf("last row = %q, want it to say it was cut", rows[1])
	}
}

func TestParagraphRewrapsWhenItsTextChanges(t *testing.T) {
	// The wrap is memoised because it is asked for twice a frame. A memo that
	// outlived its content would show the old text forever.
	p := NewParagraph("short", grid.Style{})
	if got := p.Measure(20); got != 1 {
		t.Fatalf("height = %d", got)
	}
	p.SetText(linesOf("one\ntwo\nthree", grid.Style{}))
	if got := p.Measure(20); got != 3 {
		t.Fatalf("height after the text changed = %d, want 3", got)
	}
}

func TestSpinnerAdvancesOnlyWhenTold(t *testing.T) {
	// It holds a frame number, not a clock: an idle UI must not wake up to animate
	// something nobody is waiting for.
	s := &Spinner{Frames: []string{"a", "b"}, Label: "working"}
	first := paint(10, 1, func(v grid.View) { s.Draw(v) })
	again := paint(10, 1, func(v grid.View) { s.Draw(v) })
	equalRows(t, first, again)

	s.Tick()
	next := paint(10, 1, func(v grid.View) { s.Draw(v) })
	if next[0] == first[0] {
		t.Fatalf("frame did not change after a tick: %q", next[0])
	}
	if !strings.Contains(next[0], "working") {
		t.Fatalf("row = %q, want the label", next[0])
	}
}

func TestSpinnerDropsALabelThatDoesNotFit(t *testing.T) {
	s := &Spinner{Frames: []string{"a"}, Label: "far too long to fit"}
	rows := paint(3, 1, func(v grid.View) { s.Draw(v) })
	if !strings.HasPrefix(rows[0], "a") {
		t.Fatalf("row = %q, want the glyph", rows[0])
	}
}

func TestScrollbarThumbTracksThePosition(t *testing.T) {
	top := paint(1, 4, func(v grid.View) {
		Scrollbar{Total: 40, Window: 4, Offset: 0, Track: "-", Thumb: "#"}.Draw(v)
	})
	if top[0] != "#" || top[3] != "-" {
		t.Fatalf("at the top the bar is %v, want the thumb at the top", top)
	}
	bottom := paint(1, 4, func(v grid.View) {
		Scrollbar{Total: 40, Window: 4, Offset: 36, Track: "-", Thumb: "#"}.Draw(v)
	})
	if bottom[3] != "#" || bottom[0] != "-" {
		t.Fatalf("at the end the bar is %v, want the thumb at the bottom", bottom)
	}
}

func TestScrollbarThumbNeverRoundsAway(t *testing.T) {
	// A thumb rounded down to nothing tells the user nothing.
	rows := paint(1, 4, func(v grid.View) {
		Scrollbar{Total: 10000, Window: 4, Offset: 5000, Track: "-", Thumb: "#"}.Draw(v)
	})
	if !strings.Contains(strings.Join(rows, ""), "#") {
		t.Fatalf("bar = %v, want a thumb somewhere in it", rows)
	}
}

func TestScrollbarKnowsWhenItIsPointless(t *testing.T) {
	if (Scrollbar{Total: 3, Window: 10}).Needed() {
		t.Error("a bar claims to be needed when everything already fits")
	}
	if !(Scrollbar{Total: 30, Window: 10}).Needed() {
		t.Error("a bar does not know it is needed")
	}
}

func TestHelpShowsWhatFitsAndDropsTheRest(t *testing.T) {
	help := Help{Bindings: []headless.Binding{
		{Key: input.Key{Code: input.Enter}, Does: "send"},
		{Key: input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}, Does: "quit"},
		{Key: input.Key{Code: input.Character, Rune: 'g', Mods: input.Ctrl}, Does: "tasks"},
	}}
	full := paint(40, 1, func(v grid.View) { help.Draw(v) })
	for _, want := range []string{"enter send", "ctrl+c quit", "ctrl+g tasks"} {
		if !strings.Contains(full[0], want) {
			t.Fatalf("row = %q, want %q in it", full[0], want)
		}
	}
	// Half a hint is not a hint, so the ones that do not fit are dropped whole and
	// the order decides which survive.
	narrow := paint(14, 1, func(v grid.View) { help.Draw(v) })
	if !strings.Contains(narrow[0], "enter send") {
		t.Fatalf("row = %q, want the first hint kept", narrow[0])
	}
	if strings.Contains(narrow[0], "ctrl+g") {
		t.Fatalf("row = %q, want the hints that did not fit dropped", narrow[0])
	}
}

func TestHelpSkipsHiddenBindings(t *testing.T) {
	help := Help{Bindings: []headless.Binding{
		{Key: input.Key{Code: input.Enter}, Does: "send"},
		{Key: input.Key{Code: input.F5}, Does: "secret", Hidden: true},
	}}
	rows := paint(40, 1, func(v grid.View) { help.Draw(v) })
	if strings.Contains(rows[0], "secret") {
		t.Fatalf("row = %q, want the hidden binding left out", rows[0])
	}
}

func TestTextIsMeasuredTheSameWayItIsDrawn(t *testing.T) {
	// The one invariant the whole layer rests on: a widget that measured text one
	// way and drew it another would misalign everything beside it.
	for _, s := range []string{"abc", "中文", "a中b", "é", "tab\there"} {
		width := text.Width(s)
		rows := paint(width+3, 1, func(v grid.View) { v.Text(0, 0, s, grid.Style{}) })
		if got := strings.TrimRight(rows[0], "."); len([]rune(got)) == 0 {
			t.Fatalf("%q drew nothing", s)
		}
		if tail := rows[0][len(rows[0])-3:]; tail != "..." {
			t.Fatalf("%q measured %d columns but drew past them: %q", s, width, rows[0])
		}
	}
}

func TestTableColumnWidthsFillTheSpaceExactly(t *testing.T) {
	// The right edge has to line up with whatever is drawn beside it.
	table := Table{Columns: []Column{{Width: 6}, {Flex: 1}, {Flex: 2}}, Gap: 1}
	widths := table.Widths(30)
	total := widths[0] + widths[1] + widths[2] + 2
	if total != 30 {
		t.Fatalf("widths %v plus gaps add up to %d, want 30", widths, total)
	}
	if widths[0] != 6 {
		t.Fatalf("fixed column = %d, want 6", widths[0])
	}
	if widths[2] <= widths[1] {
		t.Fatalf("widths %v, want the larger share wider", widths)
	}
}

func TestTableFlexibleColumnsHaveAFloor(t *testing.T) {
	table := Table{Columns: []Column{{Width: 20}, {Flex: 1, Min: 4}}, Gap: 1}
	widths := table.Widths(22)
	if widths[1] < 4 {
		t.Fatalf("widths %v, want the flexible column to keep its floor", widths)
	}
}

func TestTableDrawsHeaderAndRows(t *testing.T) {
	table := Table{
		Columns: []Column{{Title: "id", Width: 4}, {Title: "name", Flex: 1}},
		Rows:    2,
		Header:  true,
		Cell: func(v grid.View, row, col int, base grid.Style) {
			data := [][]string{{"a1", "alpha"}, {"b2", "bravo"}}
			Label{Text: data[row][col], Style: base}.Draw(v)
		},
	}
	rows := paint(12, 3, func(v grid.View) { table.Draw(v) })
	equalRows(t, rows, []string{
		"id...name...",
		"a1...alpha..",
		"b2...bravo..",
	})
	if got := table.Measure(12); got != 3 {
		t.Fatalf("height = %d, want the rows plus the header", got)
	}
}

func TestTableCellsAreTruncatedToTheirColumn(t *testing.T) {
	table := Table{
		Columns: []Column{{Width: 5}, {Flex: 1}},
		Rows:    1,
		Cell: func(v grid.View, _, col int, base grid.Style) {
			_ = col
			Label{Text: "far too long", Style: base, Ellipsis: "…"}.Draw(v)
		},
	}
	rows := paint(12, 1, func(v grid.View) { table.Draw(v) })
	// Neither cell may spill into the other's column.
	if !strings.HasPrefix(rows[0], "far …") {
		t.Fatalf("row = %q, want the first cell truncated at its column", rows[0])
	}
}

func TestTableRowStyleBandsTheWholeRow(t *testing.T) {
	selected := grid.Style{BG: grid.RGBColor(40, 40, 40)}
	table := Table{
		Columns:  []Column{{Flex: 1}},
		Rows:     2,
		RowStyle: func(row int) grid.Style { return map[bool]grid.Style{true: selected}[row == 1] },
		Cell: func(v grid.View, _, _ int, base grid.Style) {
			Label{Text: "x", Style: base}.Draw(v)
		},
	}
	s := grid.NewSurface(6, 2)
	table.Draw(s.View())
	for x := range 6 {
		if got := s.CellAt(x, 1).Style.BG; got != selected.BG {
			t.Fatalf("column %d of the banded row = %+v, want the row style across the whole row", x, got)
		}
	}
}

func TestOverlayDrawsIntoWhereItSaidItWould(t *testing.T) {
	// Area and Draw have to agree, or a hit test a frame later answers about the wrong
	// place.
	o := Overlay{Placement: layout.Placement{Anchor: layout.Middle, Width: 4, Height: 1}}
	s := grid.NewSurface(10, 3)
	area := o.Area(s.View())
	o.Draw(s.View()).Text(0, 0, "abcd", grid.Style{})

	rows := paint(10, 3, func(v grid.View) {
		o.Draw(v).Text(0, 0, "abcd", grid.Style{})
	})
	if !strings.Contains(rows[area.Min.Y], "abcd") {
		t.Fatalf("row %d = %q, want the layer on the row Area named", area.Min.Y, rows[area.Min.Y])
	}
	if got := strings.Index(rows[area.Min.Y], "abcd"); got != area.Min.X {
		t.Fatalf("layer starts at column %d, want %d", got, area.Min.X)
	}
}

func TestOverlayShadeRecedesWhatIsBehindWithoutErasingIt(t *testing.T) {
	// What is behind stays legible and simply recedes, which is what tells the reader it
	// is still there rather than gone.
	shade := grid.Style{Attr: grid.Dim}
	s := grid.NewSurface(8, 2)
	s.View().Text(0, 0, "behind", grid.Style{})
	Overlay{Placement: layout.Placement{Width: 2, Height: 1}, Shade: shade}.Draw(s.View())

	if got := s.CellAt(0, 0).Content; got != "b" {
		t.Fatalf("cell = %q, want what was behind still there", got)
	}
	if !s.CellAt(0, 0).Style.Attr.Has(grid.Dim) {
		t.Fatal("what is behind was not dimmed")
	}
}
