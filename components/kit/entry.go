package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Entry is one finished, labelled piece of text, ready for a transcript or the
// terminal's own scrollback.
//
// It is the stable half of a streaming interface. A recent entry may remain in a
// selectable transcript while interaction is worth its cost; an older one may be
// committed to the terminal, where it survives the program. The same passive Block
// works in both places and is measured before it is drawn because publication needs
// a row count:
//
//	e := &kit.Entry{Theme: th, Label: "build", Body: line}
//	output.Print(e)
//
// It owns no interaction lifecycle or routing state. Its transcript or printer
// decides how long it remains live. Entry is a pointer only because wrapping is
// memoised between Measure and Draw; that private cache is presentation state.
//
// Entry knows only labelled text. Whether a label denotes a person, process, log
// source or anything else is application grammar and remains with the caller.
type Entry struct {
	Theme Theme
	// Label names the source or role of the body. Empty draws no label and no gutter.
	Label string
	// LabelStyle overlays Theme.Muted. The zero value keeps the ordinary receding
	// label; a caller that gives a product role visual emphasis supplies that role's
	// style explicitly.
	LabelStyle grid.Style
	// Body is the text. Newlines are line breaks; everything else wraps.
	Body string
	// Trailing is how many blank rows follow, so consecutive entries do not run
	// together. Zero uses one, which is what separates without doubling the spacing.
	Trailing int

	paragraph  Paragraph
	cachedBody string
	cachedText grid.Style
	fresh      bool
}

var (
	_ headless.Block         = (*Entry)(nil)
	_ headless.TextProjector = (*Entry)(nil)
)

// Measure is how many rows the entry needs at this width.
func (e *Entry) Measure(width int) int {
	if e == nil {
		return 0
	}
	return layout.Sum(e.head(), len(e.body().rows(e.wrapWidth(width))), e.trailing())
}

// Draw paints the label, the body and the blank rows after it.
func (e *Entry) Draw(v grid.View) {
	if e == nil || v.Empty() {
		return
	}
	width, _ := v.Size()
	if width <= 0 {
		return
	}
	y := 0
	if e.head() > 0 {
		v.Text(0, 0, e.Label, e.Theme.Muted.Merge(e.LabelStyle))
		y = 1
	}
	body := e.body()
	_, height := v.Size()
	indent := e.indent()
	body.Draw(v.Sub(grid.Rect(indent, y, e.wrapWidth(width), layout.Remaining(height, y))))
}

// Rows returns the entry without its visual gutter, with body offsets aligned to
// where Draw places them. Label and trailing rows remain real text rows because a
// selection dragged across the entry includes the same vertical structure it saw.
func (e *Entry) Rows(width int) []text.Row {
	if e == nil {
		return nil
	}
	out := make([]text.Row, 0, e.Measure(width))
	if e.head() > 0 {
		out = append(out, text.Row{Text: e.Label})
	}
	for _, row := range e.body().Rows(e.wrapWidth(width)) {
		row.Offset = layout.Sum(row.Offset, e.indent())
		out = append(out, row)
	}
	for range e.trailing() {
		out = append(out, text.Row{})
	}
	return out
}

// gutter is how far the body is indented under its label, so the eye can find where
// one entry stops and the next starts without a rule between them.
const gutter = 2

func (e *Entry) head() int {
	if e.Label == "" {
		return 0
	}
	return 1
}

func (e *Entry) trailing() int {
	if e.Trailing > 0 {
		return e.Trailing
	}
	return 1
}

func (e *Entry) indent() int {
	if e.Label == "" {
		return 0
	}
	return gutter
}

func (e *Entry) wrapWidth(width int) int { return max(layout.Remaining(width, e.indent()), 1) }

func (e *Entry) body() *Paragraph {
	if !e.fresh || e.cachedBody != e.Body || e.cachedText != e.Theme.Text {
		e.paragraph.SetText(linesOf(e.Body, e.Theme.Text))
		e.cachedBody = e.Body
		e.cachedText = e.Theme.Text
		e.fresh = true
	}
	return &e.paragraph
}
