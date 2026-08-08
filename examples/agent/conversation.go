package main

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/markdown"
)

const retainedAgentBlocks = 8

// conversation owns the active, interactive tail of the session. It is an application
// entity rather than a component alias: publication, sticky prompts, streaming tails,
// selection and their shared lifetime are one invariant here.
type conversation struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	wheel  input.Wheel

	content   headless.Transcript
	scroll    headless.Scroll
	selection headless.Selection
	sticky    headless.Sticky
	view      kit.Transcript

	stream  markdown.Stream
	open    *markdown.Doc
	openID  headless.BlockID
	hasOpen bool
}

func newConversation(theme kit.Theme, glyphs kit.Glyphs, wheel input.Wheel) *conversation {
	c := &conversation{theme: theme, glyphs: glyphs, wheel: wheel}
	c.scroll.Wheel(wheel)
	c.sticky.MinHeight, c.sticky.Gap = 1, 1
	c.view = kit.Transcript{
		Content: &c.content, Scroll: &c.scroll, Selection: &c.selection,
		Sticky: &c.sticky, Theme: theme, Glyphs: glyphs,
	}
	c.stream.SetLook(agentMarkdownLook(theme, glyphs))
	return c
}

func (c *conversation) Draw(frame headless.Frame) { c.view.Draw(frame) }

func (c *conversation) Handle(event input.Event) bool { return c.view.Handle(event) }

func (c *conversation) User(prompt string) {
	c.FlushMarkdown()
	id := c.append(kit.Message{Theme: c.theme, Speaker: "you", Body: prompt, Own: true})
	c.sticky.Add(id)
}

func (c *conversation) Append(block headless.Block) {
	c.FlushMarkdown()
	c.append(block)
}

func (c *conversation) append(block headless.Block) headless.BlockID {
	id := c.content.Append(block)
	c.content.Finish(id)
	c.scroll.ToBottom()
	return id
}

func (c *conversation) Markdown(chunk string) {
	if stable := c.stream.Feed(chunk); len(stable) > 0 {
		c.finishOpen(stable)
	}
	c.stageOpen(c.stream.Open())
}

func (c *conversation) FlushMarkdown() {
	if stable := c.stream.Flush(); len(stable) > 0 {
		c.finishOpen(stable)
	}
	c.open, c.hasOpen = nil, false
}

func (c *conversation) finishOpen(blocks []markdown.Block) {
	if len(blocks) == 0 {
		return
	}
	if !c.hasOpen {
		doc := new(markdown.Doc)
		doc.SetBlocks(blocks)
		c.append(doc)
		return
	}
	c.open.SetBlocks(blocks)
	c.content.Changed(c.openID)
	c.content.Finish(c.openID)
	c.open, c.hasOpen = nil, false
}

func (c *conversation) stageOpen(blocks []markdown.Block) {
	if len(blocks) == 0 {
		return
	}
	if c.hasOpen {
		c.open.SetBlocks(blocks)
		c.content.Changed(c.openID)
	} else {
		c.open = new(markdown.Doc)
		c.open.SetBlocks(blocks)
		c.openID = c.content.Append(c.open)
		c.hasOpen = true
	}
	c.scroll.ToBottom()
}

func (c *conversation) Retain(printer kit.Printer) {
	if c.content.Width() <= 0 {
		return
	}
	finished := 0
	for i := range c.content.Len() {
		id := c.content.FirstBlock() + headless.BlockID(i)
		if !c.content.Finished(id) {
			break
		}
		finished++
	}
	if excess := finished - retainedAgentBlocks; excess > 0 {
		c.view.CommitN(printer, excess)
	}
	c.scroll.ToBottom()
}

func (c *conversation) Reset() {
	c.content = headless.Transcript{}
	c.scroll = headless.Scroll{}
	c.scroll.Wheel(c.wheel)
	c.selection = headless.Selection{}
	c.sticky = headless.Sticky{MinHeight: 1, Gap: 1}
	c.stream.Reset()
	c.open, c.hasOpen = nil, false
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
