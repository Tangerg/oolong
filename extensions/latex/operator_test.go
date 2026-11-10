package latex_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestExponentialUsesTheNamedOperatorGrammar(t *testing.T) {
	for _, source := range []string{`\exp`, `\exp(x)`, `\exp^2(x)`, `\exp_1(x)`} {
		t.Run(source, func(t *testing.T) {
			for _, look := range []latex.Look{{}, {Glyphs: latex.ASCII()}} {
				formula := latex.Render(source, look)
				if err := formula.Err(); err != nil {
					t.Fatalf("Render(%q): %v", source, err)
				}
				want := latex.Render(source[1:], look)
				actual := grid.NewSurface(40, formula.HeightForWidth(40))
				expected := grid.NewSurface(40, want.HeightForWidth(40))
				formula.Draw(actual.View())
				want.Draw(expected.View())
				if formula.HeightForWidth(40) != want.HeightForWidth(40) {
					t.Fatal("operator changed the expression height")
				}
				for y := range formula.HeightForWidth(40) {
					if !reflect.DeepEqual(actual.Row(y), expected.Row(y)) {
						t.Fatalf("operator changed cells on row %d", y)
					}
				}
			}
		})
	}
}
