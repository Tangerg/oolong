package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestHelpMeasuresOnlyBoundHints(t *testing.T) {
	keys := &keymap.Map{}
	keys.Bind("send", input.Chord{Code: input.Enter})
	for _, test := range []struct {
		name string
		help kit.Help
		rows int
	}{
		{name: "zero value"},
		{name: "missing map", help: kit.Help{Show: []keymap.Action{"send"}}},
		{name: "no requested hints", help: kit.Help{Keys: keys}},
		{name: "unbound hints", help: kit.Help{Keys: keys, Show: []keymap.Action{"unknown"}}},
		{name: "bound hint", help: kit.Help{Keys: keys, Show: []keymap.Action{"send"}}, rows: 1},
		{name: "mixed hints", help: kit.Help{Keys: keys, Show: []keymap.Action{"unknown", "send"}}, rows: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, width := range []int{0, 1, 40} {
				if got := test.help.HeightForWidth(width); got != test.rows {
					t.Fatalf("HeightForWidth(%d) = %d, want %d", width, got, test.rows)
				}
			}
		})
	}
}

func TestHelpTracksItsBindingOwner(t *testing.T) {
	keys := &keymap.Map{}
	help := kit.Help{Keys: keys, Show: []keymap.Action{"send"}}
	if got := help.HeightForWidth(40); got != 0 {
		t.Fatalf("unbound height = %d, want 0", got)
	}
	enter := input.Chord{Code: input.Enter}
	keys.Bind("send", enter)
	if got := help.HeightForWidth(40); got != 1 {
		t.Fatalf("bound height = %d, want 1", got)
	}
	if !keys.Unbind(enter) {
		t.Fatal("binding was not removed")
	}
	if got := help.HeightForWidth(40); got != 0 {
		t.Fatalf("removed binding height = %d, want 0", got)
	}
}
