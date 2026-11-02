package grid

import (
	"image"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/internal/grapheme"
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

func (s *Surface) Reset() {
	s.endDrawing()
	clear(s.cells)
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
func (v View) PlaceCursor(x, y int, style CursorStyle) {
	v.scope.requireWritable()
	if v.cursor == nil {
		return
	}
	p := translatePoint(image.Pt(x, y), v.origin)
	if !p.In(v.clipped()) {
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
func (v View) Blend(r image.Rectangle, over Color, opacity float64) {
	v.scope.requireWritable()
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
func (v View) Fade(r image.Rectangle, amount float64) {
	v.scope.requireWritable()
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

// Text writes s at local (x, y) and returns how many columns it advanced, including
// any it advanced outside the clip.
//
// A multi-column cluster is never split: one that would straddle an edge is dropped
// and its visible columns blanked, because part of a glyph is worse than a gap. A
// zero-width cluster joins the display atom to its left rather than taking a column.
func (v View) Text(x, y int, s string, style Style) int {
	v.scope.requireWritable()
	if v.surface == nil {
		return 0
	}
	clip := v.clipped()
	p := translatePoint(image.Pt(x, y), v.origin)
	if p.Y < clip.Min.Y || p.Y >= clip.Max.Y {
		return graphemeWidth(s)
	}

	cx := p.X
	advanced := 0
	clusters := grapheme.New(s)
	for clusters.Next() {
		cluster := clusters.Value()
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
		case end <= clip.Min.X || cx >= clip.Max.X:
			// The column still advances: a caller measuring what it drew is asking
			// about the text, not about how much of it landed.
		case cx < clip.Min.X || end > clip.Max.X:
			v.blank(min(max(cx, clip.Min.X), clip.Max.X), min(end, clip.Max.X), p.Y, style)
		default:
			v.writeAtom(cx, p.Y, w, cluster, style)
		}
		cx = end
	}
	return advanced
}

func (v View) writeAtom(cx, y, w int, cluster string, style Style) {
	surf := v.surface
	content := surf.capture.own(cluster)
	if content == "" {
		return
	}
	for offset := range w {
		surf.repairAtom(layout.Translate(cx, offset), y)
	}
	head := Cell{content: content, Style: style}
	if w > 1 {
		head.span = span(w)
	}
	*surf.cellAt(cx, y) = head
	for offset := 1; offset < w; offset++ {
		*surf.cellAt(layout.Translate(cx, offset), y) = Cell{Style: style, span: span(-offset)}
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

func (v View) combine(cx, y int, cluster string) {
	head := layout.Translate(cx, -1)
	prev := v.surface.cellAt(head, y)
	if prev != nil && prev.span < 0 {
		head = layout.Translate(head, int(prev.span))
		prev = v.surface.cellAt(head, y)
	}
	if prev == nil || prev.span < 0 || !image.Pt(head, y).In(v.clipped()) {
		return
	}
	base := prev.content
	if base == "" {
		// The implicit blank still advances a column on the wire.
		base = " "
	}
	if pool := v.surface.capture; pool != nil {
		if int64(len(cluster)) > pool.limit-int64(len(base)) {
			pool.err = ErrCaptureLimit
			return
		}
		prev.content = pool.own(base + cluster)
		return
	}
	prev.content = base + cluster
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

func graphemeWidth(s string) int {
	total := 0
	clusters := grapheme.New(s)
	for clusters.Next() {
		total = layout.Sum(total, ClusterWidth(clusters.Value()))
	}
	return total
}

// Fix ambiguous-width behavior instead of inheriting the server locale, which
// may differ from the terminal being drawn and would change identical layouts.
var columns = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: false}

// ClusterWidth is shared by measurement and drawing so terminal-column
// geometry has one authority. Width remains an estimate of terminal behavior:
// ambiguous characters use one column, while halfwidth-katakana sound marks
// U+FF9E and U+FF9F contribute their own spacing columns. Controls measure zero.
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

// Reject control bytes before they enter a cell: repaint writes cell content
// verbatim, so a stored Escape or tab would become an instruction instead of text.
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

func (s *Surface) regions() []painted {
	if s == nil {
		return nil
	}
	return s.paints
}
