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

func (b *box) empty() bool { return len(b.marks) == 0 }

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

// annotated centres one box above another and leaves the lower one on the baseline.
//
// It is what distinguishes a stacked relation from a superscript: the annotation
// belongs over the relation it qualifies, so the two read as one symbol, while a
// superscript sits beside its base and reads as a second one.
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

// indexed puts a root's index above and to the left of the radical.
//
// TeX tucks it into the crook of the sign, which a grid of cells has no room for.
// After the sign is where a power goes, so a cube root placed there would read as a
// cube; before and above it cannot be read as anything else.
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
		var run spanRun
		at := 0
		for _, item := range placed {
			if item.x > at {
				run.write(strings.Repeat(" ", item.x-at), grid.Style{})
				at = item.x
			}
			// Composition is built not to overlap. Be conservative if a malformed
			// upstream tree does: the earlier mark owns the cells already occupied.
			if item.x < at {
				continue
			}
			run.write(item.value, item.style)
			at += text.Width(item.value)
		}
		out[y] = run.done()
	}
	return out
}

// spanRun builds one row's spans without rewriting what it has already placed.
//
// Marks arrive one cell at a time and adjacent ones usually share a style. Merging
// them by appending to the last span's string copies everything written so far on
// every mark, which is quadratic in the length of the row: a formula of 56 KiB
// allocated gigabytes to draw a few thousand columns. A builder holds the run open
// until the style changes and copies it once.
type spanRun struct {
	line  text.Line
	run   strings.Builder
	style grid.Style
	open  bool
}

func (r *spanRun) write(value string, style grid.Style) {
	if value == "" {
		return
	}
	if r.open && r.style != style {
		r.close()
	}
	r.style, r.open = style, true
	r.run.WriteString(value)
}

func (r *spanRun) close() {
	if r.run.Len() > 0 {
		r.line = append(r.line, text.Span{Text: r.run.String(), Style: r.style})
		r.run.Reset()
	}
	r.open = false
}

func (r *spanRun) done() text.Line {
	r.close()
	return r.line
}

func repeatToWidth(glyph string, width int) string {
	unit := text.Width(glyph)
	if unit <= 0 || width <= 0 {
		return ""
	}
	return text.Truncate(strings.Repeat(glyph, width/unit+1), width, "")
}
