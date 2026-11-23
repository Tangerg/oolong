package kit

import (
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

// EditorConfig carries the [base.EditorConfig] in Behavior and adds the prompt and
// help appearance. The skin owns its edit session and draws the field with its
// theme, so Behavior.Session and Behavior.Look must be left zero.
// Scope selects a live theme; Appearance supplies instance token overrides.
type EditorConfig struct {
	Behavior   base.EditorConfig
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	Prompt     string
	Hints      []keymap.Action
}

// Editor dresses one [base.Editor] with a prompt and help. Scene supplies the inset
// transform, so this component has no private pointer or keyboard forwarding path.
// Editor must not be copied after first use; its collaborators share one live identity.
type Editor struct {
	noCopy     noCopy
	appearance appearance
	glyphs     Glyphs
	prompt     string
	hints      []keymap.Action
	editor     base.Editor
	group      *scene.Group
}

// NewEditor panics for negative MaxRows, an invalid cursor style, a Session or
// Look in Behavior, or competing Theme and Scope palette sources. Behavior.MaxRows
// means what it means for a [base.Editor]: zero leaves the field unlimited, so an
// editor that should stay short sets its cap.
func NewEditor(config EditorConfig) *Editor {
	look := newAppearance(config.Theme, config.Scope, config.Appearance)
	behavior := config.Behavior
	requireSkinOwned("editor Behavior.Session", behavior.Session == nil)
	requireSkinOwned("editor Behavior.Look", behavior.Look == (base.Look{}))
	c := &Editor{appearance: look, glyphs: config.Glyphs.clone(), prompt: strings.Clone(config.Prompt), hints: slices.Clone(config.Hints)}
	c.editor.SetSingleLine(behavior.SingleLine)
	c.editor.SetMask(behavior.Mask)
	c.editor.SetText(behavior.Initial)
	c.editor.ForgetHistory()
	c.editor.SetMaxRows(behavior.MaxRows)
	c.editor.SetPlaceholder(behavior.Placeholder)
	c.editor.SetKeys(behavior.Keys)
	c.editor.SetClipboard(behavior.Clipboard)
	c.editor.SetGutter(behavior.Gutter)
	c.editor.SetCursorStyle(behavior.CursorStyle)
	c.editor.Element().SetPaint(func(frame scene.Frame) { c.editor.DrawWith(frame, c.appearance.palette().Look(c.glyphs)) })
	c.group = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: c.editor.Element(), Size: layout.Flex(1)}},
		Element: scene.ElementConfig{Measure: layout.MeasureRows(c.HeightForWidth), Paint: func(frame scene.Frame) {
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

func (c *Editor) Controller() *base.Editor { return &c.editor }

func (c *Editor) Element() scene.Element { return c.group.Element() }

func (c *Editor) HeightForWidth(width int) int {
	rows := c.editor.HeightForWidth(layout.Remaining(width, c.markerWidth()))
	return layout.Sum(rows, c.help().HeightForWidth(width))
}
func (c *Editor) markerWidth() int { return text.Width(c.prompt) }
func (c *Editor) help() Help {
	return Help{Theme: c.appearance.palette(), Keys: KeysFunc(c.editor.KeysFor), Show: c.hints}
}

func (c *Editor) Glyphs() Glyphs { return c.glyphs.clone() }

func (c *Editor) SetGlyphs(value Glyphs) { setting.Replace(c.Element(), &c.glyphs, value.clone()) }

func (c *Editor) Prompt() string { return c.prompt }

// SetPrompt panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (c *Editor) SetPrompt(value string) {
	setting.Assign(c.Element(), &c.prompt, strings.Clone(value))
}

func (c *Editor) Hints() []keymap.Action { return slices.Clone(c.hints) }

// SetHints panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (c *Editor) SetHints(value []keymap.Action) {
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
