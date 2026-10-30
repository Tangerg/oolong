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
	"image"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/diff"
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

const (
	sendPrompt  keymap.Action = "send"
	cancelRun   keymap.Action = "cancel-run"
	quitAgent   keymap.Action = "quit"
	ingressSize               = 64 << 10
)

func main() {
	app, err := program.NewInline(program.InlineConfig{Terminal: term.Features{Probe: true, Mouse: true}, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
		agent, err := newAgent(context.Background(), runtime, mockBackend{delay: 18 * time.Millisecond})
		if err != nil {
			return scene.Element{}, err
		}
		return agent.Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
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
	composer     *kit.Composer
	commands     base.Commands[commandAction]
	completion   base.Completion
	body         *scene.Group
	element      scene.Element
	scope        *program.Scope

	review        *reviewRequest
	reviewConfirm *base.Confirm
	reviewForm    *base.Form
	reviewPane    reviewPane
	reviewDialog  *kit.Dialog

	model             string
	run               *agentRun
	stopClock         func()
	stopProgressReset func()
	started           time.Time
}

type commandAction struct {
	takesArgument bool
	run           func(string)
}

func newAgent(ctx context.Context, runtime *program.InlineRuntime, backend agentBackend) (*agent, error) {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	keys := base.DefaultEditorKeys()
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
	var err error
	a.scope, err = runtime.NewScope(program.ScopeConfig{Context: ctx}) //nolint:contextcheck // ScopeConfig explicitly carries ctx; the analyser does not track context through config fields.
	if err != nil {
		return nil, err
	}
	a.conversation = newConversation(theme, glyphs, runtime.Environment().Wheel(), runtime.Publisher())
	a.workflow = newWorkflow(theme, glyphs)
	a.composer = kit.NewComposer(kit.ComposerConfig{
		Theme:   theme,
		Prompt:  glyphs.Marker + " ",
		Hints:   []keymap.Action{sendPrompt, cancelRun, quitAgent},
		MaxRows: 5,
	})
	a.composer.Controller().Placeholder = "Ask the mock agent to change something"
	a.composer.Controller().Keys = keys
	a.composer.Controller().Clipboard = runtime.Clipboard()
	completionKeys := base.DefaultCompletionKeys()
	completionKeys.Bind(base.Accept, input.Chord{Code: input.Enter})
	a.completion = base.Completion{
		Look: theme.Look(glyphs), Keys: completionKeys,
		Accept: func(candidate base.Candidate, token base.Token) {
			a.composer.Controller().Replace(token.Start, token.End, candidate.Text)
			a.completion.Dismiss()
		},
	}
	a.registerCommands() //nolint:contextcheck // Deferred UI actions use a.scope.Context(), derived from ctx above, after this constructor returns.

	workflow := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { a.workflow.Draw(frame.View) }})
	status := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { agentStatus{owner: a}.Draw(frame.View) }})
	a.body = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
		{Size: layout.Flex(1), Element: a.conversation.Element()},
		{Size: layout.Fixed(a.workflow.HeightForWidth(0)), Element: workflow},
		{Size: layout.Fixed(1), Element: status},
		{Size: layout.Measured(1, 0), Element: a.composer.Element()},
	}})
	a.buildReview() //nolint:contextcheck // Review callbacks use the session Scope.Context rather than retaining the construction call.
	panel := kit.NewPanel(kit.PanelConfig{Box: kit.Box{
		Theme: theme, Glyphs: glyphs,
		Padding: layout.Symmetric(0, 1), Title: "commands", Footer: "enter complete", FooterAlign: layout.End,
	}, Content: a.completion.Element()})
	popup := scene.NewGroup(scene.GroupConfig{
		Element: scene.ElementConfig{Enabled: a.completion.Open}, Items: []scene.Item{{Element: panel.Element()}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			if !a.completion.Open() || a.reviewDialog.Controller().Open() {
				return []image.Rectangle{{}}
			}
			width := max(0, min(max(a.completion.Width()+4, 32), space.X-2))
			height := min(a.completion.HeightForWidth(width)+2, space.Y)
			y := max(space.Y-a.composer.HeightForWidth(space.X)-height, 0)
			return []image.Rectangle{grid.Area(1, y, width, height)}
		},
	})
	unsubscribe := a.composer.Controller().Session().Subscribe(func(base.ChangeMap) { a.refreshCompletion() })
	a.element = scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{
		Key:      "agent",
		Preview:  func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: a.Handle(event.Input)} }, //nolint:contextcheck // Input uses the session scope created from ctx above.
		Detached: func() { unsubscribe(); a.scope.Close(); a.conversation.content.Close() },
	}, Items: []scene.Item{{Element: a.body.Element()}, {Element: popup.Element()}, {Element: a.reviewDialog.Element()}}}).Element()
	// The scene and source lifecycle do not require terminal metadata support.
	_ = runtime.Session().SetTitle("agent mock")
	return a, nil
}

func (a *agent) Element() scene.Element { return a.element }

func (a *agent) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
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
	if a.reviewDialog.Controller().Open() {
		if key.Code == input.Esc {
			a.answerReview(false)
			return true
		}
		return false
	}
	if a.completion.Handle(event) {
		return true
	}
	if key.Code == input.Enter && key.Mods == 0 {
		a.submit()
		return true
	}
	return false
}

func (a *agent) submit() {
	line := strings.TrimSpace(a.composer.Controller().Text())
	if line == "" {
		return
	}
	if name, arg, command := parseCommand(line); command {
		a.composer.Controller().Clear()
		a.completion.Dismiss()
		a.runCommand(name, arg)
		return
	}
	if a.run != nil {
		a.status.Doing = "finish or cancel the current run first"
		return
	}
	a.composer.Controller().Clear()
	a.completion.Dismiss()
	a.startRun(line)
}

func (a *agent) registerCommands() {
	a.commands.Add(
		base.Command{Name: "help", Title: "show the commands available in this session"},
		commandAction{run: func(string) {
			a.conversation.Append(a.scope.Context(), &kit.Entry{
				Theme: a.theme, Label: "commands",
				Body: "/clear — release the live transcript\n/model <fast|careful> — choose the mock plan\n/quit — leave",
			})
		}},
	)
	a.commands.Add(
		base.Command{Name: "clear", Title: "release the live transcript"},
		commandAction{run: func(string) {
			if a.run != nil {
				a.status.Doing = "the active run owns the transcript"
				return
			}
			a.conversation.Reset()
			a.workflow.Reset()
			a.status.Doing = "cleared"
		}},
	)
	a.commands.Add(
		base.Command{Name: "model", Title: "choose fast or careful planning"},
		commandAction{takesArgument: true, run: func(arg string) {
			switch arg {
			case "fast", "careful":
				a.model = arg
				a.status.Doing = "model: " + arg
			default:
				a.status.Doing = "usage: /model fast|careful"
			}
		}},
	)
	a.commands.Add(
		base.Command{Name: "quit", Title: "leave the agent", Aliases: []string{"exit"}},
		commandAction{run: func(string) { a.runtime.Quit() }},
	)
}

func (a *agent) runCommand(name, arg string) {
	command, action, ok := a.commands.Lookup(name)
	if !ok || action.run == nil {
		a.status.Doing = "unknown command: /" + name
		return
	}
	if action.takesArgument && arg == "" {
		a.status.Doing = "/" + command.Name + " needs an argument"
		return
	}
	a.commands.Used(command.Name)
	action.run(arg)
}

func parseCommand(line string) (name, arg string, ok bool) {
	if !strings.HasPrefix(line, "/") {
		return "", "", false
	}
	name, arg, _ = strings.Cut(strings.TrimPrefix(line, "/"), " ")
	return name, strings.TrimSpace(arg), true
}

func (a *agent) refreshCompletion() {
	lines := strings.Split(a.composer.Controller().Text(), "\n")
	line, column := a.composer.Controller().Cursor()
	if line < 0 || line >= len(lines) {
		a.completion.Dismiss()
		return
	}
	token, ok := base.TokenAt(lines[line], column, base.Trigger{Prefix: "/", AtStart: true})
	if !ok {
		a.completion.Dismiss()
		return
	}
	found := a.commands.Find(token.Query)
	candidates := make([]base.Candidate, 0, len(found))
	for _, match := range found {
		candidates = append(candidates, base.Candidate{
			Text: match.Command.Name, Label: match.Command.Name,
			Detail: match.Command.Title, Matched: match.At,
		})
	}
	a.completion.Offer(token, candidates)
}

func (a *agent) startRun(prompt string) {
	a.conversation.User(a.scope.Context(), prompt)
	a.workflow.Reset()
	a.status.Doing = "planning with " + a.model
	a.setProgress(term.Progress{State: term.ProgressIndeterminate})
	a.started = time.Now()
	timer, err := a.runtime.Every(a.scope, 120*time.Millisecond, a.tick)
	if err != nil {
		a.finishRun(err)
		return
	}
	a.stopClock = timer.Stop

	ingress, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: a.runtime.Dispatcher(), Limit: ingressSize, Consume: a.accept})
	if err != nil {
		a.finishRun(err)
		return
	}
	ctx, cancel := context.WithCancel(a.scope.Context())
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
	if err := a.conversation.Err(); err != nil {
		a.finishRun(err)
	}
}

func (a *agent) accept(batch program.ByteBatch) {
	if len(batch.Data) > 0 {
		a.conversation.Markdown(a.scope.Context(), string(batch.Data))
		if err := a.conversation.Err(); err != nil {
			a.finishRun(errors.Join(batch.Err, err))
			return
		}
	}
	if batch.Final {
		a.finishRun(batch.Err)
	}
}

func (a *agent) finishRun(err error) {
	a.conversation.FinishMarkdown(a.scope.Context(), err)
	if retainErr := a.conversation.Err(); retainErr != nil {
		err = errors.Join(err, retainErr)
	}
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
	native := term.Progress{State: term.ProgressNormal, Percent: 100}
	switch {
	case err == context.Canceled: //nolint:errorlint // Only bare cancellation has no failure diagnostics to display.
		a.status.Doing = "cancelled"
		native.State = term.ProgressWarning
	case err != nil:
		a.status.Doing = "failed: " + err.Error()
		native.State = term.ProgressError
		a.conversation.Append(a.scope.Context(), &kit.Entry{Theme: a.theme, Label: "runtime", Body: err.Error()})
	default:
		a.status.Doing = "complete"
		_ = a.runtime.Session().Notify("mock agent completed")
	}
	a.settledProgress(native)
}

func (a *agent) setProgress(progress term.Progress) {
	if a.stopProgressReset != nil {
		a.stopProgressReset()
		a.stopProgressReset = nil
	}
	_ = a.runtime.Session().SetProgress(progress)
}

func (a *agent) settledProgress(progress term.Progress) {
	a.setProgress(progress)
	timer, err := a.runtime.After(a.scope, 2*time.Second, func() {
		_ = a.runtime.Session().SetProgress(term.Progress{})
		a.stopProgressReset = nil
	})
	if err != nil {
		a.status.Doing = err.Error()
		return
	}
	a.stopProgressReset = timer.Stop
}

func (a *agent) stopRun() {
	if a.run == nil {
		return
	}
	a.status.Doing = "cancelling"
	a.element.Invalidate()
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

func (a *agent) buildReview() {
	a.reviewConfirm = base.NewConfirm(base.ConfirmConfig{Label: "Allow this tool call?", Initial: true, Yes: "apply", No: "deny"})
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.theme, Glyphs: a.glyphs,
		Fields: []base.Field{a.reviewConfirm}, Keys: base.DefaultFormKeys(),
		Hints: []keymap.Action{base.Submit, base.Cancel},
		Submit: func(id base.SubmissionID) (scene.ActionResult[base.SubmissionID], error) {
			a.answerReview(a.reviewConfirm.Answer())
			return scene.ActionResult[base.SubmissionID]{State: scene.Applied, Value: id}, nil
		},
		Cancel: func(base.SubmissionID) error { a.answerReview(false); return nil },
	})
	a.reviewForm = dressed.Controller()
	change := kit.NewDiff(kit.DiffConfig{Theme: a.theme, Glyphs: a.glyphs, Numbers: true})
	content := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { change.Draw(frame.View) }, Measure: func(axis layout.Axis, width int) int {
		if axis == layout.Down {
			return change.HeightForWidth(width)
		}
		return 0
	}})
	a.reviewPane = reviewPane{diff: change, window: base.NewViewport(base.ViewportConfig{Content: content}), form: dressed, theme: a.theme}
	if err := a.reviewPane.window.Element().SetFocusable(func() bool { return false }); err != nil {
		panic(err)
	}
	a.reviewDialog = kit.NewDialog(kit.DialogConfig{
		Theme: a.theme, Glyphs: a.glyphs,
		Title: "Review tool call", Body: a.reviewPane.Element(), Where: layout.Placement{Width: 76, Height: 16, Margin: 1},
	})
}

func (a *agent) openReview(request *reviewRequest) {
	if a.review != nil {
		a.answerReview(false)
	}
	a.conversation.FlushMarkdown(a.scope.Context())
	if err := a.conversation.Err(); err != nil {
		a.finishRun(err)
		request.answer <- false
		return
	}
	a.review = request
	if _, err := a.reviewConfirm.Request(a.scope.Context(), base.ConfirmRequest{ExpectedRevision: a.reviewConfirm.Snapshot().Revision, Value: true}); err != nil {
		a.status.Doing = err.Error()
		a.answerReview(false)
		return
	}
	a.reviewPane.diff.SetHunks(diff.Between(request.proposal.Before, request.proposal.After).Hunks(2))
	a.reviewPane.title = request.proposal.Path + " — " + request.proposal.Summary
	a.reviewDialog.Controller().SetDescription(request.proposal.Summary + " — " + request.proposal.Path)
	a.status.Doing = "waiting for tool approval"
	a.reviewDialog.Controller().Show()
}

func (a *agent) answerReview(approved bool) {
	request := a.review
	if request == nil {
		return
	}
	a.review = nil
	a.reviewDialog.Controller().Dismiss()
	request.answer <- approved
	if approved {
		a.status.Doing = "applying approved change"
		return
	}
	a.status.Doing = "tool call denied"
	a.conversation.Append(a.scope.Context(), &kit.Entry{
		Theme: a.theme, Label: "tool", Body: request.proposal.Path + " — denied",
	})
}

func (a *agent) showTool(result toolResult) {
	a.conversation.FlushMarkdown(a.scope.Context())
	a.conversation.Append(a.scope.Context(), &kit.Entry{
		Theme: a.theme, Label: result.Name, Body: oneLine(result.Summary),
	})
	shown := kit.NewDiff(kit.DiffConfig{
		Theme: a.theme, Glyphs: a.glyphs,
		Hunks: diff.Between(result.Change.Before, result.Change.After).Hunks(2), Numbers: true,
	})
	a.conversation.Append(a.scope.Context(), shown)
	if err := a.conversation.Err(); err != nil {
		a.finishRun(err)
	}
}
