package latex_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
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
				if rows := formula.Rows(40); !slices.Equal(rows, want.Rows(40)) {
					t.Fatalf("operator rows = %+v, want %+v", rows, want.Rows(40))
				}
			}
		})
	}
}
