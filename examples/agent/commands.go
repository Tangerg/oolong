package main

import (
	"image"
	"strings"
	"unicode"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	commandPrefix        = "/"
	commandPopupMinWidth = 32
	commandPopupInset    = 1
)

// commandAction is what a registered command means to this application. A
// nonempty argument is both the usage placeholder and the requirement that one
// argument follows the command name.
type commandAction struct {
	argument string
	run      func(argument string)
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
			if err := a.conversation.Reset(); err != nil {
				a.inform(err.Error())
				return
			}
			a.workflow.Reset()
			a.inform("cleared")
		}},
	)
	a.commands.Add(
		base.Command{Name: "plan", Title: "choose fast or careful planning"},
		commandAction{argument: planningArgument, run: func(argument string) {
			mode, ok := parsePlanningMode(argument)
			if !ok {
				a.inform("usage: " + commandPrefix + "plan " + planningArgument)
				return
			}
			a.planningMode = mode
			a.inform("planning mode: " + string(mode))
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
		command, action, _ := a.commands.Lookup(match.Command.Name)
		usage := commandPrefix + command.Name
		if action.argument != "" {
			usage += " " + action.argument
		}
		lines = append(lines, usage+" — "+command.Title)
	}
	return strings.Join(lines, "\n")
}

func (a *agent) runCommand(name, argument string) {
	command, action, ok := a.commands.Lookup(name)
	if !ok {
		a.inform("unknown command: " + commandPrefix + name)
		return
	}
	if action.argument != "" && argument == "" {
		a.inform(commandPrefix + command.Name + " needs an argument")
		return
	}
	a.commands.Used(command.Name)
	action.run(argument)
}

func parseCommand(line string) (name, argument string, ok bool) {
	command, ok := strings.CutPrefix(line, commandPrefix)
	if !ok {
		return "", "", false
	}
	if split := strings.IndexFunc(command, unicode.IsSpace); split >= 0 {
		return command[:split], strings.TrimSpace(command[split:]), true
	}
	return command, "", true
}

// newCommandCompletion offers commands only for a slash typed at the start of the
// first prompt line, where a submitted line is parsed as a command.
func (a *agent) newCommandCompletion() *base.Completion {
	keys := base.DefaultCompletionKeys()
	keys.Bind(base.Accept, input.Chord{Code: input.Enter})
	completion := base.NewCompletion(base.CompletionConfig{
		Look: a.theme.Look(a.glyphs), Keys: keys,
		CurrentToken: func() (base.Token, bool) {
			editor := a.composer.Controller()
			if line, _ := editor.Cursor(); line != 0 {
				return base.Token{}, false
			}
			return editor.TokenAtCursor(base.Trigger{Prefix: commandPrefix, AtStart: true})
		},
		Accept: func(candidate base.Candidate, token base.Token) {
			a.composer.Controller().Replace(token.Line, token.Start, token.End, candidate.Text)
			a.completion.Dismiss()
		},
	})
	return completion
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

// commandPopup floats the completion list just above the composer. It yields to an
// open review, whose modal decision must not be covered by a stale suggestion.
func (a *agent) commandPopup() scene.Element {
	help := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		kit.Help{Theme: a.theme, Keys: a.completion.Keys(), Show: []keymap.Action{base.Accept}}.Draw(frame.View)
	}})
	body := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
		{Element: a.completion.Element(), Size: layout.Measured(0, 0)},
		{Element: help, Size: layout.Fixed(1)},
	}})
	panel := kit.NewPanel(kit.PanelConfig{Theme: a.theme, Box: kit.Box{
		Glyphs: a.glyphs, Padding: boxPadding, Title: "commands",
	}, Content: body.Element()})
	return scene.NewGroup(scene.GroupConfig{
		Element: scene.ElementConfig{Enabled: a.completion.Open}, Items: []scene.Item{{Element: panel.Element()}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			if !a.completion.Open() || a.reviewDialog.Controller().Open() {
				return []image.Rectangle{{}}
			}
			available := space.X - 2*commandPopupInset
			width := max(0, min(max(a.completion.Width()+panel.Box().Overhead().X, commandPopupMinWidth), available))
			height := min(panel.HeightForWidth(width), space.Y)
			y := max(space.Y-a.composer.HeightForWidth(space.X)-height, 0)
			return []image.Rectangle{grid.Area(commandPopupInset, y, width, height)}
		},
	}).Element()
}
