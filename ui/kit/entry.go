package kit

import (
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Entry leaves retention and permanent publication to its containing transcript
// or publisher, so it carries no interaction lifecycle or routing state. Its
// private wrapping memo serves both measurement and drawing. The label's domain
// meaning belongs to the application, not to this passive text block.
type Entry struct {
	Theme Theme
	// Label names the source or role of the body. Empty draws no label and no gutter.
	Label string
	// LabelStyle overlays Theme.Muted. The zero value keeps the ordinary receding
	// label; a caller that gives a product role visual emphasis supplies that role's
	// style explicitly.
	LabelStyle grid.Style
	Body       string
	// Trailing is how many blank rows follow, so consecutive entries do not run
	// together. Zero uses one, which is what separates without doubling the spacing.
	Trailing int

	paragraph  Paragraph
	cachedBody string
	cachedText grid.Style
	fresh      bool
}

var (
	_ base.Block     = (*Entry)(nil)
	_ text.Projector = (*Entry)(nil)
)

func (e *Entry) HeightForWidth(width int) int {
	if e == nil {
		return 0
	}
	return layout.Sum(e.head(), len(e.body().rows(e.wrapWidth(width))), e.trailing())
}

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
		e.labelLine().Draw(v, 0, 0)
		y = 1
	}
	body := e.body()
	_, height := v.Size()
	indent := e.indent()
	body.Draw(v.Sub(grid.Area(indent, y, e.wrapWidth(width), layout.Remaining(height, y))))
}

// Rows preserves the complete label projection across horizontal clipping, so
// resizing cannot change a selection's text identity. Body offsets match Draw;
// label and trailing rows preserve the entry's visible vertical structure.
func (e *Entry) Rows(width int) []text.Row {
	if e == nil {
		return nil
	}
	out := make([]text.Row, 0, e.HeightForWidth(width))
	if e.head() > 0 {
		out = append(out, text.Row{Content: e.labelLine().Display().Clone()})
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

func (e *Entry) labelLine() text.Line {
	return text.Of(e.Label, e.Theme.Muted.Merge(e.LabelStyle))
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
		e.paragraph = *NewParagraph(e.Body, e.Theme.Text)
		e.cachedBody = e.Body
		e.cachedText = e.Theme.Text
		e.fresh = true
	}
	return &e.paragraph
}
