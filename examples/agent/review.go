package main

import (
	"context"
	"errors"
	"image"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

var errReviewPending = errors.New("another tool review is pending")

var reviewPlacement = layout.Placement{Width: 76, Height: 16, Margin: 1}

// reviewRequest is one backend question waiting for a human decision. The UI owner
// settles it exactly once, after the agent has released it; the waiting backend
// reads the decision only after done closes.
type reviewRequest struct {
	ctx      context.Context //nolint:containedctx // A review is only actionable during its issuing request.
	proposal changeProposal
	done     chan struct{}
	approved bool
	err      error
}

func newReviewRequest(ctx context.Context, proposal changeProposal) *reviewRequest {
	return &reviewRequest{ctx: ctx, proposal: proposal, done: make(chan struct{})}
}

func (r *reviewRequest) settle(approved bool, err error) {
	r.approved, r.err = approved, err
	close(r.done)
}

// reviewPane shows the proposal under review above the decision form.
type reviewPane struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	proposal changeProposal
	diff     *kit.Diff
	// A review must remain readable when the diff exceeds terminal height; a
	// viewport prevents approval from depending on content the user cannot reach.
	window  *base.Viewport
	form    *kit.Form
	element scene.Element
}

// The pane keeps keyboard focus on the form, so the window is not focusable and
// receives only the page keys, which the form does not use.
func reviewWindowKeys() *keymap.Map {
	keys := &keymap.Map{}
	keys.Bind(base.ScrollPageUp, input.Chord{Code: input.PageUp})
	keys.Bind(base.ScrollPageDown, input.Chord{Code: input.PageDown})
	return keys
}

func newReviewPane(theme kit.Theme, glyphs kit.Glyphs, form *kit.Form, sessionKey func(input.Key) bool) (*reviewPane, error) {
	p := &reviewPane{theme: theme, glyphs: glyphs, form: form}
	p.diff = kit.NewDiff(kit.DiffConfig{Theme: theme, Glyphs: glyphs, Numbers: true})
	change := scene.NewElement(scene.ElementConfig{
		Paint:   func(frame scene.Frame) { p.diff.Draw(frame.View) },
		Measure: scene.MeasureRows(p.diff.HeightForWidth),
	})
	p.window = base.NewViewport(base.ViewportConfig{Content: change, Keys: reviewWindowKeys()})
	if err := p.window.Element().SetFocusable(func() bool { return false }); err != nil {
		return nil, err
	}
	title := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		kit.Label{Text: p.proposal.title(), Style: p.theme.Subtle, Ellipsis: p.glyphs.Ellipsis}.Draw(frame.View)
	}})
	p.element = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: title}, {Element: p.window.Element()}, {Element: p.form.Element()}},
		Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
			key, ok := event.Input().(input.Key)
			if !ok || !key.Down() {
				return scene.EventResult{}
			}
			if sessionKey(key) {
				return scene.EventResult{Consumed: true}
			}
			if action, ok := p.window.Keys().ActionFor(key); ok {
				return scene.EventResult{Consumed: p.window.Do(action)}
			}
			return scene.EventResult{}
		}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			// The change keeps a row even on a short terminal: approval must never
			// be asked for a change that is not on the screen.
			const titleRows, minChangeRows = 1, 1
			formRows := min(p.form.HeightForWidth(space.X), max(space.Y-titleRows-minChangeRows, 0))
			return (layout.Flow{Axis: layout.Down}).Rects(space, []layout.Slot{{Size: layout.Fixed(titleRows)}, {Size: layout.Flex(1)}, {Size: layout.Fixed(formRows)}})
		},
	}).Element()
	return p, nil
}

func (p *reviewPane) show(proposal changeProposal) {
	p.proposal = proposal
	p.diff.SetHunks(proposal.hunks())
	p.window.Scroll().ToTop()
}

func (a *agent) buildReview() error {
	a.reviewConfirm = base.NewConfirm(base.ConfirmConfig{Label: "Allow this tool call?", Initial: true, Yes: "apply", No: "deny"})
	form := kit.NewForm(kit.FormConfig{
		Theme: a.theme, Glyphs: a.glyphs,
		Fields: []base.Field{a.reviewConfirm}, Keys: base.DefaultFormKeys(),
		Hints: []keymap.Action{base.Submit, base.Cancel},
		Submit: func(base.SubmissionID) (scene.ActionState, error) {
			if err := a.answerReview(a.reviewConfirm.Answer()); err != nil {
				return scene.Unchanged, err
			}
			return scene.Applied, nil
		},
	})
	pane, err := newReviewPane(a.theme, a.glyphs, form, a.sessionKey)
	if err != nil {
		return err
	}
	a.reviewPane = pane
	a.reviewDialog = kit.NewDialog(kit.DialogConfig{
		Theme: a.theme, Glyphs: a.glyphs,
		Dismissed: func() {
			if err := a.answerReview(false); err != nil {
				a.finishRun(err)
			}
		},
		Title: "Review tool call", Body: pane.element, Where: reviewPlacement,
	})
	return nil
}

func (a *agent) openReview(request *reviewRequest) error {
	if a.review != nil {
		return errReviewPending
	}
	if err := a.conversation.FinishMarkdown(a.scope.Context(), nil); err != nil {
		return err
	}
	if _, err := a.reviewConfirm.Request(a.scope.Context(), base.ConfirmRequest{ExpectedRevision: a.reviewConfirm.Snapshot().Revision, Value: true}); err != nil {
		return err
	}
	// Reserve withdrawal delivery before presentation so a full ordinary dispatch
	// queue cannot leave an expired proposal visible or its backend waiting.
	if _, err := a.runtime.Start(a.scope, program.Work[struct{}]{
		Run: func(ctx context.Context) (struct{}, error) {
			select {
			case <-request.done:
				return struct{}{}, nil
			case <-request.ctx.Done():
				return struct{}{}, context.Cause(request.ctx) //nolint:contextcheck // The proposal expires independently of the delivery worker.
			case <-ctx.Done():
				return struct{}{}, context.Cause(ctx)
			}
		},
		Apply: func(_ struct{}, cause error) {
			if cause != nil && a.review == request {
				a.withdrawReview(cause)
			}
		},
	}); err != nil {
		return err
	}
	a.review = request
	a.reviewPane.show(request.proposal)
	a.reviewDialog.Controller().SetDescription(request.proposal.title())
	a.inform("waiting for tool approval")
	a.reviewDialog.Controller().Show()
	return nil
}

func (a *agent) answerReview(approved bool) error {
	request := a.review
	if request == nil {
		return nil
	}
	if cause := context.Cause(request.ctx); cause != nil {
		a.withdrawReview(cause)
		return nil
	}
	a.releaseReview()
	if approved {
		a.inform("applying approved change")
		request.settle(true, nil)
		return nil
	}
	a.inform("tool call denied")
	err := a.conversation.Append(a.scope.Context(), &kit.Entry{
		Theme: a.theme, Label: "tool", Body: request.proposal.Path + " — denied",
	})
	request.settle(false, err)
	return err
}

func (a *agent) withdrawReview(cause error) {
	request := a.review
	if request == nil {
		return
	}
	a.releaseReview()
	a.inform("tool review cancelled")
	request.settle(false, cause)
}

// Dismissal notifies its owner synchronously. Release the request before that
// callback can turn a completed or cancelled operation into a second decision.
func (a *agent) releaseReview() {
	a.review = nil
	a.reviewDialog.Controller().Dismiss()
}

func (a *agent) showTool(result toolResult) error {
	entry := &kit.Entry{Theme: a.theme, Label: result.Name, Body: strings.Join(strings.Fields(result.Summary), " ")}
	change := kit.NewDiff(kit.DiffConfig{Theme: a.theme, Glyphs: a.glyphs, Hunks: result.Change.hunks(), Numbers: true})
	return a.conversation.Append(a.scope.Context(), entry, change)
}
