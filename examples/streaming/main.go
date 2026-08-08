// Command streaming is Oolong's canonical streaming interface.
//
// One path exercises the architecture rather than displaying its pieces beside one
// another: a background producer writes through bounded ingress, markdown keeps an
// open tail while publishing stable blocks, a recent selectable transcript remains
// live, older stable blocks move to terminal scrollback, and a compound dialog owns
// approval and focus restoration. Resize, cancellation and failure use the same path.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/markdown"
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Inline: func(runtime *program.InlineRuntime) program.Component {
			return headless.NewRoot(newChat(runtime))
		},
		Terminal: term.Options{Probe: true},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "streaming:", err)
		os.Exit(1)
	}
}

// replySource is the application's one background boundary. Implementations may read
// a model, network or process, but must honor ctx and write bytes in source order.
// They know nothing about frames, components, publication or terminal ownership.
type replySource func(ctx context.Context, prompt string, dst io.Writer) error

const (
	send        keymap.Action = "send"
	cancelReply keymap.Action = "cancel reply"
	quit        keymap.Action = "quit"

	ingressLimit     = 64 << 10
	retainedFinished = 4
)

// chat owns the product state and composes the lower-level state machines. Each
// mutable value has one owner: only the program goroutine calls its methods; the
// source goroutine can communicate solely through ByteIngress.
type chat struct {
	runtime *program.InlineRuntime
	source  replySource
	theme   kit.Theme
	glyphs  kit.Glyphs
	keys    *keymap.Map

	content   headless.Transcript
	scroll    headless.Scroll
	selection headless.Selection
	view      kit.Transcript
	composer  kit.Composer
	status    kit.Status
	body      *headless.Container
	stack     headless.Stack

	approval bool
	confirm  *headless.Confirm
	form     *headless.Form
	dialog   *kit.Dialog
	pending  string

	stream  markdown.Stream
	open    *markdown.Doc
	openID  headless.BlockID
	hasOpen bool
	ingress *program.ByteIngress
	cancel  context.CancelFunc
	active  bool
}

func newChat(runtime *program.InlineRuntime) *chat {
	return newChatWithSource(runtime, defaultReplySource)
}

func newChatWithSource(runtime *program.InlineRuntime, source replySource) *chat {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(os.Getenv)
	keys := headless.DefaultEditorKeys()
	keys.Bind(send, input.Chord{Code: input.Enter})
	keys.Bind(cancelReply, input.Ctrl.Rune('x'))
	keys.Bind(quit, input.Ctrl.Rune('c'))

	c := &chat{
		runtime: runtime,
		source:  source,
		theme:   theme,
		glyphs:  glyphs,
		keys:    keys,
		status:  kit.Status{Theme: theme, Glyphs: glyphs, Doing: "ready"},
	}
	c.scroll.Wheel(runtime.Environment().Wheel())
	c.view = kit.Transcript{
		Content: &c.content, Scroll: &c.scroll, Selection: &c.selection,
		Theme: theme, Glyphs: glyphs,
	}
	c.composer = kit.Composer{
		Theme:       theme,
		Prompt:      glyphs.Marker + " ",
		Placeholder: "Ask something, or press ctrl+c to leave",
		Keys:        keys,
		Hints:       []keymap.Action{send, headless.InsertNewline, cancelReply, quit},
	}
	c.composer.Editor().Clipboard = runtime.Clipboard()
	c.stream.SetLook(markdownLook(theme, glyphs))

	c.body = headless.Rows(
		headless.Item{Size: layout.Flex(1), Of: &c.view},
		headless.Item{Size: layout.Fixed(1), Of: headless.Static{Of: &c.status}},
		headless.Item{Size: layout.Measured(1, 0), Of: &c.composer},
	)
	c.stack.Base = c.body
	c.buildApproval()
	runtime.Session().SetTitle("streaming")
	return c
}

// buildApproval composes behavior from headless and appearance from kit. The form
// owns submission, Dialog owns open state and focus transfer, and chat alone decides
// what approval means to the product.
func (c *chat) buildApproval() {
	c.approval = true
	c.confirm = &headless.Confirm{
		Label: "Send this prompt?", Value: headless.Bind(&c.approval), Yes: "send", No: "cancel",
	}
	formKeys := headless.DefaultFormKeys()
	c.form = &headless.Form{
		Fields: []headless.Field{c.confirm},
		Keys:   formKeys,
		Done:   c.settleApproval,
		GaveUp: c.reject,
	}
	dressed := &kit.Form{
		Of: c.form, Theme: c.theme, Glyphs: c.glyphs,
		Keys: formKeys, Hints: []keymap.Action{headless.Submit, headless.Cancel},
	}
	c.dialog = kit.NewDialog(&c.stack, c.theme, c.glyphs, "Approve prompt", dressed)
	c.dialog.Controller.SetDescription("The source starts only after approval.")
	c.dialog.Panel.Where = layout.Placement{Width: 48, Height: 7, Margin: 1}
}

func (c *chat) Draw(frame headless.Frame) { c.stack.Draw(frame) }

func (c *chat) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		action, _ := c.keys.Action(key.Chord())
		if action == quit {
			c.runtime.Quit()
			return true
		}
		// While a modal is open, its form owns Enter and Escape. Offering Enter to the
		// application first would make one keystroke mean both send and approve.
		if c.stack.Empty() {
			switch action {
			case send:
				c.requestApproval()
				return true
			case cancelReply:
				c.stopReply()
				return true
			}
		}
	}
	handled := c.stack.Handle(event)
	if c.pending != "" && !c.dialog.Open() {
		// Stack may dismiss on Escape or a click outside. Dialog has already restored
		// focus; this settles the product intent that had not been approved.
		c.pending = ""
		c.approval = true
	}
	return handled
}

func (c *chat) requestApproval() {
	prompt := strings.TrimSpace(c.composer.Text())
	if prompt == "" || c.active || c.dialog.Open() {
		return
	}
	c.pending = prompt
	c.approval = true
	c.confirm.Say(true)
	c.dialog.Controller.SetDescription("Send “" + clip(prompt) + "” to the source?")
	c.dialog.Show()
}

func (c *chat) settleApproval() {
	if !c.approval {
		c.reject()
		return
	}
	prompt := c.pending
	c.pending = ""
	c.dialog.Dismiss()
	if prompt == "" {
		return
	}
	c.startReply(prompt)
}

func (c *chat) reject() {
	c.pending = ""
	c.approval = true
	c.dialog.Dismiss()
}

func (c *chat) startReply(prompt string) {
	c.composer.Reset()
	c.selection.Clear()
	c.appendFinished(kit.Message{Theme: c.theme, Speaker: "you", Body: prompt, Own: true})
	c.stream.Reset()
	c.open, c.hasOpen = nil, false
	c.active = true
	c.status.Doing = "receiving — ctrl+x cancels"
	c.runtime.Session().SetTitle("receiving")

	ingress, err := program.NewByteIngress(c.runtime.Dispatcher(), ingressLimit, c.accept)
	if err != nil {
		c.finishReply(err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.ingress, c.cancel = ingress, cancel

	// This waiter ties source lifetime to interface ownership. Normal completion,
	// cancellation and runtime teardown all close ingress.Done; no goroutine survives
	// the state machine it serves.
	go func() {
		<-ingress.Done()
		cancel()
	}()
	go func() {
		err := c.source(ctx, prompt, ingress)
		_ = ingress.CloseWithError(err)
	}()
	c.retainWindow()
}

// accept is the sole transition from background bytes into interface state. It is
// always invoked by ByteIngress on the program owner after every earlier byte.
func (c *chat) accept(batch program.ByteBatch) {
	if len(batch.Data) > 0 {
		c.acceptMarkdown(string(batch.Data))
		c.status.Tick()
	}
	if batch.Final {
		c.finishReply(batch.Err)
		return
	}
	c.retainWindow()
}

func (c *chat) acceptMarkdown(chunk string) {
	if stable := c.stream.Feed(chunk); len(stable) > 0 {
		c.finishOpen(stable)
	}
	c.stageOpen(c.stream.Open())
}

// finishOpen converts the existing open tail into one immutable stable block. The
// identity stays the same, so selection and scroll do not jump while ownership changes.
func (c *chat) finishOpen(blocks []markdown.Block) {
	if len(blocks) == 0 {
		return
	}
	if !c.hasOpen {
		doc := new(markdown.Doc)
		doc.SetBlocks(blocks)
		id := c.content.Append(doc)
		c.content.Finish(id)
		return
	}
	c.open.SetBlocks(blocks)
	c.content.Changed(c.openID)
	c.content.Finish(c.openID)
	c.open, c.hasOpen = nil, false
}

func (c *chat) stageOpen(blocks []markdown.Block) {
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

func (c *chat) finishReply(err error) {
	if stable := c.stream.Flush(); len(stable) > 0 {
		c.finishOpen(stable)
	}
	c.open, c.hasOpen = nil, false

	switch {
	case errors.Is(err, context.Canceled):
		c.status.Doing = "cancelled"
	case err != nil:
		c.status.Doing = "failed: " + err.Error()
		c.appendFinished(kit.Message{
			Theme: c.theme, Speaker: "source", Body: err.Error(),
		})
	default:
		c.status.Doing = "complete"
		c.runtime.Session().Notify("the answer is complete")
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.runtime.Session().SetTitle("streaming")
	c.ingress, c.cancel, c.active = nil, nil, false
	c.retainWindow()
}

func (c *chat) stopReply() {
	if c.cancel == nil {
		return
	}
	c.status.Doing = "cancelling"
	c.cancel()
}

func (c *chat) appendFinished(block headless.Block) {
	id := c.content.Append(block)
	c.content.Finish(id)
	c.scroll.ToBottom()
}

// retainWindow deliberately keeps a small recent stable prefix interactive and gives
// only its excess to terminal scrollback. The open tail is never eligible. Memory,
// resize and draw work therefore follow this bound rather than the age of the session.
func (c *chat) retainWindow() {
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
	if excess := finished - retainedFinished; excess > 0 {
		c.view.CommitN(c.runtime, excess)
	}
	c.scroll.ToBottom()
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
		"Stable markdown blocks leave the open tail exactly once. Recent blocks remain",
		"selectable here; older ones move into the terminal's own scrollback.",
		"",
		"- ingress is bounded and applies backpressure",
		"- the open block may change; committed blocks cannot",
		"- resize touches only the deliberately retained window",
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
