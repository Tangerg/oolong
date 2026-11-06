// Command agent uses a mock backend so review, streaming and cancellation contracts can be
// exercised without model access or workspace changes.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/Tangerg/oolong/examples/internal/streamtask"
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
	sendPrompt              keymap.Action = "send"
	cancelRun               keymap.Action = "cancel-run"
	quitAgent               keymap.Action = "quit"
	ingressSize                           = 64 << 10
	mockStreamDelay                       = 18 * time.Millisecond
	statusTickInterval                    = 120 * time.Millisecond
	settledProgressDuration               = 2 * time.Second
)

func main() {
	app, err := program.NewInline(program.InlineConfig{Terminal: term.Features{Probe: true, Mouse: true}, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
		agent, err := newAgent(context.Background(), runtime, mockBackend{delay: mockStreamDelay})
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
	outcome      sessionOutcome
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

	planningMode      planningMode
	run               *agentRun
	stopClock         func()
	stopProgressReset func()
	started           time.Time
}

type commandAction struct {
	argument string
	run      func(string)
}

func newAgent(ctx context.Context, runtime *program.InlineRuntime, backend agentBackend) (*agent, error) {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	keys := base.DefaultEditorKeys()
	keys.Bind(sendPrompt, input.Chord{Code: input.Enter})
	keys.Bind(cancelRun, input.Ctrl.Rune('x'))
	keys.Bind(quitAgent, input.Ctrl.Rune('c'))

	a := &agent{
		runtime:      runtime,
		backend:      backend,
		theme:        theme,
		glyphs:       glyphs,
		keys:         keys,
		planningMode: planningCareful,
		status:       kit.Status{Theme: theme, Glyphs: glyphs},
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
		CurrentToken: func() (base.Token, bool) {
			editor := a.composer.Controller()
			if line, _ := editor.Cursor(); line != 0 {
				return base.Token{}, false
			}
			return editor.TokenAtCursor(base.Trigger{Prefix: "/", AtStart: true})
		},
		Accept: func(candidate base.Candidate, token base.Token) {
			a.composer.Controller().Replace(token.Start, token.End, candidate.Text)
			a.completion.Dismiss()
		},
	}
	a.registerCommands() //nolint:contextcheck // Deferred UI actions use a.scope.Context(), derived from ctx above, after this constructor returns.

	workflow := scene.NewElement(scene.ElementConfig{
		Paint: func(frame scene.Frame) { a.workflow.Draw(frame.View) },
		Measure: func(axis layout.Axis, width int) int {
			if axis == layout.Down {
				return a.workflow.HeightForWidth(width)
			}
			return 0
		},
	})
	status := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { agentStatus{owner: a}.Draw(frame.View) }})
	a.body = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
		{Size: layout.Flex(1), Element: a.conversation.Element()},
		{Size: layout.Measured(0, 0), Element: workflow},
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
		Key:     "agent",
		Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: a.Handle(event.Input())} }, //nolint:contextcheck // Input uses the session scope created from ctx above.
		Detached: func() {
			unsubscribe()
			a.withdrawReview(program.ErrScopeClosed)
			a.scope.Close()
			a.conversation.content.Close()
		},
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
	if a.sessionKey(key) {
		return true
	}
	if a.completion.Handle(event) {
		return true
	}
	if action, _ := a.keys.ActionFor(key); action == sendPrompt {
		a.submit()
		return true
	}
	return false
}

// The root and the review pane opt into the same session commands. Other
// root shortcuts must not run behind an open review.
func (a *agent) sessionKey(key input.Key) bool {
	action, _ := a.keys.ActionFor(key)
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
		a.inform("finish or cancel the current run first")
		return
	}
	a.startRun(line)
}

func (a *agent) registerCommands() {
	a.commands.Add(
		base.Command{Name: "help", Title: "show the commands available in this session"},
		commandAction{run: func(string) {
			if err := a.conversation.Append(a.scope.Context(), &kit.Entry{
				Theme: a.theme, Label: "commands",
				Body: a.commandHelp(),
			}); err != nil {
				a.inform(err.Error())
			}
		}},
	)
	a.commands.Add(
		base.Command{Name: "clear", Title: "release the live transcript"},
		commandAction{run: func(string) {
			if a.run != nil {
				a.inform("the active run owns the transcript")
				return
			}
			a.conversation.Reset()
			a.workflow.Reset()
			a.inform("cleared")
		}},
	)
	a.commands.Add(
		base.Command{Name: "plan", Title: "choose fast or careful planning"},
		commandAction{argument: planningArgument, run: func(arg string) {
			switch mode := planningMode(arg); mode {
			case planningFast, planningCareful:
				a.planningMode = mode
				a.inform("planning mode: " + arg)
			default:
				a.inform("usage: /plan " + planningArgument)
			}
		}},
	)
	a.commands.Add(
		base.Command{Name: "quit", Title: "leave the agent", Aliases: []string{"exit"}},
		commandAction{run: func(string) { a.runtime.Quit() }},
	)
}

func (a *agent) commandHelp() string {
	var lines []string
	for _, match := range a.commands.Find("") {
		cmd, action, _ := a.commands.Lookup(match.Command.Name)
		usage := "/" + cmd.Name
		if action.argument != "" {
			usage += " " + action.argument
		}
		lines = append(lines, usage+" — "+cmd.Title)
	}
	return strings.Join(lines, "\n")
}

func (a *agent) runCommand(name, arg string) {
	command, action, ok := a.commands.Lookup(name)
	if !ok || action.run == nil {
		a.inform("unknown command: /" + name)
		return
	}
	if action.argument != "" && arg == "" {
		a.inform("/" + command.Name + " needs an argument")
		return
	}
	a.commands.Used(command.Name)
	action.run(arg)
}

func parseCommand(line string) (name, arg string, ok bool) {
	if !strings.HasPrefix(line, "/") {
		return "", "", false
	}
	command := strings.TrimPrefix(line, "/")
	if split := strings.IndexFunc(command, unicode.IsSpace); split >= 0 {
		return command[:split], strings.TrimSpace(command[split:]), true
	}
	return command, "", true
}

func (a *agent) refreshCompletion() {
	token, ok := a.completion.CurrentToken()
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
	run := &agentRun{}
	a.run = run
	a.outcome = sessionOutcome{}
	a.status.Doing = ""
	if err := a.conversation.User(a.scope.Context(), prompt); err != nil {
		a.finishRun(err)
		return
	}
	a.composer.Controller().Clear()
	a.completion.Dismiss()
	a.workflow.Reset()
	request := runRequest{prompt: prompt, planningMode: a.planningMode}
	a.inform("planning with " + string(request.planningMode))
	a.setProgress(term.Progress{State: term.ProgressIndeterminate})
	a.started = time.Now()
	timer, err := a.runtime.Every(a.scope, statusTickInterval, a.tick)
	if err != nil {
		a.finishRun(err)
		return
	}
	a.stopClock = timer.Stop

	ingress, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: a.runtime.Dispatcher(), Limit: ingressSize, Consume: func(batch program.ByteBatch) {
		if a.run == run {
			a.accept(batch)
		}
	}})
	if err != nil {
		a.finishRun(err)
		return
	}
	output := &agentBridge{
		ingress: ingress, dispatch: a.runtime.Dispatcher(), owner: a, run: run,
	}

	run.task = streamtask.Start(a.runtime.Runtime, a.scope, ingress, func(ctx context.Context) error {
		return a.backend.Run(ctx, request, output)
	})
}

func (a *agent) accept(batch program.ByteBatch) {
	if len(batch.Data) > 0 {
		if err := a.conversation.Markdown(a.scope.Context(), string(batch.Data)); err != nil {
			a.finishRun(errors.Join(batch.Err, err))
			return
		}
	}
	if batch.Final {
		a.finishRun(batch.Err)
	}
}

func (a *agent) finishRun(err error) {
	err = a.conversation.FinishMarkdown(a.scope.Context(), err)
	if a.stopClock != nil {
		a.stopClock()
		a.stopClock = nil
	}
	a.withdrawReview(context.Canceled)
	if a.run != nil {
		a.run.task.Cancel()
	}
	a.run = nil
	a.status.Doing = ""
	a.status.Elapsed = ""
	if err != nil && err != context.Canceled { //nolint:errorlint // Only bare cancellation has no diagnostics to publish.
		appendErr := a.conversation.Append(a.scope.Context(), &kit.Entry{Theme: a.theme, Label: "runtime", Body: err.Error()})
		err = errors.Join(err, appendErr)
	}
	a.outcome = finishedOutcome(err)
	if progressErr := a.settledProgress(a.outcome.progress()); progressErr != nil {
		a.outcome = finishedOutcome(errors.Join(err, progressErr))
		a.setProgress(a.outcome.progress())
	}
	if a.outcome.kind == outcomeCompleted {
		_ = a.runtime.Session().Notify("mock agent completed")
	}
}

func (a *agent) inform(message string) {
	if a.run != nil {
		a.status.Doing = message
		return
	}
	a.outcome = sessionOutcome{detail: message}
}

func (a *agent) setProgress(progress term.Progress) {
	if a.stopProgressReset != nil {
		a.stopProgressReset()
		a.stopProgressReset = nil
	}
	_ = a.runtime.Session().SetProgress(progress)
}

func (a *agent) settledProgress(progress term.Progress) error {
	a.setProgress(progress)
	timer, err := a.runtime.After(a.scope, settledProgressDuration, func() {
		_ = a.runtime.Session().SetProgress(term.Progress{})
		a.stopProgressReset = nil
	})
	if err != nil {
		return err
	}
	a.stopProgressReset = timer.Stop
	return nil
}

func (a *agent) stopRun() {
	if a.run == nil {
		return
	}
	a.inform("cancelling")
	a.element.Invalidate()
	a.run.task.Cancel()
	a.withdrawReview(context.Canceled)
}

func (a *agent) tick() {
	if a.run == nil {
		return
	}
	a.status.Tick()
	a.status.Elapsed = fmt.Sprintf("%4.1fs", time.Since(a.started).Seconds())
}

func clip(s string) string { return text.Truncate(strings.TrimSpace(s), 52, "…") }

type agentRun struct{ task *program.Task }

// An idle spinner would imply unfinished work, so settled outcomes use a
// plain label derived from their typed result.
type agentStatus struct{ owner *agent }

func (s agentStatus) Draw(view grid.View) {
	if s.owner.run != nil {
		s.owner.status.Draw(view)
		return
	}
	s.owner.outcome.label(s.owner.theme, s.owner.glyphs).Draw(view)
}

func (a *agent) buildReview() {
	a.reviewConfirm = base.NewConfirm(base.ConfirmConfig{Label: "Allow this tool call?", Initial: true, Yes: "apply", No: "deny"})
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.theme, Glyphs: a.glyphs,
		Fields: []base.Field{a.reviewConfirm}, Keys: base.DefaultFormKeys(),
		Hints: []keymap.Action{base.Submit, base.Cancel},
		Submit: func(base.SubmissionID) (scene.ActionState, error) {
			if err := a.answerReview(a.reviewConfirm.Answer()); err != nil {
				return scene.Unchanged, err
			}
			return scene.Applied, nil
		},
		Cancel: func(base.SubmissionID) error { return a.answerReview(false) },
	})
	a.reviewForm = dressed.Controller()
	change := kit.NewDiff(kit.DiffConfig{Theme: a.theme, Glyphs: a.glyphs, Numbers: true})
	content := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { change.Draw(frame.View) }, Measure: func(axis layout.Axis, width int) int {
		if axis == layout.Down {
			return change.HeightForWidth(width)
		}
		return 0
	}})
	a.reviewPane = reviewPane{sessionKey: a.sessionKey, diff: change, window: base.NewViewport(base.ViewportConfig{Content: content}), form: dressed, theme: a.theme, glyphs: a.glyphs}
	if err := a.reviewPane.window.Element().SetFocusable(func() bool { return false }); err != nil {
		panic(err)
	}
	a.reviewDialog = kit.NewDialog(kit.DialogConfig{
		Theme: a.theme, Glyphs: a.glyphs,
		Dismissed: func() {
			if err := a.answerReview(false); err != nil {
				a.finishRun(err)
			}
		},
		Title: "Review tool call", Body: a.reviewPane.Element(), Where: layout.Placement{Width: 76, Height: 16, Margin: 1},
	})
}

func (a *agent) openReview(request *reviewRequest) error {
	if a.review != nil {
		return errors.New("another tool review is pending")
	}
	if err := a.conversation.FinishMarkdown(a.scope.Context(), nil); err != nil {
		return err
	}
	if _, err := a.reviewConfirm.Request(a.scope.Context(), base.ConfirmRequest{ExpectedRevision: a.reviewConfirm.Snapshot().Revision, Value: true}); err != nil {
		return err
	}
	// Reserve cancellation delivery before presentation so a full ordinary dispatch
	// queue cannot leave an expired proposal visible or an unanswered worker blocked.
	delivery, err := a.runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		return err
	}
	_, err = a.runtime.Start(delivery, program.Work[struct{}]{
		Run: func(ctx context.Context) (struct{}, error) {
			select {
			case <-request.done:
				return struct{}{}, nil
			case <-request.ctx.Done():
				return struct{}{}, context.Cause(request.ctx) //nolint:contextcheck // The proposal expires independently of the delivery worker.
			case <-a.scope.Context().Done():
				return struct{}{}, context.Cause(a.scope.Context()) //nolint:contextcheck // An agent detach also withdraws outstanding reviews.
			case <-ctx.Done():
				return struct{}{}, context.Cause(ctx)
			}
		},
		Apply: func(_ struct{}, cause error) {
			defer delivery.Close()
			if cause != nil && a.review == request {
				a.withdrawReview(cause)
			}
		},
	})
	if err != nil {
		delivery.Close()
		return err
	}
	a.review = request
	a.reviewPane.diff.SetHunks(diff.Between(request.proposal.Before, request.proposal.After).Hunks(2))
	a.reviewPane.window.Scroll().ToTop()
	a.reviewPane.title = request.proposal.Path + " — " + request.proposal.Summary
	a.reviewDialog.Controller().SetDescription(request.proposal.Summary + " — " + request.proposal.Path)
	a.inform("waiting for tool approval")
	a.reviewDialog.Controller().Show()
	return nil
}

func (a *agent) answerReview(approved bool) error {
	request := a.review
	if request == nil {
		return nil
	}
	if cause := context.Cause(request.ctx); cause != nil {
		a.withdrawReview(cause)
		return nil
	}
	a.dismissReview()
	defer close(request.done)
	request.approved = approved
	if approved {
		a.inform("applying approved change")
		return nil
	}
	a.inform("tool call denied")
	request.err = a.conversation.Append(a.scope.Context(), &kit.Entry{
		Theme: a.theme, Label: "tool", Body: request.proposal.Path + " — denied",
	})
	return request.err
}

func (a *agent) withdrawReview(cause error) {
	if request := a.review; request != nil {
		a.dismissReview()
		a.inform("tool review cancelled")
		request.err = cause
		close(request.done)
	}
}

// Dismissal notifies its owner synchronously. Release the request before that
// callback can turn a completed or cancelled operation into a second decision.
func (a *agent) dismissReview() {
	a.review = nil
	a.reviewDialog.Controller().Dismiss()
}

func (a *agent) showTool(result toolResult) error {
	entry := &kit.Entry{Theme: a.theme, Label: result.Name, Body: oneLine(result.Summary)}
	shown := kit.NewDiff(kit.DiffConfig{
		Theme: a.theme, Glyphs: a.glyphs,
		Hunks: diff.Between(result.Change.Before, result.Change.After).Hunks(2), Numbers: true,
	})
	return a.conversation.Append(a.scope.Context(), entry, shown)
}
