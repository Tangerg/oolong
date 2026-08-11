// Command form asks four questions, on a screen or in words.
//
// A form is a container with two things added: an answer checked when the keyboard
// leaves a field, and the set checked on submission. The four fields are the four
// anything ever asks for — a line of text, one choice, several, and a yes or no —
// and each of them writes into a variable of this program's own.
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

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

// answers is what the form collects. The fields bind to these, so there is one place
// the answers live and it is this program's.
type answers struct {
	name   string
	model  string
	tools  []string
	stream bool
}

func main() {
	var got answers
	form := ask(&got)

	err := program.Run(context.Background(), program.Config{
		Root:     func(runtime *program.Runtime) program.Component { return dress(runtime, form) },
		Terminal: term.Config{Probe: true},
	})
	switch {
	case errors.Is(err, term.ErrNotTerminal):
		// No screen to draw on. The same fields already know how to ask and parse
		// their answers; kit.Ask supplies the line-oriented conversation.
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

// ask builds the form. Nothing about it is a look: what it asks, what it will accept
// and where the answers go, and no more.
func ask(into *answers) *headless.Form {
	model := &headless.Select[string]{
		Label: "Which model?",
		Value: headless.Bind(&into.model),
	}
	model.SetOptions(headless.Options("fast", "balanced", "careful"))
	tools := &headless.MultiSelect[string]{
		Label: "Which tools may it use?",
		Value: headless.Bind(&into.tools),
	}
	tools.SetLimit(2)
	tools.SetOptions(headless.Options("read", "write", "run"))
	form := headless.NewForm(
		&headless.Text{
			Label:       "What is this session called?",
			Placeholder: "a name",
			Value:       headless.Bind(&into.name),
			Check: func(s string) error {
				if strings.TrimSpace(s) == "" {
					return errors.New("a name is needed")
				}
				return nil
			},
		},
		model,
		tools,
		&headless.Confirm{
			Label: "Stream the answer?",
			Value: headless.Bind(&into.stream),
		},
	)
	form.Gap = 1
	return form
}

// askInWords is the whole of the other path: the conversation, over whatever the
// program was given instead of a terminal.
func askInWords(form *headless.Form, in io.Reader, out io.Writer) error {
	return kit.Ask(form, in, out)
}

// dress is the same form with a look on it, as a component the runtime can run.
func dress(runtime *program.Runtime, form *headless.Form) program.Component {
	keys := headless.DefaultFormKeys()
	form.Keys = keys
	// Finishing with the form is what ends the program, either way. The form knows
	// nothing about there being one.
	form.Done = runtime.Quit
	form.GaveUp = runtime.Quit
	form.Focus(true)

	view := kit.NewForm(kit.FormConfig{
		Theme:      kit.Suited(runtime.Environment().Ground()),
		Glyphs:     kit.GlyphsFor(runtime.Environment().Locale()),
		Controller: form,
		Title:      "New session",
		Hints:      []keymap.Action{headless.FocusNext, headless.Submit, headless.Cancel},
	})
	return headless.NewRoot(&screen{view: view})
}

// screen is the form, drawn.
type screen struct{ view *kit.Form }

func (s *screen) Draw(v headless.Frame) { s.view.Draw(v) }

func (s *screen) Handle(ev input.Event) bool { return s.view.Handle(ev) }

func report(got answers) {
	if got.name == "" {
		fmt.Println("nothing was asked for")
		return
	}
	fmt.Printf("session %q, model %q, tools %v, streaming %v\n",
		got.name, got.model, got.tools, got.stream)
}
