package keymap_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

func bindingsOf(m *keymap.Map) []string {
	var out []string
	for _, binding := range m.Bindings() {
		out = append(out, binding.String())
	}
	return out
}

// A configured shortcut replaces the default rather than joining it: the default
// would otherwise stay live and lead the help text.
func TestRebindReplacesAnActionsSequencesInOneChange(t *testing.T) {
	var bindings keymap.Map
	bindings.Bind("send", input.Chord{Code: input.Enter})
	bindings.Bind("quit", input.Ctrl.Rune('q'))
	bindings.Bind("send", input.Ctrl.Rune('s'))
	changes := 0
	subscription := bindings.Subscribe(keymap.SubscriptionConfig{Check: func() error { return nil }, Changed: func() { changes++ }})
	defer subscription.Close()

	bindings.Rebind("send", input.Keys{input.Ctrl.Rune('j')}, input.Keys{input.Ctrl.Rune('q')}, nil, input.Keys{input.Ctrl.Rune('j')})
	if got, want := bindingsOf(&bindings), []string{"ctrl+j send", "ctrl+q send"}; !slices.Equal(got, want) {
		t.Fatalf("bindings = %v, want %v", got, want)
	}
	if changes != 1 {
		t.Fatalf("rebinding notified %d times, want once", changes)
	}
	if _, ok := bindings.Action(input.Chord{Code: input.Enter}); ok {
		t.Fatal("a replaced default still matched")
	}

	bindings.Rebind("send", input.Keys{input.Ctrl.Rune('j')}, input.Keys{input.Ctrl.Rune('q')})
	if changes != 1 {
		t.Fatal("rebinding to the current sequences notified consumers")
	}
	bindings.Rebind("send")
	if got := bindingsOf(&bindings); len(got) != 0 {
		t.Fatalf("rebinding to nothing left %v", got)
	}
}

func TestRebindRefusesAnInvalidChordBeforeChangingAnything(t *testing.T) {
	var bindings keymap.Map
	bindings.Bind("send", input.Chord{Code: input.Enter})
	defer func() {
		if recover() == nil {
			t.Fatal("an invalid chord was accepted")
		}
		if got, want := bindingsOf(&bindings), []string{"enter send"}; !slices.Equal(got, want) {
			t.Fatalf("bindings after refusal = %v, want %v", got, want)
		}
	}()
	bindings.Rebind("send", input.Keys{input.Ctrl.Rune('j')}, input.Keys{{Code: input.Character, Rune: '\n'}})
}

func TestContinuationsListWhatAPrefixCanStillComplete(t *testing.T) {
	var bindings keymap.Map
	bindings.Bind("go", input.Chord{Rune: 'g'})
	bindings.Bind("top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	bindings.Bind("quit", input.Chord{Rune: 'q'})
	bindings.Bind("end", input.Chord{Rune: 'g'}, input.Chord{Rune: 'e'})

	got := bindings.Continuations(input.Keys{{Rune: 'g'}})
	var actions []keymap.Action
	for _, binding := range got {
		actions = append(actions, binding.Action)
	}
	if want := []keymap.Action{"top", "end"}; !slices.Equal(actions, want) {
		t.Fatalf("continuations = %v, want %v", actions, want)
	}
	if !slices.Equal(got[1].Keys, input.Keys{{Rune: 'g'}, {Rune: 'e'}}) {
		t.Fatalf("a continuation lost its prefix: %v", got[1].Keys)
	}
	got[0].Keys[0] = input.Chord{Rune: 'z'}
	if again := bindings.Continuations(input.Keys{{Rune: 'g'}}); again[0].Keys[0] != (input.Chord{Rune: 'g'}) {
		t.Fatal("a returned continuation shared storage with the map")
	}
	if none := bindings.Continuations(input.Keys{{Rune: 'q'}}); len(none) != 0 {
		t.Fatalf("a complete sequence had continuations %v", none)
	}
}
