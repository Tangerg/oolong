package kit

import (
	"net/url"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
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
// Its height is not known until its width is, which is the whole reason a passive
// [headless.Block] measures itself: publication and a [headless.Static] viewport
// adapter both have to ask before they can decide how much room to give.
type Paragraph struct {
	// lines are private because every mutation must invalidate wrapped. Exposing them
	// made it possible to change the text while its cached rows still described the
	// old value.
	lines []text.Line
	// Indent is held clear on the left of every row, including continuations, so a
	// wrapped paragraph reads as one block rather than as several.
	Indent int
	// MaxRows caps the height. Zero means no cap; a cap replaces the last row it
	// keeps with one ending in an ellipsis.
	MaxRows int
	// Links makes what the text points at clickable on terminals that support it and
	// enables pure hit testing through [Paragraph.LinkAt].
	//
	// It is off by default. Text a program composed itself has nothing in it worth
	// finding, and marking up a line nobody will click costs a scan of every line
	// every time the width changes.
	Links bool
	// Exists says whether a path is a file, and is what lets the shapes that cannot be
	// told from prose be found — see [link.DetectIn]. Nil leaves them out, which is
	// right for text about somebody else's machine.
	Exists func(path string) bool

	// wrapped memoises the last wrap, which is asked for twice per frame — once to
	// measure and once to draw — and is the most expensive thing this widget does.
	wrapped []row
	atWidth int
	atLimit int
	fresh   bool
}

var _ headless.Block = (*Paragraph)(nil)

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
	p := &Paragraph{}
	p.SetText(linesOf(s, style))
	return p
}

// SetText replaces the logical lines. Paragraph copies lines and their spans; the
// caller may reuse or change its input after this returns.
func (p *Paragraph) SetText(lines []text.Line) {
	p.lines = text.CloneLines(lines)
	p.fresh = false
}

// Lines returns a deep copy of the paragraph's logical lines.
func (p *Paragraph) Lines() []text.Line {
	if p == nil {
		return nil
	}
	return text.CloneLines(p.lines)
}

// Measure is how many rows the paragraph needs at this width.
func (p *Paragraph) Measure(width int) int { return len(p.rows(width)) }

// Draw writes the paragraph, one wrapped row per row of v.
func (p *Paragraph) Draw(v grid.View) {
	w, h := v.Size()
	rows := p.rows(w)
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
	if r.line >= len(p.lines) || r.To <= r.From {
		return
	}
	whole := p.lines[r.line].String()
	if r.To > len(whole) {
		return
	}
	part := whole[r.From:r.To]
	for _, l := range link.DetectIn(whole, p.Exists) {
		start, end := max(l.Start, r.From)-r.From, min(l.End, r.To)-r.From
		if start >= end {
			continue
		}
		// A relative path is still found by LinkAt, but left without OSC 8 so the
		// terminal can resolve it against its reported directory.
		target := hyperlinkTarget(l)
		text.StampLink(v, p.Indent, y, part, start, end, target)
	}
}

// hyperlinkTarget is the presentation policy for terminal cells. Link detection
// remains independent of OSC 8; this appearance layer decides which destinations
// are safe to stamp onto a frame.
func hyperlinkTarget(l link.Link) string {
	if l.Kind == link.URL {
		return l.Target
	}
	if strings.HasPrefix(l.Target, "/") {
		return (&url.URL{Scheme: "file", Path: l.Target}).String()
	}
	return ""
}

// Rows is what the paragraph says, one entry per drawn row, so a selection over it
// can be copied.
//
// Each row carries what the wrap consumed at the break above it, which is why the
// wrap records where every row came from. Between words it swallowed a space, and
// splitting a word too long for the row swallowed nothing; neither is recoverable
// from the rows afterwards, and guessing wrong either runs two words together or
// breaks a word in half.
func (p *Paragraph) Rows(width int) []text.Row {
	rows := p.rows(width)
	out := make([]text.Row, 0, len(rows))
	prevTo, prevLine := 0, -1
	for _, r := range rows {
		row := text.Row{
			Text: r.Line.String(), Offset: p.Indent, Line: r.line + 1, Joined: r.Joined,
		}
		if r.Joined && r.line == prevLine && r.line < len(p.lines) {
			whole := p.lines[r.line].String()
			if prevTo <= r.From && r.From <= len(whole) {
				row.Gap = whole[prevTo:r.From]
			}
		}
		out = append(out, row)
		prevTo, prevLine = r.To, r.line
	}
	return out
}

// LinkAt is the complete destination at a position when the paragraph is laid out at
// width, and whether there is one. File line and column information is retained
// rather than flattened into the target string.
//
// Width is explicit because a passive Block has no committed routing lifecycle. The
// answer is a pure projection of the same wrapped rows Draw uses, so measuring or
// drawing cannot publish hidden hit-test state.
func (p *Paragraph) LinkAt(x, y, width int) (link.Link, bool) {
	if !p.Links || x < p.Indent || y < 0 {
		return link.Link{}, false
	}
	rows := p.rows(width)
	if y >= len(rows) {
		return link.Link{}, false
	}
	r := rows[y]
	if r.line >= len(p.lines) || r.To <= r.From {
		return link.Link{}, false
	}
	whole := p.lines[r.line].String()
	if r.To > len(whole) {
		return link.Link{}, false
	}
	part := whole[r.From:r.To]
	for _, destination := range link.DetectIn(whole, p.Exists) {
		start := max(destination.Start, r.From) - r.From
		end := min(destination.End, r.To) - r.From
		if start >= end {
			continue
		}
		from := layout.Sum(p.Indent, text.ColumnOf(part, start))
		to := layout.Sum(p.Indent, text.ColumnOf(part, end))
		if x >= from && x < to {
			return destination, true
		}
	}
	return link.Link{}, false
}

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
	if p.fresh && p.atWidth == room && p.atLimit == p.MaxRows {
		return p.wrapped
	}
	var rows []row
	for i, line := range p.lines {
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
	p.wrapped, p.atWidth, p.atLimit, p.fresh = rows, room, p.MaxRows, true
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
