package main

import (
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// The form owns its answer and submission. Dialog owns open state, and scene
// owns modal eligibility and focus restoration.
func (c *chat) buildApproval() {
	c.confirm = base.NewConfirm(base.ConfirmConfig{Label: "Send this prompt?", Initial: true, Yes: "send", No: "cancel"})
	dressed := kit.NewForm(kit.FormConfig{Behavior: base.FormConfig{Fields: []base.Field{c.confirm}, Keys: base.DefaultFormKeys(), Submit: func(base.SubmissionID) (scene.ActionState, error) {
		c.settleApproval()
		return scene.Applied, nil
	}}, Theme: c.theme, Glyphs: c.glyphs, Hints: []keymap.Action{base.Submit, base.Cancel}})
	c.form = dressed.Controller()
	c.dialog = kit.NewDialog(kit.DialogConfig{Behavior: base.DialogConfig{Title: "Approve prompt", Description: "The source starts only after approval.", Content: dressed.Element(), Dismissed: func() { c.takePending() }}, Theme: c.theme, Glyphs: c.glyphs, Where: layout.Placement{Width: 48, Height: 7, Margin: 1}})
}

func (c *chat) requestApproval() {
	prompt := strings.TrimSpace(c.composer.Controller().Text())
	if prompt == "" || c.document != nil || c.dialog.Controller().Open() {
		return
	}
	c.pending = prompt
	if _, err := c.confirm.Request(c.scope.Context(), base.ConfirmRequest{ExpectedRevision: c.confirm.Snapshot().Revision, Value: true}); err != nil {
		c.status.Label = err.Error()
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
