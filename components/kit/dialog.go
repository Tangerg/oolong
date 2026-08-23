package kit

import (
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/internal/identity"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Dialog is the polished composition of a [headless.Dialog] controller and its
// appearance part.
//
// The common call site needs only a stack, theme, glyphs, title and body. Controller
// and Panel expose the two parts without allowing them to be replaced independently.
type Dialog struct {
	controller *headless.Dialog
	panel      *DialogPanel
}

// DialogConfig is the complete construction state of [Dialog].
//
// Stack is required. A nil Open gives the underlying controller local ownership;
// setting it gives ownership to the caller without choosing a second constructor.
// The remaining fields configure the semantic dialog and its kit appearance together,
// so the title, panel and controller cannot be assembled out of step.
type DialogConfig struct {
	// Stack owns modal ordering and is required.
	Stack *headless.Stack
	// Open is optional caller-owned state. Nil starts locally closed.
	Open headless.Accessor[bool]
	// Theme and Glyphs define the panel appearance.
	Theme  Theme
	Glyphs Glyphs
	// Title and Description are copied semantic text; Title also labels the border.
	Title       string
	Description string
	// Body is the optional live content inside the panel.
	Body headless.Widget
	// Where places the panel, and Border selects its frame style.
	Where  layout.Placement
	Border Border
	// Keys supplies the map used to draw Hints. Hints is copied.
	Keys  *keymap.Map
	Hints []keymap.Action
}

// NewDialog constructs one dialog with kit defaults from config.
func NewDialog(config DialogConfig) *Dialog {
	panel := &DialogPanel{
		Theme:  config.Theme,
		Glyphs: config.Glyphs,
		Where:  config.Where,
		Border: config.Border,
		Keys:   config.Keys,
		Hints:  slices.Clone(config.Hints),
	}
	panel.SetBody(config.Body)
	controller := headless.NewDialog(headless.DialogConfig{
		Stack:       config.Stack,
		Open:        config.Open,
		Title:       config.Title,
		Description: config.Description,
		Content:     panel,
	})
	panel.dialog = controller
	return &Dialog{controller: controller, panel: panel}
}

// Controller returns the headless dialog that owns open state and semantics.
func (d *Dialog) Controller() *headless.Dialog {
	if d == nil {
		return nil
	}
	return d.controller
}

// Panel returns the appearance part installed in the dialog controller.
func (d *Dialog) Panel() *DialogPanel {
	if d == nil {
		return nil
	}
	return d.panel
}

// Semantics returns the underlying structural semantic projection.
func (d *Dialog) Semantics() headless.SemanticNode {
	if d == nil || d.controller == nil {
		return headless.SemanticNode{Role: headless.RoleDialog}
	}
	return d.controller.Semantics()
}

// DialogPanel is the kit appearance part of a [headless.Dialog].
//
// The behavior — open state, focus restoration, escape and outside-click policy — is
// owned below by the controller and stack. This part owns only border, palette,
// placement and body composition.
type DialogPanel struct {
	Theme Theme
	// Where the dialog goes. The zero value centres it and fills what the margin
	// leaves, which is what a dialog with a lot in it wants.
	Where layout.Placement
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

	dialog  *headless.Dialog
	body    headless.Widget
	focused bool
	content headless.PointerRegion
}

// Body returns what is inside the frame.
func (d *DialogPanel) Body() headless.Widget {
	if d == nil {
		return nil
	}
	return d.body
}

// SetBody replaces what is inside the frame and transfers keyboard ownership. A
// body that answers input receives it after the stack chooses this layer.
func (d *DialogPanel) SetBody(body headless.Widget) {
	if d == nil {
		return
	}
	if identity.Same(d.body, body) {
		return
	}
	if old, ok := d.body.(headless.Focusable); ok {
		old.Focus(false)
	}
	d.body = body
	if next, ok := d.body.(headless.Focusable); ok {
		next.Focus(d.focused)
	}
}

// Place is where the dialog goes, which is what [headless.Stack] asks.
func (d *DialogPanel) Place(image.Point) layout.Placement { return d.Where }

// Handle passes the event to the body, if the body answers input at all.
func (d *DialogPanel) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		handled, _ := d.content.Handle(mouse)
		return handled
	}
	if body, ok := d.body.(headless.Interactive); ok {
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
	if d == nil || d.focused == has {
		return
	}
	d.focused = has
	if body, ok := d.body.(headless.Focusable); ok {
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
	if w, h := v.Size(); w <= 0 || h <= 0 {
		d.content.Stage(v, image.Rectangle{}, nil)
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
	body := d.body
	d.content.Stage(v, inner, body)
	if body != nil {
		body.Draw(v.Sub(inner))
	}
}

// footer is the hints, spelled the way a border can hold them.
func (d *DialogPanel) footer() string {
	var out strings.Builder
	for _, action := range d.Hints {
		bound := d.Keys.Keys(action)
		if len(bound) == 0 {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("  ")
		}
		out.WriteString(bound[0].String())
		out.WriteByte(' ')
		out.WriteString(actionLabel(action))
	}
	return out.String()
}

func (d *DialogPanel) title() string {
	if d.dialog == nil {
		return ""
	}
	return d.dialog.Title()
}
