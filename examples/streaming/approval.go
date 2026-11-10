package main

import (
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// The form owns its answer and submission. Dialog owns open state, and scene
// owns modal eligibility and focus restoration.
func (c *chat) buildApproval() {
	c.confirm = base.NewConfirm(base.ConfirmConfig{Label: "Send this prompt?", Initial: true, Yes: "send", No: "cancel"})
	dressed := kit.NewForm(kit.FormConfig{
		Theme: c.theme, Glyphs: c.glyphs,
		Fields: []base.Field{c.confirm}, Keys: base.DefaultFormKeys(),
		Hints: []keymap.Action{base.Submit, base.Cancel},
		Submit: func(base.SubmissionID) (scene.ActionState, error) {
			c.settleApproval()
			return scene.Applied, nil
		},
	})
	c.form = dressed.Controller()
	body := scene.NewGroup(scene.GroupConfig{Overlay: true, Items: []scene.Item{{Element: dressed.Element()}}, Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
		key, ok := event.Input().(input.Key)
		return scene.EventResult{Consumed: ok && c.quitKey(key)}
	}}})
	c.dialog = kit.NewDialog(kit.DialogConfig{
		Theme: c.theme, Glyphs: c.glyphs,
		Title: "Approve prompt", Description: "The source starts only after approval.", Body: body.Element(), Dismissed: func() { c.takePending() },
		Where: layout.Placement{Width: 48, Height: 7, Margin: 1},
	})
}

func (c *chat) requestApproval() {
	prompt := strings.TrimSpace(c.composer.Controller().Text())
	if prompt == "" || c.document != nil || c.dialog.Controller().Open() {
		return
	}
	c.pending = prompt
	if _, err := c.confirm.Request(c.scope.Context(), base.ConfirmRequest{ExpectedRevision: c.confirm.Snapshot().Revision, Value: true}); err != nil {
		c.status.Doing = err.Error()
		c.takePending()
		return
	}
	c.dialog.Controller().SetDescription("Send “" + clip(prompt) + "” to the source?")
	c.dialog.Controller().Show()
}

func (c *chat) settleApproval() {
	prompt := c.takePending()
	c.dialog.Controller().Dismiss()
	if c.confirm.Answer() && prompt != "" {
		c.startReply(prompt)
	}
}

// takePending releases the prompt awaiting approval. Every path that closes the
// approval takes it first, so the dismissal callback cannot clear a prompt that is
// being sent.
func (c *chat) takePending() string {
	prompt := c.pending
	c.pending = ""
	return prompt
}
