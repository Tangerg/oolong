package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Code is a passive block of styled source text.
//
// Styling is supplied as [text.Line] values, which keeps syntax choice and the
// optional highlight module outside the component dependency graph. Code owns only
// the assembly every caller otherwise repeats: wrapping, optional line numbers and
// copying. A viewport can make a tall block interactive through [headless.Static].
type Code struct {
	// Gutter draws beside the source. Nil gives every column to the code.
	Gutter headless.RowGutter
	body   Paragraph
}

var (
	_ headless.Block         = (*Code)(nil)
	_ headless.TextProjector = (*Code)(nil)
)

// NewCode returns a code block that owns a copy of lines.
func NewCode(lines []text.Line) *Code {
	c := &Code{}
	c.SetText(lines)
	return c
}

// SetText replaces the source lines. The input is copied.
func (c *Code) SetText(lines []text.Line) { c.body.SetText(lines) }

// Lines returns a copy of the source lines.
func (c *Code) Lines() []text.Line {
	if c == nil {
		return nil
	}
	return c.body.Lines()
}

// Measure is how many wrapped rows the code needs at width.
func (c *Code) Measure(width int) int {
	if c == nil {
		return 0
	}
	return c.body.Measure(c.textWidth(width))
}

// Draw paints the number gutter and source text.
func (c *Code) Draw(view grid.View) {
	if c == nil || view.Empty() {
		return
	}
	width, height := view.Size()
	gutter := min(c.gutterWidth(), max(width, 0))
	contentWidth := layout.Remaining(width, gutter)
	if gutter > 0 {
		visible := view.Visible()
		first := min(max(visible.Min.Y, 0), max(height, 0))
		last := min(max(visible.Max.Y, first), max(height, 0))
		rows := c.body.textRows(contentWidth, first, last)
		c.Gutter.Draw(
			view.Sub(grid.Rect(0, first, gutter, last-first)),
			rows,
		)
	}
	c.body.Draw(view.Sub(grid.Rect(gutter, 0, contentWidth, height)))
}

// Rows returns the meaningful source text, with offsets aligned past its gutter.
func (c *Code) Rows(width int) []text.Row {
	if c == nil {
		return nil
	}
	gutter := c.gutterWidth()
	rows := c.body.Rows(layout.Remaining(width, gutter))
	for i := range rows {
		rows[i].Offset = layout.Sum(rows[i].Offset, gutter)
	}
	return rows
}

func (c *Code) gutterWidth() int {
	if c.Gutter == nil {
		return 0
	}
	return max(c.Gutter.Width(len(c.body.lines)), 0)
}

func (c *Code) textWidth(width int) int {
	return layout.Remaining(width, c.gutterWidth())
}
