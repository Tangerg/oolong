package grid

import (
	"image"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/internal/grapheme"
	"github.com/Tangerg/oolong/ui/core/layout"
)

// View bounds drawing intent, but display atoms remain indivisible. Replacing
// any column clears the complete old atom; appearance changes follow the head.
// Those repairs are the only writes allowed beyond the clip.
//
// Canvas views are borrowed: copies and subviews lose write authority when the
// frame or callback ends. Surface.View instead follows its surface's lifetime.
// The zero value draws nowhere.
type View struct {
	surface *Surface
	scope   *viewScope
	origin  image.Point
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

// A standalone view keeps its layout box across resize. Only the surface owns
// the storage extent; an earlier clip cannot authorize cells removed since.
func (v View) clipped() image.Rectangle {
	if v.surface == nil {
		return image.Rectangle{}
	}
	return v.clip.Intersect(v.surface.Bounds())
}

func (v View) Size() (w, h int) { return v.size.X, v.size.Y }

func (v View) Bounds() image.Rectangle { return image.Rectangle{Max: v.size} }

// Visible is the part of the view that will actually reach the screen, in the view's
// own coordinates. It is empty for a view with nowhere to draw.
func (v View) Visible() image.Rectangle {
	if v.surface == nil {
		return image.Rectangle{}
	}
	return untranslateRect(v.clipped(), v.origin)
}

func (v View) Empty() bool { return v.clipped().Empty() }

// Sub returns a view onto r, expressed in this view's coordinates. Clipping only ever
// narrows: a caller cannot hand a child room it does not have itself.
func (v View) Sub(r image.Rectangle) View {
	if v.surface == nil {
		return View{}
	}
	return View{
		surface: v.surface,
		scope:   v.scope,
		origin:  translatePoint(v.origin, r.Min),
		size:    rectangleSize(r),
		clip:    v.clipped().Intersect(translateRect(r, v.origin)),
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
// An unknown shape is a programmer error and panics before changing the cursor,
// including on a clipped or offscreen view.
func (v View) PlaceCursor(x, y int, style CursorStyle) {
	v.scope.requireWritable()
	style = style.normalized()
	if v.cursor == nil {
		return
	}
	p := translatePoint(image.Pt(x, y), v.origin)
	if !p.In(v.clipped()) {
		return
	}
	*v.cursor = Cursor{Visible: true, Pos: p, Style: style}
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
	if v.surface == nil || !p.In(v.clipped()) {
		return nil
	}
	return v.surface.cellAt(p.X, p.Y)
}

// MergeStyle lays style over the display atom whose head is at local (x, y),
// preserving the roles it already carries, and reports whether it named a head inside
// the view. It is an operation rather than a mutable Cell pointer so that changing
// appearance cannot also replace one half of a wide grapheme.
func (v View) MergeStyle(x, y int, style Style) bool {
	v.scope.requireWritable()
	p := translatePoint(image.Pt(x, y), v.origin)
	if v.surface == nil || !p.In(v.clipped()) {
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

func (v View) Ground() Ground { return v.surface.Ground() }

// Blend changes foreground and background together so a region recedes
// without moving or replacing its text. Terminal defaults resolve through
// Ground first; unknown colors remain unchanged.
// NaN opacity panics before changing cells, including for an empty region.
func (v View) Blend(r image.Rectangle, over Color, opacity float64) {
	v.scope.requireWritable()
	opacity = normalizedOpacity(opacity)
	if v.surface == nil || over.Default() || opacity <= 0 {
		return
	}
	area := v.clipped().Intersect(translateRect(r, v.origin))
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

// Fade blends each foreground toward its own background, preserving mixed
// themes without guessing one overlay color. Zero leaves text unchanged and
// one hides it; unresolved terminal-default colors are left alone.
// A NaN amount panics before changing cells, including for an empty region.
func (v View) Fade(r image.Rectangle, amount float64) {
	v.scope.requireWritable()
	amount = normalizedOpacity(amount)
	if v.surface == nil || amount <= 0 {
		return
	}
	area := v.clipped().Intersect(translateRect(r, v.origin))
	if area.Empty() {
		return
	}
	ground := v.surface.ground
	v.surface.mutateAppearance(area, func(current *Style, _ *string) {
		style := ground.Resolve(*current)
		current.FG = style.FG.Blend(style.BG, amount)
	})
}

func (v View) Fill(r image.Rectangle, style Style) {
	v.scope.requireWritable()
	if v.surface == nil {
		return
	}
	area := v.clipped().Intersect(translateRect(r, v.origin))
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

// Text never splits a multi-column cluster: one that would straddle an edge is dropped
// and its visible columns blanked, because part of a glyph is worse than a gap. A
// zero-width cluster joins the display atom to its left rather than taking a column.
// Text returns the column advance, including any width change of that preceding
// atom and any columns outside the clip.
func (v View) Text(x, y int, s string, style Style) int {
	v.scope.requireWritable()
	if v.surface == nil {
		return 0
	}
	clip := v.clipped()
	p := translatePoint(image.Pt(x, y), v.origin)
	// A row outside the clip is measured exactly as a visible one; it only skips
	// the writes. Clipping must not change the advance a caller chains from.
	visible := p.Y >= clip.Min.Y && p.Y < clip.Max.Y

	cx := p.X
	advanced := 0
	clusters := grapheme.New(s)
	for clusters.Next() {
		cluster := clusters.Value()
		if control(cluster) {
			continue
		}
		w := ClusterWidth(cluster)
		if w == 0 {
			delta := v.combine(cx, p.Y, cluster)
			cx = layout.Translate(cx, delta)
			advanced = layout.Translate(advanced, delta)
			continue
		}
		end := layout.Translate(cx, w)
		advanced = layout.Translate(advanced, w)
		switch {
		case !visible || end <= clip.Min.X || cx >= clip.Max.X:
			// The column still advances: a caller measuring what it drew is asking
			// about the text, not about how much of it landed.
		case cx < clip.Min.X || end > clip.Max.X:
			v.blank(min(max(cx, clip.Min.X), clip.Max.X), min(end, clip.Max.X), p.Y, style)
		default:
			v.writeAtom(cx, p.Y, w, Cell{content: v.surface.capture.own(cluster), Style: style})
		}
		cx = end
	}
	return advanced
}

func (v View) writeAtom(cx, y, w int, head Cell) {
	surf := v.surface
	if head.content == "" {
		return
	}
	for offset := range w {
		surf.repairAtom(layout.Translate(cx, offset), y)
	}
	head.span = 0
	if w > 1 {
		head.span = span(w)
	}
	*surf.cellAt(cx, y) = head
	for offset := 1; offset < w; offset++ {
		*surf.cellAt(layout.Translate(cx, offset), y) = Cell{Style: head.Style, Link: head.Link, span: span(-offset)}
	}
}

// blank replaces [from,to) with styled single cells, preserving the atom invariant at
// both ends. Coordinates are on the surface and already clipped.
func (v View) blank(from, to, y int, style Style) {
	for column := from; column < to; column++ {
		v.surface.repairAtom(column, y)
		*v.surface.cellAt(column, y) = Cell{Style: style}
	}
}

func ownedCluster(cluster string) string {
	if len(cluster) == 1 && cluster[0] < utf8.RuneSelf {
		return asciiClusters[cluster[0]]
	}
	return strings.Clone(cluster)
}

func (v View) combine(cx, y int, cluster string) int {
	head := layout.Translate(cx, -1)
	prev := v.surface.cellAt(head, y)
	if prev != nil && prev.span < 0 {
		head = layout.Translate(head, int(prev.span))
		prev = v.surface.cellAt(head, y)
	}
	if prev == nil || prev.span < 0 {
		return 0
	}
	base := prev.content
	if base == "" {
		// The implicit blank still advances a column on the wire.
		base = " "
	}
	if !image.Pt(head, y).In(v.clipped()) {
		// A head outside the clip is not repainted, but the join still changes
		// its width, and the advance reports that whatever was clipped.
		return graphemeWidth(base+cluster) - prev.Width()
	}
	if !v.surface.capture.admits(int64(len(base) + len(cluster))) {
		return 0
	}
	combined := *prev
	combined.content = base + cluster
	width := graphemeWidth(combined.content)
	delta := width - prev.Width()
	end := layout.Translate(head, width)
	if end > v.clipped().Max.X {
		v.blank(head, v.clipped().Max.X, y, combined.Style)
	} else {
		if v.surface.capture != nil {
			combined.content = v.surface.capture.own(combined.content)
		}
		v.writeAtom(head, y, width, combined)
	}
	return delta
}

// Link stamps target onto the display atoms whose heads occupy w columns from local
// (x, y). It is separate from [View.Text] because a link usually spans a run that was
// drawn in several pieces.
func (v View) Link(x, y, w int, target string) {
	v.scope.requireWritable()
	if v.surface == nil || w <= 0 {
		return
	}
	clip := v.clipped()
	at := translatePoint(image.Pt(x, y), v.origin)
	if at.Y < clip.Min.Y || at.Y >= clip.Max.Y {
		return
	}
	from := max(at.X, clip.Min.X)
	to := min(layout.Translate(at.X, w), clip.Max.X)
	if from >= to {
		return
	}
	target = v.surface.capture.own(LinkTarget(target))
	v.surface.mutateAppearance(Area(from, at.Y, to-from, 1), func(_ *Style, link *string) {
		*link = target
	})
}

// Paint registers r in the frame's [Painter] composition. Native content stays
// below the text plane; registration order resolves overlap within each plane.
// Unchanged cells and ordered regions with equal identities at equal positions
// persist across frames. Identity zero repaints every frame. The region must fit
// entirely because grid cannot crop opaque terminal content. Cells remain available
// for captions where the terminal supports layering.
func (v View) Paint(r image.Rectangle, id uint64, by Painter) {
	v.scope.requireWritable()
	if v.surface == nil || by == nil || r.Empty() {
		return
	}
	area := translateRect(r, v.origin)
	if !area.In(v.clipped()) {
		return
	}
	if store := v.surface.capture; store != nil {
		store.err = errScrollbackPaint
		return
	}
	v.surface.paints = append(v.surface.paints, painted{rect: area, id: id, by: by})
}
