// Command read shows an answer as it is written.
//
// It is the deepest of these and the one the library is for. Markdown arrives a few
// words at a time — from a model, over a network, from anything that produces text
// before it has finished thinking — and two things have to be true at once: what is
// certainly finished should stop being redrawn, and what is still being written
// should look like what it says so far.
//
// So a stream splits it. Every block it hands back is printed into the terminal's
// own scrollback, where it stays after this program exits, and is never parsed or
// drawn again however long the answer becomes. What is left is short by construction
// and is re-rendered on every chunk.
//
// Nothing about that is markdown's alone. The same shape — publish what is finished,
// redraw only what is not — is what any streaming interface built on this does.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/examples/internal/latexlook"
	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/latex"
	"github.com/Tangerg/oolong/markdown"
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Inline:   func(runtime *program.InlineRuntime) program.Component { return newReader(runtime) },
		Terminal: term.Features{Probe: true},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
}

// reader is the live part: the block still being written, and a line saying what is
// happening.
type reader struct {
	runtime *program.InlineRuntime
	theme   kit.Theme

	stream  markdown.Stream
	open    markdown.Doc
	pieces  []string
	at      int
	stop    func()
	spinner kit.Spinner
}

// How fast the answer arrives. A model's is decided by a model; these two are what a
// demonstration has instead, and the test turns them up so it does not have to wait.
const (
	piece = 7
	pace  = 60 * time.Millisecond
)

func newReader(runtime *program.InlineRuntime) *reader { return read(runtime, piece, pace) }

func read(runtime *program.InlineRuntime, size int, every time.Duration) *reader {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())

	r := &reader{
		runtime: runtime,
		theme:   theme,
		pieces:  pieces(answer(), size),
		spinner: kit.Spinner{Theme: theme, Glyphs: glyphs, Label: "writing"},
	}
	// Appearance is application composition: the three peer modules still share no
	// import edge and expose only their core text boundary to one another.
	look := markdownlook.New(theme, glyphs)
	formulaLook := latexlook.New(theme, runtime.Environment().Locale())
	look.SetRenderer(markdown.DisplayMath, func(_ string, source string) []text.Line {
		return latex.Render(source, formulaLook).Lines()
	})
	highlighter := highlight.New("github-dark")
	look.SetRenderer(markdown.FencedCode, highlighter.Lines)
	r.stream.SetLook(look)

	runtime.Session().SetTitle("reading")
	r.stop = runtime.Every(every, r.advance)
	return r
}

// advance takes the next piece of the answer.
func (r *reader) advance() {
	r.spinner.Tick()
	if r.at >= len(r.pieces) {
		r.finish()
		return
	}
	// Everything the stream calls finished is printed, once, and is the terminal's
	// from then on. A document is a Drawer and a Measurer and nothing else, which is
	// what lets the runtime print one without either of them knowing about the other.
	if blocks := r.stream.Feed(r.pieces[r.at]); len(blocks) > 0 {
		doc := new(markdown.Doc)
		doc.SetBlocks(blocks)
		r.runtime.Print(doc)
	}
	r.at++
	r.open.SetBlocks(r.stream.Open())
}

func (r *reader) finish() {
	if blocks := r.stream.Flush(); len(blocks) > 0 {
		doc := new(markdown.Doc)
		doc.SetBlocks(blocks)
		r.runtime.Print(doc)
	}
	r.open.SetBlocks(nil)
	r.stop()
	r.runtime.Session().SetTitle("")
	r.runtime.Session().Notify("the answer is finished")
}

// Draw is what is still being written, and a row under it.
func (r *reader) Draw(v grid.View) {
	rows := v.Subs((layout.Flow{Axis: layout.Down}).Rects(v.Bounds().Size(), []layout.Slot{
		{Size: layout.Measured(0, 0), Of: layout.MeasureFunc(r.open.Measure)},
		{Size: layout.Fixed(1)},
	}))
	r.open.Draw(rows[0])
	if r.at >= len(r.pieces) {
		kit.Label{
			Text:  "that is the whole answer — ctrl+c to leave",
			Style: r.theme.Muted,
		}.Draw(rows[1])
		return
	}
	r.spinner.Draw(rows[1])
}

func (r *reader) Handle(ev input.Event) bool {
	key, ok := ev.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	if key.Rune == 'c' && key.Mods.Has(input.Ctrl) {
		r.runtime.Quit()
		return true
	}
	return false
}

// pieces cuts the answer into the chunks it arrives in, which is what makes this a
// demonstration rather than a rendering: the boundaries fall wherever they like,
// including inside a word and inside a fence.
func pieces(whole string, size int) []string {
	var out []string
	for i := 0; i < len(whole); i += size {
		out = append(out, whole[i:min(i+size, len(whole))])
	}
	return out
}

func answer() string {
	return strings.Join([]string{
		"# What is happening here",
		"",
		"This answer is arriving a few characters at a time. Everything above the",
		"line at the bottom has been **published**: it was parsed once, printed into",
		"your terminal's own scrollback, and will never be looked at again.",
		"",
		"- A list is one block, so it is published whole.",
		"- Its items are not published one at a time.",
		"- Which is what stops a list from being rewritten as it grows.",
		"",
		"> A quotation keeps its bar down every row, including the rows a wrap made.",
		"",
		"```go",
		"func main() {",
		"\tfmt.Println(\"a fenced block is never cut in half\")",
		"}",
		"```",
		"",
		"A semantic extension can keep mathematical structure two-dimensional too:",
		"",
		"$$",
		`x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}`,
		"$$",
		"",
		"The last paragraph is the one still being written, which is why it is drawn",
		"rather than printed. When it ends, it is printed too — and then this program",
		"owns nothing on your screen at all.",
		"",
	}, "\n")
}
