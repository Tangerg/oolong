// Command form shares field owners and validation across screen and textual flows
// so each accepted submission has one meaning.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

// answers is an immutable submission snapshot read from the field owners.
type answers struct {
	name   string
	model  string
	tools  []string
	stream bool
}

const maximumSelectedTools = 2

const quitSession keymap.Action = "quit"

func main() {
	var got *answers
	form := ask(&got)
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true}, Build: func(runtime *program.Runtime) (scene.Element, error) { return dress(runtime, form), nil }})
	if err == nil {
		err = app.Run(context.Background())
	}

	switch {
	case errors.Is(err, term.ErrNotTerminal):
		if said := askInWords(form, os.Stdin, os.Stdout); said != nil {
			fmt.Fprintln(os.Stderr, "form:", said)
			os.Exit(1)
		}
	case err != nil:
		fmt.Fprintln(os.Stderr, "form:", err)
		os.Exit(1)
	}
	report(got)
}

type sessionForm struct {
	theme *kit.ThemeOwner
	view  *kit.Form
	done  func()
}

// ask stores the submitted answers through into. A form left without submitting
// leaves it nil, which is what reports that nothing was asked for.
func ask(into **answers) *sessionForm {
	name := base.NewText(base.TextConfig{
		Label: "What is this session called?", Placeholder: "a name",
		Check: func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("a name is needed")
			}
			return nil
		},
	})
	model := base.NewSelect(base.SelectConfig[string]{
		Same: base.Equal[string], Label: "Which model?",
		Options: base.Options("fast", "balanced", "careful"),
	})
	tools := base.NewMultiSelect(base.MultiSelectConfig[string]{
		Same: base.Equal[string], Label: "Which tools may it use?",
		Options: base.Options("read", "write", "run"), Limit: maximumSelectedTools,
	})
	stream := base.NewConfirm(base.ConfirmConfig{Label: "Stream the answer?"})
	form := &sessionForm{theme: kit.NewThemeOwner(kit.Theme{})}
	form.view = kit.NewForm(kit.FormConfig{
		Scope: form.theme.Scope(),
		Title: "New session", Gap: 1,
		Keys:   base.DefaultFormKeys(),
		Hints:  []keymap.Action{base.FocusNext, base.Submit, base.Cancel},
		Fields: []base.Field{name, model, tools, stream},
		Submit: func(base.SubmissionID) (scene.ActionState, error) {
			chosen, _ := model.Chosen()
			*into = &answers{name: name.Text(), model: chosen.Value, tools: tools.Taken(), stream: stream.Answer()}
			if form.done != nil {
				form.done()
			}
			return scene.Applied, nil
		},
	})
	return form
}

// The textual and visual flows submit the same field owners and validation tuple.
func askInWords(form *sessionForm, in io.Reader, out io.Writer) error {
	prompter, err := kit.NewPrompter(kit.PrompterConfig{Input: in, Output: out})
	if err != nil {
		return err
	}
	_, err = prompter.Ask(form.view.Controller())
	return err
}

func dress(runtime *program.Runtime, form *sessionForm) scene.Element {
	form.done = runtime.Quit
	form.theme.Set(kit.Suited(runtime.Environment().Ground()))
	form.view.SetGlyphs(kit.GlyphsFor(runtime.Environment().Locale()))
	var keys keymap.Map
	keys.Bind(quitSession, input.Chord{Code: input.Esc})
	keys.Bind(quitSession, input.Ctrl.Rune('c'))
	// Quitting bubbles from the focused field: the form answers Cancel while a
	// submission is pending, and leaving an unsubmitted form is this enclosing
	// handler's navigation.
	return scene.NewGroup(scene.GroupConfig{
		Overlay: true,
		Element: scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
			key, ok := event.Input().(input.Key)
			if !ok {
				return scene.EventResult{}
			}
			if action, bound := keys.ActionFor(key); bound && action == quitSession {
				runtime.Quit()
				return scene.EventResult{Consumed: true}
			}
			return scene.EventResult{}
		}}, Items: []scene.Item{{Element: form.view.Element()}},
	}).Element()
}

func report(got *answers) {
	if got == nil {
		fmt.Println("nothing was asked for")
		return
	}
	fmt.Printf("session %q, model %q, tools %v, streaming %v\n",
		got.name, got.model, got.tools, got.stream)
}
