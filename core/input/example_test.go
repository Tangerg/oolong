package input_test

import (
	"fmt"

	"github.com/Tangerg/oolong/core/input"
)

// A component is handed events and says whether it wanted them. Matching one is a type
// switch and then a comparison — there is nothing to register and nothing to unbind.
func ExampleKey() {
	handle := func(event input.Event) string {
		switch e := event.(type) {
		case input.Key:
			switch {
			case e.IsRune('c', input.Ctrl):
				return "interrupt"
			case e.Code == input.Enter:
				return "submit"
			case e.Code == input.Character:
				return "typed " + string(e.Rune)
			}
		case input.Resize:
			return fmt.Sprintf("resized to %dx%d", e.Width, e.Height)
		}
		return "ignored"
	}

	fmt.Println(handle(input.Key{Rune: 'c', Mods: input.Ctrl}))
	fmt.Println(handle(input.Key{Code: input.Enter}))
	fmt.Println(handle(input.Key{Rune: 'a'}))
	fmt.Println(handle(input.Resize{Width: 80, Height: 24}))
	// Output:
	// interrupt
	// submit
	// typed a
	// resized to 80x24
}

// A chord is a keystroke with the occurrence taken off — no timestamp, no transition,
// no text the terminal happened to report. That is what makes it the thing you can
// write down in advance, put in a configuration file, and bind to an action.
func ExampleParseChord() {
	for _, s := range []string{"ctrl+c", "shift+tab", "f5", "not a key"} {
		chord, ok := input.ParseChord(s)
		if !ok {
			fmt.Printf("%-12q unrecognised\n", s)
			continue
		}
		fmt.Printf("%-12q %s\n", s, chord)
	}
	// Output:
	// "ctrl+c"     ctrl+c
	// "shift+tab"  shift+tab
	// "f5"         f5
	// "not a key"  unrecognised
}

// Several chords typed one after another are one binding. A terminal never says so —
// it reports two keystrokes that happen to be adjacent — which is why the time a key
// arrived travels with it.
func ExampleParseKeys() {
	keys, ok := input.ParseKeys("ctrl+x ctrl+s")
	fmt.Println(ok, len(keys), keys)
	// Output:
	// true 2 ctrl+x ctrl+s
}
