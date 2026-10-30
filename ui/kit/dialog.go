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
	Keys               *keymap.Map
}

// Dialog composes the ordinary Panel and base modal controller. There is no
// second dialog-specific body route, open flag or focus owner in this layer.
type Dialog struct {
	appearance appearance
	Where      layout.Placement
	controller *base.Dialog
	panel      *Panel
}

// NewDialog composes one base dialog with an ordinary Panel. It panics for an invalid child relation or competing Theme and Scope sources.
func NewDialog(config DialogConfig) *Dialog {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	d := &Dialog{appearance: look, Where: config.Where}
	d.panel = NewPanel(PanelConfig{Box: Box{Glyphs: config.Glyphs, Border: config.Border, Padding: layout.Symmetric(0, 1)}, Content: config.Body})
	d.controller = base.NewDialog(base.DialogConfig{
		Initial: config.Initial, Title: config.Title, Description: config.Description, Content: d.panel.Element(), Keys: config.Keys, DismissOutside: config.DismissOutside,
		Place:    func(space image.Point) image.Rectangle { return d.Where.In(space) },
		Backdrop: func(frame scene.Frame) { d.appearance.palette().Scrim.Over(frame.View) },
	})
	// The title is semantic state in the controller. Painting reads it directly.
	if err := d.panel.Element().SetPaint(func(frame scene.Frame) {
		box := d.panel.Box
		box.Theme = d.appearance.palette()
		box.Title = d.controller.Title()
		box.paint(frame.View)
	}); err != nil {
		panic(err)
	}
	d.appearance.bind(d.Element())
	return d
}

// Controller returns the only dialog open-state and semantic owner.
func (d *Dialog) Controller() *base.Dialog { return d.controller }

// Panel returns the ordinary body container for content replacement and box geometry.
func (d *Dialog) Panel() *Panel { return d.panel }

// Element is the base dialog's modal identity with the kit composition attached.
func (d *Dialog) Element() scene.Element {
	if d == nil || d.controller == nil {
		return scene.Element{}
	}
	return d.controller.Element()
}
