package markdown_test

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestPreparedLayoutRemainsImmutableAcrossWidthReplacement(t *testing.T) {
	body, err := markdown.NewRenderer(markdown.RendererConfig{})(t.Context(), "alpha beta gamma")
	if err != nil {
		t.Fatal(err)
	}
	doc := body.(*markdown.Snapshot)
	if err := doc.PrepareWidth(t.Context(), 5); err != nil {
		t.Fatal(err)
	}
	prepared, ready := doc.PreparedLayout(5)
	if !ready {
		t.Fatal("cold fixture")
	}
	before := captured(t, prepared, 5)
	if got := captureText(before); !slices.Equal(got, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("prepared rows = %q", got)
	}
	if err := doc.PrepareWidth(t.Context(), 20); err != nil {
		t.Fatal(err)
	}
	if doc.HeightForWidth(20) != 1 || prepared.HeightForWidth(20) != 3 {
		t.Fatal("a retained layout changed its wrapping width")
	}
	if after := captured(t, prepared, 5); !reflect.DeepEqual(after, before) {
		t.Fatal("a later width changed the previously prepared capture")
	}
	clipped := grid.NewSurface(3, 3)
	prepared.Draw(clipped.View())
	if got := clipped.Rows(); !slices.Equal(got, []string{"alp", "bet", "gam"}) {
		t.Fatalf("prepared layout reflowed instead of clipping: %q", got)
	}
}

func TestPreparedRowsCanBeReadWhileAnotherWidthIsPrepared(t *testing.T) {
	body, err := markdown.NewRenderer(markdown.RendererConfig{})(t.Context(), "alpha beta gamma")
	if err != nil {
		t.Fatal(err)
	}
	doc := body.(*markdown.Snapshot)
	var workers sync.WaitGroup
	for _, width := range []int{5, 20} {
		workers.Go(func() {
			for range 20 {
				if err := doc.PrepareWidth(t.Context(), width); err != nil {
					t.Error(err)
					return
				}
				rows := doc.Rows(width)
				var joined []string
				for _, row := range rows {
					joined = append(joined, row.Content.String())
				}
				if got := strings.Join(joined, " "); got != "alpha beta gamma" {
					t.Errorf("width %d: %q", width, got)
					return
				}
			}
		})
	}
	workers.Wait()
}

func captureText(rows [][]grid.Cell) []string {
	out := make([]string, len(rows))
	for index, row := range rows {
		var text strings.Builder
		for _, cell := range row {
			text.WriteString(cell.Content())
		}
		out[index] = strings.TrimRight(text.String(), " ")
	}
	return out
}
