package input_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestTypedIsWhatAKeyEntersIntoAnEditor(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  input.Key
		want string
	}{
		{"a plain letter", input.Key{Code: input.Character, Rune: 'a'}, "a"},
		{"a repeat still types", input.Key{Code: input.Character, Rune: 'a', Transition: input.Repeat}, "a"},
		{"a release types nothing", input.Key{Code: input.Character, Rune: 'a', Transition: input.Release}, ""},
		{"reported text wins over the code", input.Key{Code: input.Character, Rune: '2', Mods: input.Shift, Text: "@"}, "@"},
		{"a shifted identity without text", input.Key{Code: input.Character, Rune: '2', ShiftedRune: '@', Mods: input.Shift}, "@"},
		{"a base-layout identity never types", input.Key{Code: input.Character, Rune: 'ф', BaseRune: 'a'}, "ф"},
		{"ctrl is a shortcut", input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}, ""},
		{"alt is a shortcut even with text", input.Key{Code: input.Character, Rune: 'c', Mods: input.Alt, Text: "c"}, ""},
		{"a keypad digit types its character", input.Key{Code: input.Keypad5}, "5"},
		{"a functional key types nothing", input.Key{Code: input.Enter}, ""},
		{"text-only input", input.Key{Code: input.Character, Text: "你好"}, "你好"},
	} {
		if got := tc.key.Typed(); got != tc.want {
			t.Errorf("%s: Typed() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
