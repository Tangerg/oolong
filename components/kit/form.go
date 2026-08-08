package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Form dresses a [headless.Form]: a title, the fields, and a row of hints under them.
//
// The fields draw themselves, because a field is generic over what it holds and nothing
// here could name every kind of one — see [headless.Look]. So this projects the theme
// as the handful of roles a field has, without changing the controller's own look.
type Form struct {
	// Of is the form being collected. It is spelled the way a slot names what goes in
	// it — see [github.com/Tangerg/oolong/core/layout.Slot] — because that is what
	// this is: one widget wrapped in the look it is drawn with.
	Of *headless.Form
	// Theme is the look, and Glyphs the characters the marks beside a choice are drawn
	// with.
	Theme  Theme
	Glyphs Glyphs
	// Title sits above the fields. Empty draws none and costs no row.
	Title string
	// Keys is where the hints' keystrokes are read from, and Hints are the actions to
	// show under the fields. An action with nothing bound to it is not shown.
	Keys  *keymap.Map
	Hints []keymap.Action

	body headless.PointerRegion
}

// Measure is the title, the fields, and the hints.
func (f *Form) Measure(across int) int {
	if f.Of == nil {
		return 0
	}
	return f.rows() + f.Of.Measure(across)
}

// Draw dresses the form and paints it.
func (f *Form) Draw(v headless.Frame) {
	f.body.Clear(v)
	if f.Of == nil {
		return
	}
	rects := layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Fixed(f.titleRows())},
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(f.hintRows())},
	)
	bands := v.Subs(rects)
	f.body.Stage(v, rects[1], f.Of)
	if f.titleRows() > 0 {
		Label{Text: f.Title, Style: f.Theme.Heading, Ellipsis: f.Glyphs.Ellipsis}.Draw(bands[0].View)
	}
	f.Of.DrawWith(bands[1], f.Theme.Look(f.Glyphs))
	if f.hintRows() > 0 {
		Help{Theme: f.Theme, Keys: f.Keys, Show: f.Hints}.Draw(bands[2].View)
	}
}

// Handle gives the event to the form, with a press moved up by the rows this drew
// above it.
//
// That translation is the whole reason this answers events at all. A field told a
// press was two rows further down than it was is a field that puts the caret in the
// wrong place, and it is the sort of mistake that only shows as "clicking the second
// field selects the first".
func (f *Form) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		handled, _ := f.body.Handle(mouse)
		return handled
	}
	if f.Of == nil {
		return false
	}
	return f.Of.Handle(ev)
}

// Focus passes the keyboard to the form — see [headless.Form.Focus].
func (f *Form) Focus(has bool) {
	if f.Of != nil {
		f.Of.Focus(has)
	}
}

// look is the theme as the handful of roles a field draws itself in.
//
// A field asks for a role and never for a colour, exactly as a widget here does. The
// translation is one place, which is what keeps a form looking like the rest of the
// interface without a field ever having heard of a theme.

func (f *Form) rows() int { return f.titleRows() + f.hintRows() }

func (f *Form) titleRows() int {
	if f.Title == "" {
		return 0
	}
	return 1
}

// hintRows is whether there is a hint row: one, if any of the actions asked for is
// bound to something.
func (f *Form) hintRows() int {
	for _, action := range f.Hints {
		if len(f.Keys.Keys(action)) > 0 {
			return 1
		}
	}
	return 0
}
