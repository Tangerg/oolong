package headless

import (
	"fmt"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

type benchmarkBlock struct{ bytes int }

func (b benchmarkBlock) Measure(width int) int {
	if width <= 0 {
		return 0
	}
	return max((b.bytes+width-1)/width, 1)
}

func (benchmarkBlock) Draw(grid.View) {}

// BenchmarkTranscriptCommittedStream measures steady-state ownership transfer: one
// completed block arrives, is measured, and is immediately committed. The companion
// heap tests prove that this loop does not retain a session-length object graph;
// this benchmark reports the ongoing time and allocation cost per transferred block.
func BenchmarkTranscriptCommittedStream(b *testing.B) {
	var transcript Transcript
	transcript.Resize(80)
	block := benchmarkBlock{bytes: 4 << 10}

	b.ReportAllocs()
	for b.Loop() {
		id := transcript.Append(block)
		transcript.Finish(id)
		if committed := transcript.Commit(func(Block, int) bool { return true }); committed != 1 {
			b.Fatalf("committed %d blocks, want 1", committed)
		}
	}
}

// BenchmarkTranscriptRetainedResize measures the real frame transaction used by a
// retained transcript. Alternating widths forces a complete reflow; the block count
// scale makes accidental work proportional to session history visible.
func BenchmarkTranscriptRetainedResize(b *testing.B) {
	for _, blocks := range []int{100, 10_000} {
		b.Run(fmt.Sprintf("blocks=%d", blocks), func(b *testing.B) {
			var transcript Transcript
			for range blocks {
				transcript.Append(benchmarkBlock{bytes: 240})
			}
			widget := &benchmarkTranscript{transcript: &transcript, width: 80}
			root := NewRoot(widget)
			surface := grid.NewSurface(80, 24)
			root.Draw(surface.View())

			b.ReportAllocs()
			b.ReportMetric(float64(blocks), "blocks/frame")
			for b.Loop() {
				if widget.width == 80 {
					widget.width = 79
				} else {
					widget.width = 80
				}
				root.Draw(surface.View())
			}
		})
	}
}

type benchmarkTranscript struct {
	transcript *Transcript
	width      int
}

func (w *benchmarkTranscript) Draw(frame Frame) {
	w.transcript.Stage(frame, w.width)
}
