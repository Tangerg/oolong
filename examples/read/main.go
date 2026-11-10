// Command read prepares whole-source snapshots because a late reference definition
// can change an earlier Markdown link.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
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
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	app, err := program.NewInline(program.InlineConfig{Terminal: term.Features{Probe: true}, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
		r, err := newReader(runtime)
		if err != nil {
			return scene.Element{}, err
		}
		return r.Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
}

type reader struct {
	runtime  *program.InlineRuntime
	theme    kit.Theme
	element  scene.Element
	document *content.StreamDocument
	render   content.Renderer
	view     *kit.Transcript
	pieces   [][]byte
	at       int
	scope    *program.Scope
	spinner  kit.Spinner
	stepErr  error
	keys     keymap.Map
}

// Tests control the mock delivery pace without introducing real-time waits.
const (
	piece               = 7
	pace                = 60 * time.Millisecond
	leave keymap.Action = "leave"
)

func newReader(runtime *program.InlineRuntime) (*reader, error) {
	return newPacedReader(runtime, piece, pace)
}

func newPacedReader(runtime *program.InlineRuntime, size int, every time.Duration) (*reader, error) {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())

	r := &reader{
		runtime: runtime,
		theme:   theme,
		pieces:  slices.Collect(slices.Chunk([]byte(answer()), size)),
		spinner: kit.Spinner{Theme: theme, Glyphs: glyphs, Label: "writing"},
	}
	r.keys.Bind(leave, input.Ctrl.Rune('c'))
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
	r.document, err = content.NewStreamDocument(content.StreamConfig{Format: markdown.Format})
	if err != nil {
		return nil, err
	}
	r.view, err = kit.NewTranscript(kit.TranscriptConfig{Behavior: base.TranscriptConfig{Publisher: runtime.Publisher(), Retention: base.PublishCompleted}, Theme: theme, Glyphs: glyphs})
	if err != nil {
		return nil, err
	}
	if _, err = r.view.Controller().AddDocument(r.document); err != nil {
		r.view.Controller().Close()
		return nil, err
	}
	status := scene.NewElement(scene.ElementConfig{Paint: r.paintStatus})
	r.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{
		Key: "read", Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: r.Handle(event.Input())} },
		Detached: r.view.Controller().Close,
	}, Items: []scene.Item{{Element: r.view.Element(), Size: layout.Flex(1)}, {Element: status, Size: layout.Fixed(1)}}}).Element()

	// Title and notification are optional terminal enhancements.
	_ = runtime.Session().SetTitle("reading")
	if r.scope, err = runtime.NewScope(program.ScopeConfig{}); err != nil {
		r.view.Controller().Close()
		return nil, err
	}
	if _, err = runtime.Every(r.scope, every, r.advance); err != nil {
		r.scope.Close()
		r.view.Controller().Close()
		return nil, err
	}
	return r, nil
}

func (r *reader) advance() {
	r.spinner.Tick()
	if r.at >= len(r.pieces) {
		r.finish(nil)
		return
	}
	if err := r.document.Append(r.pieces[r.at]); err != nil {
		r.finish(err)
		return
	}
	r.at++
	// This finite tutorial prepares a small snapshot synchronously. The same
	// immutable snapshot can be prepared by program.Work for network-sized sources.
	r.stepErr = r.document.Accept(r.document.Snapshot().Prepare(r.scope.Context(), r.render))
	r.element.Invalidate()
}

func (r *reader) finish(cause error) {
	r.stepErr = r.document.End(cause)
	if r.stepErr == nil {
		r.stepErr = r.document.Accept(r.document.Snapshot().Prepare(r.scope.Context(), r.render))
	}
	r.scope.Close()
	r.element.Invalidate()
	// Completion is represented in the scene even when the terminal has no notifier.
	_ = r.runtime.Session().SetTitle("")
	message := "the answer is finished"
	if err := r.problem(r.document.Current()); err != nil {
		message = err.Error()
	}
	_ = r.runtime.Session().Notify(message)
}

func (r *reader) problem(change content.DocumentChange) error {
	return errors.Join(r.stepErr, change.SourceError, change.PrepareError)
}

func (r *reader) paintStatus(frame scene.Frame) {
	change := r.document.Current()
	switch err := r.problem(change); {
	case err != nil:
		kit.Label{Text: err.Error(), Style: r.theme.Danger}.Draw(frame.View)
	case change.State == content.Final:
		x := frame.Text(0, 0, "that is the whole answer — ", r.theme.Muted)
		width, _ := frame.Size()
		kit.Help{Theme: r.theme, Keys: &r.keys, Show: []keymap.Action{leave}}.
			Draw(frame.View.Sub(grid.Area(x, 0, width-x, 1)))
	default:
		r.spinner.Draw(frame.View)
	}
}

func (r *reader) Element() scene.Element { return r.element }

func (r *reader) Handle(ev input.Event) bool {
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	if action, bound := r.keys.ActionFor(key); bound && action == leave {
		r.runtime.Quit()
		return true
	}
	return false
}

func answer() string {
	return strings.Join([]string{
		"# What is happening here",
		"",
		"This answer is arriving a few characters at a time. You are looking at",
		"a **preview**: even an earlier [reference][guide] can change when its",
		"definition arrives. Nothing here is permanent until the answer ends.",
		"",
		"- This list grows as its items arrive.",
		"- Each preview uses the whole source received so far.",
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
		"The definition below completes the earlier link. At EOF the final document",
		"is offered to the terminal's scrollback as one publication.",
		"",
		"[guide]: https://github.com/Tangerg/oolong",
		"",
	}, "\n")
}
