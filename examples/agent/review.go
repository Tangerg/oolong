package main

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
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

func (*workflow) Measure(int) int { return 6 }

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
	diff  *kit.Diff
	form  *kit.Form
	theme kit.Theme
	title string
}

func (p *reviewPane) Draw(frame headless.Frame) {
	width, height := frame.Size()
	formRows := min(p.form.Measure(width), height)
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(formRows)},
	}))
	kit.Label{Text: p.title, Style: p.theme.Subtle, Ellipsis: "…"}.Draw(rows[0].View)
	p.diff.Draw(rows[1].View)
	p.form.Draw(rows[2])
}

func (p *reviewPane) Handle(event input.Event) bool { return p.form.Handle(event) }

func (p *reviewPane) Focus(has bool) { p.form.Focus(has) }

func (a *agent) buildReview() {
	a.reviewAnswer = true
	a.reviewConfirm = &headless.Confirm{
		Label: "Allow this tool call?", Value: headless.Bind(&a.reviewAnswer),
		Yes: "apply", No: "deny",
	}
	keys := headless.DefaultFormKeys()
	a.reviewForm = headless.NewForm(a.reviewConfirm)
	a.reviewForm.Keys = keys
	a.reviewForm.Done = func() { a.answerReview(a.reviewAnswer) }
	a.reviewForm.GaveUp = func() { a.answerReview(false) }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.theme, Glyphs: a.glyphs, Controller: a.reviewForm,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.reviewPane = reviewPane{
		diff: kit.NewDiff(kit.DiffConfig{Theme: a.theme, Glyphs: a.glyphs, Numbers: true}),
		form: dressed, theme: a.theme,
	}
	a.reviewDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.theme, Glyphs: a.glyphs,
		Title: "Review tool call", Body: &a.reviewPane,
	})
	a.reviewDialog.Panel().Where = layout.Placement{Width: 76, Height: 16, Margin: 1}
}

func (a *agent) openReview(request *reviewRequest) {
	if a.review != nil {
		a.answerReview(false)
	}
	a.conversation.FlushMarkdown()
	a.conversation.Retain(a.runtime)
	a.review = request
	a.reviewAnswer = true
	a.reviewConfirm.Say(true)
	a.reviewPane.diff.SetHunks(diff.Between(request.proposal.Before, request.proposal.After).Hunks(2))
	a.reviewPane.title = request.proposal.Path + " — " + request.proposal.Summary
	a.reviewDialog.Controller().SetDescription(request.proposal.Summary + " — " + request.proposal.Path)
	a.status.Doing = "waiting for tool approval"
	a.reviewDialog.Controller().Show()
}

func (a *agent) answerReview(approved bool) {
	request := a.review
	if request == nil {
		return
	}
	a.review = nil
	a.reviewDialog.Controller().Dismiss()
	request.answer <- approved
	if approved {
		a.status.Doing = "applying approved change"
		return
	}
	a.status.Doing = "tool call denied"
	a.conversation.Append(&kit.Message{
		Theme: a.theme, Speaker: "tool", Body: request.proposal.Path + " — denied",
	})
}

func (a *agent) showTool(result toolResult) {
	a.conversation.FlushMarkdown()
	a.conversation.Append(&kit.Message{
		Theme: a.theme, Speaker: result.Name, Body: oneLine(result.Summary),
	})
	shown := kit.NewDiff(kit.DiffConfig{
		Theme: a.theme, Glyphs: a.glyphs,
		Hunks: diff.Between(result.Change.Before, result.Change.After).Hunks(2), Numbers: true,
	})
	a.conversation.Append(shown)
	a.conversation.Retain(a.runtime)
}
