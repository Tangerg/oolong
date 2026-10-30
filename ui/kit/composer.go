package kit

import (
	"image"
	"slices"
	"sync"

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
	Prompt     string
	Hints      []keymap.Action
	MaxRows    int
	editor     base.Editor
	group      *scene.Group
}

// NewComposer constructs an editor and its scene composition. It panics if Theme and Scope both declare palette sources.
func NewComposer(config ComposerConfig) *Composer {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	c := &Composer{appearance: look, Prompt: config.Prompt, Hints: slices.Clone(config.Hints), MaxRows: config.MaxRows}
	c.editor.SetText(config.Initial)
	c.editor.ForgetHistory()
	c.editor.Placeholder = config.Placeholder
	c.editor.Keys = config.Keys
	c.editor.Clipboard = config.Clipboard
	c.ensure()
	return c
}

// Controller returns the sole editor after assembling its scene parts. Its document, selection and scroll remain the editor's responsibility.
func (c *Composer) Controller() *base.Editor { c.ensure(); return &c.editor }

func (c *Composer) ensure() {
	if c.appearance.scope == nil {
		c.appearance = appearance{scope: NewThemeOwner(Theme{}).Scope()}
	}
	if c.group != nil {
		return
	}
	if err := c.editor.Element().SetPaint(func(frame scene.Frame) { c.editor.DrawWith(frame, c.appearance.palette().Look(Glyphs{})) }); err != nil {
		panic(err)
	}
	c.group = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: c.editor.Element(), Size: layout.Flex(1)}},
		Element: scene.ElementConfig{Measure: func(axis layout.Axis, width int) int {
			if axis == layout.Down {
				return c.HeightForWidth(width)
			}
			return 0
		}, Paint: func(frame scene.Frame) {
			width, height := frame.Size()
			if c.Prompt != "" {
				frame.Text(0, 0, c.Prompt, c.appearance.palette().Accent)
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
}

// Element returns the prompt, editor and help composition without a second input router.
func (c *Composer) Element() scene.Element { c.ensure(); return c.group.Element() }

// HeightForWidth measures the editor cap and help at the available width.
func (c *Composer) HeightForWidth(width int) int {
	rows := min(c.editor.HeightForWidth(layout.Remaining(width, c.markerWidth())), c.rows())
	return layout.Sum(rows, c.help().HeightForWidth(width))
}
func (c *Composer) markerWidth() int { return text.Width(c.Prompt) }
func (c *Composer) help() Help {
	return Help{Theme: c.appearance.palette(), Keys: c.keys(), Show: c.Hints}
}

var composerKeys = sync.OnceValue(base.DefaultEditorKeys)

func (c *Composer) keys() *keymap.Map {
	if c.editor.Keys != nil {
		return c.editor.Keys
	}
	return composerKeys()
}

func (c *Composer) rows() int {
	if c.MaxRows > 0 {
		return c.MaxRows
	}
	return DefaultComposerRows
}
