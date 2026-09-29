package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
)

func TestFilterHitUsesRankedIndicesWithoutChangingSelectionOrScroll(t *testing.T) {
	var filter headless.Filter[string]
	assertRowHit(t, filter.Hit, image.Pt(0, 0), -1)
	filter.SetItems([]string{"other", "a", "ab", "ac", "zz"}, func(s string) string { return s })
	filter.SetPattern("a")
	filter.Select(2)
	root := headless.NewRoot(&filter)
	root.Draw(grid.NewSurface(4, 2).View())
	assertRowHit(t, filter.Hit, image.Pt(0, 0), 1)
	if filter.Selected() != 2 || filter.Scroll().Offset() != 1 {
		t.Fatal("filter hit changed selection or scroll")
	}
	if got, _ := filter.Current(); got != "ac" {
		t.Fatalf("selection identifies %q, want ac", got)
	}
	filter.Scroll().ToTop()
	assertRowHit(t, filter.Hit, image.Pt(0, 0), 1)
	if filter.Scroll().Offset() != 0 || filter.Selected() != 2 {
		t.Fatal("filter hit overrode pending manual scrolling")
	}
	root.Draw(grid.NewSurface(4, 2).View())
	assertRowHit(t, filter.Hit, image.Pt(0, 0), 0)
}

func TestFilterHitSharesTheInnerListClipping(t *testing.T) {
	var filter headless.Filter[string]
	filter.SetItems([]string{"zero", "one", "two", "three", "four"}, func(s string) string { return s })
	view := grid.NewSurface(4, 2).View().Sub(image.Rect(-2, -1, 6, 4))
	headless.NewRoot(&filter).Draw(view)
	for _, test := range []struct {
		point image.Point
		want  int
	}{
		{point: image.Pt(2, 1), want: 1},
		{point: image.Pt(5, 2), want: 2},
		{point: image.Pt(1, 2), want: -1},
		{point: image.Pt(6, 2), want: -1},
		{point: image.Pt(3, 0), want: -1},
		{point: image.Pt(3, 3), want: -1},
	} {
		assertRowHit(t, filter.Hit, test.point, test.want)
	}
}

func TestFilterHitRejectsUnpresentedMatches(t *testing.T) {
	for _, replaceSource := range []bool{false, true} {
		var filter headless.Filter[string]
		calls := 0
		text := func(s string) string {
			calls++
			return s
		}
		filter.SetItems([]string{"alpha", "beta"}, text)
		filter.SetPattern("a")
		probe := &hitFrameProbe{child: &filter}
		root := headless.NewRoot(probe)
		view := grid.NewSurface(4, 3).View()
		root.Draw(view)
		assertRowHit(t, filter.Hit, image.Pt(0, 1), 1)
		if replaceSource {
			filter.SetItems([]string{"gamma"}, text)
		} else {
			filter.SetPattern("beta")
		}
		before := calls
		assertRowHit(t, filter.Hit, image.Pt(0, 0), -1)
		probe.inspect = func() {
			assertRowHit(t, filter.Hit, image.Pt(0, 0), -1)
		}
		root.Draw(view)
		assertRowHit(t, filter.Hit, image.Pt(0, 0), 0)
		assertRowHit(t, filter.Hit, image.Pt(0, 1), -1)
		if calls != before || filter.Selected() != 0 || filter.Scroll().Offset() != 0 {
			t.Fatal("hit queries or drawing rebuilt matches or advanced filter state")
		}
	}
}
