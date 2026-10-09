package headless

import (
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
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

func TestMatcherCancellationPreservesNewerFocus(t *testing.T) {
	var list List[string]
	var editor Editor
	slider := NewSlider(SliderConfig{})
	pane := &retainedWidget{}
	container := NewContainer(0, Item{Of: pane})
	tabs := NewTabs(TabsConfig{Items: []Tab{{Of: pane}}})
	stack := NewStack(pane)
	viewport := &Viewport{content: pane}
	var form Form
	var tree Tree[string]
	var settings Settings[string]
	text := &Text{}
	one := &Select[string]{Same: Equal[string]}
	many := &MultiSelect[string]{Same: Equal[string]}
	confirm := &Confirm{}
	trigger := &DialogTrigger{}
	for _, test := range []struct {
		name    string
		focus   func(bool)
		focused func() bool
		matcher *keymap.Matcher
	}{
		{"list", list.Focus, list.Focused, &list.matcher},
		{"editor", editor.Focus, func() bool { return !editor.blurred }, &editor.matcher},
		{"slider", slider.Focus, slider.Focused, &slider.matcher},
		{"container", container.Focus, container.state.Focused, &container.matcher},
		{"tabs", tabs.Focus, tabs.state.Focused, &tabs.matcher},
		{"stack", stack.Focus, stack.state.Focused, &stack.matcher},
		{"viewport", viewport.Focus, viewport.state.Focused, &viewport.scroll.matcher},
		{"form", form.Focus, form.body.state.Focused, &form.matcher},
		{"tree", tree.Focus, tree.Focused, &tree.matcher},
		{"settings", settings.Focus, settings.Focused, &settings.matcher},
		{"text", text.Focus, func() bool { return text.keyboard.Focused() && !text.editor.blurred }, &text.editor.matcher},
		{"select", one.Focus, func() bool { return one.keyboard.Focused() && one.list.Focused() }, &one.matcher},
		{"multi", many.Focus, func() bool { return many.keyboard.Focused() && many.list.Focused() }, &many.matcher},
		{"confirm", confirm.Focus, func() bool { return confirm.keyboard.Focused() }, &confirm.matcher},
		{"trigger", trigger.Focus, trigger.state.Focused, &trigger.matcher},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.focus(true)
			var keys keymap.Map
			keys.Bind("prefix", input.Chord{Rune: 'g'})
			keys.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
			keys.Resolve = func(time.Duration, func()) func() { return func() { test.focus(true) } }
			test.matcher.Handle(&keys, input.Key{Rune: 'g'}, func(keymap.Action) bool { return true })
			test.focus(false)
			if !test.focused() {
				t.Fatal("older blur replaced focus restored by cancellation")
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
	settings.SetItems([]string{"a", "b"})
	var keys keymap.Map
	keys.Bind("prefix", input.Chord{Rune: 'g'})
	keys.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	keys.Bind("other", input.Chord{Rune: 'h'}, input.Chord{Rune: 'h'})
	settings.EditKeys = &keys
	keys.Resolve = func(time.Duration, func()) func() {
		return func() { settings.Handle(input.Key{Rune: 'h'}) }
	}
	settings.Handle(input.Key{Rune: 'g'})
	settings.Select(1)
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
		{"tabs", func(value string) { tabs.Set(Tab{Title: value}) }, func() string { return tabs.items[0].Title }, &tabs.matcher},
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
