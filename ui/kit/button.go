package kit

import (
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

// ButtonConfig supplies one action and its finished appearance. Scope is a live
// application theme; Theme is an alternative static palette. Appearance overrides
// either source for this instance only.
type ButtonConfig struct {
	Behavior   base.ButtonConfig
	Theme      Theme
	Scope      *ThemeScope
	Appearance ThemeOverride
	// Empty truncates silently; terminal character policy comes from the caller's Glyphs.
	Ellipsis string
}

// Button dresses one base.Button. Its controller owns its action, pointer gesture,
// semantic identity, label and availability. A Button must not be copied after use.
type Button struct {
	noCopy     noCopy
	controller *base.Button
	appearance appearance
	ellipsis   string
}

type buttonBrackets struct{ prefix, suffix string }

func (b buttonBrackets) width() int {
	return layout.Sum(text.Width(b.prefix), text.Width(b.suffix))
}

func (b *Button) brackets() buttonBrackets {
	if !b.controller.Availability().Disabled && b.Element().Focused() {
		return buttonBrackets{prefix: "[>", suffix: "<]"}
	}
	return buttonBrackets{prefix: "[ ", suffix: " ]"}
}

// NewButton constructs one action-bearing button. It panics if Theme and Scope
// are both specified, because they would declare competing palette sources.
func NewButton(config ButtonConfig) *Button {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	b := &Button{controller: base.NewButton(config.Behavior), appearance: look, ellipsis: strings.Clone(config.Ellipsis)}
	element := b.controller.Element()
	if err := element.SetPaint(b.Draw); err != nil {
		panic(err)
	}
	if err := element.SetMeasure(func(axis layout.Axis, _ int) int {
		if axis == layout.Down {
			return 1
		}
		return layout.Sum(text.Width(b.controller.Label()), b.brackets().width())
	}); err != nil {
		panic(err)
	}
	b.appearance.bind(element)
	return b
}

func (b *Button) Controller() *base.Button { return b.controller }

func (b *Button) Element() scene.Element { return b.controller.Element() }

func (*Button) HeightForWidth(int) int { return 1 }

func (b *Button) Draw(frame scene.Frame) {
	b.controller.Stage(frame)
	palette := b.appearance.palette()
	style := palette.Text
	brackets := b.brackets()
	if b.controller.Availability().Disabled {
		style = palette.Muted
	} else if b.Element().Focused() {
		style = style.Merge(palette.Selection).Merge(palette.Accent)
	}
	width, _ := frame.Size()
	label := text.Of(b.controller.Label(), style).Truncate(layout.Remaining(width, brackets.width()), b.ellipsis)
	frame.Fill(frame.Bounds(), style)
	x := text.Of(brackets.prefix, style).Draw(frame.View, 0, 0)
	x = layout.Sum(x, label.Draw(frame.View, x, 0))
	text.Of(brackets.suffix, style).Draw(frame.View, x, 0)
}
