package markdown_test

import (
	"testing"

	"github.com/Tangerg/oolong/markdown"
)

func mustRender(tb testing.TB, source string, look markdown.Look) []markdown.Block {
	tb.Helper()
	blocks, err := markdown.Render(source, look)
	if err != nil {
		tb.Fatal(err)
	}
	return blocks
}

func mustFeed(tb testing.TB, stream *markdown.Stream, source string) []markdown.Block {
	tb.Helper()
	blocks, err := stream.Feed(source)
	if err != nil {
		tb.Fatal(err)
	}
	return blocks
}

func mustOpen(tb testing.TB, stream *markdown.Stream) []markdown.Block {
	tb.Helper()
	blocks, err := stream.Open()
	if err != nil {
		tb.Fatal(err)
	}
	return blocks
}

func mustFlush(tb testing.TB, stream *markdown.Stream) []markdown.Block {
	tb.Helper()
	blocks, err := stream.Flush()
	if err != nil {
		tb.Fatal(err)
	}
	return blocks
}
