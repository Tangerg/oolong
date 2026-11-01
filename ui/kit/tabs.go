package kit

import (
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// TabsConfig supplies initial panes and strip appearance. Scope supplies a live theme and Appearance supplies instance overrides.
type TabsConfig struct {
	Scope          *ThemeScope
	Appearance     ThemeOverride
	Theme          Theme
	Glyphs         Glyphs
	Items          []base.Tab
	Initial        int
	Keys           *keymap.Map
	NoWrap, NoRule bool
}

// Tabs dresses one base.Tabs controller with a measured strip and optional rule. Its controller alone owns selected-pane state.
type Tabs struct {
	appearance appearance
	rule       string
	controller *base.Tabs
}

// NewTabs constructs the controller and strip appearance. It panics for duplicate keys, an invalid selected child, or competing Theme and Scope sources.
func NewTabs(config TabsConfig) *Tabs {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	t := &Tabs{appearance: look}
	if !config.NoRule {
		t.rule = strings.Clone(config.Glyphs.Horizontal)
	}
	t.controller = base.NewTabs(base.TabsConfig{
		Items: config.Items, Initial: config.Initial, Keys: config.Keys, NoWrap: config.NoWrap, Gap: 2, StripRows: func() int {
			if t.rule != "" {
				return 2
			}
			return 1
		},
		PaintTab: func(frame scene.Frame, tab base.Tab, selected bool) {
			style := t.appearance.palette().Muted
			if selected {
				style = t.appearance.palette().Accent
			}
			frame.Text(0, 0, tab.Title, style)
		},
	})
	if err := t.controller.Strip().SetPaint(func(frame scene.Frame) {
		if t.rule != "" {
			width, height := frame.Size()
			if height > 1 {
				for x := range width {
					frame.Text(x, 1, t.rule, t.appearance.palette().Border)
				}
			}
		}
	}); err != nil {
		panic(err)
	}
	t.appearance.bind(t.Element())
	return t
}

// Rule returns the horizontal strip decoration. An empty glyph reserves no row.
func (t *Tabs) Rule() string { return t.rule }

// SetRule changes both the visible decoration and the space it occupies.
// It panics during scene presentation or observation.
func (t *Tabs) SetRule(glyph string) {
	if err := t.Element().CheckMutation(); err != nil {
		panic(err)
	}
	if t.rule != glyph {
		t.rule = strings.Clone(glyph)
		t.Element().Invalidate()
	}
}

// Controller returns the one pane-selection owner.
func (t *Tabs) Controller() *base.Tabs { return t.controller }

// Element is the controller's exact strip and panel composition.
func (t *Tabs) Element() scene.Element {
	if t == nil || t.controller == nil {
		return scene.Element{}
	}
	return t.controller.Element()
}

// HeightForWidth measures the controller's strip and current pane.
func (t *Tabs) HeightForWidth(width int) int {
	if t == nil || t.controller == nil {
		return 0
	}
	return t.controller.HeightForWidth(width)
}
