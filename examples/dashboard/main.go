// Command dashboard keeps task data with the application while scene children own
// input and layout, so multiple presentations share one task model.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true, Mouse: true}, Build: func(runtime *program.Runtime) (scene.Element, error) {
		d, err := newDashboard(runtime)
		if err != nil {
			return scene.Element{}, err
		}
		return d.Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dashboard:", err)
		os.Exit(1)
	}
}

type dashboard struct {
	runtime *program.Runtime
	theme   kit.Theme

	element scene.Element
	timer   *program.Timer
	scope   *program.Scope
	tabs    *base.Tabs
	strip   *kit.Tabs
	work    *queue
	watch   *activity
	prefs   *kit.Settings[preference]
	motion  bool
}

type preference string

const (
	ratePreference    preference    = "rate"
	motionPreference  preference    = "motion"
	quitDashboard     keymap.Action = "quit"
	firstPaneShortcut               = '1'
	lastPaneShortcut                = '9'
)

func newDashboard(runtime *program.Runtime) (*dashboard, error) {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())

	d := &dashboard{runtime: runtime, theme: theme, motion: true}
	keys := base.DefaultTabsKeys()
	keys.Bind(quitDashboard, input.Chord{Rune: 'q'})
	d.work = newQueue(theme, glyphs)
	d.watch = newActivity(theme, glyphs, d.work)
	d.prefs = kit.NewSettings(kit.SettingsConfig[preference]{
		Theme: theme, Items: []preference{ratePreference, motionPreference},
		Label: func(item preference) string { return string(item) },
		Value: d.preferenceValue, Change: d.changePreference,
	})

	d.strip = kit.NewTabs(kit.TabsConfig{
		Theme: theme, Glyphs: glyphs, Keys: keys,
		Items: []base.Tab{
			{Title: "tasks", Content: d.work.Element()},
			{Title: "activity", Content: d.watch.Element()},
			{Title: "settings", Content: d.prefs.Element()},
		},
	})
	d.tabs = d.strip.Controller()
	footer := scene.NewElement(scene.ElementConfig{Paint: d.drawHelp})
	d.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{Key: "dashboard", Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: d.Handle(event.Input())} }}, Items: []scene.Item{
		{Element: d.strip.Element(), Size: layout.Flex(1)}, {Element: footer, Size: layout.Fixed(1)},
	}}).Element()
	var err error
	d.scope, err = runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		return nil, err
	}
	d.timer, err = runtime.Every(d.scope, dashboardTickInterval, d.advance)
	if err != nil {
		d.scope.Close()
		return nil, err
	}
	return d, nil
}

func (d *dashboard) Element() scene.Element { return d.element }

func (d *dashboard) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() {
		if action, ok := d.tabs.Keys().ActionFor(key); ok && action == quitDashboard {
			d.runtime.Quit()
			return true
		}
		first, last := d.paneShortcutRange()
		if first == 0 {
			return false
		}
		for chord := range key.ShortcutChords() {
			if chord.Code != input.Character || chord.Mods != 0 || chord.Rune < first || chord.Rune > last {
				continue
			}
			d.tabs.Select(int(chord.Rune - first))
			if tab, ok := d.tabs.Current(); ok {
				_ = tab.Content.RequestFocusFirst()
			}
			return true
		}
	}
	return false
}

func (d *dashboard) paneShortcutRange() (first, last rune) {
	count := d.tabs.Len()
	if count <= 0 {
		return 0, 0
	}
	if count >= lastPaneShortcut-firstPaneShortcut+1 {
		return firstPaneShortcut, lastPaneShortcut
	}
	return firstPaneShortcut, firstPaneShortcut + rune(count) - 1
}

func (d *dashboard) drawHelp(frame scene.Frame) {
	width, height := frame.Size()
	var prefix string
	first, last := d.paneShortcutRange()
	if first != 0 && first == last {
		prefix = fmt.Sprintf("%c pane  ", first)
	} else if first != 0 {
		prefix = fmt.Sprintf("%c–%c pane  ", first, last)
	}
	x := frame.Text(0, 0, prefix, d.theme.Subtle)
	kit.Help{Theme: d.theme, Keys: d.tabs.Keys(), Show: []keymap.Action{base.PrevTab, base.NextTab, quitDashboard}}.
		Draw(frame.View.Sub(grid.Area(x, 0, width-x, height)))
}

func (d *dashboard) advance() {
	if d.work.advance(d.watch.rate.Controller().Value()) {
		d.watch.record()
	}
	completed, total := d.work.completionTotals()
	if completed >= total {
		d.timer.Stop()
		d.timer = nil
		return
	}
	if d.motion {
		d.watch.tick()
	}
}

func (d *dashboard) preferenceValue(item preference) string {
	switch item {
	case ratePreference:
		return formatRate(d.watch.rate.Controller().Value())
	case motionPreference:
		if d.motion {
			return "on"
		}
		return "off"
	default:
		return ""
	}
}

func (d *dashboard) changePreference(_ context.Context, _ int, item preference, action keymap.Action) (scene.ActionResult[bool], error) {
	result := scene.ActionResult[bool]{}
	switch item {
	case ratePreference:
		rate := d.watch.rate.Controller()
		previous := rate.Value()
		result.Value = rate.Do(action)
		if rate.Value() != previous {
			result.State = scene.Applied
		}
	case motionPreference:
		previous := d.motion
		switch action {
		case base.Decrease:
			d.motion = false
		case base.Increase:
			d.motion = true
		case base.Activate:
			d.motion = !d.motion
		default:
			return scene.ActionResult[bool]{}, nil
		}
		result.Value = true
		if d.motion != previous {
			result.State = scene.Applied
		}
	}
	return result, nil
}
