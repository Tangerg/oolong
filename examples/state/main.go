// Command state demonstrates owned drafts, controlled domain state and live themes.
//
// Each field owns its editable document. The application reads those owners once
// on submission and creates its immutable result. Validation may reject submission
// while preserving the user's draft; normalization belongs to the accepted domain
// value instead of a second writable binding that races the editor. The live
// notification field uses a versioned domain owner shared with an external action.
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

func main() {
	var values answers
	form, local := fields(&values)
	app, err := program.NewScreen(program.ScreenConfig{Build: func(runtime *program.Runtime) (scene.Element, error) {
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

func fields(values *answers) (*stateForm, *base.Text) {
	local := base.NewText(base.TextConfig{Label: "Local — the field owns this draft", Placeholder: "component state"})
	exact := base.NewText(base.TextConfig{Label: "Exact — copied only on submission", Placeholder: "owned document"})
	normalized := base.NewText(base.TextConfig{Label: "Normalized — submission stores uppercase", Placeholder: "domain normalization"})
	guarded := base.NewText(base.TextConfig{Label: "Guarded — submit at most eight characters", Placeholder: "validation preserves the draft", Check: func(value string) error {
		if utf8.RuneCountInString(value) > 8 {
			return errors.New("at most eight characters")
		}
		return nil
	}})
	form := &stateForm{theme: kit.NewThemeOwner(kit.Theme{}), notifications: newNotificationSettings()}
	form.confirm = base.NewConfirm(base.ConfirmConfig{Label: "Live — domain-owned notifications", Yes: "on", No: "off", Control: form.notifications})
	form.view = kit.NewForm(kit.FormConfig{
		Scope: form.theme.Scope(),
		Title: "Who owns this text?", Gap: 1,
		Fields: []base.Field{local, exact, normalized, guarded, form.confirm},
		Keys:   base.DefaultFormKeys(), Hints: []keymap.Action{base.FocusNext, base.Submit, base.Cancel},
		Submit: func(id base.SubmissionID) (scene.ActionResult[base.SubmissionID], error) {
			*values = answers{exact: exact.Text(), normalized: strings.ToUpper(normalized.Text()), guarded: guarded.Text()}
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
	accent := grid.Style{Attr: grid.Bold}
	buttonScope := form.theme.Scope().With(kit.ThemeOverride{Accent: &accent})
	reset := kit.NewButton(kit.ButtonConfig{
		Key: "reset-domain", Label: "Reset domain", Scope: buttonScope,
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
	})
	palette := kit.NewButton(kit.ButtonConfig{
		Key: "switch-palette", Label: "Switch palette", Scope: buttonScope,
		Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
			next := kit.Light()
			if form.theme.Scope().Snapshot().Palette == next {
				next = kit.Dark()
			}
			form.theme.Set(next)
			return scene.ActionResult[struct{}]{State: scene.Applied}, nil
		},
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
	form.view.Glyphs = kit.GlyphsFor(runtime.Environment().Locale())
	return scene.NewGroup(scene.GroupConfig{
		Axis: layout.Down,
		Element: scene.ElementConfig{Detached: form.confirm.Close, Preview: func(event scene.Event) scene.EventResult {
			key, ok := event.Input.(input.Key)
			if ok && key.Down() && (key.Code == input.Esc || key.Rune == 'c' && key.Mods.Has(input.Ctrl)) {
				runtime.Quit()
				return scene.EventResult{Consumed: true}
			}
			return scene.EventResult{}
		}}, Items: []scene.Item{{Element: form.view.Element(), Size: layout.Flex(1)}, {Element: form.buttons.Element(), Size: layout.Fixed(1)}},
	}).Element()
}
