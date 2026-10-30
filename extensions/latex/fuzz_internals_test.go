package latex

import "testing"

// FuzzGlyphsChooseSpellingAndNotSupport holds the two repertoires to saying the same
// thing about the same source.
//
// Whether this package implements a control sequence is a fact about the sequence.
// [Glyphs.Plain] chooses how a symbol it does implement is spelled, and letting it
// answer the other question as well meant it implemented everything there is: a macro
// nobody wrote came back as the letters its name is made of, with no error.
func FuzzGlyphsChooseSpellingAndNotSupport(f *testing.F) {
	for _, source := range []string{
		`x`, `\frac{a}{b}`, `\sqrt{x_1^2}`, `\sum_{i=0}^{n} i`, `\unknown{value}`,
		"\\frac{", "\x00\xff", "日本語", `\bf x`, `\alpha \leq \beta`,
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		unicode := Render(source, Look{})
		plain := Render(source, Look{Glyphs: Glyphs{Plain: true}})
		for _, formula := range []*Formula{unicode, plain} {
			_ = formula.Source()
			_ = formula.Lines()
			_ = formula.Width()
			_ = formula.HeightForWidth(80)
			_ = formula.Rows(80)
		}
		if (unicode.Err() == nil) != (plain.Err() == nil) {
			t.Fatalf("%q renders as %v with the terminal's own glyphs and %v in plain ASCII",
				source, unicode.Err(), plain.Err())
		}
	})
}

var benchmarkFormula *Formula

func BenchmarkRender(b *testing.B) {
	const source = `\frac{-b \pm \sqrt{b^2 - 4ac}}{2a}`
	b.ReportAllocs()
	for b.Loop() {
		benchmarkFormula = Render(source, Look{})
	}
}
