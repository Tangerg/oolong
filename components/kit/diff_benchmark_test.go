package kit_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
)

func BenchmarkDiffFrame(b *testing.B) {
	for _, lines := range []int{100, 1_000} {
		b.Run(strconv.Itoa(lines), func(b *testing.B) {
			script := make(diff.Script, lines)
			for i := range script {
				script[i] = diff.Line{
					Kind: diff.Added,
					Text: strings.Repeat("content ", 10),
					New:  i + 1,
				}
			}
			d := kit.NewDiff(kit.DiffConfig{
				Theme: kit.Dark(), Glyphs: kit.Unicode(),
				Hunks: []diff.Hunk{{Lines: script}}, Numbers: true,
			})
			view := grid.NewSurface(100, 60).View()
			_ = d.Measure(100) // Measure one cold frame before reporting steady-state work.

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = d.Measure(100)
				d.Draw(view)
			}
		})
	}
}
