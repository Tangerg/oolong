package main

import (
	"context"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/publication"
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
	publisher *publication.Publisher
	content   *base.Transcript
	view      *kit.Transcript
	group     *scene.Group
	render    content.Renderer
	document  *content.StreamDocument
	item      *base.TranscriptItem
	problem   error
}

func newConversation(theme kit.Theme, glyphs kit.Glyphs, wheel input.Wheel, publisher *publication.Publisher) *conversation {
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
func (c *conversation) Err() error             { return c.problem }

func (c *conversation) User(ctx context.Context, prompt string) {
	c.FlushMarkdown(ctx)
	item := c.append(&kit.Entry{Theme: c.theme, Label: "you", LabelStyle: c.theme.Accent, Body: prompt})
	if item != nil && item.State() != base.ContentRetired {
		c.content.Sticky().Add(item.ID())
	}
}

func (c *conversation) Append(ctx context.Context, block base.Block) {
	c.FlushMarkdown(ctx)
	c.append(block)
}

func (c *conversation) append(block base.Block) *base.TranscriptItem {
	item, err := c.content.Add(block)
	if err != nil {
		c.problem = err
	}
	return item
}

func (c *conversation) Markdown(ctx context.Context, chunk string) {
	if c.problem != nil {
		return
	}
	if c.document == nil {
		var err error
		c.document, err = content.NewStreamDocument(content.StreamConfig{Format: "markdown", MaxSourceBytes: 1 << 20})
		if err != nil {
			c.problem = err
			return
		}
		c.item, err = c.content.AddDocument(c.document)
		if err != nil {
			c.problem = err
			return
		}
	}
	if err := c.document.Append([]byte(chunk)); err != nil {
		c.problem = err
		return
	}
	c.prepare(ctx)
}

func (c *conversation) prepare(ctx context.Context) {
	// The mock source is deliberately small. Streaming's companion example puts
	// this same immutable Prepare result through a scoped background worker.
	if err := c.document.Accept(c.document.Snapshot().Prepare(ctx, c.render)); err != nil {
		c.problem = err
	}
}

// EOF is explicit even when the final projection is empty, so a hidden HTML
// comment cannot strand an unfinished item ahead of subsequent output.
func (c *conversation) FlushMarkdown(ctx context.Context) { c.FinishMarkdown(ctx, nil) }

func (c *conversation) FinishMarkdown(ctx context.Context, cause error) {
	if c.document == nil {
		return
	}
	if err := c.document.End(cause); err != nil {
		c.problem = err
		return
	}
	c.prepare(ctx)
	c.document = nil
}

func (c *conversation) Reset() {
	c.content.Close()
	c.document, c.item, c.problem = nil, nil, nil
	c.resetView()
}

func agentMarkdownLook(theme kit.Theme, glyphs kit.Glyphs) markdown.Look {
	return markdown.Look{
		Text: theme.Text, Headings: []grid.Style{theme.Heading, theme.Strong},
		Strong: theme.Strong, Emphasis: grid.Style{Attr: grid.Italic},
		Struck: theme.Muted, Code: theme.Info, Block: theme.Sunken,
		Link: theme.Accent, Quote: theme.Muted, Rail: theme.Subtle,
		Marker: theme.Accent, Rule: theme.Divider,
		Glyphs: markdown.Glyphs{
			Bullet: glyphs.Bullet, Bar: glyphs.Vertical, Divider: glyphs.Horizontal,
			Checked: glyphs.Taken, Unchecked: glyphs.Free,
		},
	}
}
