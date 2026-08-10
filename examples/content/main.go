// Command content composes Markdown, syntax highlighting, and LaTeX.
//
// Markdown owns the document syntax and streaming-capable block model. Highlight
// and LaTeX remain independent peer renderers, connected only at the application by
// the semantic renderer registry and their shared core text values.
//
// Press q or Ctrl+C to leave.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/examples/internal/latexlook"
	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/latex"
	"github.com/Tangerg/oolong/markdown"
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Root:     func(runtime *program.Runtime) program.Component { return newContent(runtime) },
		Terminal: term.Options{Probe: true},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "content:", err)
		os.Exit(1)
	}
}

type contentScreen struct {
	runtime *program.Runtime
	theme   kit.Theme
	doc     markdown.Doc
}

func newContent(runtime *program.Runtime) *contentScreen {
	theme := kit.Suited(runtime.Environment().Ground())
	look := markdownlook.New(theme, kit.GlyphsFor(runtime.Environment().Locale()))
	look.SetRenderer(markdown.FencedCode, highlight.Of("github-dark"))
	look.SetRenderer(markdown.DisplayMath, latex.Of(latexlook.New(
		theme, runtime.Environment().Locale(),
	)))

	screen := &contentScreen{runtime: runtime, theme: theme}
	screen.doc.SetBlocks(markdown.Render(contentSource, look))
	return screen
}

func (s *contentScreen) Draw(view grid.View) {
	rows := view.Subs(layout.Down.Rects(view.Bounds().Size(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(1)},
	))
	kit.Label{Text: "Composed content", Style: s.theme.Heading}.Draw(rows[0])
	s.doc.Draw(rows[1])
	kit.Label{Text: "q quits · Markdown + Highlight + LaTeX", Style: s.theme.Subtle}.Draw(rows[2])
}

func (s *contentScreen) Handle(event input.Event) bool {
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

const contentSource = `# Three peers, one document

Markdown recognizes the semantic blocks. The application chooses their renderers.

~~~go
look.SetRenderer(markdown.FencedCode, highlight.Of("github-dark"))
~~~

$$
x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}
$$

The modules meet through styled text, not through one another's parser trees.`
