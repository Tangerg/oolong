package kit

import (
	"image"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// tabGap is the room between two names in the strip. Two columns, because one reads
// as a single word broken in half and three reads as two rows of something.
const tabGap = 2

// Tabs draws a strip of names above the pane that is showing.
//
// The strip is the part [headless.Tabs] refuses to have an opinion about — where it
// goes, what marks the one selected, whether there is a rule under it — and this is
// one answer to it. A press on a name selects that pane; everything else is offered
// to the pane, in the pane's own coordinates.
type Tabs struct {
	// Of is what is being shown. Nil draws nothing.
	Of *headless.Tabs
	// Theme is the look: the name of the pane showing is the thing the interface is
	// about, and the rest are there for reference.
	Theme Theme
	// Glyphs draw the rule under the strip. A tabs given none draws no rule, which is
	// the rule the whole package keeps.
	Glyphs Glyphs
	// Rule draws a line under the strip, which is what makes a strip of names read as
	// tabs rather than as a row of words.
	Rule bool

	presentation headless.Snapshot[tabsPresentation]
}

// Measure is the strip, the rule, and whatever the pane showing asks for.
func (t *Tabs) Measure(across int) int {
	if t.Of == nil {
		return 0
	}
	return t.rows() + t.Of.Measure(across)
}

// Draw paints the strip, the rule under it, and the pane in what is left.
func (t *Tabs) Draw(v headless.Frame) {
	t.presentation.Stage(v, tabsPresentation{})
	if t.Of == nil {
		return
	}
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	rects := layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Fixed(t.rows() - 1)},
		layout.Slot{Size: layout.Flex(1)},
	)
	views := v.Subs(rects)
	presented := tabsPresentation{
		of:    t.Of,
		spans: t.boxes(),
		strip: image.Rect(0, 0, width, t.rows()),
		body:  rects[2],
	}
	t.presentation.Stage(v, presented)
	t.strip(views[0].View, presented)
	if t.Rule && t.Glyphs.Horizontal != "" {
		for x := range width {
			views[1].Text(x, 0, t.Glyphs.Horizontal, t.Theme.Border)
		}
	}
	t.Of.Draw(views[2])
}

// Handle sends a press on the strip to the tab it landed on, and everything else to
// the pane — moved up by the rows the strip took, so the pane is handed a position
// in its own box.
//
// That translation is the whole reason this type answers events at all. A pane told
// a press was two rows further down than it was is a pane that answers the wrong
// click, and it is the sort of mistake that only shows up as "the second row selects
// the first".
func (t *Tabs) Handle(ev input.Event) bool {
	presented := t.presentation.Value()
	if presented.of == nil {
		return false
	}
	mouse, ok := ev.(input.Mouse)
	if !ok {
		return presented.of.Handle(ev)
	}
	if mouse.Pos.In(presented.strip) {
		if mouse.Action != input.MouseDown || mouse.Button != input.ButtonLeft {
			return false
		}
		if at, on := spanAt(presented.spans, mouse.Pos.X); on {
			presented.of.Select(at)
			return true
		}
		return false
	}
	if !mouse.Pos.In(presented.body) {
		return false
	}
	mouse.Pos = mouse.Pos.Sub(presented.body.Min)
	return presented.of.Handle(mouse)
}

// Focus passes the keyboard to the panes — see [headless.Tabs.Focus] — so that a
// dressed strip is a widget a container can hold like any other.
func (t *Tabs) Focus(has bool) {
	if t.Of != nil {
		t.Of.Focus(has)
	}
}

// At is which tab a column of the strip belongs to, and whether it belongs to one:
// the room between two names is in neither.
func (t *Tabs) At(x int) (int, bool) {
	return spanAt(t.presentation.Value().spans, x)
}

// strip writes the names, the one showing in the accent and the rest muted.
func (t *Tabs) strip(v grid.View, presented tabsPresentation) {
	selected := presented.of.Selected()
	for i, box := range presented.spans {
		style := t.Theme.Muted
		if i == selected {
			style = t.Theme.Accent
		}
		Label{Text: presented.of.Items[i].Title, Style: style, Ellipsis: t.Glyphs.Ellipsis}.
			Draw(v.Sub(grid.Rect(box.from, 0, box.to-box.from, 1)))
	}
}

// span is where one name sits along the strip.
type span struct{ from, to int }

// boxes is where each name goes, which the strip and a press both need — and need
// to agree about, which is why neither works it out for itself.
func (t *Tabs) boxes() []span {
	if t.Of == nil {
		return nil
	}
	out := make([]span, 0, len(t.Of.Items))
	at := 0
	for _, tab := range t.Of.Items {
		width := text.Width(tab.Title)
		out = append(out, span{from: at, to: at + width})
		at += width + tabGap
	}
	return out
}

func spanAt(spans []span, x int) (int, bool) {
	for i, box := range spans {
		if x >= box.from && x < box.to {
			return i, true
		}
	}
	return 0, false
}

type tabsPresentation struct {
	of          *headless.Tabs
	spans       []span
	strip, body image.Rectangle
}

// rows is how tall the strip is, the rule included.
func (t *Tabs) rows() int {
	if t.Rule && t.Glyphs.Horizontal != "" {
		return 2
	}
	return 1
}
