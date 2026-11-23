package kit

import (
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

// TabsConfig carries the base tabs configuration in Behavior and the strip's
// appearance. The skin paints tabs and sizes the strip, so Behavior.PaintTab and
// Behavior.StripRows must be unset; a zero Behavior.Gap uses the skin's spacing.
// Scope supplies a live theme and Appearance supplies instance overrides.
type TabsConfig struct {
	Behavior   base.TabsConfig
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	NoRule     bool
}

// Tabs dresses one base.Tabs controller with a measured strip and optional rule. Its controller alone owns selected-pane state.
// Tabs must not be copied after first use: its controller retains this appearance.
type Tabs struct {
	noCopy     noCopy
	appearance appearance
	rule       string
	controller *base.Tabs
}

const defaultTabGap = 2

// NewTabs constructs the controller and strip appearance. It panics for duplicate
// keys, an invalid selected child, skin-owned Behavior fields, or competing Theme and
// Scope sources.
func NewTabs(config TabsConfig) *Tabs {
	look := newAppearance(config.Theme, config.Scope, config.Appearance)
	requireSkinOwned("tabs Behavior.PaintTab", config.Behavior.PaintTab == nil)
	requireSkinOwned("tabs Behavior.StripRows", config.Behavior.StripRows == nil)
	t := &Tabs{appearance: look}
	if !config.NoRule {
		t.rule = strings.Clone(config.Glyphs.Rule)
	}
	behavior := config.Behavior
	if behavior.Gap == 0 {
		behavior.Gap = defaultTabGap
	}
	behavior.StripRows = func() int {
		if t.rule != "" {
			return 2
		}
		return 1
	}
	behavior.PaintTab = func(frame scene.Frame, tab base.Tab, selected bool) {
		style := t.appearance.palette().Muted
		if selected {
			style = t.appearance.palette().Accent
		}
		text.Of(tab.Title, style).Draw(frame.View, 0, 0)
	}
	t.controller = base.NewTabs(behavior)
	t.controller.Strip().SetPaint(func(frame scene.Frame) {
		text.Of(t.rule, t.appearance.palette().Border).Tile(frame.View, 1)
	})
	t.appearance.bind(t.Element())
	return t
}

// Rule returns the horizontal strip decoration. An empty glyph reserves no row.
func (t *Tabs) Rule() string { return t.rule }

// SetRule changes both the visible decoration and the space it occupies.
// It panics during scene presentation or observation.
func (t *Tabs) SetRule(glyph string) {
	setting.Assign(t.Element(), &t.rule, strings.Clone(glyph))
}

func (t *Tabs) Controller() *base.Tabs { return t.controller }

func (t *Tabs) Element() scene.Element {
	return t.controller.Element()
}

func (t *Tabs) HeightForWidth(width int) int {
	return t.controller.HeightForWidth(width)
}
