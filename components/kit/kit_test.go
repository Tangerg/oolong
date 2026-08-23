package kit_test

import (
	"errors"
	"image"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

type cellReader interface {
	CellAt(x, y int) (grid.Cell, bool)
}

func cellAt(r cellReader, x, y int) grid.Cell {
	cell, ok := r.CellAt(x, y)
	if !ok {
		panic("test read outside grid")
	}
	return cell
}

// paint draws a widget into a surface of the given size and returns what it looks
// like, one string per row with a dot for a blank cell.
func paint(w, h int, draw func(grid.View)) []string {
	s := grid.NewSurface(w, h)
	draw(s.View())
	rows := make([]string, 0, h)
	for y := range h {
		var b strings.Builder
		for x := range w {
			c := cellAt(s, x, y)
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

func paintWidget(w, h int, widget headless.Widget) []string {
	return paint(w, h, headless.NewRoot(widget).Draw)
}

func equalRows(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("drawn:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBoxFramesAndReportsWhatIsLeft(t *testing.T) {
	box := kit.Box{Glyphs: kit.Unicode(), Padding: layout.Uniform(1)}
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

func TestBoxDrawsAClippedMiddleWithoutInventingEdges(t *testing.T) {
	box := kit.Box{Glyphs: kit.Unicode()}
	rows := paint(8, 3, func(view grid.View) {
		box.Draw(view.Sub(grid.Rect(0, -50, 8, 100)))
	})
	equalRows(t, rows, []string{
		"│......│",
		"│......│",
		"│......│",
	})
}

// TestBoxGeometryAgreesWithItself sweeps every small size a collapsing layout can
// hand a box.
//
// [Box.Draw] returns the interior it just framed and [Box.InnerRect] computes the same
// interior without drawing, so the two must never disagree — a caller that measured
// with one and drew with the other would be laying out against a rectangle the frame
// does not have. Degenerate sizes are the interesting ones because that is where a
// guard on one path and not the other stops being invisible.
func TestBoxGeometryAgreesWithItself(t *testing.T) {
	boxes := map[string]kit.Box{
		"plain":  {Glyphs: kit.Unicode()},
		"titled": {Glyphs: kit.Unicode(), Title: "title", Footer: "footer"},
		"padded": {Glyphs: kit.Unicode(), Padding: layout.Uniform(3)},
		"bare":   {Bare: true, Padding: layout.Symmetric(2, 5)},
	}
	for name, box := range boxes {
		for w := range 15 {
			for h := range 15 {
				surface := grid.NewSurface(w, h)
				dw, dh := box.Draw(surface.View()).Size()
				inner := box.InnerRect(surface.View().Bounds().Size()).Size()
				iw, ih := inner.X, inner.Y
				if dw != iw || dh != ih {
					t.Fatalf("%s at %dx%d: Draw gave %dx%d, Inner gave %dx%d", name, w, h, dw, dh, iw, ih)
				}
				if want := max(w-box.Overhead().X, 0); w > 0 && h > 0 && iw != want {
					t.Fatalf("%s at %dx%d: interior width %d, overhead leaves %d", name, w, h, iw, want)
				}
			}
		}
	}
}

func TestBoxOverheadMatchesWhatItDraws(t *testing.T) {
	// A box that reported one overhead and drew another would have its content
	// clipped, and the bug would look like it belonged to the content.
	for _, box := range []kit.Box{
		{},
		{Glyphs: kit.Unicode()},
		{Padding: layout.Uniform(2)},
		{Glyphs: kit.Unicode(), Border: kit.Unicode().Square(), Padding: layout.Symmetric(1, 2)},
	} {
		over := box.Overhead()
		s := grid.NewSurface(20, 10)
		inner := box.InnerRect(s.View().Bounds().Size()).Size()
		iw, ih := inner.X, inner.Y
		if iw != 20-over.X || ih != 10-over.Y {
			t.Errorf("box %+v: inner %dx%d does not match overhead %+v", box, iw, ih, over)
		}
	}
}

type panelChild struct {
	focused bool
	across  int
	event   input.Event
	// deaf refuses every event, which is how a press that begins no interaction is
	// told apart from one that does.
	deaf bool
}

func (c *panelChild) Draw(frame headless.Frame) {
	frame.Text(0, 0, "inside", grid.Style{})
}

func (c *panelChild) Measure(across int) int {
	c.across = across
	return 2
}

func (c *panelChild) Focus(has bool) { c.focused = has }

func (c *panelChild) Handle(event input.Event) bool {
	if c.deaf {
		return false
	}
	c.event = event
	return true
}

func TestPanelIsTheLiveCounterpartToBox(t *testing.T) {
	child := &panelChild{}
	panel := kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: kit.Theme{}, Glyphs: kit.Unicode()}, Content: child})
	panel.Box.Padding = layout.Uniform(1)
	panel.Box.Title = "pane"

	rows := paintWidget(12, 5, panel)
	if !strings.Contains(rows[0], "pane") || !strings.Contains(rows[2], "inside") {
		t.Fatalf("panel drew:\n%s\nwant its title and child inside the frame", strings.Join(rows, "\n"))
	}
	if got := panel.Measure(12); got != 6 {
		t.Fatalf("Measure = %d, want child height 2 plus four rows of chrome", got)
	}
	if child.across != 8 {
		t.Fatalf("child measured at width %d, want the 12 columns minus four columns of chrome", child.across)
	}
}

func TestPanelPreservesFocusAndTranslatesPointerCoordinates(t *testing.T) {
	child := &panelChild{}
	panel := kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: kit.Theme{}, Glyphs: kit.Unicode()}, Content: child})
	panel.Box.Padding = layout.Uniform(1)
	root := headless.NewRoot(panel)
	root.Draw(grid.NewSurface(12, 6).View())

	panel.Focus(true)
	if !child.focused {
		t.Fatal("panel did not pass keyboard ownership to its child")
	}
	if panel.Handle(input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDown}) {
		t.Fatal("panel passed a press on its border to its child")
	}
	if !panel.Handle(input.Mouse{Pos: image.Pt(3, 2), Action: input.MouseDown}) {
		t.Fatal("panel declined a press inside its content")
	}
	got, ok := child.event.(input.Mouse)
	if !ok || got.Pos != image.Pt(1, 0) {
		t.Fatalf("child received %#v, want a pointer at its local coordinate (1,0)", child.event)
	}
}

func TestPanelTransfersFocusWhenItsContentChanges(t *testing.T) {
	first := &panelChild{}
	second := &panelChild{}
	panel := kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: kit.Theme{}, Glyphs: kit.Unicode()}, Content: first})
	if panel.Content() != first || !first.focused {
		t.Fatal("a new panel did not give its content the keyboard")
	}

	panel.Focus(false)
	panel.SetContent(second)
	if first.focused || second.focused {
		t.Fatal("content installed in a blurred panel received the keyboard")
	}

	panel.Focus(true)
	panel.SetContent(first)
	if second.focused || !first.focused {
		t.Fatal("replacing focused content did not transfer keyboard ownership")
	}
}

// TestPanelKeepsAGestureThatLeavesItsInterior is the rule a panel inherits rather
// than invents: a press decides who owns the interaction, and the drag and release
// that follow belong to that owner wherever the pointer goes. Testing the interior
// on every event would stop a selection at the frame and, worse, never deliver the
// release, leaving the child believing it is still being dragged.
func TestPanelKeepsAGestureThatLeavesItsInterior(t *testing.T) {
	child := &panelChild{}
	panel := kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: kit.Theme{}, Glyphs: kit.Unicode()}, Content: child})
	headless.NewRoot(panel).Draw(grid.NewSurface(10, 6).View())

	press := input.Mouse{Pos: image.Pt(3, 3), Action: input.MouseDown}
	if !panel.Handle(press) {
		t.Fatal("panel declined a press inside its content")
	}
	for _, away := range []input.Mouse{
		{Pos: image.Pt(0, 3), Action: input.MouseDrag}, // onto the frame
		{Pos: image.Pt(4, 0), Action: input.MouseDrag}, // past it
	} {
		if !panel.Handle(away) {
			t.Fatalf("panel dropped a drag at %v that began inside it", away.Pos)
		}
	}
	release := input.Mouse{Pos: image.Pt(4, 0), Action: input.MouseUp}
	if !panel.Handle(release) {
		t.Fatal("panel dropped the release that ends the gesture")
	}
	// The gesture is over, so the interior decides again.
	if panel.Handle(input.Mouse{Pos: image.Pt(0, 3), Action: input.MouseDrag}) {
		t.Fatal("panel kept the gesture after its release")
	}
}

// TestPanelDoesNotTakeAGestureItsChildRefused keeps the hold honest: a press the
// child declined is not an interaction, and holding one would swallow the release
// belonging to whatever the pointer is really over.
func TestPanelDoesNotTakeAGestureItsChildRefused(t *testing.T) {
	child := &panelChild{deaf: true}
	panel := kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: kit.Theme{}, Glyphs: kit.Unicode()}, Content: child})
	headless.NewRoot(panel).Draw(grid.NewSurface(10, 6).View())

	if panel.Handle(input.Mouse{Pos: image.Pt(3, 3), Action: input.MouseDown}) {
		t.Fatal("panel claimed a press its child refused")
	}
	if panel.Handle(input.Mouse{Pos: image.Pt(4, 0), Action: input.MouseDrag}) {
		t.Fatal("panel held a gesture its child never took")
	}
}

func TestBoxTitleSitsInTheBorder(t *testing.T) {
	rows := paint(12, 2, func(v grid.View) {
		kit.Box{Glyphs: kit.Unicode(), Title: "Plan"}.Draw(v)
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
			kit.Box{Glyphs: kit.Unicode(), Title: "title", Footer: "footer", Padding: layout.Uniform(1)}.Draw(v)
		})
	}
}

func TestLabelTruncatesRatherThanWraps(t *testing.T) {
	// One row is all a label ever has. Folding would push whatever is below it off
	// the screen.
	rows := paint(6, 2, func(v grid.View) {
		kit.Label{Text: "far too long", Ellipsis: "…"}.Draw(v)
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
			kit.Label{Text: "ab", Align: tc.align}.Draw(v)
		})
		equalRows(t, rows, []string{tc.want})
	}
}

func TestParagraphHeightFollowsWidth(t *testing.T) {
	p := kit.NewParagraph("one two three four", grid.Style{})
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

func TestParagraphMeasuresContentBeforeItHasDrawableWidth(t *testing.T) {
	p := kit.NewParagraph("one two\nthree", grid.Style{})
	p.Indent = 3
	want := p.Measure(p.Indent + 1)
	for _, width := range []int{-1, 0, 1, p.Indent} {
		if got := p.Measure(width); got != want {
			t.Errorf("height at width %d = %d, want one-column height %d", width, got, want)
		}
	}
}

func TestParagraphKeepsNewlinesAsLineBreaks(t *testing.T) {
	p := kit.NewParagraph("first\nsecond", grid.Style{})
	if got := p.Measure(20); got != 2 {
		t.Fatalf("height = %d, want a row per line", got)
	}
}

func TestParagraphIndentsEveryRow(t *testing.T) {
	p := kit.NewParagraph("one two three", grid.Style{})
	p.Indent = 2
	rows := paint(7, 3, func(v grid.View) { p.Draw(v) })
	equalRows(t, rows, []string{"..one..", "..two..", "..three"})
}

func TestParagraphCapsItsHeight(t *testing.T) {
	p := kit.NewParagraph("one two three four five", grid.Style{})
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

func TestParagraphRewrapsWhenItsRowCapChanges(t *testing.T) {
	p := kit.NewParagraph("one two three four five", grid.Style{})
	if got := p.Measure(6); got <= 2 {
		t.Fatalf("uncapped height = %d, want more than two rows", got)
	}
	p.MaxRows = 2
	if got := p.Measure(6); got != 2 {
		t.Fatalf("height after changing the cap = %d, want 2", got)
	}
}

func TestParagraphRewrapsWhenItsTextChanges(t *testing.T) {
	// The wrap is memoised because it is asked for twice a frame. A memo that
	// outlived its content would show the old text forever.
	p := kit.NewParagraph("short", grid.Style{})
	if got := p.Measure(20); got != 1 {
		t.Fatalf("height = %d", got)
	}
	p.SetText(kit.NewParagraph("one\ntwo\nthree", grid.Style{}).Lines())
	if got := p.Measure(20); got != 3 {
		t.Fatalf("height after the text changed = %d, want 3", got)
	}
}

func TestParagraphOwnsItsTextAndReturnsSnapshots(t *testing.T) {
	lines := []text.Line{text.Of("original", grid.Style{})}
	var paragraph kit.Paragraph
	paragraph.SetText(lines)
	lines[0][0].Text = "changed input"
	if got := paragraph.Lines()[0].String(); got != "original" {
		t.Fatalf("paragraph text = %q after caller mutation", got)
	}

	snapshot := paragraph.Lines()
	snapshot[0][0].Text = "changed snapshot"
	if got := paragraph.Lines()[0].String(); got != "original" {
		t.Fatalf("paragraph text = %q after snapshot mutation", got)
	}
}

func TestSpinnerAdvancesOnlyWhenTold(t *testing.T) {
	// It holds a frame number, not a clock: an idle UI must not wake up to animate
	// something nobody is waiting for.
	s := &kit.Spinner{Glyphs: kit.Glyphs{Spinner: []string{"a", "b"}}, Label: "working"}
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

func TestBuiltInSpinnerFramesAreIndependent(t *testing.T) {
	first := kit.Unicode()
	first.Spinner[0] = "changed"
	if got := kit.Unicode().Spinner[0]; got == "changed" {
		t.Fatal("mutating one glyph set changed the built-in spinner")
	}
}

func TestSpinnerDropsALabelThatDoesNotFit(t *testing.T) {
	s := &kit.Spinner{Glyphs: kit.Glyphs{Spinner: []string{"a"}}, Label: "far too long to fit"}
	rows := paint(3, 1, func(v grid.View) { s.Draw(v) })
	if !strings.HasPrefix(rows[0], "a") {
		t.Fatalf("row = %q, want the glyph", rows[0])
	}
}

func TestScrollbarThumbTracksThePosition(t *testing.T) {
	glyphs := kit.Glyphs{ScrollTrack: "-", ScrollThumb: "#"}
	top := paint(1, 4, func(v grid.View) {
		kit.Scrollbar{Total: 40, Window: 4, Offset: 0, Glyphs: glyphs}.Draw(v)
	})
	if top[0] != "#" || top[3] != "-" {
		t.Fatalf("at the top the bar is %v, want the thumb at the top", top)
	}
	bottom := paint(1, 4, func(v grid.View) {
		kit.Scrollbar{Total: 40, Window: 4, Offset: 36, Glyphs: glyphs}.Draw(v)
	})
	if bottom[3] != "#" || bottom[0] != "-" {
		t.Fatalf("at the end the bar is %v, want the thumb at the bottom", bottom)
	}
}

func TestScrollbarThumbNeverRoundsAway(t *testing.T) {
	// A thumb rounded down to nothing tells the user nothing.
	rows := paint(1, 4, func(v grid.View) {
		kit.Scrollbar{Total: 10000, Window: 4, Offset: 5000, Glyphs: kit.ASCII()}.Draw(v)
	})
	if !strings.Contains(strings.Join(rows, ""), "#") {
		t.Fatalf("bar = %v, want a thumb somewhere in it", rows)
	}
}

func TestScrollbarUsesOverflowSafeClampedGeometry(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	rows := paint(1, 4, func(v grid.View) {
		kit.Scrollbar{
			Total: maxInt, Window: maxInt - 1, Offset: -maxInt, Glyphs: kit.ASCII(),
		}.Draw(v)
	})
	if rows[0] != "#" {
		t.Fatalf("negative offset put the thumb away from the top: %v", rows)
	}
	if got := strings.Count(strings.Join(rows, ""), "#"); got != 3 {
		t.Fatalf("large proportional thumb occupies %d rows, want 3: %v", got, rows)
	}
}

func TestScrollbarKnowsWhenItIsPointless(t *testing.T) {
	if (kit.Scrollbar{Total: 3, Window: 10}).Needed() {
		t.Error("a bar claims to be needed when everything already fits")
	}
	if !(kit.Scrollbar{Total: 30, Window: 10}).Needed() {
		t.Error("a bar does not know it is needed")
	}
}

func TestHelpShowsWhatFitsAndDropsTheRest(t *testing.T) {
	keys := &keymap.Map{}
	keys.Bind("send", input.Chord{Code: input.Enter})
	keys.Bind("quit", input.Ctrl.Rune('c'))
	keys.Bind("tasks", input.Ctrl.Rune('g'))
	help := kit.Help{Keys: keys, Show: []keymap.Action{"send", "quit", "tasks"}}
	if got := help.Measure(40); got != 1 {
		t.Fatalf("Help.Measure = %d, want its one display row", got)
	}
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

func TestHelpSkipsAnActionNobodyCanPress(t *testing.T) {
	// A hint for a key that is not bound is a hint that is wrong. There is no flag for
	// hiding one either: an action that works and that nobody needs told about is
	// simply not listed.
	keys := &keymap.Map{}
	keys.Bind("send", input.Chord{Code: input.Enter})
	help := kit.Help{Keys: keys, Show: []keymap.Action{"send", "secret"}}
	rows := paint(40, 1, func(v grid.View) { help.Draw(v) })
	if strings.Contains(rows[0], "secret") {
		t.Fatalf("row = %q, want the unbound action left out", rows[0])
	}
	if !strings.Contains(rows[0], "enter send") {
		t.Fatalf("row = %q, want the bound one shown", rows[0])
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
	table := kit.Table{Columns: []kit.Column{
		{Size: layout.Fixed(6)}, {Size: layout.Flex(1)}, {Size: layout.Flex(2)},
	}, Gap: 1}
	widths := table.Layout(30).Widths()
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

func TestTableMeasureNormalizesInvalidRowCounts(t *testing.T) {
	if got := (kit.Table{Rows: -3}).Measure(10); got != 0 {
		t.Fatalf("negative rows measured %d, want 0", got)
	}
	if got := (kit.Table{Rows: -3, Header: true}).Measure(10); got != 1 {
		t.Fatalf("header over negative rows measured %d, want 1", got)
	}
}

func TestTableLayoutOwnsTheColumnDefinitionItMeasured(t *testing.T) {
	columns := []kit.Column{{Title: "old", Size: layout.Fixed(4)}}
	tableLayout := (kit.Table{Columns: columns}).Layout(6)
	columns[0].Title = "new"
	columns[0].Size = layout.Fixed(1)

	surface := grid.NewSurface(6, 1)
	tableLayout.Titles(surface.View())
	var rendered strings.Builder
	for x := range 6 {
		rendered.WriteString(cellAt(surface, x, 0).Content)
	}
	if got := rendered.String(); !strings.HasPrefix(got, "old") || strings.Contains(got, "new") {
		t.Fatalf("layout drew %q after its source columns changed", got)
	}
	if got := tableLayout.Widths(); len(got) != 1 || got[0] != 4 {
		t.Fatalf("layout widths after source mutation = %v, want [4]", got)
	}
}

func TestTableDrawsOnlyRowsInsideTheClip(t *testing.T) {
	var styled, painted []int
	table := kit.Table{
		Columns: []kit.Column{{Size: layout.Fixed(8)}},
		Rows:    100,
		RowStyle: func(row int) grid.Style {
			styled = append(styled, row)
			return grid.Style{}
		},
		Cell: func(row, _ int) kit.Cell {
			painted = append(painted, row)
			return kit.Cell{}
		},
	}
	surface := grid.NewSurface(8, 3)
	table.Draw(surface.View().Sub(grid.Rect(0, -50, 8, 100)))

	want := []int{50, 51, 52}
	if !reflect.DeepEqual(styled, want) {
		t.Fatalf("styled rows = %v, want %v", styled, want)
	}
	if !reflect.DeepEqual(painted, want) {
		t.Fatalf("painted rows = %v, want %v", painted, want)
	}
	table.Draw(surface.View().Sub(grid.Rect(0, -200, 8, 100)))
	if !reflect.DeepEqual(styled, want) || !reflect.DeepEqual(painted, want) {
		t.Fatal("a fully clipped table evaluated hidden rows")
	}
}

func TestTableFlexibleColumnsHaveAFloor(t *testing.T) {
	table := kit.Table{Columns: []kit.Column{
		{Size: layout.Fixed(8)}, {Size: layout.Flex(1).AtLeast(4)},
	}, Gap: 1}
	if widths := table.Layout(22).Widths(); widths[1] < 4 {
		t.Fatalf("widths %v, want the flexible column to keep its floor", widths)
	}

	// And keeps it only while there is room for it. A floor that was handed out
	// anyway would tell the column it had a width the view then clipped, which the
	// column cannot see and lays its cells out against.
	squeezed := kit.Table{Columns: []kit.Column{
		{Size: layout.Fixed(20)}, {Size: layout.Flex(1).AtLeast(4)},
	}, Gap: 1}
	widths := squeezed.Layout(22).Widths()
	if total := widths[0] + widths[1] + 1; total > 22 {
		t.Fatalf("widths %v and the gap come to %d, which is more than the table has", widths, total)
	}
}

func TestTableFitsAColumnToItsWidestCell(t *testing.T) {
	data := [][]string{{"a", "one"}, {"longest", "two"}}
	table := kit.Table{
		Columns: []kit.Column{{Title: "name", Size: layout.Measured(0, 0)}, {Title: "value"}},
		Rows:    len(data),
		Header:  true,
		Cell: func(row, column int) kit.Cell {
			return kit.LabelCell(kit.Label{Text: data[row][column], Ellipsis: "…"})
		},
	}

	layout := table.Layout(20)
	if got := layout.Widths(); !reflect.DeepEqual(got, []int{7, 12}) {
		t.Fatalf("widths = %v, want the first column fitted to %q", got, "longest")
	}
	equalRows(t, paint(20, 3, table.Draw), []string{
		"name....value.......",
		"a.......one.........",
		"longest.two.........",
	})
}

func TestTableCapsAContentFittedColumn(t *testing.T) {
	table := kit.Table{
		Columns: []kit.Column{{Size: layout.Measured(0, 5)}, {Size: layout.Flex(1)}},
		Rows:    1,
		Cell: func(_, column int) kit.Cell {
			return kit.LabelCell(kit.Label{Text: []string{"far too long", "rest"}[column], Ellipsis: "…"})
		},
	}
	if got := table.Layout(12).Widths(); !reflect.DeepEqual(got, []int{5, 6}) {
		t.Fatalf("widths = %v, want the fitted column capped at five", got)
	}
}

func TestTableFitMeasuresOnlyTheSortMarkItDraws(t *testing.T) {
	table := kit.Table{
		Columns: []kit.Column{{Title: "x", Size: layout.Measured(0, 0)}, {Size: layout.Flex(1)}},
		Sorted:  func() (int, bool, bool) { return 0, false, false },
		Glyphs:  kit.Glyphs{Ascending: "very-wide", Descending: "also-wide"},
	}
	if got := table.Layout(12).Widths(); !reflect.DeepEqual(got, []int{1, 10}) {
		t.Fatalf("widths = %v, want only the visible title measured", got)
	}

	table.Sorted = func() (int, bool, bool) { return 0, false, true }
	if got := table.Layout(20).Widths()[0]; got != len("xvery-wide") {
		t.Fatalf("sorted fitted width = %d, want the title and its visible mark", got)
	}
}

func TestLabelCellDoesNotRetainARowStyleBetweenDraws(t *testing.T) {
	cell := kit.LabelCell(kit.Label{Text: "x"})
	first := grid.NewSurface(1, 1)
	cell.Draw(first.View(), grid.Style{BG: grid.RGBColor(1, 2, 3)})
	secondStyle := grid.Style{BG: grid.RGBColor(4, 5, 6)}
	second := grid.NewSurface(1, 1)
	cell.Draw(second.View(), secondStyle)

	if got := cellAt(second, 0, 0).Style.BG; got != secondStyle.BG {
		t.Fatalf("second row kept background %+v from the first draw", got)
	}
}

func TestTableDrawsHeaderAndRows(t *testing.T) {
	table := kit.Table{
		Columns: []kit.Column{{Title: "id", Size: layout.Fixed(4)}, {Title: "name"}},
		Rows:    2,
		Header:  true,
		Cell: func(row, col int) kit.Cell {
			data := [][]string{{"a1", "alpha"}, {"b2", "bravo"}}
			return kit.LabelCell(kit.Label{Text: data[row][col]})
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
	table := kit.Table{
		Columns: []kit.Column{{Size: layout.Fixed(5)}, {}},
		Rows:    1,
		Cell: func(_, col int) kit.Cell {
			_ = col
			return kit.LabelCell(kit.Label{Text: "far too long", Ellipsis: "…"})
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
	table := kit.Table{
		Columns:  []kit.Column{{}},
		Rows:     2,
		RowStyle: func(row int) grid.Style { return map[bool]grid.Style{true: selected}[row == 1] },
		Cell: func(_, _ int) kit.Cell {
			return kit.LabelCell(kit.Label{Text: "x"})
		},
	}
	s := grid.NewSurface(6, 2)
	table.Draw(s.View())
	for x := range 6 {
		if got := cellAt(s, x, 1).Style.BG; got != selected.BG {
			t.Fatalf("column %d of the banded row = %+v, want the row style across the whole row", x, got)
		}
	}
}

func TestOverlayDrawsIntoWhereItSaidItWould(t *testing.T) {
	// Area and Draw have to agree, or a hit test a frame later answers about the wrong
	// place.
	o := kit.Overlay{Anchor: layout.Middle, Width: 4, Height: 1}
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
	// is still there rather than gone. Half way to black is a colour that is neither
	// what it was nor what the sheet is made of, which is what mixing means.
	s := grid.NewSurface(8, 2)
	s.View().Text(0, 0, "behind", grid.Style{FG: grid.RGBColor(0xFF, 0xFF, 0xFF)})
	theme := kit.Theme{Scrim: kit.Scrim{Color: grid.RGBColor(0, 0, 0), Opacity: 0.5}}
	kit.Overlay{Width: 2, Height: 1, Theme: theme}.Draw(s.View())

	if got := cellAt(s, 0, 0).Content; got != "b" {
		t.Fatalf("cell = %q, want what was behind still there", got)
	}
	if got := cellAt(s, 0, 0).Style.FG.RGB(); got != (grid.RGB{R: 128, G: 128, B: 128}) {
		t.Fatalf("what is behind is %+v, want it half way to the shade", got)
	}
}

// TestOverlayShadeNeedsToKnowWhatTheTerminalDrawsOn is the end of the chain that
// begins with the startup probe. A cell nobody coloured has no numbers of its own,
// so what it mixes to depends entirely on the terminal having said what it draws
// with — and where it did not say, the honest outcome is that nothing changes.
func TestOverlayShadeNeedsToKnowWhatTheTerminalDrawsOn(t *testing.T) {
	theme := kit.Theme{Scrim: kit.Scrim{Color: grid.RGBColor(0, 0, 0), Opacity: 0.5}}
	overlay := kit.Overlay{Width: 2, Height: 1, Theme: theme}

	unasked := grid.NewSurface(8, 2)
	unasked.View().Text(0, 0, "behind", grid.Style{})
	overlay.Draw(unasked.View())
	if got := cellAt(unasked, 0, 0).Style; got != (grid.Style{}) {
		t.Errorf("a cell over an unknown terminal became %+v, want it left alone", got)
	}

	asked := grid.NewSurface(8, 2)
	asked.SetGround(grid.Ground{
		FG: grid.RGBColor(0xFF, 0xFF, 0xFF),
		BG: grid.RGBColor(0x20, 0x20, 0x20),
	})
	asked.View().Text(0, 0, "behind", grid.Style{})
	overlay.Draw(asked.View())
	if got := cellAt(asked, 0, 0).Style.FG.RGB(); got != (grid.RGB{R: 128, G: 128, B: 128}) {
		t.Errorf("text over a known terminal is %+v, want it half way to the shade", got)
	}
	if got := cellAt(asked, 0, 0).Style.BG.RGB(); got != (grid.RGB{R: 16, G: 16, B: 16}) {
		t.Errorf("the background is %+v, want it half way to the shade", got)
	}
}

// linkedCells is the target on every column of a row, so a test can state exactly
// which columns became clickable.
func linkedCells(v grid.View, y, width int) []string {
	out := make([]string, width)
	for x := range width {
		out[x] = cellAt(v, x, y).Link
	}
	return out
}

func TestParagraphMakesURLsClickable(t *testing.T) {
	s := grid.NewSurface(30, 3)
	p := kit.NewParagraph("go to https://a.test now", grid.Style{})
	p.SetLinks(kit.LinkConfig{Enabled: true})
	p.Draw(s.View())

	got := linkedCells(s.View(), 0, 30)
	for x := range len("go to ") {
		if got[x] != "" {
			t.Errorf("column %d, before the URL, is linked to %q", x, got[x])
		}
	}
	for x := len("go to "); x < len("go to https://a.test"); x++ {
		if got[x] != "https://a.test" {
			t.Errorf("column %d = %q, want the URL", x, got[x])
		}
	}
	if after := len("go to https://a.test"); got[after] != "" {
		t.Errorf("the space after the URL is linked to %q", got[after])
	}
}

func TestParagraphLeavesTextAloneUnlessAsked(t *testing.T) {
	// Text a program composed itself has no URLs worth finding, and scanning every
	// line on every width change is not free.
	s := grid.NewSurface(30, 3)
	kit.NewParagraph("go to https://a.test now", grid.Style{}).Draw(s.View())
	for x, target := range linkedCells(s.View(), 0, 30) {
		if target != "" {
			t.Fatalf("column %d is linked without being asked: %q", x, target)
		}
	}
}

// TestParagraphKeepsAWrappedURLWhole is the case that makes detecting on the logical
// line necessary. Read row by row, the first half of a split URL is a shorter address
// that resolves somewhere else, and a hyperlink to the wrong page is worse than none.
func TestParagraphKeepsAWrappedURLWhole(t *testing.T) {
	const url = "https://example.com/a/long/path"
	s := grid.NewSurface(12, 6)
	p := kit.NewParagraph(url, grid.Style{})
	p.SetLinks(kit.LinkConfig{Enabled: true})
	p.Draw(s.View())

	v, rows := s.View(), 0
	for y := range 6 {
		linked := 0
		for _, target := range linkedCells(v, y, 12) {
			if target == "" {
				continue
			}
			linked++
			if target != url {
				t.Fatalf("row %d links to %q, want the whole URL", y, target)
			}
		}
		if linked > 0 {
			rows++
		}
	}
	if rows < 3 {
		t.Fatalf("the URL was linked on %d rows, want it split across several", rows)
	}
}

func TestParagraphAnswersAClick(t *testing.T) {
	s := grid.NewSurface(30, 3)
	p := kit.NewParagraph("go to https://a.test now", grid.Style{})
	p.SetLinks(kit.LinkConfig{Enabled: true})
	p.Draw(s.View())

	if got, ok := p.LinkAt(8, 0, 30); !ok || got.Target != "https://a.test" {
		t.Errorf("a click inside the URL found %+v (%v)", got, ok)
	}
	if _, ok := p.LinkAt(1, 0, 30); ok {
		t.Error("a click before the URL found one")
	}
	if _, ok := p.LinkAt(8, 1, 30); ok {
		t.Error("a click on an empty row found one")
	}
}

func TestParagraphHitTestingReadsCurrentTextWithoutDrawHistory(t *testing.T) {
	// A passive block has no committed geometry cache: replacing its content changes
	// the pure layout query immediately and does not require a clearing draw.
	s := grid.NewSurface(30, 3)
	p := kit.NewParagraph("go to https://a.test now", grid.Style{})
	p.SetLinks(kit.LinkConfig{Enabled: true})
	p.Draw(s.View())
	if _, ok := p.LinkAt(8, 0, 30); !ok {
		t.Fatal("no link was recorded to begin with")
	}

	p.SetText(nil)
	if got, ok := p.LinkAt(8, 0, 30); ok {
		t.Errorf("a click still finds %+v after the text was replaced", got)
	}
}

// TestParagraphDoesNotLinkATruncatedRow: a row cut off with an ellipsis no longer
// draws the text its range describes, so a link stamped from that range would land
// on the ellipsis rather than on any address.
func TestParagraphDoesNotLinkATruncatedRow(t *testing.T) {
	s := grid.NewSurface(20, 2)
	p := kit.NewParagraph("first line here\nhttps://a.test is on the second row", grid.Style{})
	p.SetLinks(kit.LinkConfig{Enabled: true})
	p.MaxRows = 2
	p.Draw(s.View())

	v := s.View()
	for y := range 2 {
		for x, target := range linkedCells(v, y, 20) {
			if target == "" {
				continue
			}
			if c := cellAt(v, x, y); c.Content == "…" {
				t.Errorf("the ellipsis at (%d,%d) is linked to %q", x, y, target)
			}
		}
	}
}

// TestParagraphCopiesAsAParagraph is the end of the chain that began with the wrap
// recording where each row came from. What the user copies out of a narrow window
// has to paste as the text that was written, not as a column of fragments.
func TestParagraphCopiesAsAParagraph(t *testing.T) {
	var tr headless.Transcript
	p := kit.NewParagraph("the quick brown fox jumps over it\nand a second line", grid.Style{})
	stageContent(&tr, 12)
	tr.Append(p)

	if tr.Height() < 4 {
		t.Fatalf("the text wrapped to %d rows, want several", tr.Height())
	}

	var s headless.Selection
	s.Begin(headless.Point{Row: 0, Col: 0})
	s.Extend(headless.Point{Row: tr.Height() - 1, Col: 999})

	want := "the quick brown fox jumps over it\nand a second line"
	if got := s.Text(&tr); got != want {
		t.Errorf("copied\n  %q,\nwant\n  %q", got, want)
	}
}

// TestParagraphCopiesAWordItHadToBreak: a word longer than the row is split between
// clusters and the break swallows nothing, so putting a space back would break the
// word that was copied.
func TestParagraphCopiesAWordItHadToBreak(t *testing.T) {
	const word = "supercalifragilisticexpialidocious"
	var tr headless.Transcript
	stageContent(&tr, 10)
	tr.Append(kit.NewParagraph(word, grid.Style{}))

	var s headless.Selection
	s.Begin(headless.Point{Row: 0, Col: 0})
	s.Extend(headless.Point{Row: tr.Height() - 1, Col: 999})
	if got := s.Text(&tr); got != word {
		t.Errorf("copied %q, want %q", got, word)
	}
}

// TestTheComposerShowsASelection. Behaviour with no appearance is the arrangement
// between the two packages; a selection nobody can see is not that arrangement, it is
// a feature half finished.
func TestTheComposerShowsASelection(t *testing.T) {
	th := kit.Dark()
	c := &kit.Composer{Theme: th, Prompt: "> "}
	c.SetText("hello world")
	c.Editor().SetCursor(0, 0)
	for range 5 {
		c.Handle(input.Key{Code: input.Right, Mods: input.Shift})
	}

	s := grid.NewSurface(30, 3)
	headless.NewRoot(c).Draw(s.View())

	marked := 0
	for y := range 3 {
		for x := range 30 {
			if cell := cellAt(s.View(), x, y); cell.Style == th.Text.Merge(th.Selection) {
				marked++
			}
		}
	}
	if marked != 5 {
		t.Errorf("%d cells are marked, want the five selected", marked)
	}
}

// TestParagraphMakesAPathClickable. In an agent's output the file is the commoner
// destination, and the line number after it is the useful half.
func TestParagraphMakesAPathClickable(t *testing.T) {
	s := grid.NewSurface(40, 3)
	p := kit.NewParagraph("edited src/main.go:42 for you", grid.Style{})
	lookups := 0
	p.SetLinks(kit.LinkConfig{
		Enabled: true,
		Exists: func(path string) bool {
			lookups++
			return path == "src/main.go"
		},
	})
	detected := lookups
	p.Measure(40)
	p.Draw(s.View())

	destination, ok := p.LinkAt(len("edited "), 0, 40)
	if !ok {
		t.Fatal("the path was not recorded")
	}
	if destination.Target != "src/main.go" || destination.Line != 42 {
		t.Errorf("a click found %+v, want the path at line 42", destination)
	}
	if lookups != detected {
		t.Fatalf("Measure, Draw or LinkAt performed %d new filesystem lookups", lookups-detected)
	}
	// A relative path gets no OSC 8: the terminal knows the directory and offers to
	// open it in the editor the user actually uses.
	for x := range 40 {
		if c := cellAt(s.View(), x, 0); c.Link != "" {
			t.Errorf("column %d carries the hyperlink %q", x, c.Link)
			break
		}
	}
}

func TestParagraphEscapesAbsoluteFileHyperlinks(t *testing.T) {
	s := grid.NewSurface(50, 1)
	p := kit.NewParagraph(`open "/Applications/Demo App.app"`, grid.Style{})
	p.SetLinks(kit.LinkConfig{Enabled: true})
	p.Draw(s.View())

	want := "file:///Applications/Demo%20App.app"
	found := false
	for _, target := range linkedCells(s.View(), 0, 50) {
		if target == "" {
			continue
		}
		found = true
		if target != want {
			t.Fatalf("file hyperlink = %q, want %q", target, want)
		}
	}
	if !found {
		t.Fatal("absolute file path was not hyperlinked")
	}
}

func TestParagraphStillHyperlinksAURL(t *testing.T) {
	s := grid.NewSurface(40, 3)
	p := kit.NewParagraph("see https://a.test now", grid.Style{})
	p.SetLinks(kit.LinkConfig{Enabled: true})
	p.Draw(s.View())

	if c := cellAt(s.View(), len("see "), 0); c.Link != "https://a.test" {
		t.Errorf("the first column of the URL carries %+v", c)
	}
}

// TestTheComposerCanBeClickedInto. The field learned the mouse; a composer that did
// not pass it on would leave the default widget unable to do the first thing anybody
// tries.
func TestTheComposerCanBeClickedInto(t *testing.T) {
	c := &kit.Composer{Theme: kit.Dark(), Prompt: "> "}
	c.SetText("hello world")
	c.Editor().SetCursor(0, 0)

	s := grid.NewSurface(30, 3)
	headless.NewRoot(c).Draw(s.View())

	// Column 8 on screen is column 6 of the field, because the marker takes two.
	if !c.Handle(input.Mouse{Pos: image.Pt(8, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("the composer ignored a click")
	}
	if _, col := c.Editor().Cursor(); col != 6 {
		t.Errorf("the cursor is at %d, want 6 — the marker's width was not taken off", col)
	}
}

func TestTheComposerIgnoresAClickBeforeItHasBeenDrawn(t *testing.T) {
	// A click is about a frame, and there has not been one.
	c := &kit.Composer{Theme: kit.Dark()}
	c.SetText("hello")
	if c.Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Error("it answered a click about a frame that was never drawn")
	}
}

func TestTheStripSaysWhichPaneIsShowingAndAPressChangesIt(t *testing.T) {
	tabs := kit.NewTabs(kit.TabsConfig{
		Theme: kit.Dark(), Glyphs: kit.ASCII(),
		Items: []headless.Tab{
			{Title: "chat", Of: headless.Static{Of: kit.Label{Text: "one"}}},
			{Title: "files", Of: headless.Static{Of: kit.Label{Text: "two"}}},
		},
	})
	equalRows(t, paintWidget(14, 4, tabs), []string{
		"chat..files...",
		"--------------",
		"one...........",
		"..............",
	})

	// A press on a name selects it; a press below the strip belongs to the pane, in
	// the pane's own coordinates.
	if !tabs.Handle(input.Mouse{Pos: image.Pt(7, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("a press on a name did nothing")
	}
	if tabs.Controller().Selected() != 1 {
		t.Fatalf("the pane showing is %d", tabs.Controller().Selected())
	}
	if _, on := tabs.At(5); on {
		t.Fatal("the room between two names belongs to one of them")
	}
}

func TestNewTabsProvidesTheFinishedStripAndItsController(t *testing.T) {
	tabs := kit.NewTabs(kit.TabsConfig{
		Theme: kit.Dark(), Glyphs: kit.ASCII(),
		Items: []headless.Tab{
			{Title: "chat", Of: headless.Static{Of: kit.Label{Text: "one"}}},
			{Title: "files", Of: headless.Static{Of: kit.Label{Text: "two"}}},
		},
	})
	if tabs.Controller() == nil || !tabs.Rule {
		t.Fatal("the short constructor omitted its controller or finished default rule")
	}
	tabs.Controller().Select(1)
	if got := paintWidget(14, 4, tabs); !strings.Contains(strings.Join(got, "\n"), "two") {
		t.Fatalf("composed tabs drew %v", got)
	}
}

func TestATreeIsDrawnAsFarInAsItIsDeep(t *testing.T) {
	tree := headless.NewTree(
		headless.Node[string]{Item: "core", Children: []headless.Node[string]{{Item: "grid"}}},
		headless.Node[string]{Item: "README"},
	)
	tree.Open(0)
	view := kit.NewTree(kit.TreeConfig[string]{
		Theme: kit.Dark(), Glyphs: kit.ASCII(), Controller: tree,
		Text: func(s string) string { return s },
	})
	// A leaf starts in the same column as a branch — the mark is a blank as wide as
	// one — so a tree of files does not read as a tree of two different things.
	equalRows(t, paintWidget(12, 3, view), []string{
		"-.core......",
		".. .grid....",
		" .README....",
	})
}

func TestNilTreeIsAnEmptyWidget(t *testing.T) {
	var tree *kit.Tree[string]
	if tree.Controller() != nil || tree.Focused() || tree.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("a nil tree reported controller, focus, or handled input")
	}
	if got := tree.Measure(20); got != 0 {
		t.Fatalf("nil tree measured %d rows, want zero", got)
	}
	paintWidget(20, 1, tree)
}

func TestADressedTreeDoesNotReplaceTheControllersRenderer(t *testing.T) {
	called := 0
	controller := headless.NewTree(headless.Node[string]{Item: "root"})
	controller.Row = func(grid.View, int, headless.Shown[string], bool) {
		called++
	}
	dressed := kit.NewTree(kit.TreeConfig[string]{
		Controller: controller, Text: func(item string) string { return item },
	})
	paintWidget(8, 1, dressed)
	if called != 0 {
		t.Fatal("the dressed tree used the controller's unrelated row appearance")
	}
	paintWidget(8, 1, controller)
	if called != 1 {
		t.Fatal("drawing the dressed tree replaced the controller's row appearance")
	}
}

func TestAFormCanBeAnsweredWithoutAScreen(t *testing.T) {
	// The same form, the same values, the same complaints — and no grid. What a
	// screen reader has, what a pipe has, and what a test that would rather say
	// "good" than press the down arrow twice has.
	var (
		name  string
		model string
		sure  bool
	)
	modelField := &headless.Select[string]{Label: "Model", Value: headless.Bind(&model)}
	modelField.SetOptions(headless.Options("fast", "good"))
	form := headless.NewForm(
		&headless.Text{Label: "Name", Value: headless.Bind(&name), Check: func(s string) error {
			if s == "" {
				return errors.New("a name is needed")
			}
			return nil
		}},
		modelField,
		&headless.Confirm{Label: "Sure?", Value: headless.Bind(&sure)},
	)

	var out strings.Builder
	said := strings.NewReader("\nada\ngo\ny\n")
	if err := kit.Ask(form, said, &out); err != nil {
		t.Fatalf("answering the form: %v", err)
	}
	if name != "ada" || model != "good" || !sure {
		t.Fatalf("collected %q %q %v", name, model, sure)
	}

	// The questions say what may be said, and a refused answer is put again rather
	// than taken.
	written := out.String()
	for _, want := range []string{"Model: 1) fast 2) good", "Sure? (yes/no)", "a name is needed"} {
		if !strings.Contains(written, want) {
			t.Fatalf("the conversation was:\n%s\nwant %q in it", written, want)
		}
	}
}

func TestAPressOnAHeadingSortsTheRowsUnderIt(t *testing.T) {
	// The two halves meeting: the geometry is the table's, the order is the rows',
	// and a press is turned into a column by the one that knows where the columns
	// went.
	rows := new(headless.Table[[2]string])
	rows.SetLess(func(a, b [2]string, column int) bool { return a[column] < b[column] })
	rows.SetItems([][2]string{{"b", "2"}, {"a", "1"}})

	view := kit.Table{
		Theme:   kit.Dark(),
		Glyphs:  kit.ASCII(),
		Columns: []kit.Column{{Title: "name"}, {Title: "size"}},
		Rows:    rows.Len(),
		Sorted:  rows.Sorted,
		Header:  true,
		Cell: func(row, column int) kit.Cell {
			item, _ := rows.At(row)
			return kit.LabelCell(kit.Label{Text: item[column]})
		},
	}

	columns := view.Layout(20)
	column, on := columns.ColumnAt(2)
	if !on || column != 0 {
		t.Fatalf("a press at column 2 landed on column %d, on a heading %v", column, on)
	}
	rows.SortBy(column)
	first, _ := rows.At(0)
	if first[0] != "a" {
		t.Fatalf("after sorting by the pressed column the first row is %q", first[0])
	}
	// And the heading says so, which is the only way a reader can tell an order from
	// a coincidence.
	if drawn := paint(20, 1, columns.Titles); !strings.Contains(drawn[0], "name^") {
		t.Fatalf("the heading reads %q, want the mark beside the sorted column", drawn[0])
	}
}

func TestAPictureTakesTheRoomItNeedsOrSaysWhatItWas(t *testing.T) {
	// The three ways there is nothing to show — no picture, no cell size, no terminal
	// that draws them — all end in the same place, and the alternative text is what
	// was written for exactly that.
	var none kit.Image
	none.Alt = "a diagram"
	none.Theme = kit.Dark()
	equalRows(t, paint(12, 1, none.Draw), []string{"a diagram..."})
	if none.Measure(12) != 1 {
		t.Fatalf("a picture that cannot be shown asked for %d rows", none.Measure(12))
	}
	var width interface{ Width() int } = none
	if got := width.Width(); got != 9 {
		t.Fatalf("alternative text measured %d cells, want nine", got)
	}

	// With a handle and a cell size it keeps the room, and the cells stay blank: what
	// goes there is written by the frame, not drawn into it.
	shown := kit.Image{
		Of:   graphics.Image{ID: 3, Width: 200, Height: 100},
		Cell: image.Pt(10, 20),
	}
	if got := shown.Measure(40); got != 5 {
		t.Fatalf("a 200x100 picture in 10x20 cells took %d rows, want five", got)
	}
	equalRows(t, paint(8, 2, shown.Draw), []string{"........", "........"})
}

// spy is a body that records whether it was told it has the keyboard.
type spy struct{ focused bool }

func (s *spy) Draw(headless.Frame)     {}
func (s *spy) Focus(has bool)          { s.focused = has }
func (s *spy) Handle(input.Event) bool { return false }

func TestALayerPassesTheKeyboardToWhatIsInIt(t *testing.T) {
	// A stack hands the keyboard to the layer on top and expects the layer to pass it
	// on. A dialog is the layer, so without this the news stops at the frame and a
	// form inside one takes every keystroke while drawing no caret.
	body := &spy{}
	stack := &headless.Stack{}
	dialog := kit.NewDialog(kit.DialogConfig{Stack: stack, Theme: kit.Dark(), Glyphs: kit.ASCII(), Title: "sure?", Body: body})
	dialog.Show()
	stack.Focus(true)

	if !body.focused {
		t.Fatal("what is inside the dialog was not told it has the keyboard")
	}
	stack.Focus(false)
	if body.focused {
		t.Fatal("what is inside the dialog was not told it lost the keyboard")
	}
}
