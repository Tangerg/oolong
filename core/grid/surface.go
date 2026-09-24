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

// Surface is a rectangle of cells in row-major order. Drawing happens through the
// [View] it hands out, so no caller carries a clip rectangle beside the buffer it is
// clipping. A Surface must not be copied after first use.
type Surface struct {
	noCopy noCopy

	w, h  int
	cells []Cell
	// ground is a property of the cells' meaning rather than of one view onto them:
	// every view derived from a surface is looking at the same terminal.
	ground Ground
	// paints are the regions of this frame that something else writes into — see
	// [Painter]. A frame is drawn from nothing every time, so they die with it.
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
	w, h, n := surfaceSize(w, h)
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
	s.Reset()
}

// surfaceSize validates before its caller changes any state, so a recovered panic
// cannot leave a renderer's two buffers at different sizes.
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
func (s *Surface) Bounds() image.Rectangle { return Area(0, 0, s.w, s.h) }

// View returns a drawing view over the whole surface.
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
	if s == nil || src == nil || src.w != s.w {
		return
	}
	for step := range n {
		i := step
		if s == src && dstTop > srcTop {
			i = n - 1 - step
		}
		sy, dy := srcTop+i, dstTop+i
		if sy < 0 || sy >= src.h || dy < 0 || dy >= s.h {
			continue
		}
		copy(s.row(dy), src.row(sy))
	}
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

// View is a clipped window onto a [Surface], addressed in its own coordinates.
//
// The clip bounds drawing intent, not storage, because a multi-column display atom
// is indivisible: replacing any of its columns blanks the complete old atom, and an
// appearance change containing the atom's head changes the complete atom. Those are
// the only mutations a view causes beyond its clip.
//
// The zero View draws nowhere and reports a size of zero, which is the right answer
// for content laid out into no space at all.
type View struct {
	surface *Surface
	// origin is where this view's (0, 0) sits on the surface.
	origin image.Point
	// size is the box the view was laid out into, which is not what it may draw on:
	// content half scrolled off the screen still lays out for its whole size.
	size image.Point
	// clip is the region an operation must intersect, in surface coordinates, never
	// wider than the surface itself.
	clip image.Rectangle
	// cursor is shared by every view of the frame, and nil for a surface that is not
	// one, where placing a cursor is meaningless rather than an error.
	cursor *Cursor
}

// Size returns the box the view was laid out into.
func (v View) Size() (w, h int) { return v.size.X, v.size.Y }

// Bounds is the view's own coordinate space, origin at zero.
func (v View) Bounds() image.Rectangle { return image.Rectangle{Max: v.size} }

// Visible is the part of the view that will actually reach the screen, in the view's
// own coordinates. It is empty for a view with nowhere to draw.
func (v View) Visible() image.Rectangle {
	if v.surface == nil {
		return image.Rectangle{}
	}
	return untranslateRect(v.clip, v.origin)
}

// Empty reports whether the view has nowhere to draw.
func (v View) Empty() bool { return v.surface == nil || v.clip.Empty() }

// Sub returns a view onto r, expressed in this view's coordinates. Clipping only ever
// narrows: a caller cannot hand a child room it does not have itself.
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

// Subs returns child views for rects expressed in this view's coordinates. It
// projects rather than lays out, which is what lets geometry stay independent of the
// cell store.
func (v View) Subs(rects []image.Rectangle) []View {
	views := make([]View, len(rects))
	for i, r := range rects {
		views[i] = v.Sub(r)
	}
	return views
}

// PlaceCursor asks for the terminal's cursor at local (x, y) with style. The view
// already knows where it sits on the screen, so translating is nobody else's job.
//
// A position outside what the view may draw on is ignored, for the same reason a
// glyph there would be. A frame in which nobody places the cursor has no cursor.
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

// CellAt copies the cell at local (x, y) and reports whether it is inside the clip.
// Content and appearance change through [View.Text], [View.Fill], [View.MergeStyle]
// and [View.Link].
func (v View) CellAt(x, y int) (Cell, bool) {
	c := v.cellAt(x, y)
	if c == nil {
		return Cell{}, false
	}
	return *c, true
}

func (v View) cellAt(x, y int) *Cell {
	p := translatePoint(image.Pt(x, y), v.origin)
	if v.surface == nil || !p.In(v.clip) {
		return nil
	}
	return v.surface.cellAt(p.X, p.Y)
}

// MergeStyle lays style over the display atom whose head is at local (x, y),
// preserving the roles it already carries, and reports whether it named a head inside
// the view. It is an operation rather than a mutable Cell pointer so that changing
// appearance cannot also replace one half of a wide grapheme.
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
func (v View) Ground() Ground { return v.surface.Ground() }

// Blend moves every cell's foreground and background in r toward over by opacity.
//
// Both colours move by the same amount, which is what makes the region recede as a
// whole: text and its background keep their relationship and lose contrast against
// everything outside the sheet. Content is untouched, so what is behind stays
// readable and stays where it was.
//
// A cell whose colour is the terminal's own resolves through [View.Ground] first, and
// where that has no answer the cell keeps the colour it had — see [Color.Blend].
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

// Fade moves each cell's foreground in r toward that cell's own background by amount,
// from 0 for nothing to 1 for gone.
//
// It takes no colour because the colour is different in every cell and is already
// there, which is why a [View.Blend] cannot stand in for it: the sheet would have to
// be one colour over the themed part and another over the plain part.
//
// A cell whose colours are the terminal's own resolves through [View.Ground] first,
// and where that has no answer the cell is left alone.
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
		// A fill edge can land inside a multi-column atom, and repairing both edges
		// first keeps the part outside the fill from being orphaned.
		s.repairAtom(area.Min.X, y)
		s.repairAtom(area.Max.X-1, y)
		row := s.row(y)
		for x := area.Min.X; x < area.Max.X; x++ {
			row[x] = Cell{Style: style}
		}
	}
}

// Text writes s at local (x, y) and returns how many columns it advanced, including
// any it advanced outside the clip.
//
// A multi-column cluster is never split: one that would straddle an edge is dropped
// and its visible columns blanked, because part of a glyph is worse than a gap. A
// zero-width cluster joins the display atom to its left rather than taking a column.
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
			// The column still advances: a caller measuring what it drew is asking
			// about the text, not about how much of it landed.
		case cx < v.clip.Min.X || end > v.clip.Max.X:
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

// blank replaces [from,to) with styled single cells, preserving the atom invariant at
// both ends. Coordinates are on the surface and already clipped.
func (v View) blank(from, to, y int, style Style) {
	for column := from; column < to; column++ {
		v.surface.repairAtom(column, y)
		*v.surface.cellAt(column, y) = Cell{Style: style}
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

func ownedCluster(cluster string) string {
	if len(cluster) == 1 && cluster[0] < utf8.RuneSelf {
		return asciiClusters[cluster[0]]
	}
	return strings.Clone(cluster)
}

// combine appends a zero-width cluster to the atom owning the column to the left,
// stepping over a continuation cell to reach its head.
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

// Link stamps target onto the display atoms whose heads occupy w columns from local
// (x, y). It is separate from [View.Text] because a link usually spans a run that was
// drawn in several pieces.
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
	v.surface.mutateAppearance(Area(from, at.Y, to-from, 1), func(_ *Style, link *string) {
		*link = target
	})
}

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

// columns is built explicitly instead of using the package-level default, whose East
// Asian width setting comes from the locale of whatever machine the program runs on.
// That would make "…" one column wide here and two there — the same layout code
// producing different frames.
var columns = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: false}

// ClusterWidth is Oolong's deterministic estimate of how many terminal columns one
// grapheme cluster occupies. A control character measures zero.
//
// Everything that lays text out shares this function: measuring text one way and
// drawing it another is the cause of every misaligned terminal UI.
//
// Display width is terminal behaviour rather than a complete Unicode property, so
// this is an estimate. Ambiguous-width characters are fixed at one column, and U+FF9E
// and U+FF9F are counted separately because common terminals render those
// halfwidth-katakana sound marks as spacing characters.
func ClusterWidth(cluster string) int {
	if control(cluster) {
		return 0
	}
	width := max(columns.StringWidth(cluster), 0)
	// A sound mark alone, or one after an ASCII base, is the only adjusted shape that
	// fits in four UTF-8 bytes, and go-runewidth measures both correctly.
	if len(cluster) <= utf8.UTFMax {
		return width
	}
	// The marks are Unicode extenders but spacing terminal characters, so segmentation
	// keeps them with their base while terminal geometry gives each one a column.
	// go-runewidth caps a complete grapheme at two columns, which would hide them.
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
// Such a cluster is dropped rather than stored, because a control byte living in a
// cell would be written to the terminal verbatim on the next repaint: a tab would
// move the cursor out from under the renderer and an escape would begin a sequence
// the terminal obeys. Cells are filled from tool and model output, so this is a trust
// boundary rather than a tidiness rule.
func control(cluster string) bool {
	if cluster == "" {
		return false
	}
	if !utf8.ValidString(cluster) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(cluster)
	return r < 0x20 || r >= 0x7f && r <= 0x9f
}

// Render draws something at a size and returns what it came to, one string per row,
// with the styling dropped and trailing blanks cut.
//
// It is the way out of the grid for a program with no terminal: output being piped, a
// run under a build server, a transcript written to a file. A caller that wants the
// colours too has [EncodeRow].
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
