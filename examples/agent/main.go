// Command agent uses a mock backend so review, streaming and cancellation contracts can be
// exercised without model access or workspace changes.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	sendPrompt   keymap.Action = "send"
	cancelSource keymap.Action = "cancel-source"
	quitAgent    keymap.Action = "quit"
)

const (
	agentKey                = "agent"
	ingressSize             = 64 << 10
	mockStreamDelay         = 18 * time.Millisecond
	statusTickInterval      = 120 * time.Millisecond
	settledProgressDuration = 2 * time.Second
	promptPreviewColumns    = 52
	diffContextLines        = 2
	composerMaxRows         = 5
)

var boxPadding = layout.Symmetric(0, 1)

func main() {
	app, err := program.NewInline(program.InlineConfig{Terminal: term.Features{Probe: true, Mouse: true}, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
		agent, err := newAgent(runtime, mockBackend{delay: mockStreamDelay})
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
	scope   *program.Scope

	conversation *conversation
	workflow     workflow
	composer     *kit.Composer
	commands     base.Commands[commandAction]
	completion   *base.Completion
	planningMode planningMode
	element      scene.Element

	// run is the current backend run; outcome reports the last settled one while
	// no run is current. Exactly one of them owns the status line at a time.
	run           *agentRun
	outcome       sessionOutcome
	progressReset *program.Timer

	review        *reviewRequest
	reviewConfirm *base.Confirm
	reviewPane    *reviewPane
	reviewDialog  *kit.Dialog
}

func newAgent(runtime *program.InlineRuntime, backend agentBackend) (*agent, error) {
	environment := runtime.Environment()
	theme := kit.Suited(environment.Ground())
	glyphs := kit.GlyphsFor(environment.Locale())
	a := &agent{
		runtime: runtime, backend: backend, theme: theme, glyphs: glyphs,
		workflow: newWorkflow(theme, glyphs), planningMode: planningCareful,
	}
	if err := a.buildReview(); err != nil {
		return nil, err
	}
	keys := base.DefaultEditorKeys()
	keys.Bind(sendPrompt, input.Chord{Code: input.Enter})
	keys.Bind(cancelSource, input.Ctrl.Rune('x'))
	keys.Bind(quitAgent, input.Ctrl.Rune('c'))
	a.composer = kit.NewComposer(kit.ComposerConfig{
		Theme:       theme,
		Glyphs:      glyphs,
		Prompt:      glyphs.Marker + " ",
		Hints:       []keymap.Action{sendPrompt, cancelSource, quitAgent},
		MaxRows:     composerMaxRows,
		Placeholder: "Ask the mock agent to change something",
		Keys:        keys,
		Clipboard:   runtime.Clipboard(),
	})
	a.completion = a.newCommandCompletion()
	a.registerCommands()

	scope, err := runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		return nil, err
	}
	a.scope = scope
	a.conversation, err = newConversation(conversationConfig{Runtime: runtime.Runtime, Scope: scope, Theme: theme, Glyphs: glyphs, Wheel: environment.Wheel(), Publisher: runtime.Publisher()})
	if err != nil {
		scope.Close()
		return nil, err
	}

	workflow := scene.NewElement(scene.ElementConfig{
		Paint:   func(frame scene.Frame) { a.workflow.Draw(frame.View) },
		Measure: scene.MeasureRows(a.workflow.HeightForWidth),
	})
	status := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { a.drawStatus(frame.View) }})
	body := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
		{Size: layout.Flex(1), Element: a.conversation.Element()},
		{Size: layout.Measured(0, 0), Element: workflow},
		{Size: layout.Fixed(1), Element: status},
		{Size: layout.Measured(1, 0), Element: a.composer.Element()},
	}})
	unsubscribe := a.composer.Controller().Session().Subscribe(func(base.ChangeMap) { a.refreshCompletion() })
	a.element = scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{
		Key:     agentKey,
		Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: a.Handle(event.Input())} },
		Detached: func() {
			unsubscribe()
			a.withdrawReview(program.ErrScopeClosed)
			a.scope.Close()
			a.conversation.Close()
		},
	}, Items: []scene.Item{{Element: body.Element()}, {Element: a.commandPopup()}, {Element: a.reviewDialog.Element()}}}).Element()
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
	action, _ := a.composer.Controller().Keys().ActionFor(key)
	if a.sessionAction(action) {
		return true
	}
	if a.completion.Handle(event) {
		return true
	}
	if action == sendPrompt {
		a.submit()
		return true
	}
	return false
}

// The root and the review pane opt into the same session commands. Other
// root shortcuts must not run behind an open review.
func (a *agent) sessionKey(key input.Key) bool {
	action, _ := a.composer.Controller().Keys().ActionFor(key)
	return a.sessionAction(action)
}

func (a *agent) sessionAction(action keymap.Action) bool {
	switch action {
	case quitAgent:
		a.runtime.Quit()
		return true
	case cancelSource:
		if a.run != nil {
			a.stopSource()
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
	if name, argument, command := parseCommand(line); command {
		a.composer.Controller().Clear()
		a.completion.Dismiss()
		a.runCommand(name, argument)
		return
	}
	if a.run != nil {
		a.inform("finish or cancel the current run first")
		return
	}
	a.startRun(line)
}

// inform reports a message on the status line through its current owner.
func (a *agent) inform(message string) {
	if a.run != nil {
		a.run.status.Doing = message
		return
	}
	a.outcome = sessionOutcome{detail: message}
}

// An idle spinner would imply unfinished work, so settled outcomes use a plain
// label derived from their typed result.
func (a *agent) drawStatus(view grid.View) {
	if a.run != nil {
		a.run.status.Draw(view)
		return
	}
	a.outcome.label(a.theme, a.glyphs).Draw(view)
}
