package kit

import (
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Entry is one labelled item of a transcript: a label naming its source, the body
// indented under it, and the blank rows that part it from the next item. The body
// is any passive block — a paragraph, a document, a diff — so rich and plain items
// share one layout. Retention and publication belong to the
// containing transcript, so an entry carries no interaction state, and the
// label's domain meaning belongs to the application.
type Entry struct {
	Theme Theme
	// Label names the source or role of the body. Empty draws no label and no gutter.
	Label string
	// LabelStyle overlays Theme.Muted. The zero value keeps the ordinary receding
	// label; a caller that gives a product role visual emphasis supplies that role's
	// style explicitly.
	LabelStyle grid.Style
	// Body is drawn under the label. Nil draws the label alone.
	Body grid.Drawable
	// Trailing is how many blank rows follow, so consecutive entries do not run
	// together. Zero uses one, which is what separates without doubling the spacing.
	Trailing int
}

var (
	_ base.Block     = Entry{}
	_ text.Projector = Entry{}
)

func (e Entry) HeightForWidth(width int) int {
	return e.block().HeightForWidth(width)
}

func (e Entry) Draw(v grid.View) {
	e.block().Draw(v)
}

// Rows preserves the complete label projection across horizontal clipping, so
// resizing cannot change a selection's text identity. Body offsets match Draw;
// label and trailing rows preserve the entry's visible vertical structure.
func (e Entry) Rows(width int) []text.Row {
	return e.block().Rows(width)
}

// block composes the entry from atoms, so its measurement, drawing and text
// projection are one layout rather than three that must agree.
func (e Entry) block() Stack {
	trailing := Gap(1)
	if e.Trailing > 0 {
		trailing = Gap(e.Trailing)
	}
	if e.Label == "" {
		return Stack{e.Body, trailing}
	}
	label := Label{Text: e.Label, Style: e.Theme.Muted.Merge(e.LabelStyle)}
	return Stack{label, Indent{Body: e.Body, Left: gutter}, trailing}
}

// gutter is how far the body is indented under its label, so the eye can find where
// one entry stops and the next starts without a rule between them.
const gutter = 2
