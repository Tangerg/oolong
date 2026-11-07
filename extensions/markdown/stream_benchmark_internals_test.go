package markdown

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

var (
	benchmarkDocument grid.Drawable
	benchmarkRows     []text.Row
	benchmarkHeight   int
)

func BenchmarkMarkdownSnapshotParsing(b *testing.B) {
	for _, size := range []int{256, 4 << 10, 64 << 10} {
		b.Run(fmt.Sprintf("source=%d", size), func(b *testing.B) {
			source := "```text\n" + strings.Repeat("streaming text ", size/15)
			renderer := NewRenderer(RendererConfig{})
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			for b.Loop() {
				var err error
				benchmarkDocument, err = renderer(context.Background(), source)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMarkdownPreparationPhases(b *testing.B) {
	const width, resizedWidth, viewportRows = 100, 80, 30
	for _, size := range []int{4 << 10, 16 << 10, 64 << 10, 256 << 10} {
		source := benchmarkMixedSource(size)
		b.Run(fmt.Sprintf("bytes=%d", len(source)), func(b *testing.B) {
			render := NewRenderer(RendererConfig{})
			parse := func(b *testing.B) *Snapshot {
				b.Helper()
				body, err := render(b.Context(), source)
				if err != nil {
					b.Fatal(err)
				}
				return body.(*Snapshot)
			}
			warm := func(b *testing.B) *Snapshot {
				b.Helper()
				doc := parse(b)
				if err := doc.PrepareWidth(b.Context(), width); err != nil {
					b.Fatal(err)
				}
				return doc
			}
			b.Run("parse", func(b *testing.B) {
				b.SetBytes(int64(len(source)))
				b.ReportAllocs()
				for b.Loop() {
					benchmarkDocument = parse(b)
				}
			})
			b.Run("geometry-cold", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					b.StopTimer()
					doc := parse(b)
					b.StartTimer()
					benchmarkHeight = doc.HeightForWidth(width)
				}
			})
			b.Run("text-rows-cold", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					b.StopTimer()
					doc := parse(b)
					doc.HeightForWidth(width)
					b.StartTimer()
					benchmarkRows = doc.Rows(width)
				}
			})
			b.Run("parse-and-prepare", func(b *testing.B) {
				b.SetBytes(int64(len(source)))
				b.ReportAllocs()
				for b.Loop() {
					benchmarkDocument = warm(b)
				}
			})
			b.Run("measure-warm", func(b *testing.B) {
				doc := warm(b)
				b.ReportAllocs()
				for b.Loop() {
					benchmarkHeight = doc.HeightForWidth(width)
				}
			})
			b.Run("text-rows-warm", func(b *testing.B) {
				doc := warm(b)
				b.ReportAllocs()
				for b.Loop() {
					benchmarkRows = doc.Rows(width)
				}
			})
			b.Run("viewport-draw-warm", func(b *testing.B) {
				doc := warm(b)
				surface := grid.NewSurface(width, viewportRows)
				b.ReportAllocs()
				for b.Loop() {
					doc.Draw(surface.View())
				}
			})
			b.Run("resize-and-prepare", func(b *testing.B) {
				doc := warm(b)
				nextWidth := resizedWidth
				b.ReportAllocs()
				for b.Loop() {
					if err := doc.PrepareWidth(b.Context(), nextWidth); err != nil {
						b.Fatal(err)
					}
					if nextWidth == width {
						nextWidth = resizedWidth
					} else {
						nextWidth = width
					}
				}
			})
		})
	}
}

func benchmarkMixedSource(size int) string {
	const section = "## Step\n\nThis paragraph explains a change with `inline code`, **bold** text and a [link](https://example.com).\n" +
		"It keeps going for a while so wrapping has something to do across a typical terminal width.\n\n" +
		"- first item with some words\n- second item with `code`\n  - nested item\n\n" +
		"```go\nfunc main() {\n\tfmt.Println(\"hello\")\n\tfor i := 0; i < 10; i++ {\n\t\tfmt.Println(i)\n\t}\n}\n```\n\n"
	var source strings.Builder
	for source.Len() < size {
		source.WriteString(section)
	}
	return source.String()
}
