package main

import (
	"context"
	"image"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type reviewRequest struct {
	ctx      context.Context //nolint:containedctx // A review is only actionable during its issuing request.
	proposal changeProposal
	done     chan struct{}
	approved bool
	err      error
}

type reviewPane struct {
	sessionKey func(input.Key) bool
	diff       *kit.Diff
	// A review must remain readable when the diff exceeds terminal height; a
	// viewport prevents approval from depending on content the user cannot reach.
	window  *base.Viewport
	element scene.Element
	form    *kit.Form
	theme   kit.Theme
	glyphs  kit.Glyphs
	title   string
}

func (p *reviewPane) Element() scene.Element {
	if !p.element.IsZero() {
		return p.element
	}
	title := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		kit.Label{Text: p.title, Style: p.theme.Subtle, Ellipsis: p.glyphs.Ellipsis}.Draw(frame.View)
	}})
	p.element = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: title}, {Element: p.window.Element()}, {Element: p.form.Element()}},
		Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
			key, ok := event.Input().(input.Key)
			if !ok || !key.Down() {
				return scene.EventResult{}
			}
			if p.sessionKey(key) {
				return scene.EventResult{Consumed: true}
			}
			for chord := range key.ShortcutChords() {
				switch chord.Code {
				case input.PageUp:
					return scene.EventResult{Consumed: p.window.Do(base.ScrollPageUp)}
				case input.PageDown:
					return scene.EventResult{Consumed: p.window.Do(base.ScrollPageDown)}
				default:
					continue
				}
			}
			return scene.EventResult{}
		}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			formRows := min(p.form.HeightForWidth(space.X), max(space.Y-2, 0))
			return (layout.Flow{Axis: layout.Down}).Rects(space, []layout.Slot{{Size: layout.Fixed(1)}, {Size: layout.Flex(1)}, {Size: layout.Fixed(formRows)}})
		},
	}).Element()
	return p.element
}
