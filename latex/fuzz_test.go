package latex

import "testing"

func FuzzRenderNeverPanics(f *testing.F) {
	for _, source := range []string{
		`x`, `\frac{a}{b}`, `\sqrt{x_1^2}`, `\sum_{i=0}^{n} i`, `\unknown{value}`,
		"\\frac{", "\x00\xff", "日本語",
	} {
		f.Add(source)
	}
	f.Fuzz(func(_ *testing.T, source string) {
		formula := Render(source, Look{})
		_ = formula.Source()
		_ = formula.Err()
		_ = formula.Lines()
		_ = formula.Width()
		_ = formula.Measure(80)
		_ = formula.Rows(80)
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
