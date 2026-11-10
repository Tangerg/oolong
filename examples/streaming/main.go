// Command streaming owns source work in a scope so cancellation can settle the
// producer, bounded ingress and accepted output as one lifecycle.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Tangerg/oolong/examples/internal/docprep"
	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	app, err := program.NewInline(program.InlineConfig{Terminal: term.Features{Probe: true}, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
		chat, err := newChat(runtime)
		if err != nil {
			return scene.Element{}, err
		}
		return chat.Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "streaming:", err)
		os.Exit(1)
	}
}

// replySource is the application's one background boundary. Implementations may read
// a model, network or process, but must honor ctx and write bytes in source order.
// They know nothing about frames, components, publication or terminal ownership.
// Bare context.Canceled reports cancellation; every other error retains its diagnostics.
type replySource func(ctx context.Context, prompt string, dst io.Writer) error

const (
	send         keymap.Action = "send"
	cancelSource keymap.Action = "cancel-source"
	quit         keymap.Action = "quit"

	ingressLimit        = 64 << 10
	maxReplySourceBytes = 1 << 20
	mockChunkBytes      = 13
	mockChunkInterval   = 24 * time.Millisecond
)

// chat owns the product state and composes the lower-level state machines. Each
// mutable value has one owner: only the program goroutine calls its methods; the
// source goroutine can communicate solely through ByteIngress.
type chat struct {
	scope   *program.Scope
	runtime *program.InlineRuntime
	source  replySource
	theme   kit.Theme
	glyphs  kit.Glyphs

	content     *base.Transcript
	view        *kit.Transcript
	composer    *kit.Composer
	status      kit.Status
	element     scene.Element
	confirm     *base.Confirm
	form        *base.Form
	dialog      *kit.Dialog
	pending     string
	document    *content.StreamDocument
	item        *base.TranscriptItem
	render      content.Renderer
	preparation *docprep.Queue

	producer *program.Task
}

func newChat(runtime *program.InlineRuntime) (*chat, error) {
	return newChatWithSource(runtime, defaultReplySource)
}

func newChatWithSource(runtime *program.InlineRuntime, source replySource) (*chat, error) {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	keys := base.DefaultEditorKeys()
	keys.Bind(send, input.Chord{Code: input.Enter})
	keys.Bind(cancelSource, input.Ctrl.Rune('x'))
	keys.Bind(quit, input.Ctrl.Rune('c'))

	scope, err := runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		return nil, err
	}
	c := &chat{
		scope:   scope,
		runtime: runtime,
		source:  source,
		theme:   theme,
		glyphs:  glyphs,
		status:  kit.Status{Theme: theme, Glyphs: glyphs, Doing: "ready"},
	}
	c.preparation = docprep.New(docprep.Config{Runtime: runtime.Runtime, Scope: scope, Failed: c.setOutcome})
	c.view, err = kit.NewTranscript(kit.TranscriptConfig{Behavior: base.TranscriptConfig{Publisher: c.preparation.Publisher(runtime.Publisher()), Retention: base.PublishCompleted, PublishPartial: true}, Theme: theme, Glyphs: glyphs})
	if err != nil {
		scope.Close()
		return nil, err
	}
	c.content = c.view.Controller()
	if err := runtime.SetFramePreparation(func(width, _ int) (bool, error) { return c.preparation.Ready(width, c.content) }); err != nil {
		c.content.Close()
		scope.Close()
		return nil, err
	}
	c.content.Scroll().Wheel(runtime.Environment().Wheel())
	if err := c.view.Element().SetFocusable(func() bool { return false }); err != nil {
		panic(err)
	}
	c.composer = kit.NewComposer(kit.ComposerConfig{
		Theme:       theme,
		Glyphs:      glyphs,
		Prompt:      glyphs.Marker + " ",
		Hints:       []keymap.Action{send, base.InsertNewline, cancelSource, quit},
		Placeholder: "Ask something",
		Keys:        keys,
		Clipboard:   runtime.Clipboard(),
	})
	c.render = markdown.NewRenderer(markdown.RendererConfig{Look: markdownlook.New(theme, glyphs)})
	status := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { c.status.Draw(frame.View) }})
	body := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
		{Size: layout.Flex(1), Element: c.view.Element()},
		{Size: layout.Fixed(1), Element: status},
		{Size: layout.Measured(1, 0), Element: c.composer.Element()},
	}})
	c.buildApproval()
	c.element = scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{
		Key:     "streaming",
		Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: c.Handle(event.Input())} },
		Detached: func() {
			c.scope.Close()
			c.preparation.Close()
			c.content.Close()
		},
	}, Items: []scene.Item{{Element: body.Element()}, {Element: c.dialog.Element()}}}).Element()
	// Optional window metadata has no bearing on source acceptance.
	_ = runtime.Session().SetTitle("streaming")
	return c, nil
}

func (c *chat) Element() scene.Element { return c.element }

func (c *chat) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok {
		return false
	}
	action, _ := c.composer.Controller().Keys().ActionFor(key)
	switch action {
	case quit:
		c.runtime.Quit()
		return true
	case send:
		c.requestApproval()
		return true
	case cancelSource:
		c.stopSource()
		return true
	default:
		return false
	}
}

func (c *chat) quitKey(key input.Key) bool {
	action, _ := c.composer.Controller().Keys().ActionFor(key)
	if action != quit {
		return false
	}
	c.runtime.Quit()
	return true
}
