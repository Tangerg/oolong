// Command streaming owns source work in a scope so cancellation can settle the
// producer, bounded ingress and accepted output as one lifecycle.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/examples/internal/docprep"
	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/examples/internal/streamtask"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/core/text"
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
		Cancel: func(base.SubmissionID) error { c.dialog.Controller().Dismiss(); return nil },
	})
	c.form = dressed.Controller()
	body := scene.NewGroup(scene.GroupConfig{Overlay: true, Items: []scene.Item{{Element: dressed.Element()}}, Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
		key, ok := event.Input().(input.Key)
		return scene.EventResult{Consumed: ok && key.Down() && c.quitKey(key)}
	}}})
	c.dialog = kit.NewDialog(kit.DialogConfig{
		Theme: c.theme, Glyphs: c.glyphs,
		Title: "Approve prompt", Description: "The source starts only after approval.", Body: body.Element(), Dismissed: func() { c.pending = "" },
		Where: layout.Placement{Width: 48, Height: 7, Margin: 1},
	})
}

func (c *chat) Element() scene.Element { return c.element }

func (c *chat) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	if c.quitKey(key) {
		return true
	}
	action, _ := c.composer.Controller().Keys().ActionFor(key)
	switch action {
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

func (c *chat) requestApproval() {
	prompt := strings.TrimSpace(c.composer.Controller().Text())
	if prompt == "" || c.document != nil || c.dialog.Controller().Open() {
		return
	}
	c.pending = prompt
	if _, err := c.confirm.Request(c.scope.Context(), base.ConfirmRequest{ExpectedRevision: c.confirm.Snapshot().Revision, Value: true}); err != nil {
		c.status.Doing = err.Error()
		c.pending = ""
		return
	}
	c.dialog.Controller().SetDescription("Send “" + clip(prompt) + "” to the source?")
	c.dialog.Controller().Show()
}

func (c *chat) settleApproval() {
	prompt := c.pending
	c.dialog.Controller().Dismiss()
	if c.confirm.Answer() && prompt != "" {
		c.startReply(prompt)
	}
}

func (c *chat) startReply(prompt string) {
	_, err := c.content.Add(&kit.Entry{Theme: c.theme, Label: "you", LabelStyle: c.theme.Accent, Body: prompt})
	if err != nil {
		c.status.Doing = "failed: " + err.Error()
		c.element.Invalidate()
		return
	}
	c.composer.Controller().Clear()
	c.content.Selection().Clear()
	c.document, err = content.NewStreamDocument(content.StreamConfig{Format: "markdown", MaxSourceBytes: maxReplySourceBytes})
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

// defaultReplySource stands in for a remote producer. Its timer exists only while an
// answer is active, and every wait can be cancelled. Tests replace it with deterministic
// sources without replacing any application state or publication logic.
func defaultReplySource(ctx context.Context, prompt string, dst io.Writer) error {
	answer := reply(prompt)
	for _, chunk := range pieces(answer, mockChunkBytes) {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-time.After(mockChunkInterval):
		}
		n, err := io.WriteString(dst, chunk)
		if err != nil {
			return err
		}
		if n != len(chunk) {
			return io.ErrShortWrite
		}
	}
	return nil
}

func reply(prompt string) string {
	return strings.Join([]string{
		"# Streaming first",
		"",
		"You asked **" + clip(prompt) + "**. This answer is arriving as ordered bytes",
		"from a background producer.",
		"",
		"The complete Markdown document stays revisable until EOF. While receiving it remains",
		"selectable here; finalized output moves into the terminal's own scrollback.",
		"",
		"- ingress is bounded and applies backpressure",
		"- provisional content may change; published captures cannot",
		"- source and publication queues have explicit memory budgets",
		"",
		"> Source cancellation remains distinct from failure.",
		"",
		"When this paragraph finishes, the interface becomes idle and owns no timer.",
		"",
	}, "\n")
}

func pieces(whole string, size int) []string {
	var out []string
	for i := 0; i < len(whole); i += size {
		out = append(out, whole[i:min(i+size, len(whole))])
	}
	return out
}

func clip(s string) string {
	const limit = 40
	return text.Truncate(s, limit, "…")
}
