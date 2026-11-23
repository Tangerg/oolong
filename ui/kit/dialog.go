package kit

import (
	"image"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

// DialogConfig carries the base modal policy in Behavior and adds panel geometry and
// appearance. The skin frames Behavior.Content in a panel, places it from Where and
// draws its backdrop, so Behavior.Place and Behavior.Backdrop must be nil. The base
// controller owns semantic title and open state.
type DialogConfig struct {
	Behavior   base.DialogConfig
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	Where      layout.Placement
	// Fit sizes the panel's height to its content at the placed width, up to
	// Where.Height, or the space when that is zero. The dialog then grows and
	// shrinks with what it shows, such as a validation message, instead of
	// reserving rows for content that may never appear.
	Fit    bool
	Border Border
}

// Dialog composes the ordinary Panel and base modal controller. There is no
// second dialog-specific body route, open flag or focus owner in this layer.
// A Dialog must not be copied after first use: its controller retains this appearance.
type Dialog struct {
	noCopy     noCopy
	appearance appearance
	where      layout.Placement
	fit        bool
	controller *base.Dialog
	panel      *Panel
}

// NewDialog composes one base dialog with an ordinary Panel. It panics for an
// invalid anchor, invalid child relation, skin-owned Behavior fields or competing
// Theme and Scope sources.
func NewDialog(config DialogConfig) *Dialog {
	if err := config.Where.Anchor.Validate(); err != nil {
		panic(err)
	}
	look := newAppearance(config.Theme, config.Scope, config.Appearance)
	requireSkinOwned("dialog Behavior.Place", config.Behavior.Place == nil)
	requireSkinOwned("dialog Behavior.Backdrop", config.Behavior.Backdrop == nil)
	d := &Dialog{appearance: look, where: config.Where, fit: config.Fit}
	// The panel resolves the dialog's own scope, and its title is the controller's
	// semantic title, so neither has a second copy here.
	box := Box{Glyphs: config.Glyphs, Border: config.Border, Padding: layout.Symmetric(0, 1)}
	d.panel = newPanel(appearance{scope: look.scope}, box, config.Behavior.Content, func() string { return d.controller.Title() })
	behavior := config.Behavior
	behavior.Content = d.panel.Element()
	behavior.Place = d.place
	behavior.Backdrop = func(frame scene.Frame) { d.appearance.palette().Scrim.Over(frame.View) }
	d.controller = base.NewDialog(behavior)
	d.appearance.bind(d.Element())
	return d
}

// place anchors the panel in space. A fitting dialog measures its panel at the
// width its placement allows and anchors a rectangle of that height instead.
func (d *Dialog) place(space image.Point) image.Rectangle {
	bounds := d.where.In(space)
	if !d.fit || bounds.Empty() {
		return bounds
	}
	fitted := d.where
	fitted.Width = bounds.Dx()
	// A zero height would mean as tall as the space, so content that measures
	// nothing still takes one row.
	fitted.Height = min(bounds.Dy(), max(d.panel.HeightForWidth(bounds.Dx()), 1))
	return fitted.In(space)
}

func (d *Dialog) Controller() *base.Dialog { return d.controller }

// Panel returns the ordinary body container for content replacement and box geometry.
func (d *Dialog) Panel() *Panel { return d.panel }

func (d *Dialog) Element() scene.Element {
	return d.controller.Element()
}

func (d *Dialog) Where() layout.Placement { return d.where }

func (d *Dialog) Fit() bool { return d.fit }

// SetFit panics with scene.ErrBusy outside owner mutation, before the retained
// presentation can change.
func (d *Dialog) SetFit(value bool) { setting.Assign(d.Element(), &d.fit, value) }

// SetWhere panics for invalid Anchor or with scene.ErrBusy outside
// owner mutation, before the retained presentation can change.
func (d *Dialog) SetWhere(value layout.Placement) {
	if err := value.Anchor.Validate(); err != nil {
		panic(err)
	}
	setting.Assign(d.Element(), &d.where, value)
}
