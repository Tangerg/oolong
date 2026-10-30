package base

import (
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestCompletionCancellationCannotAcceptASupersededOffer(t *testing.T) {
	var completion Completion
	var keys keymap.Map
	keys.Bind(Accept, input.Chord{Rune: 'g'})
	keys.Bind(SelectNext, input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	keys.Resolve = func(time.Duration, func()) func() {
		return func() {
			completion.Offer(Token{Query: "new query"}, []Candidate{{Text: "new candidate"}})
		}
	}
	completion.Keys = &keys
	var accepted []string
	completion.Accept = func(candidate Candidate, token Token) {
		accepted = append(accepted, candidate.Text+":"+token.Query)
	}
	completion.Offer(Token{Query: "old query"}, []Candidate{{Text: "old candidate"}})
	if !completion.Handle(input.Key{Rune: 'g'}) {
		t.Fatal("ambiguous prefix was not handled")
	}
	if !completion.Do(Accept) {
		t.Fatal("interrupted acceptance was not handled")
	}
	if len(accepted) != 0 || !completion.Open() {
		t.Fatalf("old acceptance survived replacement: accepted=%v open=%v", accepted, completion.Open())
	}
	if !completion.Do(Accept) || !slices.Equal(accepted, []string{"new candidate:new query"}) || completion.Open() {
		t.Fatalf("new acceptance = %v open=%v", accepted, completion.Open())
	}
}

func TestMatcherCancellationPreservesSceneFocus(t *testing.T) {
	list := &List[string]{}
	list.SetItems([]string{"one"})
	text := NewText(TextConfig{Initial: "text"})
	one := NewSelect(SelectConfig[string]{Same: Equal[string], Options: Options("one")})
	many := NewMultiSelect(MultiSelectConfig[string]{Same: Equal[string], Options: Options("one")})
	confirm := NewConfirm(ConfirmConfig{})
	for _, test := range []struct {
		name    string
		element scene.Element
		matcher *keymap.Matcher
	}{
		{"list", list.Element(), &list.matcher}, {"text", text.Element(), &text.editor.matcher}, {"select", one.Element(), &one.matcher}, {"multi", many.Element(), &many.matcher}, {"confirm", confirm.Element(), &confirm.matcher},
	} {
		t.Run(test.name, func(t *testing.T) {
			other := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
			group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: test.element}, {Element: other}}})
			root := newTestRoot(group.Element())
			var keys keymap.Map
			keys.Bind("prefix", input.Chord{Rune: 'g'})
			keys.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
			keys.Resolve = func(time.Duration, func()) func() {
				return func() {
					if err := test.element.RequestFocus(); err != nil {
						t.Error(err)
					}
				}
			}
			test.matcher.Handle(&keys, input.Key{Rune: 'g'}, func(keymap.Action) bool { return true })
			if err := other.RequestFocus(); err != nil {
				t.Fatal(err)
			}
			root.root.FlushNotifications()
			if !test.element.Focused() {
				t.Fatal("old blur replaced cancellation's newer scene focus")
			}
		})
	}
}

func TestOptionReplacementCancelsAfterRestoringSelection(t *testing.T) {
	many := &MultiSelect[string]{Same: Equal[string]}
	many.SetOptions(Options("a", "b"))
	many.Toggle()
	var keys keymap.Map
	keys.Bind("prefix", input.Chord{Rune: 'g'})
	keys.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	var observed []string
	keys.Resolve = func(time.Duration, func()) func() {
		return func() { observed = many.Taken() }
	}
	many.matcher.Handle(&keys, input.Key{Rune: 'g'}, func(keymap.Action) bool { return true })
	many.SetOptions(Options("b", "a"))
	if !slices.Equal(observed, []string{"a"}) || !slices.Equal(many.Taken(), observed) {
		t.Fatalf("cancellation saw an unfinished selection: observed=%v current=%v", observed, many.Taken())
	}
}

func TestCompletionDismissCancellationMayReopen(t *testing.T) {
	var completion Completion
	completion.Offer(Token{Query: "old"}, []Candidate{{Text: "old"}})
	var keys keymap.Map
	keys.Bind("prefix", input.Chord{Rune: 'g'})
	keys.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	keys.Resolve = func(time.Duration, func()) func() {
		return func() { completion.Offer(Token{Query: "new"}, []Candidate{{Text: "new"}}) }
	}
	completion.matcher.Handle(&keys, input.Key{Rune: 'g'}, func(keymap.Action) bool { return true })
	if completion.Dismiss() {
		t.Fatal("superseded dismissal retained acceptance ownership")
	}
	current, ok := completion.Current()
	token, open := completion.Token()
	if !ok || !open || current.Text != "new" || token.Query != "new" {
		t.Fatalf("older dismissal replaced reopened completion: candidate=%+v token=%+v open=%v", current, token, open)
	}
}

func TestDerivedCollectionsCancelAfterTheirCompleteTransition(t *testing.T) {
	var filter Filter[string]
	filter.SetItems([]string{"aa", "ab", "ba", "bb"}, func(s string) string { return s })
	var table Table[int]
	table.SetLess(func(a, b, _ int) bool { return a < b })
	table.SetItems([]int{3, 2, 1})
	table.Select(0)
	for _, test := range []struct {
		name    string
		matcher *keymap.Matcher
		change  func()
		cancel  func()
		verify  func() bool
	}{
		{
			name:    "filter",
			matcher: &filter.list.matcher,
			change:  func() { filter.SetPattern("a") },
			cancel:  func() { filter.SetPattern("b"); filter.Select(1) },
			verify:  func() bool { return filter.Pattern() == "b" && filter.Selected() == 1 },
		},
		{
			name:    "table",
			matcher: &table.rows.matcher,
			change:  func() { table.SortBy(0) },
			cancel:  func() { table.Select(1) },
			verify:  func() bool { return table.Selected() == 1 },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var keys keymap.Map
			keys.Bind("prefix", input.Chord{Rune: 'g'})
			keys.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
			keys.Resolve = func(time.Duration, func()) func() { return test.cancel }
			test.matcher.Handle(&keys, input.Key{Rune: 'g'}, func(keymap.Action) bool { return true })
			test.change()
			if !test.verify() {
				t.Fatal("older derived-collection operation overwrote cancellation's completed transition")
			}
		})
	}
}

func TestSettingsContextCancellationCannotContinueANewerSequence(t *testing.T) {
	var settings Settings[string]
	settings.List().SetItems([]string{"a", "b"})
	var keys keymap.Map
	keys.Bind("prefix", input.Chord{Rune: 'g'})
	keys.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	keys.Bind("other", input.Chord{Rune: 'h'}, input.Chord{Rune: 'h'})
	settings.EditKeys = &keys
	keys.Resolve = func(time.Duration, func()) func() {
		return func() { settings.Handle(input.Key{Rune: 'h'}) }
	}
	settings.Handle(input.Key{Rune: 'g'})
	settings.List().Select(1)
	settings.Handle(input.Key{Rune: 'x'})
	if got := settings.matcher.Keys().String(); got != "h" {
		t.Fatalf("interrupted key replaced cancellation's sequence: %q", got)
	}
}

func TestCollectionReplacementPublishesBeforeMatcherCancellation(t *testing.T) {
	var list List[string]
	var tabs Tabs
	var completion Completion
	var tree Tree[string]
	one := &Select[string]{Same: Equal[string]}
	many := &MultiSelect[string]{Same: Equal[string]}
	for _, test := range []struct {
		name    string
		set     func(string)
		current func() string
		matcher *keymap.Matcher
	}{
		{"list", func(value string) { list.SetItems([]string{value}) }, func() string { return list.items[0] }, &list.matcher},
		{"tabs", func(value string) { tabs.Set(Tab{Title: value}) }, func() string { return tabs.Items()[0].Title }, &tabs.matcher},
		{"completion", func(value string) { completion.Offer(Token{}, []Candidate{{Label: value}}) }, func() string { return completion.list.items[0].Label }, &completion.matcher},
		{"select", func(value string) { one.SetOptions(Options(value)) }, func() string { return one.list.items[0].Label }, &one.matcher},
		{"multi", func(value string) { many.SetOptions(Options(value)) }, func() string { return many.list.items[0].Label }, &many.matcher},
		{"tree", func(value string) { tree.SetNodes([]Node[string]{{Item: value}}) }, func() string { return tree.nodes[0].item }, &tree.matcher},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.set("initial")
			var keys keymap.Map
			keys.Bind("prefix", input.Chord{Rune: 'g'})
			keys.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
			keys.Resolve = func(time.Duration, func()) func() { return func() { test.set("newest") } }
			test.matcher.Handle(&keys, input.Key{Rune: 'g'}, func(keymap.Action) bool { return true })
			test.set("outer")
			if test.current() != "newest" {
				t.Fatalf("cancellation replacement overwritten: %q", test.current())
			}
		})
	}
}
