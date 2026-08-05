package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

// Dialog is a framed layer for a [headless.Stack]: a titled box with something
// inside it, dimming what it covers.
//
// It is the appearance half of the modal. The behaviour — which layer has the
// keyboard, what escape does, what a click outside means — is the stack's, and
// none of it is here. What is here is a border, a palette and a shade, which is
// exactly the set of decisions a product eventually has its own opinion about.
type Dialog struct {
	Theme Theme
	Title string
	// Where the dialog goes. The zero value centres it and fills what the margin
	// leaves, which is what a dialog with a lot in it wants.
	Where layout.Placement
	// Body is what goes inside the frame. A body that answers input gets it: the
	// stack has already decided this is the layer with the keyboard.
	Body headless.Widget
	// Border draws the frame. The zero value uses [Rounded], which reads as a panel.
	Border Border
	// Hints are drawn along the bottom border, where they do not cost a row.
	Hints []headless.Binding
}

// Place is where the dialog goes, which is what [headless.Stack] asks.
func (d *Dialog) Place(layout.Size) layout.Placement { return d.Where }

// Handle passes the event to the body, if the body answers input at all.
func (d *Dialog) Handle(ev input.Event) bool {
	if body, ok := d.Body.(headless.Interactive); ok {
		return body.Handle(ev)
	}
	return false
}

// Backdrop shades what the dialog covers.
//
// It is a separate step from Draw because a layer is handed a view of its own
// area and nothing else — which is what stops it drawing outside its box, and
// what makes reaching the space behind it something it has to ask for.
//
// What it paints is the theme's, not the dialog's. Dimming is part of a look and
// varies with it — a light interface takes less of it than a dark one — so it is
// held where the rest of the look is, and a dialog given no theme dims nothing.
func (d *Dialog) Backdrop(v grid.View) { d.Theme.Scrim.Over(v) }

// Draw paints the frame and the body.
func (d *Dialog) Draw(v grid.View) {
	if w, h := v.Size(); w <= 0 || h <= 0 {
		return
	}
	box := Box{
		Border:      d.border(),
		Padding:     layout.Symmetric(0, 1),
		Style:       d.Theme.Border,
		Fill:        d.Theme.Surface,
		Title:       d.Title,
		TitleStyle:  d.Theme.Heading,
		TitleAlign:  layout.Start,
		Footer:      d.footer(),
		FooterStyle: d.Theme.Subtle,
		FooterAlign: layout.End,
	}
	inner := box.Draw(v)
	if d.Body != nil {
		d.Body.Draw(inner)
	}
}

// footer is the hints, spelled the way a border can hold them.
func (d *Dialog) footer() string {
	out := ""
	for _, b := range d.Hints {
		if b.Hidden {
			continue
		}
		if out != "" {
			out += "  "
		}
		out += b.Key.String() + " " + b.Does
	}
	return out
}

func (d *Dialog) border() Border {
	if d.Border == (Border{}) {
		return Rounded
	}
	return d.Border
}
