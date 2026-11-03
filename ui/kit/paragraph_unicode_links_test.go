package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/link"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestParagraphKeepsAUnicodeFileIdentityAcrossWrapping(t *testing.T) {
	const target = "/tmp/目录/报告.go"
	const source = target + ":42"
	const hyperlink = "file:///tmp/%E7%9B%AE%E5%BD%95/%E6%8A%A5%E5%91%8A.go"
	paragraph := kit.NewParagraph(source, grid.Style{})
	paragraph.SetLinks(kit.LinkConfig{Enabled: true, Exists: func(path string) bool {
		t.Fatalf("absolute path caused a filesystem lookup for %q", path)
		return false
	}})
	for _, width := range []int{8, 40} {
		height := paragraph.HeightForWidth(width)
		surface := grid.NewSurface(width, height)
		paragraph.Draw(surface.View())
		linked := 0
		for y := range height {
			for x := range width {
				destination, ok := paragraph.LinkAt(x, y, width)
				cell := cellAt(surface.View(), x, y)
				if !ok {
					if cell.Link != "" {
						t.Fatalf("unaddressable hyperlink at (%d,%d): %q", x, y, cell.Link)
					}
					continue
				}
				linked++
				if destination.Kind != link.File || destination.Target != target || destination.Line != 42 || destination.Text(source) != source || cell.Link != hyperlink {
					t.Fatalf("width=%d at (%d,%d): destination=%+v hyperlink=%q", width, x, y, destination, cell.Link)
				}
			}
		}
		if linked == 0 {
			t.Fatalf("width=%d lost every file link", width)
		}
	}
}
