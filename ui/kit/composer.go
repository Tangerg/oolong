package kit

import (
	"image"
	"slices"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

// DefaultComposerRows caps the editor when MaxRows is unspecified.
const DefaultComposerRows = 8

// ComposerConfig supplies initial editor state and prompt/help appearance. Scope selects a live theme; Appearance supplies instance token overrides.
type ComposerConfig struct {
	Scope       *ThemeScope
	Appearance  ThemeOverride
	Theme       Theme
	Glyphs      Glyphs
	Prompt      string
	Hints       []keymap.Action
	MaxRows     int
	Initial     string
	Placeholder string
	Keys        *keymap.Map
	Clipboard   base.Clipboard
}

// Composer composes one Editor with a prompt and help. Scene supplies the inset
// transform, so this component has no private pointer or keyboard forwarding path.
// Composer must not be copied after first use; its collaborators share one live identity.
type Composer struct {
	noCopy     noCopy
	appearance appearance
	glyphs     Glyphs
	prompt     string
	hints      []keymap.Action
	editor     base.Editor
	group      *scene.Group
}

// NewComposer panics for negative MaxRows or competing Theme and Scope palette
// sources. Zero MaxRows selects DefaultComposerRows.
func NewComposer(config ComposerConfig) *Composer {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	rows := config.MaxRows
	if rows == 0 {
		rows = DefaultComposerRows
	}
	c := &Composer{appearance: look, glyphs: config.Glyphs.clone(), prompt: config.Prompt, hints: slices.Clone(config.Hints)}
	c.editor.SetText(config.Initial)
	c.editor.ForgetHistory()
	c.editor.SetMaxRows(rows)
	c.editor.SetPlaceholder(config.Placeholder)
	c.editor.SetKeys(config.Keys)
	c.editor.SetClipboard(config.Clipboard)
	if err := c.editor.Element().SetPaint(func(frame scene.Frame) { c.editor.DrawWith(frame, c.appearance.palette().Look(c.glyphs)) }); err != nil {
		panic(err)
	}
	c.group = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: c.editor.Element(), Size: layout.Flex(1)}},
		Element: scene.ElementConfig{Measure: scene.MeasureRows(c.HeightForWidth), Paint: func(frame scene.Frame) {
			width, height := frame.Size()
			if c.prompt != "" {
				text.Of(c.prompt, c.appearance.palette().Accent).Draw(frame.View, 0, 0)
			}
			help := c.help()
			rows := help.HeightForWidth(width)
			if rows > 0 {
				help.Draw(frame.Sub(grid.Area(0, layout.Remaining(height, rows), width, rows)).View)
			}
		}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			marker := c.markerWidth()
			return []image.Rectangle{grid.Area(marker, 0, layout.Remaining(space.X, marker), layout.Remaining(space.Y, c.help().HeightForWidth(space.X)))}
		},
	})
	c.appearance.bind(c.group.Element())
	return c
}

func (c *Composer) Controller() *base.Editor { return &c.editor }

func (c *Composer) Element() scene.Element { return c.group.Element() }

func (c *Composer) HeightForWidth(width int) int {
	rows := c.editor.HeightForWidth(layout.Remaining(width, c.markerWidth()))
	return layout.Sum(rows, c.help().HeightForWidth(width))
}
func (c *Composer) markerWidth() int { return text.Width(c.prompt) }
func (c *Composer) help() Help {
	return Help{Theme: c.appearance.palette(), Keys: KeysFunc(c.editor.KeysFor), Show: c.hints}
}

func (c *Composer) Glyphs() Glyphs { return c.glyphs.clone() }

func (c *Composer) SetGlyphs(value Glyphs) { replaceGlyphs(c.Element(), &c.glyphs, value) }

func (c *Composer) Prompt() string { return c.prompt }

// SetPrompt panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (c *Composer) SetPrompt(value string) {
	element := c.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	if c.prompt == value {
		return
	}
	c.prompt = value
	element.Invalidate()
}

func (c *Composer) Hints() []keymap.Action { return slices.Clone(c.hints) }

// SetHints panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (c *Composer) SetHints(value []keymap.Action) {
	element := c.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	if slices.Equal(c.hints, value) {
		return
	}
	c.hints = slices.Clone(value)
	element.Invalidate()
}
