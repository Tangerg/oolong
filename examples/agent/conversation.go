package main

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/examples/internal/docprep"
	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	retainedCompletedItems = 8
	maxMarkdownSourceBytes = 1 << 20
	stickyMinRows          = 1
	stickyGapRows          = 1
)

// conversation composes a stream document with one transcript and one publisher.
// Only receiving source remains revisable; an accepted final capture has a receipt
// and no longer keeps its source bytes alive through the live transcript.
type conversation struct {
	theme      kit.Theme
	glyphs     kit.Glyphs
	wheel      input.Wheel
	publisher  base.TranscriptPublisher
	transcript *base.Transcript
	group      *scene.Group
	render     content.Renderer
	document   *content.StreamDocument
	prepare    *docprep.Queue
	prepareErr error
}

type conversationConfig struct {
	Runtime   *program.Runtime
	Scope     *program.Scope
	Theme     kit.Theme
	Glyphs    kit.Glyphs
	Wheel     input.Wheel
	Publisher base.TranscriptPublisher
}

func newConversation(cfg conversationConfig) (*conversation, error) {
	c := &conversation{
		theme: cfg.Theme, glyphs: cfg.Glyphs, wheel: cfg.Wheel,
		render: markdown.NewRenderer(markdown.RendererConfig{Look: agentMarkdownLook(cfg.Theme, cfg.Glyphs)}),
	}
	c.prepare = docprep.New(docprep.Config{Runtime: cfg.Runtime, Scope: cfg.Scope, Failed: func(err error) { c.prepareErr = errors.Join(c.prepareErr, err) }})
	if cfg.Publisher != nil {
		c.publisher = c.prepare.Publisher(cfg.Publisher)
	}
	c.group = scene.NewGroup(scene.GroupConfig{Overlay: true})
	if err := c.resetView(); err != nil {
		c.prepare.Close()
		return nil, err
	}
	if err := cfg.Runtime.SetFramePreparation(func(width, _ int) (bool, error) { return c.prepare.Ready(width, c.transcript) }); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// resetView installs a fresh transcript. The previous one stays installed when
// construction fails, so a refused reset leaves the conversation usable.
func (c *conversation) resetView() error {
	retention, keep := base.Retain, 0
	if c.publisher != nil {
		retention, keep = base.PublishCompleted, retainedCompletedItems
	}
	view, err := kit.NewTranscript(kit.TranscriptConfig{Behavior: base.TranscriptConfig{Publisher: c.publisher, Retention: retention, PublishPartial: true, KeepCompleted: keep}, Theme: c.theme, Glyphs: c.glyphs})
	if err != nil {
		return err
	}
	transcript := view.Controller()
	transcript.Scroll().Wheel(c.wheel)
	transcript.Sticky().SetMinHeight(stickyMinRows)
	transcript.Sticky().SetGap(stickyGapRows)
	if err := view.Element().SetFocusable(func() bool { return false }); err != nil {
		transcript.Close()
		return err
	}
	if err := c.group.Set(scene.Item{Element: view.Element()}); err != nil {
		transcript.Close()
		return err
	}
	c.transcript = transcript
	return nil
}

func (c *conversation) Element() scene.Element { return c.group.Element() }

func (c *conversation) User(ctx context.Context, prompt string) error {
	if err := c.FinishMarkdown(ctx, nil); err != nil {
		return err
	}
	c.prepareErr = nil
	item, err := c.transcript.Add(&kit.Entry{Theme: c.theme, Label: "you", LabelStyle: c.theme.Accent, Body: prompt})
	if err == nil && item.State() != base.ContentRetired {
		c.transcript.Sticky().Add(item.ID())
	}
	return err
}

func (c *conversation) Append(ctx context.Context, blocks ...base.Block) error {
	if err := c.FinishMarkdown(ctx, nil); err != nil {
		return err
	}
	for _, block := range blocks {
		if _, err := c.transcript.Add(block); err != nil {
			return err
		}
	}
	return nil
}

func (c *conversation) beginMarkdown() (*base.TranscriptItem, error) {
	document, err := content.NewStreamDocument(content.StreamConfig{Format: markdown.Format, MaxSourceBytes: maxMarkdownSourceBytes})
	if err != nil {
		return nil, err
	}
	item, err := c.transcript.AddDocument(document)
	if err == nil {
		c.document = document
	}
	return item, err
}

func (c *conversation) Markdown(ctx context.Context, chunk string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if c.document == nil {
		if _, err := c.beginMarkdown(); err != nil {
			return err
		}
	}
	if err := c.document.Append([]byte(chunk)); err != nil {
		return err
	}
	return c.prepare.Prepare(c.document, c.render, nil) //nolint:contextcheck // ctx governs source admission; the conversation scope owns accepted background preparation.
}

// A provisional projection can recover as the source grows. At EOF the accepted
// final revision owns both source and preparation diagnostics. Closing the source
// does not claim that background preparation has finished; Settled joins it.
func (c *conversation) FinishMarkdown(ctx context.Context, cause error) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	doc := c.document
	if doc == nil {
		return nil
	}
	c.document = nil
	if err := doc.End(cause); err != nil {
		return errors.Join(cause, err)
	}
	prepareErr := c.prepare.Prepare(doc, c.render, func(error) { //nolint:contextcheck // Once EOF is admitted, final preparation belongs to the conversation scope.
		if err := doc.Current().PrepareError; err != nil {
			c.prepareErr = errors.Join(c.prepareErr, err)
		}
	})
	if prepareErr == nil {
		return nil
	}
	// Refusal transfers no work to the queue. The admitted EOF still needs its
	// own final result, even when the caller's lifetime has already ended.
	failed := doc.Snapshot().Prepare(context.Background(), func(context.Context, string) (grid.Drawable, error) { return nil, prepareErr }) //nolint:contextcheck // This records a refusal synchronously; it starts no cancellable work.
	return errors.Join(prepareErr, doc.Accept(failed))
}

func (c *conversation) Settled(receive func(error)) error {
	return c.prepare.After(func() { receive(c.prepareErr) })
}

func (c *conversation) Reset() error {
	previous := c.transcript
	if err := c.resetView(); err != nil {
		return err
	}
	c.prepare.Clear()
	previous.Close()
	c.document, c.prepareErr = nil, nil
	return nil
}

func (c *conversation) Close() {
	c.prepare.Close()
	c.transcript.Close()
}

func agentMarkdownLook(theme kit.Theme, glyphs kit.Glyphs) markdown.Look {
	look := markdownlook.New(theme, glyphs)
	look.Struck, look.Block = theme.Muted, theme.Sunken
	return look
}
