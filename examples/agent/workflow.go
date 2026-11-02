package main

import (
	"fmt"
	"slices"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/kit"
)

type stepID string

const (
	stepUnderstand stepID = "understand"
	stepInspect    stepID = "inspect"
	stepReview     stepID = "review"
	stepVerify     stepID = "verify"
)

type stepState uint8

const (
	stepWaiting stepState = iota
	stepRunning
	stepDone
	stepSkipped
)

type stepUpdate struct {
	Step  stepID
	State stepState
}

type workflowStep struct {
	id    stepID
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
		{id: stepUnderstand, label: "understand request", state: stepWaiting},
		{id: stepInspect, label: "inspect ownership", state: stepWaiting},
		{id: stepReview, label: "review and apply", state: stepWaiting},
		{id: stepVerify, label: "verify invariants", state: stepWaiting},
	}
}

func (w *workflow) Apply(update stepUpdate) error {
	index := slices.IndexFunc(w.steps, func(step workflowStep) bool { return step.id == update.Step })
	if index < 0 || update.State > stepSkipped {
		return fmt.Errorf("invalid plan update for step %q with state %d", update.Step, update.State)
	}
	w.steps[index].state = update.State
	return nil
}

func (w *workflow) HeightForWidth(int) int { return layout.Sum(len(w.steps), w.box().Overhead().Y) }

func (w *workflow) box() kit.Box {
	return kit.Box{
		Theme: w.theme, Glyphs: w.glyphs, Title: "run plan",
		Padding: layout.Symmetric(0, 1),
	}
}

func (w *workflow) Draw(view grid.View) {
	inner := w.box().Draw(view)
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
