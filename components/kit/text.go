package kit

import (
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/link"
	"github.com/Tangerg/oolong/core/text"
)

// Label is one line of text that does not wrap.
//
// Text too wide for its space is truncated rather than folded, because a label is
// used where exactly one row is available — a header, a status field, a table cell —
// and a label that grew to two rows would push whatever is below it off the screen.
type Label struct {
	Text  string
	Style grid.Style
	Align layout.Align
	// Ellipsis marks a truncation. Empty means truncate silently, which is right for
	// a value the user can see in full elsewhere and wrong for prose.
	Ellipsis string
}

// Measure is one row, whatever the width.
func (l Label) Measure(int) int { return 1 }

// Draw writes the label into the first row of v.
func (l Label) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	shown := text.Truncate(l.Text, w, l.Ellipsis)
	v.Text(l.Align.Offset(w, text.Width(shown)), 0, shown, l.Style)
}

// Paragraph is text that wraps to the width it is given.
//
// Its height is not known until its width is, which is the whole reason
// [headless.Sized] exists: a container has to ask before it can decide how much room
// to give.
type Paragraph struct {
	// Lines are the logical lines. A line's own styling survives wrapping.
	Lines []text.Line
	// Indent is held clear on the left of every row, including continuations, so a
	// wrapped paragraph reads as one block rather than as several.
	Indent int
	// MaxRows caps the height. Zero means no cap; a cap replaces the last row it
	// keeps with one ending in an ellipsis.
	MaxRows int
	// Links makes the URLs in the text clickable, on terminals that support it, and
	// records where they were drawn so a click can be answered — see [Paragraph.LinkAt].
	//
	// It is off by default. Text a program composed itself has no URLs in it worth
	// finding, and marking up a line nobody will click costs a scan of every line
	// every time the width changes.
	Links bool

	// wrapped memoises the last wrap, which is asked for twice per frame — once to
	// measure and once to draw — and is the most expensive thing this widget does.
	wrapped []row
	atWidth int
	fresh   bool
	// found is where the links went in the last frame drawn.
	found link.Map
}

// row is a wrapped row and which of the paragraph's lines it came from.
//
// The index is what makes a link that wrapped still one link. Detection runs on the
// logical line, because a URL split across two rows is not two URLs — reading each
// row on its own would find a truncated address on the first and nothing on the
// second, and a hyperlink to the wrong page is worse than no hyperlink.
type row struct {
	text.Wrapped
	line int
}

// NewParagraph is a paragraph of one plain styled string. Its newlines are line
// breaks.
func NewParagraph(s string, style grid.Style) *Paragraph {
	return &Paragraph{Lines: linesOf(s, style)}
}

// SetText replaces the content.
func (p *Paragraph) SetText(lines []text.Line) {
	p.Lines = lines
	p.fresh = false
}

// Measure is how many rows the paragraph needs at this width.
func (p *Paragraph) Measure(width int) int { return len(p.rows(width)) }

// Draw writes the paragraph, one wrapped row per row of v.
func (p *Paragraph) Draw(v grid.View) {
	w, h := v.Size()
	rows := p.rows(w)
	p.found.Reset()
	for y, r := range rows {
		if y >= h {
			return
		}
		r.Draw(v, p.Indent, y)
		if p.Links {
			p.stamp(v, y, r)
		}
	}
}

// stamp makes the links on one row clickable.
//
// The row's own text is the range of the logical line it came from, so a link's
// offsets are shifted into it and the parts of a link that landed on other rows are
// clipped away. A link that wrapped is stamped on each row it covers, with the same
// target on all of them, which is how a terminal draws one hyperlink over two lines.
func (p *Paragraph) stamp(v grid.View, y int, r row) {
	if r.line >= len(p.Lines) || r.To <= r.From {
		return
	}
	whole := p.Lines[r.line].String()
	if r.To > len(whole) {
		return
	}
	part := whole[r.From:r.To]
	for _, l := range link.Detect(whole) {
		start, end := max(l.Start, r.From)-r.From, min(l.End, r.To)-r.From
		if start >= end {
			continue
		}
		col, width := text.StampLink(v, p.Indent, y, part, start, end, l.URL)
		p.found.Add(p.Indent+col, y, width, l.URL)
	}
}

// LinkAt is the URL at a position in the space the paragraph was last drawn into,
// and whether there is one.
//
// It answers a click from what was drawn rather than by looking again: the record
// comes out of the same pass that wrote the cells, so there is nothing to keep in
// step and no chance of answering about text that has since changed.
func (p *Paragraph) LinkAt(x, y int) (string, bool) { return p.found.At(x, y) }

// rows is the wrap at this width, computed once per width.
//
// Each line is wrapped on its own rather than through text.WrapAll, so that a row
// still knows which line it came from. Nothing else would: the rows of every line
// arrive in one slice, and a byte range means nothing without the line it indexes.
func (p *Paragraph) rows(width int) []row {
	room := width - p.Indent
	if room <= 0 {
		return nil
	}
	if p.fresh && p.atWidth == room {
		return p.wrapped
	}
	var rows []row
	for i, line := range p.Lines {
		for _, wrapped := range line.Wrap(room) {
			rows = append(rows, row{Wrapped: wrapped, line: i})
		}
	}
	if p.MaxRows > 0 && len(rows) > p.MaxRows {
		rows = rows[:p.MaxRows]
		last := len(rows) - 1
		rows[last].Line = cutOff(rows[last].Line, room)
		// The row no longer draws the text its range describes — it ends in an
		// ellipsis standing for everything dropped after it — so it has no
		// provenance to offer. A link stamped from the old range would land on the
		// ellipsis, and a hyperlink over "…" is worse than none.
		rows[last].From, rows[last].To = 0, 0
	}
	p.wrapped, p.atWidth, p.fresh = rows, room, true
	return rows
}

// cutOff ends a line with an ellipsis to say that content was dropped after it.
//
// Truncating to the width would not do: the last row that survived a cap usually
// fits, and a row that fits is left alone — which would leave nothing to tell the
// reader there is more.
func cutOff(l text.Line, room int) text.Line {
	const ellipsis = "…"
	budget := room - text.Width(ellipsis)
	if budget <= 0 {
		return text.Of(ellipsis, grid.Style{})
	}
	if l.Width() > budget {
		l = l.Truncate(budget, "")
	}
	style := grid.Style{}
	if n := len(l); n > 0 {
		style = l[n-1].Style
	}
	return append(l, text.Span{Text: ellipsis, Style: style})
}

// linesOf splits a string on newlines into logical lines.
func linesOf(s string, style grid.Style) []text.Line {
	var lines []text.Line
	for line := range strings.SplitSeq(s, "\n") {
		lines = append(lines, text.Of(line, style))
	}
	return lines
}
