// Command hello uses one scene owner for input and painting so even the smallest
// program has an explicit terminal lifecycle.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/Tangerg/oolong/examples/internal/drawn"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	app, err := program.NewScreen(program.ScreenConfig{Config: program.Config{}, Build: func(runtime *program.Runtime) (scene.Element, error) {
		return newHello(runtime).Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}
}

type hello struct {
	element scene.Element
	runtime *program.Runtime
	keys    keymap.Map
	pressed int
}

const quitHello keymap.Action = "quit"

func newHello(runtime *program.Runtime) *hello {
	h := &hello{runtime: runtime}
	h.element = drawn.Element("hello", h)
	h.keys.Bind(quitHello, input.Chord{Rune: 'q'})
	h.keys.Bind(quitHello, input.Ctrl.Rune('c'))
	return h
}

func (h *hello) Draw(view grid.View) {
	view.Text(0, 0, "Oolong", grid.Style{Attr: grid.Bold})
	x := view.Text(0, 1, strconv.Itoa(h.pressed)+" keys | ", grid.Style{})
	width, _ := view.Size()
	kit.Help{Keys: &h.keys, Show: []keymap.Action{quitHello}}.Draw(view.Sub(grid.Area(x, 1, width-x, 1)))
}

func (h *hello) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	if action, bound := h.keys.ActionFor(key); bound && action == quitHello {
		h.runtime.Quit()
		return true
	}
	h.pressed++
	h.element.Invalidate()
	return true
}

func (h *hello) Element() scene.Element { return h.element }
