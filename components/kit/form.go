package kit

import (
	"image"
	"slices"
	"strings"

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
	// Theme is the look, and Glyphs the characters the marks beside a choice are drawn
	// with.
	Theme  Theme
	Glyphs Glyphs
	// Title sits above the fields. Empty draws none and costs no row.
	Title string
	// Hints are the controller actions to show under the fields. Their keystrokes
	// always come from the controller's own map, so help cannot disagree with input.
	// An action with nothing bound to it is not shown.
	Hints []keymap.Action

	controller *headless.Form
	body       headless.PointerRegion
}

// FormConfig is the complete construction state of [Form].
//
// Controller is required and remains the sole owner of fields, keys, validation and
// submission. Nil Controller.Keys is materialized as the standard form map so the
// same map can drive both behavior and visible hints.
type FormConfig struct {
	// Theme and Glyphs define field roles and choice marks.
	Theme  Theme
	Glyphs Glyphs
	// Controller owns fields, keys, validation and submission and is required.
	Controller *headless.Form
	// Title is copied. Empty omits the heading row.
	Title string
	// Hints is copied and resolved through Controller.Keys.
	Hints []keymap.Action
}

// NewForm dresses one headless form without creating a second behavior surface.
func NewForm(config FormConfig) *Form {
	if config.Controller == nil {
		panic("kit: form requires a controller")
	}
	if config.Controller.Keys == nil {
		config.Controller.Keys = headless.DefaultFormKeys()
	}
	return &Form{
		Theme: config.Theme, Glyphs: config.Glyphs, controller: config.Controller,
		Title: strings.Clone(config.Title), Hints: slices.Clone(config.Hints),
	}
}

// Controller returns the headless form that owns fields and submission behavior.
func (f *Form) Controller() *headless.Form {
	if f == nil {
		return nil
	}
	return f.controller
}

// Measure is the title, the fields, and the hints.
func (f *Form) Measure(across int) int {
	if f == nil || f.controller == nil {
		return 0
	}
	return layout.Sum(f.rows(), f.controller.Measure(across))
}

// Draw dresses the form and paints it.
func (f *Form) Draw(v headless.Frame) {
	if f.controller == nil {
		f.body.Stage(v, image.Rectangle{}, nil)
		return
	}
	rects := (layout.Flow{Axis: layout.Down}).Rects(v.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(f.titleRows())},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(f.hintRows())},
	})
	bands := v.Subs(rects)
	f.body.Stage(v, rects[1], f.controller)
	if f.titleRows() > 0 {
		Label{Text: f.Title, Style: f.Theme.Heading, Ellipsis: f.Glyphs.Ellipsis}.Draw(bands[0].View)
	}
	f.controller.DrawWith(bands[1], f.Theme.Look(f.Glyphs))
	if f.hintRows() > 0 {
		Help{Theme: f.Theme, Keys: f.controller.Keys, Show: f.Hints}.Draw(bands[2].View)
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
	if f.controller == nil {
		return false
	}
	return f.controller.Handle(ev)
}

// Focus passes the keyboard to the form — see [headless.Form.Focus].
func (f *Form) Focus(has bool) {
	if f != nil && f.controller != nil {
		f.controller.Focus(has)
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
		if len(f.controller.Keys.Keys(action)) > 0 {
			return 1
		}
	}
	return 0
}
