package text

import "strings"

// Fill repeats independent copies of the line and fills unused columns with
// unlinked, default-appearance spaces. Tabs keep their in-pattern columns. A
// pattern with no drawable width produces only padding. The returned Line owns
// its span storage and retains atom boundaries between copies and padding.
func (l Line) Fill(width int) Line {
	if width <= 0 {
		return nil
	}
	pattern := line(l.units())
	columns := pattern.Width()
	if columns == 0 || columns > width {
		return Line{{Text: strings.Repeat(" ", width), BreakBefore: true}}
	}
	pattern[0].BreakBefore = true
	var out Line
	remaining := width
	for remaining >= columns {
		out = append(out, pattern...)
		remaining -= columns
	}
	if remaining > 0 {
		out = append(out, Span{Text: strings.Repeat(" ", remaining), BreakBefore: true})
	}
	return out
}
