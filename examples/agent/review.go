package main

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
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
	window *headless.Viewport
	// scrolling and choosing are where the change and the form were drawn. A pointer
	// report arrives in this widget's coordinates and each of them reasons in its
	// own, so handing one straight on aimed it at whatever was at the top of the
	// pane: a wheel on the last row of the change did nothing at all.
	scrolling headless.PointerRegion
	choosing  headless.PointerRegion
	form      *kit.Form
	theme     kit.Theme
	title     string
}

func (p *reviewPane) Draw(frame headless.Frame) {
	width, height := frame.Size()
	// The title takes a row and the change must have one: counting only the form
	// against the pane's height left the change no rows on a short terminal, and
	// somebody was asked to allow a change that was not on the screen.
	formRows := min(p.form.HeightForWidth(width), max(height-2, 0))
	rects := (layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(formRows)},
	})
	rows := frame.Subs(rects)
	kit.Label{Text: p.title, Style: p.theme.Subtle, Ellipsis: "…"}.Draw(rows[0].View)
	p.window.Draw(rows[1])
	p.scrolling.Stage(frame, rects[1], p.window)
	p.form.Draw(rows[2])
	p.choosing.Stage(frame, rects[2], p.form)
}

// Handle gives the keyboard to the form and the wheel to the change. The form owns
// the decision; the window owns being able to read what the decision is about.
func (p *reviewPane) Handle(event input.Event) bool {
	if mouse, ok := event.(input.Mouse); ok {
		if handled, _ := p.choosing.Handle(mouse); handled {
			return true
		}
		handled, _ := p.scrolling.Handle(mouse)
		return handled
	}
	if p.form.Handle(event) {
		return true
	}
	return p.window.Handle(event)
}

func (p *reviewPane) Focus(has bool) { p.form.Focus(has) }
