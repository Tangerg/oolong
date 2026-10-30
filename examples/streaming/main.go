// Command streaming is Oolong's canonical streaming interface.
//
// One path exercises the architecture rather than displaying its pieces beside one
// another: a background producer writes through bounded ingress, a bounded source
// document stays revisable until EOF, scoped workers prepare its immutable snapshots,
// and the final content transfers to terminal scrollback through a receipt. A compound
// dialog owns approval. Scene owns every coordinate and focus transition.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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
		chat, err := newChat(context.Background(), runtime)
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
	send        keymap.Action = "send"
	cancelReply keymap.Action = "cancel reply"
	quit        keymap.Action = "quit"

	ingressLimit = 64 << 10
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
	keys    *keymap.Map

	content      *base.Transcript
	view         *kit.Transcript
	composer     *kit.Composer
	status       kit.Status
	element      scene.Element
	confirm      *base.Confirm
	form         *base.Form
	dialog       *kit.Dialog
	pending      string
	document     *content.StreamDocument
	item         *base.TranscriptItem
	render       content.Renderer
	prepareScope *program.Scope

	ingress *program.ByteIngress
	cancel  context.CancelFunc
	active  bool
}

func newChat(ctx context.Context, runtime *program.InlineRuntime) (*chat, error) {
	return newChatWithSource(ctx, runtime, defaultReplySource)
}

func newChatWithSource(ctx context.Context, runtime *program.InlineRuntime, source replySource) (*chat, error) {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	keys := base.DefaultEditorKeys()
	keys.Bind(send, input.Chord{Code: input.Enter})
	keys.Bind(cancelReply, input.Ctrl.Rune('x'))
	keys.Bind(quit, input.Ctrl.Rune('c'))

	scope, err := runtime.NewScope(program.ScopeConfig{Context: ctx}) //nolint:contextcheck // ScopeConfig passes ctx to the session lifetime owner.
	if err != nil {
		return nil, err
	}
	c := &chat{
		scope:   scope,
		runtime: runtime,
		source:  source,
		theme:   theme,
		glyphs:  glyphs,
		keys:    keys,
		status:  kit.Status{Theme: theme, Glyphs: glyphs, Doing: "ready"},
	}
	c.view, err = kit.NewTranscript(kit.TranscriptConfig{
		Theme: theme, Glyphs: glyphs,
		Publisher: runtime.Publisher(), Retention: base.PublishCompleted, PublishPartial: true,
	})
	if err != nil {
		scope.Close()
		return nil, err
	}
	c.content = c.view.Controller()
	c.content.Scroll().Wheel(runtime.Environment().Wheel())
	if err := c.view.Element().SetFocusable(func() bool { return false }); err != nil {
		panic(err)
	}
	c.composer = kit.NewComposer(kit.ComposerConfig{
		Theme:  theme,
		Prompt: glyphs.Marker + " ",
		Hints:  []keymap.Action{send, base.InsertNewline, cancelReply, quit},
	})
	c.composer.Controller().Placeholder = "Ask something, or press ctrl+c to leave"
	c.composer.Controller().Keys = keys
	c.composer.Controller().Clipboard = runtime.Clipboard()
	c.render = markdown.NewRenderer(markdown.RendererConfig{Look: markdownLook(theme, glyphs)})
	status := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { c.status.Draw(frame.View) }})
	body := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
		{Size: layout.Flex(1), Element: c.view.Element()},
		{Size: layout.Fixed(1), Element: status},
		{Size: layout.Measured(1, 0), Element: c.composer.Element()},
	}})
	c.buildApproval() //nolint:contextcheck // Deferred approval and preparation use c.scope.Context(), derived from ctx above.
	c.element = scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{
		Key:     "streaming",
		Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: c.Handle(event.Input)} }, //nolint:contextcheck // Input actions use the scope created from ctx above.
		Detached: func() {
			c.scope.Close()
			if c.prepareScope != nil {
				c.prepareScope.Close()
			}
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
		Submit: func(id base.SubmissionID) (scene.ActionResult[base.SubmissionID], error) {
			c.settleApproval()
			return scene.ActionResult[base.SubmissionID]{State: scene.Applied, Value: id}, nil
		},
		Cancel: func(base.SubmissionID) error { c.reject(); return nil },
	})
	c.form = dressed.Controller()
	c.dialog = kit.NewDialog(kit.DialogConfig{
		Theme: c.theme, Glyphs: c.glyphs,
		Title: "Approve prompt", Description: "The source starts only after approval.", Body: dressed.Element(),
		Where: layout.Placement{Width: 48, Height: 7, Margin: 1},
	})
}

func (c *chat) Element() scene.Element { return c.element }

func (c *chat) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	action, _ := c.keys.Action(key.Chord())
	if action == quit {
		c.runtime.Quit()
		return true
	}
	if c.dialog.Controller().Open() {
		if key.Code == input.Esc {
			c.reject()
			return true
		}
		return false
	}
	switch action {
	case send:
		c.requestApproval()
		return true
	case cancelReply:
		c.stopReply()
		return true
	default:
		return false
	}
}

func (c *chat) requestApproval() {
	prompt := strings.TrimSpace(c.composer.Controller().Text())
	if prompt == "" || c.active || c.dialog.Controller().Open() {
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
	if !c.confirm.Answer() {
		c.reject()
		return
	}
	prompt := c.pending
	c.pending = ""
	c.dialog.Controller().Dismiss()
	if prompt == "" {
		return
	}
	c.startReply(prompt)
}

func (c *chat) reject() {
	c.pending = ""
	c.dialog.Controller().Dismiss()
}

func (c *chat) startReply(prompt string) {
	c.composer.Controller().Clear()
	c.content.Selection().Clear()
	c.appendFinished(&kit.Entry{Theme: c.theme, Label: "you", LabelStyle: c.theme.Accent, Body: prompt})
	var err error
	c.document, err = content.NewStreamDocument(content.StreamConfig{Format: "markdown", MaxSourceBytes: 1 << 20})
	if err != nil {
		c.setOutcome(err)
		return
	}
	c.item, err = c.content.AddDocument(c.document)
	if err != nil {
		c.setOutcome(err)
		return
	}
	c.active = true
	c.status.Doing = "receiving — ctrl+x cancels"
	_ = c.runtime.Session().SetTitle("receiving")
	ingress, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: c.runtime.Dispatcher(), Limit: ingressLimit, Consume: c.accept})
	if err != nil {
		c.finishReply(err)
		return
	}
	ctx, cancel := context.WithCancel(c.scope.Context())
	c.ingress, c.cancel = ingress, cancel
	go func() { <-ingress.Done(); cancel() }()
	go func() { err := c.source(ctx, prompt, ingress); _ = ingress.CloseWithError(err) }()
	c.element.Invalidate()
}

// The ingress owner appends each accepted batch once. Rendering consumes immutable
// source snapshots in scoped tasks; a newer snapshot revokes an older delivery.
func (c *chat) accept(batch program.ByteBatch) {
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
	c.prepare(false, nil)
}

func (c *chat) prepare(final bool, sourceErr error) {
	if c.prepareScope != nil {
		c.prepareScope.Close()
	}
	scope, err := c.runtime.NewScope(program.ScopeConfig{Context: c.scope.Context()})
	if err != nil {
		c.setOutcome(errors.Join(sourceErr, err))
		return
	}
	c.prepareScope = scope
	document, renderer := c.document, c.render
	snapshot := document.Snapshot()
	_, err = c.runtime.Start(scope, program.Work[content.Prepared]{
		Run: func(ctx context.Context) (content.Prepared, error) { return snapshot.Prepare(ctx, renderer), nil },
		Apply: func(prepared content.Prepared, err error) {
			defer scope.Close()
			if err == nil {
				err = document.Accept(prepared)
			}
			if err != nil {
				c.setOutcome(errors.Join(sourceErr, err))
				return
			}
			if final {
				if prepareErr := document.Current().PrepareError; prepareErr != nil {
					sourceErr = errors.Join(sourceErr, prepareErr)
				}
				c.setOutcome(sourceErr)
			}
		},
	})
	if err != nil {
		scope.Close()
		c.setOutcome(errors.Join(sourceErr, err))
	}
}

func (c *chat) finishReply(err error) {
	if c.document == nil {
		c.setOutcome(err)
		return
	}
	if endErr := c.document.End(err); endErr != nil {
		c.setOutcome(errors.Join(err, endErr))
		return
	}
	c.prepare(true, err)
}

func (c *chat) setOutcome(err error) {
	switch {
	case err == context.Canceled: //nolint:errorlint // Joined cancellation retains its failure diagnostics.
		c.status.Doing = "cancelled"
	case err != nil:
		c.status.Doing = "failed: " + err.Error()
		c.appendFinished(&kit.Entry{Theme: c.theme, Label: "source", Body: err.Error()})
	default:
		c.status.Doing = "complete"
		_ = c.runtime.Session().Notify("the answer is complete")
	}
	if c.cancel != nil {
		c.cancel()
	}
	_ = c.runtime.Session().SetTitle("streaming")
	c.ingress, c.cancel, c.active = nil, nil, false
	c.element.Invalidate()
}

func (c *chat) stopReply() {
	if c.cancel == nil {
		return
	}
	c.status.Doing = "cancelling"
	c.cancel()
	c.element.Invalidate()
}

func (c *chat) appendFinished(block base.Block) {
	if _, err := c.content.Add(block); err != nil {
		c.status.Doing = err.Error()
	}
}

func markdownLook(theme kit.Theme, glyphs kit.Glyphs) markdown.Look {
	return markdown.Look{
		Text:     theme.Text,
		Headings: []grid.Style{theme.Heading, theme.Strong},
		Strong:   theme.Strong,
		Emphasis: grid.Style{Attr: grid.Italic},
		Struck:   grid.Style{Attr: grid.Strike},
		Code:     theme.Info,
		Block:    theme.Muted,
		Link:     theme.Accent,
		Quote:    theme.Muted,
		Rail:     theme.Subtle,
		Marker:   theme.Accent,
		Rule:     theme.Subtle,
		Glyphs: markdown.Glyphs{
			Bullet: glyphs.Bullet, Bar: glyphs.Vertical, Divider: glyphs.Horizontal,
			Checked: glyphs.Taken, Unchecked: glyphs.Free,
		},
	}
}

// defaultReplySource stands in for a remote producer. Its timer exists only while an
// answer is active, and every wait can be cancelled. Tests replace it with deterministic
// sources without replacing any application state or publication logic.
func defaultReplySource(ctx context.Context, prompt string, dst io.Writer) error {
	answer := reply(prompt)
	for _, chunk := range pieces(answer, 13) {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-time.After(24 * time.Millisecond):
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
		"> Ctrl+X cancels the source without confusing cancellation with failure.",
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
