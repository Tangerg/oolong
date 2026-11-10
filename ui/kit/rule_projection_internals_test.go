package kit

import (
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestRuleRepeatsItsProjectedPatternWithinTheVisibleClip(t *testing.T) {
	for _, test := range []struct {
		name, source, shown string
		width               int
	}{
		{name: "tab", source: "\t-", shown: "        -", width: 9},
		{name: "separated variation selector", source: "❤\n\ufe0fX", shown: "❤X", width: 2},
		{name: "wide cluster", source: "界-", shown: "界-", width: 3},
		{name: "leading combining mark", source: "\u0301-", shown: "-", width: 1},
		{name: "invisible", source: "\n\u0301"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, offset := range []int{0, 1, 1_000_003} {
				actual := grid.NewSurface(20, 1)
				expected := grid.NewSurface(20, 1)
				area := grid.Area(-offset, 0, offset+20, 1)
				drawRule(actual.View().Sub(area), 0, test.source, grid.Style{})
				if test.width > 0 {
					view := expected.View().Sub(area)
					for x := offset - offset%test.width; x < offset+20; x += test.width {
						view.Text(x, 0, test.shown, grid.Style{})
					}
				}
				if got, want := actual.Rows(), expected.Rows(); !reflect.DeepEqual(got, want) {
					t.Fatalf("offset %d: rule = %q, want %q", offset, got, want)
				}
			}
		})
	}
}
