// Command agent is a complete mock coding-agent session.
//
// It deliberately uses no model SDK and changes no files. The point is the interface
// contract around that work: ordered streaming bytes cross a bounded ingress, stable
// output moves to terminal scrollback, a live plan remains small, a proposed tool call
// blocks on an explicit diff review, and cancellation settles every background edge.
package main

import (
	"context"
	"errors"
	"fmt"
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
)

const (
	sendPrompt  keymap.Action = "send"
	cancelRun   keymap.Action = "cancel-run"
	quitAgent   keymap.Action = "quit"
	ingressSize               = 64 << 10
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Inline: func(runtime *program.InlineRuntime) program.Component {
			return headless.NewRoot(newAgent(runtime, mockBackend{delay: 18 * time.Millisecond}))
		},
		Terminal: term.Options{Probe: true, Mouse: true},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "agent:", err)
		os.Exit(1)
	}
}

type agent struct {
	runtime *program.InlineRuntime
	backend agentBackend
	theme   kit.Theme
	glyphs  kit.Glyphs
	keys    *keymap.Map

	conversation *conversation
	workflow     workflow
	status       kit.Status
	composer     kit.Composer
	commands     headless.Commands
	completion   headless.Completion
	body         *headless.Container
	stack        headless.Stack

	review        *reviewRequest
	reviewAnswer  bool
	reviewConfirm *headless.Confirm
	reviewForm    *headless.Form
	reviewPane    reviewPane
	reviewDialog  *kit.Dialog

	model     string
	run       *agentRun
	stopClock func()
	started   time.Time
}

func newAgent(runtime *program.InlineRuntime, backend agentBackend) *agent {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(os.Getenv)
	keys := headless.DefaultEditorKeys()
	keys.Bind(sendPrompt, input.Chord{Code: input.Enter})
	keys.Bind(cancelRun, input.Ctrl.Rune('x'))
	keys.Bind(quitAgent, input.Ctrl.Rune('c'))

	a := &agent{
		runtime: runtime,
		backend: backend,
		theme:   theme,
		glyphs:  glyphs,
		keys:    keys,
		model:   "oolong-mock-1",
		status:  kit.Status{Theme: theme, Glyphs: glyphs, Doing: "ready"},
	}
	a.conversation = newConversation(theme, glyphs, runtime.Environment().Wheel())
	a.workflow = newWorkflow(theme, glyphs)
	a.composer = kit.Composer{
		Theme:       theme,
		Prompt:      glyphs.Marker + " ",
		Placeholder: "Ask the mock agent to change something",
		Keys:        keys,
		Hints:       []keymap.Action{sendPrompt, cancelRun, quitAgent},
		MaxRows:     5,
	}
	a.composer.Editor().Clipboard = runtime.Clipboard()
	completionKeys := headless.DefaultCompletionKeys()
	completionKeys.Bind(headless.Accept, input.Chord{Code: input.Enter})
	a.completion = headless.Completion{
		Look: theme.Look(glyphs), Keys: completionKeys,
		Accept: func(candidate headless.Candidate, token headless.Token) {
			a.composer.Editor().Replace(token.Start, token.End, candidate.Text)
		},
	}
	a.registerCommands()

	a.body = headless.Rows(
		headless.Item{Size: layout.Flex(1), Of: a.conversation},
		headless.Item{Size: layout.Fixed(a.workflow.Measure(0)), Of: headless.Static{Of: &a.workflow}},
		headless.Item{Size: layout.Fixed(1), Of: headless.Static{Of: agentStatus{owner: a}}},
		headless.Item{Size: layout.Measured(1, 0), Of: &a.composer},
	)
	a.body.Focus(true)
	a.stack.SetBase(a.body)
	a.buildReview()
	runtime.Session().SetTitle("agent mock")
	return a
}

func (a *agent) Draw(frame headless.Frame) {
	a.stack.Draw(frame)
	if a.stack.Empty() && a.completion.Open() {
		a.drawCompletion(frame)
	}
}

func (a *agent) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		action, _ := a.keys.Action(key.Chord())
		switch action {
		case quitAgent:
			a.runtime.Quit()
			return true
		case cancelRun:
			if a.run != nil {
				a.stopRun()
				return true
			}
		}
	}

	if !a.stack.Empty() {
		return a.stack.Handle(event)
	}
	if a.completion.Handle(event) {
		return true
	}
	if key, ok := event.(input.Key); ok && key.Down() && key.Code == input.Enter && key.Mods == 0 {
		a.submit()
		return true
	}
	handled := a.stack.Handle(event)
	if handled {
		a.refreshCompletion()
	}
	return handled
}

func (a *agent) submit() {
	line := strings.TrimSpace(a.composer.Text())
	if line == "" {
		return
	}
	if name, arg, command := headless.Parse(line); command {
		a.composer.Reset()
		a.completion.Dismiss()
		a.runCommand(name, arg)
		return
	}
	if a.run != nil {
		a.status.Doing = "finish or cancel the current run first"
		return
	}
	a.composer.Reset()
	a.completion.Dismiss()
	a.startRun(line)
}

func (a *agent) registerCommands() {
	a.commands.Add(headless.Command{
		Name: "help", Title: "show the commands available in this session",
		Run: func(string) {
			a.conversation.Append(kit.Message{
				Theme: a.theme, Speaker: "commands",
				Body: "/clear — release the live transcript\n/model <fast|careful> — choose the mock plan\n/quit — leave",
			})
		},
	})
	a.commands.Add(headless.Command{
		Name: "clear", Title: "release the live transcript",
		Run: func(string) {
			if a.run != nil {
				a.status.Doing = "the active run owns the transcript"
				return
			}
			a.conversation.Reset()
			a.workflow.Reset()
			a.status.Doing = "cleared"
		},
	})
	a.commands.Add(headless.Command{
		Name: "model", Title: "choose fast or careful planning", Takes: true,
		Run: func(arg string) {
			switch arg {
			case "fast", "careful":
				a.model = arg
				a.status.Doing = "model: " + arg
			default:
				a.status.Doing = "usage: /model fast|careful"
			}
		},
	})
	a.commands.Add(headless.Command{
		Name: "quit", Title: "leave the agent", Aliases: []string{"exit"},
		Run: func(string) { a.runtime.Quit() },
	})
}

func (a *agent) runCommand(name, arg string) {
	command, ok := a.commands.Lookup(name)
	if !ok || command.Run == nil {
		a.status.Doing = "unknown command: /" + name
		return
	}
	if command.Takes && arg == "" {
		a.status.Doing = "/" + command.Name + " needs an argument"
		return
	}
	a.commands.Used(command.Name)
	command.Run(arg)
}

func (a *agent) refreshCompletion() {
	lines := strings.Split(a.composer.Text(), "\n")
	line, column := a.composer.Editor().Cursor()
	if line < 0 || line >= len(lines) {
		a.completion.Dismiss()
		return
	}
	token, ok := headless.TokenAt(lines[line], column, headless.Trigger{Prefix: "/", AtStart: true})
	if !ok {
		a.completion.Dismiss()
		return
	}
	found := a.commands.Find(token.Query)
	candidates := make([]headless.Candidate, 0, len(found))
	for _, match := range found {
		candidates = append(candidates, headless.Candidate{
			Text: match.Command.Name, Label: match.Command.Name,
			Detail: match.Command.Title, Matched: match.At,
		})
	}
	a.completion.Offer(token, candidates)
}

func (a *agent) drawCompletion(frame headless.Frame) {
	width, height := frame.Size()
	rows := a.completion.Measure(width)
	if width <= 2 || rows <= 0 || height <= 2 {
		return
	}
	box := kit.Box{
		Theme: a.theme, Glyphs: a.glyphs,
		Padding: layout.Symmetric(0, 1), Title: "commands", Footer: "enter complete",
		FooterAlign: layout.End,
	}
	popupWidth := min(max(a.completion.Width()+4, 32), width-2)
	popupHeight := min(rows+2, height)
	composerRows := a.composer.Measure(width)
	y := max(height-composerRows-popupHeight, 0)
	area := grid.Rect(1, y, popupWidth, popupHeight)
	inner := box.InnerRect(area.Size())
	box.Draw(frame.View.Sub(area))
	a.completion.Draw(frame.Sub(area).Sub(inner))
}

func (a *agent) startRun(prompt string) {
	a.conversation.User(prompt)
	a.workflow.Reset()
	a.status.Doing = "planning with " + a.model
	a.started = time.Now()
	a.stopClock = a.runtime.Every(120*time.Millisecond, a.tick)

	ingress, err := program.NewByteIngress(a.runtime.Dispatcher(), ingressSize, a.accept)
	if err != nil {
		a.finishRun(err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &agentRun{cancel: cancel}
	a.run = run
	output := &agentBridge{
		ingress: ingress, dispatch: a.runtime.Dispatcher(), owner: a, run: run,
	}

	go func() {
		<-ingress.Done()
		cancel()
	}()
	go func() {
		err := a.backend.Run(ctx, prompt, output)
		_ = ingress.CloseWithError(err)
	}()
	a.conversation.Retain(a.runtime)
}

func (a *agent) accept(batch program.ByteBatch) {
	if len(batch.Data) > 0 {
		a.conversation.Markdown(string(batch.Data))
		a.conversation.Retain(a.runtime)
	}
	if batch.Final {
		a.finishRun(batch.Err)
	}
}

func (a *agent) finishRun(err error) {
	a.conversation.FlushMarkdown()
	a.conversation.Retain(a.runtime)
	if a.stopClock != nil {
		a.stopClock()
		a.stopClock = nil
	}
	if a.review != nil {
		a.answerReview(false)
	}
	if a.run != nil {
		a.run.Cancel()
	}
	a.run = nil
	a.status.Elapsed = ""
	switch {
	case errors.Is(err, context.Canceled):
		a.status.Doing = "cancelled"
	case err != nil:
		a.status.Doing = "failed: " + err.Error()
		a.conversation.Append(kit.Message{Theme: a.theme, Speaker: "runtime", Body: err.Error()})
	default:
		a.status.Doing = "complete"
		a.runtime.Session().Notify("mock agent completed")
	}
}

func (a *agent) stopRun() {
	if a.run == nil {
		return
	}
	a.status.Doing = "cancelling"
	a.run.Cancel()
	if a.review != nil {
		a.answerReview(false)
	}
}

func (a *agent) tick() {
	if a.run == nil {
		return
	}
	a.status.Tick()
	a.status.Elapsed = fmt.Sprintf("%4.1fs", time.Since(a.started).Seconds())
}

func clip(s string) string { return text.Truncate(strings.TrimSpace(s), 52, "…") }

type agentRun struct {
	cancel context.CancelFunc
}

func (r *agentRun) Cancel() { r.cancel() }

// agentStatus distinguishes an active operation from an idle outcome. A Status is a
// busy indicator even when its spinner is not ticking, so idle states use plain text
// rather than showing a frozen promise of work.
type agentStatus struct{ owner *agent }

func (agentStatus) Measure(int) int { return 1 }

func (s agentStatus) Draw(view grid.View) {
	if s.owner.run != nil {
		s.owner.status.Draw(view)
		return
	}
	style := s.owner.theme.Muted
	switch {
	case s.owner.status.Doing == "complete":
		style = s.owner.theme.Success
	case s.owner.status.Doing == "cancelled":
		style = s.owner.theme.Warning
	case strings.HasPrefix(s.owner.status.Doing, "failed"):
		style = s.owner.theme.Danger
	}
	kit.Label{Text: s.owner.status.Doing, Style: style, Ellipsis: s.owner.glyphs.Ellipsis}.Draw(view)
}
