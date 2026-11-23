package grid

import (
	"image"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/mattn/go-runewidth"
)

// Surface is a rectangle of cells in row-major order. Drawing happens through the
// [View] it hands out, so no caller carries a clip rectangle beside the buffer it is
// clipping. A Surface must not be copied after first use.
type Surface struct {
	noCopy  noCopy
	drawing *viewScope
	capture *captureStorage

	w, h  int
	cells []Cell
	// ground is a property of the cells' meaning rather than of one view onto them:
	// every view derived from a surface is looking at the same terminal.
	ground Ground
	// Painter regions belong to one frame and expire when its cells reset.
	paints []painted
}

// noCopy makes mutable grid ownership visible to go vet. Its methods are never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// NewSurface returns a blank surface. Negative dimensions collapse to zero, and a
// product too large for int panics.
func NewSurface(w, h int) *Surface {
	s := &Surface{}
	s.Resize(w, h)
	return s
}

// Resize changes the surface's size and blanks it. Content is not preserved: every
// resize is followed by a full redraw, so carrying stale cells across one would only
// make the first frame after it wrong in a subtler way.
func (s *Surface) Resize(w, h int) {
	s.resizeCells(w, h)
	s.clearRegions()
}

// resizeCells blanks and resizes cell storage while keeping painter regions.
func (s *Surface) resizeCells(w, h int) {
	w, h, n := surfaceSize(w, h)
	s.endDrawing()
	if cap(s.cells) >= n {
		// Cells beyond the new length are still scanned by the collector, and would
		// retain old content and link strings for as long as the smaller surface lives.
		cells := s.cells[:cap(s.cells)]
		clear(cells)
		s.cells = cells[:n]
	} else {
		s.cells = make([]Cell, n)
	}
	s.w, s.h = w, h
}

// surfaceSize validates before its caller changes any state, so a recovered panic
// cannot leave a renderer's two buffers at different sizes.
func surfaceSize(w, h int) (int, int, int) {
	w, h = max(w, 0), max(h, 0)
	if h > 0 && w > math.MaxInt/h {
		panic("grid: surface dimensions overflow")
	}
	return w, h, w * h
}

func (s *Surface) Reset() {
	s.endDrawing()
	clear(s.cells)
	s.clearRegions()
}

func (s *Surface) clearRegions() {
	clear(s.paints)
	s.paints = s.paints[:0]
}

// A retained callback view cannot keep a finished one-shot raster or its
// capture pool alive. Standalone surfaces keep their storage across Reset.
func (s *Surface) releaseScratch() {
	s.endDrawing()
	s.cells, s.paints, s.capture = nil, nil, nil
	s.w, s.h = 0, 0
}

func (s *Surface) Size() (w, h int) { return s.w, s.h }

// SetGround says what a default colour in these cells resolves to. It survives a
// resize and a reset, because it describes the terminal rather than the contents.
func (s *Surface) SetGround(g Ground) { s.ground = g }

func (s *Surface) Ground() Ground {
	if s == nil {
		return Ground{}
	}
	return s.ground
}

// Bounds is the surface's own rectangle, with its origin at zero.
func (s *Surface) Bounds() image.Rectangle { return Area(0, 0, s.w, s.h) }

func (s *Surface) View() View {
	if s == nil {
		return View{}
	}
	return View{surface: s, size: image.Pt(s.w, s.h), clip: s.Bounds()}
}

// CellAt copies the cell at (x, y) and reports whether it is inside the surface.
// Content changes only through a [View], which preserves complete display atoms.
func (s *Surface) CellAt(x, y int) (Cell, bool) {
	c := s.cellAt(x, y)
	if c == nil {
		return Cell{}, false
	}
	return *c, true
}

func (s *Surface) cellAt(x, y int) *Cell {
	if s == nil || x < 0 || x >= s.w || y < 0 || y >= s.h {
		return nil
	}
	return &s.cells[y*s.w+x]
}

// Row copies one row, or nil when y is outside the surface. A row is an inspection
// result, not a mutable view into the grid.
func (s *Surface) Row(y int) []Cell {
	return slices.Clone(s.row(y))
}

func (s *Surface) row(y int) []Cell {
	if s == nil || y < 0 || y >= s.h {
		return nil
	}
	return s.cells[y*s.w : (y+1)*s.w]
}

// CopyRows copies n whole rows out of src, starting at srcTop, into s starting at
// dstTop. Rows outside either surface are skipped, which is what lets a caller render
// an over-tall item into a scratch surface and lift the visible slice of it into
// place. Overlapping rows on one surface read as if copied through a temporary.
func (s *Surface) CopyRows(src *Surface, srcTop, dstTop, n int) {
	if s == nil || src == nil || src.w != s.w || s.w == 0 {
		return
	}
	first := max(0, layout.Relative(0, srcTop), layout.Relative(0, dstTop))
	end := min(n, layout.Relative(src.h, srcTop), layout.Relative(s.h, dstTop))
	if first >= end {
		return
	}
	// Intersect the requested offsets before indexing. A clipped request can
	// be much taller than either surface, and copy handles overlap in place.
	copy(s.cells[(dstTop+first)*s.w:(dstTop+end)*s.w], src.cells[(srcTop+first)*s.w:(srcTop+end)*s.w])
}

// repairAtom blanks the whole multi-column atom containing (x, y) so an overwrite
// leaves no orphaned head or continuation. Each column keeps its own style: an
// overwrite changes content ownership, not appearance outside the overwritten region.
func (s *Surface) repairAtom(x, y int) {
	from, to := s.atomAt(x, y)
	if to-from < 2 {
		return
	}
	row := s.row(y)
	for column := from; column < to; column++ {
		row[column] = Cell{Style: row[column].Style}
	}
}

// atomAt is the complete display atom containing (x, y); an ordinary cell is a
// one-column atom. Invalid private metadata is conservatively one cell, because
// public drawing never creates it and white-box tests reject it.
func (s *Surface) atomAt(x, y int) (from, to int) {
	if x < 0 || x >= s.w || y < 0 || y >= s.h {
		return x, x
	}
	row := s.row(y)
	from = x
	if row[x].span < 0 {
		from = layout.Translate(x, int(row[x].span))
	}
	if from < 0 || from >= s.w || row[from].span < 2 {
		return x, x + 1
	}
	to = min(layout.Translate(from, int(row[from].span)), s.w)
	if x >= to {
		return x, x + 1
	}
	return from, to
}

// mutateAppearance applies mutate once per display atom whose head is in area, and
// reaches appearance only. Anchoring ownership at the head makes adjacent areas a
// true partition — an atom crossing their edge belongs to exactly one — so
// non-idempotent blends cannot accumulate.
func (s *Surface) mutateAppearance(area image.Rectangle, mutate func(*Style, *string)) {
	area = area.Intersect(s.Bounds())
	for y := area.Min.Y; y < area.Max.Y; y++ {
		row := s.row(y)
		for x := area.Min.X; x < area.Max.X; {
			from, to := s.atomAt(x, y)
			if from != x {
				x++
				continue
			}
			for column := from; column < to; column++ {
				mutate(&row[column].Style, &row[column].Link)
			}
			x = max(x+1, to)
		}
	}
}

// asciiClusters gives the most common cells package-owned storage without one
// allocation per draw. Anything else goes through ownedCluster, because a cluster may
// be a short slice of a much larger caller-owned string.
var asciiClusters = func() [utf8.RuneSelf]string {
	var clusters [utf8.RuneSelf]string
	for b := range utf8.RuneSelf {
		clusters[b] = string(rune(b))
	}
	return clusters
}()

// Fix ambiguous-width behavior instead of inheriting the server locale, which
// may differ from the terminal being drawn and would change identical layouts.
var columns = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: false}

const (
	halfwidthKatakanaVoicedSoundMark     = '\uff9e'
	halfwidthKatakanaSemivoicedSoundMark = '\uff9f'
)

// Render supports nonterminal output with unstyled rows and trimmed trailing
// blanks. The callback's view expires before return so retained references
// cannot mutate the completed projection.
func Render(w, h int, draw func(View)) []string {
	s := NewSurface(w, h)
	defer s.releaseScratch()
	if draw != nil {
		draw(s.borrow(nil))
	}
	return s.Rows()
}

func (s *Surface) Rows() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, s.h)
	for y := range s.h {
		var b strings.Builder
		for _, c := range s.row(y) {
			switch {
			case c.span < 0:
			case c.content == "":
				b.WriteByte(' ')
			default:
				b.WriteString(c.content)
			}
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}

func (s *Surface) regions() []painted {
	if s == nil {
		return nil
	}
	return s.paints
}
