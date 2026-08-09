// Command keys demonstrates an exact binding that is also the prefix of a longer
// one. Press g once to move one row after a short pause, gg to jump to the first row,
// and q or Ctrl+C to leave.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

const (
	next keymap.Action = "next"
	top  keymap.Action = "top"
	quit keymap.Action = "quit"
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Root:     func(runtime *program.Runtime) program.Component { return newKeys(runtime) },
		Terminal: term.Options{Probe: true},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "keys:", err)
		os.Exit(1)
	}
}

type keys struct {
	runtime *program.Runtime
	mapKeys *keymap.Map
	matcher keymap.Matcher
	theme   kit.Theme
	row     int
	last    string
}

func newKeys(runtime *program.Runtime) *keys {
	bindings := &keymap.Map{Timeout: 400 * time.Millisecond, Resolve: runtime.After}
	bindings.Bind(next, input.Chord{Rune: 'g'})
	bindings.Bind(top, input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	bindings.Bind(quit, input.Chord{Rune: 'q'})
	bindings.Bind(quit, input.Ctrl.Rune('c'))
	return &keys{
		runtime: runtime,
		mapKeys: bindings,
		theme:   kit.Suited(runtime.Environment().Ground()),
		last:    "waiting for g or gg",
	}
}

func (k *keys) Draw(view grid.View) {
	width, height := view.Size()
	box := kit.Box{
		Theme: k.theme, Glyphs: kit.GlyphsFor(os.Getenv), Title: "key sequences",
		Padding: layout.Symmetric(0, 1),
	}
	area := grid.Rect(max((width-54)/2, 0), max((height-7)/2, 0), min(width, 54), min(height, 7))
	inside := box.Draw(view.Sub(area))
	kit.Label{Text: "g: next after 400ms   gg: top   q: quit", Style: k.theme.Text}.Draw(inside)
	kit.Label{Text: fmt.Sprintf("row %d — %s", k.row, k.last), Style: k.theme.Accent}.
		Draw(inside.Sub(grid.Rect(0, 2, inside.Bounds().Dx(), 1)))
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
	return true
}
