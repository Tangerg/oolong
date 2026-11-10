package kit

import (
	"image"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// DialogConfig supplies modal policy, ordinary panel geometry and the finished appearance. The base controller owns semantic title and open state.
type DialogConfig struct {
	Scope              *ThemeScope
	Appearance         ThemeOverride
	Initial            bool
	Theme              Theme
	Glyphs             Glyphs
	Title, Description string
	Body               scene.Element
	Where              layout.Placement
	Border             Border
	DismissOutside     bool
	Dismissed          func()
	Keys               *keymap.Map
}

// Dialog composes the ordinary Panel and base modal controller. There is no
// second dialog-specific body route, open flag or focus owner in this layer.
// A Dialog must not be copied after first use: its controller retains this appearance.
type Dialog struct {
	noCopy     noCopy
	appearance appearance
	where      layout.Placement
	controller *base.Dialog
	panel      *Panel
}

// NewDialog composes one base dialog with an ordinary Panel. It panics for an invalid anchor, invalid child relation or competing Theme and Scope sources.
func NewDialog(config DialogConfig) *Dialog {
	if err := config.Where.Anchor.Validate(); err != nil {
		panic(err)
	}
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	d := &Dialog{appearance: look, where: config.Where}
	// The panel resolves the dialog's own scope, and its title is the controller's
	// semantic title, so neither has a second copy here.
	box := Box{Glyphs: config.Glyphs, Border: config.Border, Padding: layout.Symmetric(0, 1)}
	d.panel = newPanel(appearance{scope: look.scope}, box, config.Body, func() string { return d.controller.Title() })
	d.controller = base.NewDialog(base.DialogConfig{
		Initial: config.Initial, Title: config.Title, Description: config.Description, Content: d.panel.Element(), Keys: config.Keys, DismissOutside: config.DismissOutside, Dismissed: config.Dismissed,
		Place:    func(space image.Point) image.Rectangle { return d.where.In(space) },
		Backdrop: func(frame scene.Frame) { d.appearance.palette().Scrim.Over(frame.View) },
	})
	d.appearance.bind(d.Element())
	return d
}

func (d *Dialog) Controller() *base.Dialog { return d.controller }

// Panel returns the ordinary body container for content replacement and box geometry.
func (d *Dialog) Panel() *Panel { return d.panel }

func (d *Dialog) Element() scene.Element {
	if d == nil || d.controller == nil {
		return scene.Element{}
	}
	return d.controller.Element()
}

func (d *Dialog) Where() layout.Placement { return d.where }

// SetWhere panics for invalid Anchor or with scene.ErrBusy outside
// owner mutation, before the retained presentation can change.
func (d *Dialog) SetWhere(value layout.Placement) {
	element := d.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	if err := value.Anchor.Validate(); err != nil {
		panic(err)
	}
	if d.where == value {
		return
	}
	d.where = value
	element.Invalidate()
}
