package kit

import (
	"context"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

// ButtonConfig supplies one action and its finished appearance. Scope is a live
// application theme; Theme is an alternative static palette. Appearance overrides
// either source for this instance only.
type ButtonConfig struct {
	Key, Label   string
	Keys         *keymap.Map
	Theme        Theme
	Scope        *ThemeScope
	Appearance   ThemeOverride
	Availability func() scene.Availability
	Activate     func(context.Context) (scene.ActionResult[struct{}], error)
}

// Button dresses one base.Button. Its controller owns its action, pointer gesture,
// semantic identity, label and availability. A Button must not be copied after use.
type Button struct {
	noCopy     noCopy
	controller *base.Button
	appearance appearance
}

// NewButton constructs one action-bearing button. It panics if Theme and Scope
// are both specified, because they would declare competing palette sources.
func NewButton(config ButtonConfig) *Button {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	b := &Button{controller: base.NewButton(base.ButtonConfig{Key: config.Key, Label: config.Label, Keys: config.Keys, Availability: config.Availability, Activate: config.Activate}), appearance: look}
	element := b.controller.Element()
	if err := element.SetPaint(b.Draw); err != nil {
		panic(err)
	}
	if err := element.SetMeasure(func(axis layout.Axis, _ int) int {
		if axis == layout.Down {
			return 1
		}
		return layout.Sum(text.Width(b.controller.Label()), 4)
	}); err != nil {
		panic(err)
	}
	b.appearance.bind(element)
	return b
}

// Controller is the sole behaviour owner shared by keyboard, pointer and AX.
func (b *Button) Controller() *base.Button { return b.controller }

// Element is the controller's exact scene identity with the kit skin installed.
func (b *Button) Element() scene.Element { return b.controller.Element() }

// HeightForWidth reserves one row for the button.
func (*Button) HeightForWidth(int) int { return 1 }

// Draw paints a bracketed label, including a visible focus marker without colour.
// It stages the same pointer extent as the base controller; it changes no model.
func (b *Button) Draw(frame scene.Frame) {
	b.controller.Stage(frame)
	palette := b.appearance.palette()
	style := palette.Text
	prefix, suffix := "[ ", " ]"
	if b.controller.Availability().Disabled {
		style = palette.Muted
	} else if b.Element().Focused() {
		style = style.Merge(palette.Selection).Merge(palette.Accent)
		prefix, suffix = "[>", "<]"
	}
	width, _ := frame.Size()
	label := prefix + text.Truncate(b.controller.Label(), layout.Remaining(width, 4), "…") + suffix
	frame.Fill(frame.Bounds(), style)
	Label{Text: label, Style: style}.Draw(frame.View)
}
