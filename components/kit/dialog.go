package kit

import (
	"image"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Dialog is the polished composition of a [headless.Dialog] controller and its
// appearance part.
//
// The common call site needs only a stack, theme, glyphs, title and body. Controller
// and Panel stay public so an application can extend behavior or appearance without
// forking this composition.
type Dialog struct {
	Controller *headless.Dialog
	Panel      *DialogPanel
}

// NewDialog constructs an uncontrolled dialog with kit defaults.
func NewDialog(
	stack *headless.Stack,
	theme Theme,
	glyphs Glyphs,
	title string,
	body headless.Widget,
) *Dialog {
	panel := &DialogPanel{Theme: theme, Glyphs: glyphs, Body: body}
	controller := headless.NewDialog(stack, title, panel)
	panel.Of = controller
	return &Dialog{Controller: controller, Panel: panel}
}

// NewControlledDialog constructs a kit dialog whose open state is caller-owned.
func NewControlledDialog(
	stack *headless.Stack,
	open headless.Accessor[bool],
	theme Theme,
	glyphs Glyphs,
	title string,
	body headless.Widget,
) *Dialog {
	panel := &DialogPanel{Theme: theme, Glyphs: glyphs, Body: body}
	controller := headless.NewControlledDialog(stack, open, title, panel)
	panel.Of = controller
	return &Dialog{Controller: controller, Panel: panel}
}

// Show opens the dialog.
func (d *Dialog) Show() { d.Controller.Show() }

// Dismiss closes the dialog and restores focus below it.
func (d *Dialog) Dismiss() { d.Controller.Dismiss() }

// Open reports whether the dialog is open.
func (d *Dialog) Open() bool { return d.Controller.Open() }

// Trigger constructs a headless activation part for this dialog.
func (d *Dialog) Trigger(label string, of headless.Widget) *headless.DialogTrigger {
	return d.Controller.Trigger(label, of)
}

// Semantics returns the underlying structural semantic projection.
func (d *Dialog) Semantics() headless.SemanticNode { return d.Controller.Semantics() }

// DialogPanel is the kit appearance part of a [headless.Dialog].
//
// The behavior — open state, focus restoration, escape and outside-click policy — is
// owned below by the controller and stack. This part owns only border, palette,
// placement and body composition.
type DialogPanel struct {
	// Of supplies the semantic title. [NewDialog] wires it automatically.
	Of    *headless.Dialog
	Theme Theme
	// Where the dialog goes. The zero value centres it and fills what the margin
	// leaves, which is what a dialog with a lot in it wants.
	Where layout.Placement
	// Body is what goes inside the frame. A body that answers input gets it: the
	// stack has already decided this is the layer with the keyboard.
	Body headless.Widget
	// Glyphs are the characters the frame is drawn with. See [Box.Glyphs].
	Glyphs Glyphs
	// Border draws the frame. The zero value takes the rounded one from the glyph
	// set, which reads as a panel.
	Border Border
	// Keys is where the hints' keystrokes are read from, and Hints are the actions to
	// show along the bottom border, where they do not cost a row. An action with
	// nothing bound to it is not shown.
	Keys  *keymap.Map
	Hints []keymap.Action

	content headless.PointerRegion
}

// Place is where the dialog goes, which is what [headless.Stack] asks.
func (d *DialogPanel) Place(image.Point) layout.Placement { return d.Where }

// Handle passes the event to the body, if the body answers input at all.
func (d *DialogPanel) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		handled, _ := d.content.Handle(mouse)
		return handled
	}
	if body, ok := d.Body.(headless.Interactive); ok {
		return body.Handle(ev)
	}
	return false
}

// Focus passes the keyboard to the body, if the body can hold it.
//
// A stack hands the keyboard to the layer on top and expects the layer to pass it
// on. Without this the news stops at the frame: the dialog is the layer, so a form
// inside one would never be told it is being typed at, and a field would draw no
// caret while taking every keystroke.
func (d *DialogPanel) Focus(has bool) {
	if body, ok := d.Body.(headless.Focusable); ok {
		body.Focus(has)
	}
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
func (d *DialogPanel) Backdrop(v grid.View) { d.Theme.Scrim.Over(v) }

// Draw paints the frame and the body.
func (d *DialogPanel) Draw(v headless.Frame) {
	d.content.Clear(v)
	if w, h := v.Size(); w <= 0 || h <= 0 {
		return
	}
	box := Box{
		Theme:       d.Theme,
		Glyphs:      d.Glyphs,
		Border:      d.Border,
		Padding:     layout.Symmetric(0, 1),
		Title:       d.title(),
		TitleAlign:  layout.Start,
		Footer:      d.footer(),
		FooterAlign: layout.End,
	}
	inner := box.InnerRect(v.Bounds().Size())
	box.paint(v.View)
	d.content.Stage(v, inner, d.Body)
	if d.Body != nil {
		d.Body.Draw(v.Sub(inner))
	}
}

// footer is the hints, spelled the way a border can hold them.
func (d *DialogPanel) footer() string {
	out := ""
	for _, action := range d.Hints {
		bound := d.Keys.Keys(action)
		if len(bound) == 0 {
			continue
		}
		if out != "" {
			out += "  "
		}
		out += bound[0].String() + " " + actionLabel(action)
	}
	return out
}

func (d *DialogPanel) title() string {
	if d.Of == nil {
		return ""
	}
	return d.Of.Title()
}
