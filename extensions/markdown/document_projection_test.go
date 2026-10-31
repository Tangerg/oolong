package markdown_test

import (
	"math"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
)

type measuringChild func(int) int

func (child measuringChild) HeightForWidth(width int) int { return child(width) }
func (measuringChild) Draw(grid.View)                     {}

func TestLayoutCannotPublishAgainstReplacedBlocks(t *testing.T) {
	var doc markdown.Doc
	replacement := mustRender(t, "replacement\n\nnext", markdown.Look{})
	var appearance markdown.Look
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) {
		return measuringChild(func(int) int {
			doc.SetBlocks(replacement)
			return 2
		}), nil
	})
	doc.SetBlocks(mustRender(t, "~~~custom\nold\n~~~", appearance))
	if height := doc.HeightForWidth(20); height != 2 {
		t.Fatal("measurement did not finish against its original block sequence")
	}
	rows := doc.Rows(20)
	if doc.Len() != 2 || len(rows) != 3 || rows[0].Text != "replacement" || rows[2].Text != "next" {
		t.Fatalf("after replacement: %d logical blocks, rows %+v", doc.Len(), rows)
	}
}

func TestDocumentRejectsUnrepresentableCombinedHeight(t *testing.T) {
	var appearance markdown.Look
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) {
		return measuringChild(func(width int) int {
			if width == 1 {
				return math.MaxInt
			}
			return 1
		}), nil
	})
	var doc markdown.Doc
	doc.SetBlocks(mustRender(t, "~~~custom\nold\n~~~\n\nend", appearance))
	want := doc.HeightForWidth(10)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("accepted an overflowing document height")
			}
		}()
		doc.HeightForWidth(1)
	}()
	if got := doc.HeightForWidth(10); got != want || got != 3 {
		t.Fatalf("valid layout after refusal = %d, want 3", got)
	}
}

func TestPreparedWidthsCanBeReadConcurrently(t *testing.T) {
	body, err := markdown.NewRenderer(markdown.RendererConfig{})(t.Context(), "one two")
	if err != nil {
		t.Fatal(err)
	}
	var readers sync.WaitGroup
	for _, tc := range []struct {
		width, rows int
		last        string
	}{{3, 2, "t"}, {7, 1, "o"}} {
		readers.Go(func() {
			for range 100 {
				if got := body.HeightForWidth(tc.width); got != tc.rows {
					t.Errorf("width %d: height %d, want %d", tc.width, got, tc.rows)
					return
				}
				view := grid.NewSurface(tc.width, tc.rows)
				body.Draw(view.View())
				cell, _ := view.CellAt(0, tc.rows-1)
				if cell.Content() != tc.last {
					t.Errorf("width %d: last row starts with %q, want %q", tc.width, cell.Content(), tc.last)
					return
				}
			}
		})
	}
	readers.Wait()
}
