package keymap_test

import (
	"fmt"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

func ExampleMap() {
	// A consumer names its operations; the map says which keystrokes produce each
	// name. Neither the event protocol nor the map needs to know what an operation does.
	keys := &keymap.Map{}
	keys.Bind("delete-word-back", input.Ctrl.Rune('w'))
	keys.Bind("delete-word-back", input.Alt.With(input.Backspace))
	keys.Bind("submit", input.Chord{Code: input.Enter})

	var matcher keymap.Matcher
	for _, key := range []input.Key{
		{Code: input.Character, Rune: 'w', Mods: input.Ctrl},
		{Code: input.Enter},
		{Code: input.Character, Rune: 'q'},
	} {
		var action keymap.Action
		mine, _ := matcher.Handle(keys, key, func(next keymap.Action) bool {
			action = next
			return true
		})
		fmt.Printf("%-9s %-18s mine=%v\n", key, "\""+string(action)+"\"", mine)
	}

	// Output:
	// ctrl+w    "delete-word-back" mine=true
	// enter     "submit"           mine=true
	// q         ""                 mine=false
}

func ExampleMap_Keys() {
	// Keys reports the sequences bound to an action in insertion order, so callers do
	// not have to reconstruct binding precedence.
	keys := &keymap.Map{}
	keys.Bind("delete-word-back", input.Ctrl.Rune('w'))
	keys.Bind("delete-word-back", input.Alt.With(input.Backspace))

	for _, bound := range keys.Keys("delete-word-back") {
		fmt.Println(bound)
	}

	// Output:
	// ctrl+w
	// alt+backspace
}

func ExampleMap_Bindings() {
	// Bindings is the complete snapshot used by a settings or help interface. Keys is
	// the narrower query when the caller already knows the action it wants.
	keys := &keymap.Map{}
	keys.Bind("submit", input.Chord{Code: input.Enter})
	keys.Bind("cancel", input.Chord{Code: input.Esc})

	for _, binding := range keys.Bindings() {
		fmt.Println(binding)
	}

	// Output:
	// enter submit
	// esc cancel
}

func ExampleMap_sequences() {
	// A binding can be more than one chord long. The first chord is the map's and
	// names nothing yet, which the caller has to consume rather than pass on.
	keys := &keymap.Map{}
	keys.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})

	var matcher keymap.Matcher
	for range 2 {
		var action keymap.Action
		mine, _ := matcher.Handle(keys, input.Key{Rune: 'g'}, func(next keymap.Action) bool {
			action = next
			return true
		})
		fmt.Printf("%q taken=%v waiting=%q\n", action, mine, matcher.Keys().String())
	}

	// Output:
	// "" taken=true waiting="g"
	// "go-to-top" taken=true waiting=""
}
