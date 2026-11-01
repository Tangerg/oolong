package scene_test

import (
	"image"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestMeasuredGroupReservesWhatItsFlowWillAllocate(t *testing.T) {
	for _, axis := range []layout.Axis{layout.Down, layout.Across} {
		var painted image.Point
		child := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
			painted = frame.Visible().Size()
		}})
		inner := scene.NewGroup(scene.GroupConfig{Axis: axis, Gap: 1, Items: []scene.Item{
			{Size: layout.Fixed(3), Element: child},
			{Size: layout.Flex(1).AtLeast(2), Element: scene.NewElement(scene.ElementConfig{})},
		}})
		outer := scene.NewGroup(scene.GroupConfig{Axis: axis, Items: []scene.Item{
			{Size: layout.Measured(0, 0), Element: inner.Element()},
		}})
		draw(t, root(t, outer.Element()), 8, 8, 1)
		want := image.Pt(8, 3)
		if axis == layout.Across {
			want = image.Pt(3, 8)
		}
		if got := inner.Element().Measure(axis, 8); got != 6 || painted != want {
			t.Fatalf("axis %v: group asked for %d, child visible %v; want 6 and %v", axis, got, painted, want)
		}
	}
}

func BenchmarkObservePaintedList(b *testing.B) {
	items := make([]scene.Item, 2048)
	for i := range items {
		key := strconv.Itoa(i)
		items[i] = item(key, scene.NewElement(scene.ElementConfig{Key: key}))
	}
	group := scene.NewGroup(scene.GroupConfig{Items: items})
	r, err := scene.NewRoot(scene.RootConfig{Element: group.Element()})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(r.Close)
	frame, err := r.Prepare(grid.NewSurface(80, 24).View())
	if err != nil {
		b.Fatal(err)
	}
	frame.Commit(1)
	for b.Loop() {
		page, observeErr := r.Observe(scene.Query{Limit: len(items)})
		if observeErr != nil || len(page.Nodes) != len(items) {
			b.Fatal(observeErr, len(page.Nodes))
		}
	}
}
