package markdown

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

var benchmarkDocument grid.Drawable

func BenchmarkMarkdownSnapshotPreparation(b *testing.B) {
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
