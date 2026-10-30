package main

import (
	"image"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type workflowStep struct {
	label string
	state stepState
}

type workflow struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	steps  []workflowStep
}

func newWorkflow(theme kit.Theme, glyphs kit.Glyphs) workflow {
	w := workflow{theme: theme, glyphs: glyphs}
	w.Reset()
	return w
}

func (w *workflow) Reset() {
	w.steps = []workflowStep{
		{label: "understand request", state: stepWaiting},
		{label: "inspect ownership", state: stepWaiting},
		{label: "review and apply", state: stepWaiting},
		{label: "verify invariants", state: stepWaiting},
	}
}

func (w *workflow) Apply(update stepUpdate) {
	if update.Index < 0 || update.Index >= len(w.steps) {
		return
	}
	w.steps[update.Index].state = update.State
}

func (*workflow) HeightForWidth(int) int { return 6 }

func (w *workflow) Draw(view grid.View) {
	box := kit.Box{
		Theme: w.theme, Glyphs: w.glyphs, Title: "run plan",
		Padding: layout.Symmetric(0, 1),
	}
	inner := box.Draw(view)
	width, height := inner.Size()
	for row, step := range w.steps {
		if row >= height {
			return
		}
		mark, state, style := w.glyphs.Free, "waiting", w.theme.Subtle
		switch step.state {
		case stepWaiting:
		case stepRunning:
			mark, state, style = w.glyphs.Marker, "running", w.theme.Accent
		case stepDone:
			mark, state, style = w.glyphs.Taken, "done", w.theme.Success
		case stepSkipped:
			mark, state, style = w.glyphs.Bullet, "skipped", w.theme.Muted
		}
		at := inner.Text(0, row, mark+" "+step.label, style)
		stateWidth := len(state)
		if width-at > stateWidth+1 {
			inner.Text(width-stateWidth, row, state, style)
		}
	}
}

type reviewRequest struct {
	proposal changeProposal
	answer   chan bool
}

type reviewPane struct {
	diff *kit.Diff
	// window scrolls the change. A diff is as tall as the change is, and a review
	// pane is as tall as the terminal: without a window the part that did not fit was
	// simply not there, and somebody was asked to allow a change they could not read.
	window  *base.Viewport
	element scene.Element
	form    *kit.Form
	theme   kit.Theme
	title   string
}

func (p *reviewPane) Element() scene.Element {
	if !p.element.IsZero() {
		return p.element
	}
	title := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		kit.Label{Text: p.title, Style: p.theme.Subtle, Ellipsis: "…"}.Draw(frame.View)
	}})
	p.element = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: title}, {Element: p.window.Element()}, {Element: p.form.Element()}},
		Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
			key, ok := event.Input.(input.Key)
			if !ok || !key.Down() {
				return scene.EventResult{}
			}
			switch key.Code {
			case input.PageUp:
				return scene.EventResult{Consumed: p.window.Do(base.ScrollPageUp)}
			case input.PageDown:
				return scene.EventResult{Consumed: p.window.Do(base.ScrollPageDown)}
			default:
				return scene.EventResult{}
			}
		}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			formRows := min(p.form.HeightForWidth(space.X), max(space.Y-2, 0))
			return (layout.Flow{Axis: layout.Down}).Rects(space, []layout.Slot{{Size: layout.Fixed(1)}, {Size: layout.Flex(1)}, {Size: layout.Fixed(formRows)}})
		},
	}).Element()
	return p.element
}
