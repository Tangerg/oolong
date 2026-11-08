// Command state commits field drafts once on submission so normalization cannot
// race editable text; live notification controls share one versioned domain owner.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

const maxGuardedRunes = 8

const quitSession keymap.Action = "quit"

func main() {
	var values answers
	var local *base.Text
	app, err := program.NewScreen(program.ScreenConfig{Build: func(runtime *program.Runtime) (scene.Element, error) {
		form, draft := fields(&values, kit.GlyphsFor(runtime.Environment().Locale()))
		local = draft
		return dress(runtime, form), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "state:", err)
		os.Exit(1)
	}
	fmt.Printf("local=%q exact=%q normalized=%q guarded=%q\n", local.Text(), values.exact, values.normalized, values.guarded)
}

type answers struct{ exact, normalized, guarded string }

type stateForm struct {
	notifications *notificationSettings
	confirm       *base.Confirm
	buttons       *scene.Group
	theme         *kit.ThemeOwner
	view          *kit.Form
	done          func()
}

func fields(values *answers, glyphs kit.Glyphs) (*stateForm, *base.Text) {
	local := base.NewText(base.TextConfig{Label: "Local — the field owns this draft", Placeholder: "component state"})
	exact := base.NewText(base.TextConfig{Label: "Exact — copied only on submission", Placeholder: "owned document"})
	normalized := base.NewText(base.TextConfig{Label: "Normalized — submission stores uppercase", Placeholder: "domain normalization"})
	guardedLengthHint := fmt.Sprintf("at most %d characters", maxGuardedRunes)
	guarded := base.NewText(base.TextConfig{Label: "Guarded — submit " + guardedLengthHint, Placeholder: "validation preserves the draft", Check: func(value string) error {
		if utf8.RuneCountInString(value) > maxGuardedRunes {
			return errors.New(guardedLengthHint)
		}
		return nil
	}})
	form := &stateForm{theme: kit.NewThemeOwner(kit.Theme{}), notifications: newNotificationSettings()}
	form.confirm = base.NewConfirm(base.ConfirmConfig{Label: "Live — domain-owned notifications", Yes: "on", No: "off", Control: form.notifications})
	form.view = kit.NewForm(kit.FormConfig{
		Scope: form.theme.Scope(), Glyphs: glyphs,
		Title: "Who owns this text?", Gap: 1,
		Fields: []base.Field{local, exact, normalized, guarded, form.confirm},
		Keys:   base.DefaultFormKeys(), Hints: []keymap.Action{base.FocusNext, base.Submit, base.Cancel},
		Submit: func(base.SubmissionID) (scene.ActionState, error) {
			*values = answers{exact: exact.Text(), normalized: strings.ToUpper(normalized.Text()), guarded: guarded.Text()}
			if form.done != nil {
				form.done()
			}
			return scene.Applied, nil
		},
		Cancel: func(base.SubmissionID) error {
			if form.done != nil {
				form.done()
			}
			return nil
		},
	})
	accent := grid.Style{Attr: grid.Bold}
	buttonScope := form.theme.Scope().With(kit.ThemeOverride{Accent: &accent})
	reset := kit.NewButton(kit.ButtonConfig{
		Behavior: base.ButtonConfig{
			Key: "reset-domain", Label: "Reset domain",
			Availability: func() scene.Availability {
				if !form.notifications.Snapshot().Value {
					return scene.Availability{Disabled: true, Reason: "notifications are already off"}
				}
				return scene.Availability{}
			},
			Activate: func(ctx context.Context) (scene.ActionResult[struct{}], error) {
				result, err := form.notifications.Change(ctx, base.ConfirmRequest{ExpectedRevision: form.notifications.Snapshot().Revision, Value: false})
				return scene.ActionResult[struct{}]{State: result.State}, err
			},
		}, Scope: buttonScope, Ellipsis: glyphs.Ellipsis,
	})
	palette := kit.NewButton(kit.ButtonConfig{
		Behavior: base.ButtonConfig{
			Key: "switch-palette", Label: "Switch palette",
			Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
				next := kit.Light()
				if form.theme.Scope().Snapshot().Palette == next {
					next = kit.Dark()
				}
				form.theme.Set(next)
				return scene.ActionResult[struct{}]{State: scene.Applied}, nil
			},
		}, Scope: buttonScope, Ellipsis: glyphs.Ellipsis,
	})
	form.buttons = scene.NewGroup(scene.GroupConfig{Axis: layout.Across, Gap: 2, Items: []scene.Item{
		{Element: reset.Element(), Size: layout.Measured(0, 0)},
		{Element: palette.Element(), Size: layout.Measured(0, 0)},
	}})
	return form, local
}

func dress(runtime *program.Runtime, form *stateForm) scene.Element {
	form.done = runtime.Quit
	form.theme.Set(kit.Suited(runtime.Environment().Ground()))
	var keys keymap.Map
	keys.Bind(quitSession, input.Chord{Code: input.Esc})
	keys.Bind(quitSession, input.Ctrl.Rune('c'))
	return scene.NewGroup(scene.GroupConfig{
		Axis: layout.Down,
		Element: scene.ElementConfig{Detached: form.confirm.Close, Preview: func(event scene.Event) scene.EventResult {
			key, ok := event.Input().(input.Key)
			if !ok {
				return scene.EventResult{}
			}
			if action, bound := keys.ActionFor(key); bound && action == quitSession {
				runtime.Quit()
				return scene.EventResult{Consumed: true}
			}
			return scene.EventResult{}
		}}, Items: []scene.Item{{Element: form.view.Element(), Size: layout.Flex(1)}, {Element: form.buttons.Element(), Size: layout.Fixed(1)}},
	}).Element()
}
