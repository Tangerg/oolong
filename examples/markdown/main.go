// Command markdown prepares an immutable document so drawing and measurement
// consume one complete source snapshot.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Tangerg/oolong/examples/internal/drawn"
	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/extensions/markdown"
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
	keys      keymap.Map
}

const quitMarkdown keymap.Action = "quit"

func newMarkdown(runtime *program.Runtime) *markdownScreen {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	screen := &markdownScreen{runtime: runtime, theme: theme}
	screen.element = drawn.Element("markdown", screen)
	screen.keys.Bind(quitMarkdown, input.Chord{Rune: 'q'})
	screen.keys.Bind(quitMarkdown, input.Ctrl.Rune('c'))
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
	kit.Help{Theme: s.theme, Keys: &s.keys, Show: []keymap.Action{quitMarkdown}}.Draw(rows[2])
}

func (s *markdownScreen) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok {
		return false
	}
	if action, bound := s.keys.ActionFor(key); !bound || action != quitMarkdown {
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

func (s *markdownScreen) Element() scene.Element { return s.element }
