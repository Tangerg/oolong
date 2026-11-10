package main

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/examples/internal/streamtask"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/kit"
)

func (c *chat) startReply(prompt string) {
	_, err := c.content.Add(&kit.Entry{Theme: c.theme, Label: "you", LabelStyle: c.theme.Accent, Body: prompt})
	if err != nil {
		c.status.Doing = "failed: " + err.Error()
		c.element.Invalidate()
		return
	}
	c.composer.Controller().Clear()
	c.content.Selection().Clear()
	c.document, err = content.NewStreamDocument(content.StreamConfig{Format: markdown.Format, MaxSourceBytes: maxReplySourceBytes})
	if err != nil {
		c.setOutcome(err)
		return
	}
	c.item, err = c.content.AddDocument(c.document)
	if err != nil {
		c.setOutcome(err)
		return
	}
	c.status.Doing = "receiving"
	_ = c.runtime.Session().SetTitle("receiving")
	document := c.document
	ingress, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: c.runtime.Dispatcher(), Limit: ingressLimit, Consume: func(batch program.ByteBatch) { c.accept(document, batch) }})
	if err != nil {
		c.finishReply(err)
		return
	}
	c.producer = streamtask.Start(c.runtime.Runtime, c.scope, ingress, func(ctx context.Context) error {
		return c.source(ctx, prompt, ingress)
	})
	c.element.Invalidate()
}

// The ingress owner appends each accepted batch once. Rendering consumes immutable
// source snapshots; the preparation queue coalesces pending revisions without
// pretending that cancellation has stopped an executing parser.
func (c *chat) accept(document *content.StreamDocument, batch program.ByteBatch) {
	// Closing an ingress does not withdraw a batch already queued on the UI owner.
	if c.document != document || document.Current().State == content.Finalizing {
		return
	}
	if len(batch.Data) > 0 {
		if err := c.document.Append(batch.Data); err != nil {
			c.finishReply(errors.Join(batch.Err, err))
			return
		}
		c.status.Tick()
	}
	if batch.Final {
		c.finishReply(batch.Err)
		return
	}
	c.prepare()
}

func (c *chat) prepare() {
	document := c.document
	if err := c.preparation.Prepare(document, c.render, func(err error) {
		if c.document == document {
			c.setOutcome(err)
		}
	}); err != nil {
		c.setOutcome(errors.Join(document.Current().SourceError, err))
	}
}

func (c *chat) finishReply(err error) {
	if c.document == nil {
		return
	}
	if endErr := c.document.End(err); endErr != nil {
		c.setOutcome(errors.Join(err, endErr))
		return
	}
	c.status.Doing = "preparing final answer"
	c.prepare()
}

func (c *chat) setOutcome(err error) {
	document := c.document
	c.document = nil
	c.preparation.Clear()
	c.producer.Cancel()
	c.producer = nil
	if document != nil && err != nil {
		// A local preparation failure can finish this reply before its source returns.
		// Settle the original document too: no later task will prepare that revision.
		if state := document.Current().State; state == content.Receiving || state == content.Preparing {
			if endErr := document.End(err); endErr != nil {
				err = errors.Join(err, endErr)
			}
		}
		if document.Current().State == content.Finalizing {
			failed := document.Snapshot().Prepare(c.scope.Context(), func(context.Context, string) (grid.Drawable, error) { return nil, err })
			if acceptErr := document.Accept(failed); acceptErr != nil {
				err = errors.Join(err, acceptErr)
			}
		}
	}

	switch {
	case err == context.Canceled: //nolint:errorlint // Joined cancellation retains its failure diagnostics.
		c.status.Doing = "cancelled"
	case err != nil:
		_, appendErr := c.content.Add(&kit.Entry{Theme: c.theme, Label: "source", Body: err.Error()})
		c.status.Doing = "failed: " + errors.Join(err, appendErr).Error()
	default:
		c.status.Doing = "complete"
		_ = c.runtime.Session().Notify("the answer is complete")
	}
	_ = c.runtime.Session().SetTitle("streaming")
	c.element.Invalidate()
}

func (c *chat) stopSource() {
	if !c.producer.Cancel() {
		return
	}
	c.status.Doing = "cancelling"
	c.element.Invalidate()
}
