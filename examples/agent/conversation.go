package main

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/input"

	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// conversation composes a stream document with one transcript and one publisher.
// Only receiving source remains revisable; an accepted final capture has a receipt
// and no longer keeps its source bytes alive through the live transcript.
type conversation struct {
	theme     kit.Theme
	glyphs    kit.Glyphs
	wheel     input.Wheel
	publisher base.TranscriptPublisher
	content   *base.Transcript
	view      *kit.Transcript
	group     *scene.Group
	render    content.Renderer
	document  *content.StreamDocument
}

func newConversation(theme kit.Theme, glyphs kit.Glyphs, wheel input.Wheel, publisher base.TranscriptPublisher) *conversation {
	c := &conversation{
		theme: theme, glyphs: glyphs, wheel: wheel, publisher: publisher,
		render: markdown.NewRenderer(markdown.RendererConfig{Look: agentMarkdownLook(theme, glyphs)}),
	}
	c.group = scene.NewGroup(scene.GroupConfig{Overlay: true})
	c.resetView()
	return c
}

func (c *conversation) resetView() {
	retention, keep := base.Retain, 0
	if c.publisher != nil {
		retention, keep = base.PublishCompleted, 8
	}
	view, err := kit.NewTranscript(kit.TranscriptConfig{
		Theme: c.theme, Glyphs: c.glyphs,
		Publisher: c.publisher, Retention: retention, PublishPartial: true, KeepCompleted: keep,
	})
	if err != nil {
		panic(err)
	}
	c.view, c.content = view, view.Controller()
	c.content.Scroll().Wheel(c.wheel)
	c.content.Sticky().MinHeight, c.content.Sticky().Gap = 1, 1
	if err := c.view.Element().SetFocusable(func() bool { return false }); err != nil {
		panic(err)
	}
	if err := c.group.Set(scene.Item{Element: c.view.Element()}); err != nil {
		panic(err)
	}
}

func (c *conversation) Element() scene.Element { return c.group.Element() }

func (c *conversation) User(ctx context.Context, prompt string) error {
	if err := c.FinishMarkdown(ctx, nil); err != nil {
		return err
	}
	item, err := c.content.Add(&kit.Entry{Theme: c.theme, Label: "you", LabelStyle: c.theme.Accent, Body: prompt})
	if err == nil && item.State() != base.ContentRetired {
		c.content.Sticky().Add(item.ID())
	}
	return err
}

func (c *conversation) Append(ctx context.Context, blocks ...base.Block) error {
	if err := c.FinishMarkdown(ctx, nil); err != nil {
		return err
	}
	for _, block := range blocks {
		if _, err := c.content.Add(block); err != nil {
			return err
		}
	}
	return nil
}

func (c *conversation) beginMarkdown() (*base.TranscriptItem, error) {
	document, err := content.NewStreamDocument(content.StreamConfig{Format: "markdown", MaxSourceBytes: 1 << 20})
	if err != nil {
		return nil, err
	}
	item, err := c.content.AddDocument(document)
	if err == nil {
		c.document = document
	}
	return item, err
}

func (c *conversation) Markdown(ctx context.Context, chunk string) error {
	if c.document == nil {
		if _, err := c.beginMarkdown(); err != nil {
			return err
		}
	}
	if err := c.document.Append([]byte(chunk)); err != nil {
		return err
	}
	return c.document.Accept(c.document.Snapshot().Prepare(ctx, c.render))
}

// A provisional projection can recover as the source grows. At EOF the accepted
// final revision owns both source and preparation diagnostics.
func (c *conversation) FinishMarkdown(ctx context.Context, cause error) error {
	doc := c.document
	if doc == nil {
		return cause
	}
	c.document = nil
	if err := doc.End(cause); err != nil {
		return errors.Join(cause, err)
	}
	if err := doc.Accept(doc.Snapshot().Prepare(ctx, c.render)); err != nil {
		return errors.Join(cause, err)
	}
	change := doc.Current()
	if change.PrepareError != nil {
		return errors.Join(change.SourceError, change.PrepareError)
	}
	return change.SourceError
}

func (c *conversation) Reset() {
	c.content.Close()
	c.document = nil
	c.resetView()
}

func agentMarkdownLook(theme kit.Theme, glyphs kit.Glyphs) markdown.Look {
	look := markdownlook.New(theme, glyphs)
	look.Struck, look.Block = theme.Muted, theme.Sunken
	return look
}
