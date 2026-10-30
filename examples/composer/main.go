// Command composer demonstrates a product-grade prompt without inventing a prompt
// framework: completion, history and atomic paste chips are ordinary editor behavior
// composed with application policy.
package main

import (
	"context"
	"fmt"
	"image"
	"os"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/fuzzy"
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
	submitPrompt keymap.Action    = "submit"
	quitPrompt   keymap.Action    = "quit"
	pasteElement base.ElementKind = 1
	largePaste                    = 3
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

type reference struct{ name, detail string }

type prompt struct {
	runtime *program.Runtime
	theme   kit.Theme
	glyphs  kit.Glyphs
	keys    *keymap.Map

	composer   *kit.Composer
	completion base.Completion
	history    *base.History[draft]
	output     kit.Paragraph
	status     string
	references []reference
	pastes     map[uint64]string
	element    scene.Element
}

func newPrompt(runtime *program.Runtime) *prompt {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	keys := base.DefaultEditorKeys()
	keys.Bind(submitPrompt, input.Chord{Code: input.Enter})
	keys.Bind(quitPrompt, input.Ctrl.Rune('c'))

	p := &prompt{
		runtime: runtime, theme: theme, glyphs: glyphs, keys: keys,
		status: "paste three or more lines to make one atomic chip",
		history: base.NewHistory(base.HistoryConfig[draft]{
			Clone: draft.clone,
			Equal: draft.equal,
		}),
		references: []reference{
			{name: "README.md", detail: "project introduction"},
			{name: "docs/architecture.md", detail: "ownership and layering"},
			{name: "ui/core/program", detail: "runtime and ingress"},
			{name: "ui/base", detail: "behavior without appearance"},
		},
		pastes: make(map[uint64]string),
	}
	p.composer = kit.NewComposer(kit.ComposerConfig{
		Theme: theme, Prompt: glyphs.Marker + " ",
		Hints: []keymap.Action{submitPrompt, quitPrompt}, MaxRows: 1,
	})
	p.composer.Controller().Placeholder = "Type @ to reference something"
	p.composer.Controller().Keys = keys
	p.composer.Controller().SetSingleLine(true)
	p.composer.Controller().Clipboard = runtime.Clipboard()
	p.completion = base.Completion{
		Look: theme.Look(glyphs), MaxRows: 5,
		Accept: func(candidate base.Candidate, token base.Token) {
			p.composer.Controller().Replace(token.Start, token.End, candidate.Text)
			p.completion.Dismiss()
		},
	}
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
	panel := kit.NewPanel(kit.PanelConfig{Box: kit.Box{
		Theme: theme, Glyphs: glyphs, Title: "references", Footer: "tab complete",
		FooterAlign: layout.End, Padding: layout.Symmetric(0, 1),
	}, Content: p.completion.Element()})
	popup := scene.NewGroup(scene.GroupConfig{
		Element: scene.ElementConfig{Enabled: p.completion.Open}, Items: []scene.Item{{Element: panel.Element()}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			if !p.completion.Open() {
				return []image.Rectangle{{}}
			}
			width := max(0, min(max(p.completion.Width()+4, 38), space.X-2))
			height := min(p.completion.HeightForWidth(width)+2, space.Y)
			y := max(space.Y-p.composer.HeightForWidth(space.X)-height, 0)
			return []image.Rectangle{grid.Area(1, y, width, height)}
		},
	})
	p.element = scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{
		Key: "composer", Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: p.Handle(event.Input)} }, Detached: unsubscribe,
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
	if key, ok := event.(input.Key); ok && key.Down() {
		if action, _ := p.keys.Action(key.Chord()); action == quitPrompt {
			p.runtime.Quit()
			return true
		}
	}
	if _, isMouse := event.(input.Mouse); !isMouse && p.completion.Handle(event) {
		p.releaseRemovedPastes()
		return true
	}
	if paste, ok := event.(input.Paste); ok && strings.Count(paste.Text, "\n")+1 >= largePaste {
		p.insertPaste(paste.Text)
		return true
	}
	if key, ok := event.(input.Key); ok && key.Down() && p.walk(key) {
		return true
	}
	return false
}

func (p *prompt) walk(key input.Key) bool {
	switch key.Code {
	case input.Up:
		p.recallBack()
	case input.Down:
		p.recallForward()
	case input.Enter:
		p.submit()
		return true
	default:
		return false
	}
	p.releaseRemovedPastes()
	p.refreshCompletion()
	return true
}

func (p *prompt) insertPaste(body string) {
	p.attach(body)
	p.status = fmt.Sprintf("%d-line paste attached; backspace removes it atomically",
		strings.Count(body, "\n")+1)
	p.refreshCompletion()
}

func (p *prompt) attach(body string) {
	element := p.composer.Controller().InsertElement(pasteElement, pasteLabel(body))
	if element.ID != 0 {
		if p.pastes == nil {
			p.pastes = make(map[uint64]string)
		}
		p.pastes[element.ID] = strings.Clone(body)
	}
}

type draft struct {
	text  string
	chips []chip
}

// chip is one attachment: where its label begins in the text, and the bytes it
// stands for.
type chip struct {
	at   int
	body string
}

func (d draft) clone() draft {
	d.text = strings.Clone(d.text)
	d.chips = slices.Clone(d.chips)
	return d
}

func (d draft) equal(other draft) bool {
	return d.text == other.text && slices.Equal(d.chips, other.chips)
}

func (p *prompt) current() draft {
	editor := p.composer.Controller()
	text := editor.Text()
	lines := strings.Split(text, "\n")
	starts := make([]int, len(lines))
	at := 0
	for i, line := range lines {
		starts[i] = at
		at += len(line) + 1
	}
	d := draft{text: text}
	for _, element := range editor.Elements() {
		body, ok := p.pastes[element.ID]
		if !ok || element.Line < 0 || element.Line >= len(starts) {
			continue
		}
		d.chips = append(d.chips, chip{at: starts[element.Line] + element.Start, body: body})
	}
	return d
}

// restore puts a recalled document back with its attachments, not merely its words.
//
// The chips go in as elements again, at the offsets they were recorded at, so the
// bytes behind them are attached to this draft as they were to the one it came from.
// A document nobody recorded chips for is exactly its text.
func (p *prompt) restore(d draft) {
	editor := p.composer.Controller()
	if len(d.chips) == 0 {
		editor.SetText(d.text)
		p.releaseRemovedPastes()
		return
	}
	editor.SetText("")
	p.releaseRemovedPastes()
	at := 0
	for _, c := range d.chips {
		if c.at < at || c.at > len(d.text) {
			break
		}
		editor.Insert(d.text[at:c.at])
		p.attach(c.body)
		at = c.at + len(pasteLabel(c.body))
		// InsertElement writes the separator that follows a chip. Where the recorded
		// document has one it is that one, and where it does not — the writer deleted
		// it and carried straight on — it is a word this draft never had.
		if at < len(d.text) && d.text[at] == ' ' {
			at++
			continue
		}
		editor.DeleteBack()
	}
	if at <= len(d.text) {
		editor.Insert(d.text[at:])
	}
}

func (p *prompt) recallBack() {
	if value, moved := p.history.Back(p.current()); moved {
		p.restore(value)
	}
}

func (p *prompt) recallForward() {
	if value, moved := p.history.Forward(); moved {
		p.restore(value)
	}
}

// pasteLabel is what a chip says. One function, because the label written when a
// paste arrives and the label looked for when its entry comes back have to be the
// same string.
func pasteLabel(body string) string {
	return fmt.Sprintf("[paste %d lines]", strings.Count(body, "\n")+1)
}

func (p *prompt) releaseRemovedPastes() {
	live := make(map[uint64]struct{})
	for _, id := range p.composer.Controller().RetainedElementIDs() {
		live[id] = struct{}{}
	}
	for id := range p.pastes {
		if _, ok := live[id]; !ok {
			delete(p.pastes, id)
		}
	}
}

func (p *prompt) submit() {
	body := strings.TrimSpace(p.composer.Controller().Text())
	if body == "" {
		return
	}
	sent := p.current()
	// The offsets were taken against the whole document and the entry is what is
	// left of it once the surrounding blank space is gone.
	lead := strings.Index(sent.text, body)
	var chips []chip
	for _, c := range sent.chips {
		if at := c.at - lead; at >= 0 && at <= len(body) {
			chips = append(chips, chip{at: at, body: c.body})
		}
	}
	attached := len(chips)
	p.history.Add(draft{text: body, chips: chips})
	p.output.SetText([]text.Line{
		text.Of("sent: "+body, p.theme.Text),
		text.Of(fmt.Sprintf("%d attached paste(s); the application still owns their original bytes", attached), p.theme.Muted),
	})
	p.status = "submitted; up restores it without losing the current draft"
	p.composer.Controller().Clear()
	p.composer.Controller().ForgetHistory()
	p.completion.Dismiss()
	clear(p.pastes)
}

func (p *prompt) refreshCompletion() {
	line, column := p.composer.Controller().Cursor()
	if line != 0 {
		p.completion.Dismiss()
		return
	}
	token, ok := base.TokenAt(p.composer.Controller().Text(), column, base.Trigger{Prefix: "@"})
	if !ok {
		p.completion.Dismiss()
		return
	}
	matches := fuzzy.FilterFunc(token.Query, p.references, func(r reference) string { return r.name })
	candidates := make([]base.Candidate, 0, len(matches))
	for _, match := range matches {
		item := p.references[match.Index]
		candidates = append(candidates, base.Candidate{
			Text: item.name, Label: item.name, Detail: item.detail, Matched: match.Match.At,
		})
	}
	p.completion.Offer(token, candidates)
}

func (p *prompt) Element() scene.Element { return p.element }
