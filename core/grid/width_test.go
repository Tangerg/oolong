package grid_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

// The width table is the one thing every measurement and every draw in this
// repository goes through, and it comes from a dependency. A bump that changed
// its mind about a character would move every column of every interface built on
// this, and would do it quietly: nothing else here would fail.
//
// So this pins the answers, and pins the ones where versions actually differ —
// the ambiguous-width set, emoji with and without a variation selector, the
// combining marks a cursor steps over, and the glyphs this repository itself
// draws with. A dependency update that changes any of them has to say so here
// before it can land.
func TestTheWidthTableHasNotChangedItsMind(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want int
	}{
		{"plain ascii", "a", 1},
		{"a space", " ", 1},

		// East Asian wide, which is the whole reason a cell can hold two columns.
		{"han", "中", 2},
		{"hangul", "한", 2},
		{"kana", "あ", 2},

		// Ambiguous width: narrow here, because the table is built explicitly
		// rather than taken from the locale. A machine whose environment said
		// otherwise would lay out differently from every other one.
		{"ellipsis", "…", 1},
		{"plus minus", "±", 1},
		{"degree", "°", 1},
		{"arrow", "→", 1},

		// A cluster is one thing however many code points it took.
		{"precomposed e-acute", "é", 1},
		{"decomposed e-acute", "é", 1},
		{"han with a combining mark", "中́", 2},

		// Emoji are wide; a warning sign is not, with or without the variation
		// selector that asks for an emoji presentation. That last one is the answer
		// most likely to move, which is why it is written down.
		{"emoji", "🙂", 2},
		{"thumbs up", "👍", 2},
		{"warning sign", "⚠", 1},
		{"warning sign asking to be emoji", "⚠️", 1},

		// What this repository draws its own furniture with.
		{"braille spinner", "⠋", 1},
		{"box side", "│", 1},
		{"box corner", "╭", 1},
		{"scrollbar thumb", "█", 1},

		// Control characters have no width and never reach a cell.
		{"a tab", "\t", 0},
		{"an escape", "\x1b", 0},
		{"a newline", "\n", 0},
	} {
		if got := grid.ClusterWidth(tc.in); got != tc.want {
			t.Errorf("%s (%q) is %d columns, want %d", tc.name, tc.in, got, tc.want)
		}
	}
}
