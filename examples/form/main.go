// Command form asks four questions, on a screen or in words.
//
// A form is a container with two things added: an answer checked when the keyboard
// leaves a field, and the set checked on submission. The four fields are the four
// anything ever asks for — a line of text, one choice, several, and a yes or no —
// and each owns its answer until submission creates one application snapshot.
//
// The second half is the point of the example. Run it with its output going
// somewhere that is not a terminal:
//
//	go run ./form | cat
//
// and the same form is asked one question at a time, in words. The questions are the
// fields' own, so nothing is described twice.
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

func main() {
	var got answers
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

func ask(into *answers) *sessionForm {
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
		Options: base.Options("read", "write", "run"),
	})
	tools.SetLimit(2)
	stream := base.NewConfirm(base.ConfirmConfig{Label: "Stream the answer?"})
	form := &sessionForm{theme: kit.NewThemeOwner(kit.Theme{})}
	form.view = kit.NewForm(kit.FormConfig{
		Scope: form.theme.Scope(),
		Title: "New session", Gap: 1,
		Keys:   base.DefaultFormKeys(),
		Hints:  []keymap.Action{base.FocusNext, base.Submit, base.Cancel},
		Fields: []base.Field{name, model, tools, stream},
		Submit: func(id base.SubmissionID) (scene.ActionResult[base.SubmissionID], error) {
			chosen, _ := model.Chosen()
			*into = answers{name: name.Text(), model: chosen.Value, tools: tools.Taken(), stream: stream.Answer()}
			if form.done != nil {
				form.done()
			}
			return scene.ActionResult[base.SubmissionID]{State: scene.Applied, Value: id}, nil
		},
		Cancel: func(base.SubmissionID) error {
			if form.done != nil {
				form.done()
			}
			return nil
		},
	})
	return form
}

// The textual and visual flows submit the same field owners and validation tuple.
func askInWords(form *sessionForm, in io.Reader, out io.Writer) error {
	_, err := kit.Ask(form.view.Controller(), in, out)
	return err
}

func dress(runtime *program.Runtime, form *sessionForm) scene.Element {
	form.done = runtime.Quit
	form.theme.Set(kit.Suited(runtime.Environment().Ground()))
	form.view.Glyphs = kit.GlyphsFor(runtime.Environment().Locale())
	return scene.NewGroup(scene.GroupConfig{
		Overlay: true,
		Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
			key, ok := event.Input.(input.Key)
			if ok && key.Down() && (key.Code == input.Esc || key.Rune == 'c' && key.Mods.Has(input.Ctrl)) {
				runtime.Quit()
				return scene.EventResult{Consumed: true}
			}
			return scene.EventResult{}
		}}, Items: []scene.Item{{Element: form.view.Element()}},
	}).Element()
}

func report(got answers) {
	if got.name == "" {
		fmt.Println("nothing was asked for")
		return
	}
	fmt.Printf("session %q, model %q, tools %v, streaming %v\n",
		got.name, got.model, got.tools, got.stream)
}
