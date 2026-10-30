// Command content composes Markdown, syntax highlighting, and LaTeX.
//
// Markdown owns the document syntax and streaming-capable block model. Highlight
// and LaTeX remain independent peer renderers, connected only at the application by
// the semantic renderer registry and their shared core drawable contract.
//
// Press q or Ctrl+C to leave.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Tangerg/oolong/examples/internal/latexlook"
	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/extensions/latex"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true}, Build: func(runtime *program.Runtime) (scene.Element, error) { return newContent(runtime).Element(), nil }})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "content:", err)
		os.Exit(1)
	}
}

type contentScreen struct {
	element   scene.Element
	runtime   *program.Runtime
	theme     kit.Theme
	body      grid.Drawable
	registry  *content.Registry
	renderErr error
}

func newContent(runtime *program.Runtime) *contentScreen {
	theme := kit.Suited(runtime.Environment().Ground())
	look := markdownlook.New(theme, kit.GlyphsFor(runtime.Environment().Locale()))
	highlighter := highlight.New("github-dark")
	formulaLook := latexlook.New(theme, runtime.Environment().Locale())
	screen := &contentScreen{runtime: runtime, theme: theme}
	registry, err := content.New(content.Config{Bindings: []content.Binding{
		{Format: "go", Render: func(_ context.Context, source string) (grid.Drawable, error) {
			return text.NewBlock(text.BlockConfig{Lines: highlighter.Lines("go", source), Wrap: true}), nil
		}},
		{Format: "latex", Render: func(_ context.Context, source string) (grid.Drawable, error) {
			formula := latex.Render(source, formulaLook)
			return formula, formula.Err()
		}},
		{Format: "markdown", Render: func(_ context.Context, source string) (grid.Drawable, error) {
			blocks, err := markdown.Render(source, look)
			doc := new(markdown.Doc)
			doc.SetBlocks(blocks)
			return doc, err
		}},
	}})
	if err != nil {
		screen.renderErr = err
		return screen
	}
	screen.registry = registry
	look.SetRenderer(markdown.FencedCode, func(info, source string) (grid.Drawable, error) {
		if info != "go" {
			return nil, markdown.ErrUnhandled
		}
		return registry.Render(context.Background(), content.Format(info), source)
	})
	look.SetRenderer(markdown.DisplayMath, func(_ string, source string) (grid.Drawable, error) {
		return registry.Render(context.Background(), "latex", source)
	})
	screen.show("markdown", contentSource)
	return screen
}

func (s *contentScreen) show(format content.Format, source string) {
	s.body, s.renderErr = s.registry.Render(context.Background(), format, source)
	s.element.Invalidate()
}

func (s *contentScreen) Draw(view grid.View) {
	rows := view.Subs((layout.Flow{Axis: layout.Down}).Rects(view.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(1)},
	}))
	kit.Label{Text: "Composed content", Style: s.theme.Heading}.Draw(rows[0])
	if s.body != nil {
		s.body.Draw(rows[1])
	}
	if s.renderErr != nil {
		kit.Label{Text: s.renderErr.Error(), Style: s.theme.Danger}.Draw(rows[2])
		return
	}
	kit.Label{Text: "1 Markdown · 2 Go · 3 LaTeX · q quits", Style: s.theme.Subtle}.Draw(rows[2])
}

func (s *contentScreen) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	if s.registry != nil {
		switch key.Rune {
		case '1':
			s.show("markdown", contentSource)
			return true
		case '2':
			s.show("go", "func main() { println(\"Oolong\") }")
			return true
		case '3':
			s.show("latex", `x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}`)
			return true
		}
	}
	if key.Rune != 'q' && (key.Rune != 'c' || !key.Mods.Has(input.Ctrl)) {
		return false
	}
	s.runtime.Quit()
	return true
}

const contentSource = `# Three peers, one document

Markdown recognizes the semantic blocks. The application chooses their renderers.

~~~go
highlighter := highlight.New("github-dark")
// Applications explicitly bind format names to passive content.
~~~

$$
x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}
$$

The modules share a drawable contract. Each owns its layout and optional text projection.`

func (s *contentScreen) Element() scene.Element {
	if s.element.IsZero() {
		s.element = scene.NewElement(scene.ElementConfig{
			Key:       "content",
			Focusable: func() bool { return true },
			Paint:     func(frame scene.Frame) { s.Draw(frame.View) },
			Handle: func(event scene.Event) scene.EventResult {
				return scene.EventResult{Consumed: s.Handle(event.Input)}
			},
		})
	}
	return s.element
}
