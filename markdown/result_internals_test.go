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

func mustFeed(tb testing.TB, stream *Stream, source string) []Block {
	tb.Helper()
	blocks, err := stream.Feed(source)
	if err != nil {
		tb.Fatal(err)
	}
	return blocks
}

func mustOpen(tb testing.TB, stream *Stream) []Block {
	tb.Helper()
	blocks, err := stream.Open()
	if err != nil {
		tb.Fatal(err)
	}
	return blocks
}

func mustFlush(tb testing.TB, stream *Stream) []Block {
	tb.Helper()
	blocks, err := stream.Flush()
	if err != nil {
		tb.Fatal(err)
	}
	return blocks
}
