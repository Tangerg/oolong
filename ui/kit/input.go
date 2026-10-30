package kit

import (
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// InputConfig supplies initial single-line text, validation and its appearance.
// Validate is an event-side check; rendering never runs it. Scope selects a live
// application theme and Appearance overrides its tokens for this instance.
type InputConfig struct {
	Key, Label, Initial, Placeholder string
	Validate                         func(string) error
	Keys                             *keymap.Map
	Clipboard                        base.Clipboard
	Theme                            Theme
	Scope                            *ThemeScope
	Appearance                       ThemeOverride
	Glyphs                           Glyphs
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
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	input := &Input{controller: base.NewText(base.TextConfig{Key: config.Key, Label: config.Label, Initial: config.Initial, Placeholder: config.Placeholder, Check: config.Validate, Keys: config.Keys, Clipboard: config.Clipboard}), appearance: look, glyphs: config.Glyphs}
	if err := input.controller.Element().SetPaint(input.Draw); err != nil {
		panic(err)
	}
	input.appearance.bind(input.Element())
	return input
}

// Controller returns the sole text and validation owner. Passing it to Form as a
// Field explicitly gives that form its field-presentation slot: the form's look
// replaces this standalone skin and its instance overrides. The text owner stays
// the same. Compose Element directly to retain the standalone appearance.
func (i *Input) Controller() *base.Text { return i.controller }

// Element is the controller's exact scene identity with the standalone skin.
func (i *Input) Element() scene.Element { return i.controller.Element() }

// HeightForWidth includes the field's label, answer and validation problem.
func (i *Input) HeightForWidth(width int) int { return i.controller.HeightForWidth(width) }

// Draw projects the current owner through this frame's resolved theme.
func (i *Input) Draw(frame scene.Frame) {
	i.controller.DrawWith(frame, i.appearance.palette().Look(i.glyphs))
}
