// Command composer demonstrates a product-grade prompt without inventing a prompt
// framework: completion, history and atomic paste chips are ordinary editor behavior
// composed with application policy.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/fuzzy"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/core/text"
)

const (
	submitPrompt keymap.Action        = "submit"
	quitPrompt   keymap.Action        = "quit"
	pasteElement headless.ElementKind = 1
	largePaste                        = 3
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return headless.NewRoot(newPrompt(runtime))
		},
		Terminal: term.Config{Probe: true, Mouse: true},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "composer:", err)
		os.Exit(1)
	}
}

type reference struct{ name, detail string }

type prompt struct {
	runtime *program.Runtime
	theme   kit.Theme
	glyphs  kit.Glyphs
	keys    *keymap.Map

	composer   kit.Composer
	completion headless.Completion
	history    headless.History
	output     kit.Paragraph
	status     string
	references []reference
	pastes     map[uint64]string
	field      headless.PointerRegion
}

func newPrompt(runtime *program.Runtime) *prompt {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	keys := headless.DefaultEditorKeys()
	keys.Bind(submitPrompt, input.Chord{Code: input.Enter})
	keys.Bind(quitPrompt, input.Ctrl.Rune('c'))

	p := &prompt{
		runtime: runtime, theme: theme, glyphs: glyphs, keys: keys,
		status: "paste three or more lines to make one atomic chip",
		references: []reference{
			{name: "README.md", detail: "project introduction"},
			{name: "docs/architecture.md", detail: "ownership and layering"},
			{name: "core/program", detail: "runtime and ingress"},
			{name: "components/headless", detail: "behavior without appearance"},
		},
		pastes: make(map[uint64]string),
	}
	p.composer = kit.Composer{
		Theme: theme, Prompt: glyphs.Marker + " ",
		Hints: []keymap.Action{submitPrompt, quitPrompt}, MaxRows: 1,
	}
	p.composer.Editor().Placeholder = "Type @ to reference something"
	p.composer.Editor().Keys = keys
	p.composer.Editor().SingleLine = true
	p.composer.Editor().Clipboard = runtime.Clipboard()
	p.composer.Focus(true)
	p.completion = headless.Completion{
		Look: theme.Look(glyphs), MaxRows: 5,
		Accept: func(candidate headless.Candidate, token headless.Token) {
			p.composer.Editor().Replace(token.Start, token.End, candidate.Text)
		},
	}
	p.output.SetText([]text.Line{
		text.Of("A prompt can carry text and application-owned elements without becoming a new widget kind.", theme.Text),
	})
	return p
}

func (p *prompt) Draw(frame headless.Frame) {
	width, height := frame.Size()
	composerRows := min(p.composer.Measure(width), height)
	rects := (layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(2)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(composerRows)},
	})
	rows := frame.Subs(rects)
	kit.Label{Text: "Composable prompt", Style: p.theme.Heading}.Draw(rows[0].View)
	kit.Label{Text: p.status, Style: p.theme.Subtle, Ellipsis: p.glyphs.Ellipsis}.
		Draw(rows[0].View.Sub(grid.Rect(0, 1, width, 1)))
	p.output.Draw(rows[1].View)
	p.field.Stage(frame, rects[2], &p.composer)
	p.composer.Draw(rows[2])
	if p.completion.Open() {
		p.drawCompletion(frame, composerRows)
	}
}

func (p *prompt) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		if action, _ := p.keys.Action(key.Chord()); action == quitPrompt {
			p.runtime.Quit()
			return true
		}
	}
	if p.completion.Handle(event) {
		p.releaseRemovedPastes()
		return true
	}
	if paste, ok := event.(input.Paste); ok && strings.Count(paste.Text, "\n")+1 >= largePaste {
		p.insertPaste(paste.Text)
		return true
	}
	if key, ok := event.(input.Key); ok && key.Down() {
		switch key.Code {
		case input.Up:
			if value, moved := p.history.Back(p.composer.Text()); moved {
				p.composer.SetText(value)
			}
			p.releaseRemovedPastes()
			p.refreshCompletion()
			return true
		case input.Down:
			if value, moved := p.history.Forward(); moved {
				p.composer.SetText(value)
			}
			p.releaseRemovedPastes()
			p.refreshCompletion()
			return true
		case input.Enter:
			p.submit()
			return true
		default:
		}
	}

	if mouse, ok := event.(input.Mouse); ok {
		handled, _ := p.field.Handle(mouse)
		if handled {
			p.refreshCompletion()
		}
		return handled
	}
	handled := p.composer.Handle(event)
	if handled {
		p.releaseRemovedPastes()
		p.refreshCompletion()
	}
	return handled
}

func (p *prompt) insertPaste(body string) {
	lines := strings.Count(body, "\n") + 1
	element := p.composer.Editor().InsertElement(pasteElement, fmt.Sprintf("[paste %d lines]", lines))
	if element.ID != 0 {
		p.pastes[element.ID] = strings.Clone(body)
	}
	p.status = fmt.Sprintf("%d-line paste attached; backspace removes it atomically", lines)
	p.refreshCompletion()
}

func (p *prompt) releaseRemovedPastes() {
	live := make(map[uint64]struct{})
	for _, element := range p.composer.Editor().Elements() {
		live[element.ID] = struct{}{}
	}
	for id := range p.pastes {
		if _, ok := live[id]; !ok {
			delete(p.pastes, id)
		}
	}
}

func (p *prompt) submit() {
	body := strings.TrimSpace(p.composer.Text())
	if body == "" {
		return
	}
	attached := len(p.pastes)
	p.history.Add(body)
	p.output.SetText([]text.Line{
		text.Of("sent: "+body, p.theme.Text),
		text.Of(fmt.Sprintf("%d attached paste(s); the application still owns their original bytes", attached), p.theme.Muted),
	})
	p.status = "submitted; up restores it without losing the current draft"
	p.composer.Reset()
	p.completion.Dismiss()
	clear(p.pastes)
}

func (p *prompt) refreshCompletion() {
	line, column := p.composer.Editor().Cursor()
	if line != 0 {
		p.completion.Dismiss()
		return
	}
	token, ok := headless.TokenAt(p.composer.Text(), column, headless.Trigger{Prefix: "@"})
	if !ok {
		p.completion.Dismiss()
		return
	}
	names := make([]string, len(p.references))
	for i, item := range p.references {
		names[i] = item.name
	}
	matches := fuzzy.Filter(token.Query, names)
	candidates := make([]headless.Candidate, 0, len(matches))
	for _, match := range matches {
		item := p.references[match.Index]
		candidates = append(candidates, headless.Candidate{
			Text: item.name, Label: item.name, Detail: item.detail, Matched: match.Match.At,
		})
	}
	p.completion.Offer(token, candidates)
}

func (p *prompt) drawCompletion(frame headless.Frame, composerRows int) {
	width, height := frame.Size()
	rows := p.completion.Measure(width)
	if width <= 2 || rows <= 0 {
		return
	}
	box := kit.Box{
		Theme: p.theme, Glyphs: p.glyphs, Title: "references", Footer: "tab complete",
		FooterAlign: layout.End, Padding: layout.Symmetric(0, 1),
	}
	popupWidth := min(max(p.completion.Width()+4, 38), width-2)
	popupHeight := min(rows+2, height)
	y := max(height-composerRows-popupHeight, 0)
	area := grid.Rect(1, y, popupWidth, popupHeight)
	inner := box.InnerRect(area.Size())
	box.Draw(frame.View.Sub(area))
	p.completion.Draw(frame.Sub(area).Sub(inner))
}
