// Command hello is the smallest complete Oolong program.
//
// It draws text, handles one kind of event, and stops. The other examples add
// layout, reusable components, appearance, streaming output, and background work
// one layer at a time.
//
// Press any key to increment the count. Press q or Ctrl+C to leave.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func main() {
	app, err := program.NewScreen(program.ScreenConfig{Config: program.Config{}, Build: func(runtime *program.Runtime) (scene.Element, error) {
		return (&hello{runtime: runtime}).Element(), nil
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
	keys    int
}

func (h *hello) Draw(view grid.View) {
	view.Text(0, 0, "Oolong", grid.Style{Attr: grid.Bold})
	view.Text(0, 1, strconv.Itoa(h.keys)+" keys | q quits", grid.Style{})
}

func (h *hello) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	if key.Rune == 'q' || key.Rune == 'c' && key.Mods.Has(input.Ctrl) {
		h.runtime.Quit()
		return true
	}
	h.keys++
	h.element.Invalidate()
	return true
}

func (h *hello) Element() scene.Element {
	if h.element.IsZero() {
		h.element = scene.NewElement(scene.ElementConfig{
			Key:       "hello",
			Focusable: func() bool { return true },
			Paint:     func(frame scene.Frame) { h.Draw(frame.View) },
			Handle: func(event scene.Event) scene.EventResult {
				return scene.EventResult{Consumed: h.Handle(event.Input)}
			},
		})
	}
	return h.element
}
