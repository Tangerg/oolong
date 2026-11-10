package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/text"
)

type draft struct {
	text  string
	chips []chip
}

// History stores payload identity with its offset because equal visible labels
// can refer to different original pastes.
type chip struct {
	at   int
	body pasteText
}

func (d draft) clone() draft {
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
		if !ok {
			continue
		}
		d.chips = append(d.chips, chip{at: starts[element.Line] + element.Start, body: body})
	}
	return d
}

// History must restore attachment elements, not just their labels, so each
// recalled draft keeps the original payload and editing behavior.
func (p *prompt) restore(d draft) {
	editor := p.composer.Controller()
	if len(d.chips) == 0 {
		editor.SetText(d.text)
		return
	}
	editor.SetText("")
	at := 0
	for _, c := range d.chips {
		if c.at < at || c.at > len(d.text) {
			break
		}
		editor.Insert(d.text[at:c.at])
		p.attach(c.body)
		at = c.at + len(c.body.label())
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
	p.status = "submitted; recall history without losing the current draft"
	p.composer.Controller().Clear()
	p.composer.Controller().ForgetHistory()
	p.completion.Dismiss()
	clear(p.pastes)
}
