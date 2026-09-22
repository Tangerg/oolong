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
			for _, block := range mustFeed(t, &stream, source[i:i+1]) {
				_ = block.HeightForWidth(80)
				block.Draw(grid.NewSurface(80, block.HeightForWidth(80)).View())
			}
			_ = mustOpen(t, &stream)
		}
		for _, block := range mustFlush(t, &stream) {
			_ = block.Rows(80)
		}
	})
}
