package main

import (
	"fmt"
	"slices"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
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

func (s stepState) valid() bool { return s <= stepSkipped }

type stepUpdate struct {
	Step  stepID
	State stepState
}

type workflowStep struct {
	id    stepID
	label string
	state stepState
}

// workflow is the run plan the backend advances. Its steps are fixed; a run only
// changes their states, so a stale update can never add or reorder a step.
type workflow struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	steps  []workflowStep
}

func newWorkflow(theme kit.Theme, glyphs kit.Glyphs) workflow {
	return workflow{theme: theme, glyphs: glyphs, steps: []workflowStep{
		{id: stepUnderstand, label: "understand request"},
		{id: stepInspect, label: "inspect ownership"},
		{id: stepReview, label: "review and apply"},
		{id: stepVerify, label: "verify invariants"},
	}}
}

func (w *workflow) Reset() {
	for i := range w.steps {
		w.steps[i].state = stepWaiting
	}
}

func (w *workflow) Apply(update stepUpdate) error {
	index := slices.IndexFunc(w.steps, func(step workflowStep) bool { return step.id == update.Step })
	if index < 0 || !update.State.valid() {
		return fmt.Errorf("invalid plan update for step %q with state %d", update.Step, update.State)
	}
	w.steps[index].state = update.State
	return nil
}

func (w *workflow) HeightForWidth(int) int { return layout.Sum(len(w.steps), w.box().Overhead().Y) }

func (w *workflow) box() kit.Box {
	return kit.Box{Theme: w.theme, Glyphs: w.glyphs, Title: "run plan", Padding: boxPadding}
}

func (w *workflow) appearance(state stepState) (mark, label string, style grid.Style) {
	switch state {
	case stepRunning:
		return w.glyphs.Marker, "running", w.theme.Accent
	case stepDone:
		return w.glyphs.Taken, "done", w.theme.Success
	case stepSkipped:
		return w.glyphs.Bullet, "skipped", w.theme.Muted
	default:
		return w.glyphs.Free, "waiting", w.theme.Subtle
	}
}

func (w *workflow) Draw(view grid.View) {
	inner := w.box().Draw(view)
	width, height := inner.Size()
	for row, step := range w.steps {
		if row >= height {
			return
		}
		mark, label, style := w.appearance(step.state)
		end := inner.Text(0, row, mark+" "+step.label, style)
		// The state label is right-aligned and only drawn with a column of space
		// before it, so it never runs into the step it describes.
		if labelWidth := text.Width(label); width-end > labelWidth+1 {
			inner.Text(width-labelWidth, row, label, style)
		}
	}
}
