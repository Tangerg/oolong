// Command hello is the smallest interface there is: it draws, it answers a key, and
// it stops.
//
// Everything in the library is optional except these three things. A program says
// what to run and where; a component draws itself into the space it is given and
// says whether it wants an event; the runtime does the rest — one goroutine, a frame
// when something changed, and nothing at all when nothing did.
//
// Press any key. Press q or Ctrl+C to leave.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		// Root takes a screen of its own and gives it back on the way out. The other
		// mode, Inline, draws a block in the terminal's own screen — see the streaming
		// example, which is what that is for.
		Root: func(runtime *program.Runtime) program.Component { return &hello{runtime: runtime} },
		// One round trip at startup, and the only way to learn what colour the
		// terminal draws on. A theme that has to be told is wrong for half the people
		// who run it.
		Terminal: term.Options{Probe: true},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}
}

// hello counts what it has been given.
type hello struct {
	runtime *program.Runtime
	keys    int
	theme   kit.Theme
	set     bool
}

// Draw puts a box in the middle of the screen with a line in it.
//
// The view it is handed is already the whole screen and already clipped, so the
// coordinates here are its own: this cannot draw outside its box, and nothing had to
// be told where the box is.
func (h *hello) Draw(v grid.View) {
	if !h.set {
		// The look follows what the terminal said about itself, which is known by the
		// time anything is drawn.
		h.theme, h.set = kit.Suited(h.runtime.Environment().Ground()), true
	}
	width, height := v.Size()
	box := kit.Box{
		Theme:   h.theme,
		Glyphs:  kit.GlyphsFor(os.LookupEnv),
		Title:   "hello",
		Padding: layout.Symmetric(0, 1),
	}
	// Half the width, three rows, in the middle: a slot says how much of an axis it
	// wants and where it sits across the other one.
	rows := v.Subs(layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{
			Size:  layout.Fixed(min(5, height)),
			Cross: layout.Cross{Size: min(40, width), Align: layout.Center},
		},
		layout.Slot{Size: layout.Flex(1)},
	))
	inner := box.Draw(rows[1])

	said := "press a key"
	if h.keys > 0 {
		said = strconv.Itoa(h.keys) + " keys so far"
	}
	kit.Label{Text: said, Style: h.theme.Text, Align: layout.Center}.Draw(inner)
	kit.Label{
		Text:  "q to leave",
		Style: h.theme.Muted,
		Align: layout.Center,
	}.Draw(inner.Sub(grid.Rect(0, 2, 40, 1)))
}

// Handle says whether it wanted the event. What it does not want is dropped: a
// component is the root of its own tree and there is nobody above it to pass one on
// to.
func (h *hello) Handle(ev input.Event) bool {
	key, ok := ev.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	if key.Rune == 'q' || (key.Rune == 'c' && key.Mods.Has(input.Ctrl)) {
		h.runtime.Quit()
		return true
	}
	h.keys++
	return true
}
