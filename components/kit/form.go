package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

// Form dresses a [headless.Form]: a title, the fields, and a row of hints under them.
//
// The fields draw themselves, because a field is generic over what it holds and nothing
// here could name every kind of one — see [headless.Look]. So this is where the look
// goes in rather than where the drawing happens, and it is one assignment: a theme
// becomes the handful of roles a field has, and a glyph set becomes the marks beside a
// choice.
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
	Keys  *input.Keymap
	Hints []input.Action
}

// Measure is the title, the fields, and the hints.
func (f Form) Measure(across int) int {
	if f.Of == nil {
		return 0
	}
	return f.rows() + f.Of.Measure(across)
}

// Draw dresses the form and paints it.
func (f Form) Draw(v grid.View) {
	if f.Of == nil {
		return
	}
	f.Of.Look = f.look()

	bands := layout.Rows(v,
		layout.Slot{Size: layout.Fixed(f.titleRows())},
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(f.hintRows())},
	)
	if f.titleRows() > 0 {
		Label{Text: f.Title, Style: f.Theme.Heading, Ellipsis: f.Glyphs.Ellipsis}.Draw(bands[0])
	}
	f.Of.Draw(bands[1])
	if f.hintRows() > 0 {
		Help{Theme: f.Theme, Keys: f.Keys, Show: f.Hints}.Draw(bands[2])
	}
}

// look is the theme as the handful of roles a field draws itself in.
//
// A field asks for a role and never for a colour, exactly as a widget here does. The
// translation is one place, which is what keeps a form looking like the rest of the
// interface without a field ever having heard of a theme.
func (f Form) look() headless.Look {
	return headless.Look{
		Text:      f.Theme.Text,
		Label:     f.Theme.Strong,
		Subtle:    f.Theme.Subtle,
		Selection: f.Theme.Selection,
		Accent:    f.Theme.Accent,
		Danger:    f.Theme.Danger,
		Taken:     f.Glyphs.Taken,
		Free:      f.Glyphs.Free,
	}
}

func (f Form) rows() int { return f.titleRows() + f.hintRows() }

func (f Form) titleRows() int {
	if f.Title == "" {
		return 0
	}
	return 1
}

// hintRows is whether there is a hint row: one, if any of the actions asked for is
// bound to something.
func (f Form) hintRows() int {
	for _, action := range f.Hints {
		if len(f.Keys.Keys(action)) > 0 {
			return 1
		}
	}
	return 0
}
