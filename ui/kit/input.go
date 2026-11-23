package kit

import (
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// InputConfig carries the base text field configuration in Behavior and adds its
// appearance. Behavior.Check is an event-side check; rendering never runs it. Scope
// selects a live application theme and Appearance overrides its tokens for this
// instance.
type InputConfig struct {
	Behavior   base.TextConfig
	Theme      Theme
	Scope      *ThemeScope
	Appearance ThemeOverride
	Glyphs     Glyphs
}

// Input is a finished single-line input around one base.Text. The controller owns
// its text, history, validation, semantic identity and interaction state.
// An Input must not be copied after first use.
type Input struct {
	noCopy     noCopy
	controller *base.Text
	appearance appearance
	glyphs     Glyphs
}

// NewInput constructs one controller and its standalone appearance. It panics
// when Theme and Scope both declare palette sources.
func NewInput(config InputConfig) *Input {
	look := newAppearance(config.Theme, config.Scope, config.Appearance)
	input := &Input{controller: base.NewText(config.Behavior), appearance: look, glyphs: config.Glyphs.clone()}
	input.controller.Element().SetPaint(input.Draw)
	input.appearance.bind(input.Element())
	return input
}

// Controller returns the sole text and validation owner. Passing it to Form as a
// Field explicitly gives that form its field-presentation slot: the form's look
// replaces this standalone skin and its instance overrides. The text owner stays
// the same. Compose Element directly to retain the standalone appearance.
func (i *Input) Controller() *base.Text { return i.controller }

func (i *Input) Element() scene.Element { return i.controller.Element() }

func (i *Input) HeightForWidth(width int) int { return i.controller.HeightForWidth(width) }

func (i *Input) Draw(frame scene.Frame) {
	i.controller.DrawWith(frame, i.appearance.palette().Look(i.glyphs))
}
