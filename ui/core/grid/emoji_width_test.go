package grid_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestEmojiPresentationRequiresAnEligibleAdjacentBase(t *testing.T) {
	for _, test := range []struct {
		name    string
		cluster string
		want    int
	}{
		{"letter", "A\ufe0f", 1},
		{"blank", " \ufe0f", 1},
		{"combining marks alone", "\u0301\ufe0f", 0},
		{"selectors alone", "\ufe0e\ufe0f", 0},
		{"intervening combining mark", "⚠\u0301\ufe0f", 1},
		{"intervening text selector", "⚠\ufe0e\ufe0f", 1},
		{"emoji with a combining mark", "⚠\ufe0f\u0301", 2},
		{"emoji and a spacing mark", "⚠\ufe0fﾞ", 3},
		{"emoji and two spacing marks", "⚠\ufe0fﾞﾟ", 4},
		{"selector after a spacing mark", "⚠ﾞ\ufe0f", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := grid.ClusterWidth(test.cluster); got != test.want {
				t.Fatalf("width of %q = %d, want %d", test.cluster, got, test.want)
			}
		})
	}
}

func TestEmojiPresentationPreservesFollowingColumns(t *testing.T) {
	const cluster = "⚠\ufe0fﾞﾟ"
	surface := grid.NewSurface(5, 1)
	view := surface.View()
	if advance := view.Text(0, 0, cluster+"x", grid.Style{}); advance != 5 {
		t.Fatalf("drawing advance = %d, want 5", advance)
	}
	if cell, _ := surface.CellAt(0, 0); cell.Width() != 4 || cell.Content() != cluster {
		t.Fatalf("emoji display atom = %+v, want all four columns", cell)
	}
	if cell, _ := surface.CellAt(4, 0); cell.Content() != "x" {
		t.Fatalf("following cell = %+v, want x", cell)
	}
	view.Text(3, 0, "z", grid.Style{})
	for column := range 3 {
		if cell, _ := surface.CellAt(column, 0); !cell.Blank() {
			t.Fatalf("overwriting a continuation retained orphaned cell %d: %+v", column, cell)
		}
	}
}
