package markdown

import (
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Block is one immutable piece of a rendered document.
//
// A block keeps source and layout semantics together until its final width is
// known. That distinction matters for more than wrapping: a thematic break
// stretches, and a table allocates columns or becomes records when columns stop
// being readable. Exposing pre-laid-out lines would make those decisions too early.
//
// Blocks come from [Render] and [Stream]. Their zero value is empty. They can be
// retained, copied, measured and drawn independently; [Doc] is the convenient way
// to compose them with their inter-block spacing.
type Block struct {
	lines []text.Line
	table *table

	indent int
	marker text.Line
	rail   text.Line
	rule   bool
	fixed  bool

	blankBefore bool
}

// Measure reports how many rows the block needs at width. It excludes the blank
// row that may separate this block from the one before it; [Block.BlankBefore]
// exposes that relationship to custom composers.
func (b Block) Measure(width int) int { return len(b.appendRows(nil, width)) }

// Draw writes the block into v. It excludes any blank row before the block, because
// only the composer knows whether a preceding block exists.
func (b Block) Draw(v grid.View) {
	if v.Empty() {
		return
	}
	width, _ := v.Size()
	drawRows(v, b.appendRows(nil, width))
}

// Rows returns the meaningful text and offsets of the block's physical rows at
// width. Markers and quotation rails are decoration and are not included in Text.
func (b Block) Rows(width int) []text.Row { return publicRows(b.appendRows(nil, width)) }

// BlankBefore reports whether the renderer asked for a blank row before this block.
// It is false for the first block of a document and between the items of a tight
// list, and true where separate prose blocks need to read as separate things.
func (b Block) BlankBefore() bool { return b.blankBefore }

// row is one physical row: what it says, where it starts, and the decoration that
// ends immediately before it.
type row struct {
	text.Wrapped
	at     int
	prefix text.Line
	gap    string
}

func (b Block) appendRows(dst []row, width int) []row {
	start := len(dst)
	at := max(b.indent, 0)
	room := max(layout.Remaining(width, at), 1)
	switch {
	case b.table != nil:
		dst = b.table.appendRows(dst, room)
	case b.rule:
		dst = append(dst, row{Line: stretch(b.lines, room)})
	case b.fixed:
		for _, line := range b.lines {
			dst = append(dst, row{Line: line.Truncate(room, "")})
		}
	default:
		for _, line := range b.lines {
			dst = appendWrapped(dst, line, room)
		}
	}

	for i := start; i < len(dst); i++ {
		dst[i].at = at
		dst[i].prefix = b.rail
	}
	if start < len(dst) && len(b.marker) > 0 {
		// A marker replaces the rail on the first row because the two occupy the
		// same columns: a list inside a quotation is a bar and then a deeper bullet.
		dst[start].prefix = b.marker
	}
	return dst
}

func appendWrapped(dst []row, line text.Line, width int) []row {
	whole := line.String()
	previous := 0
	for _, wrapped := range line.Wrap(width) {
		gap := ""
		if wrapped.Joined && previous <= wrapped.From && wrapped.From <= len(whole) {
			gap = whole[previous:wrapped.From]
		}
		dst = append(dst, row{Wrapped: wrapped, gap: gap})
		previous = wrapped.To
	}
	return dst
}

func drawRows(v grid.View, rows []row) {
	visible := v.Visible()
	first := min(max(visible.Min.Y, 0), len(rows))
	last := min(max(visible.Max.Y, first), len(rows))
	for y := first; y < last; y++ {
		r := rows[y]
		if len(r.prefix) > 0 {
			r.prefix.Draw(v, r.at-r.prefix.Width(), y)
		}
		r.Draw(v, r.at, y)
	}
}

func publicRows(rows []row) []text.Row {
	out := make([]text.Row, len(rows))
	for i, row := range rows {
		out[i] = text.Row{
			Text: row.Line.String(), Offset: row.at,
			Joined: row.Joined, Gap: row.gap,
		}
	}
	return out
}

// stretch repeats a line until it fills the room there is, which is what a rule
// across the page is: one character, as many times as the width says.
func stretch(lines []text.Line, room int) text.Line {
	if len(lines) == 0 || len(lines[0]) == 0 {
		return nil
	}
	span := lines[0][0]
	width := text.Width(span.Text)
	if width <= 0 {
		return nil
	}
	span.Text = strings.Repeat(span.Text, room/width+1)
	return text.Line{span}.Truncate(room, "")
}

var _ layout.Measurer = Block{}
