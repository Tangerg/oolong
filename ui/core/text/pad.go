package text

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/layout"
)

// Pad prepares an independent field and pads it to at least width columns.
// Tabs expand from the field's origin. A grapheme that would absorb a neighbouring
// space gains its own space base so padding cannot take its width, style or link.
// That completion can increase the natural width. Use Pad(0, layout.Start)
// before measurement or wrapping to prepare the same field.
// Padding has default appearance and no link. Wider fields remain whole.
func (l Line) Pad(width int, alignment layout.Align) Line {
	out := line(l.units())
	if len(out) > 0 {
		value := out.String()
		if NextCluster(" "+value, 0) > 1 {
			out[0].Text = " " + out[0].Text
		}
		if PrevCluster(value+" ", len(value)+1) < len(value) {
			out[len(out)-1].Text += " "
		}
	}
	padding := layout.Remaining(width, out.Width())
	left := alignment.Offset(padding, 0)
	if left > 0 {
		out = append(Line{{Text: strings.Repeat(" ", left)}}, out...)
	}
	if right := padding - left; right > 0 {
		out = append(out, Span{Text: strings.Repeat(" ", right)})
	}
	return out
}
