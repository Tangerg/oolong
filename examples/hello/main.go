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

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
)

func main() {
	err := program.Run(context.Background(), program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return &hello{runtime: runtime}
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}
}

type hello struct {
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
	return true
}
