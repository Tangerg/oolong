package kit

import (
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
	Glyphs     Glyphs
	Rule       bool
	controller *base.Tabs
}

// NewTabs constructs the controller and strip appearance. It panics for duplicate keys, an invalid selected child, or competing Theme and Scope sources.
func NewTabs(config TabsConfig) *Tabs {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	t := &Tabs{appearance: look, Glyphs: config.Glyphs, Rule: !config.NoRule}
	rows := 1
	if t.Rule && t.Glyphs.Horizontal != "" {
		rows = 2
	}
	t.controller = base.NewTabs(base.TabsConfig{
		Items: config.Items, Initial: config.Initial, Keys: config.Keys, NoWrap: config.NoWrap, Gap: 2, StripRows: rows,
		PaintTab: func(frame scene.Frame, tab base.Tab, selected bool) {
			style := t.appearance.palette().Muted
			if selected {
				style = t.appearance.palette().Accent
			}
			frame.Text(0, 0, tab.Title, style)
		},
	})
	if err := t.controller.Strip().SetPaint(func(frame scene.Frame) {
		if t.Rule && t.Glyphs.Horizontal != "" {
			width, height := frame.Size()
			if height > 1 {
				for x := range width {
					frame.Text(x, 1, t.Glyphs.Horizontal, t.appearance.palette().Border)
				}
			}
		}
	}); err != nil {
		panic(err)
	}
	t.appearance.bind(t.Element())
	return t
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
