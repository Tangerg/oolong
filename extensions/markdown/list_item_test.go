package markdown_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestListMarkersSurviveInvisibleChildren(t *testing.T) {
	appearance := look()
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) {
		return text.NewBlock(text.BlockConfig{}), nil
	})
	for _, test := range []struct {
		name, source string
		want         []string
	}{
		{"ordered paragraph", "1. ```empty\n   ```\n\n   visible", []string{"1. visible"}},
		{"unordered paragraph", "- ```empty\n  ```\n\n  visible", []string{"• visible"}},
		{"empty first item", "1. ```empty\n   ```\n2. second", []string{"1.", "2. second"}},
		{"all children empty", "1. ```empty\n   ```\n\n   ```empty\n   ```", []string{"1."}},
		{"quoted empty item", "> 1. ```empty\n>    ```\n> 2. second", []string{"│ 1.", "│ 2. second"}},
		{"nested first visible item", "1. ```empty\n   ```\n\n   - nested\n\n   tail", []string{"1. • nested", "", "   tail"}},
		{"nested empty item", "1. - ```empty\n     ```", []string{"1. •"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			blocks := mustRender(t, test.source, appearance)
			equal(t, rows(t, 30, blocks), test.want)
			var doc markdown.Doc
			doc.SetBlocks(blocks)
			projected := doc.Rows(30)
			if len(projected) != len(test.want) {
				t.Fatalf("projected %d rows for %d drawn rows", len(projected), len(test.want))
			}
			for _, row := range projected {
				if strings.ContainsAny(row.Content.String(), "•│") || strings.Contains(row.Content.String(), "1.") {
					t.Fatalf("list furniture became searchable content: %+v", row)
				}
			}
		})
	}
}

type widthDependentContent struct{ minimum int }

func (content widthDependentContent) HeightForWidth(width int) int {
	if width < content.minimum {
		return 0
	}
	return 1
}

func (content widthDependentContent) Draw(view grid.View) {
	width, _ := view.Size()
	if content.HeightForWidth(width) > 0 {
		view.Text(0, 0, "wide", grid.Style{})
	}
}

func (content widthDependentContent) Rows(width int) []text.Row {
	if content.HeightForWidth(width) == 0 {
		return nil
	}
	return []text.Row{{Content: text.Of("wide", grid.Style{})}}
}

func TestListMarkerPlacementFollowsTheCurrentWidth(t *testing.T) {
	appearance := look()
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) {
		return widthDependentContent{minimum: 4}, nil
	})
	blocks := mustRender(t, "1. ```custom\n   ```\n\n   end", appearance)
	var doc markdown.Doc
	doc.SetBlocks(blocks)
	for _, width := range []int{7, 6, 9, 6, 0, 1, 7} {
		var want []string
		var projected []text.Row
		switch width {
		case 0:
		case 1:
			want = []string{"e", "n", "d"}
			projected = []text.Row{{Content: text.Of("e", grid.Style{})}, {Content: text.Of("n", grid.Style{}), Joined: true}, {Content: text.Of("d", grid.Style{}), Joined: true}}
		case 6:
			want = []string{"1. end"}
			projected = []text.Row{{Content: text.Of("end", grid.Style{}), Offset: 3}}
		default:
			want = []string{"1. wide", "", "   end"}
			projected = []text.Row{{Content: text.Of("wide", grid.Style{}), Offset: 3}, {}, {Content: text.Of("end", grid.Style{}), Offset: 3}}
		}
		equal(t, rows(t, width, blocks), want)
		if got := doc.Rows(width); !sameTextRows(got, projected) {
			t.Fatalf("width %d: projection %+v, want %+v", width, got, projected)
		}
		if got := doc.HeightForWidth(width); got != len(projected) {
			t.Fatalf("width %d: height %d, want %d", width, got, len(projected))
		}
	}
}

func TestAnEntirelyInvisibleItemProjectsOnlyItsBlankContentColumn(t *testing.T) {
	appearance := look()
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) {
		return widthDependentContent{minimum: 10}, nil
	})
	blocks := mustRender(t, "> 1. ```custom\n>    ```", appearance)
	for _, width := range []int{8, 15, 8, 1, 0} {
		block := blocks[0]
		projected := block.Rows(width)
		if width == 0 {
			if len(projected) != 0 || block.HeightForWidth(width) != 0 {
				t.Fatal("zero-width item created a visible row")
			}
			continue
		}
		want := text.Row{Offset: min(5, width-1)}
		if width >= 15 {
			want.Content = text.Of("wide", grid.Style{})
		}
		if !sameTextRows(projected, []text.Row{want}) || block.HeightForWidth(width) != 1 {
			t.Fatalf("width %d: rows %+v, height %d", width, projected, block.HeightForWidth(width))
		}
	}
}

func TestRetainedListItemKeepsItsChildrenAfterDocumentReplacement(t *testing.T) {
	blocks := mustRender(t, "1. first\n\n   second\n\n2. other", look())
	if len(blocks) != 2 {
		t.Fatalf("list yielded %d blocks, want two complete items", len(blocks))
	}
	retained := blocks[0]
	var original markdown.Doc
	original.SetBlocks(blocks)
	original.SetBlocks(mustRender(t, "replacement", look()))
	blocks[0] = blocks[1]
	want := []string{"1. first", "", "   second"}
	equal(t, rows(t, 20, []markdown.Block{retained}), want)
	surface := grid.NewSurface(20, retained.HeightForWidth(20))
	retained.Draw(surface.View())
	if cellAt(surface, 0, 0).Content() != "1" || cellAt(surface, 3, 2).Content() != "s" {
		t.Fatal("retained item could not draw independently of its original document")
	}
}

func TestNestedExtensionErrorsIdentifyTheContainingResultBlock(t *testing.T) {
	failure := errors.New("partial extension")
	appearance := look()
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) {
		return text.NewBlock(text.BlockConfig{Lines: []text.Line{text.Of("partial", grid.Style{})}}), failure
	})
	for _, test := range []struct {
		source string
		index  int
	}{
		{"before\n\n1. ```custom\n   source\n   ```\n\n   tail", 1},
		{"before\n\n1. - ```custom\n     source\n     ```", 1},
		{"1. first\n2. ```custom\n   source\n   ```", 1},
		{"1. first\n2. second\n\n```custom\nsource\n```", 2},
	} {
		blocks, err := markdown.Render(test.source, appearance)
		var located *markdown.ExtensionError
		if !errors.Is(err, failure) || !errors.As(err, &located) || located.Block != test.index {
			t.Fatalf("source %q: diagnostic %+v, want result block %d", test.source, located, test.index)
		}
		var content strings.Builder
		for _, row := range blocks[located.Block].Rows(30) {
			content.WriteString(row.Content.String())
		}
		if !strings.Contains(content.String(), "partial") {
			t.Fatalf("diagnostic points away from partial content: %q", content.String())
		}
	}
}

func sameTextRows(a, b []text.Row) bool {
	return slices.EqualFunc(a, b, func(a, b text.Row) bool {
		return a.Content.String() == b.Content.String() && a.Content.Width() == b.Content.Width() &&
			a.Offset == b.Offset && a.SourceLine == b.SourceLine && a.Joined == b.Joined && a.Gap == b.Gap
	})
}
