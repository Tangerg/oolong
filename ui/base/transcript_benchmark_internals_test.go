package base

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

type benchmarkBlock struct{ bytes int }

func (b benchmarkBlock) HeightForWidth(width int) int {
	if width <= 0 {
		return 0
	}
	return max((b.bytes+width-1)/width, 1)
}
func (benchmarkBlock) Draw(grid.View) {}

func BenchmarkTranscriptCommittedStream(b *testing.B) {
	p := transcriptPublisher(b)
	transcript, err := NewTranscript(TranscriptConfig{Publisher: p, Retention: PublishCompleted})
	if err != nil {
		b.Fatal(err)
	}
	block := benchmarkBlock{bytes: 4 << 10}
	sequence := uint64(0)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := transcript.Add(block); err != nil {
			b.Fatal(err)
		}
		sequence++
		p.Commit(p.Prepare(), sequence)
		p.Written(sequence)
		p.FlushNotifications()
		if transcript.Len() != 0 {
			b.Fatal("completed content retained")
		}
	}
}

func BenchmarkTranscriptRetainedReflow(b *testing.B) {
	for _, blocks := range []int{100, 10_000} {
		b.Run(fmt.Sprintf("blocks=%d", blocks), func(b *testing.B) {
			transcript, err := NewTranscript(TranscriptConfig{MaxItems: blocks})
			if err != nil {
				b.Fatal(err)
			}
			for range blocks {
				if _, err = transcript.Add(benchmarkBlock{bytes: 240}); err != nil {
					b.Fatal(err)
				}
			}
			width := 80
			element := transcript.Element()
			if setErr := element.SetPaint(func(frame scene.Frame) { transcript.Stage(frame, width) }); setErr != nil {
				b.Fatal(setErr)
			}
			root, err := scene.NewRoot(scene.RootConfig{Element: element})
			if err != nil {
				b.Fatal(err)
			}
			defer root.Close()
			surface := grid.NewSurface(80, 24)
			sequence := uint64(0)
			b.ReportAllocs()
			b.ReportMetric(float64(blocks), "blocks/frame")
			for b.Loop() {
				width = 159 - width
				candidate, err := root.Prepare(surface.View())
				if err != nil {
					b.Fatal(err)
				}
				sequence++
				candidate.Commit(sequence)
				root.Wrote(sequence)
			}
		})
	}
}

func BenchmarkTranscriptSteadyTextProjection(b *testing.B) {
	for _, kib := range []int{4, 16, 64, 256} {
		b.Run(fmt.Sprintf("%dKiB", kib), func(b *testing.B) {
			transcript, err := NewTranscript(TranscriptConfig{})
			if err != nil {
				b.Fatal(err)
			}
			defer transcript.Close()
			line := strings.Repeat("abcdefghijklmnopqrstuvwxyz ", (kib*1024)/27+1)
			body := text.NewBlock(text.BlockConfig{Lines: []text.Line{text.Of(line, grid.Style{})}, Wrap: true})
			if _, addErr := transcript.Add(body); addErr != nil {
				b.Fatal(addErr)
			}
			element := transcript.Element()
			if paintErr := element.SetPaint(func(frame scene.Frame) { transcript.Stage(frame, 80) }); paintErr != nil {
				b.Fatal(paintErr)
			}
			root, err := scene.NewRoot(scene.RootConfig{Element: element})
			if err != nil {
				b.Fatal(err)
			}
			defer root.Close()
			surface := grid.NewSurface(80, 24)
			sequence := uint64(0)
			paint := func() {
				candidate, err := root.Prepare(surface.View())
				if err != nil {
					b.Fatal(err)
				}
				sequence++
				candidate.Commit(sequence)
				root.Wrote(sequence)
			}
			paint()
			b.ReportAllocs()
			for b.Loop() {
				paint()
			}
		})
	}
}
