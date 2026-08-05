package kit

import (
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// Diff draws a change to a text.
//
// It takes hunks rather than lines because a change of three lines in a file of two
// thousand is a view nobody reads unless the two thousand are left out — and the break
// between one hunk and the next is how the reader is told they were. [diff.Hunks] is
// what makes them.
//
// It is a [headless.Sized], so a change too tall for its pane goes in a
// [headless.Viewport] and scrolls with no further arrangement.
type Diff struct {
	// Hunks are the parts of the change worth showing.
	Hunks []diff.Hunk
	// Theme is the look. A diff is the one place in an interface where colour carries
	// meaning on its own, which is why the theme has three styles for it and nothing
	// else uses them.
	Theme Theme
	// Glyphs are the characters the break between hunks is drawn with.
	Glyphs Glyphs
	// Numbers puts each line's number in both texts down the left. Off draws the marks
	// alone, which is what a narrow pane has room for.
	Numbers bool
}

// Measure is a row per line, and one for each break between hunks.
func (d Diff) Measure(int) int {
	rows := max(len(d.Hunks)-1, 0)
	for _, h := range d.Hunks {
		rows += len(h.Lines)
	}
	return rows
}

// Draw paints the hunks in order, with a break between them.
func (d Diff) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	gutter := d.gutter()
	y := 0
	for i, hunk := range d.Hunks {
		if i > 0 {
			d.gap(v, y, w)
			y++
		}
		for _, line := range hunk.Lines {
			if y >= h {
				return
			}
			d.line(v, y, w, gutter, line)
			y++
		}
	}
}

// line draws one line of the change: its numbers, its mark, and its text, all in the
// one style that says what happened to it.
//
// The style covers the whole row rather than the text alone. A background that stopped
// where the text did would leave a diff looking like ragged bunting, and the eye reads
// the block of colour long before it reads the mark.
func (d Diff) line(v grid.View, y, w, gutter int, line diff.Line) {
	style := d.style(line.Kind)
	v.Fill(grid.Rect(0, y, w, 1), style)

	x := 0
	if gutter > 0 {
		x += v.Text(x, y, d.numbers(line, gutter), style.Merge(d.Theme.Subtle))
	}
	x += v.Text(x, y, line.Kind.String(), style)
	v.Text(x, y, text.Truncate(line.Text, max(w-x, 0), "…"), style)
}

// numbers is a line's place in each text, right-aligned in the gutter, with a blank
// where the line is not in one of them.
func (d Diff) numbers(line diff.Line, gutter int) string {
	each := (gutter - 2) / 2
	return pad(number(line.Old), each) + " " + pad(number(line.New), each) + " "
}

// gutter is how wide the numbers are, or zero when they are not drawn: a column each
// for the widest number in either text, one between them, and one before the mark so
// that a line with only one of the two numbers does not read as a longer one.
func (d Diff) gutter() int {
	if !d.Numbers {
		return 0
	}
	widest := 1
	for _, hunk := range d.Hunks {
		for _, line := range hunk.Lines {
			widest = max(widest, len(number(line.Old)), len(number(line.New)))
		}
	}
	return 2*widest + 2
}

// gap is the break between two hunks, which is what says lines were left out.
func (d Diff) gap(v grid.View, y, w int) {
	mark := d.Glyphs.Ellipsis
	if mark == "" {
		return
	}
	for x := 0; x < w; x += text.Width(mark) {
		v.Text(x, y, mark, d.Theme.Subtle)
	}
}

func (d Diff) style(kind diff.Kind) grid.Style {
	switch kind {
	case diff.Added:
		return d.Theme.Added
	case diff.Removed:
		return d.Theme.Removed
	default:
		return d.Theme.Context
	}
}

// number is a line number as it is written, and nothing at all for a line that is not
// in that text.
func number(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// pad right-aligns s in w columns, which is how a column of numbers is read.
func pad(s string, w int) string {
	return strings.Repeat(" ", max(w-text.Width(s), 0)) + s
}
