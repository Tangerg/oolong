// Command latex renders mathematical LaTeX without Markdown.
//
// Render returns one immutable Formula. The formula measures, draws, and exposes
// selectable rows from the same two-dimensional box layout, so it can be placed
// anywhere a passive grid drawable can be used.
//
// Press q or Ctrl+C to leave.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Tangerg/oolong/examples/internal/latexlook"
	"github.com/Tangerg/oolong/extensions/latex"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true}, Build: func(runtime *program.Runtime) (scene.Element, error) { return newLatex(runtime).Element(), nil }})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "latex:", err)
		os.Exit(1)
	}
}

type latexScreen struct {
	element scene.Element
	runtime *program.Runtime
	theme   kit.Theme
	formula *latex.Formula
}

func newLatex(runtime *program.Runtime) *latexScreen {
	theme := kit.Suited(runtime.Environment().Ground())
	look := latexlook.New(theme, runtime.Environment().Locale())
	look.Align = layout.Center
	return &latexScreen{
		runtime: runtime,
		theme:   theme,
		formula: latex.Render(latexSource, look),
	}
}

func (s *latexScreen) Draw(view grid.View) {
	rows := view.Subs((layout.Flow{Axis: layout.Down}).Rects(view.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(1)},
	}))
	kit.Label{Text: "LaTeX", Style: s.theme.Heading}.Draw(rows[0])

	width, height := rows[1].Size()
	wanted := min(s.formula.HeightForWidth(width), height)
	top := max((height-wanted)/2, 0)
	s.formula.Draw(rows[1].Sub(grid.Area(0, top, width, wanted)))

	kit.Label{Text: "q quits · latex.Render → *latex.Formula", Style: s.theme.Subtle}.Draw(rows[2])
}

func (s *latexScreen) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	if key.Rune != 'q' && (key.Rune != 'c' || !key.Mods.Has(input.Ctrl)) {
		return false
	}
	s.runtime.Quit()
	return true
}

const latexSource = `x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}`

func (s *latexScreen) Element() scene.Element {
	if s.element.IsZero() {
		s.element = scene.NewElement(scene.ElementConfig{
			Key:       "latex",
			Focusable: func() bool { return true },
			Paint:     func(frame scene.Frame) { s.Draw(frame.View) },
			Handle: func(event scene.Event) scene.EventResult {
				return scene.EventResult{Consumed: s.Handle(event.Input)}
			},
		})
	}
	return s.element
}
