// Command composer keeps paste payloads with application-owned elements so undo and
// history can preserve their exact bytes.
package main

import (
	"context"
	"fmt"
	"image"
	"os"

	"github.com/Tangerg/oolong/ui/base"
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

const (
	submitPrompt                 keymap.Action    = "submit"
	previousPrompt               keymap.Action    = "previous-prompt"
	nextPrompt                   keymap.Action    = "next-prompt"
	quitPrompt                   keymap.Action    = "quit"
	pasteElement                 base.ElementKind = 1
	largePasteLines                               = 3
	minimumReferencePopupColumns                  = 38
)

func main() {
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true, Mouse: true}, Build: func(runtime *program.Runtime) (scene.Element, error) {
		return newPrompt(runtime).Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "composer:", err)
		os.Exit(1)
	}
}

type prompt struct {
	runtime *program.Runtime
	theme   kit.Theme
	glyphs  kit.Glyphs

	composer   *kit.Composer
	completion *base.Completion
	history    *base.History[draft]
	output     kit.Paragraph
	status     string
	references []reference
	pastes     map[base.ElementID]pasteText
	element    scene.Element
}

func newPrompt(runtime *program.Runtime) *prompt {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	keys := base.DefaultEditorKeys()
	keys.Bind(submitPrompt, input.Chord{Code: input.Enter})
	keys.Bind(previousPrompt, input.Chord{Code: input.Up})
	keys.Bind(nextPrompt, input.Chord{Code: input.Down})
	keys.Bind(quitPrompt, input.Ctrl.Rune('c'))

	p := &prompt{
		runtime: runtime, theme: theme, glyphs: glyphs,
		status: fmt.Sprintf("paste %d or more lines to make one atomic chip", largePasteLines),
		history: base.NewHistory(base.HistoryConfig[draft]{
			Clone: draft.clone,
			Equal: draft.equal,
		}),
		references: []reference{
			{name: "README.md", detail: "project introduction"},
			{name: "docs/core/architecture.md", detail: "ownership and layering"},
			{name: "ui/core/program", detail: "runtime and ingress"},
			{name: "ui/base", detail: "behavior without appearance"},
		},
		pastes: make(map[base.ElementID]pasteText),
	}
	p.composer = kit.NewComposer(kit.ComposerConfig{
		Theme: theme, Glyphs: glyphs, Prompt: glyphs.Marker + " ", Keys: keys,
		Hints: []keymap.Action{submitPrompt, previousPrompt, nextPrompt, quitPrompt}, MaxRows: 1,
		Placeholder: "Type @ to reference something", Clipboard: runtime.Clipboard(),
	})
	p.composer.Controller().SetSingleLine(true)
	p.completion = base.NewCompletion(base.CompletionConfig{
		Look: theme.Look(glyphs), MaxRows: 5, Keys: base.DefaultCompletionKeys(),
		CurrentToken: func() (base.Token, bool) {
			return p.composer.Controller().TokenAtCursor(base.Trigger{Prefix: "@"})
		},
		Accept: func(candidate base.Candidate, token base.Token) {
			p.composer.Controller().Replace(token.Line, token.Start, token.End, candidate.Text)
			p.completion.Dismiss()
		},
	})
	p.output.SetText([]text.Line{
		text.Of("A prompt can carry text and application-owned elements without becoming a new widget kind.", theme.Text),
	})
	unsubscribe := p.composer.Controller().Session().Subscribe(func(base.ChangeMap) {
		p.releaseRemovedPastes()
		p.refreshCompletion()
	})
	body := scene.NewGroup(scene.GroupConfig{
		Element: scene.ElementConfig{Paint: p.Draw}, Items: []scene.Item{{Element: p.composer.Element()}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			height := min(p.composer.HeightForWidth(space.X), space.Y)
			return []image.Rectangle{grid.Area(0, space.Y-height, space.X, height)}
		},
	})
	completionHelp := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		kit.Help{Theme: p.theme, Keys: p.completion.Keys(), Show: []keymap.Action{base.Accept}}.Draw(frame.View)
	}})
	completionBody := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
		{Element: p.completion.Element(), Size: layout.Measured(0, 0)},
		{Element: completionHelp, Size: layout.Fixed(1)},
	}})
	panel := kit.NewPanel(kit.PanelConfig{Theme: theme, Box: kit.Box{
		Glyphs: glyphs, Title: "references", Padding: layout.Symmetric(0, 1),
	}, Content: completionBody.Element()})
	popup := scene.NewGroup(scene.GroupConfig{
		Element: scene.ElementConfig{Enabled: p.completion.Open}, Items: []scene.Item{{Element: panel.Element()}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			if !p.completion.Open() {
				return []image.Rectangle{{}}
			}
			width := max(0, min(max(p.completion.Width()+panel.Box().Overhead().X, minimumReferencePopupColumns), space.X-2))
			height := min(panel.HeightForWidth(width), space.Y)
			y := max(space.Y-p.composer.HeightForWidth(space.X)-height, 0)
			return []image.Rectangle{grid.Area(1, y, width, height)}
		},
	})
	p.element = scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{
		Key: "composer", Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: p.Handle(event.Input())} }, Detached: unsubscribe,
	}, Items: []scene.Item{{Element: body.Element()}, {Element: popup.Element()}}}).Element()
	return p
}

func (p *prompt) Draw(frame scene.Frame) {
	width, height := frame.Size()
	composerRows := min(p.composer.HeightForWidth(width), height)
	rects := (layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(2)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(composerRows)},
	})
	rows := frame.Subs(rects)
	kit.Label{Text: "Composable prompt", Style: p.theme.Heading}.Draw(rows[0].View)
	kit.Label{Text: p.status, Style: p.theme.Subtle, Ellipsis: p.glyphs.Ellipsis}.
		Draw(rows[0].View.Sub(grid.Area(0, 1, width, 1)))
	p.output.Draw(rows[1].View)
}

func (p *prompt) Handle(event input.Event) bool {
	var action keymap.Action
	if key, ok := event.(input.Key); ok {
		action, _ = p.composer.Controller().Keys().ActionFor(key)
	}
	if action == quitPrompt {
		p.runtime.Quit()
		return true
	}
	if _, isMouse := event.(input.Mouse); !isMouse && p.completion.Handle(event) {
		return true
	}
	if paste, ok := event.(input.Paste); ok && pasteText(paste.Text).lineCount() >= largePasteLines {
		p.insertPaste(paste.Text)
		return true
	}
	switch action {
	case submitPrompt:
		p.submit()
	case previousPrompt:
		p.recallBack()
	case nextPrompt:
		p.recallForward()
	default:
		return false
	}
	return true
}

func (p *prompt) Element() scene.Element { return p.element }
