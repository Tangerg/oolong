package markdown

import (
	"fmt"
	"strings"
	"testing"
)

var benchmarkBlocks []Block

// BenchmarkOpenMarkdownUpdate measures the one part of a stream that is allowed to
// be revisited: an unfinished block. An unclosed fence is deliberately the worst
// honest shape because none of it can be published before the closing fence arrives.
func BenchmarkOpenMarkdownUpdate(b *testing.B) {
	for _, size := range []int{256, 4 << 10, 64 << 10} {
		b.Run(fmt.Sprintf("tail=%d", size), func(b *testing.B) {
			tail := "```text\n" + strings.Repeat("streaming text ", size/15)
			var stream Stream
			stream.Feed(tail)
			b.ReportAllocs()
			b.ReportMetric(float64(len(tail)), "tail-bytes/update")
			for b.Loop() {
				// Feed invalidates this cache after appending a chunk. Invalidating it
				// directly holds the tail size constant, so Go's adaptive benchmark
				// calibration measures the render rather than an ever-growing input.
				stream.fresh = false
				benchmarkBlocks = stream.Open()
			}
		})
	}
}

// BenchmarkOpenMarkdownCachedRead separates parsing from the defensive ownership
// copy returned to callers. Measure and Draw may ask for the same open tail in one
// frame; the second call must not parse it again.
func BenchmarkOpenMarkdownCachedRead(b *testing.B) {
	var stream Stream
	stream.Feed("```text\n" + strings.Repeat("streaming text ", 4<<10/15))
	stream.Open()

	b.ReportAllocs()
	for b.Loop() {
		benchmarkBlocks = stream.Open()
	}
}
