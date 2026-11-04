package text

import "strings"

// Fill keeps dividers and padding on the same terminal-column boundaries as
// text layout. It repeats whole copies of pattern and fills unused columns with
// spaces; nonpositive widths produce empty text. Tabs become their in-pattern
// spaces, and a pattern with no drawable width produces only padding.
func Fill(pattern string, width int) string {
	if width <= 0 {
		return ""
	}
	pattern = line(Line{{Text: pattern}}.units()).String()
	columns := Width(pattern)
	if columns == 0 || columns > width {
		return strings.Repeat(" ", width)
	}
	filled := strings.Repeat(pattern, width/columns)
	// Adjacent copies can join into different graphemes. Only the complete
	// projection can decide how many cells remain, or whether a final atom fits.
	for Width(filled) > width {
		// A grapheme boundary can split a pattern copy. Keep complete copies,
		// then remeasure because removing the suffix can change the final cluster.
		copies := len(prefix(filled, width)) / len(pattern)
		filled = filled[:copies*len(pattern)]
	}
	if Width(filled) < width {
		// A trailing Unicode Prepend cluster can absorb the first padding
		// space. After that boundary, every additional space is independent.
		filled += " "
		filled += strings.Repeat(" ", width-Width(filled))
	}
	return filled
}
