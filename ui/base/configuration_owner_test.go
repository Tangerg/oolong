package base_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/fuzzy"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func configurationFailure(change func()) (failure any) {
	defer func() { failure = recover() }()
	change()
	return nil
}

func TestMountedConfigurationChangesShareTheOwnersMutationBoundary(t *testing.T) {
	text := base.NewText(base.TextConfig{})
	confirm := base.NewConfirm(base.ConfirmConfig{})
	choice := base.NewSelect(base.SelectConfig[int]{})
	multi := base.NewMultiSelect(base.MultiSelectConfig[int]{})
	editor := base.NewEditor(base.EditorConfig{})
	list := base.NewList(base.ListConfig[int]{})
	filter := base.NewFilter(base.FilterConfig[int]{})
	var tree base.Tree[int]
	table := base.NewTable(base.TableConfig[int]{})
	settings := base.NewSettings(base.SettingsConfig[int]{})
	slider := base.NewSlider(base.SliderConfig{})
	tabs := base.NewTabs(base.TabsConfig{})
	form := base.NewForm(base.FormConfig{})
	viewport := base.NewViewport(base.ViewportConfig{})
	completion := base.NewCompletion(base.CompletionConfig{})
	transcript, err := base.NewTranscript(base.TranscriptConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transcript.Close)
	sticky := transcript.Sticky()
	var keys keymap.Map
	look := base.Look{Ellipsis: "..."}
	cursor := grid.CursorStyle{Shape: grid.CursorBar}
	clipboard := new(directedClipboard)
	gutter := numberedGutter{}
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	for _, test := range []struct {
		name    string
		element scene.Element
		change  func()
		read    func() any
		want    any
	}{
		{"text label", text.Element(), func() { text.SetLabel("name") }, func() any { return text.Label() }, "name"},
		{"text placeholder", text.Element(), func() { text.SetPlaceholder("hint") }, func() any { return text.Placeholder() }, "hint"},
		{"text keys", text.Element(), func() { text.SetKeys(&keys) }, func() any { return text.Keys() }, &keys},
		{"text clipboard", text.Element(), func() { text.SetClipboard(clipboard) }, func() any { return text.Clipboard() }, clipboard},
		{"text gutter", text.Element(), func() { text.SetGutter(gutter) }, func() any { return text.Gutter() }, gutter},
		{"text cursor", text.Element(), func() { text.SetCursorStyle(cursor) }, func() any { return text.CursorStyle() }, cursor},
		{"confirm label", confirm.Element(), func() { confirm.SetLabel("continue") }, func() any { return confirm.Label() }, "continue"},
		{"confirm yes", confirm.Element(), func() { confirm.SetYes("accept") }, func() any { return confirm.Yes() }, "accept"},
		{"confirm no", confirm.Element(), func() { confirm.SetNo("decline") }, func() any { return confirm.No() }, "decline"},
		{"confirm keys", confirm.Element(), func() { confirm.SetKeys(&keys) }, func() any { return confirm.Keys() }, &keys},
		{"select label", choice.Element(), func() { choice.SetLabel("choice") }, func() any { return choice.Label() }, "choice"},
		{"select rows", choice.Element(), func() { choice.SetRows(2) }, func() any { return choice.Rows() }, 2},
		{"select row", choice.Element(), func() { choice.SetRow(func(grid.View, base.Option[int], bool, bool, base.Look) {}) }, func() any { return choice.Row() != nil }, true},
		{"select keys", choice.Element(), func() { choice.SetKeys(&keys) }, func() any { return choice.Keys() }, &keys},
		{"multi label", multi.Element(), func() { multi.SetLabel("choices") }, func() any { return multi.Label() }, "choices"},
		{"multi rows", multi.Element(), func() { multi.SetRows(2) }, func() any { return multi.Rows() }, 2},
		{"multi row", multi.Element(), func() { multi.SetRow(func(grid.View, base.Option[int], bool, bool, base.Look) {}) }, func() any { return multi.Row() != nil }, true},
		{"multi keys", multi.Element(), func() { multi.SetKeys(&keys) }, func() any { return multi.Keys() }, &keys},
		{"editor placeholder", editor.Element(), func() { editor.SetPlaceholder("hint") }, func() any { return editor.Placeholder() }, "hint"},
		{"editor look", editor.Element(), func() { editor.SetLook(look) }, func() any { return editor.Look() }, look},
		{"editor rows", editor.Element(), func() { editor.SetMaxRows(2) }, func() any { return editor.MaxRows() }, 2},
		{"editor clipboard", editor.Element(), func() { editor.SetClipboard(clipboard) }, func() any { return editor.Clipboard() }, clipboard},
		{"editor gutter", editor.Element(), func() { editor.SetGutter(gutter) }, func() any { return editor.Gutter() }, gutter},
		{"editor cursor", editor.Element(), func() { editor.SetCursorStyle(cursor) }, func() any { return editor.CursorStyle() }, cursor},
		{"editor keys", editor.Element(), func() { editor.SetKeys(&keys) }, func() any { return editor.Keys() }, &keys},
		{"list row", list.Element(), func() { list.SetRow(func(grid.View, int, int, bool) {}) }, func() any { return list.Row() != nil }, true},
		{"list wrap", list.Element(), func() { list.SetWrap(true) }, func() any { return list.Wrap() }, true},
		{"list keys", list.Element(), func() { list.SetKeys(&keys) }, func() any { return list.Keys() }, &keys},
		{"filter row", filter.Element(), func() { filter.SetRow(func(grid.View, int, int, fuzzy.Match, bool) {}) }, func() any { return filter.Row() != nil }, true},
		{"filter keys", filter.Element(), func() { filter.SetKeys(&keys) }, func() any { return filter.Keys() }, &keys},
		{"tree row", tree.Element(), func() { tree.SetRow(func(grid.View, int, base.Shown[int], bool) {}) }, func() any { return tree.Row() != nil }, true},
		{"tree keys", tree.Element(), func() { tree.SetKeys(&keys) }, func() any { return tree.Keys() }, &keys},
		{"table row", table.Element(), func() { table.SetRow(func(grid.View, int, int, bool) {}) }, func() any { return table.Row() != nil }, true},
		{"table wrap", table.Element(), func() { table.SetWrap(true) }, func() any { return table.Wrap() }, true},
		{"table keys", table.Element(), func() { table.SetKeys(&keys) }, func() any { return table.Keys() }, &keys},
		{"settings change", settings.Element(), func() {
			settings.SetChange(func(context.Context, int, int, keymap.Action) (scene.ActionResult[bool], error) {
				return scene.ActionResult[bool]{}, nil
			})
		}, func() any { return settings.Change() != nil }, true},
		{"settings keys", settings.Element(), func() { settings.SetEditKeys(&keys) }, func() any { return settings.EditKeys() }, &keys},
		{"slider keys", slider.Element(), func() { slider.SetKeys(&keys) }, func() any { return slider.Keys() }, &keys},
		{"tabs wrap", tabs.Element(), func() { tabs.SetNoWrap(true) }, func() any { return tabs.NoWrap() }, true},
		{"tabs keys", tabs.Element(), func() { tabs.SetKeys(&keys) }, func() any { return tabs.Keys() }, &keys},
		{"form gap", form.Element(), func() { form.SetGap(2) }, func() any { return form.Gap() }, 2},
		{"form keys", form.Element(), func() { form.SetKeys(&keys) }, func() any { return form.Keys() }, &keys},
		{"viewport keys", viewport.Element(), func() { viewport.SetKeys(&keys) }, func() any { return viewport.Keys() }, &keys},
		{"sticky minimum", transcript.Element(), func() { sticky.SetMinHeight(2) }, func() any { return sticky.MinHeight() }, 2},
		{"sticky gap", transcript.Element(), func() { sticky.SetGap(2) }, func() any { return sticky.Gap() }, 2},
		{"completion look", completion.Element(), func() { completion.SetLook(look) }, func() any { return completion.Look() }, look},
		{"completion token", completion.Element(), func() {
			completion.SetCurrentToken(func() (base.Token, bool) { return base.Token{Query: "query"}, true })
		}, func() any { token, _ := completion.CurrentToken(); return token.Query }, "query"},
		{"completion renderer", completion.Element(), func() { completion.SetRenderer(base.DefaultCompletionRenderer{}) }, func() any { return completion.Renderer() }, base.DefaultCompletionRenderer{}},
		{"completion rows", completion.Element(), func() { completion.SetMaxRows(2) }, func() any { return completion.MaxRows() }, 2},
		{"completion accept", completion.Element(), func() { completion.SetAccept(func(base.Candidate, base.Token) {}) }, func() any { return completion.Accept() != nil }, true},
		{"completion keys", completion.Element(), func() { completion.SetKeys(&keys) }, func() any { return completion.Keys() }, &keys},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := scenes.Mount(test.element)
			before := test.read()
			if reflect.DeepEqual(before, test.want) {
				t.Fatal("fixture does not change configuration")
			}
			revision, changed := root.Progress().ModelRevision, root.Changes()
			failure, _ := configurationFailure(func() { test.element.ReadOnly(test.change) }).(error)
			if !errors.Is(failure, scene.ErrBusy) {
				t.Fatalf("pure mutation failure = %v", failure)
			}
			pending, err := root.Prepare(grid.NewSurface(20, 4).View())
			if err != nil {
				t.Fatal(err)
			}
			failure, _ = configurationFailure(test.change).(error)
			pending.Abort()
			if !errors.Is(failure, scene.ErrBusy) || !reflect.DeepEqual(test.read(), before) || root.Progress().ModelRevision != revision {
				t.Fatalf("rejected mutation changed configuration: failure=%v before=%v after=%v", failure, before, test.read())
			}
			select {
			case <-changed:
				t.Fatal("rejected configuration change notified its scene")
			default:
			}
			test.change()
			if !reflect.DeepEqual(test.read(), test.want) || root.Progress().ModelRevision <= revision {
				t.Fatalf("configuration was not published by its owner: got %v, want %v", test.read(), test.want)
			}
			select {
			case <-changed:
			default:
				t.Fatal("configuration change did not wake its scene")
			}
		})
	}
}

func TestLabelChangeInvalidatesSemanticPagination(t *testing.T) {
	first := base.NewText(base.TextConfig{Label: "first"})
	second := base.NewText(base.TextConfig{Label: "second"})
	group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: first.Element()}, {Element: second.Element()}}})
	root := semanticOwner(t, group.Element())
	page, err := root.Observe(scene.Query{Limit: 1})
	if err != nil || !page.Truncated {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	second.SetLabel("renamed")
	if _, err = root.Observe(scene.Query{Limit: 1, Cursor: page.Next}); !errors.Is(err, scene.ErrObservationChanged) {
		t.Fatalf("stale semantic cursor accepted: %v", err)
	}
	observation, err := root.Observe(scene.Query{Root: second.Element().Target()})
	if err != nil || len(observation.Nodes) != 1 || observation.Nodes[0].Name != "renamed" || observation.Progress.ModelRevision <= page.Progress.ModelRevision {
		t.Fatalf("renamed field = %+v, %v", observation, err)
	}
}

func TestBorrowedKeyMapChecksEveryConsumerAndNotifiesOnlyCurrentBorrowers(t *testing.T) {
	var keys, replacement keymap.Map
	first := base.NewList(base.ListConfig[int]{Items: []int{1, 2}, Keys: &keys})
	second := base.NewList(base.ListConfig[int]{Items: []int{1, 2}, Keys: &keys})
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	a, b := scenes.Mount(first), scenes.Mount(second)
	scenes.Focus(first, true)
	scenes.Focus(second, true)
	pending, err := b.Prepare(grid.NewSurface(10, 2).View())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pending.Abort)
	beforeA, beforeB := a.Progress().ModelRevision, b.Progress().ModelRevision
	for _, change := range []func(){
		func() { keys.Bind(base.SelectNext, input.Chord{Rune: 'n'}) },
		func() { keys.SetTimeout(2 * time.Second) },
		func() { keys.SetResolver(func(time.Duration, func()) func() { return func() {} }) },
	} {
		failure, _ := configurationFailure(change).(error)
		if !errors.Is(failure, scene.ErrBusy) || a.Progress().ModelRevision != beforeA || b.Progress().ModelRevision != beforeB {
			t.Fatalf("shared map bypassed a borrower's pending frame: %v", failure)
		}
	}
	pending.Abort()
	if keys.Timeout() != keymap.DefaultTimeout || keys.Resolver() != nil || first.Handle(input.Key{Rune: 'n'}) {
		t.Fatal("rejected map mutation partially changed bindings or scheduling")
	}
	changedA, changedB := a.Changes(), b.Changes()
	keys.Bind(base.SelectNext, input.Chord{Rune: 'n'})
	for _, changed := range []<-chan struct{}{changedA, changedB} {
		select {
		case <-changed:
		default:
			t.Fatal("shared map did not wake each current borrower")
		}
	}
	if !first.Handle(input.Key{Rune: 'n'}) || !second.Handle(input.Key{Rune: 'n'}) || first.Selected() != 1 || second.Selected() != 1 {
		t.Fatal("live key rebinding did not reach both consumers")
	}
	first.SetKeys(&replacement)
	beforeA, beforeB = a.Progress().ModelRevision, b.Progress().ModelRevision
	keys.Bind(base.SelectFirst, input.Chord{Rune: 'f'})
	if a.Progress().ModelRevision != beforeA || b.Progress().ModelRevision <= beforeB || first.Keys() != &replacement || second.Keys() != &keys {
		t.Fatal("replaced key lease retained ownership of the old map")
	}
	replacement.Bind(base.SelectFirst, input.Chord{Rune: 'r'})
	if !first.Handle(input.Key{Rune: 'r'}) || first.Selected() != 0 {
		t.Fatal("replacement map was copied or did not control its borrower")
	}
}

func TestInvalidConfigurationIsRefusedBeforePublication(t *testing.T) {
	text := base.NewText(base.TextConfig{})
	editor := base.NewEditor(base.EditorConfig{})
	choice := base.NewSelect(base.SelectConfig[int]{Rows: 2})
	multi := base.NewMultiSelect(base.MultiSelectConfig[int]{Rows: 2})
	form := base.NewForm(base.FormConfig{Gap: 2})
	completion := base.NewCompletion(base.CompletionConfig{MaxRows: 2})
	transcript, err := base.NewTranscript(base.TranscriptConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transcript.Close)
	sticky := transcript.Sticky()
	sticky.SetGap(2)
	sticky.SetMinHeight(2)
	invalidCursor := grid.CursorStyle{Shape: grid.CursorBar + 1}
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	for _, test := range []struct {
		name    string
		element scene.Element
		change  func()
		read    func() any
	}{
		{"text cursor", text.Element(), func() { text.SetCursorStyle(invalidCursor) }, func() any { return text.CursorStyle() }},
		{"editor cursor", editor.Element(), func() { editor.SetCursorStyle(invalidCursor) }, func() any { return editor.CursorStyle() }},
		{"editor rows", editor.Element(), func() { editor.SetMaxRows(-1) }, func() any { return editor.MaxRows() }},
		{"select rows", choice.Element(), func() { choice.SetRows(-1) }, func() any { return choice.Rows() }},
		{"multi rows", multi.Element(), func() { multi.SetRows(-1) }, func() any { return multi.Rows() }},
		{"form gap", form.Element(), func() { form.SetGap(-1) }, func() any { return form.Gap() }},
		{"completion rows", completion.Element(), func() { completion.SetMaxRows(-1) }, func() any { return completion.MaxRows() }},
		{"sticky gap", transcript.Element(), func() { sticky.SetGap(-1) }, func() any { return sticky.Gap() }},
		{"sticky minimum", transcript.Element(), func() { sticky.SetMinHeight(-1) }, func() any { return sticky.MinHeight() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := scenes.Mount(test.element)
			before, revision := test.read(), root.Progress().ModelRevision
			if failure := configurationFailure(test.change); failure == nil {
				t.Fatal("invalid configuration was accepted")
			}
			if !reflect.DeepEqual(test.read(), before) || root.Progress().ModelRevision != revision {
				t.Fatal("invalid configuration changed its owner")
			}
		})
	}
	for name, construct := range map[string]func(){
		"text cursor":     func() { base.NewText(base.TextConfig{CursorStyle: invalidCursor}) },
		"editor cursor":   func() { base.NewEditor(base.EditorConfig{CursorStyle: invalidCursor}) },
		"editor rows":     func() { base.NewEditor(base.EditorConfig{MaxRows: -1}) },
		"select rows":     func() { base.NewSelect(base.SelectConfig[int]{Rows: -1}) },
		"multi rows":      func() { base.NewMultiSelect(base.MultiSelectConfig[int]{Rows: -1}) },
		"form gap":        func() { base.NewForm(base.FormConfig{Gap: -1}) },
		"completion rows": func() { base.NewCompletion(base.CompletionConfig{MaxRows: -1}) },
		"tabs gap":        func() { base.NewTabs(base.TabsConfig{Gap: -1}) },
		"sticky gap":      func() { base.NewSticky(base.StickyConfig{Gap: -1}) },
		"sticky minimum":  func() { base.NewSticky(base.StickyConfig{MinHeight: -1}) },
	} {
		t.Run("construction/"+name, func(t *testing.T) {
			if configurationFailure(construct) == nil {
				t.Fatal("construction accepted invalid configuration")
			}
		})
	}
}

func TestPresentationConfigurationReadsAreDetachedValues(t *testing.T) {
	editor := base.NewEditor(base.EditorConfig{Look: base.Look{Ellipsis: "..."}})
	root := semanticOwner(t, editor.Element())
	revision := root.Progress().ModelRevision
	look := editor.Look()
	look.Ellipsis = "more"
	if editor.Look().Ellipsis != "..." || root.Progress().ModelRevision != revision {
		t.Fatal("editing a presentation snapshot mutated the live owner")
	}
	editor.SetLook(look)
	if editor.Look().Ellipsis != "more" || root.Progress().ModelRevision <= revision {
		t.Fatal("owner did not publish the revised presentation")
	}
}
