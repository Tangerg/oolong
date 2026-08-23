package grid_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
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

// text reads back a row as plain characters, with a dot for a blank cell, so a
// test can state what the grid looks like instead of what it contains.
func text(s *grid.Surface, y int) string {
	var b strings.Builder
	w, _ := s.Size()
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
	return b.String()
}

// flush renders one frame and returns the bytes, without the frame markers.
func flush(t *testing.T, s *grid.Screen, cursor grid.Cursor, draw func(grid.View)) string {
	t.Helper()
	v := s.Frame()
	if cursor.Visible {
		v.PlaceCursor(cursor.Pos.X, cursor.Pos.Y, cursor.Style)
	}
	if draw != nil {
		draw(v)
	}
	var buf bytes.Buffer
	if err := s.Flush(&buf); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	out := buf.String()
	if out == "" {
		return ""
	}
	// Spelled out rather than read from the package: a test that took the markers
	// from the code under test could not notice them changing.
	const beginSync, endSync = "\x1b[?2026h", "\x1b[?2026l"
	if !strings.HasPrefix(out, beginSync) || !strings.HasSuffix(out, endSync) {
		t.Fatalf("frame is not wrapped for atomic application: %q", out)
	}
	return strings.TrimSuffix(strings.TrimPrefix(out, beginSync), endSync)
}

func TestZeroCellIsABlankSingleColumn(t *testing.T) {
	var c grid.Cell
	if c.Width() != 1 || !c.Blank() {
		t.Fatalf("zero grid.Cell = %+v, want a blank single-width cell", c)
	}
	// A freshly sized surface must already be a grid of blanks, not a grid of
	// orphaned continuation cells.
	s := grid.NewSurface(3, 1)
	if got := text(s, 0); got != "..." {
		t.Fatalf("new surface row = %q, want blanks", got)
	}
}

func TestRectNormalizesInvalidAndOverflowingExtents(t *testing.T) {
	if got := grid.Rect(4, 5, -2, -3); got != (image.Rectangle{Min: image.Pt(4, 5), Max: image.Pt(4, 5)}) {
		t.Fatalf("negative extent = %v, want an empty rectangle at the origin", got)
	}
	maxInt := int(^uint(0) >> 1)
	if got := grid.Rect(maxInt-1, 0, 10, 1); got.Max.X != maxInt || got.Min.X != maxInt-1 {
		t.Fatalf("overflowing extent = %v, want it saturated", got)
	}
}

func TestSurfaceRejectsAnOverflowingAreaClearly(t *testing.T) {
	defer func() {
		if got := recover(); got != "grid: surface dimensions overflow" {
			t.Fatalf("panic = %v", got)
		}
	}()
	grid.NewSurface(int(^uint(0)>>1), 2)
}

func TestResizeValidatesBeforeChangingRendererState(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	type renderer interface {
		Size() (int, int)
		Resize(w, h int)
	}
	tests := []struct {
		name string
		new  func() renderer
	}{
		{name: "surface", new: func() renderer { return grid.NewSurface(3, 2) }},
		{name: "screen", new: func() renderer { return grid.NewScreen(3, 2) }},
		{name: "inline", new: func() renderer { return grid.NewInline(3, 2) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			renderer := test.new()
			func() {
				defer func() {
					if got := recover(); got != "grid: surface dimensions overflow" {
						t.Fatalf("panic = %v", got)
					}
				}()
				renderer.Resize(maxInt, 2)
			}()
			if w, h := renderer.Size(); w != 3 || h != 2 {
				t.Fatalf("size after rejected resize = %dx%d, want 3x2", w, h)
			}
		})
	}
}

func TestTextWritesAndClips(t *testing.T) {
	s := grid.NewSurface(6, 2)
	v := s.View()

	if got := v.Text(0, 0, "hello", grid.Style{}); got != 5 {
		t.Fatalf("advance = %d, want 5", got)
	}
	if got := text(s, 0); got != "hello." {
		t.Fatalf("row 0 = %q", got)
	}
	// Past the right edge: written where it fits, dropped where it does not.
	v.Text(4, 1, "abcd", grid.Style{})
	if got := text(s, 1); got != "....ab" {
		t.Fatalf("row 1 = %q, want the overflow dropped", got)
	}
	// Off the bottom: no panic, and the advance is still reported so a caller
	// laying out a line gets the same answer wherever it lands.
	if got := v.Text(0, 9, "xyz", grid.Style{}); got != 3 {
		t.Fatalf("off-surface advance = %d, want 3", got)
	}
}

func TestWideClustersOccupyTwoColumns(t *testing.T) {
	s := grid.NewSurface(6, 1)
	if got := s.View().Text(0, 0, "中文", grid.Style{}); got != 4 {
		t.Fatalf("advance = %d, want 4 columns for two wide clusters", got)
	}
	if cellAt(s, 0, 0).Width() != 2 || cellAt(s, 1, 0).Width() != 0 {
		t.Fatal("a wide cluster did not claim a head and a trailing cell")
	}
	if got := text(s, 0); got != "中文.." {
		t.Fatalf("row = %q", got)
	}
}

func TestInspectionCannotMutateTheSurface(t *testing.T) {
	s := grid.NewSurface(3, 1)
	s.View().Text(0, 0, "中x", grid.Style{})

	cell := cellAt(s, 0, 0)
	cell.Content = "broken"
	if cell.Content != "broken" {
		t.Fatal("the detached inspection value could not be changed")
	}
	row := s.Row(0)
	row[0], row[1] = grid.Cell{}, grid.Cell{}

	if got := text(s, 0); got != "中x" {
		t.Fatalf("mutating inspection results changed the surface to %q", got)
	}
	if cellAt(s, 0, 0).Width() != 2 || cellAt(s, 1, 0).Width() != 0 {
		t.Fatal("mutating inspection results broke a wide-cell pair")
	}
}

func TestMergeStyleCannotBreakTheCellPair(t *testing.T) {
	s := grid.NewSurface(2, 1)
	v := s.View()
	v.Text(0, 0, "中", grid.Style{})
	over := grid.Style{Attr: grid.Bold}
	if !v.MergeStyle(0, 0, over) || !v.MergeStyle(1, 0, over) {
		t.Fatal("visible cells were not restyled")
	}
	if cellAt(s, 0, 0).Width() != 2 || cellAt(s, 1, 0).Width() != 0 {
		t.Fatal("restyling broke a wide-cell pair")
	}
	if !cellAt(s, 0, 0).Style.Attr.Has(grid.Bold) || !cellAt(s, 1, 0).Style.Attr.Has(grid.Bold) {
		t.Fatal("style was not merged onto both cells")
	}
}

func TestWideClusterIsNeverSplitAtTheRightEdge(t *testing.T) {
	// Two columns: the letter takes one, and the wide cluster cannot have the one
	// that is left. Half a glyph would be worse than a gap.
	s := grid.NewSurface(2, 1)
	s.View().Text(0, 0, "a中", grid.Style{})
	if got := text(s, 0); got != "a." {
		t.Fatalf("row = %q, want the wide cluster dropped and its column blanked", got)
	}
	if c := cellAt(s, 1, 0); c.Width() != 1 || !c.Blank() {
		t.Fatalf("cell 1 = %+v, want a blank single cell", c)
	}
	// One more column and it does fit, exactly.
	wider := grid.NewSurface(3, 1)
	wider.View().Text(0, 0, "a中", grid.Style{})
	if got := text(wider, 0); got != "a中" {
		t.Fatalf("row = %q, want the wide cluster to have fitted", got)
	}
}

func TestOverwritingHalfOfAWidePairBlanksTheOther(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   int
		want string
	}{
		{"head", 0, "x."},
		{"trailing", 1, ".x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := grid.NewSurface(2, 1)
			v := s.View()
			v.Text(0, 0, "中", grid.Style{})
			v.Text(tc.at, 0, "x", grid.Style{})
			if got := text(s, 0); got != tc.want {
				t.Fatalf("row = %q, want %q", got, tc.want)
			}
			for x := range 2 {
				if w := cellAt(s, x, 0).Width(); w == 0 || w == 2 {
					t.Fatalf("cell %d is still half of a pair (width %d)", x, w)
				}
			}
		})
	}
}

func TestZeroWidthClusterJoinsTheCellToItsLeft(t *testing.T) {
	s := grid.NewSurface(3, 1)
	// A combining acute arriving on its own after the letter it modifies.
	s.View().Text(0, 0, "éx", grid.Style{})
	if got := cellAt(s, 0, 0).Content; got != "é" {
		t.Fatalf("cell 0 = %q, want the mark folded into the letter", got)
	}
	if got := cellAt(s, 1, 0).Content; got != "x" {
		t.Fatalf("cell 1 = %q, want the next letter to have kept its column", got)
	}
}

func TestFillStylesAndBlanks(t *testing.T) {
	s := grid.NewSurface(4, 2)
	v := s.View()
	v.Text(0, 0, "abcd", grid.Style{})
	style := grid.Style{FG: grid.RGBColor(1, 2, 3)}
	v.Fill(grid.Rect(1, 0, 2, 1), style)
	if got := text(s, 0); got != "a..d" {
		t.Fatalf("row = %q, want the filled span blanked", got)
	}
	if got := cellAt(s, 1, 0).Style; got != style {
		t.Fatalf("style = %+v, want the fill's", got)
	}
}

func TestSubViewNarrowsAndKeepsItsOwnCoordinates(t *testing.T) {
	s := grid.NewSurface(10, 4)
	inner := s.View().Sub(grid.Rect(2, 1, 3, 2))

	if w, h := inner.Size(); w != 3 || h != 2 {
		t.Fatalf("size = %dx%d, want 3x2", w, h)
	}
	inner.Text(0, 0, "abcdef", grid.Style{})
	if got := text(s, 1); got != "..abc....." {
		t.Fatalf("row 1 = %q, want the write placed and clipped by the sub-view", got)
	}
	// A nested view cannot widen what it was given.
	wider := inner.Sub(grid.Rect(-5, 0, 20, 1))
	wider.Text(0, 0, "ZZZZZZZZZZ", grid.Style{})
	if got := text(s, 1); got != "..ZZZ....." {
		t.Fatalf("row 1 = %q, want the nested view still clipped to its parent", got)
	}
}

func TestViewSizeIsNominalAndVisibleIsClipped(t *testing.T) {
	s := grid.NewSurface(6, 2)
	// A widget laid out half off the right edge still lays out for its whole box.
	v := s.View().Sub(grid.Rect(4, 0, 6, 1))
	if w, _ := v.Size(); w != 6 {
		t.Fatalf("Size width = %d, want the box it was laid out into", w)
	}
	if got := v.Visible().Dx(); got != 2 {
		t.Fatalf("Visible width = %d, want only what reaches the screen", got)
	}
}

func TestZeroViewDrawsNowhere(t *testing.T) {
	var v grid.View
	if !v.Empty() {
		t.Fatal("the zero grid.View claims it can draw")
	}
	// None of these may panic: a widget given no room still runs its draw code.
	v.Fill(grid.Rect(0, 0, 5, 5), grid.Style{})
	v.Link(0, 0, 3, "https://example.test")
	if got := v.Text(0, 0, "hello", grid.Style{}); got != 0 {
		t.Fatalf("advance = %d, want 0", got)
	}
	if _, ok := v.CellAt(0, 0); ok {
		t.Fatal("the zero grid.View handed out a cell")
	}
	if w, h := v.Size(); w != 0 || h != 0 {
		t.Fatalf("size = %dx%d, want zero", w, h)
	}
}

func TestStyleMerge(t *testing.T) {
	base := grid.Style{FG: grid.RGBColor(10, 10, 10), BG: grid.RGBColor(20, 20, 20), Attr: grid.Bold}
	over := grid.Style{BG: grid.RGBColor(30, 30, 30), Attr: grid.Underline}
	got := base.Merge(over)

	if got.FG != base.FG {
		t.Error("a default foreground in the overlay dropped the one underneath")
	}
	if got.BG != over.BG {
		t.Error("the overlay's background did not win")
	}
	if !got.Attr.Has(grid.Bold | grid.Underline) {
		t.Errorf("attributes = %b, want both to survive", got.Attr)
	}
}

func TestColorBlend(t *testing.T) {
	black, white := grid.RGBColor(0, 0, 0), grid.RGBColor(255, 255, 255)
	if got := black.Blend(white, 0.5).RGB(); got != (grid.RGB{128, 128, 128}) {
		t.Fatalf("halfway = %+v, want mid grey", got)
	}
	if got := black.Blend(white, 2); got != white {
		t.Fatal("opacity above one did not clamp to the overlay")
	}
	// A colour that defers to the terminal is not a number, so a blend involving one
	// cannot be computed. What cannot be resolved is left alone rather than guessed
	// at — see [grid.Ground] for where the numbers come from.
	if got := (grid.Color{}).Blend(white, 0.5); !got.Default() {
		t.Fatal("blending from the terminal default invented a value for it")
	}
	if got := black.Blend(grid.Color{}, 0.5); got != black {
		t.Fatal("blending toward the terminal default changed the colour it started from")
	}
}

func TestGroundResolvesWhatWasLeftToTheTerminal(t *testing.T) {
	ground := grid.Ground{FG: grid.RGBColor(0xC0, 0xCA, 0xF5), BG: grid.RGBColor(0x1A, 0x1B, 0x26)}
	got := ground.Resolve(grid.Style{Attr: grid.Bold})
	if got.FG != ground.FG || got.BG != ground.BG {
		t.Errorf("an unstyled cell resolved to %+v, want the terminal's own colours", got)
	}
	if !got.Attr.Has(grid.Bold) {
		t.Error("resolving a style dropped its attributes")
	}

	// What was stated stays stated: resolving fills gaps and decides nothing else.
	stated := grid.Style{FG: grid.RGBColor(1, 2, 3), BG: grid.RGBColor(4, 5, 6)}
	if got := ground.Resolve(stated); got != stated {
		t.Errorf("a fully stated style resolved to %+v, want it untouched", got)
	}

	// A terminal that said nothing leaves the style alone, which is what makes an
	// unanswered probe cost a missing effect rather than a wrong colour.
	if got := (grid.Ground{}).Resolve(grid.Style{}); got != (grid.Style{}) {
		t.Errorf("an unknown terminal resolved an unstyled cell to %+v", got)
	}
}

func TestBlendMixesEveryCellItCoversAndKeepsTheContent(t *testing.T) {
	s := grid.NewSurface(4, 2)
	v := s.View()
	v.Text(0, 0, "ab", grid.Style{FG: grid.RGBColor(0xFF, 0xFF, 0xFF), BG: grid.RGBColor(0, 0, 0)})
	v.Blend(grid.Rect(0, 0, 1, 1), grid.RGBColor(0, 0, 0), 0.5)

	if got := cellAt(s, 0, 0).Content; got != "a" {
		t.Errorf("content = %q, want the cell mixed rather than erased", got)
	}
	if got := cellAt(s, 0, 0).Style.FG.RGB(); got != (grid.RGB{R: 128, G: 128, B: 128}) {
		t.Errorf("foreground = %+v, want it half way to the sheet", got)
	}
	// Only what it covers. A sheet is a rectangle, and a cell outside it is not part
	// of the thing receding.
	if got := cellAt(s, 1, 0).Style.FG.RGB(); got != (grid.RGB{R: 255, G: 255, B: 255}) {
		t.Errorf("the cell beside the sheet became %+v", got)
	}
}

// TestBlendResolvesThroughTheSurfacesGround is the whole point of asking the
// terminal what it draws with. A cell nobody coloured is the commonest cell there
// is, and until the answer arrives it cannot be mixed with anything.
func TestBlendResolvesThroughTheSurfacesGround(t *testing.T) {
	s := grid.NewSurface(2, 1)
	s.View().Text(0, 0, "ab", grid.Style{})
	s.View().Blend(s.Bounds(), grid.RGBColor(0, 0, 0), 0.5)
	if got := cellAt(s, 0, 0).Style; got != (grid.Style{}) {
		t.Fatalf("an unstyled cell over an unknown terminal became %+v", got)
	}

	s.SetGround(grid.Ground{FG: grid.RGBColor(0xFF, 0xFF, 0xFF), BG: grid.RGBColor(0x20, 0x20, 0x20)})
	s.View().Blend(s.Bounds(), grid.RGBColor(0, 0, 0), 0.5)
	if got := cellAt(s, 0, 0).Style.FG.RGB(); got != (grid.RGB{R: 128, G: 128, B: 128}) {
		t.Errorf("foreground = %+v, want the terminal's own colour half way to the sheet", got)
	}
	if got := cellAt(s, 0, 0).Style.BG.RGB(); got != (grid.RGB{R: 16, G: 16, B: 16}) {
		t.Errorf("background = %+v, want the terminal's own colour half way to the sheet", got)
	}
}

// TestFadeDissolvesEachCellIntoWhateverItIsDrawnOn — which is a different colour in
// every cell, and is why this takes no colour of its own.
func TestFadeDissolvesEachCellIntoWhateverItIsDrawnOn(t *testing.T) {
	s := grid.NewSurface(3, 1)
	v := s.View()
	white := grid.RGBColor(0xFF, 0xFF, 0xFF)
	v.Text(0, 0, "a", grid.Style{FG: white, BG: grid.RGBColor(0, 0, 0)})
	v.Text(1, 0, "b", grid.Style{FG: white, BG: grid.RGBColor(0x40, 0x40, 0x40)})
	v.Text(2, 0, "c", grid.Style{FG: white})
	v.Fade(s.Bounds(), 0.5)

	if got := cellAt(s, 0, 0).Style.FG.RGB(); got != (grid.RGB{R: 128, G: 128, B: 128}) {
		t.Errorf("over black the text is %+v", got)
	}
	if got := cellAt(s, 1, 0).Style.FG.RGB(); got != (grid.RGB{R: 160, G: 160, B: 160}) {
		t.Errorf("over a lighter cell the text is %+v, want it to have gone a shorter way", got)
	}
	// The background is what the text dissolves into, so it is not itself moved.
	if got := cellAt(s, 1, 0).Style.BG.RGB(); got != (grid.RGB{R: 0x40, G: 0x40, B: 0x40}) {
		t.Errorf("the background moved to %+v", got)
	}
	// A cell on the terminal's own background over a terminal that never said what
	// that is has nothing to dissolve into, and keeps what it had.
	if got := cellAt(s, 2, 0).Style.FG; got != white {
		t.Errorf("over an unknown terminal the text became %+v", got.RGB())
	}

	// Nothing asked for is nothing done, which is what keeps a header that is not
	// moving from being rewritten every frame.
	before := cellAt(s, 0, 0)
	v.Fade(s.Bounds(), 0)
	if got := cellAt(s, 0, 0); got != before {
		t.Errorf("fading by nothing changed the cell to %+v", got)
	}
}

// TestBlendClipsToTheViewBecauseADrawingViewIsABox: a widget cannot dim what is
// outside its box any more than it can draw there.
func TestBlendClipsToTheViewBecauseADrawingViewIsABox(t *testing.T) {
	s := grid.NewSurface(4, 1)
	white := grid.Style{FG: grid.RGBColor(0xFF, 0xFF, 0xFF)}
	s.View().Text(0, 0, "abcd", white)

	inner := s.View().Sub(grid.Rect(0, 0, 2, 1))
	inner.Blend(grid.Rect(0, 0, 100, 100), grid.RGBColor(0, 0, 0), 1)

	if got := cellAt(s, 1, 0).Style.FG.RGB(); got != (grid.RGB{}) {
		t.Errorf("a cell inside the view is %+v, want it painted", got)
	}
	if got := cellAt(s, 2, 0).Style; got != white {
		t.Errorf("a cell outside the view is %+v, want it untouched", got)
	}
}

// TestSurfaceKeepsItsGroundAcrossAResize because it describes the terminal and not
// the contents: a resize blanks the cells, and the terminal is still the same one.
func TestSurfaceKeepsItsGroundAcrossAResize(t *testing.T) {
	ground := grid.Ground{BG: grid.RGBColor(0x1A, 0x1B, 0x26)}
	s := grid.NewSurface(2, 1)
	s.SetGround(ground)
	s.Resize(4, 2)
	if got := s.Ground(); got != ground {
		t.Errorf("ground = %+v, want it to survive a resize", got)
	}
	if got := s.View().Ground(); got != ground {
		t.Errorf("a view onto it reports %+v", got)
	}
}

// TestAFrameCarriesTheGroundAcrossTheSwap: a screen draws into one surface and
// shows the other, and swaps them every flush. A ground set on one of them would
// work on every other frame, which is the kind of bug that reads as flicker.
func TestAFrameCarriesTheGroundAcrossTheSwap(t *testing.T) {
	ground := grid.Ground{FG: grid.RGBColor(0xFF, 0xFF, 0xFF), BG: grid.RGBColor(0, 0, 0)}
	s := grid.NewScreen(2, 1)
	s.SetGround(ground)
	for frame := range 3 {
		if got := s.Frame().Ground(); got != ground {
			t.Fatalf("frame %d was drawn against %+v", frame, got)
		}
		if err := s.Flush(io.Discard); err != nil {
			t.Fatalf("flushing frame %d: %v", frame, err)
		}
	}
}

func TestFirstFrameRepaintsAndAnIdenticalFrameIsSilent(t *testing.T) {
	s := grid.NewScreen(4, 1)
	draw := func(v grid.View) { v.Text(0, 0, "abcd", grid.Style{}) }

	first := flush(t, s, grid.Cursor{}, draw)
	if !strings.Contains(first, "abcd") {
		t.Fatalf("first frame = %q, want the content painted", first)
	}
	// An unchanged frame writes nothing: not the cells, not the frame markers,
	// and above all not a cursor command, which would restart the blink.
	if second := flush(t, s, grid.Cursor{}, draw); second != "" {
		t.Fatalf("unchanged frame wrote %q, want silence", second)
	}
}

func TestDiffWritesOnlyWhatChanged(t *testing.T) {
	s := grid.NewScreen(10, 2)
	flush(t, s, grid.Cursor{}, func(v grid.View) {
		v.Text(0, 0, "unchanged", grid.Style{})
		v.Text(0, 1, "before", grid.Style{})
	})
	out := flush(t, s, grid.Cursor{}, func(v grid.View) {
		v.Text(0, 0, "unchanged", grid.Style{})
		v.Text(0, 1, "beFore", grid.Style{})
	})

	if !strings.Contains(out, "F") {
		t.Fatalf("frame = %q, want the changed cell", out)
	}
	if strings.Contains(out, "unchanged") {
		t.Fatalf("frame = %q, want the untouched row left alone", out)
	}
	// Just the reset pair, one position, and one glyph.
	if len(out) > 20 {
		t.Fatalf("frame = %q (%d bytes), want a minimal stream", out, len(out))
	}
}

func TestCursorCommands(t *testing.T) {
	s := grid.NewScreen(4, 2)
	at := func(x, y int) grid.Cursor { return grid.Cursor{Visible: true, Pos: image.Pt(x, y)} }

	first := flush(t, s, at(1, 0), func(v grid.View) { v.Text(0, 0, "ab", grid.Style{}) })
	if !strings.Contains(first, "\x1b[?25h") {
		t.Fatalf("first frame = %q, want the cursor shown", first)
	}

	// Same cursor, same cells: silence, so the blink timer survives.
	if out := flush(t, s, at(1, 0), func(v grid.View) { v.Text(0, 0, "ab", grid.Style{}) }); out != "" {
		t.Fatalf("idle frame = %q, want silence", out)
	}
	// Moved: repositioned, and not re-shown.
	out := flush(t, s, at(2, 1), func(v grid.View) { v.Text(0, 0, "ab", grid.Style{}) })
	if !strings.Contains(out, "\x1b[2;3H") {
		t.Fatalf("frame = %q, want the cursor moved to row 2 column 3", out)
	}
	if strings.Contains(out, "\x1b[?25h") {
		t.Fatalf("frame = %q, want no redundant show", out)
	}
	// Hidden: one command, and nothing else.
	out = flush(t, s, grid.Cursor{}, func(v grid.View) { v.Text(0, 0, "ab", grid.Style{}) })
	if out != "\x1b[?25l" {
		t.Fatalf("frame = %q, want only the hide", out)
	}
}

func TestCursorStyleIsPartOfTheFrameDiff(t *testing.T) {
	s := grid.NewScreen(4, 1)
	bar := grid.Cursor{
		Visible: true, Pos: image.Pt(1, 0),
		Style: grid.CursorStyle{Shape: grid.CursorBar, Blink: true},
	}
	first := flush(t, s, bar, nil)
	if !strings.Contains(first, "\x1b[5 q") {
		t.Fatalf("first frame = %q, want a blinking bar", first)
	}
	if idle := flush(t, s, bar, nil); idle != "" {
		t.Fatalf("unchanged cursor wrote %q", idle)
	}

	underline := bar
	underline.Style = grid.CursorStyle{Shape: grid.CursorUnderline}
	if changed := flush(t, s, underline, nil); changed != "\x1b[4 q" {
		t.Fatalf("style change = %q, want only a steady underline", changed)
	}
	if got := s.Cursor(); got != underline {
		t.Fatalf("committed cursor = %+v, want %+v", got, underline)
	}
}

func TestAnUnknownCursorShapeBecomesTheTerminalDefault(t *testing.T) {
	s := grid.NewScreen(2, 1)
	view := s.Frame()
	view.PlaceCursor(0, 0, grid.CursorStyle{Shape: grid.CursorShape(255), Blink: true})
	if got := s.Cursor().Style; got != (grid.CursorStyle{}) {
		t.Fatalf("cursor style = %+v, want the zero default", got)
	}
}

func TestWritingCellsReanchorsAnUnmovedCursor(t *testing.T) {
	s := grid.NewScreen(6, 1)
	cursor := grid.Cursor{Visible: true, Pos: image.Pt(0, 0)}
	flush(t, s, cursor, func(v grid.View) { v.Text(0, 0, "aa", grid.Style{}) })

	// The glyph left the terminal's cursor after it, so the frame has to say
	// where the cursor belongs even though it did not move.
	out := flush(t, s, cursor, func(v grid.View) { v.Text(0, 0, "ab", grid.Style{}) })
	if strings.Count(out, "\x1b[1;1H") != 1 {
		t.Fatalf("frame = %q, want the cursor re-anchored once", out)
	}
}

func TestResizeRepaintsInFull(t *testing.T) {
	s := grid.NewScreen(4, 1)
	flush(t, s, grid.Cursor{}, func(v grid.View) { v.Text(0, 0, "abcd", grid.Style{}) })
	s.Resize(6, 1)
	out := flush(t, s, grid.Cursor{}, func(v grid.View) { v.Text(0, 0, "abcd", grid.Style{}) })
	if !strings.Contains(out, "abcd") {
		t.Fatalf("frame after resize = %q, want a full repaint", out)
	}
}

func TestScrollUsesTheTerminalsOwnShift(t *testing.T) {
	const w, h = 24, 10
	s := grid.NewScreen(w, h)
	rows := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliett"}
	flush(t, s, grid.Cursor{}, func(v grid.View) {
		for y, r := range rows {
			v.Text(0, y, r, grid.Style{})
		}
	})

	// The same content one row higher, with a new row at the bottom: a pure shift.
	out := flush(t, s, grid.Cursor{}, func(v grid.View) {
		for y := range h - 1 {
			v.Text(0, y, rows[y+1], grid.Style{})
		}
		v.Text(0, h-1, "kilo", grid.Style{})
	})
	if !strings.Contains(out, "\x1b[1S") {
		t.Fatalf("frame = %q, want the terminal asked to scroll", out)
	}
	if !strings.Contains(out, "kilo") {
		t.Fatalf("frame = %q, want the exposed row painted", out)
	}
	for _, carried := range rows[1:] {
		if strings.Contains(out, carried) {
			t.Fatalf("frame = %q, want %q carried by the scroll rather than repainted", out, carried)
		}
	}
}

func TestScrollIsRefusedWhenItWouldCostMore(t *testing.T) {
	// Two rows of content and a one-row change: a shift would move as much as it
	// saves, so the plain diff has to win.
	s := grid.NewScreen(20, 3)
	flush(t, s, grid.Cursor{}, func(v grid.View) {
		v.Text(0, 0, "one", grid.Style{})
		v.Text(0, 1, "two", grid.Style{})
		v.Text(0, 2, "three", grid.Style{})
	})
	out := flush(t, s, grid.Cursor{}, func(v grid.View) {
		v.Text(0, 0, "one", grid.Style{})
		v.Text(0, 1, "TWO", grid.Style{})
		v.Text(0, 2, "three", grid.Style{})
	})
	if strings.Contains(out, "S") && strings.Contains(out, "\x1b[") && strings.Contains(out, "\x1b[1S") {
		t.Fatalf("frame = %q, want the plain diff for a single-row edit", out)
	}
	if !strings.Contains(out, "TWO") {
		t.Fatalf("frame = %q, want the edit painted", out)
	}
}

func TestHyperlinksOpenAndClose(t *testing.T) {
	s := grid.NewScreen(10, 1)
	out := flush(t, s, grid.Cursor{}, func(v grid.View) {
		v.Text(0, 0, "link", grid.Style{})
		v.Link(0, 0, 4, "https://example.test/x")
	})
	if !strings.Contains(out, "\x1b]8;;"+"https://example.test/x"+"\x1b\\") {
		t.Fatalf("frame = %q, want the hyperlink opened", out)
	}
	if !strings.Contains(out, "\x1b]8;;\x1b\\") {
		t.Fatalf("frame = %q, want the hyperlink closed", out)
	}
	if strings.LastIndex(out, "\x1b]8;;") > strings.LastIndex(out, "\x1b]8;;\x1b\\") {
		t.Fatalf("frame = %q, want no hyperlink left open at the end", out)
	}
}

func TestHyperlinkTargetWithControlBytesIsDropped(t *testing.T) {
	s := grid.NewScreen(10, 1)
	// A target carrying the string terminator could close the sequence early and
	// have what follows read as terminal commands. Cells can be filled from tool
	// output, so this is a trust boundary rather than a formatting nicety.
	out := flush(t, s, grid.Cursor{}, func(v grid.View) {
		v.Text(0, 0, "x", grid.Style{})
		v.Link(0, 0, 1, "https://ok\x1b\\\x1b]0;pwned\x07")
	})
	if strings.Contains(out, "\x1b]8;;") {
		t.Fatalf("frame = %q, want the unsafe target dropped", out)
	}
	if !strings.Contains(out, "x") {
		t.Fatalf("frame = %q, want the text still painted", out)
	}
}

func TestStyleIsStatedOncePerRun(t *testing.T) {
	s := grid.NewScreen(8, 1)
	red := grid.Style{FG: grid.RGBColor(255, 0, 0)}
	out := flush(t, s, grid.Cursor{}, func(v grid.View) { v.Text(0, 0, "abcd", red) })
	// One SGR for the run, plus the frame's opening and closing resets.
	if got := strings.Count(out, "\x1b[0;38;2;255;0;0m"); got != 1 {
		t.Fatalf("frame = %q, want one style statement, got %d", out, got)
	}
}

func TestEncodeRowIsSelfContainedInlineText(t *testing.T) {
	s := grid.NewSurface(6, 1)
	v := s.View()
	v.Text(0, 0, "hi", grid.Style{FG: grid.RGBColor(1, 2, 3)})
	v.Link(0, 0, 2, "https://example.test")

	row := grid.EncodeRow(s.Row(0), grid.TrueColor)
	if strings.Contains(row, "\x1b[1;") || strings.Contains(row, "H") {
		t.Fatalf("row = %q, want nothing that moves the cursor", row)
	}
	if strings.LastIndex(row, "\x1b]8;;") > strings.LastIndex(row, "\x1b]8;;\x1b\\") {
		t.Fatalf("row = %q, want no hyperlink left open", row)
	}
	if !strings.Contains(row, "hi") {
		t.Fatalf("row = %q, want the text", row)
	}
	// Trailing blanks are not printed: they cost bytes for nothing and, on a
	// full-width row, wrap the cursor before the caller asked it to.
	if strings.HasSuffix(row, " ") {
		t.Fatalf("row = %q, want the blank tail dropped", row)
	}
}

func TestEncodeRowSkipsTrailingHalvesOfWideClusters(t *testing.T) {
	s := grid.NewSurface(4, 1)
	s.View().Text(0, 0, "中文", grid.Style{})
	if got := grid.EncodeRow(s.Row(0), grid.TrueColor); got != "中文" {
		t.Fatalf("row = %q, want each wide cluster emitted once", got)
	}
}

// failWriter fails after letting n bytes through.
type failWriter struct{ n int }

func (w *failWriter) Write(p []byte) (int, error) {
	if w.n <= 0 {
		return 0, errWrite
	}
	w.n -= len(p)
	return len(p), nil
}

var errWrite = &writeError{}

type writeError struct{}

func (*writeError) Error() string { return "write failed" }

func TestAFailedWriteForcesAFullRepaint(t *testing.T) {
	s := grid.NewScreen(4, 1)
	s.Frame().Text(0, 0, "abcd", grid.Style{})
	if err := s.Flush(&failWriter{}); err == nil {
		t.Fatal("Flush hid a write failure")
	}
	// Some prefix of the frame may have landed, so the terminal's contents are
	// unknown and diffing against them would be a guess.
	out := flush(t, s, grid.Cursor{}, func(v grid.View) { v.Text(0, 0, "abcd", grid.Style{}) })
	if !strings.Contains(out, "abcd") {
		t.Fatalf("frame after a failed write = %q, want a full repaint", out)
	}
}

func TestInvalidateForcesAFullRepaint(t *testing.T) {
	s := grid.NewScreen(4, 1)
	draw := func(v grid.View) { v.Text(0, 0, "abcd", grid.Style{}) }
	flush(t, s, grid.Cursor{Visible: true}, draw)
	s.Invalidate()
	out := flush(t, s, grid.Cursor{Visible: true}, draw)
	if !strings.Contains(out, "abcd") || !strings.Contains(out, "\x1b[?25h") {
		t.Fatalf("frame after Invalidate = %q, want everything re-stated", out)
	}
}

func TestCopyRowsLiftsRowsBetweenSurfaces(t *testing.T) {
	src := grid.NewSurface(4, 4)
	for y := range 4 {
		src.View().Text(0, y, strings.Repeat(string(rune('a'+y)), 4), grid.Style{})
	}
	dst := grid.NewSurface(4, 2)
	dst.CopyRows(src, 2, 0, 2)
	if got := text(dst, 0); got != "cccc" {
		t.Fatalf("row 0 = %q", got)
	}
	if got := text(dst, 1); got != "dddd" {
		t.Fatalf("row 1 = %q", got)
	}
	// Out-of-range rows are skipped rather than fatal, which is what lets a
	// caller lift the visible slice of an over-tall item into place.
	dst.CopyRows(src, 3, 1, 4)
	if got := text(dst, 1); got != "dddd" {
		t.Fatalf("row 1 = %q, want the copy to have stopped at the edge", got)
	}
}

func TestSurfaceMethodsTolerateANilReceiver(t *testing.T) {
	var s *grid.Surface
	if _, ok := s.CellAt(0, 0); ok || s.Row(0) != nil {
		t.Fatal("a nil surface handed out cells")
	}
	if !s.View().Empty() {
		t.Fatal("a nil surface handed out a drawable view")
	}
}

func TestControlCharactersNeverReachACell(t *testing.T) {
	// Cells are filled from tool output and model output. A control byte stored in
	// one would be written to the terminal verbatim on the next repaint: a tab or
	// carriage return would move the cursor out from under the renderer, and an
	// escape would begin a sequence the terminal obeys.
	s := grid.NewSurface(20, 1)
	s.View().Text(0, 0, "a\x1b]0;title\x07b\tc\rd", grid.Style{})

	for x := range 20 {
		if c := cellAt(s, x, 0); strings.ContainsAny(c.Content, "\x1b\x07\t\r") {
			t.Fatalf("cell %d holds a control character: %q", x, c.Content)
		}
	}
	if got := text(s, 0); got != "a]0;titlebcd........" {
		t.Fatalf("row = %q, want only the printable characters, none of them shifted", got)
	}
}

func TestControlCharactersAreNotFoldedIntoTheCellBefore(t *testing.T) {
	// A zero-width cluster joins the cell to its left, and a control character
	// measures zero. Folding one in would smuggle it into a cell that looks
	// printable.
	s := grid.NewSurface(4, 1)
	s.View().Text(0, 0, "a\x1b", grid.Style{})
	if got := cellAt(s, 0, 0).Content; got != "a" {
		t.Fatalf("cell 0 = %q, want the escape dropped rather than appended", got)
	}
}

func TestTheCursorBelongsToWhoeverDrawsIt(t *testing.T) {
	s := grid.NewScreen(20, 5)
	// A widget speaks in its own coordinates; nobody in between carries the answer.
	out := flush(t, s, grid.Cursor{}, func(v grid.View) {
		v.Sub(grid.Rect(4, 2, 10, 1)).PlaceCursor(3, 0, grid.CursorStyle{})
	})
	if !strings.Contains(out, "\x1b[3;8H") {
		t.Fatalf("frame = %q, want the cursor at row 3 column 8", out)
	}
	if !strings.Contains(out, "\x1b[?25h") {
		t.Fatalf("frame = %q, want the cursor shown", out)
	}
}

func TestAFrameNobodyPlacedTheCursorInHasNoCursor(t *testing.T) {
	s := grid.NewScreen(8, 1)
	out := flush(t, s, grid.Cursor{}, func(v grid.View) { v.Text(0, 0, "text", grid.Style{}) })
	if !strings.Contains(out, "\x1b[?25l") {
		t.Fatalf("frame = %q, want the cursor hidden when nothing owns it", out)
	}
}

func TestAWidgetScrolledOffScreenCannotMoveTheCursor(t *testing.T) {
	s := grid.NewScreen(10, 2)
	out := flush(t, s, grid.Cursor{}, func(v grid.View) {
		// The box starts past the right edge, so it has nowhere to draw and no say
		// over the cursor either.
		v.Sub(grid.Rect(20, 0, 5, 1)).PlaceCursor(0, 0, grid.CursorStyle{})
	})
	if strings.Contains(out, "\x1b[?25h") {
		t.Fatalf("frame = %q, want no cursor from a view with nowhere to draw", out)
	}
}

func TestPlacingTheCursorOnAPlainSurfaceIsHarmless(_ *testing.T) {
	// A scratch surface is not a frame. Placing a cursor there means nothing, and
	// meaning nothing is not the same as being an error.
	grid.NewSurface(4, 1).View().PlaceCursor(1, 0, grid.CursorStyle{})
}

func TestASurfaceCanBeReadAsText(t *testing.T) {
	// The way out for a program with no terminal: output being piped, a run under a
	// build server, a transcript written to a file.
	rows := grid.Render(12, 3, func(v grid.View) {
		v.Text(0, 0, "hello 世界", grid.Style{FG: grid.RGBColor(255, 0, 0)})
		v.Text(2, 2, "indented", grid.Style{})
	})
	want := []string{"hello 世界", "", "  indented"}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row %d is %q, want %q", i, rows[i], want[i])
		}
	}
	// A wide cluster is written once and not twice: it is one thing that takes two
	// columns, and text has no columns.
	if got := len([]rune(rows[0])); got != 8 {
		t.Fatalf("the row came to %d runes, want the text back as it was written", got)
	}
	if len(rows) != 3 {
		t.Fatalf("%d rows, want one per row of the surface", len(rows))
	}
}

// picture is something that writes itself into a region of a frame, recording what
// it was asked to do so a test can state the lifecycle rather than the bytes.
type picture struct {
	name string
	log  *[]string
}

type falliblePicture struct {
	paintErr error
	eraseErr error
}

func (p *falliblePicture) Paint(w io.Writer, _ image.Point) error {
	if p.paintErr != nil {
		return p.paintErr
	}
	_, err := io.WriteString(w, "<picture>")
	return err
}

func (p *falliblePicture) Erase(w io.Writer) error {
	if p.eraseErr != nil {
		return p.eraseErr
	}
	_, err := io.WriteString(w, "</picture>")
	return err
}

type regionCanvas interface {
	Frame() grid.View
	Flush(w io.Writer) error
}

type brokenShortWriter struct {
	bytes.Buffer
	limit int
}

func (w *brokenShortWriter) Write(p []byte) (int, error) {
	return w.Buffer.Write(p[:min(len(p), w.limit)])
}

func TestCanvasesRejectShortWritesWithoutSettling(t *testing.T) {
	for name, makeCanvas := range map[string]func() regionCanvas{
		"screen": func() regionCanvas { return grid.NewScreen(10, 4) },
		"inline": func() regionCanvas { return grid.NewInline(10, 4) },
	} {
		t.Run(name, func(t *testing.T) {
			canvas := makeCanvas()
			canvas.Frame().Text(0, 0, "complete", grid.Style{})
			broken := &brokenShortWriter{limit: 3}
			if err := canvas.Flush(broken); !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("Flush error = %v, want io.ErrShortWrite", err)
			}
			if broken.Len() != 3 {
				t.Fatalf("broken writer received %d bytes, want its accepted prefix", broken.Len())
			}

			var out bytes.Buffer
			canvas.Frame().Text(0, 0, "complete", grid.Style{})
			if err := canvas.Flush(&out); err != nil {
				t.Fatalf("retry Flush: %v", err)
			}
			if !strings.Contains(out.String(), "complete") {
				t.Fatalf("retry received %q", out.String())
			}

			out.Reset()
			canvas.Frame().Text(0, 0, "complete", grid.Style{})
			if err := canvas.Flush(&out); err != nil {
				t.Fatalf("unchanged Flush: %v", err)
			}
			if out.Len() != 0 {
				t.Fatalf("frame was not settled after successful retry: %q", out.String())
			}
		})
	}
}

func TestAPainterFailureDoesNotPublishOrSettleTheFrame(t *testing.T) {
	cause := errors.New("picture transport failed")
	for name, makeCanvas := range map[string]func() regionCanvas{
		"screen": func() regionCanvas { return grid.NewScreen(10, 4) },
		"inline": func() regionCanvas { return grid.NewInline(10, 4) },
	} {
		t.Run(name, func(t *testing.T) {
			canvas := makeCanvas()
			picture := &falliblePicture{paintErr: cause}
			var out bytes.Buffer
			draw := func() {
				canvas.Frame().Paint(grid.Rect(1, 1, 4, 2), 1, picture)
			}

			draw()
			if err := canvas.Flush(&out); !errors.Is(err, cause) {
				t.Fatalf("Flush error = %v, want painter cause", err)
			}
			if out.Len() != 0 {
				t.Fatalf("failed logical frame reached the terminal: %q", out.String())
			}

			picture.paintErr = nil
			draw()
			if err := canvas.Flush(&out); err != nil {
				t.Fatalf("retry: %v", err)
			}
			if !strings.Contains(out.String(), "<picture>") {
				t.Fatalf("the unsettled picture was not painted by the next frame: %q", out.String())
			}
		})
	}
}

func TestAPainterEraseFailureKeepsTheOldRegionUnsettled(t *testing.T) {
	cause := errors.New("picture erase failed")
	for name, makeCanvas := range map[string]func() regionCanvas{
		"screen": func() regionCanvas { return grid.NewScreen(10, 4) },
		"inline": func() regionCanvas { return grid.NewInline(10, 4) },
	} {
		t.Run(name, func(t *testing.T) {
			canvas := makeCanvas()
			picture := &falliblePicture{}
			var out bytes.Buffer

			canvas.Frame().Paint(grid.Rect(1, 1, 4, 2), 1, picture)
			if err := canvas.Flush(&out); err != nil {
				t.Fatalf("opening frame: %v", err)
			}
			out.Reset()

			picture.eraseErr = cause
			canvas.Frame()
			if err := canvas.Flush(&out); !errors.Is(err, cause) {
				t.Fatalf("Flush error = %v, want erase cause", err)
			}
			if out.Len() != 0 {
				t.Fatalf("failed erase frame reached the terminal: %q", out.String())
			}

			picture.eraseErr = nil
			canvas.Frame()
			if err := canvas.Flush(&out); err != nil {
				t.Fatalf("retry: %v", err)
			}
			if !strings.Contains(out.String(), "</picture>") {
				t.Fatalf("the unsettled region was not erased by the next frame: %q", out.String())
			}
		})
	}
}

func TestChangingOnlyAPaintRegionStillProducesAFrame(t *testing.T) {
	for name, makeCanvas := range map[string]func() regionCanvas{
		"screen": func() regionCanvas { return grid.NewScreen(10, 4) },
		"inline": func() regionCanvas { return grid.NewInline(10, 4) },
	} {
		t.Run(name, func(t *testing.T) {
			var log []string
			first := picture{name: "first", log: &log}
			second := picture{name: "second", log: &log}
			canvas := makeCanvas()
			var out bytes.Buffer

			canvas.Frame().Paint(grid.Rect(1, 1, 4, 2), 1, first)
			if err := canvas.Flush(&out); err != nil {
				t.Fatal(err)
			}
			log = nil
			out.Reset()

			// The cells, bounds and cursor are unchanged; identity is the only
			// difference and is still enough to replace what the terminal remembers.
			canvas.Frame().Paint(grid.Rect(1, 1, 4, 2), 2, second)
			if err := canvas.Flush(&out); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(log, ","); got != "erase first,paint second 4x2" {
				t.Fatalf("region operations = %q", got)
			}
		})
	}
}

func (p picture) Paint(w io.Writer, size image.Point) error {
	*p.log = append(*p.log, fmt.Sprintf("paint %s %dx%d", p.name, size.X, size.Y))
	_, err := io.WriteString(w, "<"+p.name+">")
	return err
}

func (p picture) Erase(w io.Writer) error {
	*p.log = append(*p.log, "erase "+p.name)
	_, err := io.WriteString(w, "</"+p.name+">")
	return err
}

func TestAFrameKeepsRoomForSomethingItCannotDraw(t *testing.T) {
	// What a cell cannot hold: a picture, a plot in pixels, anything whose contents
	// are bytes the terminal understands and the grid does not.
	var log []string
	s := grid.NewScreen(10, 4)
	var out bytes.Buffer

	draw := func(at image.Rectangle, id uint64, name string) {
		v := s.Frame()
		v.Text(0, 0, "caption", grid.Style{})
		v.Paint(at, id, picture{name: name, log: &log})
		if err := s.Flush(&out); err != nil {
			t.Fatal(err)
		}
	}

	box := grid.Rect(1, 1, 6, 3)
	draw(box, 1, "a")
	if len(log) != 1 || log[0] != "paint a 6x3" {
		t.Fatalf("the first frame did %v", log)
	}
	if !strings.Contains(out.String(), "<a>") {
		t.Fatalf("what it painted never reached the terminal: %q", out.String())
	}

	// The same thing in the same place is not painted again: an unchanged frame is
	// silent about its regions as it is about its cells.
	log = nil
	draw(box, 1, "a")
	if len(log) != 0 {
		t.Fatalf("an unchanged region did %v", log)
	}

	// Moved is erased and painted again, in that order: a terminal that remembers
	// what it was shown would otherwise hold both until the old one was taken away.
	log = nil
	draw(grid.Rect(2, 1, 6, 3), 1, "a")
	if len(log) != 2 || log[0] != "erase a" || log[1] != "paint a 6x3" {
		t.Fatalf("a region that moved did %v", log)
	}

	// A different picture in the same place replaces it, and a frame that asks for
	// none takes it away.
	log = nil
	draw(grid.Rect(2, 1, 6, 3), 2, "b")
	if len(log) != 2 || log[0] != "erase a" || log[1] != "paint b 6x3" {
		t.Fatalf("a region that changed did %v", log)
	}
	log = nil
	v := s.Frame()
	v.Text(0, 0, "caption", grid.Style{})
	if err := s.Flush(&out); err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0] != "erase b" {
		t.Fatalf("a region that went away did %v", log)
	}
}

func TestARegionThatDoesNotFitIsNotPainted(t *testing.T) {
	// Half a picture squashed into the part that fits is worse than none: this layer
	// knows how many cells a region has and nothing about what is in it.
	var log []string
	s := grid.NewScreen(10, 4)
	var out bytes.Buffer
	v := s.Frame()
	v.Paint(grid.Rect(6, 2, 8, 4), 1, picture{name: "a", log: &log})
	if err := s.Flush(&out); err != nil {
		t.Fatal(err)
	}
	if len(log) != 0 {
		t.Fatalf("a region hanging off the edge did %v", log)
	}
}

func TestWhatWasPaintedIsSaidAgainAfterAFullRepaint(t *testing.T) {
	// A repaint says nothing about a region: what a terminal remembers being shown is
	// not a cell, and writing over the cells does not undo it. So a resize, or a
	// terminal handed to something else and taken back, starts the region again.
	var log []string
	s := grid.NewScreen(10, 4)
	var out bytes.Buffer
	paint := func() {
		v := s.Frame()
		v.Paint(grid.Rect(0, 0, 4, 2), 7, picture{name: "a", log: &log})
		if err := s.Flush(&out); err != nil {
			t.Fatal(err)
		}
	}
	paint()
	log = nil
	s.Invalidate()
	paint()
	if len(log) != 2 || log[0] != "erase a" || log[1] != "paint a 4x2" {
		t.Fatalf("after a full repaint the region did %v", log)
	}
}
