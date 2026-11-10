package latex

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// A shared baseline lets independently rendered atoms compose without flattening
// fractions or scripts. above and below are row distances from that baseline.
type box struct {
	width, above, below int
	marks               []mark
}

type mark struct {
	x, y  int
	value text.Line
}

func (b *box) empty() bool { return len(b.marks) == 0 }

func atom(value string, style grid.Style) box {
	if value == "" {
		return box{}
	}
	line := text.Of(value, style).Wrap(0)[0].Line
	return box{width: line.Width(), marks: []mark{{value: line}}}
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
			value: text.Of(glyphs.Horizontal, style).Fill(width),
		})
	}
	return out
}

func scripted(base, superscript, subscript box) box {
	if superscript.empty() && subscript.empty() {
		return base
	}
	out := box{
		width: base.width + max(superscript.width, subscript.width),
		above: base.above,
		below: base.below,
	}
	out.add(base, 0, 0)
	if !superscript.empty() {
		out.above = max(out.above, superscript.above+superscript.below+1)
		out.add(superscript, base.width, -1-superscript.below)
	}
	if !subscript.empty() {
		out.below = max(out.below, subscript.above+subscript.below+1)
		out.add(subscript, base.width, 1+subscript.above)
	}
	return out
}

// A stacked relation needs its annotation above the relation; a superscript
// would instead change the meaning of the neighboring operand.
func annotated(base, annotation box) box {
	if annotation.empty() {
		return base
	}
	width := max(base.width, annotation.width)
	out := box{
		width: width,
		above: base.above + annotation.above + annotation.below + 1,
		below: base.below,
	}
	out.add(base, (width-base.width)/2, 0)
	out.add(annotation, (width-annotation.width)/2, -base.above-1-annotation.below)
	return out
}

// A terminal cannot tuck an index into a radical. Placing it before the sign
// keeps a root index distinct from a power of the radicand.
func indexed(root, index box) box {
	if index.empty() {
		return root
	}
	out := box{
		width: index.width + root.width,
		above: max(root.above, index.above+index.below+1),
		below: root.below,
	}
	out.add(index, 0, -1-index.below)
	out.add(root, index.width, 0)
	return out
}

func overlined(content box, glyphs Glyphs, style grid.Style) box {
	if content.width == 0 {
		return content
	}
	out := box{width: content.width, above: content.above + 1, below: content.below}
	out.add(content, 0, 0)
	out.marks = append(out.marks, mark{
		y: -out.above, value: text.Of(glyphs.Horizontal, style).Fill(content.width),
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
			x: (width - text.Width(value)) / 2, y: y, value: text.Of(value, style).Wrap(0)[0].Line,
		})
	}
	return out
}

func radical(content box, glyphs Glyphs, textStyle, ruleStyle grid.Style) box {
	return horizontal(atom(glyphs.Radical, textStyle), overlined(content, glyphs, ruleStyle))
}

func (b *box) lines() ([]text.Line, error) {
	if len(b.marks) == 0 {
		return nil, nil
	}
	rows := make([][]mark, b.above+b.below+1)
	for _, placed := range b.marks {
		row := placed.y + b.above
		if row < 0 || row >= len(rows) || len(placed.value) == 0 {
			continue
		}
		rows[row] = append(rows[row], placed)
	}

	out := make([]text.Line, len(rows))
	for y, placed := range rows {
		slices.SortStableFunc(placed, func(a, b mark) int { return a.x - b.x })
		var line text.Line
		at := 0
		for _, item := range placed {
			if item.x > at {
				line = append(line, text.Span{Text: strings.Repeat(" ", item.x-at), BreakBefore: true})
				at = item.x
			}
			// The box owns column placement; an overlap cannot be repaired by
			// clipping an independently prepared mathematical atom.
			if item.x < at {
				return nil, &formulaError{message: "formula atoms overlap in terminal columns"}
			}
			for i, span := range item.value {
				span.BreakBefore = span.BreakBefore || i == 0
				line = append(line, span)
			}
			at += item.value.Width()
		}
		out[y] = line
	}
	return out, nil
}
