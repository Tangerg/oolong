package base_test

import (
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func deferredKeys(first, second keymap.Action) (*keymap.Map, *func()) {
	var resolve func()
	keys := &keymap.Map{Resolve: func(_ time.Duration, callback func()) func() {
		resolve = callback
		return func() {}
	}}
	x := input.Chord{Rune: 'x'}
	keys.Bind(first, x)
	keys.Bind(second, x, x)
	return keys, &resolve
}

func focusElement(t *testing.T, element scene.Element) {
	t.Helper()
	if err := element.RequestFocus(); err != nil {
		t.Fatal(err)
	}
}

func TestDeferredScrollCannotOutliveKeyboardOwnership(t *testing.T) {
	for _, kind := range []string{"transcript", "viewport"} {
		t.Run(kind, func(t *testing.T) {
			keys, resolve := deferredKeys(base.ScrollDown, base.ScrollBottom)
			var element scene.Element
			var scroll *base.Scroll
			if kind == "transcript" {
				transcript, err := base.NewTranscript(base.TranscriptConfig{Keys: keys})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(transcript.Close)
				if _, err := transcript.Add(transcriptHuge{rows: 20}); err != nil {
					t.Fatal(err)
				}
				element, scroll = transcript.Element(), transcript.Scroll()
			} else {
				content := scene.NewElement(scene.ElementConfig{Measure: func(layout.Axis, int) int { return 20 }})
				viewport := base.NewViewport(base.ViewportConfig{Content: content, Keys: keys})
				element, scroll = viewport.Element(), viewport.Scroll()
			}
			other := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
			group := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
				{Element: element, Size: layout.Flex(1)}, {Element: other, Size: layout.Fixed(1)},
			}})
			fixture := newTestRoot(group.Element())
			t.Cleanup(fixture.root.Close)
			fixture.Draw(grid.NewSurface(20, 6).View())
			scroll.ToTop()
			focusElement(t, element)
			if !fixture.root.Handle(input.Key{Rune: 'x'}) || *resolve == nil {
				t.Fatal("ambiguous key was not deferred")
			}
			before := scroll.Offset()
			focusElement(t, other)
			focusElement(t, element)
			(*resolve)()
			if got := scroll.Offset(); got != before {
				t.Fatalf("old keyboard ownership scrolled from %d to %d", before, got)
			}
		})
	}
}

func TestDeferredFormInputTracksContinuousFocusWithin(t *testing.T) {
	for _, transition := range []string{"within", "leave-return", "close", "remount"} {
		t.Run(transition, func(t *testing.T) {
			keys, resolve := deferredKeys(base.Submit, base.Cancel)
			first, second := base.NewConfirm(base.ConfirmConfig{}), base.NewConfirm(base.ConfirmConfig{})
			calls := 0
			form := base.NewForm(base.FormConfig{Fields: []base.Field{first, second}, Keys: keys, Submit: func(base.SubmissionID) (scene.ActionState, error) {
				calls++
				return scene.Applied, nil
			}})
			other := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
			group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: form.Element()}, {Element: other}}})
			fixture := newTestRoot(group.Element())
			t.Cleanup(fixture.root.Close)
			focusElement(t, first.Element())
			if !fixture.root.Handle(input.Key{Rune: 'x'}) || *resolve == nil {
				t.Fatal("form did not retain its prefix")
			}
			switch transition {
			case "within":
				focusElement(t, second.Element())
			case "leave-return":
				focusElement(t, other)
				focusElement(t, first.Element())
			case "close":
				fixture.root.Close()
			case "remount":
				fixture.SetContent(scene.Element{})
				fixture.SetContent(group.Element())
				focusElement(t, first.Element())
			}
			(*resolve)()
			want := 0
			if transition == "within" {
				want = 1
			}
			if calls != want {
				t.Fatalf("submission calls = %d, want %d", calls, want)
			}
		})
	}
}

func TestPartialSequenceCannotResumeAfterFocusReturns(t *testing.T) {
	keys := &keymap.Map{}
	keys.Bind(base.Submit, input.Chord{Rune: 'x'}, input.Chord{Rune: 'x'})
	field := base.NewConfirm(base.ConfirmConfig{})
	calls := 0
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}, Keys: keys, Submit: func(base.SubmissionID) (scene.ActionState, error) {
		calls++
		return scene.Applied, nil
	}})
	other := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	fixture := newTestRoot(scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: form.Element()}, {Element: other}}}).Element())
	t.Cleanup(fixture.root.Close)
	focusElement(t, field.Element())
	fixture.root.Handle(input.Key{Rune: 'x'})
	focusElement(t, other)
	focusElement(t, field.Element())
	fixture.root.Handle(input.Key{Rune: 'x'})
	if calls != 0 {
		t.Fatal("the new focus resumed an old partial sequence")
	}
	fixture.root.Handle(input.Key{Rune: 'x'})
	if calls != 1 {
		t.Fatal("the new focus could not start a fresh sequence")
	}
}

func TestDeferredFormNavigationUsesItsKeyboardLifetime(t *testing.T) {
	keys, resolve := deferredKeys(base.FocusNext, base.FocusPrev)
	first, second := base.NewConfirm(base.ConfirmConfig{}), base.NewConfirm(base.ConfirmConfig{})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{first, second}, Keys: keys})
	fixture := newTestRoot(form.Element())
	t.Cleanup(fixture.root.Close)
	focusElement(t, first.Element())
	fixture.root.Handle(input.Key{Rune: 'x'})
	if *resolve == nil {
		t.Fatal("navigation was not deferred")
	}
	(*resolve)()
	if !second.Element().Focused() {
		t.Fatal("deferred navigation retained an expired event delivery")
	}
}

func TestDeferredContainerInputCannotOutliveItsRoot(t *testing.T) {
	for _, kind := range []string{"tabs", "dialog"} {
		t.Run(kind, func(t *testing.T) {
			content := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
			var element scene.Element
			var resolve *func()
			var changed func() bool
			if kind == "tabs" {
				keys, callback := deferredKeys(base.NextTab, base.PrevTab)
				tabs := base.NewTabs(base.TabsConfig{Keys: keys, Items: []base.Tab{
					{Key: "first", Content: content}, {Key: "second", Content: scene.NewElement(scene.ElementConfig{})},
				}})
				element, resolve = tabs.Element(), callback
				changed = func() bool { return tabs.Selected() != 0 }
			} else {
				keys, callback := deferredKeys(base.Close, "unused")
				dismissed := false
				dialog := base.NewDialog(base.DialogConfig{Initial: true, Content: content, Keys: keys, Dismissed: func() { dismissed = true }})
				element, resolve = dialog.Element(), callback
				changed = func() bool { return dismissed }
			}
			fixture := newTestRoot(element)
			focusElement(t, content)
			fixture.root.Handle(input.Key{Rune: 'x'})
			if *resolve == nil {
				t.Fatal("container did not retain its prefix")
			}
			fixture.root.Close()
			(*resolve)()
			if changed() {
				t.Fatal("closed root allowed a deferred input effect")
			}
		})
	}
}

func TestDelegatedCompletionKeepsOnlyItsOriginalKeyboardLifetime(t *testing.T) {
	for _, test := range []struct {
		name      string
		moveFocus bool
	}{
		{name: "current"},
		{name: "focus moved", moveFocus: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			keys, resolve := deferredKeys(base.Accept, base.Dismiss)
			calls := 0
			completion := &base.Completion{Keys: keys, Accept: func(base.Candidate, base.Token) { calls++ }}
			completion.Offer(base.Token{}, []base.Candidate{{Text: "candidate"}})
			editor := base.NewText(base.TextConfig{})
			other := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
			group := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
				return scene.EventResult{Consumed: completion.Handle(event.Input())}
			}}, Items: []scene.Item{{Element: editor.Element()}, {Element: completion.Element()}, {Element: other}}})
			fixture := newTestRoot(group.Element())
			t.Cleanup(fixture.root.Close)
			focusElement(t, editor.Element())
			fixture.root.Handle(input.Key{Rune: 'x'})
			if *resolve == nil {
				t.Fatal("preview could not delegate a key to its completion")
			}
			want := 1
			if test.moveFocus {
				focusElement(t, other)
				focusElement(t, editor.Element())
				want = 0
			}
			(*resolve)()
			if calls != want {
				t.Fatalf("delegated acceptance calls=%d, want %d", calls, want)
			}
		})
	}
}
