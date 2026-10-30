// Command keys demonstrates an exact binding that is also the prefix of a longer
// one. Press g once to move one row after a short pause, gg to jump to the first row,
// and q or Ctrl+C to leave.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	next keymap.Action = "next"
	top  keymap.Action = "top"
	quit keymap.Action = "quit"
)

func main() {
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true}, Build: newKeys})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "keys:", err)
		os.Exit(1)
	}
}

type keys struct {
	element scene.Element
	runtime *program.Runtime
	mapKeys *keymap.Map
	matcher keymap.Matcher
	theme   kit.Theme
	glyphs  kit.Glyphs
	row     int
	last    string
}

func newKeys(runtime *program.Runtime) (scene.Element, error) {
	scope, err := runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		return scene.Element{}, err
	}
	k := &keys{runtime: runtime}
	bindings := &keymap.Map{Timeout: 400 * time.Millisecond, Resolve: func(delay time.Duration, resolve func()) func() {
		timer, err := runtime.After(scope, delay, resolve)
		if err != nil {
			k.last = err.Error()
			return func() {}
		}
		return timer.Stop
	}}
	bindings.Bind(next, input.Chord{Rune: 'g'})
	bindings.Bind(top, input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	bindings.Bind(quit, input.Chord{Rune: 'q'})
	bindings.Bind(quit, input.Ctrl.Rune('c'))
	*k = keys{
		runtime: runtime,
		mapKeys: bindings,
		theme:   kit.Suited(runtime.Environment().Ground()),
		glyphs:  kit.GlyphsFor(runtime.Environment().Locale()),
		last:    "waiting for g or gg",
	}
	return k.Element(), nil
}

func (k *keys) Draw(view grid.View) {
	width, height := view.Size()
	box := kit.Box{
		Theme: k.theme, Glyphs: k.glyphs, Title: "key sequences",
		Padding: layout.Symmetric(0, 1),
	}
	area := grid.Area(max((width-54)/2, 0), max((height-7)/2, 0), min(width, 54), min(height, 7))
	inside := box.Draw(view.Sub(area))
	kit.Label{Text: "g: next after 400ms   gg: top   q: quit", Style: k.theme.Text}.Draw(inside)
	kit.Label{Text: fmt.Sprintf("row %d — %s", k.row, k.last), Style: k.theme.Accent}.
		Draw(inside.Sub(grid.Area(0, 2, inside.Bounds().Dx(), 1)))
}

func (k *keys) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok {
		return false
	}
	_, handled := k.matcher.Handle(k.mapKeys, key, k.Do)
	return handled
}

func (k *keys) Do(action keymap.Action) bool {
	switch action {
	case next:
		k.row++
		k.last = "resolved g"
	case top:
		k.row = 0
		k.last = "resolved gg"
	case quit:
		k.runtime.Quit()
	default:
		return false
	}
	k.element.Invalidate()
	return true
}

func (k *keys) Element() scene.Element {
	if k.element.IsZero() {
		k.element = scene.NewElement(scene.ElementConfig{
			Key:       "keys",
			Focusable: func() bool { return true },
			Paint:     func(frame scene.Frame) { k.Draw(frame.View) },
			Handle: func(event scene.Event) scene.EventResult {
				return scene.EventResult{Consumed: k.Handle(event.Input)}
			},
		})
	}
	return k.element
}
