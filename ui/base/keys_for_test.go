package base_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

// A hint reads the keys a controller actually handles. A controller left on its
// defaults has no map of its own, and its hint must still name the default keys
// rather than nothing.
func TestKeysForNamesTheDefaultsAControllerHandles(t *testing.T) {
	transcript := base.NewTranscript(base.TranscriptConfig{})
	for name, test := range map[string]struct {
		keysFor func(keymap.Action) []input.Keys
		action  keymap.Action
		want    *keymap.Map
	}{
		"list":       {base.NewList(base.ListConfig[string]{}).KeysFor, base.SelectNext, base.DefaultListKeys()},
		"filter":     {base.NewFilter(base.FilterConfig[string]{}).KeysFor, base.SelectNext, base.DefaultListKeys()},
		"tree":       {base.NewTree(base.TreeConfig[string]{}).KeysFor, base.Expand, base.DefaultTreeKeys()},
		"tabs":       {base.NewTabs(base.TabsConfig{}).KeysFor, base.NextTab, base.DefaultTabsKeys()},
		"completion": {base.NewCompletion(base.CompletionConfig{}).KeysFor, base.Accept, base.DefaultCompletionKeys()},
		"transcript": {transcript.KeysFor, base.ScrollPageDown, base.DefaultScrollKeys()},
	} {
		got, want := test.keysFor(test.action), test.want.Keys(test.action)
		if len(want) == 0 || !slices.EqualFunc(got, want, slices.Equal) {
			t.Errorf("%s: KeysFor(%s) = %v, want the default %v", name, test.action, got, want)
		}
	}

	custom := &keymap.Map{}
	custom.Bind(base.SelectNext, input.Chord{Rune: 'j'})
	list := base.NewList(base.ListConfig[string]{Keys: custom})
	if got := list.KeysFor(base.SelectNext); len(got) != 1 || !slices.Equal(got[0], input.Keys{{Rune: 'j'}}) {
		t.Fatalf("a configured map read %v, want only its own binding", got)
	}
}
