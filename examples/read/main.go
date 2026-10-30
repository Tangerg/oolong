// Command read shows an answer as it is written.
//
// It is the deepest of these and the one the library is for. Markdown arrives a few
// words at a time — from a model, over a network, from anything that produces text
// before it has finished thinking — and two things have to be true at once: what is
// certainly finished should stop being redrawn, and what is still being written
// should look like what it says so far.
//
// One StreamDocument owns the complete bounded source. Markdown previews remain
// revisable until EOF, since later reference definitions can change earlier text.
// The final immutable document transfers to the publisher, which separately reports
// acceptance and successful writing into terminal scrollback.
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

	"github.com/Tangerg/oolong/examples/internal/latexlook"
	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/extensions/latex"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/base"
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
	app, err := program.NewInline(program.InlineConfig{Terminal: term.Features{Probe: true}, Build: func(runtime *program.InlineRuntime) (scene.Element, error) { return newReader(runtime).Element(), nil }})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
}

// reader is the live part: the block still being written, and a line saying what is
// happening.
type reader struct {
	runtime   *program.InlineRuntime
	theme     kit.Theme
	element   scene.Element
	document  *content.StreamDocument
	render    content.Renderer
	view      *kit.Transcript
	pieces    []string
	at        int
	timer     *program.Timer
	scope     *program.Scope
	spinner   kit.Spinner
	renderErr error
	finished  bool
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
	look.SetRenderer(markdown.DisplayMath, func(_ string, source string) (grid.Drawable, error) {
		formula := latex.Render(source, formulaLook)
		return formula, formula.Err()
	})
	highlighter := highlight.New("github-dark")
	look.SetRenderer(markdown.FencedCode, func(info, source string) (grid.Drawable, error) {
		return text.NewBlock(text.BlockConfig{Lines: highlighter.Lines(info, source), Wrap: true}), nil
	})
	r.render = markdown.NewRenderer(markdown.RendererConfig{Look: look})
	var err error
	r.document, err = content.NewStreamDocument(content.StreamConfig{Format: "markdown"})
	if err != nil {
		panic(err)
	}
	r.view, err = kit.NewTranscript(kit.TranscriptConfig{Theme: theme, Glyphs: glyphs, Publisher: runtime.Publisher(), Retention: base.PublishCompleted})
	if err != nil {
		panic(err)
	}
	if _, err = r.view.Controller().AddDocument(r.document); err != nil {
		panic(err)
	}
	status := scene.NewElement(scene.ElementConfig{Paint: r.paintStatus})
	r.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{
		Key: "read", Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: r.Handle(event.Input)} },
		Detached: r.view.Controller().Close,
	}, Items: []scene.Item{{Element: r.view.Element(), Size: layout.Flex(1)}, {Element: status, Size: layout.Fixed(1)}}}).Element()

	// Title and notification are optional terminal enhancements.
	_ = runtime.Session().SetTitle("reading")
	r.scope, r.renderErr = runtime.NewScope(program.ScopeConfig{})
	if r.renderErr == nil {
		r.timer, r.renderErr = runtime.Every(r.scope, every, r.advance)
	}
	return r
}

func (r *reader) advance() {
	r.spinner.Tick()
	if r.at >= len(r.pieces) {
		r.finish()
		return
	}
	if err := r.document.Append([]byte(r.pieces[r.at])); err != nil {
		r.renderErr = err
		r.timer.Stop()
		return
	}
	r.at++
	// This finite tutorial prepares a small snapshot synchronously. The same
	// immutable snapshot can be prepared by program.Work for network-sized sources.
	r.renderErr = r.document.Accept(r.document.Snapshot().Prepare(r.scope.Context(), r.render))
	r.element.Invalidate()
}

func (r *reader) finish() {
	r.renderErr = r.document.End(nil)
	if r.renderErr == nil {
		r.renderErr = r.document.Accept(r.document.Snapshot().Prepare(r.scope.Context(), r.render))
	}
	r.timer.Stop()
	r.finished = r.renderErr == nil
	r.element.Invalidate()
	// Completion is represented in the scene even when the terminal has no notifier.
	_ = r.runtime.Session().SetTitle("")
	_ = r.runtime.Session().Notify("the answer is finished")
}

func (r *reader) paintStatus(frame scene.Frame) {
	switch {
	case r.renderErr != nil:
		kit.Label{Text: r.renderErr.Error(), Style: r.theme.Danger}.Draw(frame.View)
	case r.finished:
		kit.Label{Text: "that is the whole answer — ctrl+c to leave", Style: r.theme.Muted}.Draw(frame.View)
	default:
		r.spinner.Draw(frame.View)
	}
}

func (r *reader) Element() scene.Element { return r.element }

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
