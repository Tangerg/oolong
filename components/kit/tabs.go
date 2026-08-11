package kit

import (
	"image"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
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
	// Theme is the look: the name of the pane showing is the thing the interface is
	// about, and the rest are there for reference.
	Theme Theme
	// Glyphs draw the rule under the strip. A tabs given none draws no rule, which is
	// the rule the whole package keeps.
	Glyphs Glyphs
	// Rule draws a line under the strip, which is what makes a strip of names read as
	// tabs rather than as a row of words.
	Rule bool

	controller   *headless.Tabs
	presentation headless.Snapshot[tabsPresentation]
	body         headless.PointerRegion
}

// TabsConfig is the complete construction state of [Tabs].
//
// Selection chooses state ownership in the same configuration that chooses content
// and appearance. Nil keeps selection local; a non-nil accessor gives ownership to
// the caller. NoRule suppresses the rule under the tab strip; its zero value keeps
// the kit default.
type TabsConfig struct {
	// Theme and Glyphs define the strip appearance.
	Theme  Theme
	Glyphs Glyphs
	// Items are copied into the constructed headless controller.
	Items []headless.Tab
	// Selection is optional caller-owned state. Nil keeps state local.
	Selection headless.Accessor[int]
	// Keys maps tab actions. Nil uses the headless defaults.
	Keys *keymap.Map
	// NoWrap stops movement at the ends. NoRule hides the separator below the strip.
	NoWrap, NoRule bool
}

// NewTabs constructs the kit's finished tab strip and its sole headless controller.
func NewTabs(config TabsConfig) *Tabs {
	return &Tabs{
		controller: headless.NewTabs(headless.TabsConfig{
			Items: config.Items, Selection: config.Selection, Keys: config.Keys, NoWrap: config.NoWrap,
		}),
		Theme:  config.Theme,
		Glyphs: config.Glyphs,
		Rule:   !config.NoRule,
	}
}

// Controller returns the headless tabs that own selection and pane behavior.
func (t *Tabs) Controller() *headless.Tabs {
	if t == nil {
		return nil
	}
	return t.controller
}

// Measure is the strip, the rule, and whatever the pane showing asks for.
func (t *Tabs) Measure(across int) int {
	if t.controller == nil {
		return 0
	}
	return layout.Sum(t.rows(), t.controller.Measure(across))
}

// Draw paints the strip, the rule under it, and the pane in what is left.
func (t *Tabs) Draw(v headless.Frame) {
	controller := t.controller
	if controller == nil {
		t.presentation.Stage(v, tabsPresentation{})
		t.body.Stage(v, image.Rectangle{}, nil)
		return
	}
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		t.presentation.Stage(v, tabsPresentation{})
		t.body.Stage(v, image.Rectangle{}, nil)
		return
	}
	rects := (layout.Flow{Axis: layout.Down}).Rects(v.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Fixed(t.rows() - 1)},
		{Size: layout.Flex(1)},
	})
	views := v.Subs(rects)
	presented := tabsPresentation{
		of:    controller,
		spans: t.boxes(),
		strip: image.Rect(0, 0, width, t.rows()),
		body:  rects[2],
	}
	t.presentation.Stage(v, presented)
	t.body.Stage(v, presented.body, presented.of)
	t.strip(views[0].View, presented)
	if t.Rule && t.Glyphs.Horizontal != "" {
		for x := range width {
			views[1].Text(x, 0, t.Glyphs.Horizontal, t.Theme.Border)
		}
	}
	controller.Draw(views[2])
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
	mouse, ok := ev.(input.Mouse)
	if !ok {
		if presented.of == nil {
			return false
		}
		return presented.of.Handle(ev)
	}
	if handled, delivered := t.body.Handle(mouse); delivered {
		return handled
	}
	if presented.of == nil {
		return false
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
	return false
}

// Focus passes the keyboard to the panes — see [headless.Tabs.Focus] — so that a
// dressed strip is a widget a container can hold like any other.
func (t *Tabs) Focus(has bool) {
	if t.controller != nil {
		t.controller.Focus(has)
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
		tab, _ := presented.of.At(i)
		Label{Text: tab.Title, Style: style, Ellipsis: t.Glyphs.Ellipsis}.
			Draw(v.Sub(grid.Rect(box.from, 0, box.to-box.from, 1)))
	}
}

// span is where one name sits along the strip.
type span struct{ from, to int }

// boxes is where each name goes, which the strip and a press both need — and need
// to agree about, which is why neither works it out for itself.
func (t *Tabs) boxes() []span {
	if t.controller == nil {
		return nil
	}
	out := make([]span, 0, t.controller.Len())
	at := 0
	for i := range t.controller.Len() {
		tab, _ := t.controller.At(i)
		width := text.Width(tab.Title)
		out = append(out, span{from: at, to: layout.Sum(at, width)})
		at = layout.Sum(at, width, tabGap)
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
