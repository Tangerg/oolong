package text_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

func BenchmarkLineDraw(b *testing.B) {
	for name, line := range map[string]text.Line{
		"ASCII":      text.Of(strings.Repeat("a", 120), grid.Style{}),
		"CJK":        text.Of(strings.Repeat("中文", 30), grid.Style{}),
		"SplitEmoji": {{Text: "👩"}, {Text: "\u200d💻"}},
	} {
		b.Run(name, func(b *testing.B) {
			v := grid.NewSurface(120, 40).View()
			b.ReportAllocs()
			for b.Loop() {
				for y := range 40 {
					line.Draw(v, 0, y)
				}
			}
		})
	}
}

func BenchmarkBlockStableDraw(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(strings.Repeat("x", n/1000), func(b *testing.B) {
			lines := make([]text.Line, n)
			for i := range lines {
				lines[i] = text.Of("a stable code line", grid.Style{})
			}
			block := text.NewBlock(text.BlockConfig{Lines: lines, Wrap: true})
			v := grid.NewSurface(80, 3).View()
			block.Draw(v)
			b.ReportAllocs()
			for b.Loop() {
				block.Draw(v)
			}
		})
	}
}
