// Command markdown renders a finished Markdown document.
//
// The parser produces immutable blocks. A Doc composes those blocks, measures them
// at the final terminal width, and draws them directly; no component-specific
// Markdown widget or intermediate string is needed.
//
// Press q or Ctrl+C to leave.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true}, Build: func(runtime *program.Runtime) (scene.Element, error) { return newMarkdown(runtime).Element(), nil }})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "markdown:", err)
		os.Exit(1)
	}
}

type markdownScreen struct {
	element   scene.Element
	runtime   *program.Runtime
	theme     kit.Theme
	doc       markdown.Doc
	renderErr error
}

func newMarkdown(runtime *program.Runtime) *markdownScreen {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	screen := &markdownScreen{runtime: runtime, theme: theme}
	blocks, err := markdown.Render(markdownSource, markdownlook.New(theme, glyphs))
	screen.renderErr = err
	screen.doc.SetBlocks(blocks)
	return screen
}

func (s *markdownScreen) Draw(view grid.View) {
	rows := view.Subs((layout.Flow{Axis: layout.Down}).Rects(view.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(1)},
	}))
	kit.Label{Text: "Markdown", Style: s.theme.Heading}.Draw(rows[0])
	s.doc.Draw(rows[1])
	if s.renderErr != nil {
		kit.Label{Text: s.renderErr.Error(), Style: s.theme.Danger}.Draw(rows[2])
		return
	}
	kit.Label{Text: "q quits · Render → Blocks → Doc", Style: s.theme.Subtle}.Draw(rows[2])
}

func (s *markdownScreen) Handle(event input.Event) bool {
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

const markdownSource = `# Structured text stays structured

A finished document becomes **immutable blocks** that still know how to lay
themselves out at the width they finally receive.

- paragraphs wrap at the drawing boundary
- lists retain their marker and spacing
- links keep [their destination](https://example.com)

> Parsing, measuring, drawing, and selection share one document model.

| Value | Owner |
| --- | --- |
| syntax | Markdown |
| cells | final grid |`

func (s *markdownScreen) Element() scene.Element {
	if s.element.IsZero() {
		s.element = scene.NewElement(scene.ElementConfig{
			Key:       "markdown",
			Focusable: func() bool { return true },
			Paint:     func(frame scene.Frame) { s.Draw(frame.View) },
			Handle: func(event scene.Event) scene.EventResult {
				return scene.EventResult{Consumed: s.Handle(event.Input)}
			},
		})
	}
	return s.element
}
