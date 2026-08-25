// Command state shows the four outcomes of one headless text-field API: local
// state, an exact caller binding, normalization, and rejection.
//
// A field with no Value owns its answer. Giving it an Accessor makes the caller's
// value authoritative; the same editing operations then write through that seam.
// Accessor.Set may accept, normalize, or reject an edit, and Accessor.Value reports
// which outcome actually became state. No second controlled-widget API is involved.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/program"
)

func main() {
	var values answers
	form, local := fields(&values)
	err := program.Run(context.Background(), program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return dress(runtime, form)
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "state:", err)
		os.Exit(1)
	}
	fmt.Printf("local=%q bound=%q normalized=%q guarded=%q\n",
		local.Editor().Text(), values.bound, values.normalized, values.guarded)
}

type answers struct {
	bound      string
	normalized string
	guarded    string
}

func fields(values *answers) (*headless.Form, *headless.Text) {
	local := &headless.Text{
		Label:       "Local — the field owns this answer",
		Placeholder: "component state",
	}
	form := headless.NewForm(
		local,
		&headless.Text{
			Label:       "Bound — the caller owns an exact value",
			Placeholder: "headless.Bind",
			Value:       headless.Bind(&values.bound),
		},
		&headless.Text{
			Label:       "Normalized — the caller stores uppercase",
			Placeholder: "Accessor normalizes",
			Value:       uppercase{value: &values.normalized},
		},
		&headless.Text{
			Label:       "Guarded — the caller rejects more than eight characters",
			Placeholder: "Accessor validates",
			Value:       limited{value: &values.guarded, runes: 8},
		},
	)
	form.Gap = 1
	return form, local
}

func dress(runtime *program.Runtime, form *headless.Form) program.Component {
	form.Keys = headless.DefaultFormKeys()
	form.Done = runtime.Quit
	form.GaveUp = runtime.Quit
	form.Focus(true)
	view := kit.NewForm(kit.FormConfig{
		Theme:      kit.Suited(runtime.Environment().Ground()),
		Glyphs:     kit.GlyphsFor(runtime.Environment().Locale()),
		Controller: form,
		Title:      "Who owns this text?",
		Hints:      []keymap.Action{headless.FocusNext, headless.Submit, headless.Cancel},
	})
	return headless.NewRoot(view)
}

type uppercase struct{ value *string }

func (u uppercase) Value() string { return *u.value }

func (u uppercase) Set(value string) { *u.value = strings.ToUpper(value) }

type limited struct {
	value *string
	runes int
}

func (l limited) Value() string { return *l.value }

func (l limited) Set(value string) {
	if utf8.RuneCountInString(value) <= l.runes {
		*l.value = value
	}
}
