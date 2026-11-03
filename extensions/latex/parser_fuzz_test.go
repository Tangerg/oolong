package latex_test

import (
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
)

// A repertoire changes symbol spelling, never the supported grammar.
func FuzzGlyphsChooseSpellingAndNotSupport(f *testing.F) {
	for _, source := range []string{
		`x`, `\frac{a}{b}`, `\sqrt{x_1^2}`, `\sum_{i=0}^{n} i`, `\unknown{value}`,
		"\\frac{", "\x00\xff", "日本語", `\bf x`, `\alpha \leq \beta`,
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		unicodeFormula := latex.Render(source, latex.Look{})
		plainFormula := latex.Render(source, latex.Look{Glyphs: latex.Glyphs{Plain: true}})
		for _, formula := range []*latex.Formula{unicodeFormula, plainFormula} {
			_ = formula.Source()
			_ = formula.Lines()
			_ = formula.Width()
			_ = formula.HeightForWidth(80)
			_ = formula.Rows(80)
		}
		if (unicodeFormula.Err() == nil) != (plainFormula.Err() == nil) {
			t.Fatalf("%q renders as %v with the terminal's own glyphs and %v in plain ASCII",
				source, unicodeFormula.Err(), plainFormula.Err())
		}
	})
}

var benchmarkFormula *latex.Formula

func BenchmarkRender(b *testing.B) {
	const source = `\frac{-b \pm \sqrt{b^2 - 4ac}}{2a}`
	b.ReportAllocs()
	for b.Loop() {
		benchmarkFormula = latex.Render(source, latex.Look{})
	}
}
