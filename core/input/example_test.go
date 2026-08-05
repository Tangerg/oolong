package input_test

import (
	"fmt"

	"github.com/Tangerg/oolong/core/input"
)

func ExampleKeymap() {
	// A widget names what it can do; the map says which keystrokes produce the name.
	// Neither knows the other, so either can be replaced without touching the other.
	keys := &input.Keymap{}
	keys.Bind("delete-word-back", input.Ctrl.Rune('w'))
	keys.Bind("delete-word-back", input.Alt.With(input.Backspace))
	keys.Bind("submit", input.Chord{Code: input.Enter})

	var pending input.Pending
	for _, key := range []input.Key{
		{Code: input.Character, Rune: 'w', Mods: input.Ctrl},
		{Code: input.Enter},
		{Code: input.Character, Rune: 'q'},
	} {
		action, mine := keys.Lookup(key, &pending)
		fmt.Printf("%-9s %-18s mine=%v\n", key, "\""+string(action)+"\"", mine)
	}

	// Output:
	// ctrl+w    "delete-word-back" mine=true
	// enter     "submit"           mine=true
	// q         ""                 mine=false
}

func ExampleKeymap_Keys() {
	// What a hint row asks: the keystrokes bound to an action, in the order they were
	// bound, so the one that works everywhere can be put first.
	keys := &input.Keymap{}
	keys.Bind("delete-word-back", input.Ctrl.Rune('w'))
	keys.Bind("delete-word-back", input.Alt.With(input.Backspace))

	for _, bound := range keys.Keys("delete-word-back") {
		fmt.Println(bound)
	}
	fmt.Println(input.Action("delete-word-back").Does())

	// Output:
	// ctrl+w
	// alt+backspace
	// delete word back
}

func ExampleKeymap_sequences() {
	// A binding can be more than one chord long. The first chord is the map's and
	// names nothing yet, which the caller has to consume rather than pass on.
	keys := &input.Keymap{}
	keys.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})

	var pending input.Pending
	for range 2 {
		action, mine := keys.Lookup(input.Key{Rune: 'g'}, &pending)
		fmt.Printf("%q taken=%v waiting=%q\n", action, mine, pending.Keys().String())
	}

	// Output:
	// "" taken=true waiting="g"
	// "go-to-top" taken=true waiting=""
}
