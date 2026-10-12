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
//
// A Tabs value must not be copied after first use: its controller, committed strip
// geometry and pane pointer routing are one dressed component owner.
type Tabs struct {
	noCopy noCopy

	Theme  Theme
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
	Theme  Theme
	Glyphs Glyphs
	// Items are copied into the constructed headless controller.
	Items     []headless.Tab
	Selection headless.Accessor[int]
	// Keys maps tab actions. Nil uses the headless defaults.
	Keys           *keymap.Map
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

// HeightForWidth is the strip, the rule, and whatever the pane showing asks for.
func (t *Tabs) HeightForWidth(across int) int {
	if t.controller == nil {
		return 0
	}
	return layout.Sum(t.rows(), t.controller.HeightForWidth(across))
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

// Handle routes pointer input through the committed pane and strip. Pane coordinates
// are local; only primary presses select strip tabs. Pane cancellation may redraw or
// replace the route, so consumed or superseded input cannot fall through to the strip.
func (t *Tabs) Handle(ev input.Event) bool {
	mouse, ok := ev.(input.Mouse)
	if !ok {
		presented := t.presentation.Value()
		if presented.of == nil {
			return false
		}
		return presented.of.Handle(ev)
	}
	if handled, delivered := t.body.Handle(mouse); handled || delivered {
		return handled
	}
	presented := t.presentation.Value()
	if presented.of == nil {
		return false
	}
	if mouse.Pos.In(presented.strip) {
		if mouse.Action != input.MouseDown || mouse.Button != input.ButtonLeft {
			return false
		}
		if at, on := spanAt(presented.spans, mouse.Pos.X); on {
			return presented.of.SelectPresented(at)
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

func (t *Tabs) strip(v grid.View, presented tabsPresentation) {
	selected := presented.of.Selected()
	for i, box := range presented.spans {
		style := t.Theme.Muted
		if i == selected {
			style = t.Theme.Accent
		}
		tab, _ := presented.of.At(i)
		Label{Text: tab.Title, Style: style, Ellipsis: t.Glyphs.Ellipsis}.
			Draw(v.Sub(grid.Area(box.from, 0, box.to-box.from, 1)))
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

func (t *Tabs) rows() int {
	if t.Rule && t.Glyphs.Horizontal != "" {
		return 2
	}
	return 1
}
