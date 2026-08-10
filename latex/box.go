package latex

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// box is the terminal analogue of TeX's box: marks placed relative to one
// baseline. above and below count rows on either side of that baseline.
type box struct {
	width, above, below int
	marks               []mark
}

type mark struct {
	x, y  int
	value string
	style grid.Style
}

func atom(value string, style grid.Style) box {
	if value == "" {
		return box{}
	}
	return box{
		width: text.Width(value), marks: []mark{{value: value, style: style}},
	}
}

func horizontal(parts ...box) box {
	var out box
	for _, part := range parts {
		out.above = max(out.above, part.above)
		out.below = max(out.below, part.below)
	}
	x := 0
	for _, part := range parts {
		out.add(part, x, 0)
		x += part.width
	}
	out.width = x
	return out
}

func (b *box) add(part box, dx, dy int) {
	for _, placed := range part.marks {
		placed.x += dx
		placed.y += dy
		b.marks = append(b.marks, placed)
	}
}

func stack(numerator, denominator box, rule bool, glyphs Glyphs, style grid.Style) box {
	width := max(numerator.width, denominator.width)
	if rule {
		width += 2
	}
	var out box
	out.width = width
	out.above = numerator.above + numerator.below + 1
	out.below = denominator.above + denominator.below + 1
	out.add(numerator, (width-numerator.width)/2, -1-numerator.below)
	out.add(denominator, (width-denominator.width)/2, 1+denominator.above)
	if rule {
		out.marks = append(out.marks, mark{
			value: repeatToWidth(glyphs.Horizontal, width), style: style,
		})
	}
	return out
}

func scripted(base, superscript, subscript box) box {
	if superscript.width == 0 && subscript.width == 0 {
		return base
	}
	var out box
	out.width = base.width + max(superscript.width, subscript.width)
	out.above = max(base.above, superscript.above+superscript.below+1)
	out.below = max(base.below, subscript.above+subscript.below+1)
	out.add(base, 0, 0)
	out.add(superscript, base.width, -1-superscript.below)
	out.add(subscript, base.width, 1+subscript.above)
	return out
}

func overlined(content box, glyphs Glyphs, style grid.Style) box {
	if content.width == 0 {
		return content
	}
	out := box{width: content.width, above: content.above + 1, below: content.below}
	out.add(content, 0, 0)
	out.marks = append(out.marks, mark{
		y: -out.above, value: repeatToWidth(glyphs.Horizontal, content.width), style: style,
	})
	return out
}

func delimited(content box, left, right Delimiter, style grid.Style) box {
	return horizontal(
		delimiter(content.above, content.below, left, style),
		content,
		delimiter(content.above, content.below, right, style),
	)
}

func delimiter(above, below int, glyphs Delimiter, style grid.Style) box {
	if above == 0 && below == 0 {
		return atom(glyphs.Single, style)
	}
	width := max(text.Width(glyphs.Top), text.Width(glyphs.Middle), text.Width(glyphs.Bottom))
	out := box{width: width, above: above, below: below}
	for y := -above; y <= below; y++ {
		value := glyphs.Middle
		switch y {
		case -above:
			value = glyphs.Top
		case below:
			value = glyphs.Bottom
		}
		out.marks = append(out.marks, mark{
			x: (width - text.Width(value)) / 2, y: y, value: value, style: style,
		})
	}
	return out
}

func radical(content box, glyphs Glyphs, textStyle, ruleStyle grid.Style) box {
	return horizontal(atom(glyphs.Radical, textStyle), overlined(content, glyphs, ruleStyle))
}

func (b *box) lines() []text.Line {
	if len(b.marks) == 0 {
		return nil
	}
	rows := make([][]mark, b.above+b.below+1)
	for _, placed := range b.marks {
		row := placed.y + b.above
		if row < 0 || row >= len(rows) || placed.value == "" {
			continue
		}
		rows[row] = append(rows[row], placed)
	}

	out := make([]text.Line, len(rows))
	for y, placed := range rows {
		slices.SortStableFunc(placed, func(a, b mark) int { return a.x - b.x })
		at := 0
		for _, item := range placed {
			if item.x > at {
				out[y] = appendSpan(out[y], text.Span{Text: strings.Repeat(" ", item.x-at)})
				at = item.x
			}
			// Composition is built not to overlap. Be conservative if a malformed
			// upstream tree does: the earlier mark owns the cells already occupied.
			if item.x < at {
				continue
			}
			out[y] = appendSpan(out[y], text.Span{Text: item.value, Style: item.style})
			at += text.Width(item.value)
		}
	}
	return out
}

func appendSpan(line text.Line, span text.Span) text.Line {
	if span.Text == "" {
		return line
	}
	if len(line) > 0 && line[len(line)-1].Style == span.Style && line[len(line)-1].Link == span.Link {
		line[len(line)-1].Text += span.Text
		return line
	}
	return append(line, span)
}

func repeatToWidth(glyph string, width int) string {
	unit := text.Width(glyph)
	if unit <= 0 || width <= 0 {
		return ""
	}
	return text.Truncate(strings.Repeat(glyph, width/unit+1), width, "")
}
