package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/text"
)

func (p *prompt) insertPaste(body string) {
	paste := pasteText(body)
	p.attach(paste)
	p.status = fmt.Sprintf("%d-line paste attached; backspace removes it atomically", paste.lineCount())
}

func (p *prompt) attach(body pasteText) {
	element := p.composer.Controller().InsertElement(pasteElement, body.label())
	if element.ID != (base.ElementID{}) {
		p.pastes[element.ID] = body
	}
}

type pasteText string

func (p pasteText) lineCount() int {
	return strings.Count(text.NormalizeLineEndings(string(p)), "\n") + 1
}

func (p pasteText) label() string {
	return fmt.Sprintf("[paste %d lines]", p.lineCount())
}

// releaseRemovedPastes forgets a payload once the editor retains its element
// neither in the document nor in undo history.
func (p *prompt) releaseRemovedPastes() {
	retained := p.composer.Controller().RetainedElementIDs()
	maps.DeleteFunc(p.pastes, func(id base.ElementID, _ pasteText) bool { return !slices.Contains(retained, id) })
}
