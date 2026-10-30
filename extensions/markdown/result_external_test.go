package markdown_test

import (
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
)

func mustRender(tb testing.TB, source string, look markdown.Look) []markdown.Block {
	tb.Helper()
	blocks, err := markdown.Render(source, look)
	if err != nil {
		tb.Fatal(err)
	}
	return blocks
}
