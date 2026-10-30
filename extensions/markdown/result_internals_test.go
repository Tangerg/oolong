package markdown

import (
	"testing"
)

func mustRender(tb testing.TB, source string, look Look) []Block {
	tb.Helper()
	blocks, err := Render(source, look)
	if err != nil {
		tb.Fatal(err)
	}
	return blocks
}
