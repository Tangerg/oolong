package kit

import (
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Diff draws a change to a text.
//
// It takes hunks rather than lines because a change of three lines in a file of two
// thousand is a view nobody reads unless the two thousand are left out — and the break
// between one hunk and the next is how the reader is told they were. [diff.Hunks] is
// what makes them.
//
// It is a passive [headless.Block]. A change too tall for its pane becomes live
// viewport content through [headless.Static], without giving finished content an
// interaction lifecycle of its own.
//
// Diff owns its hunks and appearance. Mutations go through its methods so the wrapped
// representation measured and drawn in one frame cannot describe different inputs.
type Diff struct {
	hunks   []diff.Hunk
	theme   Theme
	glyphs  Glyphs
	numbers bool
	wrapped diffLayout
}

var _ headless.Block = (*Diff)(nil)

// NewDiff returns a diff that owns a copy of hunks.
func NewDiff(theme Theme, glyphs Glyphs, hunks []diff.Hunk) *Diff {
	d := &Diff{theme: theme, glyphs: glyphs}
	d.SetHunks(hunks)
	return d
}

// SetHunks replaces the parts of the change worth showing. The input and its text are
// copied, so changing or releasing the source cannot change or retain the component.
func (d *Diff) SetHunks(hunks []diff.Hunk) {
	if d == nil {
		return
	}
	d.hunks = cloneHunks(hunks)
	d.invalidate()
}

// Hunks returns a copy of the parts of the change being shown.
func (d *Diff) Hunks() []diff.Hunk {
	if d == nil {
		return nil
	}
	return cloneHunks(d.hunks)
}

// SetTheme changes the look. A diff is the one place in an interface where colour
// carries meaning on its own, which is why the theme has three styles for it and
// nothing else uses them.
func (d *Diff) SetTheme(theme Theme) {
	if d == nil {
		return
	}
	d.theme = theme
	d.invalidate()
}

// SetGlyphs changes the characters used between hunks and beside continuation rows.
func (d *Diff) SetGlyphs(glyphs Glyphs) {
	if d == nil {
		return
	}
	d.glyphs = glyphs
	d.invalidate()
}

// ShowNumbers controls whether each line's number in both texts appears down the
// left. Without numbers only the marks remain, which is what a narrow pane has room
// for.
func (d *Diff) ShowNumbers(show bool) {
	if d == nil || d.numbers == show {
		return
	}
	d.numbers = show
	d.invalidate()
}

// Measure reports the physical rows needed at width. Long lines wrap through the
// same layout Draw consumes, so measurement cannot promise rows drawing truncates.
func (d *Diff) Measure(width int) int {
	if d == nil {
		return 0
	}
	return len(d.layout(width))
}

// Draw paints the hunks in order, with a break between them.
func (d *Diff) Draw(v grid.View) {
	if d == nil || v.Empty() {
		return
	}
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	rows := d.layout(width)
	visible := v.Visible()
	first := min(max(visible.Min.Y, 0), len(rows))
	last := min(max(visible.Max.Y, first), len(rows))
	for y := first; y < last; y++ {
		row := rows[y]
		if row.gap {
			d.gap(v, y, width)
			continue
		}
		d.line(v, y, width, row)
	}
}

// diffLayout is one width's complete physical representation. Diff memoises one
// layout because Measure and Draw ask for the same width in every frame. It is private
// presentation state, not semantic component state.
type diffLayout struct {
	width int
	rows  []diffRow
	fresh bool
}

type diffRow struct {
	kind          diff.Kind
	numbers, mark string
	content       text.Line
	gap           bool
}

func (d *Diff) layout(width int) []diffRow {
	// Measure(0) still answers the content's height, as other text measurers do. Draw
	// cannot paint a zero-width view, but pretending the content has no rows would
	// make a parent collapse it permanently.
	width = max(width, 1)
	if d.wrapped.fresh && d.wrapped.width == width {
		return d.wrapped.rows
	}
	gutter := d.gutter(width)
	contentWidth := max(layout.Remaining(width, gutter.width()), 1)
	rows := d.wrapped.rows[:0]
	for hunkIndex, hunk := range d.hunks {
		if hunkIndex > 0 {
			rows = append(rows, diffRow{gap: true})
		}
		for _, line := range hunk.Lines {
			style := d.style(line.Kind)
			wrapped := text.Of(line.Text, style).Wrap(contentWidth)
			for physical, content := range wrapped {
				row := diffRow{kind: line.Kind, content: content.Line}
				if physical == 0 {
					row.numbers, row.mark = gutter.margin.of(line), gutter.mark(line.Kind)
				} else {
					row.numbers, row.mark = gutter.margin.blank(), gutter.continuation
				}
				rows = append(rows, row)
			}
		}
	}
	if len(rows) == 0 {
		rows = nil
	} else if cap(rows) > 2*len(rows)+16 {
		rows = append([]diffRow(nil), rows...)
	}
	d.wrapped = diffLayout{width: width, rows: rows, fresh: true}
	return rows
}

// invalidate releases references held only by a stale layout while preserving empty
// storage that the next layout can reuse.
func (d *Diff) invalidate() {
	clear(d.wrapped.rows)
	d.wrapped.rows = d.wrapped.rows[:0]
	d.wrapped.fresh = false
}

// line draws one physical row of a change. Its numbers, mark and content sit in the
// style that says what happened to the logical line.
//
// The style covers the whole row rather than the text alone. A background that stopped
// where the text did would leave a diff looking like ragged bunting, and the eye reads
// the block of colour long before it reads the mark.
func (d *Diff) line(v grid.View, y, width int, row diffRow) {
	style := d.style(row.kind)
	v.Fill(grid.Rect(0, y, width, 1), style)
	x := v.Text(0, y, row.numbers, style.Merge(d.theme.Subtle))
	markStyle := style
	if row.mark != row.kind.String() {
		markStyle = style.Merge(d.theme.Subtle)
	}
	x = layout.Sum(x, v.Text(x, y, row.mark, markStyle))
	row.content.Draw(v, x, y)
}

// margin is the column of line numbers down the left of a diff: how wide each of the
// two numbers is, or zero for a diff that does not draw them.
type margin struct{ each int }

func (m margin) width() int {
	if m.each == 0 {
		return 0
	}
	return 2*m.each + 2
}

func (m margin) blank() string { return strings.Repeat(" ", m.width()) }

// margin is how wide the numbers have to be to hold every line's.
func (d *Diff) margin() margin {
	if !d.numbers {
		return margin{}
	}
	widest := 1
	for _, hunk := range d.hunks {
		for _, line := range hunk.Lines {
			widest = max(widest, decimalWidth(line.Old), decimalWidth(line.New))
		}
	}
	return margin{each: widest}
}

// of is a line's place in each text, right-aligned, with a blank where the line is not
// in one of them.
//
// A column between the two and one after them, so that a line with only one of the
// numbers does not read as a longer one and the marks stay in a column of their own.
func (m margin) of(line diff.Line) string {
	if m.each == 0 {
		return ""
	}
	return right(decimal(line.Old), m.each) + " " + right(decimal(line.New), m.each) + " "
}

type diffGutter struct {
	margin       margin
	showMark     bool
	continuation string
}

// diffContentFloor is the least room worth preserving beside line numbers. Below
// it the numbers yield to the changed text; the colour and mark still say what the
// row is, while a one-column body would say almost nothing at a glance.
const diffContentFloor = 4

func (d *Diff) gutter(width int) diffGutter {
	gutter := diffGutter{}
	if width > 1 {
		gutter.showMark = true
		gutter.continuation = d.glyphs.Vertical
		if text.Width(gutter.continuation) != 1 {
			gutter.continuation = " "
		}
	}
	if numbers := d.margin(); d.numbers && width >= numbers.width()+1+diffContentFloor {
		gutter.margin = numbers
	}
	return gutter
}

func (g diffGutter) width() int {
	width := g.margin.width()
	if g.showMark {
		width++
	}
	return width
}

func (g diffGutter) mark(kind diff.Kind) string {
	if !g.showMark {
		return ""
	}
	return kind.String()
}

// gap is the break between two hunks, which is what says lines were left out.
func (d *Diff) gap(v grid.View, y, w int) {
	mark := d.glyphs.Ellipsis
	step := text.Width(mark)
	if step <= 0 {
		return
	}
	for x := 0; x < w; x += step {
		v.Text(x, y, mark, d.theme.Subtle)
	}
}

func (d *Diff) style(kind diff.Kind) grid.Style {
	switch kind {
	case diff.Added:
		return d.theme.Added
	case diff.Removed:
		return d.theme.Removed
	default:
		return d.theme.Context
	}
}

func cloneHunks(hunks []diff.Hunk) []diff.Hunk {
	if hunks == nil {
		return nil
	}
	out := make([]diff.Hunk, len(hunks))
	for i, hunk := range hunks {
		out[i] = diff.Hunk{Old: hunk.Old, New: hunk.New, Lines: make(diff.Script, len(hunk.Lines))}
		for j, line := range hunk.Lines {
			line.Text = strings.Clone(line.Text)
			out[i].Lines[j] = line
		}
	}
	return out
}
