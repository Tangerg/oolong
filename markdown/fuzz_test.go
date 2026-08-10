package markdown

import (
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

func FuzzStreamNeverPanics(f *testing.F) {
	for _, source := range []string{
		"plain text",
		"$$\n\\frac{a}{b}\n$$\n",
		"```math\nx^2\n```\n",
		"$$\nnot closed",
		"> - nested\n>\n> content\n",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 8<<10 {
			t.Skip()
		}
		var stream Stream
		for i := range len(source) {
			for _, block := range stream.Feed(source[i : i+1]) {
				_ = block.Measure(80)
				block.Draw(grid.NewSurface(80, block.Measure(80)).View())
			}
			_ = stream.Open()
		}
		for _, block := range stream.Flush() {
			_ = block.Rows(80)
		}
	})
}
