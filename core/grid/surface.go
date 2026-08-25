package grid

import (
	"image"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/layout"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// Surface is a rectangle of cells in row-major order. It is storage and
// geometry; drawing happens through the [View] it hands out, so no caller has to
// carry a clip rectangle alongside the buffer it is clipping. A Surface must not
// be copied after first use: its cells and paint regions have one mutable owner.
type Surface struct {
	noCopy noCopy

	w, h  int
	cells []Cell
	// ground is what a default colour in these cells resolves to. It lives on the
	// surface rather than being passed to whoever needs it because it is a property
	// of the cells' meaning, not of one view onto them: every view derived from a
	// surface is looking at the same terminal.
	ground Ground
	// paints are the regions of this frame that something else writes into — see
	// [Painter]. They belong to the surface for the same reason the cells do: they
	// are what this frame is, and a frame is drawn from nothing every time.
	paints []painted
}

// noCopy makes mutable grid ownership visible to go vet. Its methods are never
// called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// NewSurface returns a blank surface of the given size. Negative dimensions
// collapse to zero. It panics with a grid error when the dimensions' product
// cannot be represented by int.
func NewSurface(w, h int) *Surface {
	s := &Surface{}
	s.Resize(w, h)
	return s
}

// Resize changes the surface's size and blanks it. Content is not preserved:
// every resize is followed by a full redraw, so carrying stale cells across one
// would only make the first frame after it wrong in a subtler way. Negative
// dimensions collapse to zero. Resize panics with a grid error when the
// dimensions' product cannot be represented by int.
func (s *Surface) Resize(w, h int) {
	w, h, n := surfaceSize(w, h)
	if cap(s.cells) >= n {
		// Clear the whole allocation before shrinking it. Cells beyond the new length
		// are still scanned by the garbage collector and would otherwise retain old
		// content and hyperlink strings for as long as the smaller surface lives.
		cells := s.cells[:cap(s.cells)]
		clear(cells)
		s.cells = cells[:n]
	} else {
		s.cells = make([]Cell, n)
	}
	s.w, s.h = w, h
	s.Reset()
}

// surfaceSize is the one interpretation of dimensions used by surfaces and the
// renderers that own them. It validates before a caller changes any state, so a
// recovered configuration panic cannot leave a renderer's two buffers at
// different sizes.
func surfaceSize(w, h int) (int, int, int) {
	w, h = max(w, 0), max(h, 0)
	if h > 0 && w > maxInt/h {
		panic("grid: surface dimensions overflow")
	}
	return w, h, w * h
}

// Reset blanks every cell and forgets the regions something else was to paint.
func (s *Surface) Reset() {
	clear(s.cells)
	clear(s.paints)
	s.paints = s.paints[:0]
}

// Size returns the surface's width and height.
func (s *Surface) Size() (w, h int) { return s.w, s.h }

// SetGround says what a default colour in these cells resolves to. It survives a
// resize and a reset, because it describes the terminal rather than the contents.
func (s *Surface) SetGround(g Ground) { s.ground = g }

// Ground is what a default colour in these cells resolves to.
func (s *Surface) Ground() Ground {
	if s == nil {
		return Ground{}
	}
	return s.ground
}

// Bounds is the surface's own rectangle, with its origin at zero.
func (s *Surface) Bounds() image.Rectangle { return Rect(0, 0, s.w, s.h) }

// View returns a drawing view over the whole surface.
func (s *Surface) View() View {
	if s == nil {
		return View{}
	}
	return View{surface: s, size: image.Pt(s.w, s.h), clip: s.Bounds()}
}

// CellAt returns a copy of the cell at (x, y) and whether the coordinates are
// inside the surface. Content can only be changed by drawing through a [View],
// which preserves complete display atoms.
func (s *Surface) CellAt(x, y int) (Cell, bool) {
	c := s.cellAt(x, y)
	if c == nil {
		return Cell{}, false
	}
	return *c, true
}

// cellAt is the package's mutable access to one cell.
func (s *Surface) cellAt(x, y int) *Cell {
	if s == nil || x < 0 || x >= s.w || y < 0 || y >= s.h {
		return nil
	}
	return &s.cells[y*s.w+x]
}

// Row returns a copy of one row, or nil when y is outside the surface. A row is an
// inspection result, not a mutable view into the grid.
func (s *Surface) Row(y int) []Cell {
	return slices.Clone(s.row(y))
}

// row is the package's mutable access to one row.
func (s *Surface) row(y int) []Cell {
	if s == nil || y < 0 || y >= s.h {
		return nil
	}
	return s.cells[y*s.w : (y+1)*s.w]
}

// CopyRows copies n whole rows out of src, starting at srcTop, into s starting at
// dstTop. Rows that fall outside either surface are skipped, which is what lets a
// caller render an over-tall item into a scratch surface and lift the visible
// slice of it into place.
func (s *Surface) CopyRows(src *Surface, srcTop, dstTop, n int) {
	if s == nil || src == nil || src.w != s.w {
		return
	}
	for i := range n {
		sy, dy := srcTop+i, dstTop+i
		if sy < 0 || sy >= src.h || dy < 0 || dy >= s.h {
			continue
		}
		copy(s.row(dy), src.row(sy))
	}
}

// repairAtom keeps the display-atom invariant across an overwrite of (x, y).
// When that cell belongs to a multi-column atom, the whole atom is blanked so no
// orphaned head or continuation survives. Each column keeps its own style: an
// overwrite changes content ownership, not appearance outside the overwritten
// region.
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

// atomAt returns the complete display atom containing (x, y). An ordinary cell
// is a one-column atom. Invalid private metadata is conservatively treated as one
// cell; public drawing operations never create it, and white-box tests reject it.
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

// mutateAppearance applies mutate once to every display atom whose head is in area.
// The head owns appearance because it is the only column the painter emits. Anchoring
// ownership there also makes adjacent areas a true partition: an atom crossing their
// edge belongs to exactly one of them, so non-idempotent blends cannot accumulate.
//
// The callback can reach only appearance fields. Content geometry remains owned by
// the drawing operations that repair and replace atoms.
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

// View is a clipped window onto a [Surface], addressed in its own coordinates.
//
// A view bounds drawing intent to one box: coordinates are local, and an operation
// that meets no cell inside the clip does nothing. The clip is not a storage
// isolation barrier because a multi-column display atom is indivisible. Replacing
// any of its columns blanks the complete old atom, preserving the old style on
// repaired columns outside the clip. Appearance belongs to the atom's head column:
// an appearance area containing that head changes the complete atom, including
// Style and Link outside the clip; an area containing only continuations does not.
// These are the only mutations a view may cause beyond its clip.
//
// The zero View draws nowhere and reports a size of zero, which is the right
// answer for content laid out into no space at all.
type View struct {
	surface *Surface
	// origin is where this view's (0, 0) sits on the surface.
	origin image.Point
	// size is the box the view was laid out into, which is not the same as what
	// it may draw on: content half scrolled off the screen still lays out for
	// its whole size and simply loses the part outside the clip.
	size image.Point
	// clip is the region an operation must intersect, in surface coordinates,
	// never wider than the surface itself. Content repair and head-owned appearance
	// changes may settle an old atom across this edge as described on View.
	clip image.Rectangle
	// cursor is where the frame's terminal cursor is recorded, shared by every
	// view of the frame. It is nil for a surface that is not a frame, where
	// placing a cursor is meaningless rather than an error.
	cursor *Cursor
}

// Size returns the box the view was laid out into.
func (v View) Size() (w, h int) { return v.size.X, v.size.Y }

// Bounds is the view's own coordinate space, origin at zero.
func (v View) Bounds() image.Rectangle { return image.Rectangle{Max: v.size} }

// Visible is the part of the view that will actually reach the screen, in the
// view's own coordinates. It is empty for a view with nowhere to draw.
func (v View) Visible() image.Rectangle {
	if v.surface == nil {
		return image.Rectangle{}
	}
	return untranslateRect(v.clip, v.origin)
}

// Empty reports whether the view has nowhere to draw.
func (v View) Empty() bool { return v.surface == nil || v.clip.Empty() }

// Sub returns a view onto r, expressed in this view's coordinates. Clipping only
// ever narrows: a caller cannot hand a child room it does not have itself. As with
// every View, replacing a column at the narrowed edge may blank the rest of an old
// display atom that crosses that edge. An appearance operation applies to a complete
// atom only when its head remains inside the narrowed view.
func (v View) Sub(r image.Rectangle) View {
	if v.surface == nil {
		return View{}
	}
	return View{
		surface: v.surface,
		origin:  translatePoint(v.origin, r.Min),
		size:    rectangleSize(r),
		clip:    v.clip.Intersect(translateRect(r, v.origin)),
		cursor:  v.cursor,
	}
}

// Subs returns child views for rects expressed in this view's coordinates.
//
// It performs projection, not layout: the rectangles may come from any geometry
// model. Keeping that distinction lets geometry remain independent of the cell
// store while callers turn a complete arrangement into views in one operation.
func (v View) Subs(rects []image.Rectangle) []View {
	views := make([]View, len(rects))
	for i, r := range rects {
		views[i] = v.Sub(r)
	}
	return views
}

// PlaceCursor asks for the terminal's cursor to sit at local (x, y) with style.
//
// It is how the drawing owner places the cursor without anyone in between
// having to carry the answer: the view already knows where it sits on the screen,
// so the caller speaks in local coordinates and the translation is nobody's job.
//
// A position outside what the view may draw on is ignored, for the same reason a
// glyph there would be: content scrolled off the screen does not get to move the
// cursor. A frame in which nobody places the cursor is a frame with no cursor,
// which is the right answer when nothing is being typed into.
func (v View) PlaceCursor(x, y int, style CursorStyle) {
	if v.cursor == nil {
		return
	}
	p := translatePoint(image.Pt(x, y), v.origin)
	if !p.In(v.clip) {
		return
	}
	*v.cursor = Cursor{Visible: true, Pos: p, Style: style.normalized()}
}

// CellAt returns a copy of the cell at local (x, y) and whether it is inside the
// clip. Use drawing operations such as [View.Text], [View.Fill], [View.MergeStyle]
// and [View.Link] to change content or appearance.
func (v View) CellAt(x, y int) (Cell, bool) {
	c := v.cellAt(x, y)
	if c == nil {
		return Cell{}, false
	}
	return *c, true
}

// cellAt is the package's mutable access through a view's clip.
func (v View) cellAt(x, y int) *Cell {
	p := translatePoint(image.Pt(x, y), v.origin)
	if v.surface == nil || !p.In(v.clip) {
		return nil
	}
	return v.surface.cellAt(p.X, p.Y)
}

// MergeStyle lays style over the display atom whose head is at local (x, y),
// preserving any roles it already carries. It reports whether the coordinates named
// an atom head inside the view. A multi-column atom is restyled as one glyph even
// when its continuations cross the clip, as described on [View].
//
// Styling is an operation rather than a mutable Cell pointer so changing appearance
// cannot also replace one half of a wide grapheme.
func (v View) MergeStyle(x, y int, style Style) bool {
	p := translatePoint(image.Pt(x, y), v.origin)
	if v.surface == nil || !p.In(v.clip) {
		return false
	}
	if head, _ := v.surface.atomAt(p.X, p.Y); head != p.X {
		return false
	}
	v.surface.mutateAppearance(image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))}, func(current *Style, _ *string) {
		*current = current.Merge(style)
	})
	return true
}

// Ground is what a default colour in this view's cells resolves to.
//
// A caller that mixes colours with what is underneath asks here.
// Nothing above this package carries the answer around: the view is already where
// drawing happens, and it already knows which terminal it is bound for.
func (v View) Ground() Ground { return v.surface.Ground() }

// Blend paints a translucent sheet of colour over r, in this view's coordinates:
// every cell's foreground and background move toward over by opacity.
//
// This is how a layer floats above what it covers instead of erasing it. Both
// colours move, and by the same amount, which is what makes the region recede as a
// whole — text and its background keep their relationship and simply lose contrast
// against everything outside the sheet. Content is untouched, so what is behind
// stays readable and stays where it was.
//
// A cell whose colour is the terminal's own is resolved through [View.Ground]
// first. Where that has no answer the cell keeps the colour it had, which is the
// rule stated on [Color.Blend] and the reason a program asks the terminal what it
// draws on before the first frame. A multi-column atom whose head is in r is blended
// as one glyph even when its continuations cross an edge, as described on [View].
func (v View) Blend(r image.Rectangle, over Color, opacity float64) {
	if v.surface == nil || over.Default() || opacity <= 0 {
		return
	}
	area := v.clip.Intersect(translateRect(r, v.origin))
	if area.Empty() {
		return
	}
	ground := v.surface.ground
	v.surface.mutateAppearance(area, func(current *Style, _ *string) {
		style := ground.Resolve(*current)
		current.FG = style.FG.Blend(over, opacity)
		current.BG = style.BG.Blend(over, opacity)
	})
}

// Fade dissolves what is in r into whatever it is drawn on: each cell's foreground
// moves toward that cell's own background by amount, from 0 for nothing to 1 for
// gone.
//
// It is the other half of compositing and the one that takes no colour, because the
// colour is different in every cell and is already there. A header sliding out from
// under the next one, and a sweep of light along a line still arriving, are both
// this — and neither could be a [View.Blend], because the sheet would have to be a
// different colour over the themed part than over the plain part.
//
// A cell whose colours are the terminal's own is resolved through [View.Ground]
// first, and where that has no answer the cell is left alone. A multi-column atom
// whose head is in r is faded as one glyph even when its continuations cross an edge,
// as described on [View].
func (v View) Fade(r image.Rectangle, amount float64) {
	if v.surface == nil || amount <= 0 {
		return
	}
	area := v.clip.Intersect(translateRect(r, v.origin))
	if area.Empty() {
		return
	}
	ground := v.surface.ground
	v.surface.mutateAppearance(area, func(current *Style, _ *string) {
		style := ground.Resolve(*current)
		current.FG = style.FG.Blend(style.BG, amount)
	})
}

// Fill blanks every cell in r, in this view's coordinates, and gives it style.
// An old display atom crossing an edge of the filled area is blanked completely,
// including any part outside the view's clip, as described on [View].
func (v View) Fill(r image.Rectangle, style Style) {
	if v.surface == nil {
		return
	}
	area := v.clip.Intersect(translateRect(r, v.origin))
	if area.Empty() {
		return
	}
	s := v.surface
	for y := area.Min.Y; y < area.Max.Y; y++ {
		// A fill edge can land in the middle of a multi-column atom; repairing
		// both edges first keeps the part outside the fill from being orphaned.
		s.repairAtom(area.Min.X, y)
		s.repairAtom(area.Max.X-1, y)
		row := s.row(y)
		for x := area.Min.X; x < area.Max.X; x++ {
			row[x] = Cell{Style: style}
		}
	}
}

// Text writes s at local (x, y) and returns how many columns it advanced,
// including any it advanced outside the clip.
//
// Text is grapheme-aware. A multi-column cluster is never split: one that would
// straddle an edge is dropped and its visible columns are blanked, because part
// of a glyph is worse than a gap. A zero-width cluster — a combining mark
// arriving on its own — joins the display atom to its left instead of consuming
// a column of its own. Replacing an old atom that crosses a clip edge may blank
// the rest of that old atom outside the clip, as described on [View].
func (v View) Text(x, y int, s string, style Style) int {
	if v.surface == nil {
		return 0
	}
	p := translatePoint(image.Pt(x, y), v.origin)
	if p.Y < v.clip.Min.Y || p.Y >= v.clip.Max.Y {
		return graphemeWidth(s)
	}

	surf := v.surface
	cx := p.X
	advanced := 0
	state := -1
	var cluster string
	for len(s) > 0 {
		cluster, s, _, state = uniseg.StepString(s, state)
		w := ClusterWidth(cluster)
		if control(cluster) {
			continue
		}
		if w == 0 {
			v.combine(cx, p.Y, cluster)
			continue
		}
		end := layout.Translate(cx, w)
		advanced = layout.Sum(advanced, w)
		switch {
		case end <= v.clip.Min.X || cx >= v.clip.Max.X:
			// Entirely outside the clip.
		case cx < v.clip.Min.X || end > v.clip.Max.X:
			// The atom straddles a clip edge. Blank every visible column rather
			// than leave pieces of either the old or the new atom behind.
			v.blank(min(max(cx, v.clip.Min.X), v.clip.Max.X), min(end, v.clip.Max.X), p.Y, style)
		default:
			for offset := range w {
				surf.repairAtom(layout.Translate(cx, offset), p.Y)
			}
			head := Cell{content: ownedCluster(cluster), Style: style}
			if w > 1 {
				head.span = span(w)
			}
			*surf.cellAt(cx, p.Y) = head
			for offset := 1; offset < w; offset++ {
				*surf.cellAt(layout.Translate(cx, offset), p.Y) = Cell{Style: style, span: span(-offset)}
			}
		}
		cx = end
	}
	return advanced
}

// blank replaces [from,to) with styled single cells while preserving the atom
// invariant around both ends. Coordinates are on the surface and already clipped.
func (v View) blank(from, to, y int, style Style) {
	for column := from; column < to; column++ {
		v.surface.repairAtom(column, y)
		*v.surface.cellAt(column, y) = Cell{Style: style}
	}
}

// asciiClusters gives the most common cells package-owned storage without one
// allocation per draw. Other clusters are cloned by ownedCluster because they may
// be short slices of a much larger caller-owned string.
var asciiClusters = func() [utf8.RuneSelf]string {
	var clusters [utf8.RuneSelf]string
	for b := range utf8.RuneSelf {
		clusters[b] = string(rune(b))
	}
	return clusters
}()

func ownedCluster(cluster string) string {
	if len(cluster) == 1 && cluster[0] < utf8.RuneSelf {
		return asciiClusters[cluster[0]]
	}
	return strings.Clone(cluster)
}

// combine appends a zero-width cluster to the display atom that owns the column
// to the left, stepping over a continuation cell to reach its head.
func (v View) combine(cx, y int, cluster string) {
	head := layout.Translate(cx, -1)
	prev := v.surface.cellAt(head, y)
	if prev != nil && prev.span < 0 {
		head = layout.Translate(head, int(prev.span))
		prev = v.surface.cellAt(head, y)
	}
	if prev == nil || prev.span < 0 || !image.Pt(head, y).In(v.clip) {
		return
	}
	prev.content += cluster
}

// Link stamps target onto the display atoms whose heads occupy w columns starting
// at local (x, y), turning text that has already been written into a hyperlink. It
// is separate from [View.Text] because a link usually spans a run that was drawn
// in several pieces. A multi-column atom receives one link even when it crosses
// an edge, as described on [View].
func (v View) Link(x, y, w int, target string) {
	if v.surface == nil || w <= 0 {
		return
	}
	at := translatePoint(image.Pt(x, y), v.origin)
	if at.Y < v.clip.Min.Y || at.Y >= v.clip.Max.Y {
		return
	}
	from := max(at.X, v.clip.Min.X)
	to := min(layout.Translate(at.X, w), v.clip.Max.X)
	if from >= to {
		return
	}
	target = strings.Clone(target)
	v.surface.mutateAppearance(Rect(from, at.Y, to-from, 1), func(_ *Style, link *string) {
		*link = target
	})
}

// graphemeWidth is how many columns s would occupy.
func graphemeWidth(s string) int {
	total := 0
	state := -1
	var cluster string
	for len(s) > 0 {
		cluster, s, _, state = uniseg.StepString(s, state)
		total = layout.Sum(total, ClusterWidth(cluster))
	}
	return total
}

// columns is the width table every measurement in the TUI goes through.
//
// It is built explicitly instead of using the package-level default, whose
// East Asian width setting is decided by the locale environment variables of
// whatever machine the program happens to run on. That would make a character
// like "…" one column wide here and two there — the same layout code producing
// different frames, and golden output that passes on one developer's machine and
// fails on another's. Ambiguous-width characters are narrow, which is what a
// terminal not told otherwise does with them.
var columns = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: false}

// ClusterWidth returns Oolong's deterministic estimate of how many terminal
// columns one grapheme cluster occupies. A control character measures zero.
//
// Everything that lays text out shares this function. Measuring text one way and
// drawing it another is the cause of every misaligned terminal UI, so there is one
// answer and one place it comes from.
//
// Display width is terminal behavior, not a complete Unicode property. Different
// terminals may shape an unusual cluster differently. This estimate fixes
// ambiguous-width characters to one column, follows go-runewidth's grapheme rules,
// and separately counts U+FF9E and U+FF9F because common terminals render those
// halfwidth-katakana sound marks as spacing characters. Other spacing combining
// marks retain the dependency's answer until terminal evidence supports a rule
// that does not also break emoji and other ligated clusters.
func ClusterWidth(cluster string) int {
	if control(cluster) {
		return 0
	}
	width := max(columns.StringWidth(cluster), 0)
	// A sound mark alone, or one after an ASCII base, is the only adjusted shape
	// that can fit in four UTF-8 bytes; go-runewidth already measures both correctly.
	// Every shape in which its two-column cap can hide a sound mark is longer.
	if len(cluster) <= utf8.UTFMax {
		return width
	}
	// Halfwidth katakana sound marks are Unicode extenders but spacing terminal
	// characters. Grapheme segmentation therefore keeps them with their base while
	// terminal geometry still gives each mark a column. go-runewidth caps a complete
	// grapheme at two columns, so measure the base and these spacing extenders
	// separately when one occurs. The uncommon path may allocate; ordinary text and
	// emoji stay on the dependency's optimized grapheme measurement.
	marks := strings.Count(cluster, "ﾞ") + strings.Count(cluster, "ﾟ")
	if marks == 0 {
		return width
	}
	base := strings.Map(func(r rune) rune {
		if r == '\uff9e' || r == '\uff9f' {
			return -1
		}
		return r
	}, cluster)
	return layout.Sum(max(columns.StringWidth(base), 0), marks)
}

// control reports whether a cluster begins with a control character.
//
// Such a cluster is dropped rather than stored. A control byte living in a cell
// would be written to the terminal verbatim on the next repaint — a tab or
// carriage return would move the cursor out from under the renderer, and an
// escape would begin a sequence the terminal obeys. Cells are filled from tool
// output and model output, so this is a trust boundary and not a tidiness rule.
// Anything above this package that wants a tab to occupy columns expands it
// first, where the column it starts at is known.
func control(cluster string) bool {
	if cluster == "" {
		return false
	}
	b := cluster[0]
	return b < 0x20 || b == 0x7f
}

// Render draws something at a size and returns what it came to, one string per
// row, with the styling dropped and trailing blanks cut.
//
// It is the way out of the grid for a program that has no terminal: output being
// piped, a run under a build server, a transcript written to a file. Everything
// above this package draws into a [View] and cannot be asked for text any other
// way, so without this every caller writes the same walk over the cells — which is
// exactly what every test in this repository had done.
//
// The styling is dropped rather than encoded because that is what "as text" means.
// A caller that wants the colours as well already has [EncodeRow], which is what a
// frame is made of.
func Render(w, h int, draw func(View)) []string {
	s := NewSurface(w, h)
	if draw != nil {
		draw(s.View())
	}
	return s.Rows()
}

// Rows is what the surface says, one string per row, with the styling dropped and
// trailing blanks cut.
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
				// A continuation column, which the atom's head already wrote.
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
